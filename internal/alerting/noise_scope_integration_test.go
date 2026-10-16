package alerting

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresNoiseScopeMustBeAnExplicitSelection(t *testing.T) {
	f := newAlertFixture(t)
	second := uuid.New()
	f.addHost(second, "another-authorized-host")
	f.grant("user", f.a.SubjectID, second)
	f.grant("service_account", f.sa, second)
	input := map[string]any{"enabled": true, "starts_at": time.Now().UTC(), "ends_at": time.Now().UTC().Add(time.Hour), "reason": "deployment", "matchers": []Matcher{{Key: "cluster", Operator: "=", Value: "named-resource"}}}
	for _, kind := range []string{"silence", "inhibition"} {
		t.Run(kind, func(t *testing.T) {
			spec := map[string]any{}
			for key, value := range input {
				spec[key] = value
			}
			if kind == "inhibition" {
				spec = map[string]any{"enabled": true, "source": []Matcher{{Key: "severity", Operator: "=", Value: "critical"}}, "target": []Matcher{{Key: "severity", Operator: "=", Value: "warning"}}, "equal": []string{"resource_id"}, "reason": "dependency"}
			}
			for _, value := range []string{"omitted", "null"} {
				delete(spec, "resource_ids")
				if value == "null" {
					spec["resource_ids"] = nil
				}
				raw, _ := json.Marshal(spec)
				_, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: kind, Operation: "create", Name: "Missing identity", Spec: raw}, uuid.NewString())
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("%s selection silently expanded: %v", value, err)
				}
			}
			for _, ids := range [][]uuid.UUID{{f.host}, {}} {
				spec["resource_ids"] = ids
				raw, _ := json.Marshal(spec)
				pending, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: kind, Operation: "create", Name: "Explicit selection", Spec: raw}, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				var impact struct {
					ResourceIDs []uuid.UUID `json:"resource_ids"`
				}
				if json.Unmarshal(pending.Preview, &impact) != nil {
					t.Fatal("invalid scope facts")
				}
				want := 1
				if len(ids) == 0 {
					want = 2
				}
				if len(impact.ResourceIDs) != want {
					t.Fatalf("explicit selection mismatch: %v", impact.ResourceIDs)
				}
			}
			rows, err := f.s.Configurations(t.Context(), f.a, kind)
			if err != nil || len(rows) != 0 {
				t.Fatal("previews changed business state", rows, err)
			}
		})
	}
}
