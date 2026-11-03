package metricmeta

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

const MaxDescriptors = 128
const Origin = "authorized_series_metadata"

// Descriptor describes an authorized input series, never the units/type of a
// derived PromQL expression's output. It contains no sample or arbitrary label.
type Descriptor struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Unit        string `json:"unit"`
	StorageType string `json:"storage_type"`
	Temporality string `json:"temporality"`
	Monotonic   bool   `json:"monotonic"`
	SourceKey   string `json:"source_key"`
}

// Kind retains the existing catalog classification; both discovery and query
// evidence use this one rule instead of inferring semantics from metric names.
func Kind(storageType, temporality string, monotonic bool) string {
	if storageType == "sum" {
		if strings.HasSuffix(strings.ToLower(temporality), "cumulative") && monotonic {
			return "counter"
		}
		return "gauge"
	}
	if storageType == "" {
		return "unknown"
	}
	return storageType
}

func (d Descriptor) key() string {
	return strings.Join([]string{d.Name, d.SourceKey, d.StorageType, d.Temporality, strconv.FormatBool(d.Monotonic), d.Unit}, "\x00")
}

type Tracker struct {
	mu        sync.Mutex
	items     map[string]Descriptor
	truncated bool
}

func (t *Tracker) Observe(d Descriptor) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if d.Name == "" || len(d.Name) > 1024 || len(d.Unit) > 256 || len(d.StorageType) > 64 || len(d.Temporality) > 64 || len(d.SourceKey) > 128 {
		t.truncated = true
		return
	}
	d.Type = Kind(d.StorageType, d.Temporality, d.Monotonic)
	if t.items == nil {
		t.items = map[string]Descriptor{}
	}
	key := d.key()
	if _, exists := t.items[key]; exists {
		return
	}
	if len(t.items) >= MaxDescriptors {
		t.truncated = true
		return
	}
	t.items[key] = d
}

func (t *Tracker) Snapshot() ([]Descriptor, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	keys := make([]string, 0, len(t.items))
	for key := range t.items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Descriptor, 0, len(keys))
	for _, key := range keys {
		result = append(result, t.items[key])
	}
	return result, !t.truncated
}
