package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/gen/openapi/hostapi"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"net/http"
	"time"
)

type HostReconnectActionService interface {
	HostReconnectInput(context.Context, uuid.UUID, uuid.UUID, int64) (resource.HostInput, error)
	PreviewHostReconnect(context.Context, resource.Subject, uuid.UUID, uuid.UUID, uuid.UUID, int64, string) (db.PendingAction, error)
}

func (handler HostHandler) PreviewHostReconnect(ctx context.Context, request hostapi.PreviewHostReconnectRequestObject) (hostapi.PreviewHostReconnectResponseObject, error) {
	p, failure := handler.auth(ctx, true, request.Params.XCSRFToken, "host.manage")
	if failure != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	if _, failure = handler.auth(ctx, false, "", "host.test"); failure != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: *failure, StatusCode: http.StatusForbidden}, nil
	}
	if request.Body == nil || handler.Reconnect == nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, resource.ErrActionUnavailable), StatusCode: http.StatusServiceUnavailable}, nil
	}
	id := uuid.UUID(request.Id)
	if _, err := handler.Service.GetHost(ctx, p.EnterpriseIDValue(), id, p.AuthorizedResourceIDs); err != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	input, err := handler.Reconnect.HostReconnectInput(ctx, p.EnterpriseIDValue(), id, request.Body.ExpectedVersion)
	if err != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	test, err := handler.Service.CreateHostConnectionTest(ctx, resourceSubject(p), p.EnterpriseIDValue(), input, request.Params.IdempotencyKey+":probe")
	if err != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for test.Status == "queued" || test.Status == "running" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, resource.ErrConnectionTestNeeded), StatusCode: http.StatusGatewayTimeout}, nil
		case <-ticker.C:
		}
		test, err = handler.Queries.GetConnectionTest(ctx, db.GetConnectionTestParams{ID: test.ID, EnterpriseID: p.EnterpriseIDValue()})
		if err != nil {
			return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
		}
	}
	action, err := handler.Reconnect.PreviewHostReconnect(ctx, resourceSubject(p), p.EnterpriseIDValue(), id, test.ID, request.Body.ExpectedVersion, request.Params.IdempotencyKey)
	if err != nil {
		return hostapi.PreviewHostReconnectdefaultJSONResponse{Body: hostError(ctx, err), StatusCode: resourceStatus(err)}, nil
	}
	return hostapi.PreviewHostReconnect201JSONResponse(pendingForHost(action)), nil
}
