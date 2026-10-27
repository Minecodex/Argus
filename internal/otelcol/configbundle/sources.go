package configbundle

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
)

type Source struct {
	ServiceID             string   `json:"service_id,omitempty"`
	NamespaceUID          string   `json:"namespace_uid,omitempty"`
	ID                    string   `json:"id"`
	Generation            string   `json:"generation"`
	Key                   string   `json:"key"`
	Type                  string   `json:"type"`
	Signals               []string `json:"signals"`
	Revision              int64    `json:"revision"`
	CapabilityVersion     string   `json:"capability_version"`
	ResourceType          string   `json:"resource_type"`
	ResourceID            string   `json:"resource_id"`
	CollectionID          string   `json:"collection_id,omitempty"`
	CollectionRevision    int64    `json:"collection_revision,omitempty"`
	AssignmentEpoch       int64    `json:"assignment_epoch,omitempty"`
	IngestEndpointID      string   `json:"ingest_endpoint_id,omitempty"`
	IngestEndpointVersion int64    `json:"ingest_endpoint_version,omitempty"`
}

// bindSources stamps origin before batching. A forwarding receiver keeps the
// authenticated downstream origin; it must never relabel it as Gateway OTLP.
func bindSources(config map[string]any, target string, input RenderInput) []Source {
	generation, err := uuid.Parse(input.SourceGeneration)
	if err != nil {
		generation = uuid.NewSHA1(uuid.NameSpaceOID, []byte(input.CollectorID+"/unregistered"))
	}
	revision := max(input.ConfigRevision, 1)
	service := config["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	processors := config["processors"].(map[string]any)
	bound := map[string]any{}
	indexed := map[string]*Source{}
	for pipelineName, raw := range pipelines {
		pipeline := raw.(map[string]any)
		receivers := pipeline["receivers"].([]string)
		for _, receiver := range receivers {
			if receiver == "otlp/downstream" || receiver == "nop/control" || strings.HasSuffix(receiver, "/application") {
				bound[pipelineName] = pipeline
				continue
			}
			key := target + "/" + receiver
			source, ok := indexed[key]
			if !ok {
				typ, _, _ := strings.Cut(receiver, "/")
				source = &Source{ID: uuid.NewSHA1(generation, []byte(key)).String(), Generation: generation.String(), Key: key, Type: typ, Revision: revision, CapabilityVersion: "v1"}
				source.ResourceType, source.ResourceID = input.ResourceType, input.ResourceID
				if collection := sourceCollection(receiver, input.Collections); collection != nil {
					if definition, ok := builtincollection.Lookup(collection.Integration); ok {
						source.Type = definition.SourceType
					}
					source.ResourceType, source.ResourceID = collection.TargetType, collection.TargetID
					source.CollectionID, source.CollectionRevision, source.AssignmentEpoch = collection.ID, collection.Revision, collection.AssignmentEpoch
					if collection.Service != nil {
						source.ServiceID = collection.Service.ID
					}
					source.NamespaceUID = collection.NamespaceUID
				}
				indexed[key] = source
			}
			signal, _, _ := strings.Cut(pipelineName, "/")
			source.Signals = append(source.Signals, signal)
			processorName := "resource/source_" + strings.ReplaceAll(source.ID, "-", "")
			processors[processorName] = map[string]any{"attributes": []map[string]any{
				{"key": "argus.source.id", "value": source.ID, "action": "upsert"},
				{"key": "argus.source.revision", "value": strconv.FormatInt(revision, 10), "action": "upsert"},
			}}
			chain := []string{}
			for _, name := range pipeline["processors"].([]string) {
				if name == "batch" {
					chain = append(chain, processorName)
				}
				chain = append(chain, name)
			}
			if !containsName(chain, processorName) {
				chain = append(chain, processorName)
			}
			name := pipelineName
			if len(receivers) > 1 {
				name = signal + "/" + strings.ReplaceAll(receiver, "/", "_")
			}
			bound[name] = map[string]any{"receivers": []string{receiver}, "processors": chain, "exporters": pipeline["exporters"]}
		}
	}
	service["pipelines"] = bound
	keys := make([]string, 0, len(indexed))
	for key := range indexed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []Source{}
	for _, key := range keys {
		item := indexed[key]
		sort.Strings(item.Signals)
		result = append(result, *item)
	}
	return result
}

func containsName(names []string, name string) bool {
	for _, value := range names {
		if value == name {
			return true
		}
	}
	return false
}

func Sources(value []byte) ([]Source, error) {
	var bundle Bundle
	if err := json.Unmarshal(value, &bundle); err != nil {
		return nil, err
	}
	if bundle.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported Collector bundle schema")
	}
	return bundle.Sources, nil
}
