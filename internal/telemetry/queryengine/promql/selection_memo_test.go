package promqlengine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestRepeatedRateSelectorsReadOnceWithoutChangingCacheRatio(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	var memo selectionMemo
	reads := 0
	queryable := storage.MockQueryable{MockQuerier: &storage.MockQuerier{SelectMockFunction: func(sorted bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
		return memo.selectSet(t.Context(), selectionKey(sorted, hints, matchers), func() storage.SeriesSet {
			reads++
			name, increment := "hits", 80.0
			for _, matcher := range matchers {
				if matcher.Name == "__name__" && matcher.Value == "misses" {
					name, increment = "misses", 20
				}
			}
			return &seriesSet{items: []*series{{labels: labelSet(map[string]string{"__name__": name}), samples: chunks.SampleSlice{querySample{timestamp: start.UnixMilli(), value: 0}, querySample{timestamp: start.Add(time.Minute).UnixMilli(), value: increment}}}}, index: -1}
		})
	}}}
	engine := promql.NewEngine(promql.EngineOpts{MaxSamples: 1000, Timeout: time.Second, LookbackDelta: 5 * time.Minute, Parser: parser.NewParser(parser.Options{})})
	expression := `(rate(hits[5m]) / (rate(hits[5m]) + rate(misses[5m]))) and (rate(hits[5m]) + rate(misses[5m]) > 0)`
	query, err := engine.NewInstantQuery(t.Context(), &queryable, promql.NewPrometheusQueryOpts(false, 5*time.Minute), expression, start.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()
	result := query.Exec(t.Context())
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	values, ok := result.Value.(promql.Vector)
	if !ok || len(values) != 1 || values[0].F != .8 || reads != 2 {
		t.Fatalf("ratio or physical reads changed: values=%v reads=%d", result.Value, reads)
	}
}

func TestSelectionMemoWaitCancellationAndQueryIsolation(t *testing.T) {
	var memo selectionMemo
	var calls atomic.Int32
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	load := func() storage.SeriesSet {
		calls.Add(1)
		close(started)
		<-release
		return &seriesSet{items: []*series{{labels: labelSet(map[string]string{"__name__": "x"})}}, index: -1}
	}
	go func() {
		defer close(done)
		set := memo.selectSet(t.Context(), "same", load)
		if !set.Next() {
			t.Error("initial result lost")
		}
	}()
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := memo.selectSet(ctx, "same", load).Err(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	<-done
	first := memo.selectSet(t.Context(), "same", load)
	second := memo.selectSet(t.Context(), "same", load)
	if !first.Next() || first.Next() || !second.Next() || second.Next() || calls.Load() != 1 {
		t.Fatal("shared cursor or duplicate read")
	}
	var nextQuery selectionMemo
	nextQuery.selectSet(t.Context(), "same", func() storage.SeriesSet { calls.Add(1); return storage.ErrSeriesSet(ErrBudget) })
	if calls.Load() != 2 {
		t.Fatal("results leaked across query scopes")
	}
}

func TestSelectionMemoKeyRetainsLimitsMatchersAndSortContract(t *testing.T) {
	a := labels.MustNewMatcher(labels.MatchEqual, "__name__", "a")
	b := labels.MustNewMatcher(labels.MatchEqual, "host", "b")
	key := selectionKey(true, &storage.SelectHints{Limit: 10}, []*labels.Matcher{a, b})
	if key != selectionKey(true, &storage.SelectHints{Limit: 10}, []*labels.Matcher{b, a}) {
		t.Fatal("matcher order changed predicate identity")
	}
	if key == selectionKey(true, &storage.SelectHints{Limit: 1}, []*labels.Matcher{a, b}) || key == selectionKey(false, &storage.SelectHints{Limit: 10}, []*labels.Matcher{a, b}) || key == selectionKey(true, &storage.SelectHints{Limit: 10}, []*labels.Matcher{a}) {
		t.Fatal("distinct selection constraints shared a cache key")
	}
}
