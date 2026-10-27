package tooltemplatefixture

import (
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
)

// Controlled template fixtures use the production public DTOs, never runtime
// telemetry. The browser fixture server is explicitly separate from real mode.
func monitoringFixture(tool string) map[string]any {
	app, target, collection := uuid.NewSHA1(host, []byte("app")), uuid.NewSHA1(host, []byte("target")), uuid.NewSHA1(host, []byte("collection"))
	application := object(monitoring.Application{ID: app, Key: "orders-production", Name: "orders-production", Environment: "production", Status: "active", Version: 2, CreatedAt: now, UpdatedAt: now})
	page := func(row map[string]any) map[string]any { return map[string]any{"items": []any{row}} }
	if strings.HasSuffix(tool, ".preview") && (strings.HasPrefix(tool, "application.") || strings.HasPrefix(tool, "monitoring.") || tool == "kubernetes.object.change.preview") {
		return Normal("change-preview")
	}
	switch tool {
	case "monitoring.collection.form":
		return map[string]any{"schema_version": "argus.monitoring_host_form/v1", "status": "requires_user_input", "integration": "mysql", "prefill": map[string]string{"name": "orders-db", "endpoint": "mysql.example.test:3306"}}
	case "kubernetes.drain.cancel.preview":
		value := Normal("change-preview")
		value["action_type"], value["title"] = "kubernetes.object.cancel_drain", "Cancel node drain"
		value["preview"] = map[string]any{"operation_id": collection, "cluster_id": host, "node_stays_cordoned": true, "in_flight_batch_may_finish": true}
		return value
	case "kubernetes.helm.list", "kubernetes.helm.history":
		return page(object(kuberneteshelm.Release{Name: "orders-api", Namespace: "orders", Revision: 2, Chart: "orders", ChartVersion: "1.0.0", Status: "deployed", UpdatedAt: now}))
	case "kubernetes.helm.change.preview":
		value := Normal("change-preview")
		value["action_type"], value["title"] = "kubernetes.helm.rollback", "Rollback Helm orders-api"
		value["preview"] = map[string]any{"name": "orders-api", "namespace": "orders", "operation": "rollback", "expected_revision": 2, "rollback_revision": 1, "keep_history": true, "objects": []any{map[string]any{"name": "orders-settings", "namespace": "orders", "action": "apply", "validation": "server", "definition": map[string]any{"kind": "ConfigMap"}}}}
		return value
	case "application.list":
		return page(application)
	case "application.get":
		return application
	case "application.service.list":
		return page(object(monitoring.ApplicationService{ID: uuid.NewSHA1(app, []byte("api")), ApplicationID: app, Namespace: "payments", Name: "orders-api", Status: "active", Version: 1}))
	case "application.endpoint.list":
		return page(map[string]any{"id": uuid.NewSHA1(app, []byte("endpoint")), "name": "orders-otlp", "application_id": app, "protocol": "otlp", "signals": []string{"traces", "logs", "metrics"}, "enabled": true})
	case "application.endpoint.connection":
		return object(monitoring.IngestConnection{
			Name: "orders-otlp", EndpointID: uuid.NewSHA1(app, []byte("endpoint")), ApplicationID: app,
			CollectorID: collection, Environment: "production", Protocol: "otlp",
			Signals: []string{"traces"}, DesiredEnabled: true,
			ConfigurationStatus: "pending", CollectorStatus: "installing", ApplicationStatus: "active",
			ObservationState: "waiting", AuthenticationHeader: "authorization", AuthenticationScheme: "Bearer",
			ServiceNameRequired: true, NetworkPrerequisite: "resolve_collector_dns_or_use_local_address_with_tls_server_name",
			Listeners: []monitoring.IngestListener{{Transport: "grpc", Port: 24317, Endpoint: "grpcs://collector.example.test:24317", LocalEndpoint: "grpcs://127.0.0.1:24317"}},
			TLS:       monitoring.IngestTLS{Required: true, MinimumVersion: "1.3", ServerName: "collector.example.test", TrustBundleEpoch: 1, CABundlePEM: "PUBLIC_CA_TEMPLATE_FIXTURE", CABundleSHA256: strings.Repeat("0", 64), CAFingerprints: []string{strings.Repeat("0", 64)}},
		})
	case "application.key.list":
		return page(map[string]any{"id": uuid.NewSHA1(app, []byte("key")), "name": "SDK Key metadata", "endpoint_id": uuid.NewSHA1(app, []byte("endpoint")), "version": 1, "expires_at": now.AddDate(0, 1, 0)})
	case "monitoring.integration.list":
		entry, _ := monitoringcatalog.Lookup("mysql")
		return page(object(entry))
	case "monitoring.builtin.list":
		definition, _ := builtincollection.Lookup("host_metrics")
		return page(object(monitoring.Builtin{Definition: definition, ID: collection, Available: true}))
	case "monitoring.target.list":
		return page(object(monitoring.Target{ID: target, Name: "mysql-production", Kind: "database", Engine: "mysql", Status: "active", Version: 1}))
	case "monitoring.collector.choices":
		return page(object(monitoring.CollectorChoice{ID: collection, Name: "db-prod-01", Resource: monitoring.ResourceRef{Kind: "host", ID: host}, Platform: "linux_amd64", Status: "converged", CollectorVersion: monitoringcatalog.CollectorVersion}))
	case "monitoring.collection.list", "monitoring.collection.get":
		row := object(monitoring.Collection{ID: collection, Name: "mysql-production", Integration: "mysql", Target: monitoring.ResourceRef{Kind: "monitoring_target", ID: target}, DesiredEnabled: true, DesiredRevision: 3, EffectiveRevision: 2, ApplyStatus: "applying", Health: "unknown", Freshness: "no_data", Status: "active", Version: 3})
		if strings.HasSuffix(tool, ".get") {
			return row
		}
		return page(row)
	case "kubernetes.workbench.types":
		definition, _ := kubernetesinventory.Lookup("deployment")
		return map[string]any{"items": []any{object(definition)}}
	case "kubernetes.object.list", "kubernetes.object.get":
		page := resourceFixture(tool, "kubernetes-resource-list")
		if strings.HasSuffix(tool, ".get") {
			return page["items"].([]any)[0].(map[string]any)
		}
		return page
	case "kubernetes.operation.list", "kubernetes.operation.get":
		row := object(kubernetesworkbench.Operation{ID: collection, ClusterID: host, Type: "deployment", Name: "orders-api", Status: "applied"})
		if strings.HasSuffix(tool, ".get") {
			return row
		}
		return page(row)
	}
	return nil
}
