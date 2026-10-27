package telemetry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/connector"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/trustbundle"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// The signing CA is controlled here; database access, certificate admission,
// SDK discovery, writer settlement and query audit use the real role accounts.
func TestPostgresTelemetryRuntimeRolesAndLateRoleReconciliation(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	directory := filepath.Join("..", "..", "migrations", "postgresql")
	if err := postgres.RunMigrations(ctx, address, directory, postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	owner, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := owner.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	created := []string{}
	t.Cleanup(func() {
		for _, role := range created {
			name := pgx.Identifier{role}.Sanitize()
			if _, err := owner.Pool.Exec(context.Background(), "DROP OWNED BY "+name); err != nil {
				t.Error(err)
				continue
			}
			if _, err := owner.Pool.Exec(context.Background(), "DROP ROLE "+name); err != nil {
				t.Error(err)
			}
		}
	})
	for _, role := range []string{"argus_telemetry_ingest", "argus_telemetry_query", "argus_telemetry_writer"} {
		var exists bool
		if err := owner.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", role).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			exec("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE")
			created = append(created, role)
		}
	}
	// Roles can be provisioned after a schema was migrated. Re-running the
	// official migration entry point must repair that deployment order too.
	if err := postgres.RunMigrations(ctx, address, directory, postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	if err := postgres.RunMigrations(ctx, address, directory, postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	stores := map[string]*postgres.Store{}
	for _, role := range []string{"argus_telemetry_ingest", "argus_telemetry_query", "argus_telemetry_writer"} {
		stores[role] = runtimeRoleStore(t, address, role)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	caSerial := uuid.New()
	template := &x509.Certificate{SerialNumber: new(big.Int).SetBytes(caSerial[:]), Subject: pkix.Name{CommonName: "Owned runtime role CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	encoded, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(encoded)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded})
	material, err := trustbundle.Parse(caPEM, now)
	if err != nil {
		t.Fatal(err)
	}
	var epoch int64
	if err = owner.Pool.QueryRow(ctx, "SELECT coalesce(max(epoch),0)+1 FROM pki_trust_bundles").Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	_, err = owner.Queries.CreateTrustBundle(ctx, db.CreateTrustBundleParams{Epoch: epoch, State: trustbundle.StateStable, BundlePem: string(caPEM), BundleSha256: material.SHA256, CurrentCaFingerprints: material.Fingerprints, NextCaFingerprints: []string{}, StartedAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := owner.Pool.Exec(context.Background(), "DELETE FROM pki_trust_bundles WHERE epoch=$1 AND bundle_sha256=$2", epoch, material.SHA256)
		if err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(path, caPEM, 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("read-only-trust-waits-for-control-plane", func(t *testing.T) {
		schema := "role_bootstrap_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		quoted := pgx.Identifier{schema}.Sanitize()
		exec("CREATE SCHEMA " + quoted)
		t.Cleanup(func() {
			if _, err := owner.Pool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
				t.Error(err)
			}
		})
		exec("CREATE TABLE " + quoted + ".pki_trust_bundles (LIKE public.pki_trust_bundles INCLUDING ALL)")
		exec("GRANT USAGE ON SCHEMA " + quoted + " TO argus_telemetry_query")
		exec("GRANT SELECT ON " + quoted + ".pki_trust_bundles TO argus_telemetry_query")
		reader := runtimeRoleStore(t, address, "argus_telemetry_query", schema)
		denyRuntimeSQL(t, reader, "INSERT INTO pki_trust_bundles SELECT * FROM pki_trust_bundles WHERE false")
		waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			bundle, err := (trustbundle.Service{Store: reader}).AwaitCurrent(waitCtx)
			if err == nil && bundle.Epoch != 1 {
				err = errors.New("wrong initial trust epoch")
			}
			done <- err
		}()
		select {
		case err := <-done:
			t.Fatalf("reader did not wait for control-plane initialization: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		exec("INSERT INTO "+quoted+".pki_trust_bundles(epoch,state,bundle_pem,bundle_sha256,current_ca_fingerprints,next_ca_fingerprints,started_at,last_error) VALUES(1,'stable',$1,$2,$3,'{}',now(),'')", string(caPEM), material.SHA256, material.Fingerprints)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
	enterprise, host, collector, generation, distribution, creator := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Runtime role regression',$2,'UTC')", enterprise, "runtime-roles-"+enterprise.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Role collector','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, enterprise)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,components,artifact_manifest,catalog_revision) VALUES($1,$2,$3,$4,$5,'supported',$6,'[]',1)", distribution, distribution.String(), configbundle.DistributionVersion, configbundle.CollectorVersion, configbundle.CatalogConfigSchemaVersion, configbundle.DistributionComponents("linux_amd64"))
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, enterprise, host, distribution, generation)
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector)
	for _, role := range []string{"argus_telemetry_ingest", "argus_telemetry_query"} {
		t.Run(role+"-trust", func(t *testing.T) {
			bundle := trustbundle.Service{Store: stores[role], MountedPath: path, InitialEpoch: epoch}
			current, err := bundle.Current(ctx)
			if err != nil || current.Epoch != epoch {
				t.Fatalf("read public trust as runtime role: %v", err)
			}
			if err = bundle.AcknowledgeMounted(ctx, "role-regression-"+uuid.NewString()); err != nil {
				t.Fatal(err)
			}
			denyRuntimeSQL(t, stores[role], "UPDATE pki_trust_bundles SET state=state WHERE false")
			denyRuntimeSQL(t, stores[role], "SELECT * FROM secret_versions LIMIT 0")
			denyRuntimeSQL(t, stores[role], "UPDATE collector_instances SET desired_revision=desired_revision WHERE false")
		})
	}
	t.Run("ingest-enrollment-rotation-sdk", func(t *testing.T) {
		ingest := stores["argus_telemetry_ingest"]
		identity := IdentityService{Store: ingest, Issuer: runtimeRoleIssuer{ca, key, caPEM, x509.ExtKeyUsageClientAuth}, ServerIssuer: runtimeRoleIssuer{ca, key, caPEM, x509.ExtKeyUsageServerAuth}, TrustBundles: trustbundle.Service{Store: ingest}}
		token, err := identity.CreateEnrollmentToken(ctx, owner.Queries, collector)
		if err != nil {
			t.Fatal(err)
		}
		clientCSR, serverCSR := runtimeRoleCSR(t, collector, false), runtimeRoleCSR(t, collector, true)
		enrolled, err := identity.Enroll(ctx, token, collector.String(), clientCSR, serverCSR)
		if err != nil {
			t.Fatal(err)
		}
		block, _ := pem.Decode([]byte(enrolled.ClientCertificatePEM))
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = (PostgresIngestControl{Queries: ingest.Queries}).ResolveCollectorIdentity(ctx, strings.ToLower(certificate.SerialNumber.Text(16))); err != nil {
			t.Fatal(err)
		}
		if _, err = identity.Rotate(ctx, certificate, runtimeRoleCSR(t, collector, false), runtimeRoleCSR(t, collector, true)); err != nil {
			t.Fatal(err)
		}
		app, endpoint, keyID, source := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Role application','production',$4)", app, enterprise, app.String(), creator)
		exec("INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,collector_id,created_by) VALUES($1,$2,$3,'Role SDK','otlp',ARRAY['traces'],$4,$5)", endpoint, enterprise, app, collector, creator)
		digest := sha256.Sum256([]byte(uuid.NewString()))
		exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) VALUES($1,$2,$3,$4,'role-test',$5,now()+interval '1 hour',$6)", keyID, enterprise, endpoint, app, digest[:], creator)
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,ingest_endpoint_id,ingest_endpoint_version) VALUES($1,$2,$3,$4,'application',$5,'otlp','otlp',ARRAY['traces'],1,decode(repeat('00',32),'hex'),'v1',$6,1)", source, enterprise, collector, generation, app, endpoint)
		attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.application.key.id", Value: stringValue(keyID.String())}, {Key: "service.name", Value: stringValue("role-service")}, {Key: "service.namespace", Value: stringValue("acceptance")}}
		request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: attrs}}}}
		server := IngestServer{Control: PostgresIngestControl{Queries: ingest.Queries}, Applications: monitoring.Service{Store: ingest}}
		if _, err = server.prepareTracePayload(ctx, TrustedIdentity{EnterpriseID: enterprise, CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "direct"}, request); err != nil {
			t.Fatal(err)
		}
		if attributesMap(request.ResourceSpans[0].Resource.Attributes)["deployment.environment.name"] != "production" {
			t.Fatal("trusted application context missing")
		}
		denyRuntimeSQL(t, ingest, "UPDATE monitoring_ingest_endpoints SET enabled=false WHERE false")
	})
	t.Run("collection-observation-and-recovery-without-config-writes", func(t *testing.T) {
		collection := uuid.New()
		exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,effective_collector_id,effective_revision,effective_assignment_epoch,apply_status,created_by) VALUES($1,$2,'Role sample observation','host_metrics','host',$3,$4,$4,1,1,'effective',$5)", collection, enterprise, host, collector, creator)
		ingest := stores["argus_telemetry_ingest"]
		observer := PostgresIngestControl{Queries: ingest.Queries}
		identity := TrustedIdentity{EnterpriseID: enterprise, CollectorID: collector}
		first := time.Now().UTC().Truncate(time.Microsecond)
		failed := CollectionSample{ID: collection, Revision: 1, Epoch: 1, Health: "failed", ErrorCode: "COLLECTION_SCRAPE_FAILED", HasData: false, ObservedAt: &first}
		if err := observer.ObserveCollectionSamples(ctx, identity, []CollectionSample{failed}); err != nil {
			t.Fatal(err)
		}
		row, err := owner.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: enterprise, ID: collection})
		if err != nil || row.Health != "failed" || row.ErrorCode != "COLLECTION_SCRAPE_FAILED" || row.LastSampleAt.Valid {
			t.Fatal("failed sample observation lost its state", err)
		}
		recovered := first.Add(time.Second)
		success := CollectionSample{ID: collection, Revision: 1, Epoch: 1, Health: "healthy", HasData: true, ObservedAt: &recovered, DataAt: &recovered}
		if err = observer.ObserveCollectionSamples(ctx, identity, []CollectionSample{success}); err != nil {
			t.Fatal(err)
		}
		row, err = owner.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: enterprise, ID: collection})
		if err != nil || row.Health != "healthy" || row.ErrorCode != "" || !row.LastSampleAt.Valid || !row.LastSampleAt.Time.Equal(recovered) {
			t.Fatal("recovered sample observation lost freshness or retained the scrape error", err)
		}
		stale := failed
		stale.Epoch = 2
		if err = observer.ObserveCollectionSamples(ctx, identity, []CollectionSample{stale}); err != nil {
			t.Fatal(err)
		}
		row, err = owner.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: enterprise, ID: collection})
		if err != nil || row.Health != "healthy" || !row.LastSampleAt.Time.Equal(recovered) {
			t.Fatal("foreign execution epoch changed observation state", err)
		}
		denyRuntimeSQL(t, ingest, "UPDATE monitoring_collections SET desired_enabled=false WHERE false")
		denyRuntimeSQL(t, ingest, "SELECT address FROM hosts LIMIT 0")
	})
	t.Run("writer-tenant-retention-usage", func(t *testing.T) {
		writer := PostgresWriterControl{Queries: stores["argus_telemetry_writer"].Queries}
		if writable, err := writer.TelemetryWritable(ctx, enterprise); err != nil || !writable {
			t.Fatalf("writer tenant admission: %v", err)
		}
		if _, err := writer.Retention(ctx, enterprise); err != nil {
			t.Fatal(err)
		}
		if err := writer.IncrementUsage(ctx, enterprise, TelemetryUsageDelta{Bytes: 100, Metrics: 1}); err != nil {
			t.Fatal(err)
		}
		denyRuntimeSQL(t, stores["argus_telemetry_writer"], "SELECT * FROM secret_versions LIMIT 0")
	})
	t.Run("query-audit-append-without-update", func(t *testing.T) {
		query := stores["argus_telemetry_query"]
		if err := query.InReadCommittedTx(ctx, func(q *db.Queries) error {
			id := uuid.NullUUID{UUID: enterprise, Valid: true}
			if err := audit.InitializeChain(ctx, q, "enterprise", id); err != nil {
				return err
			}
			_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: id, ActorType: "system", ActorID: "telemetry-query", Action: "telemetry.query", ResourceType: "query", ResourceID: uuid.NewString(), Result: "success"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		denyRuntimeSQL(t, query, "UPDATE audit_events SET result=result WHERE false")
		denyRuntimeSQL(t, query, "DELETE FROM audit_events WHERE false")
	})
}

func runtimeRoleStore(t *testing.T, address, role string, schemas ...string) *postgres.Store {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(address)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		if err == nil && len(schemas) == 1 {
			_, err = c.Exec(ctx, "SET search_path TO "+pgx.Identifier{schemas[0]}.Sanitize())
		}
		return err
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var actual string
	if err = pool.QueryRow(t.Context(), "SELECT current_user").Scan(&actual); err != nil || actual != role {
		t.Fatal("runtime role not active", err)
	}
	return &postgres.Store{Pool: pool, Queries: db.New(pool)}
}

func denyRuntimeSQL(t *testing.T, store *postgres.Store, query string) {
	t.Helper()
	_, err := store.Pool.Exec(t.Context(), query)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatalf("runtime role did not reject forbidden SQL: %v", err)
	}
}

