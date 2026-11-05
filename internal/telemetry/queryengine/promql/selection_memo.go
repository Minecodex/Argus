package promqlengine

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/annotations"
)

// A memo belongs to one Querier, whose tenant, authorized source snapshot, time
// bounds and budgets are already frozen. Never share these results across queries.
// Repeated AST selectors get separate cursors/iterators over the same immutable
// samples, while actual database reads and their progress are charged only once.
type selectionMemo struct {
	mu      sync.Mutex
	entries map[string]*selectionRead
}

type selectionRead struct {
	ready    chan struct{}
	items    []storage.Series
	err      error
	warnings annotations.Annotations
}

func selectionKey(sorted bool, hints *storage.SelectHints, matchers []*labels.Matcher) string {
	values := []string{}
	for _, matcher := range matchers {
		if matcher != nil {
			values = append(values, matcher.String())
		}
	}
	slices.Sort(values)
	key, _ := json.Marshal(struct {
		Sorted   bool
		Hints    *storage.SelectHints
		Matchers []string
	}{sorted, hints, values})
	return string(key)
}

func (m *selectionMemo) selectSet(ctx context.Context, key string, load func() storage.SeriesSet) storage.SeriesSet {
	m.mu.Lock()
	if m.entries == nil {
		m.entries = map[string]*selectionRead{}
	}
	entry, found := m.entries[key]
	if !found {
		entry = &selectionRead{ready: make(chan struct{})}
		m.entries[key] = entry
	}
	m.mu.Unlock()
	if !found {
		set := load()
		for set.Next() {
			entry.items = append(entry.items, set.At())
		}
		entry.err, entry.warnings = set.Err(), maps.Clone(set.Warnings())
		close(entry.ready)
	} else {
		select {
		case <-entry.ready:
		case <-ctx.Done():
			return storage.ErrSeriesSet(ctx.Err())
		}
	}
	return &replayedSeriesSet{read: entry, index: -1}
}

type replayedSeriesSet struct {
	read  *selectionRead
	index int
}

func (s *replayedSeriesSet) Next() bool { s.index++; return s.index < len(s.read.items) }
func (s *replayedSeriesSet) At() storage.Series {
	if s.index < 0 || s.index >= len(s.read.items) {
		return nil
	}
	return s.read.items[s.index]
}
func (s *replayedSeriesSet) Err() error                        { return s.read.err }
func (s *replayedSeriesSet) Warnings() annotations.Annotations { return maps.Clone(s.read.warnings) }
