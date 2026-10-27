package argusdev

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
)

var errPlan8ModelCleanup = errors.New("Plan8 failed model run cleanup incomplete")

// A failed fault subcase is retained as failed. Independent following stages
// may run only after real control convergence and a new host metric sample.
func (a *App) plan8RecoverHostLogStage(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	if host == nil {
		return errors.New("host log recovery has no enrolled host")
	}
	hostID, err := uuid.Parse(host.HostID)
	if err != nil {
		return err
	}
	collector, err := uuid.Parse(host.CollectorID)
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err = a.plan8WaitCollectorPaths(bounded, env); err != nil {
		return err
	}
	after := time.Now().UTC()
	_, err = a.plan8ServiceEffective(bounded, env, builtincollection.StableID("host", hostID, "host_metrics"), collector, true, &after)
	return err
}

// Stop only the exact Run created by this driver, through the authenticated
// production API. A timed-out scenario must not keep acting during the next one.
func (a *App) plan8StopFailedModelTask(ctx context.Context, env *E2EEnvironment, run string) error {
	if uuid.Validate(run) != nil {
		return errPlan8ModelCleanup
	}
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	client, err := scenarioHTTP(env)
	if err != nil {
		return errors.Join(errPlan8ModelCleanup, err)
	}
	state, err := client.PollJSON(bounded, "plan8-failed-model-state", "enterprise", "/runs/"+run, enterpriseHeaders(env, ""))
	if err != nil {
		return errors.Join(errPlan8ModelCleanup, err)
	}
	switch state["status"] {
	case "succeeded", "failed", "cancelled", "timed_out":
		return nil
	case "pending", "running", "waiting_input", "waiting_approval", "waiting_system":
		_, err = client.JSON(bounded, "plan8-failed-model-cancel", "enterprise", http.MethodPost, "/runs/"+run+"/cancel", http.StatusOK, nil, enterpriseHeaders(env, p5RequestKey("plan8-failed-model-cancel", run)))
		if err == nil {
			err = a.waitRunStatus(bounded, env, run, "cancelled", 15*time.Second)
		}
	default:
		err = fmt.Errorf("unexpected model Run state")
	}
	if err != nil {
		return errors.Join(errPlan8ModelCleanup, err)
	}
	return nil
}
