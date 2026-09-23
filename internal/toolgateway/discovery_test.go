package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
)

func formalDiscoveryGateway(t *testing.T) (*Gateway, *toolruntime.Set) {
	t.Helper()
	store := &postgres.Store{}
	base := ResourceTools{Store: store}
	registry := mcp.NewRegistry()
	for _, register := range []func(*mcp.Registry) error{base.Register, (LifecycleTools{Base: base}).Register, (CollectorPreviewTools{Base: base}).Register, (telemetry.Tools{Service: telemetry.Service{Store: store}}).Register, (workspace.Service{}).RegisterTools} {
		if err := register(registry); err != nil {
			t.Fatal(err)
		}
	}
	g, err := New(registry, "discovery-test")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	return g, set
}

func TestFormalDiscoveryCoversEveryToolAndBusinessQuery(t *testing.T) {
	g, set := formalDiscoveryGateway(t)
	if len(g.manifests) != 52 {
		t.Fatalf("formal tools=%d", len(g.manifests))
	}
	for _, m := range g.manifests {
		if strings.HasSuffix(m.ToolID, ".commit") {
			t.Fatal("hidden commit described")
		}
		if m.Description == m.Title+". "+m.ToolID || len(m.DiscoveryHash) != 64 || len(m.Keywords) == 0 || len(m.Examples) == 0 || m.ResultDescription == "" {
			t.Fatalf("incomplete %s", m.ToolID)
		}
	}
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"*"}}
	for _, tc := range []struct{ category, query, want string }{
		{"host", "创建", "create.preview"}, {"host", "新增主机", "create.preview"}, {"host", "create", "create.preview"}, {"host", "列表", "list"}, {"host", "list", "list"}, {"host", "删除", "delete.preview"}, {"host", "delete", "delete.preview"},
		{"host", "主机 删除", "delete.preview"}, {"host", "采集器 升级", "collector.upgrade.preview"}, {"host", "install", "collector.install.preview"}, {"host", "uninstall", "collector.uninstall.preview"},
		{"k8s", "命名空间", "namespace.list"}, {"k8s", "容器 日志", "pod.logs"}, {"metric", "CPU", "query"}, {"log", "错误日志", "query"}, {"trace", "慢调用", "query"}, {"connector", "清单", "list"}, {"workflow", "发布 下载", "publish_file"},
	} {
		t.Run(tc.category+"/"+tc.query, func(t *testing.T) {
			call := toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": tc.category, "query": tc.query}}
			got, err := set.Invoke(t.Context(), "tool.search", call)
			if err != nil {
				t.Fatal(err)
			}
			items := got.Data["items"].([]map[string]any)
			found := false
			for _, item := range items {
				if item["name"] == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %s in %+v", tc.want, items)
			}
			if tc.query == "install" {
				for _, item := range items {
					if strings.Contains(item["name"].(string), "uninstall") {
						t.Fatal("install matched uninstall")
					}
				}
			}
			if tc.query == "主机 删除" && (len(items) != 1 || items[0]["name"] != "delete.preview") {
				t.Fatalf("unrelated category hits: %+v", items)
			}
		})
	}
	call := toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": "host", "query": "创建"}}
	if _, err := set.Invoke(t.Context(), "tool.search", call); err != nil {
		t.Fatal(err)
	}
	call.Principal.Permissions = []string{"host.read"}
	got, err := set.Invoke(t.Context(), "tool.search", call)
	if err != nil || len(got.Data["items"].([]map[string]any)) != 0 {
		t.Fatal("cached privileged discovery leaked after revocation")
	}
	call.Arguments = map[string]any{"category": "host", "name": "create.preview"}
	if _, err := set.Invoke(t.Context(), "tool.describe", call); err == nil {
		t.Fatal("Describe ignored revocation")
	}
	call.Principal = p
	value, err := set.Invoke(t.Context(), "tool.describe", call)
	if err != nil {
		t.Fatal(err)
	}
	if value.Data["result_description"] == "" || len(value.Data["examples"].([]any)) == 0 || value.Data["discovery_hash"] == "" {
		t.Fatal("Describe omitted business semantics")
	}
	value.Data["keywords"].([]any)[0] = "tampered"
	again, err := set.Invoke(t.Context(), "tool.describe", call)
	if err != nil {
		t.Fatal(err)
	}
	if again.Data["keywords"].([]any)[0] == "tampered" {
		t.Fatal("cached metadata is mutable")
	}
}

