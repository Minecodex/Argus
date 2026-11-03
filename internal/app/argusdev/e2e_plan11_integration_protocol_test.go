package argusdev

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"slices"
	"testing"
)

func TestTargetAcceptsExactLegacyInputButRequiresTypedSealedOutput(t *testing.T) {
	for _, test := range []struct {
		body  string
		valid bool
	}{
		{`{"resource_ids":["target"]}`, true},
		{`{"resource_ids":["other"]}`, false},
		{`{"resource_ids":["target","other"]}`, false},
		{`{"scope":{"mode":"explicit","resources":[{"type":"monitoring_target","id":"target"}]}}`, true},
		{`{"scope":{"mode":"explicit","resources":[{"type":"host","id":"target"}]}}`, false},
		{`{"resource_ids":["target"],"scope":null}`, false},
	} {
		var args map[string]any
		if err := json.Unmarshal([]byte(test.body), &args); err != nil {
			t.Fatal(err)
		}
		if plan11ExactTargetArguments(args, "target") != test.valid {
			t.Fatal("target input boundary changed", test.body)
		}
	}
	for _, kind := range []string{"monitoring_target", "host", "application"} {
		evidence := map[string]any{"query_context": map[string]any{"resources": []any{map[string]any{"type": kind, "id": "target"}}}}
		if plan11ExactTargetResult(evidence, "target") != (kind == "monitoring_target") {
			t.Fatal("typed sealed output identity was confused", kind)
		}
	}
	if plan11ExactTargetResult(map[string]any{}, "target") {
		t.Fatal("missing sealed context accepted for target")
	}
}

func TestIntegrationDiscoveryDoesNotAcceptCrossTypeIdentityCollision(t *testing.T) {
	for _, kind := range []string{"monitoring_target", "application", "host", ""} {
		row := map[string]any{"ref": map[string]any{"type": kind, "id": "same-uuid"}}
		if plan11IntegrationHasRef(row, "same-uuid") != (kind == "monitoring_target") {
			t.Fatal("typed discovery identity changed", kind)
		}
		if plan11IntegrationHasRef(row, "another-uuid") {
			t.Fatal("foreign target accepted", kind)
		}
	}
	if plan11IntegrationHasRef(nil, "same-uuid") || plan11IntegrationHasRef(map[string]any{"id": "same-uuid"}, "same-uuid") {
		t.Fatal("untyped or absent reference accepted")
	}
}

func TestTargetPhaseBudgetMatchesCurrentLinuxDedicatedCatalog(t *testing.T) {
	names := plan11TargetModelNames(nil)
	expected := map[string]bool{}
	for _, entry := range monitoringcatalog.All() {
		if entry.AdapterReady && entry.Selection == "candidate" && entry.Family != "ingestion" && slices.Contains(entry.Platforms, "linux_amd64") {
			expected["plan11-target-"+entry.Key+"-zh-1"] = true
		}
	}
	if len(names) != len(expected) || len(names) != 19 {
		t.Fatal("target budget is inconsistent with actual admitted catalog", len(names), len(expected))
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !expected[name] || seen[name] {
			t.Fatal("unknown or duplicate target scenario", name)
		}
		seen[name] = true
	}
	if len(plan11TargetModelNames([]string{"mysql", "http-tcp-tls-dns-icmp"})) != 6 || len(plan8SelectedModelNames("integrations")) != 0 {
		t.Fatal("partial target budget or zero-model data boundary changed")
	}
	if !plan8StageSelected("plan11-targets", "additional-supported-integrations") || plan8StageSelected("plan11-targets", "sdk-application-protocols") {
		t.Fatal("target phase leaked unrelated model/key work")
	}
}

func TestRawTargetUnitsKeepCodesAndBytesAsOriginalValues(t *testing.T) {
	min, max := 200.0, 503.0
	c := plan11Conclusion{Unit: "1", DisplayUnit: "1", Transform: "identity", ValueMin: &min, ValueMax: &max, DisplayMin: &min, DisplayMax: &max}
	if err := plan11MatchRawTargetConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max}); err != nil {
		t.Fatal(err)
	}
	c.Transform = "percent"
	if err := plan11MatchRawTargetConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max}); err == nil {
		t.Fatal("dimensionless status codes became percentages")
	}
	c.Unit, c.DisplayUnit, c.Transform = "By", "By", "identity"
	if err := plan11MatchRawTargetConclusion(c, "By", plan11ValueEvidence{Min: min, Max: max}); err != nil {
		t.Fatal("raw byte statistics were forced into another unit", err)
	}
}

func TestObservedByteUnitAliasesKeepScaleAndUnknownAnnotations(t *testing.T) {
	number := 71.0
	for _, source := range []string{"By", "bytes", "{bytes}"} {
		for _, reported := range []string{"By", "byte", "bytes", "{bytes}"} {
			c := plan11Conclusion{Unit: reported, DisplayUnit: reported, Transform: "identity", ValueMin: &number, ValueMax: &number, DisplayMin: &number, DisplayMax: &number}
			if err := plan11MatchRawTargetConclusion(c, source, plan11ValueEvidence{Min: number, Max: number}); err != nil {
				t.Fatal("equivalent byte spelling rejected", source, reported, err)
			}
		}
	}
	for _, reported := range []string{"GB", "GiB", "1", "{widgets}", "widgets"} {
		c := plan11Conclusion{Unit: reported, DisplayUnit: reported, Transform: "identity", ValueMin: &number, ValueMax: &number, DisplayMin: &number, DisplayMax: &number}
		if plan11MatchRawTargetConclusion(c, "By", plan11ValueEvidence{Min: number, Max: number}) == nil {
			t.Fatal("different dimensions/scales accepted", reported)
		}
	}
	if plan11DisplayUnit("{widgets}") == plan11DisplayUnit("widgets") || plan11DisplayUnit("GB") == plan11DisplayUnit("GiB") {
		t.Fatal("unit normalization guessed unknown annotations or binary scales")
	}
}

func TestTargetFiltersSelectOnlyExplicitIntegrationKeys(t *testing.T) {
	selected, err := plan8ParseIntegrationGroups("memcached,rabbitmq,oracle,dns,icmp")
	if err != nil {
		t.Fatal(err)
	}
	names := plan11TargetModelNames(selected)
	if len(names) != 5 || !slices.Contains(names, "plan11-target-memcached-zh-1") || slices.Contains(names, "plan11-target-apache-zh-1") {
		t.Fatal("single-key retry expanded model budget", names)
	}
	if !plan8IntegrationGroupSelected(selected, "cache-web-coordination") || plan8IntegrationTargetSelected(selected, "cache-web-coordination", "apache") {
		t.Fatal("group preparation widened selected target execution")
	}
	if len(plan11TargetModelNames([]string{"cache-web-coordination"})) != 4 {
		t.Fatal("existing group selector changed")
	}
	if _, err = plan8ParseIntegrationGroups("unknown-target"); err == nil {
		t.Fatal("unknown integration accepted")
	}
}
