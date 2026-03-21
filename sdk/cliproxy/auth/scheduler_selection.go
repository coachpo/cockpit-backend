package auth

import (
	"context"
	"strings"
	"time"

	cliproxyexecutor "github.com/coachpo/cockpit-backend/sdk/cliproxy/executor"
)

// pickSingle returns the next auth for a single provider/model request using scheduler state.
func (s *authScheduler) pickSingle(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, tried map[string]struct{}) (*Auth, error) {
	if s == nil {
		return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	providerKey := strings.ToLower(strings.TrimSpace(provider))
	modelKey := canonicalModelKey(model)
	pinnedAuthID := pinnedAuthIDFromMetadata(opts.Metadata)
	preferWebsocket := cliproxyexecutor.DownstreamWebsocket(ctx) && providerKey == "codex" && pinnedAuthID == ""

	s.mu.Lock()
	defer s.mu.Unlock()
	providerState := s.providers[providerKey]
	if providerState == nil {
		return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	shard := providerState.ensureModelLocked(modelKey, time.Now())
	if shard == nil {
		return nil, &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	predicate := func(entry *scheduledAuth) bool {
		if entry == nil || entry.auth == nil {
			return false
		}
		if pinnedAuthID != "" && entry.auth.ID != pinnedAuthID {
			return false
		}
		if len(tried) > 0 {
			if _, ok := tried[entry.auth.ID]; ok {
				return false
			}
		}
		return true
	}
	if picked := shard.pickReadyLocked(preferWebsocket, s.strategy, predicate); picked != nil {
		return picked, nil
	}
	return nil, shard.unavailableErrorLocked(provider, model, predicate)
}

// pickMixed returns the next auth and provider for a mixed-provider request.
func (s *authScheduler) pickMixed(ctx context.Context, providers []string, model string, opts cliproxyexecutor.Options, tried map[string]struct{}) (*Auth, string, error) {
	if s == nil {
		return nil, "", &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	normalized := normalizeProviderKeys(providers)
	if len(normalized) == 0 {
		return nil, "", &Error{Code: "provider_not_found", Message: "no provider supplied"}
	}
	pinnedAuthID := pinnedAuthIDFromMetadata(opts.Metadata)
	modelKey := canonicalModelKey(model)

	s.mu.Lock()
	defer s.mu.Unlock()
	if pinnedAuthID != "" {
		providerKey := s.authProviders[pinnedAuthID]
		if providerKey == "" || !containsProvider(normalized, providerKey) {
			return nil, "", &Error{Code: "auth_not_found", Message: "no auth available"}
		}
		providerState := s.providers[providerKey]
		if providerState == nil {
			return nil, "", &Error{Code: "auth_not_found", Message: "no auth available"}
		}
		shard := providerState.ensureModelLocked(modelKey, time.Now())
		predicate := func(entry *scheduledAuth) bool {
			if entry == nil || entry.auth == nil || entry.auth.ID != pinnedAuthID {
				return false
			}
			if len(tried) == 0 {
				return true
			}
			_, ok := tried[pinnedAuthID]
			return !ok
		}
		if picked := shard.pickReadyLocked(false, s.strategy, predicate); picked != nil {
			return picked, providerKey, nil
		}
		return nil, "", shard.unavailableErrorLocked("mixed", model, predicate)
	}

	predicate := triedPredicate(tried)
	candidateShards := make([]*modelScheduler, len(normalized))
	bestPriority := 0
	hasCandidate := false
	now := time.Now()
	for providerIndex, providerKey := range normalized {
		providerState := s.providers[providerKey]
		if providerState == nil {
			continue
		}
		shard := providerState.ensureModelLocked(modelKey, now)
		candidateShards[providerIndex] = shard
		if shard == nil {
			continue
		}
		priorityReady, okPriority := shard.highestReadyPriorityLocked(false, predicate)
		if !okPriority {
			continue
		}
		if !hasCandidate || priorityReady > bestPriority {
			bestPriority = priorityReady
			hasCandidate = true
		}
	}
	if !hasCandidate {
		return nil, "", s.mixedUnavailableErrorLocked(normalized, model, tried)
	}

	if s.strategy == schedulerStrategyFillFirst {
		for providerIndex, providerKey := range normalized {
			shard := candidateShards[providerIndex]
			if shard == nil {
				continue
			}
			picked := shard.pickReadyAtPriorityLocked(false, bestPriority, s.strategy, predicate)
			if picked != nil {
				return picked, providerKey, nil
			}
		}
		return nil, "", s.mixedUnavailableErrorLocked(normalized, model, tried)
	}

	cursorKey := strings.Join(normalized, ",") + ":" + modelKey
	start := 0
	if len(normalized) > 0 {
		start = s.mixedCursors[cursorKey] % len(normalized)
	}
	for offset := 0; offset < len(normalized); offset++ {
		providerIndex := (start + offset) % len(normalized)
		providerKey := normalized[providerIndex]
		shard := candidateShards[providerIndex]
		if shard == nil {
			continue
		}
		picked := shard.pickReadyAtPriorityLocked(false, bestPriority, schedulerStrategyRoundRobin, predicate)
		if picked == nil {
			continue
		}
		s.mixedCursors[cursorKey] = providerIndex + 1
		return picked, providerKey, nil
	}
	return nil, "", s.mixedUnavailableErrorLocked(normalized, model, tried)
}

// mixedUnavailableErrorLocked synthesizes the mixed-provider cooldown or unavailable error.
func (s *authScheduler) mixedUnavailableErrorLocked(providers []string, model string, tried map[string]struct{}) error {
	now := time.Now()
	total := 0
	cooldownCount := 0
	earliest := time.Time{}
	for _, providerKey := range providers {
		providerState := s.providers[providerKey]
		if providerState == nil {
			continue
		}
		shard := providerState.ensureModelLocked(canonicalModelKey(model), now)
		if shard == nil {
			continue
		}
		localTotal, localCooldownCount, localEarliest := shard.availabilitySummaryLocked(triedPredicate(tried))
		total += localTotal
		cooldownCount += localCooldownCount
		if !localEarliest.IsZero() && (earliest.IsZero() || localEarliest.Before(earliest)) {
			earliest = localEarliest
		}
	}
	if total == 0 {
		return &Error{Code: "auth_not_found", Message: "no auth available"}
	}
	if cooldownCount == total && !earliest.IsZero() {
		resetIn := earliest.Sub(now)
		if resetIn < 0 {
			resetIn = 0
		}
		return newModelCooldownError(model, "", resetIn)
	}
	return &Error{Code: "auth_unavailable", Message: "no auth available"}
}

// triedPredicate builds a filter that excludes auths already attempted for the current request.
func triedPredicate(tried map[string]struct{}) func(*scheduledAuth) bool {
	if len(tried) == 0 {
		return func(entry *scheduledAuth) bool { return entry != nil && entry.auth != nil }
	}
	return func(entry *scheduledAuth) bool {
		if entry == nil || entry.auth == nil {
			return false
		}
		_, ok := tried[entry.auth.ID]
		return !ok
	}
}

// normalizeProviderKeys lowercases, trims, and de-duplicates provider keys while preserving order.
func normalizeProviderKeys(providers []string) []string {
	seen := make(map[string]struct{}, len(providers))
	out := make([]string, 0, len(providers))
	for _, provider := range providers {
		providerKey := strings.ToLower(strings.TrimSpace(provider))
		if providerKey == "" {
			continue
		}
		if _, ok := seen[providerKey]; ok {
			continue
		}
		seen[providerKey] = struct{}{}
		out = append(out, providerKey)
	}
	return out
}

// containsProvider reports whether provider is present in the normalized provider list.
func containsProvider(providers []string, provider string) bool {
	for _, candidate := range providers {
		if candidate == provider {
			return true
		}
	}
	return false
}
