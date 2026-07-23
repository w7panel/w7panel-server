package helper

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache()

	if cache == nil {
		t.Fatal("NewMemoryCache returned nil")
	}

	if cache.items == nil {
		t.Fatal("cache.items is nil, expected empty map")
	}

	if len(cache.items) != 0 {
		t.Fatalf("expected empty cache, got %d items", len(cache.items))
	}
}

func TestMemoryCache_Set(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	cache.mu.RLock()
	item, exists := cache.items["key1"]
	cache.mu.RUnlock()

	if !exists {
		t.Fatal("key1 should exist in cache")
	}

	if item.Value != "value1" {
		t.Fatalf("expected value1, got %v", item.Value)
	}

	if item.Expiration != 0 {
		t.Fatalf("expected no expiration (0), got %d", item.Expiration)
	}
}

func TestMemoryCache_Set_WithExpiration(t *testing.T) {
	cache := NewMemoryCache()

	duration := 100 * time.Millisecond
	cache.Set("key1", "value1", duration)

	cache.mu.RLock()
	item, exists := cache.items["key1"]
	cache.mu.RUnlock()

	if !exists {
		t.Fatal("key1 should exist in cache")
	}

	if item.Expiration == 0 {
		t.Fatal("expected expiration to be set")
	}

	expectedExpiration := time.Now().Add(duration).UnixNano()
	if item.Expiration < expectedExpiration-1e9 || item.Expiration > expectedExpiration+1e9 {
		t.Fatalf("expiration time is not within expected range: got %d, expected around %d", item.Expiration, expectedExpiration)
	}
}

func TestMemoryCache_Set_Overwrite(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)
	cache.Set("key1", "value2", 0)

	cache.mu.RLock()
	item, exists := cache.items["key1"]
	cache.mu.RUnlock()

	if !exists {
		t.Fatal("key1 should exist in cache")
	}

	if item.Value != "value2" {
		t.Fatalf("expected value2, got %v", item.Value)
	}
}

func TestMemoryCache_Get_NotFound(t *testing.T) {
	cache := NewMemoryCache()

	value, ok := cache.Get("nonexistent")

	if ok {
		t.Fatal("expected ok to be false for nonexistent key")
	}

	if value != nil {
		t.Fatalf("expected nil value, got %v", value)
	}
}

func TestMemoryCache_Get_Found(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	value, ok := cache.Get("key1")

	if !ok {
		t.Fatal("expected ok to be true")
	}

	if value != "value1" {
		t.Fatalf("expected value1, got %v", value)
	}
}

func TestMemoryCache_Get_Expired(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	value, ok := cache.Get("key1")

	if ok {
		t.Fatal("expected ok to be false for expired key")
	}

	if value != nil {
		t.Fatalf("expected nil value for expired key, got %v", value)
	}
}

func TestMemoryCache_Get_Concurrent(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Get("key1")
			}
		}()
	}

	wg.Wait()
}

func TestMemoryCache_Remember_Miss(t *testing.T) {
	cache := NewMemoryCache()
	calls := 0

	value, err := cache.Remember("key1", time.Minute, func() (interface{}, error) {
		calls++
		return "value1", nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if value != "value1" {
		t.Fatalf("expected value1, got %v", value)
	}
	if calls != 1 {
		t.Fatalf("expected callback to be called once, got %d", calls)
	}

	cached, ok := cache.Get("key1")
	if !ok {
		t.Fatal("expected value to be cached")
	}
	if cached != "value1" {
		t.Fatalf("expected cached value1, got %v", cached)
	}
}

func TestMemoryCache_Remember_Hit(t *testing.T) {
	cache := NewMemoryCache()
	cache.Set("key1", "cached", time.Minute)

	value, err := cache.Remember("key1", time.Minute, func() (interface{}, error) {
		return "loaded", nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if value != "cached" {
		t.Fatalf("expected cached, got %v", value)
	}
}

func TestMemoryCache_Remember_Error(t *testing.T) {
	cache := NewMemoryCache()
	expectedErr := errors.New("load failed")

	value, err := cache.Remember("key1", time.Minute, func() (interface{}, error) {
		return "value1", expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected load failed error, got %v", err)
	}
	if value != nil {
		t.Fatalf("expected nil value, got %v", value)
	}
	if _, ok := cache.Get("key1"); ok {
		t.Fatal("expected failed callback result not to be cached")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	cache.Delete("key1")

	cache.mu.RLock()
	_, exists := cache.items["key1"]
	cache.mu.RUnlock()

	if exists {
		t.Fatal("key1 should not exist after Delete")
	}
}

func TestMemoryCache_Delete_NonExistent(t *testing.T) {
	cache := NewMemoryCache()

	cache.Delete("nonexistent")
}

func TestMemoryCache_Delete_Concurrent(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			cache.Delete("key1")
		}()
	}

	wg.Wait()
}

func TestMemoryCache_Has_NotFound(t *testing.T) {
	cache := NewMemoryCache()

	if cache.Has("nonexistent") {
		t.Fatal("expected Has to return false for nonexistent key")
	}
}

func TestMemoryCache_Has_Found(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	if !cache.Has("key1") {
		t.Fatal("expected Has to return true for existing key")
	}
}

func TestMemoryCache_Has_Expired(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	if cache.Has("key1") {
		t.Fatal("expected Has to return false for expired key")
	}
}

func TestMemoryCache_Has_Concurrent(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Has("key1")
			}
		}()
	}

	wg.Wait()
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)
	cache.Set("key3", "value3", 0)

	cache.Clear()

	cache.mu.RLock()
	itemCount := len(cache.items)
	cache.mu.RUnlock()

	if itemCount != 0 {
		t.Fatalf("expected 0 items after Clear, got %d", itemCount)
	}
}

