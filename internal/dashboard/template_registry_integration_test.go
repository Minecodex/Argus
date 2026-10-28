package dashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestBuiltinTemplatesCompileNativeMetrics(t *testing.T) {
	for _, definition := range BuiltinTemplates() {
		t.Run(definition.Key, func(t *testing.T) {
			if _, hash, err := validateTemplate(definition); err != nil || len(hash) != 64 {
				t.Fatalf("template validation: %v", err)
			}
			for _, panel := range definition.Spec.Panels {
				minimum := 256
				if definition.Key == "argus.model-governance" && panel.Type == "stat" {
					minimum = 128
				}
				if panel.Layout.H*definition.Spec.Layout.RowHeight < minimum {
					t.Fatal("native panel clips chart and query controls")
				}
				if panel.Unit == "short" || panel.Unit == "ops" || panel.Unit == "reqps" {
					t.Fatal("Grafana reference unit leaked into rendered units")
				}
			}
		})
	}
	definition := BuiltinTemplates()[0]
	definition.Key = "test.mysql." + uuid.NewString()
	definition.IntegrationKeys = []string{"clickhouse"}
	if _, _, err := validateTemplate(definition); err == nil {
		t.Fatal("deferred resource received a default template")
	}
}

func TestPostgresManagedReadsUpgradeAndCopiesKeepTheirDefinition(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	service := Service{Store: f.store}
	definition := BuiltinTemplates()[0]
	definition.Key = "test.copy." + uuid.NewString()
	if err := service.SyncTemplates(ctx, []TemplateDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	managed, err := service.ApplyManagedTemplate(ctx, f.actor.EnterpriseID, definition.Key)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.Get(ctx, f.actor, managed.ID); err != nil {
		t.Fatalf("system definition not readable: %v", err)
	}
	draft, err := service.CreateDraft(ctx, f.actor, DraftInput{DashboardID: managed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if draft.DashboardID.Valid || draft.BaseRevisionID.Valid || draft.BaseObjectVersion != 0 {
		t.Fatal("edit default did not create an independent personal draft")
	}
	savedSpec := append([]byte{}, draft.Spec...)
	stored, err := service.Draft(ctx, f.actor, draft.ID)
	if err != nil || stored.SourceTemplateKey.String != definition.Key || stored.SourceTemplateVersion.Int64 != 1 {
		t.Fatal("copy origin not frozen")
	}
	updated := definition
	updated.Version = 2
	updated.Spec = EmptySpec()
	updated.Spec.Panels = definition.Spec.Panels[:1]
	if err = service.SyncTemplates(ctx, []TemplateDefinition{updated}); err != nil {
		t.Fatal(err)
	}
	latest, _, err := service.Get(ctx, f.actor, managed.ID)
	if err != nil || latest.ActiveRevisionID == managed.ActiveRevisionID {
		t.Fatal("managed read did not see latest template")
	}
	stored, err = service.Draft(ctx, f.actor, draft.ID)
	if err != nil || !bytes.Equal(savedSpec, stored.Spec) || stored.SourceTemplateVersion.Int64 != 1 {
		t.Fatal("template upgrade modified a user copy")
	}
	if _, err = service.PreviewLifecycle(ctx, f.actor, LifecycleInput{Operation: "archive", ID: managed.ID, ExpectedVersion: latest.Version}, uuid.NewString()); !errors.Is(err, ErrDenied) {
		t.Fatalf("user archived a system definition: %v", err)
	}
}

func TestPostgresManagedTemplateVersionMonotonicityAndReadOnly(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	service := Service{Store: f.store}
	definition := BuiltinTemplates()[0]
	definition.Key = "test.version." + uuid.NewString()
	if err := service.SyncTemplates(ctx, []TemplateDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	first, err := service.ApplyManagedTemplate(ctx, f.actor.EnterpriseID, definition.Key)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{EnterpriseID: f.actor.EnterpriseID, ID: first.ActiveRevisionID.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before.SampleReport, []byte(`"status":"passed"`)) {
		t.Fatal("template installation fabricated samples")
	}
	if _, err = f.store.Pool.Exec(ctx, "UPDATE dashboards SET name='user edit' WHERE enterprise_id=$1 AND id=$2", f.actor.EnterpriseID, first.ID); err == nil {
		t.Fatal("managed dashboard mutable through regular writes")
	}
	if _, err = f.store.Pool.Exec(ctx, "UPDATE dashboard_template_versions SET name='rewrite' WHERE template_key=$1 AND version=1", definition.Key); err == nil {
		t.Fatal("immutable template version rewritten")
	}
	conflicting := definition
	conflicting.Name = "Another definition"
	if err = service.SyncTemplates(ctx, []TemplateDefinition{conflicting}); !errors.Is(err, ErrConflict) {
		t.Fatalf("version identity conflict: %v", err)
	}
	updated := definition
	updated.Version = 2
	updated.Spec = EmptySpec()
	updated.Spec.Panels = definition.Spec.Panels[:1]
	if err = service.SyncTemplates(ctx, []TemplateDefinition{updated}); err != nil {
		t.Fatal(err)
	}
	second, err := service.ApplyManagedTemplate(ctx, f.actor.EnterpriseID, definition.Key)
	if err != nil || second.ID != first.ID || second.ActiveRevisionID == first.ActiveRevisionID {
		t.Fatalf("managed upgrade: %+v %v", second, err)
	}
	old, err := f.store.Queries.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{EnterpriseID: f.actor.EnterpriseID, ID: before.ID})
	if err != nil || !bytes.Equal(old.Spec, before.Spec) {
		t.Fatal("running query's old revision was overwritten")
	}
	if err = service.SyncTemplates(ctx, []TemplateDefinition{definition}); err != nil {
		t.Fatal(err)
	}
	head, err := f.store.Queries.GetDashboardTemplateHead(ctx, definition.Key)
	if err != nil || head.Version != 2 {
		t.Fatal("old server downgraded template head")
	}
	third, err := service.ApplyManagedTemplate(ctx, f.actor.EnterpriseID, definition.Key)
	if err != nil || third.ActiveRevisionID != second.ActiveRevisionID {
		t.Fatal("same template version created another revision")
	}
	var count int
	if err = f.store.Pool.QueryRow(ctx, "SELECT count(*) FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_type IN ('host','kubernetes_cluster','application','monitoring_target')", f.actor.EnterpriseID).Scan(&count); err != nil || count != 0 {
		t.Fatal("default dashboard granted infrastructure access")
	}
	var content map[string]any
	if json.Unmarshal(old.Spec, &content) != nil {
		t.Fatal("old revision is no longer a standalone definition")
	}
}
