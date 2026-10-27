package connector

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestPostgresCollectorFailurePreservesOnlyVerifiedEffectiveRoute(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, test := range []struct {
		name, operation, code, route, expected string
		effective, desired                     int64
		verified                               bool
	}{
		{"candidate-config-rejected", "configure", "COLLECTOR_CONFIG_INVALID", "active", "active", 1, 2, true},
		{"rollback-unverified", "configure", "COLLECTOR_ROLLBACK_UNVERIFIED", "active", "degraded", 1, 2, true},
		{"no-effective-config", "configure", "COLLECTOR_CONFIG_INVALID", "active", "degraded", 0, 2, false},
		{"unverified-effective-config", "configure", "COLLECTOR_CONFIG_INVALID", "active", "degraded", 1, 2, false},
		{"changed-route-never-activated", "configure", "COLLECTOR_CONFIG_INVALID", "pending", "degraded", 1, 2, true},
		{"revoked-route", "configure", "COLLECTOR_CONFIG_INVALID", "invalidated", "invalidated", 1, 2, true},
		{"install-failed", "install", "COLLECTOR_ARTIFACT_INVALID", "active", "degraded", 1, 2, true},
		{"uninstall-failed", "uninstall", "COLLECTOR_MANAGEMENT_FAILED", "active", "degraded", 1, 2, true},
		{"stale-result", "configure", "COLLECTOR_CONFIG_INVALID", "active", "active", 1, 3, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			tx, err := store.Pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			exec := func(query string, args ...any) {
				t.Helper()
				if _, err := tx.Exec(ctx, query, args...); err != nil {
					t.Fatal(err)
				}
			}
			e, h, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned route outcome',$2,'UTC')", e, e.String())
			exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,'Owned route host','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", h, e)
			exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)VALUES($1,'Owned route fixture',$2,'fixture','fixture','supported','[]',1)", d, d.String())
			exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision)VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','installing',$5,$6)", c, e, h, d, test.desired, test.effective)
			bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: c.String(), Collections: []configbundle.Collection{}})
			previousStatus := "effective"
			if !test.verified {
				previousStatus = "failed"
			}
			exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)VALUES($1,$2,1,'{}',$3,decode(repeat('01',32),'hex'),$4)", uuid.New(), c, bundle, previousStatus)
			exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)VALUES($1,$2,2,'{}',$3,decode(repeat('02',32),'hex'),'applying')", uuid.New(), c, bundle)
			exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,transport,status)VALUES($1,$2,$3,'direct_argus','direct',$4)", uuid.New(), e, c, test.route)
			payload, err := protojson.Marshal(&connectorv1.CollectorManagementCommand{CollectorId: c.String(), ResourceId: h.String(), ResourceType: "host", Operation: test.operation, DesiredRevision: 2, Transport: "direct", ConfigSha256: "candidate"})
			if err != nil {
				t.Fatal(err)
			}
			if err = finalizeCollectorManagement(ctx, store.Queries.WithTx(tx), db.ConnectorCommand{EnterpriseID: e, Payload: payload}, "failed", nil, test.code); err != nil {
				t.Fatal(err)
			}
			var status string
			var effective int64
			if err = tx.QueryRow(ctx, "SELECT r.status,c.effective_revision FROM telemetry_routes r JOIN collector_instances c ON c.id=r.collector_id WHERE r.collector_id=$1", c).Scan(&status, &effective); err != nil {
				t.Fatal(err)
			}
			if status != test.expected || effective != test.effective {
				t.Errorf("candidate failure changed verified prior execution: status=%s expected=%s effective=%d expected=%d", status, test.expected, effective, test.effective)
			}
		})
	}
}
