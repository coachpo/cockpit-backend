package registry

import (
	"sort"
	"strings"
	"time"
)

// GetAvailableModels returns all models that have at least one available client
// Parameters:
//   - handlerType: The handler type to filter models for (e.g., "openai", "codex")
//
// Returns:
//   - []map[string]any: List of available models in the requested format
func (r *ModelRegistry) GetAvailableModels(handlerType string) []map[string]any {
	now := time.Now()

	r.mutex.RLock()
	if cache, ok := r.availableModelsCache[handlerType]; ok && (cache.expiresAt.IsZero() || now.Before(cache.expiresAt)) {
		models := cloneModelMaps(cache.models)
		r.mutex.RUnlock()
		return models
	}
	r.mutex.RUnlock()

	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	if cache, ok := r.availableModelsCache[handlerType]; ok && (cache.expiresAt.IsZero() || now.Before(cache.expiresAt)) {
		return cloneModelMaps(cache.models)
	}

	models, expiresAt := r.buildAvailableModelsLocked(handlerType, now)
	r.availableModelsCache[handlerType] = availableModelsCacheEntry{
		models:    cloneModelMaps(models),
		expiresAt: expiresAt,
	}

	return models
}

func (r *ModelRegistry) buildAvailableModelsLocked(handlerType string, now time.Time) ([]map[string]any, time.Time) {
	models := make([]map[string]any, 0, len(r.models))
	var expiresAt time.Time

	for _, registration := range r.models {
		availableClients := registration.Count

		expiredClients := 0
		for _, quotaTime := range registration.QuotaExceededClients {
			if quotaTime == nil {
				continue
			}
			recoveryAt := quotaTime.Add(modelQuotaExceededWindow)
			if now.Before(recoveryAt) {
				expiredClients++
				if expiresAt.IsZero() || recoveryAt.Before(expiresAt) {
					expiresAt = recoveryAt
				}
			}
		}

		cooldownSuspended := 0
		otherSuspended := 0
		if registration.SuspendedClients != nil {
			for _, reason := range registration.SuspendedClients {
				if strings.EqualFold(reason, "quota") {
					cooldownSuspended++
					continue
				}
				otherSuspended++
			}
		}

		effectiveClients := availableClients - expiredClients - otherSuspended
		if effectiveClients < 0 {
			effectiveClients = 0
		}

		if effectiveClients > 0 || (availableClients > 0 && (expiredClients > 0 || cooldownSuspended > 0) && otherSuspended == 0) {
			model := r.convertModelToMap(registration.Info, handlerType)
			if model != nil {
				models = append(models, model)
			}
		}
	}

	return models, expiresAt
}

func cloneModelMaps(models []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(models))
	for _, model := range models {
		if model == nil {
			cloned = append(cloned, nil)
			continue
		}
		copyModel := make(map[string]any, len(model))
		for key, value := range model {
			copyModel[key] = cloneModelMapValue(value)
		}
		cloned = append(cloned, copyModel)
	}
	return cloned
}

func cloneModelMapValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copyMap := make(map[string]any, len(typed))
		for key, entry := range typed {
			copyMap[key] = cloneModelMapValue(entry)
		}
		return copyMap
	case []any:
		copySlice := make([]any, len(typed))
		for i, entry := range typed {
			copySlice[i] = cloneModelMapValue(entry)
		}
		return copySlice
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

