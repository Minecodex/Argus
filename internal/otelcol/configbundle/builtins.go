package configbundle

import (
	"errors"
	"slices"
	"strings"

	"github.com/kakj-go/Argus/internal/builtincollection"
)

func builtinPlacement(collection Collection, target string) bool {
	definition, ok := builtincollection.Lookup(collection.Integration)
	return ok && (definition.Placement == "all" || definition.Placement == target)
}

func addBuiltin(config map[string]any, input RenderInput, collection Collection) error {
	definition, ok := builtincollection.Lookup(collection.Integration)
	if !ok || collection.TargetType != input.ResourceType || collection.TargetID != input.ResourceID || collection.Service != nil || collection.NamespaceUID != "" || collection.HasCredential || !slices.Contains(definition.ResourceTypes, input.ResourceType) || !slices.Contains(definition.Platforms, input.Platform) {
		return errors.New("builtin collection does not belong to this resource")
	}
	value, err := builtincollection.ParseFor(collection.Integration, collection.Config)
	if err != nil {
		return err
	}
	if !collection.Enabled {
		return nil
	}
	receivers := config["receivers"].(map[string]any)
	processors := config["processors"].(map[string]any)
	service := config["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	suffix := "collection_" + strings.ReplaceAll(collection.ID, "-", "")
	name := definition.Component + "/" + suffix
	var receiver map[string]any
	switch definition.Key {
	case "host_metrics":
		receiver = hostReceivers(map[string]bool{"host-basic": true}, input.Platform)["hostmetrics"].(map[string]any)
		receiver["collection_interval"] = value.Interval
	case "host_process_metrics":
		// Never export full argv, executable paths or user names. Process
		// identity and metric visibility remain those of the local Collector.
		process := map[string]any{"resource_attributes": map[string]any{"process.command": map[string]any{"enabled": false}, "process.command_line": map[string]any{"enabled": false}, "process.executable.path": map[string]any{"enabled": false}, "process.owner": map[string]any{"enabled": false}}}
		scrapers := map[string]any{"process": process}
		if input.Platform != "windows_amd64" {
			scrapers["processes"] = map[string]any{}
		}
		receiver = map[string]any{"collection_interval": value.Interval, "scrapers": scrapers}
	case "host_systemd_metrics":
		receiver = map[string]any{"collection_interval": value.Interval, "scope": "system", "units": []string{"*.service"}, "metrics": map[string]any{"systemd.service.restarts": map[string]any{"enabled": true}}}
	case "host_windows_service_metrics":
		receiver = map[string]any{"collection_interval": value.Interval}
	case "kubernetes_node_metrics":
		receiver = map[string]any{"collection_interval": value.Interval, "auth_type": "serviceAccount", "endpoint": "https://${env:K8S_NODE_NAME}:10250", "ca_file": "/var/run/argus-kubelet/pki/kubelet.crt", "insecure_skip_verify": false}
	case "kubernetes_cluster_metrics":
		receiver = map[string]any{"auth_type": "serviceAccount", "collection_interval": value.Interval}
	case "kubernetes_apiserver_metrics":
		receiver = map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{{
			"job_name": "argus-kubernetes-apiserver", "scrape_interval": value.Interval, "scheme": "https", "metrics_path": "/metrics",
			"static_configs":         []map[string]any{{"targets": []string{"kubernetes.default.svc:443"}}},
			"authorization":          map[string]any{"type": "Bearer", "credentials_file": "/var/run/secrets/kubernetes.io/serviceaccount/token"},
			"tls_config":             map[string]any{"ca_file": "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "server_name": "kubernetes.default.svc", "insecure_skip_verify": false},
			"sample_limit":           100000,
			"metric_relabel_configs": []map[string]any{{"source_labels": []string{"__name__"}, "regex": "apiserver_request_total|apiserver_request_duration_seconds_(bucket|sum|count)|apiserver_current_inflight_requests|process_resident_memory_bytes", "action": "keep"}},
		}}}}
	case "kubernetes_coredns_metrics":
		receiver = kubernetesCoreDNSReceiver(value.Interval)
	case "kubernetes_scheduler_metrics", "kubernetes_controller_metrics", "kubernetes_etcd_metrics":
		receiver = kubernetesControlMetricReceiver(definition.Key, value)
	case "collector_metrics":
		receiver = map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{{"job_name": "argus-collector-self", "scrape_interval": value.Interval, "static_configs": []map[string]any{{"targets": []string{"127.0.0.1:8888"}}}}}}}
		telemetry, ok := service["telemetry"].(map[string]any)
		if !ok {
			telemetry = map[string]any{}
			service["telemetry"] = telemetry
		}
		telemetry["metrics"] = map[string]any{"readers": []map[string]any{{"pull": map[string]any{"exporter": map[string]any{"prometheus": map[string]any{"host": "127.0.0.1", "port": 8888}}}}}}
	default:
		return errors.New("unsupported builtin collection")
	}
	receivers[name] = receiver
	identity := resourceIdentity(input.CollectorID, collection.TargetID, collection.TargetType)
	processor := "resource/" + suffix
	processors[processor] = resourceProcessor(identity)
	exporter := "otlp/argus"
	if config["exporters"].(map[string]any)["otlp/gateway"] != nil {
		exporter = "otlp/gateway"
	}
	chain := []string{"memory_limiter"}
	if definition.Key == "kubernetes_cluster_metrics" || definition.Key == "kubernetes_node_metrics" {
		enrich := "k8sattributes/metric_namespace"
		// The pinned native processor supplies Pod identity, not Namespace UID.
		// Ingest binds that immutable object UID through the trusted inventory;
		// a Namespace name alone never grants access to its metric rows.
		metadata := map[string]any{"auth_type": "serviceAccount", "wait_for_metadata": true, "wait_for_metadata_timeout": "20s", "extract": map[string]any{"metadata": []string{"k8s.namespace.name", "k8s.pod.uid"}}, "pod_association": []map[string]any{{"sources": []map[string]string{{"from": "resource_attribute", "name": "k8s.pod.uid"}}}}}
		if definition.Key == "kubernetes_node_metrics" {
			metadata["filter"] = map[string]string{"node_from_env_var": "K8S_NODE_NAME"}
		}
		processors[enrich] = metadata
		chain = append(chain, enrich)
	}
	chain = append(chain, processor, "batch")
	pipelines["metrics/"+suffix] = map[string]any{"receivers": []string{name}, "processors": chain, "exporters": []string{exporter}}
	return nil
}