func discoveryFixtureGateway(t *testing.T, doc mcp.Discovery, reverse bool) *Gateway {
	t.Helper()
	r := mcp.NewRegistry()
	ids := []string{"host.alpha", "host.beta"}
	if reverse {
		ids[0], ids[1] = ids[1], ids[0]
	}
	for _, id := range ids {
		if err := r.Register(mcp.Metadata{ID: id, Discovery: doc, Risk: "read", Visibility: mcp.Visible, InputVersion: "1", OutputVersion: "1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) { return mcp.Result{}, nil }}); err != nil {
			t.Fatal(err)
		}
	}
	g, err := New(r, "fixed-code")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestDiscoveryRevisionRankingPagingAndRestore(t *testing.T) {
	doc := fixtureDiscovery("stable")
	a := discoveryFixtureGateway(t, doc, false)
	shuffled := doc
	shuffled.Keywords = []string{"测试主机", "检查", "INSPECT", "inspect"}
	b := discoveryFixtureGateway(t, shuffled, true)
	if a.Revision != b.Revision {
		t.Fatal("keyword or registration order changed catalog")
	}
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New()}
	call := toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": "host", "query": "检查", "limit": float64(1)}}
	first, err := a.search(call, "host")
	if err != nil {
		t.Fatal(err)
	}
	if first.Data["items"].([]map[string]any)[0]["name"] != "alpha" {
		t.Fatal("unstable tie ranking")
	}
	cursor := first.Data["next_cursor"]
	call.Arguments["cursor"] = cursor
	second, err := b.search(call, "host")
	if err != nil || second.Data["items"].([]map[string]any)[0]["name"] != "beta" {
		t.Fatalf("unstable cursor: %v", err)
	}
	for _, change := range []func(*mcp.Discovery){func(d *mcp.Discovery) { d.Title += " changed" }, func(d *mcp.Discovery) { d.Description += " changed" }, func(d *mcp.Discovery) { d.Keywords = append(d.Keywords, "新增词") }, func(d *mcp.Discovery) { d.ResultDescription += " changed" }, func(d *mcp.Discovery) { d.Preconditions = []string{"New prerequisite"} }, func(d *mcp.Discovery) { d.Revision = "2" }, func(d *mcp.Discovery) {
		d.Examples = []mcp.DiscoveryExample{{Request: "changed", Guidance: "new guidance"}}
	}} {
		changed := doc
		change(&changed)
		c := discoveryFixtureGateway(t, changed, false)
		if c.Revision == a.Revision || c.manifests[0].Version == a.manifests[0].Version {
			t.Fatal("metadata edit did not invalidate versions")
		}
		if _, err := c.search(call, "host"); err == nil {
			t.Fatal("old cursor accepted by new metadata revision")
		}
		original, _ := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{a}}).Build(t.Context(), p)
		restored, err := (toolruntime.CompositeFactory{Providers: []toolruntime.Provider{c}}).Restore(t.Context(), p, original.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		_, err = restored.Invoke(t.Context(), "tool.search", toolruntime.Invocation{Principal: p, Arguments: map[string]any{"category": "host"}})
		var typed toolruntime.Error
		if !errors.As(err, &typed) || typed.Kind != "TOOL_VERSION_UNAVAILABLE" {
			t.Fatalf("old Run used new discovery: %v", err)
		}
	}
	// Caller mutation cannot alter a frozen catalog or its discovery hash.
	frozen, _ := json.Marshal(a.manifests)
	doc.Keywords[0] = "replaced"
	after, _ := json.Marshal(a.manifests)
	if !reflect.DeepEqual(frozen, after) {
		t.Fatal("caller changed frozen metadata")
	}
}

func TestGatewayRejectsIncompleteBusinessMetadata(t *testing.T) {
	for _, change := range []func(*mcp.Discovery){func(d *mcp.Discovery) { *d = mcp.Discovery{} }, func(d *mcp.Discovery) { d.Description = " " }, func(d *mcp.Discovery) { d.Keywords = nil }, func(d *mcp.Discovery) { d.Examples = nil }, func(d *mcp.Discovery) { d.SchemaVersion = "unknown" }, func(d *mcp.Discovery) { d.Title = strings.Repeat("x", 161) }} {
		d := fixtureDiscovery("invalid")
		change(&d)
		r := mcp.NewRegistry()
		if err := r.Register(mcp.Metadata{ID: "host.invalid", Discovery: d, Visibility: mcp.Visible, MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) { return mcp.Result{}, nil }}); err != nil {
			t.Fatal(err)
		}
		if _, err := New(r, "invalid-doc"); err == nil {
			t.Fatal("incomplete metadata accepted")
		}
	}
}

func TestDiscoveryExactKeywordBeatsIncidentalDescription(t *testing.T) {
	exact := Manifest{Name: "alpha", Discovery: mcp.Discovery{Title: "Other action", Description: "Other business documentation", Keywords: []string{"a needle phrase", "needle"}}}
	incidental := Manifest{Name: "beta", Discovery: mcp.Discovery{Title: "Other action", Description: "needle", Keywords: []string{"other"}}}
	a, matched := discoveryScore(exact, "needle")
	b, _ := discoveryScore(incidental, "needle")
	if !matched || a <= b {
		t.Fatal("authored keyword did not outrank incidental description")
	}
	exact.Keywords = []string{"needle", "a needle phrase"}
	reordered, _ := discoveryScore(exact, "needle")
	if reordered != a {
		t.Fatal("keyword order changed relevance")
	}
	if _, matched := discoveryScore(exact, "needle absent"); matched {
		t.Fatal("partial multi-term query accepted")
	}
}

func TestSearchBoundsSummaryAndDescribeRetainsDocumentation(t *testing.T) {
	doc := fixtureDiscovery("long")
	doc.Description = strings.Repeat("业务说明 ", 300)
	g := discoveryFixtureGateway(t, doc, false)
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: g.Revision}, g.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	call := toolruntime.Invocation{Arguments: map[string]any{"category": "host", "query": "inspect"}}
	result, err := set.Invoke(t.Context(), "tool.search", call)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Data["items"].([]map[string]any) {
		summary := item["summary"].(string)
		if !utf8.ValidString(summary) || utf8.RuneCountInString(summary) > 256 || summary == doc.Description {
			t.Fatal("Search returned unbounded documentation")
		}
		if _, exists := item["input_schema"]; exists {
			t.Fatal("Search exposed full schema")
		}
	}
	call.Arguments = map[string]any{"category": "host", "name": "alpha"}
	result, err = set.Invoke(t.Context(), "tool.describe", call)
	if err != nil {
		t.Fatal(err)
	}
	if result.Data["description"] != doc.Description || result.Data["schema_version"] != "argus.tool_manifest/v1" {
		t.Fatal("Describe lost complete documented contract")
	}
}
