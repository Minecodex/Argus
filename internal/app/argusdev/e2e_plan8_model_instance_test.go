package argusdev

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func TestPlan8DeclaredLogServiceUsesPublicIdentityContract(t *testing.T) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "api", "openapi", "generated", "monitoringapi.bundle.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	application := uuid.New()
	raw, err := json.Marshal(plan8DeclaredServiceBody(application))
	if err != nil {
		t.Fatal(err)
	}
	var wire any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if err = document.Components.Schemas["MonitoringMutation"].Value.VisitJSON(wire); err != nil {
		t.Fatal(err)
	}
	var mutation monitoring.Mutation
	if err = json.Unmarshal(raw, &mutation); err != nil {
		t.Fatal(err)
	}
	if mutation.Service == nil || mutation.Service.ApplicationID != application || mutation.Service.Namespace != "acceptance" || mutation.Service.Name != "file-api" {
		t.Fatal("public service identity was lost")
	}
	service := wire.(map[string]any)["service"].(map[string]any)
	service["name"] = service["service_name"]
	delete(service, "service_name")
	if err = document.Components.Schemas["MonitoringMutation"].Value.VisitJSON(wire); err == nil {
		t.Fatal("legacy service name field admitted")
	}
}

func TestPlan8ModelConfirmationAllowsOnlySingleInstanceDisable(t *testing.T) {
	app, service := uuid.New(), uuid.New()
	before := monitoring.Collection{ID: uuid.New(), Name: "Owned MySQL", Integration: "mysql", Target: monitoring.ResourceRef{Kind: "monitoring_target", ID: uuid.New()}, CollectorID: uuid.New(), ApplicationID: &app, ServiceID: &service, Config: json.RawMessage(`{"timeout":"5s","interval":"5s"}`), Version: 4, DesiredEnabled: true}
	for _, operation := range []string{"set_enabled", "update"} {
		for _, change := range []string{"none", "id", "version", "type", "enabled", "name", "target", "collector", "app", "service", "config", "credential", "bindings", "private_draft"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				preview := plan8ServiceUpdateBody(before, before.CollectorID, false)
				preview["operation"] = operation
				in := preview["collection"].(map[string]any)
				in["config"] = json.RawMessage(`{"interval":"5s","timeout":"5s"}`)
				id, version, actionType := before.ID, before.Version, "monitoring.collection."+operation
				switch change {
				case "id":
					id = uuid.New()
				case "version":
					version++
				case "type":
					actionType = "monitoring.collection.archive"
				case "enabled":
					in["enabled"] = true
				case "name":
					in["name"] = "Different"
				case "target":
					in["target"] = monitoring.ResourceRef{Kind: "monitoring_target", ID: uuid.New()}
				case "collector":
					in["collector_id"] = uuid.New()
				case "app":
					in["application_id"] = uuid.New()
				case "service":
					in["service_id"] = uuid.New()
				case "config":
					in["config"] = json.RawMessage(`{"interval":"60s"}`)
				case "credential":
					in["credential_update"] = "remove"
				case "bindings":
					in["bindings"] = []monitoring.BindingInput{{ApplicationID: uuid.New(), Target: before.Target}}
				case "private_draft":
					in["credential_draft_id"] = uuid.New()
				}
				err := plan8ValidateModelDisable(actionType, id, version, preview, before)
				if (err == nil) != (change == "none") {
					t.Fatalf("unexpected approval outcome: %v", err)
				}
			})
		}
	}
}

func TestPlan8MonitoringDiscoveryCannotCountAlertCalls(t *testing.T) {
	other := `[{"tool":"tool.search","status":"succeeded","input":{"category":"alert"}},{"tool":"tool.describe","status":"succeeded","input":{"category":"alert","name":"rule.list"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"alert","name":"rule.list"}}]`
	if verifyTypedToolDiscoveryOrder(other, "monitoring") == nil {
		t.Fatal("other category counted as monitoring discovery")
	}
	actual := `[{"tool":"tool.search","status":"succeeded","input":{"category":"monitoring"}},{"tool":"tool.describe","status":"succeeded","input":{"category":"monitoring","name":"collection.change.preview"}},{"tool":"tool.invoke","status":"succeeded","input":{"category":"monitoring","name":"collection.change.preview"}}]`
	if err := verifyTypedToolDiscoveryOrder(actual, "monitoring"); err != nil {
		t.Fatal(err)
	}
}
