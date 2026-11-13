package alerting_test

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"strings"
	"testing"
)

// Public cross-domain discovery checks belong outside the alerting package.
func TestAlertSilenceCategoryIncludesAuthorizedCreation(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (alerting.Tools{Service: alerting.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	hub, err := toolgateway.New(registry, "plan6-discovery-regression")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: hub.Revision}, hub.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"mute silence suppress alert temporary", "create maintenance window downtime", "创建 临时 屏蔽", "temporary silence"} {
		call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.silence.manage"}}, Arguments: map[string]any{"category": "alert"}}
		result, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Data["items"].([]map[string]any) {
			found = found || item["name"] == "silence.create.preview"
			if strings.HasSuffix(item["name"].(string), ".commit") {
				t.Fatal("private commit discovered")
			}
		}
		if !found {
			t.Fatalf("failed-model query %q cannot discover silence creation", query)
		}
		call.Principal.Permissions = []string{"alert.read"}
		result, err = set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range result.Data["items"].([]map[string]any) {
			if item["name"] == "silence.create.preview" {
				t.Fatal("discovery bypassed revoked write permission")
			}
		}
	}
}

func TestCurrentAlertCategoryIncludesEventsAndHealth(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (alerting.Tools{Service: alerting.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	hub, err := toolgateway.New(registry, "plan6-current-alert-discovery")
	if err != nil {
		t.Fatal(err)
	}
	set, err := toolruntime.NewSet(toolruntime.Snapshot{NativeCatalog: hub.Revision}, hub.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"cpu", "cluster alerts list", "cluster CPU", "当前告警 评估健康", "memory no data"} {
		call := toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), Permissions: []string{"alert.read"}}, Arguments: map[string]any{"category": "alert"}}
		result, err := set.Invoke(t.Context(), "tool.search", call)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Data["items"].([]map[string]any) {
			if item["name"] == "event.query" {
				found = true
			}
			if strings.HasSuffix(item["name"].(string), ".commit") {
				t.Fatal("private commit discovered")
			}
		}
		if !found {
			t.Fatalf("current-alert question %q only discovers configuration tools", query)
		}
	}
}

func TestNativeHubAcceptsAlertCatalog(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := (alerting.Tools{Service: alerting.Service{Store: &postgres.Store{}}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidatePairs(); err != nil {
		t.Fatal(err)
	}
	if _, err := toolgateway.New(registry, "plan6-test"); err != nil {
		t.Fatal(err)
	}
}
