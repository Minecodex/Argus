package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/artifacthttp"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/sshtarget"
)

const (
	commandTimeout             = 45 * time.Second
	collectorManagementTimeout = 3 * time.Minute
	hostRemovalTimeout         = 5 * time.Minute
)

type commandOutcome struct {
	result *anypb.Any
	code   string
	detail string
	stage  string
	stop   bool
}

type commandExecutor struct{}

func (commandExecutor) execute(parent context.Context, command *connectorv1.ConnectorCommand, credential, operationSecret []byte) commandOutcome {
	ctx, cancel := context.WithTimeout(parent, timeoutForCommand(command.GetCommandType()))
	defer cancel()
	if command.GetTypedPayload() == nil || command.GetCommandId() == "" || command.GetExpiresAt() == nil || time.Now().After(command.GetExpiresAt().AsTime()) {
		return commandOutcome{code: "CONNECTOR_COMMAND_INVALID", detail: "missing, malformed, or expired command metadata"}
	}
	var value proto.Message
	var err error
	switch command.GetCommandType() {
	case "host_connection_probe":
		value, err = executeHostProbe(ctx, command.GetTypedPayload(), credential)
	case "kubernetes_connection_probe":
		value, err = executeKubernetesProbe(ctx, command.GetTypedPayload(), credential)
	case "kubernetes_resource_query":
		value, err = executeKubernetesQuery(ctx, command.GetTypedPayload(), credential)
	case "kubernetes_pod_logs":
		value, err = executeKubernetesLogs(ctx, command.GetTypedPayload(), credential)
	case "connector_uninstall":
		var request connectorv1.ConnectorUninstall
		if err = command.GetTypedPayload().UnmarshalTo(&request); err == nil {
			value = &connectorv1.ConnectorUninstallResult{IdentityRemoved: true, ServiceStopped: true}
		}
	case "collector_management":
		value, err = executeCollectorManagement(ctx, command.GetTypedPayload(), credential)
	case "host_connector_install":
		value, err = executeHostConnectorInstall(ctx, command.GetTypedPayload(), credential, operationSecret)
	case "host_connector_removal":
		value, err = executeHostConnectorRemoval(ctx, command.GetTypedPayload(), credential)
	case "host_windows_rdp_configure":
		var request connectorv1.HostWindowsRDPConfigure
		if err = command.GetTypedPayload().UnmarshalTo(&request); err == nil {
			value, err = configureWindowsRDP(ctx, &request)
		}
	default:
		err = errors.New("unsupported Connector command")
	}
	if err != nil {
		code := "CONNECTOR_COMMAND_FAILED"
		stage := ""
		if command.GetCommandType() == "host_connection_probe" {
			partial, _ := value.(*connectorv1.HostConnectionProbeResult)
			if partial == nil {
				partial = &connectorv1.HostConnectionProbeResult{}
			}
			typed, _ := anypb.New(partial)
			detail := err.Error()
			if callbackCode := sshtarget.CallbackFailureCode(err); callbackCode != "" {
				code = "HOST_ONBOARDING_CALLBACK_" + callbackCode
				detail = code
			}
			return commandOutcome{code: code, detail: detail, result: typed}
		}
		if command.GetCommandType() == "collector_management" {
			code = collectorManagementFailureCode(err)
			stage = collectorManagementFailureStage(err)
		} else if command.GetCommandType() == "host_connector_install" || command.GetCommandType() == "host_connector_removal" {
			stage = hostConnectorInstallFailureStage(err)
			if command.GetCommandType() == "host_connector_install" {
				if artifactCode := artifacthttp.FailureCode(err); artifactCode != "" {
					code = "HOST_ONBOARDING_ARTIFACT_" + artifactCode
				}
				if callbackCode := sshtarget.CallbackFailureCode(err); callbackCode != "" {
					code = "HOST_ONBOARDING_CALLBACK_" + callbackCode
				}
			}
		}
		return commandOutcome{code: code, detail: err.Error(), stage: stage}
	}
	typed, err := anypb.New(value)
	if err != nil {
		return commandOutcome{code: "CONNECTOR_RESULT_INVALID", detail: err.Error()}
	}
	return commandOutcome{result: typed, stop: command.GetCommandType() == "connector_uninstall"}
}

func timeoutForCommand(commandType string) time.Duration {
	if commandType == "collector_management" {
		return collectorManagementTimeout
	}
	if commandType == "host_connector_removal" {
		return hostRemovalTimeout
	}
	return commandTimeout
}