type runtimeRoleIssuer struct {
	ca    *x509.Certificate
	key   *ecdsa.PrivateKey
	pem   []byte
	usage x509.ExtKeyUsage
}

func (i runtimeRoleIssuer) Issue(_ context.Context, _ uuid.UUID, csr *x509.CertificateRequest, ttl time.Duration) (connector.Certificate, error) {
	id := uuid.New()
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: new(big.Int).SetBytes(id[:]), Subject: csr.Subject, URIs: csr.URIs, DNSNames: csr.DNSNames, NotBefore: now.Add(-time.Second), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{i.usage}}
	der, err := x509.CreateCertificate(rand.Reader, template, i.ca, csr.PublicKey, i.key)
	if err != nil {
		return connector.Certificate{}, err
	}
	return connector.Certificate{PEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), CABundlePEM: string(i.pem), SerialNumber: template.SerialNumber.Text(16), CertificateRequestName: "owned-role-test", IssuerGeneration: 1, NotBefore: template.NotBefore, NotAfter: template.NotAfter}, nil
}
func runtimeRoleCSR(t *testing.T, id uuid.UUID, server bool) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	name := CollectorCertificateURI(id)
	dns := []string{}
	if server {
		name = CollectorServerCertificateURI(id)
		dns = []string{CollectorCertificateDNSName(id)}
	}
	uri, err := url.Parse(name)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}, DNSNames: dns}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}
