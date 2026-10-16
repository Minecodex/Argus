package toolgateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresConnectorPresentationRelationsRespectCurrentAccess(t *testing.T) {
	address := os.Getenv("ARGUS_TEMPLATE_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned disposable template PostgreSQL")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, department, user, role, host, denied, deleted, cluster := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Connector relations',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Readers')", department, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Relation reader')", user, e, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Connector reader')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'connector.read'),($1,'host.read'),($1,'kubernetes.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, user, role)
	for id, name := range map[uuid.UUID]string{host: "Authorized host", denied: "Hidden host", deleted: "Deleted host"} {
		exec("INSERT INTO hosts(id,enterprise_id,name,platform,role,control_path,environment,labels_hash) VALUES($1,$2,$3,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", id, e, name)
	}
	exec("UPDATE hosts SET status='deleted',deleted_at=now() WHERE id=$1", deleted)
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,environment,labels_hash) VALUES($1,$2,'Authorized cluster','https://kubernetes.example.test','in_cluster','development',decode(repeat('00',32),'hex'))", cluster, e)
	for id, kind := range map[uuid.UUID]string{host: "host", deleted: "host", cluster: "kubernetes_cluster"} {
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,$4,$5)", uuid.New(), e, user, kind, id)
	}
	connectors := map[uuid.UUID]uuid.UUID{}
	for _, target := range []uuid.UUID{host, denied, deleted, cluster} {
		id := uuid.New()
		connectors[target] = id
		kind, hostID, clusterID := "host", uuid.NullUUID{UUID: target, Valid: true}, uuid.NullUUID{}
		if target == cluster {
			kind, hostID, clusterID = "kubernetes", uuid.NullUUID{}, uuid.NullUUID{UUID: cluster, Valid: true}
		}
		exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,kubernetes_cluster_id,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),now()+interval '1 hour')", id, e, kind, "Connector "+id.String(), hostID, clusterID, id.String())
	}
	registry := mcp.NewRegistry()
	exec("UPDATE connectors SET software_version='fixture-v9.8.7' WHERE id=$1", connectors[host])
	tools := ResourceTools{Store: store, Resources: resource.Service{Store: store}}
	if err = tools.Register(registry); err != nil {
		t.Fatal(err)
	}
	present := func(tool string, input map[string]any) *toolruntime.Presentation {
		t.Helper()
		metadata, ok := registry.Lookup(tool)
		if !ok {
			t.Fatal("missing native connector tool")
		}
		call := mcp.Call{ToolID: tool, Enterprise: e.String(), Subject: user.String(), SubjectType: "user", Input: input}
		if err = metadata.Authorize(ctx, call); err != nil {
			t.Fatal(err)
		}
		result, e := metadata.Execute(ctx, call)
		if e != nil {
			t.Fatal(e)
		}
		value, e := metadata.Present(ctx, call, result)
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	assert := func(p *toolruntime.Presentation, target uuid.UUID, name string, available bool) {
		t.Helper()
		raw, _ := json.Marshal(p.Binding)
		if strings.Contains(string(raw), "Hidden host") || strings.Contains(string(raw), "Deleted host") {
			t.Fatal("unavailable relation name exposed")
		}
		if name != "" && strings.Contains(string(raw), name) != available {
			t.Fatal("relationship name disagrees with current authorization", name)
		}
		found := false
		for _, ref := range p.Resources {
			found = found || ref.ID == target.String()
		}
		if found != available {
			t.Fatal("navigation capability disagrees with current authorization", target, found)
		}
	}
	for target, name := range map[uuid.UUID]string{host: "Authorized host", cluster: "Authorized cluster", denied: "", deleted: ""} {
		assert(present("connector.get", map[string]any{"connector_id": connectors[target].String()}), target, name, name != "")
	}
	list := present("connector.list", map[string]any{})
	rawList, _ := json.Marshal(list.Binding)
	if !strings.Contains(string(rawList), `"software_version":"fixture-v9.8.7"`) {
		t.Fatal("list lost native software version")
	}
	assert(list, host, "Authorized host", true)
	assert(list, cluster, "Authorized cluster", true)
	assert(list, denied, "", false)
	assert(list, deleted, "", false)
	exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='host.read'", role)
	assert(present("connector.get", map[string]any{"connector_id": connectors[host].String()}), host, "Authorized host", false)
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", e, cluster)
	assert(present("connector.get", map[string]any{"connector_id": connectors[cluster].String()}), cluster, "Authorized cluster", false)
}
