package registry

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Global model registry instance
var globalRegistry *ModelRegistry
var registryOnce sync.Once

// GetGlobalRegistry returns the global model registry instance
func GetGlobalRegistry() *ModelRegistry {
	registryOnce.Do(func() {
		globalRegistry = &ModelRegistry{
			models:               make(map[string]*ModelRegistration),
			clientModels:         make(map[string][]string),
			clientModelInfos:     make(map[string]map[string]*ModelInfo),
			clientProviders:      make(map[string]string),
			availableModelsCache: make(map[string]availableModelsCacheEntry),
			mutex:                &sync.RWMutex{},
		}
	})
	return globalRegistry
}

func (r *ModelRegistry) ensureAvailableModelsCacheLocked() {
	if r.availableModelsCache == nil {
		r.availableModelsCache = make(map[string]availableModelsCacheEntry)
	}
}

func (r *ModelRegistry) invalidateAvailableModelsCacheLocked() {
	if len(r.availableModelsCache) == 0 {
		return
	}
	clear(r.availableModelsCache)
}

// SetHook sets an optional hook for observing model registration changes.
func (r *ModelRegistry) SetHook(hook ModelRegistryHook) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.hook = hook
}

const defaultModelRegistryHookTimeout = 5 * time.Second
const modelQuotaExceededWindow = 5 * time.Minute

func (r *ModelRegistry) triggerModelsRegistered(provider, clientID string, models []*ModelInfo) {
	hook := r.hook
	if hook == nil {
		return
	}
	modelsCopy := cloneModelInfosUnique(models)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("model registry hook OnModelsRegistered panic: %v", recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), defaultModelRegistryHookTimeout)
		defer cancel()
		hook.OnModelsRegistered(ctx, provider, clientID, modelsCopy)
	}()
}

func (r *ModelRegistry) triggerModelsUnregistered(provider, clientID string) {
	hook := r.hook
	if hook == nil {
		return
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Errorf("model registry hook OnModelsUnregistered panic: %v", recovered)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), defaultModelRegistryHookTimeout)
		defer cancel()
		hook.OnModelsUnregistered(ctx, provider, clientID)
	}()
}

func cloneModelInfo(model *ModelInfo) *ModelInfo {
	if model == nil {
		return nil
	}
	copyModel := *model
	if len(model.SupportedGenerationMethods) > 0 {
		copyModel.SupportedGenerationMethods = append([]string(nil), model.SupportedGenerationMethods...)
	}
	if len(model.SupportedParameters) > 0 {
		copyModel.SupportedParameters = append([]string(nil), model.SupportedParameters...)
	}
	if len(model.SupportedInputModalities) > 0 {
		copyModel.SupportedInputModalities = append([]string(nil), model.SupportedInputModalities...)
	}
	if len(model.SupportedOutputModalities) > 0 {
		copyModel.SupportedOutputModalities = append([]string(nil), model.SupportedOutputModalities...)
	}
	if model.Thinking != nil {
		copyThinking := *model.Thinking
		if len(model.Thinking.Levels) > 0 {
			copyThinking.Levels = append([]string(nil), model.Thinking.Levels...)
		}
		copyModel.Thinking = &copyThinking
	}
	return &copyModel
}

func cloneModelInfosUnique(models []*ModelInfo) []*ModelInfo {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]*ModelInfo, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if model == nil || model.ID == "" {
			continue
		}
		if _, exists := seen[model.ID]; exists {
			continue
		}
		seen[model.ID] = struct{}{}
		cloned = append(cloned, cloneModelInfo(model))
	}
	return cloned
}
