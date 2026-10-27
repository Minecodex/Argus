package configbundle

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
)

var logObjectName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)

func kubernetesLogReceiver(config map[string]any, collection Collection, rule logcollection.Config, suffix string) (map[string]any, []string, error) {
	w := rule.Workload
	if w == nil || !logObjectName.MatchString(w.Namespace) || !logObjectName.MatchString(w.Name) || w.NamespaceUID == "" || collection.NamespaceUID != w.NamespaceUID {
		return nil, nil, errors.New("Kubernetes log namespace identity is required")
	}
	if uid, e := uuid.Parse(w.UID); e != nil || uid == uuid.Nil {
		return nil, nil, errors.New("Kubernetes workload UID is invalid")
	}
	pod := w.Name + "-*_*"
	if w.Type == "pod" {
		pod = w.Name + "_" + w.UID
	}
	paths := make([]string, 0, len(w.Containers))
	for _, container := range w.Containers {
		paths = append(paths, "/var/log/pods/"+w.Namespace+"_"+pod+"/"+container+"/*.log")
	}
	operators := []map[string]any{{"type": "container", "id": "container", "add_metadata_from_filepath": true, "max_log_size": rule.MaxLogBytes, "on_error": "drop_quiet"}}
	if rule.MultilineStart != "" {
		operators = append(operators, map[string]any{"type": "recombine", "combine_field": "body", "is_first_entry": "body matches " + strconv.Quote(collectorLiteral(rule.MultilineStart)), "source_identifier": "attributes[\"log.file.path\"]", "force_flush_period": "1s", "max_log_size": rule.MaxLogBytes})
	}
	if rule.Format == "json" {
		operators = append(operators, map[string]any{"type": "json_parser", "parse_from": "body", "parse_to": "attributes", "on_error": "send_quiet"})
	}
	receiver := map[string]any{"include": paths, "start_at": rule.StartAt, "storage": "file_storage", "encoding": rule.Encoding, "include_file_path": true, "include_file_path_resolved": true, "include_file_record_offset": true, "max_log_size": strconv.Itoa(rule.MaxLogBytes), "operators": operators, "retry_on_failure": map[string]any{"enabled": true, "initial_interval": "1s", "max_interval": "10s", "max_elapsed_time": "5m"}}
	metadata := []string{"k8s.namespace.name", "k8s.pod.name", "k8s.pod.uid", "k8s.pod.start_time", "k8s.node.name", "k8s.replicaset.uid", "k8s.replicaset.name", "k8s.deployment.uid", "k8s.deployment.name", "k8s.daemonset.uid", "k8s.daemonset.name", "k8s.statefulset.uid", "k8s.statefulset.name", "k8s.job.uid", "k8s.job.name", "k8s.cronjob.uid", "k8s.cronjob.name"}
	enrich, guard := "k8sattributes/"+suffix, "argus_log_identity/"+suffix
	processors := config["processors"].(map[string]any)
	// ReplicaSet/Job events may arrive after their Pod event when watches recover.
	// The native processor recalculates cached Pod owners on informer resync.
	// Bound that recovery well within the five-minute filelog retry budget; the
	// cache remains scoped to this node and namespace, not the entire cluster.
	processors[enrich] = map[string]any{"auth_type": "serviceAccount", "passthrough": false, "wait_for_metadata": true, "wait_for_metadata_timeout": "30s", "watch_sync_period": "10s", "pod_delete_grace_period": "10m", "filter": map[string]string{"node_from_env_var": "K8S_NODE_NAME", "namespace": w.Namespace}, "extract": map[string]any{"metadata": metadata}, "pod_association": []map[string]any{{"sources": []map[string]string{{"from": "resource_attribute", "name": "k8s.pod.uid"}}}}}
	processors[guard] = map[string]any{"namespace": w.Namespace, "workload_type": w.Type, "workload_uid": w.UID, "node_name": "${env:K8S_NODE_NAME}", "containers": w.Containers}
	config["extensions"].(map[string]any)["file_storage"] = map[string]any{"directory": "/var/lib/argus-otelcol/log-checkpoints", "create_directory": true}
	config["extensions"].(map[string]any)["argus_identity"].(map[string]any)["delivery_state_file"] = "/var/lib/argus-otelcol/log-checkpoints/delivery-state.json"
	service := config["service"].(map[string]any)
	extensions := service["extensions"].([]string)
	if !containsName(extensions, "file_storage") {
		service["extensions"] = append(extensions, "file_storage")
	}
	config["exporters"].(map[string]any)["otlp/gateway"].(map[string]any)["sending_queue"] = map[string]any{"enabled": true, "storage": "file_storage", "queue_size": 4096, "block_on_overflow": true}
	return receiver, []string{enrich, guard}, nil
}

func HasKubernetesLogs(raw []byte) bool {
	var bundle Bundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.SchemaVersion != SchemaVersion {
		return false
	}
	for _, collection := range bundle.Collections {
		if collection.Enabled && collection.Integration == "k8slog" {
			return true
		}
	}
	return false
}
