package S3ClientCache

import (
	"fmt"
	"sync"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/InseeFrLab/s3-operator/internal/s3/factory"
)

var (
	logger = ctrl.Log.WithValues("logger", "s3clientCache")
)

// Cache is a basic in-memory key-value cache implementation.
type S3ClientCache struct {
	items map[string]factory.S3Client // The map storing key-value pairs.
	mu    sync.Mutex                  // Mutex for controlling concurrent access to the cache.
}

// New creates a new Cache instance.
func New() *S3ClientCache {
	logger.Info("Creation of S3ClientCache successfully")
	return &S3ClientCache{
		items: make(map[string]factory.S3Client),
	}
}

// Set adds or updates a key-value pair in the cache.
func (c *S3ClientCache) Set(key string, value factory.S3Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info(fmt.Sprintf("Add S3Client %s in cache successfully", key))
	c.items[key] = value
}

// Get retrieves the value associated with the given key from the cache. The bool
// return value will be false if no matching key is found, and true otherwise.
func (c *S3ClientCache) Get(key string) (factory.S3Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info(fmt.Sprintf("Try getting S3Client %s in cache", key))

	value, found := c.items[key]
	return value, found
}

// Remove deletes the key-value pair with the specified key from the cache.
func (c *S3ClientCache) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	logger.Info(fmt.Sprintf("Successfully remove S3Client %s in cache", key))

	delete(c.items, key)
}

// Pop removes and returns the value associated with the specified key from the cache.
func (c *S3ClientCache) Pop(key string) (factory.S3Client, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, found := c.items[key]

	// If the key is found, delete the key-value pair from the cache.
	if found {
		delete(c.items, key)
	}

	return value, found
}

func (c *S3ClientCache) GetAllowedNamespaces(key string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var allowedNamepaces []string

	logger.Info(fmt.Sprintf("Get AllowedNamespaces for S3Client %s in cache", key))

	for _, s3Client := range c.items {
		allowedNamepaces = append(allowedNamepaces, s3Client.GetConfig().AllowedNamespaces...)
	}
	return allowedNamepaces
}

type S3ClientCacheError struct {
	Reason string
}

type S3ClientNotFound struct {
	Reason string
}

func (r *S3ClientCacheError) Error() string {
	return fmt.Sprintf("%s", r.Reason)
}

func (r *S3ClientNotFound) Error() string {
	return fmt.Sprintf("%s", r.Reason)
}
