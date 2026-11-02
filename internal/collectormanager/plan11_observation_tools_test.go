package collectormanager_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Uses the real SDK observations already transported through Collector, Kafka
// and Writer. Only identity/enrollment provisioning is a controlled fixture.
func verifyPlan11CollectedObservationTools(t *testing.T, f *nativeSDKPipeline, service telemetry.Service, user, role uuid.UUID) {
	t.Helper()
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'workspace.use') ON CONFLICT DO NOTHING", role)
	registry := mcp.NewRegistry()
	key := bytes.Repeat([]byte{9}, 32)
	if err := (toolgateway.ObservationTools{Base: toolgateway.ResourceTools{Store: f.store}, Telemetry: service, Monitoring: monitoring.Service{Store: f.store, CursorKey: key}, Dashboard: dashboard.Service{Store: f.store, ProvideDefaults: true}, CursorKey: key}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := (telemetry.Tools{Service: service}).Register(registry); err != nil {
		t.Fatal(err)
	}
	gateway, err := toolgateway.New(registry, "plan11-real-sdk")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: gateway.Revision}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	principal, err := (conversation.Service{Store: f.store}).ToolPrincipal(f.ctx, f.enterprise, user, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(category, name string, args map[string]any) (toolruntime.Result, error) {
		t.Helper()
		t.Logf("Plan11 real pipeline invoke %s/%s", category, name)
		// Match the actual model JSON wire representation, including nested arrays.
		wire, e := json.Marshal(args)
		if e != nil {
			return toolruntime.Result{}, e
		}
		if e = json.Unmarshal(wire, &args); e != nil {
			return toolruntime.Result{}, e
		}
		for _, step := range []struct {
			name string
			args map[string]any
		}{{"tool.search", map[string]any{"category": category}}, {"tool.describe", map[string]any{"category": category, "name": name}}} {
			if _, e := set.Invoke(f.ctx, step.name, toolruntime.Invocation{ID: uuid.New(), Principal: principal, Arguments: step.args}); e != nil {
				return toolruntime.Result{}, e
			}
		}
		result, e := set.Invoke(f.ctx, "tool.invoke", toolruntime.Invocation{ID: uuid.New(), Principal: principal, Arguments: map[string]any{"category": category, "name": name, "arguments": args}})
		if e != nil {
			return result, fmt.Errorf("%s/%s: %w", category, name, e)
		}
		return result, nil
	}
	resources, err := invoke("observe", "resources.list", map[string]any{"resource_types": []string{"application"}})
	if err != nil {
		t.Fatal(err)
	}
	items := resources.Data["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != f.apps[0].app.String() {
		t.Fatal("resource tool crossed application scope")
	}
	ref := map[string]any{"type": "application", "id": f.apps[0].app.String()}
	caps, err := invoke("observe", "capabilities.get", map[string]any{"resources": []any{ref}, "sections": []string{"sources", "next_tools"}})
	if err != nil {
		t.Fatal(err)
	}
	if caps.Presentation == nil || len(caps.Presentation.Resources) != 1 {
		t.Fatal("capability presentation lost authorized resource")
	}
	sourceRows := caps.Data["items"].([]any)[0].(map[string]any)["sections"].(map[string]any)["sources"].(map[string]any)["items"].([]any)
	if len(sourceRows) == 0 {
		t.Fatal("collected source missing")
	}
	keyValue := sourceRows[0].(map[string]any)["source_key"].(string)
	if strings.Contains(keyValue, f.apps[1].source) {
		t.Fatal("source tool crossed applications")
	}
	scope := map[string]any{"mode": "explicit", "resources": []any{ref}, "source_type": "otlp", "source_keys": []string{keyValue}}
	from, to := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), time.Now().Add(time.Second).UTC().Format(time.RFC3339)
	catalog, err := invoke("observe", "catalog.query", map[string]any{"scope": scope, "signal": "metrics", "kind": "metrics", "from": from, "to": to, "limit": 100})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, raw := range catalog.Data["metrics"].([]any) {
		m := raw.(map[string]any)
		found = found || m["name"] == "plan8_sdk_request_count" && m["observed"] == true && m["type"] == "counter"
	}
	if !found {
		t.Fatal("native catalog omitted real metric")
	}
	metrics, err := invoke("metric", "query", map[string]any{"scope": scope, "query": "sum(plan8_sdk_request_count)", "from": from, "to": to})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(metrics.Data)
	if !bytes.Contains(encoded, []byte(`"2"`)) || metrics.Data["query_context"] == nil || metrics.Presentation == nil {
		t.Fatal("native metric result lost value, scope or presentation")
	}
	var metricWire struct {
		Meta struct {
			Latest string `json:"latest_sample_at"`
			Points struct {
				Count    int
				From, To string
				Basis    string `json:"time_basis"`
			} `json:"returned_points"`
		} `json:"argus_meta"`
	}
	if json.Unmarshal(encoded, &metricWire) != nil || metricWire.Meta.Latest == "" || metricWire.Meta.Points.Count != 1 || metricWire.Meta.Points.From == "" || metricWire.Meta.Points.To != metricWire.Meta.Points.From || metricWire.Meta.Points.Basis != "promql_evaluation_timestamps" {
		t.Fatal("native metric output lost actual observation/evaluation time facts")
	}
	logs, err := invoke("log", "query", map[string]any{"scope": scope, "query": "* | limit 10", "from": from, "to": to})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(logs.Data)
	if !bytes.Contains(encoded, []byte("observe-first-event")) || bytes.Contains(encoded, []byte(f.private)) {
		t.Fatal("native log evidence or private field boundary failed")
	}
	traces, err := invoke("trace", "query", map[string]any{"scope": scope, "application_view": "services", "from": from, "to": to})
	if err != nil {
		t.Fatal(err)
	}
	if traces.Presentation == nil || traces.Data["query_context"] == nil {
		t.Fatal("APM lost typed scope/presentation")
	}
	scope["source_keys"] = []string{f.apps[1].source + ":1"}
	if _, err = invoke("metric", "query", map[string]any{"scope": scope, "query": "sum(plan8_sdk_request_count)", "from": from, "to": to}); err == nil {
		t.Fatal("native query accepted another application's source")
	}
	t.Log("Plan11 native Search/Describe/Invoke: actual SDK resource/source/catalog, metrics=2, correlated logs, APM, typed presentations and unauthorized-source rejection passed")
}
