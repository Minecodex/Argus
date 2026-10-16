package tooltemplatefixture

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
)

const connectorID = "30000000-0000-4000-8000-000000000003"

func connectorContext(data map[string]any) {
	data["resources"] = []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}, map[string]any{"id": connectorID, "name": "db-prod-connector", "type": "connector"}}
	data["_resource_refs"] = []any{map[string]any{"id": host, "type": "host"}, map[string]any{"id": connectorID, "type": "connector"}}
}

func connectorScenarios() map[string]Fixture {
	scenarios := map[string]Fixture{}
	for _, kind := range []string{"host", "cluster", "unavailable", "unassociated"} {
		data := resourceFixture("connector.get", "connector-detail")
		expected := []string{"db-prod-01"}
		if kind != "host" {
			delete(data, "host_id")
			data["related_resources"] = []any{}
			data["resources"], data["_resource_refs"] = []any{}, []any{}
		}
		switch kind {
		case "cluster":
			data["role"], data["kubernetes_cluster_id"] = "kubernetes", cluster
			data["related_resources"] = []any{map[string]any{"id": cluster, "type": "kubernetes_cluster", "name": "orders-production", "status": "available"}}
			data["resources"] = []any{map[string]any{"id": cluster, "type": "kubernetes_cluster", "name": "orders-production"}}
			data["_resource_refs"] = []any{map[string]any{"id": cluster, "type": "kubernetes_cluster"}}
			expected = []string{"orders-production"}
		case "unavailable":
			data["host_id"] = host
			data["related_resources"] = []any{map[string]any{"id": host, "type": "host", "status": "unavailable"}}
			expected = []string{host.String()}
		case "unassociated":
			expected = []string{"db-prod-connector"}
		}
		scenarios["connector-"+kind] = Fixture{Tool: "connector.get", Data: data, Expected: expected}
	}
	return scenarios
}

func compositeScenarios() map[string]Fixture {
	scenarios := map[string]Fixture{}
	for _, mode := range []string{"every", "any"} {
		data := alertFixture("alert-event-detail", "event")
		raw, _ := json.Marshal(data["instance"])
		var instance alerting.Instance
		_ = json.Unmarshal(raw, &instance)
		spec := policySpec(85)
		spec.Mode = mode
		spec.Rules[1].Name, spec.Rules[1].Expression, spec.Rules[1].Unit = "Memory member", "host_memory_used_mib", "MiB"
		spec.Rules[1].Threshold, spec.Rules[1].RecoveryThreshold = 3072, 2048
		instance.State.Members = []alerting.MemberValue{
			{RuleID: spec.Rules[0].ID, Name: "CPU member", Expression: spec.Rules[0].Expression, Value: 97.25, Threshold: 85, RecoveryThreshold: 75, Unit: "%", Severity: "critical", Firing: true},
			{RuleID: spec.Rules[1].ID, Name: "Memory member", Expression: spec.Rules[1].Expression, Value: 4096.75, Threshold: 3072, RecoveryThreshold: 2048, Unit: "MiB", Severity: "warning", Firing: mode == "every", Recovering: mode == "any"},
		}
		if mode == "any" {
			instance.State.Members[1].Value = 1024.75
		}
		instance.State.RuleID, instance.State.Value = uuid.Nil, &instance.State.Members[0].Value
		instance.State.PolicySnapshot, _ = json.Marshal(spec)
		data["instance"] = object(instance)
		expected := []string{"CPU member", "Memory member", "97.25 %", "MiB", "host_memory_used_mib"}
		scenarios["composite-"+mode] = Fixture{Tool: "alert.event.get", Data: data, Expected: expected}
		for _, view := range []struct{ name, tool string }{{"events", "alert.event.query"}, {"history", "alert.history.query"}} {
			row := instance
			if view.name == "history" {
				row.Lifecycle, row.ResolvedAt = "resolved", &now
			}
			page := map[string]any{"items": []any{object(row)}, "resources": data["resources"]}
			scenarios["composite-"+mode+"-"+view.name] = Fixture{Tool: view.tool, Data: page, Expected: []string{"CPU member", "Memory member"}}
		}
	}
	return scenarios
}
