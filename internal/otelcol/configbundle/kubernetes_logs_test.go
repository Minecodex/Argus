package configbundle

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/logcollection"
)

func TestKubernetesLogsRunOnlyOnNodesWithDurableAcceptance(t *testing.T) {
	ns, workload, collection := uuid.NewString(), uuid.NewString(), uuid.NewString()
	rule, _ := json.Marshal(logcollection.Config{Scope: "component", Workload: &logcollection.Workload{NamespaceID: uuid.New(), Namespace: "orders", NamespaceUID: ns, Type: "deployment", Name: "orders", UID: workload, Containers: []string{"main"}}})
	input := RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), ResourceType: "kubernetes_cluster", Platform: "linux_amd64", Role: "direct", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", ProfileKeys: []string{"k8s-node-container"}}
	baseline, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := Extract(baseline, "kubernetes_agent")
	if strings.Contains(string(base), "/var/log/pods") || strings.Contains(string(base), `"file_storage"`) {
		t.Fatal("fixed node metrics implicitly collect or persist logs")
	}
	input.Collections = []Collection{{ID: collection, Integration: "k8slog", TargetType: "kubernetes_cluster", TargetID: input.ResourceID, NamespaceUID: ns, Config: rule, Revision: 1, AssignmentEpoch: 1, Enabled: true}}
	bundle, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	if !HasKubernetesLogs(bundle) {
		t.Fatal("node log deployment requirement lost")
	}
	gateway, _ := Extract(bundle, "kubernetes_gateway")
	if strings.Contains(string(gateway), "/var/log/pods/") {
		t.Fatal("Gateway reads node files")
	}
	agent, _ := Extract(bundle, "kubernetes_agent")
	var config map[string]any
	if err = json.Unmarshal(agent, &config); err != nil {
		t.Fatal(err)
	}
	pipeline := nestedMap(t, config, "service", "pipelines", "logs/collection_"+strings.ReplaceAll(collection, "-", ""))
	if slices.Contains(stringSlice(t, pipeline, "processors"), "batch") {
		t.Fatal("log checkpoint can acknowledge volatile batch")
	}
	queue := nestedMap(t, config, "exporters", "otlp/gateway", "sending_queue")
	if queue["storage"] != "file_storage" || queue["enabled"] != true {
		t.Fatal("node exporter has no persistent queue")
	}
	suffix := "collection_" + strings.ReplaceAll(collection, "-", "")
	metadata := nestedMap(t, config, "processors", "k8sattributes/"+suffix)
	resyncValue, _ := metadata["watch_sync_period"].(string)
	resync, err := time.ParseDuration(resyncValue)
	if err != nil || resync <= 0 {
		t.Fatal("log owner recovery must explicitly enable native informer resync")
	}
	retry := nestedMap(t, config, "receivers", "filelog/"+suffix, "retry_on_failure")
	budgetValue, _ := retry["max_elapsed_time"].(string)
	budget, err := time.ParseDuration(budgetValue)
	if err != nil || budget != 5*time.Minute || resync*3 >= budget {
		t.Fatal("owner cache recovery has no margin within the bounded log retry budget")
	}
	filter := nestedMap(t, metadata, "filter")
	if filter["namespace"] != "orders" || filter["node_from_env_var"] != "K8S_NODE_NAME" {
		t.Fatal("frequent log owner resync must remain scoped to its node and namespace")
	}
	sources, err := Sources(bundle)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range sources {
		if source.CollectionID == collection {
			found = true
			if source.NamespaceUID != ns || source.ResourceType != "kubernetes_cluster" {
				t.Fatal("Namespace source identity lost")
			}
		}
	}
	if !found {
		t.Fatal("node collection source absent")
	}
}
