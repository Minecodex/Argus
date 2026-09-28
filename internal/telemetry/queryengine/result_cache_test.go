package queryengine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countedEngine struct {
	calls   int
	partial bool
	fail    bool
}

func (e *countedEngine) Execute(context.Context, Request) (Result, error) {
	e.calls++
	if e.fail {
		return Result{}, errors.New("unavailable")
	}
	sample := time.Unix(10, 0).UTC()
	return Result{Language: LanguageKQL, ResultType: "log_entries", Data: []map[string]any{{"body": "password=secret", "count": uint64(9007199254740993)}}, Meta: QueryMeta{ScannedBytes: 10, ScannedRows: 1, Partial: e.partial, LatestSampleAt: &sample}}, nil
}
func cacheRequest() Request {
	return Request{CacheNamespace: strings.Repeat("a", 64), Language: LanguageKQL, Expression: "*", Start: time.Unix(1, 0), End: time.Unix(20, 0), Scope: Scope{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", ResourceIDs: []uuid.UUID{uuid.New()}, SourceKeys: []string{"source:1"}, AuthorizationVersion: 1}}
}
func TestResultCachePreservesProjectionAuditCostAndOriginalFreshness(t *testing.T) {
	engine, audit := &countedEngine{}, &captureAudit{}
	cache := NewResultCache(1<<20, 30*time.Second)
	c := Coordinator{KQL: engine, Audit: audit, Cache: cache}
	request := cacheRequest()
	first, err := c.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if engine.calls != 1 || first.Meta.CacheHit || !second.Meta.CacheHit || !audit.event.Meta.CacheHit || !audit.event.Success {
		t.Fatal("cache hit bypassed the cache/audit contract")
	}
	if second.Meta.QueryCompletedAt == nil || !second.Meta.QueryCompletedAt.Equal(*first.Meta.QueryCompletedAt) || second.Meta.LatestSampleAt == nil || second.Meta.LatestSampleAt.Unix() != 10 || second.Meta.SampleTimeBasis != "observed_query_samples" || second.Meta.IngestionStatus != "unknown" {
		t.Fatalf("freshness fabricated on cache hit: %+v", second.Meta)
	}
	if second.Meta.ScannedBytes != 10 || second.Meta.ReturnedRows != 1 {
		t.Fatal("cache lost cumulative budget cost")
	}
	encoded, _ := json.Marshal(second.Data)
	if strings.Contains(string(encoded), "secret") || !strings.Contains(string(encoded), "9007199254740993") {
		t.Fatalf("cached data lost masking or integer precision: %s", encoded)
	}
	second.Data.([]any)[0].(map[string]any)["body"] = "mutated"
	third, _ := c.Execute(context.Background(), request)
	encoded, _ = json.Marshal(third.Data)
	if strings.Contains(string(encoded), "mutated") {
		t.Fatal("consumer mutated shared cache state")
	}
}

func TestCacheIdentitySeparatesScopeDefinitionTimeAndBudget(t *testing.T) {
	original := cacheRequest()
	base := resultCacheKey(original)
	changes := []func(*Request){
		func(r *Request) { r.CacheNamespace = strings.Repeat("b", 64) },
		func(r *Request) { r.Scope.EnterpriseID = uuid.New() },
		func(r *Request) { r.Scope.SubjectID = uuid.New() },
		func(r *Request) { r.Scope.SubjectType = "service_account" },
		func(r *Request) { r.Scope.AuthorizationVersion++ },
		func(r *Request) { r.Scope.ResourceIDs = []uuid.UUID{uuid.New()} },
		func(r *Request) { r.Scope.SourceKeys = []string{"source:2"} },
		func(r *Request) { r.Language = LanguagePromQL },
		func(r *Request) { r.Expression = "other" },
		func(r *Request) { r.Pipeline = "limit 1" },
		func(r *Request) { r.Variables = map[string]any{"environment": "prod"} },
		func(r *Request) { r.Operation = "queryTrace" },
		func(r *Request) { r.Instant = true },
		func(r *Request) { r.Start = r.Start.Add(time.Second) },
		func(r *Request) { r.End = r.End.Add(time.Second) },
		func(r *Request) { r.Step = time.Minute },
		func(r *Request) { r.Budget.MaxRows = 1 },
		func(r *Request) { r.Budget.MaxScanBytes = 1 },
		func(r *Request) { r.Budget.MaxResultBytes = 1 },
		func(r *Request) { r.Budget.Timeout = time.Millisecond },
	}
	for i, change := range changes {
		r := original
		change(&r)
		if key := resultCacheKey(r); key == "" || key == base {
			t.Fatalf("identity change %d was ignored", i)
		}
	}
	for _, change := range []func(*Request){func(r *Request) { r.CacheNamespace = "" }, func(r *Request) { r.Scope.SubjectID = uuid.Nil }, func(r *Request) { r.Scope.ResourceIDs = nil }, func(r *Request) { r.Scope.SourceKeys = nil }, func(r *Request) { r.Start = time.Time{} }} {
		r := original
		change(&r)
		if resultCacheKey(r) != "" {
			t.Fatal("incomplete/ad hoc scope enabled caching")
		}
	}
}

func TestCacheTTLBoundedEvictionAndUncacheableResults(t *testing.T) {
	now := time.Now()
	cache := NewResultCache(600, time.Second)
	cache.now = func() time.Time { return now }
	r := Result{Data: strings.Repeat("x", 200)}
	cache.put("a", r)
	cache.put("b", r)
	if cache.bytes > cache.limit {
		t.Fatal("cache exceeded its memory bound")
	}
	if _, ok := cache.get("a"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, ok := cache.get("b"); !ok {
		t.Fatal("new result was not cached")
	}
	now = now.Add(time.Second)
	if _, ok := cache.get("b"); ok {
		t.Fatal("expired cache result survived")
	}
	cache.put("large", Result{Data: strings.Repeat("x", 1000)})
	cache.put("partial", Result{Meta: QueryMeta{Partial: true}})
	if cache.lru.Len() != 0 {
		t.Fatal("oversized or partial result cached")
	}
}

func TestCoordinatorNeverCachesFailuresPartialOrCancelledExecutions(t *testing.T) {
	for _, partial := range []bool{false, true} {
		e := &countedEngine{partial: partial, fail: !partial}
		c := Coordinator{KQL: e, Cache: NewResultCache(1<<20, time.Minute)}
		r := cacheRequest()
		_, _ = c.Execute(context.Background(), r)
		_, _ = c.Execute(context.Background(), r)
		if e.calls != 2 {
			t.Fatal("partial/error result was reused")
		}
	}
	e := &countedEngine{}
	c := Coordinator{KQL: e, Cache: NewResultCache(1<<20, time.Minute)}
	r := cacheRequest()
	_, _ = c.Execute(context.Background(), r)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Execute(ctx, r); err == nil {
		t.Fatal("cache bypassed cancellation")
	}
	r.Budget.MaxResultBytes = 1
	if _, err := c.Execute(context.Background(), r); !errors.Is(err, ErrBudget) {
		t.Fatal("cache bypassed a smaller budget")
	}
}
