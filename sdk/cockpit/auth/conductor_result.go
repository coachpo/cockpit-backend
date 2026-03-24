package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/coachpo/cockpit-backend/internal/registry"
)

func (m *Manager) retrySettings() (int, int, time.Duration) {
	if m == nil {
		return 0, 0, 0
	}
	return int(m.requestRetry.Load()), int(m.maxRetryCredentials.Load()), time.Duration(m.maxRetryInterval.Load())
}

func (m *Manager) closestCooldownWait(providers []string, model string, attempt int) (time.Duration, bool) {
	if m == nil || len(providers) == 0 {
		return 0, false
	}
	now := time.Now()
	defaultRetry := int(m.requestRetry.Load())
	if defaultRetry < 0 {
		defaultRetry = 0
	}
	providerSet := make(map[string]struct{}, len(providers))
	for i := range providers {
		key := strings.TrimSpace(strings.ToLower(providers[i]))
		if key == "" {
			continue
		}
		providerSet[key] = struct{}{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var (
		found   bool
		minWait time.Duration
	)
	for _, auth := range m.auths {
		if auth == nil {
			continue
		}
		providerKey := strings.TrimSpace(strings.ToLower(auth.Provider))
		if _, ok := providerSet[providerKey]; !ok {
			continue
		}
		effectiveRetry := defaultRetry
		if override, ok := auth.RequestRetryOverride(); ok {
			effectiveRetry = override
		}
		if effectiveRetry < 0 {
			effectiveRetry = 0
		}
		if attempt >= effectiveRetry {
			continue
		}
		blocked, reason, next := isAuthBlockedForModel(auth, model, now)
		if !blocked || next.IsZero() || reason == blockReasonDisabled {
			continue
		}
		wait := next.Sub(now)
		if wait < 0 {
			continue
		}
		if !found || wait < minWait {
			minWait = wait
			found = true
		}
	}
	return minWait, found
}

func (m *Manager) shouldRetryAfterError(err error, attempt int, providers []string, model string, maxWait time.Duration) (time.Duration, bool) {
	if err == nil || maxWait <= 0 {
		return 0, false
	}
	if status := statusCodeFromError(err); status == http.StatusOK {
		return 0, false
	}
	if isRequestInvalidError(err) {
		return 0, false
	}
	wait, found := m.closestCooldownWait(providers, model, attempt)
	if !found || wait > maxWait {
		return 0, false
	}
	return wait, true
}

func waitForCooldown(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *Manager) MarkResult(ctx context.Context, result Result) {
	if result.AuthID == "" {
		return
	}

	shouldResumeModel := false
	shouldSuspendModel := false
	suspendReason := ""
	clearModelQuota := false
	setModelQuota := false
	var authSnapshot *Auth

	m.mu.Lock()
	if auth, ok := m.auths[result.AuthID]; ok && auth != nil {
		now := time.Now()

		if result.Success {
			if result.Model != "" {
				state := ensureModelState(auth, result.Model)
				resetModelState(state, now)
				updateAggregatedAvailability(auth, now)
				if !hasModelError(auth, now) {
					auth.LastError = nil
					auth.StatusMessage = ""
					auth.Status = StatusActive
				}
				auth.UpdatedAt = now
				shouldResumeModel = true
				clearModelQuota = true
			} else {
				clearAuthStateOnSuccess(auth, now)
			}
		} else {
			if result.Model != "" {
				state := ensureModelState(auth, result.Model)
				state.Unavailable = true
				state.Status = StatusError
				state.UpdatedAt = now
				if result.Error != nil {
					state.LastError = cloneError(result.Error)
					state.StatusMessage = result.Error.Message
					auth.LastError = cloneError(result.Error)
					auth.StatusMessage = result.Error.Message
				}

				statusCode := statusCodeFromResult(result.Error)
				switch statusCode {
				case 401:
					next := now.Add(30 * time.Minute)
					state.NextRetryAfter = next
					suspendReason = "unauthorized"
					shouldSuspendModel = true
				case 402, 403:
					next := now.Add(30 * time.Minute)
					state.NextRetryAfter = next
					suspendReason = "payment_required"
					shouldSuspendModel = true
				case 404:
					next := now.Add(12 * time.Hour)
					state.NextRetryAfter = next
					suspendReason = "not_found"
					shouldSuspendModel = true
				case 429:
					var next time.Time
					backoffLevel := state.Quota.BackoffLevel
					if result.RetryAfter != nil {
						next = now.Add(*result.RetryAfter)
					} else {
						cooldown, nextLevel := nextQuotaCooldown(backoffLevel, quotaCooldownDisabledForAuth(auth))
						if cooldown > 0 {
							next = now.Add(cooldown)
						}
						backoffLevel = nextLevel
					}
					state.NextRetryAfter = next
					state.Quota = QuotaState{Exceeded: true, Reason: "quota", NextRecoverAt: next, BackoffLevel: backoffLevel}
					suspendReason = "quota"
					shouldSuspendModel = true
					setModelQuota = true
				case 408, 500, 502, 503, 504:
					if quotaCooldownDisabledForAuth(auth) {
						state.NextRetryAfter = time.Time{}
					} else {
						next := now.Add(1 * time.Minute)
						state.NextRetryAfter = next
					}
				default:
					state.NextRetryAfter = time.Time{}
				}

				auth.Status = StatusError
				auth.UpdatedAt = now
				updateAggregatedAvailability(auth, now)
			} else {
				applyAuthFailureState(auth, result.Error, result.RetryAfter, now)
			}
		}

		_ = m.persist(ctx, auth)
		authSnapshot = auth.Clone()
	}
	m.mu.Unlock()
	if m.scheduler != nil && authSnapshot != nil {
		m.scheduler.upsertAuth(authSnapshot)
	}

	if clearModelQuota && result.Model != "" {
		registry.GetGlobalRegistry().ClearModelQuotaExceeded(result.AuthID, result.Model)
	}
	if setModelQuota && result.Model != "" {
		registry.GetGlobalRegistry().SetModelQuotaExceeded(result.AuthID, result.Model)
	}
	if shouldResumeModel {
		registry.GetGlobalRegistry().ResumeClientModel(result.AuthID, result.Model)
	} else if shouldSuspendModel {
		registry.GetGlobalRegistry().SuspendClientModel(result.AuthID, result.Model, suspendReason)
	}

	m.hook.OnResult(ctx, result)
}
