package toolgateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

type discoveryCache struct {
	mu      sync.Mutex
	entries map[string]discoveryEntry
}
type discoveryEntry struct {
	result  toolruntime.Result
	expires time.Time
}

func discoveryKey(revision, operation string, call toolruntime.Invocation) string {
	data, _ := json.Marshal([]any{revision, operation, call.Principal.EnterpriseID, call.Principal.UserID, call.Principal.AuthorizationVersion, call.Principal.Permissions, call.Arguments})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func cloneDiscovery(value toolruntime.Result) toolruntime.Result {
	value.Data = cloneDiscoveryValue(value.Data).(map[string]any)
	return value
}

func cloneDiscoveryValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value))
		for key, item := range value {
			copy[key] = cloneDiscoveryValue(item)
		}
		return copy
	case []map[string]any:
		copy := make([]map[string]any, len(value))
		for i, item := range value {
			copy[i] = cloneDiscoveryValue(item).(map[string]any)
		}
		return copy
	case []any:
		copy := make([]any, len(value))
		for i, item := range value {
			copy[i] = cloneDiscoveryValue(item)
		}
		return copy
	case []string:
		return append([]string{}, value...)
	default:
		return value
	}
}
func (cache *discoveryCache) get(key string) (toolruntime.Result, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok || time.Now().After(entry.expires) {
		delete(cache.entries, key)
		return toolruntime.Result{}, false
	}
	return cloneDiscovery(entry.result), true
}
func (cache *discoveryCache) put(key string, value toolruntime.Result) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = map[string]discoveryEntry{}
	}
	if len(cache.entries) >= 256 {
		oldest := ""
		var expiry time.Time
		for key, entry := range cache.entries {
			if oldest == "" || entry.expires.Before(expiry) {
				oldest = key
				expiry = entry.expires
			}
		}
		delete(cache.entries, oldest)
	}
	cache.entries[key] = discoveryEntry{cloneDiscovery(value), time.Now().Add(time.Minute)}
}
