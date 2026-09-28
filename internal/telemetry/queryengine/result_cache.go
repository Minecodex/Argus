package queryengine

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
)

// ResultCache contains projected results only. Scope authorization remains the
// caller's responsibility before and after every execution, including a hit.
// Serialized entries are immutable and the cache is disposable per query process.
type ResultCache struct {
	mu           sync.Mutex
	entries      map[string]*list.Element
	lru          *list.List
	bytes, limit int64
	ttl          time.Duration
	now          func() time.Time
}
type cachedResult struct {
	key     string
	data    []byte
	expires time.Time
}

func NewResultCache(limit int64, ttl time.Duration) *ResultCache {
	return &ResultCache{entries: map[string]*list.Element{}, lru: list.New(), limit: limit, ttl: ttl, now: time.Now}
}

func resultCacheKey(request Request) string {
	// Namespace is minted by the dashboard domain from its immutable revision
	// and source/parameter contract. Generic/ad hoc queries do not opt in.
	if len(request.CacheNamespace) != 64 || request.Scope.SubjectID == uuid.Nil || request.Scope.EnterpriseID == uuid.Nil || len(request.Scope.ResourceIDs) == 0 || len(request.Scope.SourceKeys) == 0 || request.Start.IsZero() || request.End.IsZero() {
		return ""
	}
	if _, err := hex.DecodeString(request.CacheNamespace); err != nil {
		return ""
	}
	request.Scope.ResourceIDs = slices.Clone(request.Scope.ResourceIDs)
	request.Scope.SourceKeys = slices.Clone(request.Scope.SourceKeys)
	slices.SortFunc(request.Scope.ResourceIDs, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	slices.Sort(request.Scope.SourceKeys)
	encoded, err := json.Marshal(struct {
		Request    Request
		Projection string
	}{request, datapolicy.Version})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (cache *ResultCache) get(key string) (Result, bool) {
	if cache == nil || key == "" {
		return Result{}, false
	}
	cache.mu.Lock()
	entry := cache.entries[key]
	if entry == nil {
		cache.mu.Unlock()
		return Result{}, false
	}
	value := entry.Value.(cachedResult)
	if !cache.now().Before(value.expires) {
		cache.remove(entry)
		cache.mu.Unlock()
		return Result{}, false
	}
	cache.lru.MoveToFront(entry)
	cache.mu.Unlock()
	var result Result
	decoder := json.NewDecoder(bytes.NewReader(value.data))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		return Result{}, false
	}
	result.Meta.CacheHit = true
	return result, true
}

func (cache *ResultCache) put(key string, result Result) {
	if cache == nil || key == "" || result.Meta.Partial || cache.limit <= 0 || cache.ttl <= 0 {
		return
	}
	encoded, err := json.Marshal(result)
	if err != nil || int64(len(encoded)) > cache.limit {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if old := cache.entries[key]; old != nil {
		cache.remove(old)
	}
	for cache.bytes+int64(len(encoded)) > cache.limit || cache.lru.Len() >= 1024 {
		cache.remove(cache.lru.Back())
	}
	cache.entries[key] = cache.lru.PushFront(cachedResult{key, encoded, cache.now().Add(cache.ttl)})
	cache.bytes += int64(len(encoded))
}

func (cache *ResultCache) remove(entry *list.Element) {
	value := entry.Value.(cachedResult)
	delete(cache.entries, value.key)
	cache.bytes -= int64(len(value.data))
	cache.lru.Remove(entry)
}