// GetAvailableModelsByProvider returns models available for the given provider identifier.
// Parameters:
//   - provider: Provider identifier (e.g., "codex", "openai")
//
// Returns:
//   - []*ModelInfo: List of available models for the provider
func (r *ModelRegistry) GetAvailableModelsByProvider(provider string) []*ModelInfo {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return nil
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()

	type providerModel struct {
		count int
		info  *ModelInfo
	}

	providerModels := make(map[string]*providerModel)

	for clientID, clientProvider := range r.clientProviders {
		if clientProvider != provider {
			continue
		}
		modelIDs := r.clientModels[clientID]
		if len(modelIDs) == 0 {
			continue
		}
		clientInfos := r.clientModelInfos[clientID]
		for _, modelID := range modelIDs {
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				continue
			}
			entry := providerModels[modelID]
			if entry == nil {
				entry = &providerModel{}
				providerModels[modelID] = entry
			}
			entry.count++
			if entry.info == nil {
				if clientInfos != nil {
					if info := clientInfos[modelID]; info != nil {
						entry.info = info
					}
				}
				if entry.info == nil {
					if reg, ok := r.models[modelID]; ok && reg != nil && reg.Info != nil {
						entry.info = reg.Info
					}
				}
			}
		}
	}

	if len(providerModels) == 0 {
		return nil
	}

	now := time.Now()
	result := make([]*ModelInfo, 0, len(providerModels))

	for modelID, entry := range providerModels {
		if entry == nil || entry.count <= 0 {
			continue
		}
		registration, ok := r.models[modelID]

		expiredClients := 0
		cooldownSuspended := 0
		otherSuspended := 0
		if ok && registration != nil {
			if registration.QuotaExceededClients != nil {
				for clientID, quotaTime := range registration.QuotaExceededClients {
					if clientID == "" {
						continue
					}
					if p, okProvider := r.clientProviders[clientID]; !okProvider || p != provider {
						continue
					}
					if quotaTime != nil && now.Sub(*quotaTime) < modelQuotaExceededWindow {
						expiredClients++
					}
				}
			}
			if registration.SuspendedClients != nil {
				for clientID, reason := range registration.SuspendedClients {
					if clientID == "" {
						continue
					}
					if p, okProvider := r.clientProviders[clientID]; !okProvider || p != provider {
						continue
					}
					if strings.EqualFold(reason, "quota") {
						cooldownSuspended++
						continue
					}
					otherSuspended++
				}
			}
		}

		availableClients := entry.count
		effectiveClients := availableClients - expiredClients - otherSuspended
		if effectiveClients < 0 {
			effectiveClients = 0
		}

		if effectiveClients > 0 || (availableClients > 0 && (expiredClients > 0 || cooldownSuspended > 0) && otherSuspended == 0) {
			if entry.info != nil {
				result = append(result, cloneModelInfo(entry.info))
				continue
			}
			if ok && registration != nil && registration.Info != nil {
				result = append(result, cloneModelInfo(registration.Info))
			}
		}
	}

	return result
}

// GetModelCount returns the number of available clients for a specific model
// Parameters:
//   - modelID: The model ID to check
//
// Returns:
//   - int: Number of available clients for the model
func (r *ModelRegistry) GetModelCount(modelID string) int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if registration, exists := r.models[modelID]; exists {
		now := time.Now()

		// Count clients that have exceeded quota but haven't recovered yet
		expiredClients := 0
		for _, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) < modelQuotaExceededWindow {
				expiredClients++
			}
		}
		suspendedClients := 0
		if registration.SuspendedClients != nil {
			suspendedClients = len(registration.SuspendedClients)
		}
		result := registration.Count - expiredClients - suspendedClients
		if result < 0 {
			return 0
		}
		return result
	}
	return 0
}

// GetModelProviders returns provider identifiers that currently supply the given model
// Parameters:
//   - modelID: The model ID to check
//
// Returns:
//   - []string: Provider identifiers ordered by availability count (descending)
func (r *ModelRegistry) GetModelProviders(modelID string) []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	registration, exists := r.models[modelID]
	if !exists || registration == nil || len(registration.Providers) == 0 {
		return nil
	}

	type providerCount struct {
		name  string
		count int
	}
	providers := make([]providerCount, 0, len(registration.Providers))
	for name, count := range registration.Providers {
		if count <= 0 {
			continue
		}
		providers = append(providers, providerCount{name: name, count: count})
	}
	if len(providers) == 0 {
		return nil
	}

	sort.Slice(providers, func(i, j int) bool {
		if providers[i].count == providers[j].count {
			return providers[i].name < providers[j].name
		}
		return providers[i].count > providers[j].count
	})

	result := make([]string, 0, len(providers))
	for _, item := range providers {
		result = append(result, item.name)
	}
	return result
}
