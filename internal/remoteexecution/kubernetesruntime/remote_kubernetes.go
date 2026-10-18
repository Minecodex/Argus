package kubernetesruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/runtimewire"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type kubernetesRuntimeBridge struct {
	options                                Options
	api                                    kubernetes.Interface
	config                                 *rest.Config
	wire                                   *runtimewire.Client
	namespace, pod, container, containerID string
	podUID                                 types.UID
	restarts                               int32
	owned                                  bool
	closed                                 atomic.Bool
	helperDirectory                        string
}

func openKubernetesRuntime(ctx context.Context, b *connectorv1.RemoteExecutionBatch, credential []byte) (*kubernetesRuntimeBridge, error) {
	if b == nil || b.ResourceType != "kubernetes_cluster" || b.KubernetesApiServer == "" {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	for _, id := range []string{b.ResourceId, b.SessionId, b.SessionGeneration, b.RequestId} {
		if _, err := uuid.Parse(id); err != nil {
			return nil, errors.New("REMOTE_BINDING_INVALID")
		}
	}
	config, err := connectorKubeconfig(credential, b.KubernetesApiServer)
	if err != nil || len(credential) > 0 && (config.CAFile != "" || config.CertFile != "" || config.KeyFile != "" || config.BearerTokenFile != "") || config.Impersonate.UserName != "" {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	return OpenWithConfig(ctx, b, config, environmentOptions())
}

func OpenWithConfig(ctx context.Context, b *connectorv1.RemoteExecutionBatch, config *rest.Config, options Options) (*Runtime, error) {
	if b == nil || config == nil || b.ResourceType != "kubernetes_cluster" {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	for _, id := range []string{b.ResourceId, b.SessionId, b.SessionGeneration, b.RequestId} {
		if _, err := uuid.Parse(id); err != nil {
			return nil, errors.New("REMOTE_BINDING_INVALID")
		}
	}
	config = rest.CopyConfig(config)
	if config.Insecure || config.ExecProvider != nil || config.AuthProvider != nil || config.Impersonate.UserName != "" {
		return nil, errors.New("REMOTE_IDENTITY_UNAVAILABLE")
	}
	server, err := url.Parse(b.KubernetesApiServer)
	if err != nil || server.Scheme != "https" || server.Host == "" || server.User != nil || server.RawQuery != "" || server.Fragment != "" {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	config.Host = server.String()
	config.Timeout = 30 * time.Second
	api, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.New("REMOTE_ROUTE_UNAVAILABLE")
	}
	bridge := &kubernetesRuntimeBridge{api: api, config: config, options: options}
	var pod *corev1.Pod
	helper := "/usr/local/bin/argus-remote-runtime"
	switch b.Interpreter {
	case "kubectl":
		if options.FilesOnly {
			pod, err = bridge.existingCommandPod(ctx, b)
		} else {
			pod, err = bridge.createCommandPod(ctx, b)
		}
		if err != nil {
			return nil, err
		}
		bridge.owned = true
		bridge.container = "commands"
	case "pod_bash":
		if b.Namespace == "" || b.Pod == "" || b.PodUid == "" || b.Container == "" {
			return nil, errors.New("REMOTE_BINDING_INVALID")
		}
		pod, err = api.CoreV1().Pods(b.Namespace).Get(ctx, b.Pod, metav1.GetOptions{})
		if err != nil || string(pod.UID) != b.PodUid {
			return nil, errors.New("REMOTE_POD_CHANGED")
		}
		bridge.container = b.Container
	default:
		return nil, errors.New("REMOTE_INTERPRETER_UNAVAILABLE")
	}
	bridge.namespace = pod.Namespace
	bridge.pod = pod.Name
	bridge.podUID = pod.UID
	opened := false
	defer func() {
		if !opened && bridge.owned && !options.FilesOnly {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = api.CoreV1().Pods(pod.Namespace).Delete(cleanup, pod.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &pod.UID}})
		}
	}()
	if err = bridge.pinContainer(pod); err != nil {
		return nil, err
	}
	if b.Interpreter == "kubectl" && !options.FilesOnly {
		if err = bridge.verifyIsolation(ctx, pod); err != nil {
			return nil, err
		}
	}
	if b.Interpreter == "pod_bash" {
		helper, err = bridge.installHelper(ctx, b)
		if err != nil {
			return nil, err
		}
	}
	args := []string{helper, "--kubernetes-resource-id", b.ResourceId, "--interpreter", b.Interpreter}
	if b.Interpreter == "pod_bash" {
		args = append(args, "--pod-uid", b.PodUid)
	}
	inputRead, inputWrite := io.Pipe()
	outputRead, outputWrite := io.Pipe()
	lifetime, cancel := context.WithCancel(context.Background())
	bridge.wire = runtimewire.NewClient(inputWrite, outputRead, func() { cancel(); inputRead.Close(); outputRead.Close() }, bridge.proxy)
	go func() {
		defer bridge.closed.Store(true)
		err := bridge.exec(lifetime, args, inputRead, outputWrite, io.Discard)
		_ = outputWrite.CloseWithError(err)
		bridge.wire.Close()
	}()
	opened = true
	return bridge, nil
}
func (b *kubernetesRuntimeBridge) pinContainer(pod *corev1.Pod) error {
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name == b.container && c.State.Running != nil && c.ContainerID != "" {
			b.containerID = c.ContainerID
			b.restarts = c.RestartCount
			return nil
		}
	}
	return errors.New("REMOTE_CONTAINER_UNAVAILABLE")
}
func (b *kubernetesRuntimeBridge) validate(ctx context.Context) error {
	pod, err := b.api.CoreV1().Pods(b.namespace).Get(ctx, b.pod, metav1.GetOptions{})
	if err != nil || pod.UID != b.podUID || pod.DeletionTimestamp != nil {
		return errors.New("REMOTE_POD_CHANGED")
	}
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name == b.container && c.ContainerID == b.containerID && c.RestartCount == b.restarts && c.State.Running != nil {
			return nil
		}
	}
	return errors.New("REMOTE_CONTEXT_LOST")
}
func (b *kubernetesRuntimeBridge) call(ctx context.Context, kind string, payload proto.Message) (proto.Message, error) {
	if b.options.FilesOnly {
		allowed := false
		switch p := payload.(type) {
		case *connectorv1.RemoteExecutionControl:
			allowed = p.Operation == "recover_files" || p.Operation == "reclaim" || p.Operation == "status"
		case *connectorv1.RemoteExecutionFile:
			allowed = p.Operation == "list_output" || p.Operation == "read_output"
		}
		if !allowed {
			return nil, errors.New("REMOTE_CONTEXT_LOST")
		}
	}
	if err := b.validate(ctx); err != nil {
		return nil, err
	}
	result, err := b.wire.Call(ctx, kind, payload)
	if err == nil {
		err = b.validate(ctx)
	}
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && control.Operation == "reclaim" && err == nil && b.owned {
		err = b.api.CoreV1().Pods(b.namespace).Delete(ctx, b.pod, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &b.podUID}})
		if err == nil {
			b.wire.Close()
		}
	}
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && control.Operation == "reclaim" && err == nil && !b.owned && b.helperDirectory != "" {
		err = b.removeHelper(ctx)
		if err == nil {
			b.Close()
		}
	}
	return result, err
}
func (b *kubernetesRuntimeBridge) exec(ctx context.Context, args []string, input io.Reader, output, stderr io.Writer) error {
	request := b.api.CoreV1().RESTClient().Post().Resource("pods").Name(b.pod).Namespace(b.namespace).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: b.container, Command: args, Stdin: input != nil, Stdout: true, Stderr: true, TTY: false}, scheme.ParameterCodec)
	config := rest.CopyConfig(b.config)
	config.Timeout = 0
	executor, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, request.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: input, Stdout: output, Stderr: stderr, Tty: false})
}
func (b *kubernetesRuntimeBridge) proxy(ctx context.Context, p runtimewire.ProxyRequest) runtimewire.ProxyResponse {
	failure := runtimewire.ProxyResponse{Status: http.StatusForbidden, Body: []byte("Kubernetes request rejected by the approved connection")}
	if b.options.FilesOnly {
		return failure
	}
	if err := b.validate(ctx); err != nil {
		return failure
	}
	if p.Method != "GET" && p.Method != "POST" && p.Method != "PUT" && p.Method != "PATCH" && p.Method != "DELETE" && p.Method != "HEAD" {
		return failure
	}
	uri, err := url.ParseRequestURI(p.URI)
	if err != nil || uri.IsAbs() || uri.Host != "" || !strings.HasPrefix(uri.Path, "/") || strings.HasPrefix(uri.Path, "//") || strings.Contains(uri.Path, "\\") {
		return failure
	}
	for key := range p.Header {
		if strings.HasPrefix(strings.ToLower(key), "impersonate-") || strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Upgrade") {
			return failure
		}
	}
	if uri.Query().Get("watch") == "true" {
		return runtimewire.ProxyResponse{Status: http.StatusBadRequest, Body: []byte("Use a bounded query instead of watch")}
	}
	base, err := url.Parse(b.config.Host)
	if err != nil {
		return failure
	}
	base.Path = strings.TrimRight(base.Path, "/") + uri.Path
	base.RawQuery = uri.RawQuery
	request, err := http.NewRequestWithContext(ctx, p.Method, base.String(), bytes.NewReader(p.Body))
	if err != nil {
		return failure
	}
	for _, name := range []string{"Accept", "Content-Type"} {
		request.Header.Set(name, p.Header.Get(name))
	}
	transport, err := rest.TransportFor(b.config)
	if err != nil {
		return failure
	}
	client := http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return runtimewire.ProxyResponse{Status: http.StatusBadGateway, Body: []byte("Kubernetes API unavailable")}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, runtimewire.MaxResponseBytes+1))
	if err != nil || len(body) > runtimewire.MaxResponseBytes {
		return runtimewire.ProxyResponse{Status: http.StatusBadGateway, Body: []byte("Kubernetes response exceeds transfer limit")}
	}
	return runtimewire.ProxyResponse{Status: response.StatusCode, Header: response.Header, Body: body}
}

func remoteNamespace() string {
	v := os.Getenv("ARGUS_REMOTE_NAMESPACE")
	if v == "" {
		v = os.Getenv("ARGUS_CONNECTOR_KUBERNETES_NAMESPACE")
	}
	return v
}
