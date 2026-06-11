package apiclient

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultCache = newApiClientCache(1 * time.Minute)

type apiClientCacheEntry struct {
	namespace string
	name      string
	client    *apiclientv1alpha1.ApiClient
}

type apiClientCache struct {
	mu              sync.RWMutex
	cache           map[string]*apiClientCacheEntry
	clientIDIndex   map[string]string
	namespaceLoaded map[string]bool
	recentAccess    map[string]time.Time
	flushInterval   time.Duration
	startOnce       sync.Once
}

func newApiClientCache(flushInterval time.Duration) *apiClientCache {
	return &apiClientCache{
		cache:           make(map[string]*apiClientCacheEntry),
		clientIDIndex:   make(map[string]string),
		namespaceLoaded: make(map[string]bool),
		recentAccess:    make(map[string]time.Time),
		flushInterval:   flushInterval,
	}
}

func GetCachedApiClient(ctx context.Context, namespace, name string, loader func(context.Context, string, string) (*apiclientv1alpha1.ApiClient, error)) (*apiclientv1alpha1.ApiClient, error) {
	return defaultCache.get(ctx, namespace, name, loader)
}

func GetCachedApiClientByID(ctx context.Context, namespace, clientID string, loader func(context.Context, string) ([]apiclientv1alpha1.ApiClient, error)) (*apiclientv1alpha1.ApiClient, error) {
	return defaultCache.getByClientID(ctx, namespace, clientID, loader)
}

func UpsertCache(client *apiclientv1alpha1.ApiClient) {
	defaultCache.upsert(client)
}

func DeleteCache(namespace, name string) {
	defaultCache.delete(namespace, name)
}

func MarkAccessed(namespace, name string, accessTime time.Time) {
	defaultCache.markAccessed(namespace, name, accessTime)
}