func executeHostProbe(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.HostConnectionProbeResult, error) {
	var request connectorv1.HostConnectionProbe
	if payload.UnmarshalTo(&request) != nil || request.Address == "" || request.Port == 0 || request.Username == "" || len(credential) == 0 {
		return nil, errors.New("invalid Host probe")
	}
	started := time.Now()
	resolved, err := resolveTarget(ctx, request.Address)
	if err != nil {
		return nil, err
	}
	result := &connectorv1.HostConnectionProbeResult{ResolvedIps: resolved}
	defer func() { result.LatencyMillis = uint64(time.Since(started).Milliseconds()) }()
	switch request.Protocol {
	case "ssh":
		auth, err := connectorSSHAuth(credential)
		if err != nil {
			return nil, err
		}
		configuration := &ssh.ClientConfig{User: request.Username, Auth: []ssh.AuthMethod{auth}, Timeout: 10 * time.Second,
			HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
				result.HostKeyFingerprint = ssh.FingerprintSHA256(key)
				if request.ExpectedHostKeyFingerprint != "" && request.ExpectedHostKeyFingerprint != result.HostKeyFingerprint {
					return errors.New("Host key changed")
				}
				return nil
			}}
		raw, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(request.Address, fmt.Sprint(request.Port)))
		if err != nil {
			return nil, err
		}
		defer raw.Close()
		stopOnCancel := context.AfterFunc(ctx, func() { _ = raw.Close() })
		defer stopOnCancel()
		clientConnection, channels, requests, err := ssh.NewClientConn(raw, net.JoinHostPort(request.Address, fmt.Sprint(request.Port)), configuration)
		if err != nil {
			return nil, err
		}
		connection := ssh.NewClient(clientConnection, channels, requests)
		defer connection.Close()
		evidence, err := sshtarget.Probe(connection, request.Platform)
		if err != nil {
			_ = connection.Close()
			return nil, err
		}
		result.Platform, result.Architecture = evidence.Platform, evidence.Architecture
		result.DistributionVersion, result.ServiceManager = evidence.DistributionVersion, evidence.ServiceManager
		result.Privileged, result.FreeDiskBytes = evidence.Privileged, evidence.FreeDiskBytes
		result.RemoteVersion = string(connection.ServerVersion())
		if frozen := request.GetOnboarding(); frozen != nil {
			if frozen.GetControlPath() == "executor_tunnel" {
				return result, &sshtarget.CallbackError{Code: "CONFIG_INVALID"}
			}
			plan := installation.CallbackProbePlan{ControlPath: frozen.GetControlPath(), EnrollmentEndpoint: frozen.GetEnrollmentEndpoint(), GatewayEndpoint: frozen.GetGatewayEndpoint(),
				EnrollDialAddress: frozen.GetEnrollDialAddress(), GatewayDialAddress: frozen.GetGatewayDialAddress(), TrustBundlePEM: frozen.GetTrustBundlePem(), TrustBundleEpoch: frozen.GetTrustBundleEpoch(), RelayPortGeneration: frozen.GetRelayPortGeneration()}
			if err := sshtarget.ProbeOnboarding(ctx, connection, plan); err != nil {
				return result, err
			}
			result.CallbackVerified, result.CallbackControlPath = true, plan.ControlPath
		}
	default:
		return nil, errors.New("unsupported Host probe protocol")
	}
	return result, nil
}

func executeKubernetesProbe(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.KubernetesConnectionProbeResult, error) {
	var request connectorv1.KubernetesConnectionProbe
	if payload.UnmarshalTo(&request) != nil || request.ApiServer == "" {
		return nil, errors.New("invalid Kubernetes probe")
	}
	config, err := connectorKubeconfig(credential, request.ApiServer)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return nil, err
	}
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return nil, err
	}
	ready := uint32(0)
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready++
				break
			}
		}
	}
	names := make([]string, 0, len(namespaces.Items))
	for _, namespace := range namespaces.Items {
		names = append(names, namespace.Name)
	}
	return &connectorv1.KubernetesConnectionProbeResult{ServerVersion: version.GitVersion, NodeCount: uint32(len(nodes.Items)),
		ReadyNodeCount: ready, Namespaces: names}, nil
}

