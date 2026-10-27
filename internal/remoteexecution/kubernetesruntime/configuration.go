package kubernetesruntime

import (
	"context"
	"errors"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/rest"
	"os"
)

type Runtime = kubernetesRuntimeBridge
type Options struct {
	Namespace, Image, HelperDirectory string
	FilesOnly                         bool
}

func Open(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*Runtime, error) {
	return openKubernetesRuntime(ctx, b, credential)
}
func Recover(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*Runtime, error) {
	config, err := connectorKubeconfig(credential, "")
	if err != nil {
		return nil, err
	}
	options := environmentOptions()
	options.FilesOnly = true
	return OpenWithConfig(ctx, b, config, options)
}
func (b *kubernetesRuntimeBridge) Closed() bool { return b.closed.Load() }
func (b *kubernetesRuntimeBridge) Call(ctx context.Context, kind string, payload proto.Message) (proto.Message, error) {
	return b.call(ctx, kind, payload)
}
func (b *kubernetesRuntimeBridge) Close() {
	b.closed.Store(true)
	if b.wire != nil {
		b.wire.Close()
	}
}
func connectorKubeconfig(value []byte, _ string) (*rest.Config, error) {
	config, err := kubernetesaccess.Config(value, "")
	if err != nil {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	return config, nil
}

func environmentOptions() Options {
	return Options{Namespace: remoteNamespace(), Image: os.Getenv("ARGUS_REMOTE_KUBECTL_IMAGE"), HelperDirectory: os.Getenv("ARGUS_REMOTE_HELPER_DIRECTORY")}
}