func TestMemoryCache_Clear_Concurrent(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", "value1", 0)
	cache.Set("key2", "value2", 0)

	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			cache.Set("key1", "value1", 0)
		}()
		go func() {
			defer wg.Done()
			cache.Clear()
		}()
	}

	wg.Wait()
}

func TestMemoryCache_SetNilValue(t *testing.T) {
	cache := NewMemoryCache()

	cache.Set("key1", nil, 0)

	value, ok := cache.Get("key1")

	if !ok {
		t.Fatal("expected ok to be true for nil value")
	}

	if value != nil {
		t.Fatalf("expected nil value, got %v", value)
	}
}

func TestMemoryCache_SetDifferentTypes(t *testing.T) {
	cache := NewMemoryCache()

	testCases := []struct {
		key   string
		value interface{}
	}{
		{"string", "value"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1}},
		{"struct", struct{ Name string }{"test"}},
	}

	for _, tc := range testCases {
		t.Run(tc.key, func(t *testing.T) {
			cache.Set(tc.key, tc.value, 0)

			value, ok := cache.Get(tc.key)

			if !ok {
				t.Fatalf("expected ok to be true for %s", tc.key)
			}

			if !reflect.DeepEqual(value, tc.value) {
				t.Fatalf("expected %v, got %v", tc.value, value)
			}
		})
	}
}

func TestMemoryCache_CacheItemStruct(t *testing.T) {
	item := CacheItem{
		Value:      "test",
		Expiration: 1234567890,
	}

	if item.Value != "test" {
		t.Fatalf("expected Value to be 'test', got %v", item.Value)
	}

	if item.Expiration != 1234567890 {
		t.Fatalf("expected Expiration to be 1234567890, got %d", item.Expiration)
	}
}

func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	cache := NewMemoryCache()

	var wg sync.WaitGroup
	wg.Add(20)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('0'+idx))
			cache.Set(key, idx, 0)
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('0'+idx))
			cache.Get(key)
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('0'+idx))
			cache.Has(key)
		}(i)
	}

	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer wg.Done()
			key := "key" + string(rune('0'+idx))
			cache.Delete(key)
		}(i)
	}

	wg.Wait()

	cache.Clear()
}

func TestDefaultCacheFunctions(t *testing.T) {
	defaultCache.Clear()

	Set("testkey", "testvalue", 0)

	value, ok := Get("testkey")
	if !ok {
		t.Fatal("expected Get to return true")
	}
	if value != "testvalue" {
		t.Fatalf("expected 'testvalue', got %v", value)
	}

	if !Check("testkey", "testvalue") {
		t.Fatal("expected Check to return true for matching value")
	}

	if Check("testkey", "wrongvalue") {
		t.Fatal("expected Check to return false for non-matching value")
	}

	if Check("nonexistent", "value") {
		t.Fatal("expected Check to return false for nonexistent key")
	}

	rememberValue, err := Remember("remember-key", time.Minute, func() (interface{}, error) {
		return "remember-value", nil
	})
	if err != nil {
		t.Fatalf("expected Remember to return nil error, got %v", err)
	}
	if rememberValue != "remember-value" {
		t.Fatalf("expected Remember to return remember-value, got %v", rememberValue)
	}

	rememberValue, err = Remember("remember-key", time.Minute, func() (interface{}, error) {
		return "wrong-value", nil
	})
	if err != nil {
		t.Fatalf("expected Remember cache hit to return nil error, got %v", err)
	}
	if rememberValue != "remember-value" {
		t.Fatalf("expected Remember cache hit to return remember-value, got %v", rememberValue)
	}

	defaultCache.Delete("testkey")

	if Check("testkey", "testvalue") {
		t.Fatal("expected Check to return false after Delete")
	}

	defaultCache.Clear()
}
