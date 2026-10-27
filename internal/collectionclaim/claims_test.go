package collectionclaim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestInstanceResponsibilitiesReplaceDisabledProfilesAndKeepTargetIdentity(t *testing.T) {
	host, collector, profile := uuid.New(), uuid.New(), uuid.New()
	instance := db.CollectorInstance{ID: collector, ResourceType: "host", ResourceID: host}
	profiles := []db.CollectionProfile{{ID: profile, ProfileKey: "host-basic", Signals: []string{"metrics"}, ClaimTypes: []string{"host"}}}
	collections := []configbundle.Collection{{ID: uuid.NewString(), Integration: "host_metrics", TargetType: "host", TargetID: host.String(), Config: json.RawMessage(`{}`), Enabled: false, Revision: 1, AssignmentEpoch: 1}}
	for _, target := range []string{uuid.NewString(), uuid.NewString()} {
		collections = append(collections, configbundle.Collection{ID: uuid.NewString(), Integration: "mysql", TargetType: "monitoring_target", TargetID: target, Config: json.RawMessage(`{}`), Enabled: true, Revision: 1, AssignmentEpoch: 1})
	}
	claims, err := Plan(instance, profiles, []uuid.UUID{profile}, collections, nil)
	if err != nil || len(claims) != 2 {
		t.Fatal("legacy profile revived a disabled instance", claims, err)
	}
	if claims[0].Physical == claims[1].Physical || !strings.HasPrefix(claims[0].Physical, "monitoring_target:") {
		t.Fatal("service responsibility followed collector host instead of stable target")
	}
	collections[1].AssignmentEpoch = 2
	next, err := Plan(db.CollectorInstance{ID: uuid.New(), ResourceType: "host", ResourceID: uuid.New()}, nil, nil, collections[1:], nil)
	if err != nil || next[0].Physical != claims[0].Physical || next[1].Physical != claims[1].Physical {
		t.Fatal("moving a collector changed logical service responsibility", err)
	}
}

func TestNodeClaimsAliasOnlyVerifiedHostsAndRetainUnboundNodes(t *testing.T) {
	cluster, host := uuid.New(), uuid.New()
	collector := db.CollectorInstance{ID: uuid.New(), ResourceType: "kubernetes_cluster", ResourceID: cluster}
	collection := configbundle.Collection{ID: uuid.NewString(), Integration: "kubernetes_node_metrics", TargetType: "kubernetes_cluster", TargetID: cluster.String(), Config: json.RawMessage(`{}`), Enabled: true, Revision: 1, AssignmentEpoch: 1}
	bindings := []db.KubernetesNodeHostBinding{{NodeUid: "known-node", Status: "verified", HostID: nullable(host)}, {NodeUid: "candidate-node", Status: "proposed", HostID: nullable(host)}}
	claims, err := Plan(collector, nil, nil, []configbundle.Collection{collection}, bindings)
	if err != nil || len(claims) != 2 {
		t.Fatal("unbound node responsibility disappeared", claims, err)
	}
	seen := map[string]bool{}
	for _, claim := range claims {
		seen[claim.Physical] = true
	}
	if !seen["host:"+host.String()] || !seen["kubernetes_node:"+cluster.String()+":candidate-node"] {
		t.Fatal("an IP proposal gained Host authority", seen)
	}
}
