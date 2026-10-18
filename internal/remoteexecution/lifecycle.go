package remoteexecution

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func ConfiguredIdleTimeout() time.Duration {
	n, err := strconv.Atoi(os.Getenv("ARGUS_REMOTE_IDLE_SECONDS"))
	if err != nil || n < 60 || n > 7*24*3600 {
		return DefaultIdleTimeout
	}
	return time.Duration(n) * time.Second
}
func (s Service) RunReconciler(ctx context.Context, logger *slog.Logger) error {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			if err := s.ReconcileLateReceipts(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("Remote receipt reconciliation deferred", "error", err)
			}
			if err := s.ReclaimIdle(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("Remote session reclamation deferred", "error", err)
			}
		}
	}
}
func (s Service) ReclaimIdle(ctx context.Context) error {
	timeout := s.IdleTimeout
	if timeout <= 0 {
		timeout = ConfiguredIdleTimeout()
	}
	sessions, err := s.Store.Queries.ListIdleRemoteGenerations(ctx, timeValue(time.Now().Add(-timeout)))
	if err != nil {
		return err
	}
	for _, session := range sessions {
		request, err := s.Store.Queries.GetLastRemoteGenerationRequest(ctx, db.GetLastRemoteGenerationRequestParams{SessionID: session.SessionID, Generation: session.Generation, EnterpriseID: session.EnterpriseID})
		if errors.Is(err, pgx.ErrNoRows) {
			if e := retryStateTx(ctx, s.Store, func(q *db.Queries) error { _, e := reclaimGeneration(ctx, q, session); return e }); e != nil {
				return e
			}
			continue
		}
		if err != nil {
			return err
		}
		var spec Spec
		if json.Unmarshal(request.Spec, &spec) != nil {
			return Error("REMOTE_PLAN_INVALID")
		}
		identity, err := s.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: session.IdentityID, EnterpriseID: session.EnterpriseID})
		if err != nil {
			continue
		}
		for n, step := range spec.Steps {
			if step.SessionID != session.SessionID || step.Generation != session.Generation {
				continue
			}
			binding := batchBinding(request, step, n+1, session.ContextRevision)
			if s.Transport == nil {
				break
			}
			encoded, e := protojson.Marshal(binding)
			if e != nil {
				return e
			}
			payload, _ := json.Marshal(Task{Operation: "reclaim", EnterpriseID: request.EnterpriseID, RequestID: request.ID, IdentityID: identity.ID, Binding: encoded})
			err = retryStateTx(ctx, s.Store, func(q *db.Queries) error {
				n, e := reclaimGeneration(ctx, q, session)
				if e != nil {
					return e
				}
				if n != 1 {
					return nil
				}
				_, e = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.NewSHA1(session.SessionID, []byte("reclaim/"+session.Generation.String())), EnterpriseID: uuid.NullUUID{UUID: request.EnterpriseID, Valid: true}, Queue: Queue, Payload: payload, MaxAttempts: 20, AvailableAt: timeValue(time.Now())})
				return e
			})
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func reclaimGeneration(ctx context.Context, q *db.Queries, g db.ListIdleRemoteGenerationsRow) (int64, error) {
	current, err := q.LockRemoteSession(ctx, db.LockRemoteSessionParams{ID: g.SessionID, EnterpriseID: g.EnterpriseID})
	if err != nil {
		return 0, err
	}
	n, err := q.ReclaimRemoteGeneration(ctx, db.ReclaimRemoteGenerationParams{SessionID: g.SessionID, Generation: g.Generation, EnterpriseID: g.EnterpriseID, LastActiveAt: g.LastActiveAt})
	if err != nil || n != 1 {
		return n, err
	}
	if current.Generation == g.Generation {
		_, err = q.MarkRemoteSessionReclaimed(ctx, db.MarkRemoteSessionReclaimedParams{ID: current.ID, EnterpriseID: g.EnterpriseID, Generation: g.Generation, LastActiveAt: current.LastActiveAt})
	}
	return n, err
}

func (s Service) handleReclamation(ctx context.Context, task Task) error {
	var binding connectorv1.RemoteExecutionBatch
	if protojson.Unmarshal(task.Binding, &binding) != nil || binding.RequestId != task.RequestID.String() {
		return Error("REMOTE_BINDING_INVALID")
	}
	identity, err := s.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: task.IdentityID, EnterpriseID: task.EnterpriseID})
	if err != nil {
		return err
	}
	if s.Transport == nil {
		return Error("REMOTE_ROUTE_UNAVAILABLE")
	}
	raw, err := s.Transport.Call(ctx, identity, "remote_execution_control", &connectorv1.RemoteExecutionControl{Operation: "reclaim", SessionId: binding.SessionId, SessionGeneration: binding.SessionGeneration, RequestId: binding.RequestId, Binding: &binding}, "reclaim/"+binding.SessionId+"/"+binding.SessionGeneration, 30*time.Second, nil)
	if err != nil {
		return err
	}
	var receipt connectorv1.RemoteExecutionControlResult
	if protojson.Unmarshal(raw, &receipt) != nil || receipt.SessionId != binding.SessionId || receipt.SessionGeneration != binding.SessionGeneration || receipt.Status != "reclaimed" {
		return Error("REMOTE_COMPLETION_INVALID")
	}
	return nil
}
