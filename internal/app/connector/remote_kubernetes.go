package connector

import (
	"context"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/kubernetesruntime"
)

type kubernetesRuntimeBridge = kubernetesruntime.Runtime

func openKubernetesRuntime(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*kubernetesRuntimeBridge, error) {
	return kubernetesruntime.Open(ctx, b, credential)
}
func recoverKubernetesRuntime(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*kubernetesRuntimeBridge, error) {
	return kubernetesruntime.Recover(ctx, b, credential)
}
