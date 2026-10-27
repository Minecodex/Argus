package collectormanager_test

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/identity"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupActualPickerMetadata(t *testing.T, f *nativeSDKPipeline, session string) (string, string, string, func()) {
	t.Helper()
	var user, role uuid.UUID
	if err := f.store.Pool.QueryRow(f.ctx, `SELECT s.user_id,b.role_id FROM sessions s JOIN role_bindings b ON b.subject_id=s.user_id AND b.subject_type='user' WHERE s.token_hash=$1 AND s.enterprise_id=$2 LIMIT 1`, identity.TokenHash(session), f.enterprise).Scan(&user, &role); err != nil {
		t.Fatal("owned picker session missing", err)
	}
	addedPermissions := []string{}
	for _, permission := range []string{"monitoring.read", "monitoring.manage", "application.manage", "collector.use"} {
		result, err := f.store.Pool.Exec(f.ctx, `INSERT INTO role_permissions(role_id,permission_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, role, permission)
		if err != nil {
			t.Fatal(err)
		}
		if result.RowsAffected() == 1 {
			addedPermissions = append(addedPermissions, permission)
		}
	}
	hostGrant := uuid.New()
	f.exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)`, hostGrant, f.enterprise, user, f.host)
	apps, services := []uuid.UUID{}, []uuid.UUID{}
	records := []map[string]any{}
	serviceRecords := []map[string]any{}
	for i := 0; i < 80; i++ {
		id := uuid.New()
		apps = append(apps, id)
		environment := "production"
		if i%2 == 1 {
			environment = "testing"
		}
		name := fmt.Sprintf("PickerMeta %03d", i)
		applicationKey := id.String()
		if i == 10 {
			applicationKey = "picker-identifier-match-031"
		}
		f.exec(`INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,$4,$5,$6)`, id, f.enterprise, applicationKey, name, environment, user)
		f.exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)`, uuid.New(), f.enterprise, user, id)
		records = append(records, map[string]any{"id": id, "name": name, "environment": environment})
	}
	for i := 0; i < 80; i++ {
		id := uuid.New()
		services = append(services, id)
		serviceRecords = append(serviceRecords, map[string]any{"id": id, "name": fmt.Sprintf("PickerSvc %03d", i)})
		f.exec(`INSERT INTO monitoring_application_services(id,enterprise_id,application_id,service_namespace,service_name,first_seen_at,last_seen_at) VALUES($1,$2,$3,'metadata',$4,NULL,NULL)`, id, f.enterprise, apps[0], fmt.Sprintf("PickerSvc %03d", i))
	}
	nonce := uuid.NewString()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Owned-Fixture")), []byte(nonce)) != 1 {
			w.WriteHeader(403)
			return
		}
		var input struct {
			Operation string `json:"operation"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil {
			w.WriteHeader(400)
			return
		}
		var err error
		switch input.Operation {
		case "revoke":
			_, err = f.store.Pool.Exec(r.Context(), `DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_type='user' AND subject_id=$2 AND resource_type='application' AND resource_id=$3`, f.enterprise, user, apps[31])
		case "restore":
			_, err = f.store.Pool.Exec(r.Context(), `INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4) ON CONFLICT DO NOTHING`, uuid.New(), f.enterprise, user, apps[31])
		case "archive", "activate":
			status := "archived"
			if input.Operation == "activate" {
				status = "active"
			}
			_, err = f.store.Pool.Exec(r.Context(), `UPDATE monitoring_applications SET status=$4,version=version+1 WHERE enterprise_id=$1 AND id=$2 AND created_by=$3`, f.enterprise, apps[31], user, status)
		default:
			w.WriteHeader(400)
			return
		}
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	}))
	metadata := map[string]any{"scope": "real_metadata_pg_and_actual_http_browser_with_controlled_session_grant_and_archive_injection_no_monitoring_values_fabricated", "applications": records, "service_application": apps[0], "revoked_candidate": apps[31], "foreign_application": f.apps[1].app, "service_count": len(services), "services": serviceRecords, "collector_id": f.collector, "collector_name": "Controlled SDK transport"}
	body, _ := json.Marshal(metadata)
	path := filepath.Join(t.TempDir(), "picker-metadata.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		control.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.store.Pool.Exec(ctx, `DELETE FROM monitoring_application_services WHERE enterprise_id=$1 AND id=ANY($2::uuid[])`, f.enterprise, services); err != nil {
			t.Error("owned service metadata cleanup failed", err)
		}
		for _, query := range []string{`DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2 AND resource_type='application' AND resource_id=ANY($3::uuid[])`, `DELETE FROM monitoring_applications WHERE enterprise_id=$1 AND created_by=$2 AND id=ANY($3::uuid[])`} {
			if _, err := f.store.Pool.Exec(ctx, query, f.enterprise, user, apps); err != nil {
				t.Error("owned picker metadata cleanup failed", err)
			}
		}
		if _, err := f.store.Pool.Exec(ctx, `DELETE FROM data_authorization_grants WHERE id=$1 AND enterprise_id=$2 AND subject_id=$3`, hostGrant, f.enterprise, user); err != nil {
			t.Error(err)
		}
		for _, permission := range addedPermissions {
			if _, err := f.store.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_id=$2`, role, permission); err != nil {
				t.Error(err)
			}
		}
	}
	return path, control.URL, nonce, cleanup
}
