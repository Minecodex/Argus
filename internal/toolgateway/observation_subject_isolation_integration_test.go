package toolgateway

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
)

// Actual PostgreSQL identities and department grants share one server-side
// directory. A valid cursor is not authority to read another subject's data.
func TestPostgresObservationDirectorySeparatesEnterpriseUserAndDepartment(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_PLAN11_TEST_DATABASE_URL", "observe_subjects", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, foreignEnterprise := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{enterprise, foreignEnterprise} {
		exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'Isolation','UTC')", id)
	}
	exec("INSERT INTO permissions(id,description,registry_version) VALUES('host.read','Read Host',$1) ON CONFLICT DO NOTHING", authorization.PermissionRegistryVersion)
	actors := []authorization.Actor{}
	departments, hosts := []uuid.UUID{}, [][]uuid.UUID{}
	for i := 0; i < 3; i++ {
		owner := enterprise
		if i == 2 {
			owner = foreignEnterprise
		}
		department, user, role := uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,$3)", department, owner, department.String())
		exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Reader')", user, owner, department, user.String())
		exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,$3)", role, owner, role.String())
		exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'host.read')", role)
		exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), owner, user, role)
		actors = append(actors, authorization.Actor{EnterpriseID: owner, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1})
		departments = append(departments, department)
		owned := []uuid.UUID{uuid.New(), uuid.New()}
		for _, host := range owned {
			exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,$3,'127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, owner, "Isolation "+host.String())
			exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'department',$3,'host',$4)", uuid.New(), owner, department, host)
		}
		hosts = append(hosts, owned)
	}
	resolver := observationscope.Resolver{Queries: store.Queries}
	key := bytes.Repeat([]byte{19}, 32)
	input := observationscope.DiscoveryInput{ResourceTypes: []string{"host"}, Limit: 1}
	for i, actor := range actors {
		first, err := resolver.Discover(ctx, actor, key, input)
		if err != nil || len(first.Items) != 1 || first.Complete || first.NextCursor == "" || !slices.Contains(hosts[i], first.Items[0].ID) {
			t.Fatalf("subject %d first page: %+v %v", i, first, err)
		}
		next := input
		next.Cursor = first.NextCursor
		second, err := resolver.Discover(ctx, actor, key, next)
		if err != nil || len(second.Items) != 1 || !second.Complete || !slices.Contains(hosts[i], second.Items[0].ID) || first.Items[0].ID == second.Items[0].ID {
			t.Fatalf("subject %d second page: %+v %v", i, second, err)
		}
		for j, other := range actors {
			if i == j {
				continue
			}
			if page, err := resolver.Discover(ctx, other, key, next); err == nil || len(page.Items) != 0 {
				t.Fatalf("subject %d cursor reused by %d: %+v %v", i, j, page, err)
			}
			selection := observationscope.Selection{Mode: observationscope.Explicit, Types: []string{"host"}, IDs: []uuid.UUID{hosts[i][0]}}
			if snapshot, err := resolver.Directory(ctx, other, selection); err == nil || len(snapshot.Resources) != 0 {
				t.Fatalf("subject %d host read by %d: %+v %v", i, j, snapshot.Resources, err)
			}
		}
	}
	// Simulate a membership move without an authorization-version bump to prove
	// the current department is consulted; the cursor still cannot grant rows.
	exec("UPDATE enterprise_users SET department_id=$1 WHERE id=$2", departments[1], actors[0].SubjectID)
	page, err := resolver.Discover(ctx, actors[0], key, observationscope.DiscoveryInput{ResourceTypes: []string{"host"}, Limit: 100})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("current department not resolved: %+v %v", page, err)
	}
	for _, item := range page.Items {
		if !slices.Contains(hosts[1], item.ID) {
			t.Fatal("old department authority retained")
		}
	}
	// Ordinary observation authority does not confer dashboard authoring or a
	// publication confirmation capability. This uses the real same PG actor.
	registry := mcp.NewRegistry()
	observe := ObservationTools{Base: ResourceTools{Store: store}, Monitoring: monitoring.Service{Store: store}, Telemetry: telemetry.Service{Store: store}, Dashboard: dashboard.Service{Store: store}, CursorKey: key}
	if err := observe.Register(registry); err != nil {
		t.Fatal(err)
	}
	actor := actors[0]
	call := mcp.Call{Enterprise: actor.EnterpriseID.String(), Subject: actor.SubjectID.String(), SubjectType: actor.SubjectType, ToolID: "observe.capabilities.get", Input: map[string]any{"resources": []any{map[string]any{"type": "host", "id": hosts[1][0].String()}}}}
	if _, err := registry.Call(ctx, call); err != nil {
		t.Fatal("authorized read-only capabilities", err)
	}
	if _, ok := registry.Lookup("observe.publish.commit"); ok {
		t.Fatal("observation exposed a publication Commit")
	}
	dashboardActor := dashboard.Actor{EnterpriseID: actor.EnterpriseID, SubjectID: actor.SubjectID, SubjectType: actor.SubjectType, AuthorizationVersion: actor.AuthorizationVersion}
	if _, err := (dashboard.Service{Store: store}).CreateDraft(ctx, dashboardActor, dashboard.DraftInput{Name: "Unprivileged observation draft"}); !errors.Is(err, dashboard.ErrDenied) {
		t.Fatal("observation grant did not preserve dashboard authorization", err)
	}
	var drafts, pending int
	if err := store.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM dashboard_drafts),(SELECT count(*) FROM pending_actions)").Scan(&drafts, &pending); err != nil || drafts != 0 || pending != 0 {
		t.Fatalf("read-only observation prepared a mutation: drafts=%d pending=%d err=%v", drafts, pending, err)
	}
}
