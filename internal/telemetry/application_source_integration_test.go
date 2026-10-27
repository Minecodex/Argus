package telemetry

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// Transport authentication is a controlled precondition here. Source/endpoint/
// Key resolution and discovery use real PostgreSQL and production domain code.
func TestPostgresApplicationTraceSourceAdmissionAndKeyRevocation(t *testing.T) {
	url := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, url, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, host, collector, generation, distribution, creator := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Trace source admission',$2,'UTC')", e, "trace-source-"+e.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Collector transport','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,components,artifact_manifest,catalog_revision) VALUES($1,$2,$3,$4,$5,'supported',$6,'[]',1)", distribution, distribution.String(), configbundle.DistributionVersion, configbundle.CollectorVersion, configbundle.CatalogConfigSchemaVersion, configbundle.DistributionComponents("linux_amd64"))
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, e, host, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	server := IngestServer{Control: PostgresIngestControl{Queries: store.Queries}, Applications: monitoring.Service{Store: store}}
	sender := TrustedIdentity{EnterpriseID: e, CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "direct"}
	for _, protocol := range []string{"otlp", "jaeger"} {
		t.Run(protocol, func(t *testing.T) {
			app, endpoint, key, source := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Application','production',$4)", app, e, app.String(), creator)
			exec("INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,collector_id,created_by) VALUES($1,$2,$3,$4,$4,ARRAY['traces'],$5,$6)", endpoint, e, app, protocol, collector, creator)
			digest := sha256.Sum256([]byte(uuid.NewString()))
			exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) VALUES($1,$2,$3,$4,'controlled-test',$5,now()+interval '1 hour',$6)", key, e, endpoint, app, digest[:], creator)
			exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,ingest_endpoint_id,ingest_endpoint_version) VALUES($1,$2,$3,$4,'application',$5,$6,$6,ARRAY['traces'],1,decode(repeat('00',32),'hex'),'v1',$7,1)", source, e, collector, generation, app, protocol, endpoint)
			request := func(environment string) *collecttraces.ExportTraceServiceRequest {
				attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.application.key.id", Value: stringValue(key.String())}, {Key: "service.name", Value: stringValue("orders")}, {Key: "service.namespace", Value: stringValue("business")}, {Key: "argus.application.id", Value: stringValue(uuid.NewString())}}
				if environment != "" {
					attrs = append(attrs, &commonpb.KeyValue{Key: "deployment.environment.name", Value: stringValue(environment)})
				}
				return &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: attrs}}}}
			}
			valid := request("")
			if _, err := server.prepareTracePayload(ctx, sender, valid); err != nil {
				t.Fatal(err)
			}
			attrs := attributesMap(valid.ResourceSpans[0].Resource.Attributes)
			if attrs["argus.resource.id"] != app.String() || attrs["argus.resource.type"] != "application" || attrs["deployment.environment.name"] != "production" || attrs["argus.service.id"] == "" || attrs["argus.source.type"] != protocol {
				t.Fatal("application source lost trusted identity")
			}
			serviceID := attrs["argus.service.id"]
			again := request("production")
			if _, err := server.prepareTracePayload(ctx, sender, again); err != nil || attributesMap(again.ResourceSpans[0].Resource.Attributes)["argus.service.id"] != serviceID {
				t.Fatal("SDK identity was not stable")
			}
			bad := request("testing")
			before, _ := proto.Marshal(bad)
			if _, err := server.prepareTracePayload(ctx, sender, bad); err == nil {
				t.Fatal("SDK overrode fixed application environment")
			}
			after, _ := proto.Marshal(bad)
			if !bytes.Equal(before, after) {
				t.Fatal("rejected SDK batch mutated")
			}
			exec("UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1 WHERE id=$1", key)
			if _, err := server.prepareTracePayload(ctx, sender, request("")); err == nil {
				t.Fatal("revoked Key accepted through receiver cache")
			}
		})
	}
}
