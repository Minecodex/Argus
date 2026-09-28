package configbundle

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type Source struct {
	ID                string   `json:"id"`
	Generation        string   `json:"generation"`
	Key               string   `json:"key"`
	Type              string   `json:"type"`
	Signals           []string `json:"signals"`
	Revision          int64    `json:"revision"`
	CapabilityVersion string   `json:"capability_version"`
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
			if receiver == "otlp/downstream" {
				bound[pipelineName] = pipeline
				continue
			}
			key := target + "/" + receiver
			source, ok := indexed[key]
			if !ok {
				typ, _, _ := strings.Cut(receiver, "/")
				source = &Source{ID: uuid.NewSHA1(generation, []byte(key)).String(), Generation: generation.String(), Key: key, Type: typ, Revision: revision, CapabilityVersion: "v1"}
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
	return bundle.Sources, nil
}
