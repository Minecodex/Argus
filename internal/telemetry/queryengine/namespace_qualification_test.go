package queryengine

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNamespaceMetricQualificationPreservesCoverageAndBudgetSemantics(t *testing.T) {
	key := uuid.NewString() + ":1"
	for _, row := range []struct {
		language  Language
		keys      []string
		qualified bool
	}{
		{LanguagePromQL, []string{key + "@" + uuid.NewString()}, true},
		{LanguagePromQL, []string{key}, false},
		{LanguagePromQL, nil, false},
		{LanguageKQL, []string{key + "@" + uuid.NewString()}, false},
	} {
		engine := &captureEngineResult{result: Result{Language: row.language, Meta: QueryMeta{Warnings: []string{"existing"}}}}
		c := &Coordinator{PromQL: engine, KQL: engine}
		result, err := c.Execute(t.Context(), Request{Language: row.language, Expression: "sum(metric)", Scope: Scope{EnterpriseID: uuid.New(), SourceKeys: row.keys}, Budget: Budget{Timeout: time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(result.Meta.Warnings, NamespaceMetricQualification) != row.qualified || result.Meta.Partial || !slices.Contains(result.Meta.Warnings, "existing") {
			t.Fatal("coverage qualification changed scope or invented missing data", result.Meta)
		}
	}
}
