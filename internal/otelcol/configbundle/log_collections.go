package configbundle

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"sort"
	"strconv"
	"strings"
)

func collectorLiteral(value string) string { return strings.ReplaceAll(value, "$", "$$") }
func collectorLiterals(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, collectorLiteral(value))
	}
	return out
}

func addLogCollection(config map[string]any, input RenderInput, collection Collection) error {
	rule, err := logcollection.Normalize(collection.Integration, input.Platform, collection.Config)
	if err != nil {
		return err
	}
	isKubernetes := collection.Integration == "k8slog"
	if isKubernetes && input.ResourceType != "kubernetes_cluster" || !isKubernetes && input.ResourceType != "host" {
		return errors.New("log rule has an incompatible execution location")
	}
	if collection.HasCredential {
		return errors.New("local log collection does not accept remote credentials")
	}
	if rule.Scope == "business" {
		if collection.TargetType != "application" || collection.Service == nil {
			return errors.New("business logs require a registered service")
		}
		if id, e := uuid.Parse(collection.Service.ID); e != nil || id == uuid.Nil || collection.Service.Name == "" || collection.Service.Environment == "" {
			return errors.New("business log identity is invalid")
		}
	} else if collection.Service != nil || collection.TargetType == "application" {
		return errors.New("infrastructure log identity is invalid")
	}
	if !collection.Enabled {
		return nil
	}
	profiles := make([]string, 0, len(input.ProfileKeys))
	for _, profile := range input.ProfileKeys {
		key, _, _ := strings.Cut(profile, "@")
		profiles = append(profiles, key)
	}
	if conflict, err := logcollection.ConflictsWithLegacy(context.Background(), rule, collection.Integration, input.Platform, profiles); err != nil || conflict {
		return errors.New("log rule overlaps an enabled legacy profile")
	}
	suffix := "collection_" + strings.ReplaceAll(collection.ID, "-", "")
	entry, _ := monitoringcatalog.Lookup(collection.Integration)
	receiverName := entry.Component + "/" + suffix
	receiver := map[string]any{"start_at": rule.StartAt, "storage": "file_storage", "retry_on_failure": map[string]any{"enabled": true, "initial_interval": "1s", "max_interval": "10s", "max_elapsed_time": "5m"}}
	prefix := []string{"memory_limiter"}
	// Stanza's non-quiet parse error path serializes the original entry.
	// Freeze info-level diagnostics and keep malformed records without logging
	// their bodies; Ingest still applies the uniform data redaction policy.
	service := config["service"].(map[string]any)
	telemetry, ok := service["telemetry"].(map[string]any)
	if !ok {
		telemetry = map[string]any{}
		service["telemetry"] = telemetry
	}
	telemetry["logs"] = map[string]any{"level": "info"}
	switch collection.Integration {
	case "filelog":
		receiver["include"] = collectorLiterals(rule.Include)
		receiver["exclude"] = protectedLogExclusions(rule.Exclude, input.Platform)
		receiver["encoding"] = rule.Encoding
		receiver["include_file_path"] = true
		receiver["include_file_path_resolved"] = true
		receiver["include_file_record_offset"] = true
		receiver["max_log_size"] = strconv.Itoa(rule.MaxLogBytes)
		receiver["force_flush_period"] = "1s"
		receiver["operators"] = []map[string]any{protectedLogOperator()}
		if rule.MultilineStart != "" {
			receiver["multiline"] = map[string]any{"line_start_pattern": collectorLiteral(rule.MultilineStart)}
		}
		if rule.Format == "json" {
			receiver["operators"] = jsonFileLogOperators()
		}
	case "journald":
		receiver["units"] = collectorLiterals(rule.Units)
		// The upstream default is info; an unfiltered rule must also keep debug.
		receiver["priority"] = "debug"
	case "eventlog":
		receiver["channel"] = collectorLiteral(rule.Channel)
	case "k8slog":
		var processors []string
		receiver, processors, err = kubernetesLogReceiver(config, collection, rule, suffix)
		if err != nil {
			return err
		}
		prefix = append(prefix, processors...)
	default:
		return errors.New("unsupported log receiver")
	}
	config["receivers"].(map[string]any)[receiverName] = receiver
	attributes := resourceProcessor(resourceIdentity(input.CollectorID, collection.TargetID, collection.TargetType))["attributes"].([]map[string]any)
	values := map[string]string{"argus.collection.id": collection.ID, "argus.collection.revision": strconv.FormatInt(collection.Revision, 10), "argus.assignment.epoch": strconv.FormatInt(collection.AssignmentEpoch, 10)}
	if service := collection.Service; service != nil {
		values["argus.application.id"] = collection.TargetID
		values["argus.service.id"] = service.ID
		values["service.name"] = service.Name
		values["service.namespace"] = service.Namespace
		values["deployment.environment.name"] = service.Environment
	}
	if collection.NamespaceUID != "" {
		values["argus.namespace.uid"] = collection.NamespaceUID
	}
	for key, value := range values {
		attributes = append(attributes, map[string]any{"key": key, "value": collectorLiteral(value), "action": "upsert"})
	}
	sort.Slice(attributes, func(i, j int) bool { return attributes[i]["key"].(string) < attributes[j]["key"].(string) })
	processor := "resource/" + suffix
	config["processors"].(map[string]any)[processor] = map[string]any{"attributes": attributes}
	exporter := "otlp/argus"
	if isKubernetes {
		exporter = "otlp/gateway"
	}
	// Log checkpoints may advance only after durable exporter acceptance. An
	// asynchronous batch processor here would acknowledge volatile log data.
	config["service"].(map[string]any)["pipelines"].(map[string]any)["logs/"+suffix] = map[string]any{"receivers": []string{receiverName}, "processors": append(prefix, processor), "exporters": []string{exporter}}
	return nil
}
