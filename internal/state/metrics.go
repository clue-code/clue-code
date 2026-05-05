package state

import (
	"sync"
	"sync/atomic"
)

// atomic counters for state observability metrics.
var (
	writeContentionTotal atomic.Int64
	watchDroppedTotal    atomic.Int64

	staleLagMu      sync.Mutex
	sessionStaleLag float64
)

// IncWriteContention increments the write-contention counter.
// Called on every WriteWithRetry retry due to flock contention.
// key and scope are accepted for future label cardinality but currently
// aggregated into a single counter.
func IncWriteContention(key, scope string) {
	writeContentionTotal.Add(1)
}

// IncWatchDropped increments the watch-dropped counter.
// Called when Watch()'s 64-buffered channel overflows and an event is dropped.
func IncWatchDropped() {
	watchDroppedTotal.Add(1)
}

// SetSessionStaleLag records the staleness lag (in seconds) of the most
// stale session detected during GetStatus. This is a gauge — each call
// overwrites the previous value.
func SetSessionStaleLag(secs float64) {
	staleLagMu.Lock()
	sessionStaleLag = secs
	staleLagMu.Unlock()
}

// SnapshotMetrics returns a point-in-time copy of all state metrics.
func SnapshotMetrics() map[string]any {
	staleLagMu.Lock()
	lag := sessionStaleLag
	staleLagMu.Unlock()

	return map[string]any{
		"state_write_contention_total": writeContentionTotal.Load(),
		"session_stale_lag_seconds":    lag,
		"state_watch_dropped_total":    watchDroppedTotal.Load(),
	}
}
