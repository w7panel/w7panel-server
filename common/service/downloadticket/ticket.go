// Package downloadticket creates short-lived opaque grants for workload jobs
// that cannot send an Authorization header while fetching a panel file.
package downloadticket

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type grant struct {
	path      string
	expires   time.Time
	remaining int
}

var store = struct {
	sync.Mutex
	grants map[string]grant
}{grants: map[string]grant{}}

func Issue(path string) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	expires := time.Now().Add(5 * time.Minute)
	store.Lock()
	defer store.Unlock()
	store.grants[token] = grant{path: path, expires: expires, remaining: 4}
	return token, expires, nil
}

// Consume is deliberately atomic, has a five-minute lifetime and permits no
// more than four transfers. It is called before opening the requested file.
func Consume(token, path string) bool {
	store.Lock()
	defer store.Unlock()
	value, ok := store.grants[token]
	if !ok {
		return false
	}
	if time.Now().After(value.expires) || value.remaining < 1 {
		delete(store.grants, token)
		return false
	}
	if value.path != path {
		return false
	}
	value.remaining--
	if value.remaining == 0 {
		delete(store.grants, token)
	} else {
		store.grants[token] = value
	}
	return true
}
