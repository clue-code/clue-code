package state

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	watchBufSize    = 64
	pollInterval    = time.Second
)

// Watch returns a channel that emits SessionDescriptor events whenever the
// global sessions index changes. Uses fsnotify as primary with 1 Hz polling
// as fallback. Channel is buffered at 64; on overflow the oldest event is
// dropped and IncWatchDropped is incremented.
func Watch(ctx context.Context) (<-chan SessionDescriptor, error) {
	dir, err := globalSessionsDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	indexPath := filepath.Join(dir, indexFile)

	ch := make(chan SessionDescriptor, watchBufSize)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Warn("state: fsnotify unavailable, using poll-only watch", "err", err)
		watcher = nil
	} else {
		// Watch the directory so we catch index.json creation too.
		if err := watcher.Add(dir); err != nil {
			slog.Warn("state: fsnotify add failed, using poll-only watch", "dir", dir, "err", err)
			watcher.Close() //nolint:errcheck
			watcher = nil
		}
	}

	go func() {
		defer func() {
			if watcher != nil {
				watcher.Close() //nolint:errcheck
			}
			close(ch)
		}()

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		var lastSessions []SessionDescriptor

		emit := func(sessions []SessionDescriptor) {
			for _, s := range sessions {
				select {
				case ch <- s:
				default:
					// Buffer full — drop oldest by draining one then sending.
					select {
					case <-ch:
					default:
					}
					IncWatchDropped()
					select {
					case ch <- s:
					default:
					}
				}
			}
			lastSessions = sessions
		}

		refresh := func() {
			sessions, err := readIndex(indexPath)
			if err != nil {
				slog.Warn("state: watch refresh error", "err", err)
				return
			}
			if sessionsChanged(lastSessions, sessions) {
				emit(sessions)
			}
		}

		// Initial snapshot.
		refresh()

		for {
			if watcher != nil {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					if filepath.Base(event.Name) == indexFile {
						refresh()
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					slog.Warn("state: fsnotify error", "err", err)
				case <-ticker.C:
					refresh()
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					refresh()
				}
			}
		}
	}()

	return ch, nil
}

// sessionsChanged returns true if the two slices differ (by ID set).
func sessionsChanged(old, new []SessionDescriptor) bool {
	if len(old) != len(new) {
		return true
	}
	oldIDs := make(map[string]bool, len(old))
	for _, s := range old {
		oldIDs[s.ID] = true
	}
	for _, s := range new {
		if !oldIDs[s.ID] {
			return true
		}
	}
	return false
}
