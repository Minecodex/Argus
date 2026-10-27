package configbundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
)

// Collection is a frozen desired assignment, including disabled instances.
// A revision is acknowledged only after the entire bundle becomes effective.
type Collection struct {
	Service         *CollectionService `json:"service,omitempty"`
	NamespaceUID    string             `json:"namespace_uid,omitempty"`
	ID              string             `json:"id"`
	Integration     string             `json:"integration"`
	TargetID        string             `json:"target_id"`
	TargetType      string             `json:"target_type"`
	Identity        json.RawMessage    `json:"identity"`
	Config          json.RawMessage    `json:"config"`
	Revision        int64              `json:"revision"`
	AssignmentEpoch int64              `json:"assignment_epoch"`
	Enabled         bool               `json:"enabled"`
	HasCredential   bool               `json:"has_credential"`
}

type CollectionService struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Environment string `json:"environment"`
}

func addCollections(config map[string]any, input RenderInput) error {
	if len(input.Collections) > 256 {
		return errors.New("Collector collection budget exceeded")
	}
	receivers := config["receivers"].(map[string]any)
	processors := config["processors"].(map[string]any)
	pipelines := config["service"].(map[string]any)["pipelines"].(map[string]any)
	seen := map[string]bool{}
	for _, collection := range input.Collections {
		id, err := uuid.Parse(collection.ID)
		target, terr := uuid.Parse(collection.TargetID)
		if err != nil || id == uuid.Nil || terr != nil || target == uuid.Nil || collection.Revision < 1 || collection.AssignmentEpoch < 1 || seen[collection.ID] {
			return errors.New("invalid or duplicate collection assignment")
		}
		seen[collection.ID] = true
		if _, ok := builtincollection.Lookup(collection.Integration); ok {
			if err := addBuiltin(config, input, collection); err != nil {
				return err
			}
			continue
		}
		if logcollection.IsIntegration(collection.Integration) {
			if err := addLogCollection(config, input, collection); err != nil {
				return err
			}
			continue
		}
		if collection.TargetType != "monitoring_target" || collection.Service != nil || collection.NamespaceUID != "" {
			return errors.New("native resource collection identity is invalid")
		}
		entry, exists := monitoringcatalog.Lookup(collection.Integration)
		if !exists || !entry.AdapterReady || !slices.Contains(DistributionComponents(input.Platform), entry.Component) {
			return errors.New("collection component unavailable in this distribution")
		}
		receiver, err := monitoringcatalog.ReceiverConfig(entry.Key, id, collection.Identity, collection.Config, collection.HasCredential)
		if err != nil {
			return fmt.Errorf("collection %s configuration invalid", collection.ID)
		}
		if !collection.Enabled {
			continue
		}
		suffix := "collection_" + strings.ReplaceAll(id.String(), "-", "")
		receiverName := entry.Component + "/" + suffix
		processorName := "resource/" + suffix
		receivers[receiverName] = receiver
		attributes := resourceProcessor(resourceIdentity(input.CollectorID, collection.TargetID, collection.TargetType))["attributes"].([]map[string]any)
		for key, value := range map[string]string{
			"argus.collection.id": collection.ID, "argus.collection.revision": strconv.FormatInt(collection.Revision, 10),
			"argus.assignment.epoch": strconv.FormatInt(collection.AssignmentEpoch, 10), "argus.target.engine": entry.Key,
		} {
			attributes = append(attributes, map[string]any{"key": key, "value": value, "action": "upsert"})
		}
		sort.Slice(attributes, func(i, j int) bool { return attributes[i]["key"].(string) < attributes[j]["key"].(string) })
		processors[processorName] = map[string]any{"attributes": attributes}
		pipelines["metrics/"+suffix] = map[string]any{"receivers": []string{receiverName}, "processors": []string{"memory_limiter", processorName, "batch"}, "exporters": []string{"otlp/argus"}}
	}
	return nil
}

func sourceCollection(receiver string, collections []Collection) *Collection {
	_, instance, _ := strings.Cut(receiver, "/")
	for i := range collections {
		if instance == "collection_"+strings.ReplaceAll(collections[i].ID, "-", "") {
			return &collections[i]
		}
	}
	return nil
}

func RequiresMonitoringCredentials(raw []byte) bool {
	var bundle Bundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.SchemaVersion != SchemaVersion {
		return false
	}
	for _, item := range bundle.Collections {
		if item.Enabled && item.HasCredential {
			return true
		}
	}
	return false
}

// The upstream Collector requires a pipeline even when every business source
// is disabled. Its native nop components keep control extensions alive without
// opening an intake port or manufacturing a telemetry sample.
func addControlOnlyPipeline(config map[string]any) {
	pipelines := config["service"].(map[string]any)["pipelines"].(map[string]any)
	if len(pipelines) != 0 {
		return
	}
	config["receivers"].(map[string]any)["nop/control"] = map[string]any{}
	config["exporters"].(map[string]any)["nop/control"] = map[string]any{}
	pipelines["metrics/control"] = map[string]any{"receivers": []string{"nop/control"}, "processors": []string{}, "exporters": []string{"nop/control"}}
}
