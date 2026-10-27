package monitoring

import (
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"testing"
	"time"
)

func TestPostgresSDKRejectsOldDistributionAtCreationAuthenticationAndDiscovery(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	ctx := t.Context()
	components := []string{"otlp", "argus_application_auth", "argus_application_identity"}
	f.exec("UPDATE collector_distribution_versions SET version=$2,components=$3 WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector, configbundle.DistributionVersion, components)
	app := f.application("Versioned SDK application", "production")
	input := Mutation{Kind: "endpoint", Operation: "create", Endpoint: &IngestEndpointInput{Name: "Versioned OTLP", ApplicationID: app.ID, CollectorID: collector, Protocol: "otlp", Signals: []string{"traces"}, Enabled: true}}
	endpoint, err := f.apply(input)
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.apply(Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.s.RevealIngestKey(ctx, f.a, key.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, version string
		components    []string
		supported     bool
	}{
		{"previous-identity-validation", "0.3.0-plan8", components, false},
		{"unknown-distribution", "99.0.0", components, false},
		{"missing-auth", configbundle.DistributionVersion, []string{"otlp", "argus_application_identity"}, false},
		{"locked-current-distribution", configbundle.DistributionVersion, components, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.exec("UPDATE collector_distribution_versions SET version=$2,components=$3 WHERE id=(SELECT distribution_version_id FROM collector_instances WHERE id=$1)", collector, test.version, test.components)
			_, previewErr := f.s.Preview(ctx, f.a, input, uuid.NewString())
			_, keyErr := f.s.Preview(ctx, f.a, Mutation{Kind: "ingest_key", Operation: "create", IngestKey: &IngestKeyInput{EndpointID: endpoint.ResourceID, ExpiresAt: time.Now().Add(time.Hour)}}, uuid.NewString())
			_, authErr := AuthenticateIngestKey(ctx, f.s.Store.Queries, collector, f.a.EnterpriseID, "otlp", "traces", token)
			_, discoverErr := f.s.RegisterIngestResources(ctx, f.a.EnterpriseID, collector, "traces", []IngestObservation{{KeyID: key.ResourceID, EndpointID: endpoint.ResourceID, ApplicationID: app.ID, Protocol: "otlp", ServiceName: "versioned-service"}})
			if test.supported {
				if previewErr != nil || keyErr != nil || authErr != nil || discoverErr != nil {
					t.Fatalf("locked distribution refused: preview=%v key=%v auth=%v discovery=%v", previewErr, keyErr, authErr, discoverErr)
				}
			} else {
				if !errors.Is(previewErr, ErrUnsupported) {
					t.Errorf("unverified SDK creation admitted: %v", previewErr)
				}
				if !errors.Is(keyErr, ErrUnsupported) {
					t.Errorf("unverified SDK key creation admitted: %v", keyErr)
				}
				if !errors.Is(authErr, ErrIngestRejected) {
					t.Errorf("unverified SDK authentication admitted: %v", authErr)
				}
				if !errors.Is(discoverErr, ErrIngestRejected) {
					t.Errorf("unverified SDK discovery admitted: %v", discoverErr)
				}
			}
		})
	}
}
