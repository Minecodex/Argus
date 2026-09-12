package resource

import (
	"context"
	"reflect"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// OnboardingProbePlanner builds the authoritative callback route without
// widening the lifecycle extension required by unrelated resource consumers.
type OnboardingProbePlanner interface {
	PlanHostOnboardingProbe(context.Context, *db.Queries, uuid.UUID, string, string, uuid.NullUUID) (*installation.CallbackProbePlan, error)
}

func (service Service) validateOnboardingProbe(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, input HostInput, plan *installation.CallbackProbePlan, result ConnectionTestResult) error {
	if service.OnboardingProbes == nil || plan == nil || !result.CallbackVerified || result.CallbackControlPath != input.ControlPath {
		return ErrConnectionTestNeeded
	}
	current, err := service.OnboardingProbes.PlanHostOnboardingProbe(ctx, q, enterpriseID, input.ControlPath, input.SSHPath, input.BastionScopeID)
	if err != nil || !reflect.DeepEqual(current, plan) {
		return ErrConnectionTestNeeded
	}
	return nil
}
