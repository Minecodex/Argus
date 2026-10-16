package tooltemplatefixture

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func policySpec(threshold float64) alerting.PolicySpec {
	return alerting.PolicySpec{Enabled: true, Resources: alerting.ResourceSelector{Mode: "explicit", IDs: []uuid.UUID{host}}, IntervalSeconds: 30, Rules: []alerting.Rule{
		{ID: uuid.NewSHA1(host, []byte("cpu-rule")), Name: "CPU pressure rule", Expression: "host_cpu_usage_percent", Operator: ">", Threshold: threshold, RecoveryThreshold: 75, Unit: "%", Severity: "critical"},
		{ID: uuid.NewSHA1(host, []byte("memory-rule")), Name: "Memory pressure rule", Expression: "host_memory_usage_percent", Operator: ">", Threshold: 80, RecoveryThreshold: 70, Unit: "%", Severity: "warning"},
	}}
}
func alertFixture(entry, kind string) map[string]any {
	base := map[string]any{"resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}}, "observed_at": now}
	spec := policySpec(85)
	raw, _ := json.Marshal(spec)
	value := 91.
	instance := alerting.Instance{ID: uuid.NewSHA1(host, []byte("event")), PolicyID: uuid.NewSHA1(host, []byte("policy")), PolicyVersion: 3, ResourceIDs: []uuid.UUID{host}, Lifecycle: "firing", Health: "normal", Severity: "critical", StartedAt: &now, LastEvaluatedAt: now, State: alerting.InstanceState{Name: "CPU pressure", PolicyName: "Host resource pressure", RuleID: spec.Rules[0].ID, Metric: "host_cpu_usage_percent", Value: &value, Threshold: 85, RecoveryThreshold: 75, Unit: "%", PolicySnapshot: raw, NotificationState: "failed", NoiseReasons: []string{"silence: maintenance", "inhibition: upstream outage"}}}
	instance.State.LastSuccessfulAt = &now
	if entry == "alert-history" {
		instance.Lifecycle = "resolved"
		instance.ResolvedAt = &now
	}
	if entry == "alert-event-detail" {
		base["instance"] = object(instance)
		base["events"] = []any{object(alerting.Event{ID: uuid.NewSHA1(host, []byte("event-timeline")), InstanceID: instance.ID, EventType: "firing", CreatedAt: now, Payload: json.RawMessage(`{"value":91,"threshold":85}`)})}
		return base
	}
	if entry == "alert-notifications" {
		base["page"] = object(alerting.Page[alerting.Delivery]{Items: []alerting.Delivery{{ID: uuid.NewSHA1(host, []byte("delivery")), ChannelID: uuid.NewSHA1(host, []byte("channel")), ResourceIDs: []uuid.UUID{host}, Status: "failed", Attempt: 2, LastError: "SMTP connection refused", Message: alerting.Message{Subject: "CPU pressure on db-prod-01", Text: "CPU 91% > threshold 85%", EventType: "firing"}, CreatedAt: now}}, Complete: true, Total: 1})
		return base
	}
	if entry == "alert-policy-detail" || entry == "alert-policy-list" || entry == "alert-settings-list" {
		var config any = policySpec(95)
		switch kind {
		case "silence":
			config = alerting.SilenceSpec{Enabled: true, ResourceIDs: []uuid.UUID{host}, StartsAt: now, EndsAt: now.AddDate(0, 0, 1), Reason: "Scheduled maintenance"}
		case "inhibition":
			config = alerting.InhibitionSpec{Enabled: true, ResourceIDs: []uuid.UUID{host}, Reason: "Upstream outage"}
		case "channel":
			config = alerting.ChannelSpec{Enabled: true, Provider: "email", DisplayAddress: "ops@example.test"}
		case "route":
			config = alerting.RouteSpec{Enabled: true, Priority: 1}
		case "settings":
			config = alerting.SettingsSpec{EvaluatorID: host}
		case "grouping":
			config = alerting.GroupingSpec{GroupBy: []string{"policy_id"}, WaitSeconds: 30}
		case "template":
			config = alerting.TemplateSpec{Content: "Alert {{ .Subject }}"}
		}
		raw, _ = json.Marshal(config)
		row := alerting.Configuration{ID: instance.PolicyID, Kind: kind, Name: "Host resource pressure", Version: 4, Spec: raw, Health: "normal"}
		if entry == "alert-policy-detail" {
			base["configuration"] = object(row)
		} else {
			base["items"] = []any{object(row)}
		}
		return base
	}
	base["page"] = object(alerting.Page[alerting.Instance]{Items: []alerting.Instance{instance}, Complete: true, Total: 1, ResourceIDs: []uuid.UUID{host}})
	return base
}
func NormalForTool(tool, entry string) map[string]any {
	if data := resourceFixture(tool, entry); data != nil {
		return data
	}
	if strings.HasPrefix(entry, "alert-") {
		parts := strings.Split(tool, ".")
		return alertFixture(entry, parts[1])
	}
	if entry == "collector-detail" || entry == "collector-list" {
		row := object(db.CollectorInstance{ID: uuid.NewSHA1(host, []byte("collector")), ResourceID: host, ResourceType: "host", Status: "active", Platform: "linux_amd64", Role: "host", DesiredRevision: 7, EffectiveRevision: 6, Version: 3, DistributionVersionID: uuid.NewSHA1(host, []byte("distribution")), LastSeenAt: pgtype.Timestamptz{Time: now, Valid: true}})
		data := map[string]any{"resources": []any{map[string]any{"id": host, "type": "host", "name": "db-prod-01"}}}
		if entry == "collector-detail" {
			data["collector"] = row
		} else {
			data["items"] = []any{row}
		}
		return data
	}
	if entry == "telemetry-catalog" {
		if strings.HasSuffix(tool, ".resources") {
			return map[string]any{"resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host"}}, "source_capabilities": []any{map[string]any{"source_type": "hostmetrics", "signal": "metrics", "capability_version": "v1", "resource_ids": []uuid.UUID{host}, "data_state": "not_sampled"}}}
		}
		return object(telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{{Name: "db_connections", Type: "gauge", Unit: "connections", Labels: []string{"database", "service_name"}}}, Fields: []telemetry.CatalogField{}, Values: []string{}, Complete: true})
	}
	return Normal(entry)
}
func BusinessScenarios() map[string]Fixture {
	fields := object(telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{}, Fields: []telemetry.CatalogField{{Name: "service_name", Type: "string"}}, Values: []string{}, Complete: true})
	values := object(telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{}, Fields: []telemetry.CatalogField{}, Values: []string{"orders-api", "payments-api"}, Membership: map[string]bool{"orders-api": true, "retired-api": false}, HasMore: true, Complete: false})
	scenarios := map[string]Fixture{
		"catalog-fields":    {Tool: "telemetry.dashboard.catalog", Data: fields, Expected: []string{"service_name", "string"}},
		"catalog-values":    {Tool: "telemetry.dashboard.catalog", Data: values, Expected: []string{"orders-api", "payments-api", "retired-api"}},
		"catalog-resources": {Tool: "telemetry.dashboard.catalog.resources", Data: NormalForTool("telemetry.dashboard.catalog.resources", "telemetry-catalog"), Expected: []string{"db-prod-01", "hostmetrics"}},
	}
	for _, group := range []map[string]Fixture{compositeScenarios(), connectorScenarios(), freshnessScenarios(), queryShapeScenarios(), apmCompletenessScenarios(), metricSamplingScenarios()} {
		for name, value := range group {
			scenarios[name] = value
		}
	}
	return scenarios
}
