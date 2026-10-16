package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func authorize(ctx context.Context, q *db.Queries, a Actor, permission string) error {
	if err := dashboardaccess.Authorize(ctx, q, a, permission); err != nil {
		return ErrDenied
	}
	return nil
}
func actorResources(ctx context.Context, q *db.Queries, a Actor) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	for _, kind := range []struct{ kind, permission string }{{"host", "host.read"}, {"kubernetes_cluster", "kubernetes.read"}} {
		if err := dashboardaccess.Authorize(ctx, q, a, kind.permission); err != nil {
			if errors.Is(err, dashboardaccess.ErrDenied) {
				continue
			}
			return nil, err
		}
		values, err := dashboardaccess.AuthorizedIDs(ctx, q, a, kind.kind)
		if err != nil {
			return nil, err
		}
		ids = append(ids, values...)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return slices.Compact(ids), nil
}
func containsAll(have, want []uuid.UUID) bool {
	for _, id := range want {
		if !slices.Contains(have, id) {
			return false
		}
	}
	return true
}
func configuredEvaluator(ctx context.Context, q *db.Queries, enterprise uuid.UUID) (Actor, error) {
	configs, err := q.ListAlertConfigurations(ctx, db.ListAlertConfigurationsParams{EnterpriseID: enterprise, Kind: "settings"})
	if err != nil || len(configs) != 1 {
		return Actor{}, ErrEvaluator
	}
	spec, err := decodeSpec[SettingsSpec](configs[0].Spec)
	if err != nil {
		return Actor{}, ErrEvaluator
	}
	account, err := q.GetServiceAccount(ctx, db.GetServiceAccountParams{EnterpriseID: enterprise, ID: spec.EvaluatorID})
	if err != nil {
		return Actor{}, ErrEvaluator
	}
	return Actor{EnterpriseID: enterprise, SubjectType: "service_account", SubjectID: account.ID, AuthorizationVersion: account.AuthorizationVersion}, nil
}
func evaluator(ctx context.Context, q *db.Queries, enterprise uuid.UUID) (Actor, error) {
	a, err := configuredEvaluator(ctx, q, enterprise)
	if err != nil {
		return a, err
	}
	ids, err := actorResources(ctx, q, a)
	if err != nil || len(ids) == 0 {
		return a, ErrEvaluator
	}
	return a, nil
}
func selectedResources(ctx context.Context, q *db.Queries, a Actor, s ResourceSelector) ([]uuid.UUID, error) {
	return selectorResources(ctx, q, a, s, true)
}

// Reading a disabled policy still requires its entire configured resource scope.
// A stopped evaluator does not grant its creator an authorization bypass.
func selectorResources(ctx context.Context, q *db.Queries, a Actor, s ResourceSelector, evaluation bool) ([]uuid.UUID, error) {
	result := []uuid.UUID{}
	for _, kind := range s.Types {
		permission := "host.read"
		if kind == "kubernetes_cluster" {
			permission = "kubernetes.read"
		}
		if evaluation && dashboardaccess.Authorize(ctx, q, a, permission) != nil {
			continue
		}
		ids, err := dashboardaccess.AuthorizedIDs(ctx, q, a, kind)
		if !evaluation && a.SubjectType == "service_account" {
			ids, err = q.ListAlertConfiguredResourceIDs(ctx, db.ListAlertConfiguredResourceIDsParams{EnterpriseID: a.EnterpriseID, EvaluatorID: a.SubjectID, ResourceType: kind})
		}
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if s.Mode == "explicit" && !slices.Contains(s.IDs, id) {
				continue
			}
			labels := map[string]string{}
			if kind == "host" {
				row, e := q.GetHost(ctx, db.GetHostParams{ID: id, EnterpriseID: a.EnterpriseID})
				if e != nil {
					continue
				}
				_ = json.Unmarshal(row.Labels, &labels)
			} else {
				row, e := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: id, EnterpriseID: a.EnterpriseID})
				if e != nil {
					continue
				}
				_ = json.Unmarshal(row.Labels, &labels)
			}
			if matches(labels, s.Matchers) {
				result = append(result, id)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	result = slices.Compact(result)
	if s.Mode == "explicit" && !containsAll(result, s.IDs) {
		return nil, ErrDenied
	}
	return result, nil
}
func configurationResources(ctx context.Context, q *db.Queries, a Actor, row db.AlertConfiguration) ([]uuid.UUID, error) {
	switch row.Kind {
	case "policy":
		p, err := decodeSpec[PolicySpec](row.Spec)
		if err != nil {
			return nil, err
		}
		if p.Resources.Mode == "explicit" {
			return p.Resources.IDs, nil
		}
		eval, err := configuredEvaluator(ctx, q, a.EnterpriseID)
		if err != nil {
			return nil, err
		}
		return selectorResources(ctx, q, eval, p.Resources, false)
	case "silence", "inhibition":
		p, err := decodeSpec[SilenceSpec](row.Spec)
		if row.Kind == "inhibition" {
			i, e := decodeSpec[InhibitionSpec](row.Spec)
			err = e
			p.ResourceIDs = i.ResourceIDs
		}
		if err != nil {
			return nil, err
		}
		if len(p.ResourceIDs) > 0 {
			return p.ResourceIDs, nil
		}
		eval, err := evaluator(ctx, q, a.EnterpriseID)
		if err != nil {
			return nil, err
		}
		return actorResources(ctx, q, eval)
	case "route", "template", "grouping":
		return globalConfigurationScope(ctx, q, a)
	default:
		return []uuid.UUID{}, nil
	}
}

func globalConfigurationScope(ctx context.Context, q *db.Queries, a Actor) ([]uuid.UUID, error) {
	eval, e := evaluator(ctx, q, a.EnterpriseID)
	if e != nil {
		return nil, e
	}
	ids, e := actorResources(ctx, q, eval)
	if e != nil {
		return nil, e
	}
	allowed, e := actorResources(ctx, q, a)
	if e != nil || !containsAll(allowed, ids) {
		return nil, ErrDenied
	}
	return ids, nil
}
