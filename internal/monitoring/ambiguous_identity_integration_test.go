package monitoring

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A configured Collector is only a collection-placement identity here. This
// fixture does not establish connectivity, a network domain or real telemetry.
func TestPostgresAmbiguousNamesAndEndpointsKeepExplicitIdentities(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	prod := f.application("same-service", "production")
	testApp := f.application("same-service", "testing")
	if prod.ID == testApp.ID {
		t.Fatal("same-name applications in different environments merged")
	}
	firstCollector := f.collector()
	// The shared helper deliberately uses one fixed display name per fixture.
	f.exec("UPDATE hosts SET name='identity-placement-a' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", firstCollector)
	collectors := []uuid.UUID{firstCollector, f.collector()}
	var targets []uuid.UUID
	for _, collector := range collectors {
		created, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{
			Name: "same-config", TargetName: "same-database", Integration: "nginx",
			TargetIdentity: json.RawMessage(`{"endpoint":"http://10.0.0.8/status"}`),
			CollectorID:    collector, Config: json.RawMessage(`{}`), Enabled: false,
		}})
		if err != nil {
			t.Fatal(err)
		}
		row, err := f.s.Store.Queries.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
		if err != nil || row.CollectorID.UUID != collector || row.DesiredEnabled {
			t.Fatal("disabled collection lost its explicit collector identity", err)
		}
		targets = append(targets, row.TargetID)
	}
	if targets[0] == targets[1] {
		t.Fatal("same endpoint/name across collectors was silently merged")
	}
	resolver := observationscope.Resolver{Queries: f.s.Store.Queries}
	key := bytes.Repeat([]byte{13}, 32)
	apps, err := resolver.Discover(ctx, f.a, key, observationscope.DiscoveryInput{ResourceTypes: []string{"application"}, Search: "same-service", Limit: 10})
	if err != nil || len(apps.Items) != 2 {
		t.Fatal("same-name environment candidates were collapsed or omitted", err)
	}
	environments := map[string]uuid.UUID{}
	for _, item := range apps.Items {
		environments[item.Environment] = item.ID
	}
	if environments["production"] != prod.ID || environments["testing"] != testApp.ID {
		t.Fatal("application environment identity changed")
	}
	page, err := resolver.Discover(ctx, f.a, key, observationscope.DiscoveryInput{ResourceTypes: []string{"monitoring_target"}, Search: "same-database", Limit: 10})
	if err != nil || len(page.Items) != 2 {
		t.Fatal("same-name endpoint candidates were collapsed", err)
	}
	for _, item := range page.Items {
		if item.Environment != "" || (item.ID != targets[0] && item.ID != targets[1]) {
			t.Fatal("target inferred a collector environment or a foreign identity")
		}
	}
	for _, id := range targets {
		snapshot, err := resolver.Resolve(ctx, f.a, observationscope.Selection{Mode: observationscope.Explicit, Types: []string{"monitoring_target"}, IDs: []uuid.UUID{id}, Signal: "metrics"})
		if err != nil || len(snapshot.Resources) != 1 || snapshot.Resources[0].ID != id {
			t.Fatal("explicit target selection expanded to its same-name peer", err)
		}
	}
	t.Log("Cross-environment application names and same-address target names retain distinct IDs; collector placement is not called a verified network domain")
}

func TestPostgresTargetIdentityDoesNotGrantCollectorHostPlacement(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	created, err := f.apply(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{
		Name: "identity-only", TargetName: "same-address-database", Integration: "nginx",
		TargetIdentity: json.RawMessage(`{"endpoint":"http://10.0.0.8/status"}`), CollectorID: collector,
		Config: json.RawMessage(`{}`), Enabled: false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(t.Context(), db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), f.a.EnterpriseID, f.other.SubjectID, row.TargetID)
	resolver := observationscope.Resolver{Queries: f.s.Store.Queries}
	page, err := resolver.Discover(t.Context(), f.other, bytes.Repeat([]byte{13}, 32), observationscope.DiscoveryInput{ResourceTypes: []string{"monitoring_target"}, Search: "same-address-database", Limit: 10})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != row.TargetID || page.Items[0].ParentID != nil || page.Items[0].ParentName != "" || page.Items[0].Environment != "" {
		t.Fatal("target-only directory lost explicit identity or inferred private placement", err)
	}
	choices, err := f.s.CollectorChoices(t.Context(), f.other, ListInput{Limit: 10})
	if err != nil || len(choices.Items) != 0 {
		t.Fatal("target grant exposed the Collector's unauthorized Host", err)
	}
	grant := uuid.New()
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) SELECT $1,$2,'user',$3,'host',resource_id FROM collector_instances WHERE id=$4", grant, f.a.EnterpriseID, f.other.SubjectID, collector)
	choices, err = f.s.CollectorChoices(t.Context(), f.other, ListInput{Limit: 10})
	if err != nil || len(choices.Items) != 1 || choices.Items[0].ID != collector || choices.Items[0].Resource.Kind != "host" {
		t.Fatal("explicit authorized placement was unavailable", err)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	choices, err = f.s.CollectorChoices(t.Context(), f.other, ListInput{Limit: 10})
	if err != nil || len(choices.Items) != 0 {
		t.Fatal("revoked Host placement survived through a target relationship", err)
	}
}

func TestPostgresDisabledDatabaseIdentityAcceptsGatewayPlacement(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	f.exec("UPDATE collector_instances SET role='edge_gateway' WHERE id=$1", collector)
	created, err := f.applyCredentials(Mutation{Kind: "collection", Operation: "create", Collection: &CollectionInput{
		Name: "disabled-gateway-identity", TargetName: "same-address-database", Integration: "mysql",
		TargetIdentity: json.RawMessage(`{"endpoint":"10.77.0.8:3306"}`), CollectorID: collector,
		Config: json.RawMessage(`{"interval":"5s","timeout":"5s"}`), Enabled: false, CredentialUpdate: "replace",
	}}, json.RawMessage(`{"username":"inactive_fixture","password":"isolated-fixture-only"}`))
	if err != nil {
		t.Fatal("disabled metadata target rejected the actual gateway role", err)
	}
	row, err := f.s.Store.Queries.GetMonitoringCollection(t.Context(), db.GetMonitoringCollectionParams{EnterpriseID: f.a.EnterpriseID, ID: created.ResourceID})
	if err != nil || row.CollectorID.UUID != collector || row.DesiredEnabled || row.TargetType != "monitoring_target" {
		t.Fatal("gateway placement metadata lost its exact identity or became enabled", err)
	}
}
