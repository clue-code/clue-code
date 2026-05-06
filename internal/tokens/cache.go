// Package tokens — cache.go implements a 3-level token cache:
//  1. LRU memory cache (bounded by capacity)
//  2. JSON disk persistence (one file per entry, sha256-keyed)
//  3. mtime-based invalidation for skill file changes (no fsnotify)
//
// Thread-safe via sync.RWMutex on the LRU map and sync/atomic for counters.
package tokens

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Entry is one cached token-count result.
type Entry struct {
	Key         string    `json:"key"`
	Usage       Usage     `json:"usage"`
	Payload     []byte    `json:"payload"`      // serialized response (opaque to cache)
	CreatedAt   time.Time `json:"created_at"`
	SourcePath  string    `json:"source_path"`  // optional: SKILL.md path for mtime invalidation
	SourceMtime time.Time `json:"source_mtime"` // mtime recorded at Put time
}

// CacheStats holds runtime statistics for the cache.
type CacheStats struct {
	Hits    int64
	Misses  int64
	Size    int
	HitRate float64
}

// Cache is the public interface for the 3-level token cache.
type Cache interface {
	// Get returns the entry for key. Returns false if missing, expired, or
	// mtime-invalidated.
	Get(key string) (Entry, bool)

	// Put stores an entry in memory and on disk.
	Put(key string, e Entry)

	// InvalidateByMtime scans all in-memory entries whose SourcePath matches
	// path and drops those whose recorded mtime no longer matches the file on
	// disk. Returns the number of entries dropped.
	InvalidateByMtime(path string) int

	// Stats returns a snapshot of hit/miss counters and current in-memory size.
	Stats() CacheStats

	// Clear removes all entries from memory and disk.
	Clear()
}

// NewCache returns a Cache backed by an LRU memory store of the given capacity
// and a disk store under diskDir. If diskDir is empty, os.UserConfigDir() is
// used (macOS: ~/Library/Application Support/clue-code/tokens-cache/). Falls
// back to /tmp/clue-code-cache on failure.
func NewCache(capacity int, diskDir string) (Cache, error) {
	if capacity <= 0 {
		capacity = 512
	}

	dir, err := resolveDiskDir(diskDir)
	if err != nil {
		return nil, err
	}

	c := &cache{
		capacity: capacity,
		lru:      make(map[string]*lruNode, capacity+1),
		diskDir:  dir,
	}
	c.head = &lruNode{}
	c.tail = &lruNode{}
	c.head.next = c.tail
	c.tail.prev = c.head

	// Pre-warm memory from disk (best-effort; ignore errors).
	c.warmFromDisk()

	return c, nil
}

// resolveDiskDir computes and creates the disk cache directory.
func resolveDiskDir(override string) (string, error) {
	if override != "" {
		if err := os.MkdirAll(override, 0700); err != nil {
			return "", fmt.Errorf("tokens/cache: mkdir %q: %w", override, err)
		}
		return override, nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		slog.Warn("tokens/cache: os.UserConfigDir() failed, using /tmp fallback", "err", err)
		base = "/tmp"
	}

	dir := filepath.Join(base, "clue-code", "tokens-cache")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("tokens/cache: mkdir %q: %w", dir, err)
	}
	return dir, nil
}

// diskKey returns the filename (without directory) for a cache key.
func diskKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x.json", sum[:8])
}

// ---- LRU doubly-linked list nodes ----

type lruNode struct {
	key   string
	entry Entry
	prev  *lruNode
	next  *lruNode
}

// ---- cache implementation ----

type cache struct {
	mu       sync.RWMutex
	capacity int
	lru      map[string]*lruNode
	head     *lruNode // sentinel MRU end
	tail     *lruNode // sentinel LRU end
	diskDir  string

	hits   atomic.Int64
	misses atomic.Int64
}

// Get retrieves an entry. On mtime mismatch the entry is evicted and false is
// returned. Disk is consulted only when the key is absent from memory.
func (c *cache) Get(key string) (Entry, bool) {
	c.mu.Lock()
	node, ok := c.lru[key]
	if ok {
		// Validate mtime before returning.
		if !c.mtimeValid(node.entry) {
			c.removeNode(node)
			delete(c.lru, key)
			c.mu.Unlock()
			c.misses.Add(1)
			return Entry{}, false
		}
		c.moveToFront(node)
		e := node.entry
		c.mu.Unlock()
		c.hits.Add(1)
		return e, true
	}
	c.mu.Unlock()

	// Try disk.
	e, found := c.loadFromDisk(key)
	if !found {
		c.misses.Add(1)
		return Entry{}, false
	}

	// Validate mtime on disk-loaded entry.
	if !c.mtimeValid(e) {
		_ = os.Remove(filepath.Join(c.diskDir, diskKey(key)))
		c.misses.Add(1)
		return Entry{}, false
	}

	// Promote to memory.
	c.mu.Lock()
	c.insertFront(key, e)
	c.evictIfNeeded()
	c.mu.Unlock()

	c.hits.Add(1)
	return e, true
}

