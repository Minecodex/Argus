package toolgateway

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

// This measures the metadata-only maximum batch, not Receiver/telemetry
// throughput. An over-limit request must fail before any database acquisition.
func TestPostgresObservationCapabilityMaximumBatchRoundTrips(t *testing.T) {
	address := os.Getenv("ARGUS_PLAN11_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned isolated Plan11 PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, user, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'Plan11 batch','UTC')", enterprise)
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Batch readers')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Batch reader')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Batch reader')", role, enterprise)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'monitoring.read'),($1,'telemetry.dashboard.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	refs := []resourceref.Ref{}
	for i := 0; i < 21; i++ {
		id := uuid.New()
		exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,$3,'database','mysql','{}',$4)", id, enterprise, id.String(), user)
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), enterprise, user, id)
		refs = append(refs, resourceref.Ref{Type: "monitoring_target", ID: id})
	}
	actor := authorization.Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	key := bytes.Repeat([]byte{21}, 32)
	tools := ObservationTools{Base: ResourceTools{Store: store}, Monitoring: monitoring.Service{Store: store, CursorKey: key}, Dashboard: dashboard.Service{Store: store, ProvideDefaults: true}, CursorKey: key}
	before := store.Pool.Stat().AcquireCount()
	started := time.Now()
	result, err := tools.capabilities(t.Context(), actor, mcp.Call{Input: map[string]any{"resources": refs[:20]}})
	elapsed := time.Since(started)
	queries := store.Pool.Stat().AcquireCount() - before
	if err != nil {
		t.Fatal(err)
	}
	data, ok := result.(map[string]any)
	if !ok || len(data["items"].([]any)) != 20 || queries < 1 {
		t.Fatal("maximum authorized batch did not return its actual metadata")
	}
	before = store.Pool.Stat().AcquireCount()
	if _, err = tools.capabilities(t.Context(), actor, mcp.Call{Input: map[string]any{"resources": refs}}); err == nil {
		t.Fatal("over-limit batch was accepted")
	}
	if store.Pool.Stat().AcquireCount() != before {
		t.Fatal("over-limit input reached the database")
	}
	evidence := map[string]any{"passed": true, "layer": "L2", "scope": "20 authorized monitoring targets, four default capability sections, no telemetry backend or target probes", "resources": 20, "pool_acquisitions": queries, "elapsed_ms": elapsed.Milliseconds(), "over_limit_database_acquisitions": 0, "receiver_throughput_tested": false}
	if path := os.Getenv("ARGUS_PLAN11_BATCH_EVIDENCE"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("batch evidence path must be absolute")
		}
		raw, _ := json.MarshalIndent(evidence, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("maximum capability batch: resources=20 pool_acquisitions=%d elapsed_ms=%d", queries, elapsed.Milliseconds())
}
