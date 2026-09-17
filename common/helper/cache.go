package helper

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	defaultCache = NewMemoryCache()
)

func Set(key string, value interface{}, duration time.Duration) {
	defaultCache.Set(key, value, duration)
}
func Get(key string) (interface{}, bool) {
	return defaultCache.Get(key)
}

func Remember(key string, duration time.Duration, callback func() (interface{}, error)) (interface{}, error) {
	return defaultCache.Remember(key, duration, callback)
}

// RememberWithTTL loads a missing value once per key and lets the loader set
// the lifetime of the returned value. A non-positive lifetime returns the
// value without storing it.
func RememberWithTTL(key string, callback func() (interface{}, time.Duration, error)) (interface{}, error) {
	return defaultCache.RememberWithTTL(key, callback)
}

func Check(key string, value interface{}) bool {
	val, ok := defaultCache.Get(key)
	return ok && val == value
}

// CacheItem represents a cached item with expiration
type CacheItem struct {
	Value      interface{}
	Expiration int64 // Unix timestamp, 0 means no expiration
}

// MemoryCache is an in-memory cache with thread-safe operations
type MemoryCache struct {
	mu        sync.RWMutex
	items     map[string]*CacheItem
	loadGroup singleflight.Group
}

// NewMemoryCache creates a new MemoryCache instance
func NewMemoryCache() *MemoryCache {
	c := &MemoryCache{
		items: make(map[string]*CacheItem),
	}
	// Start cleanup goroutine
	go c.cleanupExpired()
	return c
}

// Set stores a value in the cache with optional expiration
// duration of 0 means no expiration
func (c *MemoryCache) Set(key string, value interface{}, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration int64
	if duration > 0 {
		expiration = time.Now().Add(duration).UnixNano()
	}

	c.items[key] = &CacheItem{
		Value:      value,
		Expiration: expiration,
	}
}

// Get retrieves a value from the cache
// Returns the value and a boolean indicating if the key was found and not expired
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		delete(c.items, key)
		return nil, false
	}

	return item.Value, true
}

// Remember returns the cached value if present. Otherwise it calls callback,
// stores the successful result, and returns it.
func (c *MemoryCache) Remember(key string, duration time.Duration, callback func() (interface{}, error)) (interface{}, error) {
	return c.RememberWithTTL(key, func() (interface{}, time.Duration, error) {
		value, err := callback()
		return value, duration, err
	})
}

// RememberWithTTL behaves like Remember, while allowing the loader to choose
// the cache lifetime after obtaining a value (for example, from a token
// expiry). Concurrent cache misses for the same key share one loader call.
func (c *MemoryCache) RememberWithTTL(key string, callback func() (interface{}, time.Duration, error)) (interface{}, error) {
	if value, ok := c.Get(key); ok {
		return value, nil
	}

	value, err, _ := c.loadGroup.Do(key, func() (interface{}, error) {
		if cached, ok := c.Get(key); ok {
			return cached, nil
		}
		loaded, duration, err := callback()
		if err != nil {
			return nil, err
		}
		if duration > 0 {
			c.Set(key, loaded, duration)
		}
		return loaded, nil
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Delete removes a key from the cache
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Has checks if a key exists in the cache and is not expired
func (c *MemoryCache) Has(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return false
	}

	// Check if expired
	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		delete(c.items, key)
		return false
	}

	return true
}

// Clear removes all items from the cache
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*CacheItem)
}

// cleanupExpired periodically removes expired items from the cache
func (c *MemoryCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now().UnixNano()
		for key, item := range c.items {
			if item.Expiration > 0 && now > item.Expiration {
				delete(c.items, key)
			}
		}
		c.mu.Unlock()
	}
}
