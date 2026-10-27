// Package collectionclaim normalizes responsibility for fixed instances and
// service/log collections using the same ledger as legacy profile migrations.
package collectionclaim

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrConflict = errors.New("collection responsibility conflicts with another instance")

type Spec struct {
	Physical                string
	Type, Signal            string
	Selector                json.RawMessage
	CollectionID, ProfileID uuid.UUID
	Epoch                   int64
}

func Rebuild(ctx context.Context, q *db.Queries, enterprise uuid.UUID, collector db.CollectorInstance, profileIDs []uuid.UUID, collections []configbundle.Collection) error {
	profiles, err := q.ListCollectionProfiles(ctx)
	if err != nil {
		return err
	}
	bindings, err := q.ListKubernetesNodeHostBindings(ctx, db.ListKubernetesNodeHostBindingsParams{EnterpriseID: enterprise, KubernetesClusterID: collector.ResourceID})
	if err != nil {
		return err
	}
	specs, err := Plan(collector, profiles, profileIDs, collections, bindings)
	if err != nil {
		return err
	}
	if err := q.ReleaseCollectorClaims(ctx, db.ReleaseCollectorClaimsParams{EnterpriseID: enterprise, CollectorID: collector.ID}); err != nil {
		return err
	}
	for _, spec := range specs {
		hash := sha256.Sum256(spec.Selector)
		primary, err := q.GetActivePrimaryCollectionClaim(ctx, db.GetActivePrimaryCollectionClaimParams{EnterpriseID: enterprise, PhysicalResourceRef: spec.Physical, ClaimType: spec.Type, SelectorHash: hash[:]})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && primary.CollectorID == collector.ID {
			continue
		}
		input := db.CreateInstanceCollectionClaimParams{ID: uuid.New(), EnterpriseID: enterprise, PhysicalResourceRef: spec.Physical, CollectorID: collector.ID, ProfileID: nullable(spec.ProfileID), CollectionID: nullable(spec.CollectionID), AssignmentEpoch: pgtype.Int8{Int64: spec.Epoch, Valid: spec.Epoch > 0}, ConfigRevision: pgtype.Int8{Int64: collector.DesiredRevision, Valid: collector.DesiredRevision > 0}, ClaimType: spec.Type, Signal: spec.Signal, Selector: spec.Selector, SelectorHash: hash[:], Ownership: "primary"}
		if err == nil {
			if primary.CollectionID.Valid && primary.CollectionID.UUID != spec.CollectionID {
				return ErrConflict
			}
			input.Ownership = "migration"
			input.PrimaryClaimID = nullable(primary.ID)
			input.RollbackPlan, _ = json.Marshal(map[string]any{"restore_primary_claim_id": primary.ID, "collector_id": primary.CollectorID})
			input.ExpiresAt = pgtype.Timestamptz{Time: time.Now().UTC().Add(15 * time.Minute), Valid: true}
		}
		if _, err := q.CreateInstanceCollectionClaim(ctx, input); err != nil {
			return err
		}
	}
	return nil
}

func nullable(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }

func Plan(collector db.CollectorInstance, profiles []db.CollectionProfile, selected []uuid.UUID, collections []configbundle.Collection, bindings []db.KubernetesNodeHostBinding) ([]Spec, error) {
	result := []Spec{}
	overridden := map[string]bool{}
	for _, collection := range collections {
		if d, ok := builtincollection.Lookup(collection.Integration); ok && d.LegacyProfile != "" {
			overridden[d.LegacyProfile] = true
		}
	}
	for _, collection := range collections {
		if !collection.Enabled {
			continue
		}
		id, err := uuid.Parse(collection.ID)
		if err != nil {
			return nil, err
		}
		physical := collection.TargetType + ":" + collection.TargetID
		family, signal, selector := collection.Integration+".metrics", "metrics", json.RawMessage(`{}`)
		refs := []string{physical}
		if d, ok := builtincollection.Lookup(collection.Integration); ok {
			switch d.Key {
			case "host_metrics":
				family = "host"
			case "kubernetes_node_metrics":
				family = "kubernetes-node"
				refs = nodeRefs(collector, bindings)
			case "kubernetes_cluster_metrics":
				family = "kubernetes-cluster"
			case "collector_metrics":
				family = "collector"
				refs = []string{"collector:" + collector.ID.String()}
			default:
				family = d.SourceType
			}
		} else if logcollection.IsIntegration(collection.Integration) {
			signal = "logs"
			family = "log." + collection.Integration
			physical = collector.ResourceType + ":" + collector.ResourceID.String()
			refs = []string{physical}
			var config map[string]any
			if json.Unmarshal(collection.Config, &config) != nil {
				return nil, ErrConflict
			}
			delete(config, "interval")
			delete(config, "timeout")
			selector, _ = json.Marshal(config)
		} else {
			entry, ok := monitoringcatalog.Lookup(collection.Integration)
			if !ok || !entry.AdapterReady {
				return nil, ErrConflict
			}
		}
		for _, ref := range refs {
			result = append(result, Spec{Physical: ref, Type: family, Signal: signal, Selector: selector, CollectionID: id, Epoch: collection.AssignmentEpoch})
		}
	}
	for _, id := range selected {
		var profile db.CollectionProfile
		found := false
		for _, p := range profiles {
			if p.ID == id {
				profile, found = p, true
				break
			}
		}
		if !found {
			return nil, ErrConflict
		}
		if overridden[profile.ProfileKey] {
			continue
		}
		claims := profile.ClaimTypes
		if len(claims) == 0 {
			claims = []string{profile.ProfileKey}
		}
		if len(profile.Signals) == 0 || len(claims) != 1 && len(claims) != len(profile.Signals) {
			return nil, ErrConflict
		}
		refs := []string{collector.ResourceType + ":" + collector.ResourceID.String()}
		if profile.ProfileKey == "k8s-node-container" {
			refs = nodeRefs(collector, bindings)
		}
		for i, signal := range profile.Signals {
			family := claims[0]
			if len(claims) == len(profile.Signals) {
				family = claims[i]
			} else if len(profile.Signals) > 1 {
				family += "." + signal
			}
			for _, ref := range refs {
				result = append(result, Spec{Physical: ref, Type: family, Signal: signal, Selector: json.RawMessage(`{}`), ProfileID: id})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		return a.Physical+"/"+a.Type+"/"+string(a.Selector) < b.Physical+"/"+b.Type+"/"+string(b.Selector)
	})
	return result, nil
}

func nodeRefs(collector db.CollectorInstance, bindings []db.KubernetesNodeHostBinding) []string {
	refs := []string{}
	for _, binding := range bindings {
		ref := "kubernetes_node:" + collector.ResourceID.String() + ":" + binding.NodeUid
		if binding.Status == "verified" && binding.HostID.Valid {
			ref = "host:" + binding.HostID.UUID.String()
		}
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		refs = append(refs, "kubernetes_cluster:"+collector.ResourceID.String())
	}
	sort.Strings(refs)
	return refs
}
