package dashboard

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resource"
)

func TestPostgresScopeDirectorySearchPaginationAndCursorBinding(t *testing.T) {
	f := newAnalysisFixture(t)
	ctx := t.Context()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i, env := range []string{"production", "testing", "development"} {
		id := uuid.New()
		name := "Orders"
		if i == 2 {
			name = "Hidden billing"
		}
		execute("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,$4,$5,$6)", id, f.actor.EnterpriseID, id.String(), name, env, f.actor.SubjectID)
		if i < 2 {
			execute("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, id)
		}
	}
	service := Service{Store: f.store, Actions: resource.PendingActionService{Key: bytes.Repeat([]byte{1}, 32)}}
	input := ScopeDirectoryInput{Kind: "application", Search: "Orders", Limit: 1}
	first, err := service.ScopeDirectory(ctx, f.actor, input)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	input.Cursor = first.NextCursor
	second, err := service.ScopeDirectory(ctx, f.actor, input)
	if err != nil || len(second.Items) != 1 || first.Items[0].ID == second.Items[0].ID || first.Items[0].Environment == second.Items[0].Environment || second.NextCursor != "" {
		t.Fatalf("second page: %+v %v", second, err)
	}
	input.Search = "Hidden"
	if _, err = service.ScopeDirectory(ctx, f.actor, input); err == nil {
		t.Fatal("search reused another cursor")
	}
	input.Search = "Orders"
	input.Kind = "host"
	if _, err = service.ScopeDirectory(ctx, f.actor, input); err == nil {
		t.Fatal("resource kind reused another cursor")
	}
	input.Kind = "application"
	input.Cursor = ""
	input.Search = "Hidden"
	page, err := service.ScopeDirectory(ctx, f.actor, input)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ungranted application exposed: %+v %v", page, err)
	}
}
