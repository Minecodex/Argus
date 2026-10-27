package connector

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"google.golang.org/protobuf/types/known/anypb"
	"k8s.io/client-go/kubernetes"
)

func executePinnedPodLogs(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.KubernetesPinnedPodLogsResult, error) {
	result := &connectorv1.KubernetesPinnedPodLogsResult{SchemaVersion: kubernetespodlogs.SchemaVersion}
	fail := func(err error) (*connectorv1.KubernetesPinnedPodLogsResult, error) {
		result.ErrorCode = kubernetespodlogs.ErrorCode(err)
		return result, nil
	}
	var request connectorv1.KubernetesPinnedPodLogs
	if payload == nil || payload.UnmarshalTo(&request) != nil || request.SchemaVersion != kubernetespodlogs.SchemaVersion {
		return fail(kubernetesinventory.ErrProtocol)
	}
	if _, err := uuid.Parse(request.ClusterId); err != nil {
		return fail(kubernetesinventory.ErrInvalid)
	}
	if _, err := uuid.Parse(request.SessionId); err != nil {
		return fail(kubernetesinventory.ErrInvalid)
	}
	var query kubernetespodlogs.Query
	if len(request.QueryJson) > 16<<10 || json.Unmarshal(request.QueryJson, &query) != nil {
		return fail(kubernetesinventory.ErrInvalid)
	}
	if err := query.Validate(); err != nil {
		return fail(err)
	}
	config, err := connectorKubeconfig(credential, query.APIServer)
	if err != nil {
		return fail(kubernetesinventory.ErrUnavailable)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fail(kubernetesinventory.ErrUnavailable)
	}
	batch, err := kubernetespodlogs.Read(ctx, client, query)
	if err != nil {
		return fail(err)
	}
	result.BatchJson, err = json.Marshal(batch)
	if err != nil {
		return fail(kubernetesinventory.ErrUnavailable)
	}
	if len(result.BatchJson) > 512<<10 {
		result.BatchJson = nil
		return fail(kubernetesinventory.ErrBudget)
	}
	return result, nil
}
