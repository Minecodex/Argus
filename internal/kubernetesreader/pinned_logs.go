package kubernetesreader

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"k8s.io/client-go/kubernetes"
)

func (reader Reader) ReadPinnedPodLogs(ctx context.Context, cluster db.KubernetesCluster, id uuid.UUID, query kubernetespodlogs.Query) (kubernetespodlogs.Batch, error) {
	if id == uuid.Nil {
		return kubernetespodlogs.Batch{}, kubernetesinventory.ErrInvalid
	}
	if query.APIServer != cluster.ApiServer {
		return kubernetespodlogs.Batch{}, kubernetesinventory.ErrIdentityChanged
	}
	if err := query.Validate(); err != nil {
		return kubernetespodlogs.Batch{}, err
	}
	if cluster.ConnectionMode == "direct" {
		config, client, cleanup, err := reader.directConfig(ctx, cluster)
		if err != nil {
			return kubernetespodlogs.Batch{}, err
		}
		defer cleanup()
		api, err := kubernetes.NewForConfigAndClient(config, client)
		if err != nil {
			return kubernetespodlogs.Batch{}, kubernetesinventory.ErrUnavailable
		}
		return kubernetespodlogs.Read(ctx, api, query)
	}
	raw, _ := json.Marshal(query)
	request := &connectorv1.KubernetesPinnedPodLogs{SchemaVersion: kubernetespodlogs.SchemaVersion, ClusterId: cluster.ID.String(), SessionId: id.String(), QueryJson: raw}
	response := &connectorv1.KubernetesPinnedPodLogsResult{}
	if err := reader.connectorCommand(ctx, cluster, "kubernetes_pinned_pod_logs", request, response); err != nil {
		return kubernetespodlogs.Batch{}, err
	}
	if response.SchemaVersion != kubernetespodlogs.SchemaVersion || len(response.BatchJson) > 768<<10 {
		return kubernetespodlogs.Batch{}, kubernetesinventory.ErrProtocol
	}
	if err := kubernetespodlogs.ErrorFromCode(response.ErrorCode); err != nil {
		return kubernetespodlogs.Batch{}, err
	}
	var batch kubernetespodlogs.Batch
	if json.Unmarshal(response.BatchJson, &batch) != nil {
		return batch, kubernetesinventory.ErrProtocol
	}
	return batch, nil
}
