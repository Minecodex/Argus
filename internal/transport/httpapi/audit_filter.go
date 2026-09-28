package httpapi

import auditapi "github.com/kakj-go/Argus/internal/gen/openapi/audit"

// Filters are applied to the authorized domain before paging; every condition
// participates in the signed cursor so a next page cannot change its scope.
func auditFilters(p auditapi.ListAuditEventsParams) map[string]any {
	return map[string]any{"action": p.Action, "actor_id": p.ActorId, "resource_type": p.ResourceType, "resource_id": p.ResourceId, "result": p.Result, "query": p.Query, "from": p.From, "to": p.To}
}
