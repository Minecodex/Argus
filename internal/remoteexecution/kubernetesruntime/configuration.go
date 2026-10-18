package kubernetesruntime

import (
	"context"
	"errors"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	if len(value) == 0 {
		return rest.InClusterConfig()
	}
	config, err := clientcmd.RESTConfigFromKubeConfig(value)
	if err != nil || config.Insecure || config.ExecProvider != nil || config.AuthProvider != nil || config.Proxy != nil || config.WrapTransport != nil || config.Impersonate.UserName != "" || config.CAFile != "" || config.CertFile != "" || config.KeyFile != "" || config.BearerTokenFile != "" {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	return config, nil
}
func environmentOptions() Options {
	return Options{Namespace: remoteNamespace(), Image: os.Getenv("ARGUS_REMOTE_KUBECTL_IMAGE"), HelperDirectory: os.Getenv("ARGUS_REMOTE_HELPER_DIRECTORY")}
}
