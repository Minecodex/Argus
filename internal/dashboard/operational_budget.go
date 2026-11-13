package dashboard

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"time"
)

func (r Runtime) executeOperational(ctx context.Context, a Actor, p Panel, result Execution, round *operationalRound) (TargetExecution, error) {
	meter := runMeter{store: r.Store, actor: a}
	var ticket budgetTicket
	if a.RunID != uuid.Nil {
		var err error
		ticket, err = meter.reserve(ctx, budgetCost{Bytes: 4 << 20, Rows: 2000}, 20*time.Second, false)
		if err != nil {
			return TargetExecution{ID: "resources", Status: "skipped_budget", Code: "QUERY_BUDGET_EXCEEDED"}, nil
		}
	}
	value, err := r.executeOperationalData(ctx, a, p, result, round)
	if err != nil {
		if a.RunID != uuid.Nil {
			_ = meter.settle(ctx, ticket, nil)
		}
		return value, err
	}
	encoded, _ := json.Marshal(value.Data)
	cost := budgetCost{Bytes: int64(len(encoded)), Rows: operationalRows(value.Data)}
	round.bytes += cost.Bytes
	if round.bytes > 8<<20 || cost.Bytes > 4<<20 || a.RunID != uuid.Nil && exceeds(cost, ticket.cost) {
		if a.RunID != uuid.Nil {
			_ = meter.settle(ctx, ticket, nil)
		}
		return TargetExecution{ID: "resources", Status: "skipped_budget", Code: "QUERY_BUDGET_EXCEEDED"}, nil
	}
	if a.RunID != uuid.Nil {
		if err = meter.settle(ctx, ticket, &cost); err != nil {
			return TargetExecution{}, err
		}
	}
	return value, nil
}

func operationalRows(data any) int64 {
	switch v := data.(type) {
	case kubernetesworkbench.Snapshot:
		return int64(len(v.Resources))
	case map[string]any:
		if items, ok := v["items"].([]map[string]any); ok {
			return int64(len(items))
		}
	}
	return 0
}