func executeKubernetesQuery(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.KubernetesResourceQueryResult, error) {
	var request connectorv1.KubernetesResourceQuery
	if payload.UnmarshalTo(&request) != nil || request.ClusterId == "" || request.ResourceType == "" {
		return nil, errors.New("invalid Kubernetes query")
	}
	config, err := connectorKubeconfig(credential, "")
	if err != nil {
		return nil, err
	}
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	gvr, namespaced, ok := connectorResourceMapping(request.ResourceType)
	if !ok {
		return nil, errors.New("unsupported Kubernetes resource type")
	}
	var source dynamic.ResourceInterface = client.Resource(gvr)
	if namespaced {
		if request.Namespace == "" {
			return nil, errors.New("Kubernetes namespace is required")
		}
		source = client.Resource(gvr).Namespace(request.Namespace)
	}
	limit := int64(request.Limit)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	options := metav1.ListOptions{Limit: limit, Continue: request.ContinueToken, LabelSelector: request.LabelSelector}
	if request.Name != "" {
		options.FieldSelector = "metadata.name=" + request.Name
	}
	list, err := source.List(ctx, options)
	if err != nil {
		return nil, err
	}
	maxBytes := int(request.MaxResultBytes)
	if maxBytes <= 0 || maxBytes > 1<<20 {
		maxBytes = 1 << 20
	}
	result := &connectorv1.KubernetesResourceQueryResult{ContinueToken: list.GetContinue()}
	used := 0
	for _, item := range list.Items {
		encoded, err := json.Marshal(item.Object)
		if err != nil {
			return nil, err
		}
		if used+len(encoded) > maxBytes {
			result.Truncated = true
			break
		}
		result.ResourcesJson = append(result.ResourcesJson, encoded)
		used += len(encoded)
	}
	return result, nil
}

func executeKubernetesLogs(ctx context.Context, payload *anypb.Any, credential []byte) (*connectorv1.KubernetesPodLogsResult, error) {
	var request connectorv1.KubernetesPodLogsQuery
	if payload.UnmarshalTo(&request) != nil || request.ClusterId == "" || request.Namespace == "" || request.Pod == "" {
		return nil, errors.New("invalid Kubernetes logs query")
	}
	config, err := connectorKubeconfig(credential, "")
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	tail := int64(request.TailLines)
	options := &corev1.PodLogOptions{TailLines: &tail, Container: request.Container}
	stream, err := client.CoreV1().Pods(request.Namespace).GetLogs(request.Pod, options).Stream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	limit := int64(request.MaxResultBytes)
	if limit <= 0 || limit > 1<<20 {
		limit = 1 << 20
	}
	content, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil {
		return nil, err
	}
	truncated := int64(len(content)) > limit
	if truncated {
		content = content[:limit]
	}
	return &connectorv1.KubernetesPodLogsResult{Content: content, Truncated: truncated}, nil
}

func connectorKubeconfig(value []byte, apiServer string) (*rest.Config, error) {
	if len(value) == 0 {
		return rest.InClusterConfig()
	}
	config, err := clientcmd.RESTConfigFromKubeConfig(value)
	if err != nil || config.Insecure || config.ExecProvider != nil || config.AuthProvider != nil || config.Proxy != nil || config.WrapTransport != nil {
		return nil, errors.New("unsafe kubeconfig")
	}
	if apiServer != "" {
		parsed, err := url.Parse(apiServer)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("Kubernetes API server must use HTTPS")
		}
		config.Host = parsed.String()
	}
	config.Timeout = commandTimeout
	return config, nil
}

func connectorSSHAuth(value []byte) (ssh.AuthMethod, error) {
	if bytes.Contains(value, []byte("PRIVATE KEY")) {
		signer, err := ssh.ParsePrivateKey(value)
		if err != nil {
			return nil, err
		}
		return ssh.PublicKeys(signer), nil
	}
	return ssh.Password(string(value)), nil
}

func resolveTarget(ctx context.Context, hostname string) ([]string, error) {
	values, err := (&net.Resolver{}).LookupIPAddr(ctx, hostname)
	if err != nil || len(values) == 0 {
		return nil, errors.New("target DNS resolution failed")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.IP.String())
	}
	return result, nil
}

func rejectConnectorRedirect(_ *http.Request, _ []*http.Request) error {
	return errors.New("redirects are not allowed")
}

func connectorResourceMapping(resourceType string) (schema.GroupVersionResource, bool, bool) {
	values := map[string]struct {
		gvr        schema.GroupVersionResource
		namespaced bool
	}{
		"namespace":   {schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, false},
		"node":        {schema.GroupVersionResource{Version: "v1", Resource: "nodes"}, false},
		"pod":         {schema.GroupVersionResource{Version: "v1", Resource: "pods"}, true},
		"service":     {schema.GroupVersionResource{Version: "v1", Resource: "services"}, true},
		"deployment":  {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, true},
		"statefulset": {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, true},
		"daemonset":   {schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, true},
	}
	value, ok := values[strings.ToLower(resourceType)]
	return value.gvr, value.namespaced, ok
}
