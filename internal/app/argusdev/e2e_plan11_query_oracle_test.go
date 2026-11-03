package argusdev

import (
	"encoding/json"
	"testing"
)

func TestPlan11PointOracleRejectsRequestedWindowSubstitutedForActualPoints(t *testing.T) {
	body := []byte(`{"status":"success","argus_meta":{"latest_sample_at":"2026-10-30T00:32:13Z"},"data":{"resultType":"matrix","result":[{"metric":{},"values":[[1793320216,"0.79"],[1793320276,"0.78"],[1793320336,"0.79"]]}]},"query_context":{"resources":[{"type":"host","id":"host"}],"from":"2026-10-30T00:02:16Z","to":"2026-10-30T00:32:16Z"}}`)
	w, err := plan11OriginalPointEvidence(body, "host")
	if err != nil {
		t.Fatal(err)
	}
	n := 3
	c := plan11Conclusion{PointCount: &n, FirstPointAt: "2026-10-30T00:30:16Z", LastPointAt: "2026-10-30T00:32:16Z"}
	if err = plan11MatchPointConclusion(c, w); err != nil {
		t.Fatal(err)
	}
	c.FirstPointAt = "2026-10-30T00:02:16Z"
	if plan11MatchPointConclusion(c, w) == nil {
		t.Fatal("requested start passed as actual first point")
	}
	var data map[string]any
	_ = json.Unmarshal(body, &data)
	data["query_context"].(map[string]any)["resources"] = []any{map[string]any{"type": "host", "id": "host"}, map[string]any{"type": "host", "id": "other"}}
	expanded, _ := json.Marshal(data)
	if _, err = plan11OriginalPointEvidence(expanded, "host"); err == nil {
		t.Fatal("query included an unrequested host")
	}
}
func TestPlan11ReadOracleRequiresTheActualFile(t *testing.T) {
	path := "/workspace/queries/metrics/query.json"
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`[{"tool":"read","status":"succeeded","input":{"path":"/workspace/unrelated.json"}}]`, false},
		{`[{"tool":"bash","status":"succeeded","exit_code":0,"partial":false,"input":{"command":"date"}}]`, false},
		{`[{"tool":"read","status":"succeeded","input":{"path":"queries/metrics/query.json"}}]`, true},
		{`[{"tool":"bash","status":"succeeded","exit_code":0,"partial":false,"input":{"command":"cat /workspace/queries/metrics/query.json"}}]`, true},
		{`[{"tool":"bash","status":"succeeded","exit_code":0,"partial":false,"input":{"command":"argus-metric-evidence queries/metrics/query.json --unit 1"}}]`, true},
		{`[{"tool":"bash","status":"succeeded","exit_code":0,"partial":false,"input":{"command":"cd /workspace/queries/metrics && argus-metric-evidence query.json --unit 1"}}]`, true},
		{`[{"tool":"bash","status":"succeeded","exit_code":0,"partial":false,"input":{"command":"cd /workspace/unrelated && argus-metric-evidence query.json --unit 1"}}]`, false},
	} {
		if plan11ReadActualQuery(tc.raw, path) != tc.want {
			t.Fatal(tc)
		}
	}
}

func TestPlan11MetricInputIsExactlyTheRequestedHost(t *testing.T) {
	if !plan11ExactHostArguments(map[string]any{"resource_ids": []any{"host"}}, "host") {
		t.Fatal("exact legacy scope rejected")
	}
	if plan11ExactHostArguments(map[string]any{"resource_ids": []any{"host", "other"}}, "host") {
		t.Fatal("broader legacy scope accepted")
	}
	scope := map[string]any{"mode": "explicit", "resources": []any{map[string]any{"type": "host", "id": "host"}}}
	if !plan11ExactHostArguments(map[string]any{"scope": scope}, "host") {
		t.Fatal("exact typed scope rejected")
	}
	scope["resources"] = []any{map[string]any{"type": "application", "id": "host"}}
	if plan11ExactHostArguments(map[string]any{"scope": scope}, "host") {
		t.Fatal("cross-type ID accepted")
	}
}

func TestPlan11RestrictedScopeOracleRejectsFreeQueriesAndDrafts(t *testing.T) {
	for _, tc := range []struct {
		category, name string
		allowed        bool
	}{
		{"dashboard", "query", true}, {"dashboard", "context.resolve", true},
		{"dashboard", "draft.create", false}, {"dashboard", "publish.preview", false},
		{"metric", "query", false}, {"log", "query", false},
		{"observe", "capabilities.get", false}, {"observe", "resources.list", true},
	} {
		if plan11RestrictedBusiness(tc.category, tc.name) != tc.allowed {
			t.Fatal(tc)
		}
	}
}
