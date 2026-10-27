package telemetry

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestTypedTraceViewsUsePortalVocabularyAndCurrentSchema(t *testing.T) {
	for _, view := range traceApplicationViews {
		t.Run(view, func(t *testing.T) {
			document, operation, variables, err := traceToolQuery(map[string]any{"application_view": view}, 5000)
			if err != nil || operation != "ApplicationAPM" || !strings.Contains(document, "limit:100") && view != "traces" {
				t.Fatalf("typed application query was not bounded to the portal vocabulary: %v", err)
			}
			if err = skywalking.ValidateToolDocument(document, operation, variables); err != nil {
				t.Fatal("typed view does not match the actual engine schema", err)
			}
		})
	}
	id := uuid.NewString()
	_, _, variables, err := traceToolQuery(map[string]any{"application_view": "services", "service_ids": []any{id}, "limit": float64(10)}, 10)
	if err != nil || len(variables["serviceIds"].([]any)) != 1 || variables["serviceIds"].([]any)[0] != id {
		t.Fatal("typed query lost its service constraint", err)
	}
	for _, test := range []struct {
		view, page string
		budget     int
	}{
		{"services", "limit:99", 100}, {"services", "limit:1", 2},
		{"services", "limit:100", 101}, {"services", "limit:100", 5000},
		{"traces", "pageSize:1", 1},
	} {
		document, _, _, err := traceToolQuery(map[string]any{"application_view": test.view, "limit": float64(test.budget)}, test.budget)
		if err != nil || !strings.Contains(document, test.page) {
			t.Fatalf("APM page did not fit the unchanged execution budget %d: %v", test.budget, err)
		}
	}
}

func TestTraceQuerySchemaHasExclusiveModesAndNoScopeFallback(t *testing.T) {
	schema, err := toolruntime.CompileInputSchema(dslQueryInputSchema("skywalking_graphql", "traces"))
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"application_view": "services", "resource_ids": []any{uuid.NewString()}, "from": "2026-10-21T10:00:00Z", "to": "2026-10-21T10:30:00Z"}
	if err = schema.Validate(input); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]any{
		{"document": "query { queryAPMServices { rows { sampleCount } } }"},
		{"variables": map[string]any{}}, {"operation_name": "Override"},
		{"application_view": "invented"}, {"service_ids": []any{}}, {"resource_ids": []any{}},
	} {
		copy := map[string]any{}
		for key, value := range input {
			copy[key] = value
		}
		for key, value := range bad {
			copy[key] = value
		}
		if err = schema.Validate(copy); err == nil {
			t.Fatal("exclusive mode, empty scope or invalid view was accepted")
		}
	}
	delete(input, "application_view")
	input["document"] = "query { queryTraces(pageSize:10) { total } }"
	if err = schema.Validate(input); err != nil {
		t.Fatal("expert document mode was lost", err)
	}
}

func TestTraceDocumentFailureIsSafeAndActionableBeforeExecution(t *testing.T) {
	const canary = "private-credential-canary"
	for _, input := range []map[string]any{
		{"document": `query { queryBasicTraces(condition:{secret:"` + canary + `"}) { total } }`},
		{"document": `query { unknownRoot }`},
		{"document": `mutation { forbidden }`},
		{"application_view": "services", "document": canary},
		{"application_view": "services", "service_ids": []any{}},
		{"application_view": "services", "limit": float64(1)},
		{"application_view": "services", "bucket_seconds": float64(60)},
	} {
		limit := 100
		if input["limit"] != nil {
			limit = int(input["limit"].(float64))
		}
		_, _, _, err := traceToolQuery(input, limit)
		var feedback toolruntime.Error
		if !errors.As(err, &feedback) || feedback.Code() != "TOOL_INPUT_INVALID" || feedback.Details["schema_validation"] != true || strings.Contains(feedback.Error(), canary) {
			t.Fatal("invalid Trace request did not produce bounded safe schema feedback", err)
		}
	}
	query := "query { queryBasicTraces(serviceName:\"orders\",pageSize:10) { total } }"
	if _, _, _, err := traceToolQuery(map[string]any{"document": query}, 100); err != nil {
		t.Fatal("supported flat-argument trace query was rejected", err)
	}
}
