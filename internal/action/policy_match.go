package action

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func approvalPolicyQuery(enterpriseID uuid.UUID, action db.PendingAction, plan db.PendingActionPlan) db.ListMatchingApprovalPoliciesParams {
	return db.ListMatchingApprovalPoliciesParams{
		EnterpriseID: enterpriseID,
		Column2:      action.Risk,
		Column3:      plan.CommitTool,
		Column4:      action.ResourceType,
	}
}

func matchingApprovalPolicies(ctx context.Context, q *db.Queries, action db.PendingAction, plan db.PendingActionPlan, candidates []db.ApprovalPolicy) ([]db.ApprovalPolicy, error) {
	if action.ActionType == "remote_execution.execute" {
		var spec struct {
			Kind  string `json:"kind"`
			Steps []struct {
				Target struct {
					Type string `json:"resource_type"`
				} `json:"target"`
			} `json:"steps"`
		}
		if json.Unmarshal(plan.ImmutablePlan, &spec) != nil {
			return nil, ErrInvalidated
		}
		kinds := []string{}
		for _, step := range spec.Steps {
			kinds = append(kinds, step.Target.Type)
		}
		tools := []string{plan.CommitTool}
		if spec.Kind == "connect" {
			tools = append(tools, "connect")
		} else {
			tools = append(tools, "workflow.remote_plan.preview")
		}
		return q.ListRemoteMatchingApprovalPolicies(ctx, db.ListRemoteMatchingApprovalPoliciesParams{EnterpriseID: action.EnterpriseID, Risk: action.Risk, Tools: tools, ResourceTypes: kinds})
	}
	return candidates, nil
}
