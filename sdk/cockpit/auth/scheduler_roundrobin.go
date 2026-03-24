package auth

// readyBucket keeps the ready views for one priority level.
type readyBucket struct {
	all readyView
	ws  readyView
}

type readyView struct {
	flat   []*scheduledAuth
	cursor int
}

// buildReadyBucket prepares the general and websocket-only ready views for one priority bucket.
func buildReadyBucket(entries []*scheduledAuth) *readyBucket {
	bucket := &readyBucket{}
	bucket.all = buildReadyView(entries)
	wsEntries := make([]*scheduledAuth, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && entry.meta != nil && entry.meta.websocketEnabled {
			wsEntries = append(wsEntries, entry)
		}
	}
	bucket.ws = buildReadyView(wsEntries)
	return bucket
}

func buildReadyView(entries []*scheduledAuth) readyView {
	return readyView{flat: append([]*scheduledAuth(nil), entries...)}
}

// pickFirst returns the first ready entry that satisfies predicate without advancing cursors.
func (v *readyView) pickFirst(predicate func(*scheduledAuth) bool) *scheduledAuth {
	for _, entry := range v.flat {
		if predicate == nil || predicate(entry) {
			return entry
		}
	}
	return nil
}

func (v *readyView) pickRoundRobin(predicate func(*scheduledAuth) bool) *scheduledAuth {
	if len(v.flat) == 0 {
		return nil
	}
	start := 0
	if len(v.flat) > 0 {
		start = v.cursor % len(v.flat)
	}
	for offset := 0; offset < len(v.flat); offset++ {
		index := (start + offset) % len(v.flat)
		entry := v.flat[index]
		if predicate != nil && !predicate(entry) {
			continue
		}
		v.cursor = index + 1
		return entry
	}
	return nil
}
