package cachex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/KiriKirby/phytozome-go/internal/appfs"
	phygoboost "github.com/KiriKirby/phytozome-go/internal/phygoboost"
	"github.com/cespare/xxhash/v2"
	"github.com/dgraph-io/ristretto/v2"
	"github.com/goccy/go-json"
	"github.com/gofrs/flock"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vmihailenco/msgpack/v5"
	bolt "go.etcd.io/bbolt"
)

type Cache struct {
	namespace string
	root      string
	memory    *ristretto.Cache[string, []byte]
	fallback  *lru.Cache[string, []byte]
	index     *bolt.DB
	lockPath  string
	mu        sync.Mutex
}

type diskIndexEntry struct {
	Path      string `json:"path" msgpack:"path"`
	Size      int64  `json:"size" msgpack:"size"`
	UpdatedAt int64  `json:"updated_at" msgpack:"updated_at"`
}

var (
	registryMu sync.Mutex
	registry   = map[string]*Cache{}
)

func Open(namespace string, parts ...string) (*Cache, error) {
	namespace = cleanPart(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("cache namespace is empty")
	}
	cacheKey := namespace + "/" + strings.Join(parts, "/")

	registryMu.Lock()
	defer registryMu.Unlock()
	if cache, ok := registry[cacheKey]; ok {
		return cache, nil
	}

	pathParts := append([]string{namespace}, parts...)
	root, err := appfs.CacheDir(pathParts...)
	if err != nil {
		return nil, err
	}

	cache := &Cache{namespace: namespace, root: root}
	cache.lockPath = filepath.Join(root, ".cache.lock")
	cache.memory = newMemoryCache()
	if fallback, err := lru.New[string, []byte](512); err == nil {
		cache.fallback = fallback
	}
	if index, err := bolt.Open(filepath.Join(root, "cache-index.bbolt"), 0o600, &bolt.Options{Timeout: 150 * time.Millisecond}); err == nil {
		cache.index = index
		_ = cache.index.Update(func(tx *bolt.Tx) error {
			_, err := tx.CreateBucketIfNotExists([]byte("entries"))
			return err
		})
	}
	phygoboost.RegisterCleaner(cache.TrimMemory)
	registry[cacheKey] = cache
	return cache, nil
}

func MustOpen(namespace string, parts ...string) *Cache {
	cache, err := Open(namespace, parts...)
	if err != nil {
		return &Cache{namespace: namespace}
	}
	return cache
}

func (c *Cache) ReadJSON(key string, dst any) bool {
	data, ok := c.ReadBytes("json", key)
	if !ok {
		return false
	}
	return json.Unmarshal(data, dst) == nil
}

func (c *Cache) WriteJSON(key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = c.WriteBytes("json", key, data)
}

func (c *Cache) ReadMsgpack(key string, dst any) bool {
	data, ok := c.ReadBytes("msgpack", key)
	if !ok {
		return false
	}
	return msgpack.Unmarshal(data, dst) == nil
}

func (c *Cache) WriteMsgpack(key string, value any) {
	data, err := msgpack.Marshal(value)
	if err != nil {
		return
	}
	_ = c.WriteBytes("msgpack", key, data)
}

func (c *Cache) ReadText(key string) (string, bool) {
	data, ok := c.ReadBytes("txt", key)
	if !ok {
		return "", false
	}
	return string(data), true
}

func (c *Cache) WriteText(key string, value string) {
	_ = c.WriteBytes("txt", key, []byte(value))
}

func (c *Cache) ReadBytes(ext string, key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	cacheKey := memoryKey(ext, key)
	c.mu.Lock()
	if c.memory != nil {
		if value, ok := c.memory.Get(cacheKey); ok {
			c.mu.Unlock()
			return append([]byte(nil), value...), true
		}
	}
	if c.fallback != nil {
		if value, ok := c.fallback.Get(cacheKey); ok {
			c.mu.Unlock()
			return append([]byte(nil), value...), true
		}
	}
	c.mu.Unlock()
	var data []byte
	err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		unlock := c.lockShared()
		defer unlock()
		path, err := c.filePathLocked(ext, key)
		if err != nil {
			return err
		}
		var readErr error
		data, readErr = os.ReadFile(path)
		return readErr
	})
	if err != nil {
		return nil, false
	}
	c.setMemory(cacheKey, data)
	return data, true
}

func (c *Cache) WriteBytes(ext string, key string, data []byte) error {
	if c == nil {
		return nil
	}
	writeErr := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		unlock := c.lockExclusive()
		defer unlock()
		path, err := c.filePathLocked(ext, key)
		if err != nil {
			return err
		}
		return writeAtomically(path, data)
	})
	if writeErr != nil {
		return writeErr
	}
	c.setMemory(memoryKey(ext, key), data)
	path, err := c.filePath(ext, key)
	if err != nil {
		return err
	}
	c.recordIndex(ext, key, path, int64(len(data)))
	phygoboost.ObserveCacheBytes(int64(len(data)))
	return nil
}

