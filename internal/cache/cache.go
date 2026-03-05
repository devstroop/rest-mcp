// Package cache provides an in-memory HTTP response cache that supports
// ETag and Last-Modified conditional request headers.
package cache

import (
	"sync"
	"time"
)

// Entry is a cached HTTP response with conditional request metadata.
type Entry struct {
	Body         string
	StatusCode   int
	ETag         string
	LastModified string
	CachedAt     time.Time
}

// Cache is a thread-safe in-memory response cache keyed by HTTP method + URL.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	maxSize int
}

// New creates a Cache with the given maximum number of entries.
// When the limit is reached, the oldest entry is evicted.
func New(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &Cache{
		entries: make(map[string]*Entry, maxSize),
		maxSize: maxSize,
	}
}

// Key returns the cache key for a given method and URL.
func Key(method, url string) string {
	return method + " " + url
}

// Get returns a cached entry if it exists, or nil.
func (c *Cache) Get(key string) *Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.entries[key]
}

// Set stores a cache entry. If the cache is full, the oldest entry is evicted.
func (c *Cache) Set(key string, entry *Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity and this is a new key
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = entry
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*Entry, c.maxSize)
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evictOldest removes the entry with the oldest CachedAt time.
// Must be called with mu held.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, e := range c.entries {
		if oldestKey == "" || e.CachedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
