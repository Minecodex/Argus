package monitoring

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
)

type CollectorChoice struct {
	ID                  uuid.UUID   `json:"id"`
	Resource            ResourceRef `json:"resource"`
	Name                string      `json:"name"`
	Platform            string      `json:"platform"`
	Status              string      `json:"status"`
	Version             int64       `json:"version"`
	CollectorVersion    string      `json:"collector_version"`
	DistributionVersion string      `json:"distribution_version"`
	Components          []string    `json:"components"`
	SupportStatus       string      `json:"support_status"`
}

func (s Service) CollectorChoices(ctx context.Context, a Actor, in ListInput) (Page[CollectorChoice], error) {
	out := Page[CollectorChoice]{Items: []CollectorChoice{}}
	if s.Store == nil {
		return out, ErrUnavailable
	}
	q := s.Store.Queries
	if err := authorization.Authorize(ctx, q, a, "collector.use"); err != nil {
		return out, err
	}
	hosts, err := authorization.AuthorizedResourceIDs(ctx, q, a, "host")
	if err != nil {
		return out, err
	}
	clusters, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_cluster")
	if err != nil {
		return out, err
	}
	namespaceIDs, err := authorization.AuthorizedResourceIDs(ctx, q, a, "kubernetes_namespace")
	if err != nil {
		return out, err
	}
	namespaces, err := q.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: a.EnterpriseID, NamespaceIds: namespaceIDs, ClusterIds: []uuid.UUID{}})
	if err != nil {
		return out, err
	}
	for _, namespace := range namespaces {
		if !slices.Contains(clusters, namespace.ClusterID) {
			clusters = append(clusters, namespace.ClusterID)
		}
	}
	limit, after, binding, err := s.page(a, "collector_choices", in)
	if err != nil {
		return out, err
	}
	rows, err := q.ListMonitoringCollectorChoices(ctx, db.ListMonitoringCollectorChoicesParams{EnterpriseID: a.EnterpriseID, HostIds: hosts, ClusterIds: clusters, AfterID: after, Search: in.Search, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		out.NextCursor, err = s.next(binding, rows[limit-1].ID)
		if err != nil {
			return out, err
		}
		rows = rows[:limit]
	}
	for _, row := range rows {
		out.Items = append(out.Items, CollectorChoice{ID: row.ID, Resource: ResourceRef{Kind: row.ResourceType, ID: row.ResourceID}, Name: row.ResourceName, Platform: row.Platform, Status: row.Status, Version: row.Version, CollectorVersion: row.CollectorVersion, DistributionVersion: row.DistributionVersion, Components: row.Components, SupportStatus: row.SupportStatus})
	}
	return out, authorization.Authorize(ctx, q, a, "collector.use")
}
