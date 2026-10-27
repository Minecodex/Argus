package connector

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (gateway *Gateway) pinnedLogCommand(ctx context.Context, command db.ConnectorCommand) (*connectorv1.KubernetesPinnedPodLogs, error) {
	var request connectorv1.KubernetesPinnedPodLogs
	var query kubernetespodlogs.Query
	if json.Unmarshal(command.Payload, &request) != nil || request.SchemaVersion != kubernetespodlogs.SchemaVersion || len(request.QueryJson) > 16<<10 || json.Unmarshal(request.QueryJson, &query) != nil {
		return nil, ErrCommandState
	}
	cluster, err := uuid.Parse(request.ClusterId)
	if err != nil || command.OperationRef != cluster.String() {
		return nil, ErrCommandState
	}
	id, err := uuid.Parse(request.SessionId)
	if err != nil {
		return nil, ErrCommandState
	}
	if err = kubernetesworkbench.ValidateLogDelivery(ctx, gateway.Service.Store.Queries, command.EnterpriseID, cluster, id, query); err != nil {
		return nil, err
	}
	return &request, nil
}
