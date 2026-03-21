// Package registry provides centralized model management for all AI service providers.
// It implements a dynamic model registry with reference counting to track active clients
// and automatically hide models when no clients are available or when quota is exceeded.
package registry

import (
	"context"
	"strings"
	"sync"
	"time"

	misc "github.com/coachpo/cockpit-backend/internal/misc"
	log "github.com/sirupsen/logrus"
)

// ModelInfo represents information about an available model
type ModelInfo struct {
	// ID is the unique identifier for the model
	ID string `json:"id"`
	// Object type for the model (typically "model")
	Object string `json:"object"`
	// Created timestamp when the model was created
	Created int64 `json:"created"`
	// OwnedBy indicates the organization that owns the model
	OwnedBy string `json:"owned_by"`
	// Type indicates the model type (e.g., "codex", "openai")
	Type string `json:"type"`
	// DisplayName is the human-readable name for the model
	DisplayName string `json:"display_name,omitempty"`
	// Name is used for Gemini-style model names
	Name string `json:"name,omitempty"`
	// Version is the model version
	Version string `json:"version,omitempty"`
	// Description provides detailed information about the model
	Description string `json:"description,omitempty"`
	// InputTokenLimit is the maximum input token limit
	InputTokenLimit int `json:"inputTokenLimit,omitempty"`
	// OutputTokenLimit is the maximum output token limit
	OutputTokenLimit int `json:"outputTokenLimit,omitempty"`
	// SupportedGenerationMethods lists supported generation methods
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
	// ContextLength is the context window size
	ContextLength int `json:"context_length,omitempty"`
	// MaxCompletionTokens is the maximum completion tokens
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`
	// SupportedParameters lists supported parameters
	SupportedParameters []string `json:"supported_parameters,omitempty"`
	// SupportedInputModalities lists supported input modalities (e.g., TEXT, IMAGE, VIDEO, AUDIO)
	SupportedInputModalities []string `json:"supportedInputModalities,omitempty"`
	// SupportedOutputModalities lists supported output modalities (e.g., TEXT, IMAGE)
	SupportedOutputModalities []string `json:"supportedOutputModalities,omitempty"`

	// Thinking holds provider-specific reasoning/thinking budget capabilities.
	// This is optional and currently used for Gemini thinking budget normalization.
	Thinking *ThinkingSupport `json:"thinking,omitempty"`

	// UserDefined indicates this model was defined through config file's models[]
	// array (e.g., openai-compatibility.*.models[], *-api-key.models[]).
	// UserDefined models have thinking configuration passed through without validation.
	UserDefined bool `json:"-"`
}

type availableModelsCacheEntry struct {
	models    []map[string]any
	expiresAt time.Time
}

// ThinkingSupport describes a model family's supported internal reasoning budget range.
// Values are interpreted in provider-native token units.
type ThinkingSupport struct {
	// Min is the minimum allowed thinking budget (inclusive).
	Min int `json:"min,omitempty"`
	// Max is the maximum allowed thinking budget (inclusive).
	Max int `json:"max,omitempty"`
	// ZeroAllowed indicates whether 0 is a valid value (to disable thinking).
	ZeroAllowed bool `json:"zero_allowed,omitempty"`
	// DynamicAllowed indicates whether -1 is a valid value (dynamic thinking budget).
	DynamicAllowed bool `json:"dynamic_allowed,omitempty"`
	// Levels defines discrete reasoning effort levels (e.g., "low", "medium", "high").
	// When set, the model uses level-based reasoning instead of token budgets.
	Levels []string `json:"levels,omitempty"`
}

// ModelRegistration tracks a model's availability
type ModelRegistration struct {
	// Info contains the model metadata
	Info *ModelInfo
	// InfoByProvider maps provider identifiers to specific ModelInfo to support differing capabilities.
	InfoByProvider map[string]*ModelInfo
	// Count is the number of active clients that can provide this model
	Count int
	// LastUpdated tracks when this registration was last modified
	LastUpdated time.Time
	// QuotaExceededClients tracks which clients have exceeded quota for this model
	QuotaExceededClients map[string]*time.Time
	// Providers tracks available clients grouped by provider identifier
	Providers map[string]int
	// SuspendedClients tracks temporarily disabled clients keyed by client ID
	SuspendedClients map[string]string
}

// ModelRegistryHook provides optional callbacks for external integrations to track model list changes.
// Hook implementations must be non-blocking and resilient; calls are executed asynchronously and panics are recovered.
type ModelRegistryHook interface {
	OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo)
	OnModelsUnregistered(ctx context.Context, provider, clientID string)
}

// ModelRegistry manages the global registry of available models
type ModelRegistry struct {
	// models maps model ID to registration information
	models map[string]*ModelRegistration
	// clientModels maps client ID to the models it provides
	clientModels map[string][]string
	// clientModelInfos maps client ID to a map of model ID -> ModelInfo
	// This preserves the original model info provided by each client
	clientModelInfos map[string]map[string]*ModelInfo
	// clientProviders maps client ID to its provider identifier
	clientProviders map[string]string
	// mutex ensures thread-safe access to the registry
	mutex *sync.RWMutex
	// availableModelsCache stores per-handler snapshots for GetAvailableModels.
	availableModelsCache map[string]availableModelsCacheEntry
	// hook is an optional callback sink for model registration changes
	hook ModelRegistryHook
}

// LookupModelInfo searches dynamic registry (provider-specific > global) then static definitions.
func LookupModelInfo(modelID string, provider ...string) *ModelInfo {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}

	p := ""
	if len(provider) > 0 {
		p = strings.ToLower(strings.TrimSpace(provider[0]))
	}

	if info := GetGlobalRegistry().GetModelInfo(modelID, p); info != nil {
		return cloneModelInfo(info)
	}
	return cloneModelInfo(LookupStaticModelInfo(modelID))
}

// RegisterClient registers a client and its supported models
// Parameters:
//   - clientID: Unique identifier for the client
//   - clientProvider: Provider name (e.g., "codex", "openai")
//   - models: List of models that this client can provide
func (r *ModelRegistry) RegisterClient(clientID, clientProvider string, models []*ModelInfo) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	provider := strings.ToLower(clientProvider)
	uniqueModelIDs := make([]string, 0, len(models))
	rawModelIDs := make([]string, 0, len(models))
	newModels := make(map[string]*ModelInfo, len(models))
	newCounts := make(map[string]int, len(models))
	for _, model := range models {
		if model == nil || model.ID == "" {
			continue
		}
		rawModelIDs = append(rawModelIDs, model.ID)
		newCounts[model.ID]++
		if _, exists := newModels[model.ID]; exists {
			continue
		}
		newModels[model.ID] = model
		uniqueModelIDs = append(uniqueModelIDs, model.ID)
	}

	if len(uniqueModelIDs) == 0 {
		// No models supplied; unregister existing client state if present.
		r.unregisterClientInternal(clientID)
		delete(r.clientModels, clientID)
		delete(r.clientModelInfos, clientID)
		delete(r.clientProviders, clientID)
		r.invalidateAvailableModelsCacheLocked()
		misc.LogCredentialSeparator()
		return
	}

	now := time.Now()

	oldModels, hadExisting := r.clientModels[clientID]
	oldProvider := r.clientProviders[clientID]
	providerChanged := oldProvider != provider
	if !hadExisting {
		// Pure addition path.
		for _, modelID := range rawModelIDs {
			model := newModels[modelID]
			r.addModelRegistration(modelID, provider, model, now)
		}
		r.clientModels[clientID] = append([]string(nil), rawModelIDs...)
		// Store client's own model infos
		clientInfos := make(map[string]*ModelInfo, len(newModels))
		for id, m := range newModels {
			clientInfos[id] = cloneModelInfo(m)
		}
		r.clientModelInfos[clientID] = clientInfos
		if provider != "" {
			r.clientProviders[clientID] = provider
		} else {
			delete(r.clientProviders, clientID)
		}
		r.invalidateAvailableModelsCacheLocked()
		r.triggerModelsRegistered(provider, clientID, models)
		log.Debugf("Registered client %s from provider %s with %d models", clientID, clientProvider, len(rawModelIDs))
		misc.LogCredentialSeparator()
		return
	}

	oldCounts := make(map[string]int, len(oldModels))
	for _, id := range oldModels {
		oldCounts[id]++
	}

	added := make([]string, 0)
	for _, id := range uniqueModelIDs {
		if oldCounts[id] == 0 {
			added = append(added, id)
		}
	}

	removed := make([]string, 0)
	for id := range oldCounts {
		if newCounts[id] == 0 {
			removed = append(removed, id)
		}
	}

	// Handle provider change for overlapping models before modifications.
	if providerChanged && oldProvider != "" {
		for id, newCount := range newCounts {
			if newCount == 0 {
				continue
			}
			oldCount := oldCounts[id]
			if oldCount == 0 {
				continue
			}
			toRemove := newCount
			if oldCount < toRemove {
				toRemove = oldCount
			}
			if reg, ok := r.models[id]; ok && reg.Providers != nil {
				if count, okProv := reg.Providers[oldProvider]; okProv {
					if count <= toRemove {
						delete(reg.Providers, oldProvider)
						if reg.InfoByProvider != nil {
							delete(reg.InfoByProvider, oldProvider)
						}
					} else {
						reg.Providers[oldProvider] = count - toRemove
					}
				}
			}
		}
	}

	// Apply removals first to keep counters accurate.
	for _, id := range removed {
		oldCount := oldCounts[id]
		for i := 0; i < oldCount; i++ {
			r.removeModelRegistration(clientID, id, oldProvider, now)
		}
	}

	for id, oldCount := range oldCounts {
		newCount := newCounts[id]
		if newCount == 0 || oldCount <= newCount {
			continue
		}
		overage := oldCount - newCount
		for i := 0; i < overage; i++ {
			r.removeModelRegistration(clientID, id, oldProvider, now)
		}
	}

	// Apply additions.
	for id, newCount := range newCounts {
		oldCount := oldCounts[id]
		if newCount <= oldCount {
			continue
		}
		model := newModels[id]
		diff := newCount - oldCount
		for i := 0; i < diff; i++ {
			r.addModelRegistration(id, provider, model, now)
		}
	}

	// Update metadata for models that remain associated with the client.
	addedSet := make(map[string]struct{}, len(added))
	for _, id := range added {
		addedSet[id] = struct{}{}
	}
	for _, id := range uniqueModelIDs {
		model := newModels[id]
		if reg, ok := r.models[id]; ok {
			reg.Info = cloneModelInfo(model)
			if provider != "" {
				if reg.InfoByProvider == nil {
					reg.InfoByProvider = make(map[string]*ModelInfo)
				}
				reg.InfoByProvider[provider] = cloneModelInfo(model)
			}
			reg.LastUpdated = now
			// Re-registering an existing client/model binding starts a fresh registry
			// snapshot for that binding. Cooldown and suspension are transient
			// scheduling state and must not survive this reconciliation step.
			if reg.QuotaExceededClients != nil {
				delete(reg.QuotaExceededClients, clientID)
			}
			if reg.SuspendedClients != nil {
				delete(reg.SuspendedClients, clientID)
			}
			if providerChanged && provider != "" {
				if _, newlyAdded := addedSet[id]; newlyAdded {
					continue
				}
				overlapCount := newCounts[id]
				if oldCount := oldCounts[id]; oldCount < overlapCount {
					overlapCount = oldCount
				}
				if overlapCount <= 0 {
					continue
				}
				if reg.Providers == nil {
					reg.Providers = make(map[string]int)
				}
				reg.Providers[provider] += overlapCount
			}
		}
	}

	// Update client bookkeeping.
	if len(rawModelIDs) > 0 {
		r.clientModels[clientID] = append([]string(nil), rawModelIDs...)
	}
	// Update client's own model infos
	clientInfos := make(map[string]*ModelInfo, len(newModels))
	for id, m := range newModels {
		clientInfos[id] = cloneModelInfo(m)
	}
	r.clientModelInfos[clientID] = clientInfos
	if provider != "" {
		r.clientProviders[clientID] = provider
	} else {
		delete(r.clientProviders, clientID)
	}

	r.invalidateAvailableModelsCacheLocked()
	r.triggerModelsRegistered(provider, clientID, models)
	if len(added) == 0 && len(removed) == 0 && !providerChanged {
		// Only metadata (e.g., display name) changed; skip separator when no log output.
		return
	}

	log.Debugf("Reconciled client %s (provider %s) models: +%d, -%d", clientID, provider, len(added), len(removed))
	misc.LogCredentialSeparator()
}

// ClientSupportsModel reports whether the client registered support for modelID.
func (r *ModelRegistry) ClientSupportsModel(clientID, modelID string) bool {
	clientID = strings.TrimSpace(clientID)
	modelID = strings.TrimSpace(modelID)
	if clientID == "" || modelID == "" {
		return false
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	models, exists := r.clientModels[clientID]
	if !exists || len(models) == 0 {
		return false
	}

	for _, id := range models {
		if strings.EqualFold(strings.TrimSpace(id), modelID) {
			return true
		}
	}

	return false
}
