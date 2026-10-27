package monitoring

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/trustbundle"
)

type connectionTrust struct{ bundle trustbundle.Bundle }

func (c connectionTrust) Current(context.Context) (trustbundle.Bundle, error) { return c.bundle, nil }

func TestPostgresSDKConnectionGuideKeepsApplicationScopeAndCurrentConfiguration(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	f.exec("UPDATE collector_distribution_versions SET components=ARRAY['otlp','argus_application_auth','argus_application_identity'] WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector)
	f.exec("UPDATE hosts SET address='192.0.2.197' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", collector)
	app := f.application("SDK connection guide", "production")
	endpoint, err := f.apply(Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "Actual SDK guide", ApplicationID: app.ID, CollectorID: collector, Protocol: "otlp", Signals: []string{"traces", "logs"}, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	raw, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	material, err := trustbundle.Parse(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), now)
	if err != nil {
		t.Fatal(err)
	}
	f.s.TrustBundles = connectionTrust{trustbundle.Bundle{Epoch: 3, State: trustbundle.StateStable, Material: material}}
	if _, err = f.s.IngestConnection(ctx, f.other, endpoint.ResourceID); !errors.Is(err, ErrDenied) {
		t.Fatal("guide crossed application data authorization", err)
	}
	guide, err := f.s.IngestConnection(ctx, f.a, endpoint.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if guide.ConfigurationStatus != "pending" || guide.Environment != "production" || !guide.ServiceNameRequired || guide.ObservationState != "waiting" || len(guide.Listeners) != 2 || guide.TLS.ServerName != "collector-"+collector.String()+".argus.telemetry" || guide.TLS.TrustBundleEpoch != 3 || guide.TLS.CABundleSHA256 != material.SHA256 {
		t.Fatal("guide did not reflect actual configuration/trust/application state")
	}
	encoded, _ := json.Marshal(guide)
	for _, forbidden := range []string{"192.0.2.197", "PRIVATE KEY", "argus_app."} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("guide exposed an infrastructure address or private credential")
		}
	}
	instance, err := f.s.Store.Queries.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: f.a.EnterpriseID, ID: collector})
	if err != nil {
		t.Fatal(err)
	}
	config, err := configbundle.Render(configbundle.RenderInput{CollectorID: collector.String(), ResourceID: instance.ResourceID.String(), ResourceType: "host", Role: "leaf", Platform: "linux_amd64", SourceGeneration: instance.SourceGeneration.String(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://authority.test/enroll", IngestHTTPEndpoint: "https://authority.test", IngestGRPCEndpoint: "grpcs://authority.test:4317", ApplicationEndpoints: []configbundle.ApplicationEndpoint{{ID: endpoint.ResourceID.String(), ApplicationID: app.ID.String(), Protocol: "otlp", Signals: []string{"traces", "logs"}, Version: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,decode(repeat('00',32),'hex'),'effective')", uuid.New(), collector, config)
	f.exec("UPDATE collector_instances SET effective_revision=1 WHERE id=$1", collector)
	guide, err = f.s.IngestConnection(ctx, f.a, endpoint.ResourceID)
	if err != nil || guide.ConfigurationStatus != "effective" {
		t.Fatal("guide ignored the verified configuration snapshot", err)
	}
	f.exec("UPDATE monitoring_ingest_endpoints SET version=2 WHERE id=$1", endpoint.ResourceID)
	guide, err = f.s.IngestConnection(ctx, f.a, endpoint.ResourceID)
	if err != nil || guide.ConfigurationStatus != "pending" {
		t.Fatal("new endpoint version borrowed old configuration success", err)
	}
	// An application grant provides its SDK settings, without a host grant or
	// infrastructure telemetry. The guide exposes canonical DNS, not Host IP.
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), f.a.EnterpriseID, f.other.SubjectID, app.ID)
	other, err := f.s.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: f.a.EnterpriseID, ID: f.other.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	f.other.AuthorizationVersion = other.AuthorizationVersion
	if err = authorization.RequireResource(ctx, f.s.Store.Queries, f.other, "host", instance.ResourceID); !errors.Is(err, ErrDenied) {
		t.Fatal("test actor unexpectedly has infrastructure access", err)
	}
	if _, err = f.s.IngestConnection(ctx, f.other, endpoint.ResourceID); err != nil {
		t.Fatal("application-only SDK settings require unrelated infrastructure grants", err)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2 AND resource_id=$3", f.a.EnterpriseID, f.other.SubjectID, app.ID)
	if _, err = f.s.IngestConnection(ctx, f.other, endpoint.ResourceID); !errors.Is(err, ErrDenied) {
		t.Fatal("revoked application could still read SDK settings", err)
	}
}
