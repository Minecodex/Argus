package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTargetCacheNamespaceIsolatesPublishedSourceAndParameterContracts(t *testing.T) {
	runtime := Runtime{cacheDashboard: uuid.New(), cacheRevision: uuid.New()}
	panel := Panel{ID: "cpu", SourceBinding: SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}}
	target := Target{ID: "a", SourceDefinition: Definition{DSL: &DSL{Expression: "up"}}}
	scope := queryScope{From: time.Unix(1, 0), To: time.Unix(2, 0), Resources: []ResourceScope{{ID: uuid.New(), Type: "host"}}, Sources: []ResolvedSource{{ID: uuid.New(), Revision: 1, Generation: uuid.New(), CapabilityVersion: "v1"}}}
	base := runtime.targetCacheNamespace(panel, target, nil, nil, nil, scope)
	if base == "" || (Runtime{}).targetCacheNamespace(panel, target, nil, nil, nil, scope) != "" {
		t.Fatal("draft/publication cache boundary failed")
	}
	for _, change := range []func(*Runtime, *Panel, *Target, *queryScope){
		func(r *Runtime, _ *Panel, _ *Target, _ *queryScope) { r.cacheDashboard = uuid.New() },
		func(r *Runtime, _ *Panel, _ *Target, _ *queryScope) { r.cacheRevision = uuid.New() },
		func(_ *Runtime, p *Panel, _ *Target, _ *queryScope) { p.ID = "other" },
		func(_ *Runtime, p *Panel, _ *Target, _ *queryScope) { p.SourceBinding.CapabilityVersion = "v2" },
		func(_ *Runtime, _ *Panel, t *Target, _ *queryScope) { t.ID = "detail" },
		func(_ *Runtime, _ *Panel, t *Target, _ *queryScope) { t.SourceDefinition.DSL.Expression = "other" },
		func(_ *Runtime, _ *Panel, _ *Target, s *queryScope) { s.Sources[0].Revision++ },
		func(_ *Runtime, _ *Panel, _ *Target, s *queryScope) { s.Sources[0].Generation = uuid.New() },
		func(_ *Runtime, _ *Panel, _ *Target, s *queryScope) {
			s.Sources = append(s.Sources, ResolvedSource{ID: uuid.New()})
		},
	} {
		r := runtime
		p := panel
		var v Target
		var s queryScope
		raw, _ := json.Marshal(target)
		_ = json.Unmarshal(raw, &v)
		raw, _ = json.Marshal(scope)
		_ = json.Unmarshal(raw, &s)
		change(&r, &p, &v, &s)
		if r.targetCacheNamespace(p, v, nil, nil, nil, s) == base {
			t.Fatal("published identity change reused cached result")
		}
	}
	selection := map[string]Selection{"environment": {Values: []string{"prod"}}}
	for _, key := range []string{runtime.targetCacheNamespace(panel, target, selection, nil, nil, scope), runtime.targetCacheNamespace(panel, target, nil, selection, nil, scope), runtime.targetCacheNamespace(panel, target, nil, nil, selection, scope)} {
		if key == base {
			t.Fatal("parameter context omitted")
		}
	}
}

func TestCachedTraceTotalsKeepEmptyResultSemantics(t *testing.T) {
	for _, positive := range []any{float64(2), json.Number("2")} {
		if emptyTypedResult("traces", map[string]any{"queryBasicTraces": map[string]any{"total": positive}}) {
			t.Fatal("cached positive total became no_data")
		}
	}
	if !emptyTypedResult("traces", map[string]any{"queryBasicTraces": map[string]any{"total": json.Number("0")}}) {
		t.Fatal("cached zero total became success")
	}
}
