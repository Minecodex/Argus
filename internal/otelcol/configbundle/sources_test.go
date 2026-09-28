package configbundle

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestReceiverSourceIdentitySurvivesReconfigureButChangesOnReinstall(t *testing.T) {
	input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 1, ResourceType: "host", Role: "direct", RouteKind: "direct_argus", Transport: "direct", ProfileKeys: []string{"host-basic", "otlp-receiver"}, EnrollmentEndpoint: "https://api.example.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test:4318"}
	first, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := Sources(first)
	if err != nil || len(sources) != 2 {
		t.Fatalf("receiver source registration: %+v %v", sources, err)
	}
	config, _ := Extract(first, "host")
	var parsed map[string]any
	_ = json.Unmarshal(config, &parsed)
	pipelines := parsed["service"].(map[string]any)["pipelines"].(map[string]any)
	for _, value := range pipelines {
		pipeline := value.(map[string]any)
		if len(pipeline["receivers"].([]any)) != 1 {
			t.Fatal("different receivers were mixed before source stamping")
		}
	}
	input.ConfigRevision = 2
	next, _ := Render(input)
	nextSources, _ := Sources(next)
	if nextSources[0].ID != sources[0].ID || nextSources[0].Revision != 2 {
		t.Fatal("reconfiguration changed the installation identity")
	}
	input.SourceGeneration = uuid.NewString()
	installed, _ := Render(input)
	installedSources, _ := Sources(installed)
	if installedSources[0].ID == sources[0].ID {
		t.Fatal("reinstallation reused a historic source")
	}
}