func (c *Cache) Delete(ext string, key string) {
	if c == nil {
		return
	}
	cacheKey := memoryKey(ext, key)
	c.mu.Lock()
	if c.memory != nil {
		c.memory.Del(cacheKey)
	}
	if c.fallback != nil {
		c.fallback.Remove(cacheKey)
	}
	c.mu.Unlock()
	_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		unlock := c.lockExclusive()
		defer unlock()
		path, err := c.filePathLocked(ext, key)
		if err != nil {
			return err
		}
		return os.Remove(path)
	})
}

func (c *Cache) TrimMemory() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memory != nil {
		c.memory.Clear()
	}
	if c.fallback != nil {
		c.fallback.Purge()
	}
	runtime.GC()
}

func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memory != nil {
		c.memory.Close()
		c.memory = nil
	}
	if c.fallback != nil {
		c.fallback.Purge()
		c.fallback = nil
	}
	if c.index == nil {
		return nil
	}
	err := c.index.Close()
	c.index = nil
	return err
}

func CloseAll() error {
	registryMu.Lock()
	caches := make([]*Cache, 0, len(registry))
	for _, cache := range registry {
		caches = append(caches, cache)
	}
	registry = map[string]*Cache{}
	registryMu.Unlock()

	var firstErr error
	for _, cache := range caches {
		if err := cache.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Cache) setMemory(key string, data []byte) {
	if len(data) == 0 {
		return
	}
	copyData := append([]byte(nil), data...)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memory != nil {
		c.memory.Set(key, copyData, int64(len(copyData)))
	}
	if c.fallback != nil && len(copyData) <= 2*1024*1024 {
		c.fallback.Add(key, copyData)
	}
}

func (c *Cache) recordIndex(ext string, key string, path string, size int64) {
	c.mu.Lock()
	index := c.index
	c.mu.Unlock()
	if index == nil {
		return
	}
	indexKey := []byte(memoryKey(ext, key))
	entry, err := msgpack.Marshal(diskIndexEntry{Path: path, Size: size, UpdatedAt: time.Now().Unix()})
	if err != nil {
		return
	}
	_ = phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		unlock := c.lockExclusive()
		defer unlock()
		return index.Update(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte("entries"))
			if bucket == nil {
				return nil
			}
			return bucket.Put(indexKey, entry)
		})
	})
}

func (c *Cache) filePath(ext string, key string) (string, error) {
	var path string
	err := phygoboost.RunDisk(context.Background(), func(ctx context.Context) error {
		var err error
		path, err = c.filePathLocked(ext, key)
		return err
	})
	return path, err
}

func (c *Cache) filePathLocked(ext string, key string) (string, error) {
	if c.root == "" {
		return "", fmt.Errorf("cache root unavailable")
	}
	ext = strings.TrimPrefix(cleanPart(ext), ".")
	if ext == "" {
		ext = "bin"
	}
	name := hashKey(key) + "." + ext
	groupDir := filepath.Join(c.root, ext)
	if err := os.MkdirAll(groupDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(groupDir, name), nil
}

func writeAtomically(path string, data []byte) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (c *Cache) lockShared() func() {
	if c == nil || c.lockPath == "" {
		return func() {}
	}
	lock := flock.New(c.lockPath)
	if err := lock.RLock(); err != nil {
		return func() {}
	}
	return func() {
		_ = lock.Unlock()
		_ = lock.Close()
	}
}

func (c *Cache) lockExclusive() func() {
	if c == nil || c.lockPath == "" {
		return func() {}
	}
	lock := flock.New(c.lockPath)
	if err := lock.Lock(); err != nil {
		return func() {}
	}
	return func() {
		_ = lock.Unlock()
		_ = lock.Close()
	}
}

func newMemoryCache() *ristretto.Cache[string, []byte] {
	maxCost := phygoboost.MemoryCacheBudgetBytes()
	if maxCost < 8*1024*1024 {
		maxCost = 8 * 1024 * 1024
	}
	cache, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: maxCost / 64,
		MaxCost:     maxCost,
		BufferItems: 64,
		Metrics:     false,
		KeyToHash: func(key string) (uint64, uint64) {
			a := xxhash.Sum64String(key)
			return a, bitsReverse64(a)
		},
	})
	if err != nil {
		return nil
	}
	return cache
}

func memoryKey(ext string, key string) string {
	return cleanPart(ext) + ":" + key
}

func hashKey(key string) string {
	sum := xxhash.Sum64String(key)
	return fmt.Sprintf("%016x", sum)
}

func bitsReverse64(v uint64) uint64 {
	v = (v&0x5555555555555555)<<1 | (v>>1)&0x5555555555555555
	v = (v&0x3333333333333333)<<2 | (v>>2)&0x3333333333333333
	v = (v&0x0f0f0f0f0f0f0f0f)<<4 | (v>>4)&0x0f0f0f0f0f0f0f0f
	v = (v&0x00ff00ff00ff00ff)<<8 | (v>>8)&0x00ff00ff00ff00ff
	v = (v&0x0000ffff0000ffff)<<16 | (v>>16)&0x0000ffff0000ffff
	return (v << 32) | (v >> 32)
}

func cleanPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `/\.`)
	value = strings.ReplaceAll(value, string(filepath.Separator), "_")
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	if value == "" || value == ".." {
		return ""
	}
	return value
}

func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "cache root unavailable")
}

