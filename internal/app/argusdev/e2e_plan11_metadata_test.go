package argusdev

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestMetadataSemanticsPhaseDeclaresOnlyItsSDKDataAndTwelveTasks(t *testing.T) {
	if len(plan8SelectedModelNames("plan11-metadata")) != 12 || plan8SDKModelKeySelected("plan11-metadata", "otlp_http") {
		t.Fatal("metadata task declaration silently dispatches a Key-delivery model task")
	}
	for _, stage := range []string{"formal-onboarding-and-managed-collector", "actual-query-and-dashboard", "normal-production-backend", "monitoring-api", "sdk-application-protocols", "plan11-metadata-semantics"} {
		if !plan8StageSelected("plan11-metadata", stage) {
			t.Fatal("metadata semantics lost its actual SDK/production prerequisite", stage)
		}
	}
	for _, stage := range []string{"plan11-resource-identity", "plan11-customer-mcp", "plan11-chat-interactions", "real-model-application-revocation", "plan11-profile-comparison"} {
		if plan8StageSelected("plan11-metadata", stage) {
			t.Fatal("metadata semantics gained an unrelated paid model scenario", stage)
		}
	}
	for _, phase := range []string{"all", "plan11", "plan11-identity", "plan11-mcp"} {
		if plan8StageSelected(phase, "plan11-metadata-semantics") {
			t.Fatal("existing phase acquired new metadata tasks", phase)
		}
	}
	for _, locale := range []string{"zh", "en"} {
		if !strings.Contains(plan11SDKCoveragePrompt(locale, "actual-app-id"), "actual-app-id") || !strings.Contains(plan11CollectionActivityPrompt(locale, "actual-collection-id"), "actual-collection-id") {
			t.Fatal("business prompts lost the actual explicitly requested identity")
		}
	}
}

func TestMetadataSemanticsTraceAllowsEndpointMetadataWithoutRawKeyOrQueries(t *testing.T) {
	if err := plan11MetadataTraceAllowed(`[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"endpoint.list"}},{"Tool":"tool.invoke","Input":{"Category":"application","Name":"endpoint.connection"}},{"Tool":"tool.invoke","Input":{"Category":"application","Name":"key.list"}}]`); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`[{"Tool":"connect"}]`, `[{"Tool":"tool.invoke","Input":{"Category":"metric","Name":"query"}}]`, `[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"key.reveal"}}]`, `[{"Tool":"tool.invoke","Input":{"Category":"application","Name":"change.preview"}}]`} {
		if !errors.Is(plan11MetadataTraceAllowed(raw), errPlan11NavigationBoundary) {
			t.Fatal("metadata-only task crossed its read/public/no-query boundary", raw)
		}
	}
}

func TestPostgresMetadataSemanticsEvidenceQueriesUseCurrentSchema(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, query := range []string{plan11SDKEndpointReadSQL(uuid.NewString(), uuid.NewString(), uuid.NewString()), plan11CollectionActivityReadSQL(uuid.NewString(), uuid.New())} {
		var count int
		if err := store.Pool.QueryRow(t.Context(), query).Scan(&count); err != nil || count != 0 {
			t.Fatal("metadata evidence query failed or matched a different Run", err, count)
		}
	}
}
