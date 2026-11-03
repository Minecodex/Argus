package toolgateway

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestCollectionReadContractsDistinguishTargetAndInstanceIdentities(t *testing.T) {
	reg := monitoringDomainRegistry(t, &postgres.Store{})
	get, ok := reg.Lookup("monitoring.collection.get")
	if !ok || get.Discovery.Revision != "3" || get.OutputVersion != "argus.monitoring_result/v2" || !strings.Contains(get.Discovery.Description, "target, Host, Cluster and Collector UUIDs") {
		t.Fatal("collection identity contract is absent")
	}
	id := get.InputSchema["properties"].(map[string]any)["id"].(map[string]any)
	if !strings.Contains(id["description"].(string), "collection.list") || get.Validate(map[string]any{"id": uuid.NewString()}) != nil {
		t.Fatal("identity hint changed or lost the valid collection-id contract")
	}
	if get.Validate(map[string]any{"target_id": uuid.NewString()}) == nil {
		t.Fatal("target identity became an alias for collection identity")
	}
	list, ok := reg.Lookup("monitoring.collection.list")
	if !ok || list.Discovery.Revision != "3" || list.OutputVersion != "argus.monitoring_result/v2" || !strings.Contains(list.Discovery.Description, "not target names or endpoints") || !strings.Contains(list.Discovery.Description, "item.target.kind/id") {
		t.Fatal("directory search or relation semantics are missing")
	}
}
