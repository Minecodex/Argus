package httpapi

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
	"time"
)

func TestNamespaceDirectoryOmitsClusterStatisticsAndCredentialReferences(t *testing.T) {
	value := db.KubernetesCluster{ID: uuid.New(), Name: "parent", CredentialID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, ConnectorID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, BastionScopeID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, NodeCount: 8, ReadyNodeCount: 7, StatisticsObservedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	limited := toKubernetesDirectoryCluster(value, false)
	if limited.Id != value.ID || limited.Name != "parent" || limited.NodeCount != nil || limited.ReadyNodeCount != nil || limited.StatisticsObservedAt != nil || limited.CredentialId != nil || limited.ConnectorId != nil || limited.BastionScopeId != nil {
		t.Fatal("namespace directory exposed cluster telemetry or private references")
	}
	full := toKubernetesDirectoryCluster(value, true)
	if full.NodeCount == nil || *full.NodeCount != 8 || full.CredentialId == nil {
		t.Fatal("cluster-authorized projection changed")
	}
}
func TestDataAuthorizationResourceTypesFollowDomainRegistry(t *testing.T) {
	for _, kind := range []string{"host", "kubernetes_cluster", "dashboard", "application", "monitoring_target", "kubernetes_namespace"} {
		if !validDataAuthorizationResourceType(kind) {
			t.Fatalf("registered grant root rejected by HTTP: %s", kind)
		}
	}
	for _, kind := range []string{"service", "collector", "pod", ""} {
		if validDataAuthorizationResourceType(kind) {
			t.Fatalf("non-root authorization accepted: %s", kind)
		}
	}
}