// Put stores an entry in memory and asynchronously on disk.
func (c *cache) Put(key string, e Entry) {
	e.Key = key
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}

	// Snapshot mtime if SourcePath set.
	if e.SourcePath != "" && e.SourceMtime.IsZero() {
		if fi, err := os.Stat(e.SourcePath); err == nil {
			e.SourceMtime = fi.ModTime()
		}
	}

	c.mu.Lock()
	if node, ok := c.lru[key]; ok {
		node.entry = e
		c.moveToFront(node)
	} else {
		c.insertFront(key, e)
		c.evictIfNeeded()
	}
	c.mu.Unlock()

	// Persist to disk synchronously (best-effort; error is logged not returned).
	// Synchronous write keeps disk and memory consistent and avoids goroutine
	// leaks under test TempDir cleanup.
	c.saveToDisk(e)
}

// InvalidateByMtime scans all in-memory entries for path and drops those whose
// recorded mtime differs from the current file mtime on disk.
func (c *cache) InvalidateByMtime(path string) int {
	fi, statErr := os.Stat(path)

	c.mu.Lock()
	defer c.mu.Unlock()

	dropped := 0
	for key, node := range c.lru {
		if node.entry.SourcePath != path {
			continue
		}
		// If the file no longer exists or mtime changed → drop.
		if statErr != nil || !fi.ModTime().Equal(node.entry.SourceMtime) {
			c.removeNode(node)
			delete(c.lru, key)
			go func(k string) {
				_ = os.Remove(filepath.Join(c.diskDir, diskKey(k)))
			}(key)
			dropped++
		}
	}
	return dropped
}

// Stats returns a CacheStats snapshot.
func (c *cache) Stats() CacheStats {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	var rate float64
	if total > 0 {
		rate = float64(hits) / float64(total)
	}

	c.mu.RLock()
	size := len(c.lru)
	c.mu.RUnlock()

	return CacheStats{
		Hits:    hits,
		Misses:  misses,
		Size:    size,
		HitRate: rate,
	}
}

// Clear removes all in-memory and on-disk entries.
func (c *cache) Clear() {
	c.mu.Lock()
	c.lru = make(map[string]*lruNode, c.capacity+1)
	c.head.next = c.tail
	c.tail.prev = c.head
	c.mu.Unlock()

	entries, err := os.ReadDir(c.diskDir)
	if err != nil {
		return
	}
	for _, de := range entries {
		if filepath.Ext(de.Name()) == ".json" {
			_ = os.Remove(filepath.Join(c.diskDir, de.Name()))
		}
	}
}

// ---- LRU helpers (must be called with mu held) ----

func (c *cache) insertFront(key string, e Entry) {
	node := &lruNode{key: key, entry: e}
	node.next = c.head.next
	node.prev = c.head
	c.head.next.prev = node
	c.head.next = node
	c.lru[key] = node
}

func (c *cache) moveToFront(node *lruNode) {
	if c.head.next == node {
		return
	}
	c.removeNode(node)
	node.next = c.head.next
	node.prev = c.head
	c.head.next.prev = node
	c.head.next = node
}

func (c *cache) removeNode(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *cache) evictIfNeeded() {
	for len(c.lru) > c.capacity {
		victim := c.tail.prev
		if victim == c.head {
			break
		}
		c.removeNode(victim)
		delete(c.lru, victim.key)
	}
}

// ---- mtime validation ----

func (c *cache) mtimeValid(e Entry) bool {
	if e.SourcePath == "" {
		return true
	}
	fi, err := os.Stat(e.SourcePath)
	if err != nil {
		return false // file gone → invalid
	}
	return fi.ModTime().Equal(e.SourceMtime)
}

// ---- Disk I/O ----

func (c *cache) saveToDisk(e Entry) {
	data, err := json.Marshal(e)
	if err != nil {
		slog.Warn("tokens/cache: marshal entry", "key", e.Key, "err", err)
		return
	}
	path := filepath.Join(c.diskDir, diskKey(e.Key))
	if err := os.WriteFile(path, data, 0600); err != nil {
		slog.Warn("tokens/cache: write entry", "path", path, "err", err)
	}
}

func (c *cache) loadFromDisk(key string) (Entry, bool) {
	path := filepath.Join(c.diskDir, diskKey(key))
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("tokens/cache: read entry", "path", path, "err", err)
		}
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		slog.Warn("tokens/cache: unmarshal entry", "path", path, "err", err)
		return Entry{}, false
	}
	return e, true
}

// warmFromDisk loads all valid JSON files from disk into memory up to capacity.
// Called once at startup; errors are silently ignored to keep startup fast.
func (c *cache) warmFromDisk() {
	entries, err := os.ReadDir(c.diskDir)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, de := range entries {
		if len(c.lru) >= c.capacity {
			break
		}
		if filepath.Ext(de.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.diskDir, de.Name()))
		if err != nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(data, &e); err != nil || e.Key == "" {
			continue
		}
		if _, exists := c.lru[e.Key]; !exists {
			c.insertFront(e.Key, e)
		}
	}
}