func (c *apiClientCache) get(ctx context.Context, namespace, name string, loader func(context.Context, string, string) (*apiclientv1alpha1.ApiClient, error)) (*apiclientv1alpha1.ApiClient, error) {
	c.start()

	if client := c.getCached(namespace, name); client != nil {
		return client, nil
	}

	client, err := loader(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	c.upsert(client)
	return client.DeepCopy(), nil
}

func (c *apiClientCache) getByClientID(ctx context.Context, namespace, clientID string, loader func(context.Context, string) ([]apiclientv1alpha1.ApiClient, error)) (*apiclientv1alpha1.ApiClient, error) {
	c.start()

	if client := c.lookupByClientID(namespace, clientID); client != nil {
		return client, nil
	}

	if err := c.loadNamespace(ctx, namespace, loader); err != nil {
		return nil, err
	}

	return c.lookupByClientID(namespace, clientID), nil
}

func (c *apiClientCache) getCached(namespace, name string) *apiclientv1alpha1.ApiClient {
	key := c.key(namespace, name)
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()
	if !ok || entry.client == nil {
		return nil
	}
	return entry.client.DeepCopy()
}

func (c *apiClientCache) lookupByClientID(namespace, clientID string) *apiclientv1alpha1.ApiClient {
	indexKey := c.clientIDKey(namespace, clientID)

	c.mu.RLock()
	nameKey, ok := c.clientIDIndex[indexKey]
	if !ok {
		c.mu.RUnlock()
		return nil
	}
	entry, ok := c.cache[nameKey]
	c.mu.RUnlock()
	if !ok || entry.client == nil {
		return nil
	}
	return entry.client.DeepCopy()
}

func (c *apiClientCache) loadNamespace(ctx context.Context, namespace string, loader func(context.Context, string) ([]apiclientv1alpha1.ApiClient, error)) error {
	c.mu.RLock()
	loaded := c.namespaceLoaded[namespace]
	c.mu.RUnlock()
	if loaded {
		return nil
	}

	items, err := loader(ctx, namespace)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.namespaceLoaded[namespace] {
		return nil
	}

	for i := range items {
		client := items[i].DeepCopy()
		key := c.key(client.Namespace, client.Name)
		c.cache[key] = &apiClientCacheEntry{
			namespace: client.Namespace,
			name:      client.Name,
			client:    client,
		}
		if client.Spec.ClientID != "" {
			c.clientIDIndex[c.clientIDKey(client.Namespace, client.Spec.ClientID)] = key
		}
	}
	c.namespaceLoaded[namespace] = true
	return nil
}

func (c *apiClientCache) upsert(client *apiclientv1alpha1.ApiClient) {
	if client == nil {
		return
	}
	c.start()

	key := c.key(client.Namespace, client.Name)
	copyClient := client.DeepCopy()

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.cache[key]; ok && old.client != nil && old.client.Spec.ClientID != "" {
		delete(c.clientIDIndex, c.clientIDKey(old.client.Namespace, old.client.Spec.ClientID))
	}

	c.cache[key] = &apiClientCacheEntry{
		namespace: copyClient.Namespace,
		name:      copyClient.Name,
		client:    copyClient,
	}
	if copyClient.Spec.ClientID != "" {
		c.clientIDIndex[c.clientIDKey(copyClient.Namespace, copyClient.Spec.ClientID)] = key
	}
}

func (c *apiClientCache) delete(namespace, name string) {
	c.start()

	key := c.key(namespace, name)

	c.mu.Lock()
	defer c.mu.Unlock()

	if old, ok := c.cache[key]; ok && old.client != nil && old.client.Spec.ClientID != "" {
		delete(c.clientIDIndex, c.clientIDKey(old.client.Namespace, old.client.Spec.ClientID))
	}
	delete(c.cache, key)
	delete(c.recentAccess, key)
}

func (c *apiClientCache) markAccessed(namespace, name string, accessTime time.Time) {
	c.start()

	key := c.key(namespace, name)
	c.mu.Lock()
	if last, ok := c.recentAccess[key]; !ok || accessTime.After(last) {
		c.recentAccess[key] = accessTime.UTC()
	}
	if entry, ok := c.cache[key]; ok && entry.client != nil {
		entry.client.Status.LastAccessedAt = &metav1.Time{Time: accessTime.UTC()}
	}
	c.mu.Unlock()
}

func (c *apiClientCache) start() {
	c.startOnce.Do(func() {
		ticker := time.NewTicker(c.flushInterval)
		go func() {
			for range ticker.C {
				if err := c.flush(); err != nil {
					slog.Error("flush api client access time failed", "err", err)
				}
			}
		}()
	})
}

func (c *apiClientCache) flush() error {
	pending := c.snapshotPending()
	if len(pending) == 0 {
		return nil
	}

	var flushErr error
	for key, accessTime := range pending {
		namespace, name, ok := c.parseKey(key)
		if !ok {
			continue
		}
		if err := c.persistAccessTime(namespace, name, accessTime); err != nil {
			if flushErr == nil {
				flushErr = err
			}
			slog.Error("persist api client access time failed", "namespace", namespace, "name", name, "err", err)
			c.restorePending(key, accessTime)
		}
	}
	return flushErr
}

func (c *apiClientCache) snapshotPending() map[string]time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.recentAccess) == 0 {
		return nil
	}

	pending := make(map[string]time.Time, len(c.recentAccess))
	for key, accessTime := range c.recentAccess {
		pending[key] = accessTime
	}
	c.recentAccess = make(map[string]time.Time)
	return pending
}

func (c *apiClientCache) restorePending(key string, accessTime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if last, ok := c.recentAccess[key]; !ok || accessTime.After(last) {
		c.recentAccess[key] = accessTime
	}
}

func (c *apiClientCache) persistAccessTime(namespace, name string, accessTime time.Time) error {
	sdk := k8s.NewK8sClient()
	k8sClient, err := sdk.ToSigClient()
	if err != nil {
		return err
	}

	current := &apiclientv1alpha1.ApiClient{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, current); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if current.Status.LastAccessedAt != nil && current.Status.LastAccessedAt.Time.Equal(accessTime.UTC()) {
		return nil
	}

	original := current.DeepCopy()
	current.Status.LastAccessedAt = &metav1.Time{Time: accessTime.UTC()}

	if err := k8sClient.Patch(context.Background(), current, ctrlclient.MergeFrom(original)); err != nil {
		return err
	}

	c.upsert(current)
	return nil
}

func (c *apiClientCache) key(namespace, name string) string {
	return namespace + "/" + name
}

func (c *apiClientCache) clientIDKey(namespace, clientID string) string {
	return namespace + "/" + clientID
}

func (c *apiClientCache) parseKey(key string) (string, string, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i], key[i+1:], true
		}
	}
	return "", "", false
}
