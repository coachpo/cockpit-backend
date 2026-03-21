package registry

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// SetModelQuotaExceeded marks a model as quota exceeded for a specific client
// Parameters:
//   - clientID: The client that exceeded quota
//   - modelID: The model that exceeded quota
func (r *ModelRegistry) SetModelQuotaExceeded(clientID, modelID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	if registration, exists := r.models[modelID]; exists {
		now := time.Now()
		registration.QuotaExceededClients[clientID] = &now
		r.invalidateAvailableModelsCacheLocked()
		log.Debugf("Marked model %s as quota exceeded for client %s", modelID, clientID)
	}
}

// ClearModelQuotaExceeded removes quota exceeded status for a model and client
// Parameters:
//   - clientID: The client to clear quota status for
//   - modelID: The model to clear quota status for
func (r *ModelRegistry) ClearModelQuotaExceeded(clientID, modelID string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	if registration, exists := r.models[modelID]; exists {
		delete(registration.QuotaExceededClients, clientID)
		r.invalidateAvailableModelsCacheLocked()
	}
}

// SuspendClientModel marks a client's model as temporarily unavailable until explicitly resumed.
// Parameters:
//   - clientID: The client to suspend
//   - modelID: The model affected by the suspension
//   - reason: Optional description for observability
func (r *ModelRegistry) SuspendClientModel(clientID, modelID, reason string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	registration, exists := r.models[modelID]
	if !exists || registration == nil {
		return
	}
	if registration.SuspendedClients == nil {
		registration.SuspendedClients = make(map[string]string)
	}
	if _, already := registration.SuspendedClients[clientID]; already {
		return
	}
	registration.SuspendedClients[clientID] = reason
	registration.LastUpdated = time.Now()
	r.invalidateAvailableModelsCacheLocked()
	if reason != "" {
		log.Debugf("Suspended client %s for model %s: %s", clientID, modelID, reason)
	} else {
		log.Debugf("Suspended client %s for model %s", clientID, modelID)
	}
}

// ResumeClientModel clears a previous suspension so the client counts toward availability again.
// Parameters:
//   - clientID: The client to resume
//   - modelID: The model being resumed
func (r *ModelRegistry) ResumeClientModel(clientID, modelID string) {
	if clientID == "" || modelID == "" {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.ensureAvailableModelsCacheLocked()

	registration, exists := r.models[modelID]
	if !exists || registration == nil || registration.SuspendedClients == nil {
		return
	}
	if _, ok := registration.SuspendedClients[clientID]; !ok {
		return
	}
	delete(registration.SuspendedClients, clientID)
	registration.LastUpdated = time.Now()
	r.invalidateAvailableModelsCacheLocked()
	log.Debugf("Resumed client %s for model %s", clientID, modelID)
}

// CleanupExpiredQuotas removes expired quota tracking entries
func (r *ModelRegistry) CleanupExpiredQuotas() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	invalidated := false

	for modelID, registration := range r.models {
		for clientID, quotaTime := range registration.QuotaExceededClients {
			if quotaTime != nil && now.Sub(*quotaTime) >= modelQuotaExceededWindow {
				delete(registration.QuotaExceededClients, clientID)
				invalidated = true
				log.Debugf("Cleaned up expired quota tracking for model %s, client %s", modelID, clientID)
			}
		}
	}
	if invalidated {
		r.invalidateAvailableModelsCacheLocked()
	}
}
