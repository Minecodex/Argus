package connector

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func helmRequest(command db.ConnectorCommand) (*connectorv1.KubernetesHelmCommand, kuberneteshelm.Request, uuid.UUID, uuid.UUID, error) {
	var request connectorv1.KubernetesHelmCommand
	var input kuberneteshelm.Request
	if json.Unmarshal(command.Payload, &request) != nil || request.SchemaVersion != kuberneteshelm.SchemaVersion || len(request.PrivateMaterialJson) != 0 || kuberneteshelm.DecodeRequest(request.RequestJson, &input) != nil || input.OperationID != request.OperationId || input.SchemaVersion != kuberneteshelm.SchemaVersion {
		return nil, input, uuid.Nil, uuid.Nil, ErrCommandState
	}
	cluster, e := uuid.Parse(request.ClusterId)
	if e != nil || command.OperationRef != cluster.String() {
		return nil, input, uuid.Nil, uuid.Nil, ErrCommandState
	}
	id, e := uuid.Parse(request.OperationId)
	if e != nil {
		return nil, input, uuid.Nil, uuid.Nil, ErrCommandState
	}
	if request.Mode != "query" && request.Mode != "preview" && request.Mode != "apply" && request.Mode != "observe" {
		return nil, input, uuid.Nil, uuid.Nil, ErrCommandState
	}
	if request.Mode == "query" && (input.Operation != "list" && input.Operation != "history") {
		return nil, input, uuid.Nil, uuid.Nil, ErrCommandState
	}
	return &request, input, cluster, id, nil
}
func (gateway Gateway) helmCommand(ctx context.Context, command db.ConnectorCommand) (*connectorv1.KubernetesHelmCommand, error) {
	r, input, cluster, id, e := helmRequest(command)
	if e != nil {
		return nil, e
	}
	domain := kubernetesworkbench.Service{Store: gateway.Service.Store}
	if r.Mode == "apply" || r.Mode == "observe" {
		if e = domain.ValidateHelmDelivery(ctx, command.EnterpriseID, cluster, id, input, r.Mode == "apply"); e != nil {
			return nil, e
		}
	}
	if r.Mode != "query" {
		material, e := gateway.Credentials.LoadHelmInput(ctx, command.EnterpriseID, cluster, input, r.Mode == "preview", "connector", command.ConnectorID.String())
		if e != nil {
			return nil, e
		}
		defer material.Clear()
		r.PrivateMaterialJson, e = json.Marshal(material)
		if e != nil {
			return nil, ErrCommandState
		}
	}
	return r, nil
}

func (gateway Gateway) helmWriteGrant(ctx context.Context, identity TrustedIdentity, epoch int64, r *connectorv1.KubernetesHelmWriteRequest) *connectorv1.KubernetesHelmWriteGrant {
	grant := &connectorv1.KubernetesHelmWriteGrant{CommandId: r.CommandId, ConnectionEpoch: uint64(epoch), OperationId: r.OperationId, Nonce: r.Nonce, ExpiresAt: timestamppb.New(time.Now().UTC().Add(2 * time.Second))}
	if r.ConnectionEpoch != uint64(epoch) || len(r.Nonce) != 32 {
		return grant
	}
	q := gateway.Service.Store.Queries
	connector, e := q.GetConnector(ctx, db.GetConnectorParams{EnterpriseID: identity.EnterpriseID, ID: identity.ConnectorID})
	if e != nil || connector.Status != "online" || connector.ConnectionEpoch != epoch {
		return grant
	}
	command, e := q.GetConnectorCommand(ctx, db.GetConnectorCommandParams{CommandID: r.CommandId, ConnectorID: identity.ConnectorID, ConnectionEpoch: epoch})
	if e != nil || command.EnterpriseID != identity.EnterpriseID || command.CommandType != "kubernetes_helm" || command.Status != "running" || !time.Now().Before(command.ExpiresAt.Time) {
		return grant
	}
	wire, input, cluster, id, e := helmRequest(command)
	if e != nil || wire.Mode != "apply" || id.String() != r.OperationId {
		return grant
	}
	op, e := q.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: identity.EnterpriseID, ID: id})
	if e != nil || !op.ConnectorCommandID.Valid || op.ConnectorCommandID.UUID != command.ID {
		return grant
	}
	domain := kubernetesworkbench.Service{Store: gateway.Service.Store}
	e = domain.ValidateHelmWrite(ctx, identity.EnterpriseID, cluster, id, input, kuberneteshelm.Write{Group: r.Group, Version: r.Version, Resource: r.Resource, Namespace: r.Namespace, Name: r.Name, Verb: r.Verb})
	grant.Allowed = e == nil
	grant.ExpiresAt = timestamppb.New(time.Now().UTC().Add(2 * time.Second))
	return grant
}
