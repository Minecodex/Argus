package tooltemplatefixture

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func TestBusinessFixturesRoundTripThroughFormalTypes(t *testing.T) {
	collector := NormalForTool("telemetry.collector.get", "collector-detail")["collector"]
	raw, _ := json.Marshal(collector)
	var c db.CollectorInstance
	if err := json.Unmarshal(raw, &c); err != nil || c.DesiredRevision != 7 || c.EffectiveRevision != 6 || !c.LastSeenAt.Valid {
		t.Fatal(c, err)
	}
	row := NormalForTool("alert.policy.get", "alert-policy-detail")["configuration"]
	raw, _ = json.Marshal(row)
	var config alerting.Configuration
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	var spec alerting.PolicySpec
	if err := json.Unmarshal(config.Spec, &spec); err != nil || len(spec.Rules) != 2 || spec.Rules[0].Expression != "host_cpu_usage_percent" || spec.Rules[0].Threshold != 95 {
		t.Fatal(spec, err)
	}
	for name, fixture := range BusinessScenarios() {
		if fixture.Tool != "telemetry.dashboard.catalog" {
			continue
		}
		raw, _ = json.Marshal(fixture.Data)
		var result telemetry.DataCatalogResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(name, err)
		}
		if len(result.Fields)+len(result.Values) == 0 {
			t.Fatal("scenario has no business rows", name)
		}
	}
}
func TestBoundaryFixturesMutateNestedBusinessFields(t *testing.T) {
	catalog, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range catalog.Bindings {
		if binding.Mode != "template" {
			continue
		}
		variants := Variants(binding.Entry, binding.Data)
		for _, name := range []string{"id-only", "missing-name", "long-name", "special-characters"} {
			seen := 0
			transform(variants[name], func(key string, value any) (any, bool) {
				if displayField(key) {
					if text, ok := value.(string); ok {
						seen++
						if name == "long-name" && !strings.Contains(text, LongMarker) {
							t.Fatalf("%s %s kept nested field %s", binding.Tool, name, key)
						}
						if name == "special-characters" && text != SpecialMarker {
							t.Fatal(binding.Tool, key)
						}
						if (name == "missing-name" || name == "id-only") && text != "" {
							t.Fatal(binding.Tool, name, key)
						}
					}
				}
				return nil, false
			})
			if seen == 0 && name == "long-name" {
				t.Fatal("variant did not exercise a displayed field", binding.Tool)
			}
		}
		if reflect.DeepEqual(variants["normal"], variants["empty"]) {
			t.Fatal("empty is normal", binding.Tool)
		}
		if len(binding.Expected) == 0 {
			t.Fatal("missing content contract", binding.Tool)
		}
	}
	if catalog.Cases["alert-event-detail"]["empty"]["instance"] != nil {
		t.Fatal("empty alert retained instance")
	}
}

func TestCompositeFixturesCarryFormalObservationsAndFrozenModes(t *testing.T) {
	for name, fixture := range compositeScenarios() {
		if fixture.Tool != "alert.event.get" {
			continue
		}
		raw, _ := json.Marshal(fixture.Data["instance"])
		var instance alerting.Instance
		if err := json.Unmarshal(raw, &instance); err != nil || len(instance.State.Members) != 2 || instance.State.RuleID.String() != "00000000-0000-0000-0000-000000000000" {
			t.Fatal("invalid composite fixture", name, err)
		}
		var snapshot alerting.PolicySpec
		if err := json.Unmarshal(instance.State.PolicySnapshot, &snapshot); err != nil || !strings.HasSuffix(name, snapshot.Mode) || len(snapshot.Rules) != 2 || instance.State.Members[1].Unit != "MiB" {
			t.Fatal("composite snapshot or units drifted", name, err)
		}
	}
}
