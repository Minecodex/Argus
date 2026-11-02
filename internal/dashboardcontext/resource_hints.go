package dashboardcontext

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardaccess"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Resource hints are authoritative identities, not query or execution grants.
// Resolve their current names after authorization; never trust a composer label.
func resourceHintFacts(ctx context.Context, q *db.Queries, p toolruntime.Principal, refs []toolruntime.ResourceRef) ([]map[string]any, error) {
	out := []map[string]any{}
	if len(refs) > 20 {
		return nil, invalid()
	}
	a := dashboardaccess.Actor{EnterpriseID: p.EnterpriseID, SubjectID: p.UserID, SubjectType: "user", AuthorizationVersion: p.AuthorizationVersion}
	seen := map[string]bool{}
	for _, ref := range refs {
		id, err := uuid.Parse(ref.ID)
		if err != nil || id == uuid.Nil || seen[ref.Type+"/"+ref.ID] {
			return nil, invalid()
		}
		seen[ref.Type+"/"+ref.ID] = true
		name := ""
		switch ref.Type {
		case "dashboard":
			if err = dashboardaccess.Authorize(ctx, q, a, "telemetry.dashboard.read"); err != nil {
				return nil, err
			}
			if err = dashboardaccess.RequireObject(ctx, q, a, "dashboard", id); err != nil {
				return nil, err
			}
			row, e := q.GetDashboard(ctx, db.GetDashboardParams{EnterpriseID: p.EnterpriseID, ID: id})
			if e != nil {
				return nil, e
			}
			name = row.Name
		case "connector":
			if err = dashboardaccess.Authorize(ctx, q, a, "connector.read"); err != nil {
				return nil, err
			}
			row, e := q.GetConnector(ctx, db.GetConnectorParams{EnterpriseID: p.EnterpriseID, ID: id})
			if e != nil {
				return nil, e
			}
			name = row.Name
		default:
			if !resourceref.ValidType(ref.Type) {
				return nil, invalid()
			}
			snapshot, e := (observationscope.Resolver{Queries: q}).Resolve(ctx, a, observationscope.Selection{Mode: observationscope.Explicit, IDs: []uuid.UUID{id}, Types: []string{ref.Type}, Signal: "catalog"})
			if e != nil {
				return nil, e
			}
			if len(snapshot.Resources) != 1 {
				return nil, dashboardaccess.ErrDenied
			}
			name = snapshot.Resources[0].Name
		}
		out = append(out, map[string]any{"type": ref.Type, "id": id, "name": name})
	}
	return out, nil
}
