package argusdev

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesstreamclient"
	"github.com/kakj-go/Argus/internal/kubernetesstreamgateway"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	corev1 "k8s.io/api/core/v1"
)

func plan8StreamURL(env *E2EEnvironment, ticket kubernetesworkbench.StreamTicket) error {
	destination, err := url.Parse(ticket.WebsocketURL)
	origin, originErr := url.Parse(env.EnterpriseOrigin())
	if err != nil || originErr != nil || destination.Scheme != "wss" || destination.Host != origin.Host || destination.User != nil || destination.RawQuery != "" || destination.Fragment != "" || destination.Path != "/v1/kubernetes/sessions/"+ticket.Session.ID.String() || ticket.Protocol != kubernetesstream.WebSocketProtocol || len(ticket.Ticket) != 43 {
		return errors.New("Kubernetes stream ticket has an untrusted destination or protocol")
	}
	return nil
}

func (a *App) plan8KubernetesStreamTicket(ctx context.Context, env *E2EEnvironment, cluster string, input kubernetesworkbench.StreamInput) (kubernetesworkbench.StreamTicket, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return kubernetesworkbench.StreamTicket{}, err
	}
	value, err := client.JSON(ctx, "plan8-k8s-stream-create-"+input.Mode, "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/"+cluster+"/stream-sessions", 201, input, enterpriseHeaders(env, ""))
	if err != nil {
		return kubernetesworkbench.StreamTicket{}, err
	}
	raw, _ := json.Marshal(value)
	var ticket kubernetesworkbench.StreamTicket
	if json.Unmarshal(raw, &ticket) != nil {
		return ticket, errors.New("invalid Kubernetes stream ticket")
	}
	return ticket, plan8StreamURL(env, ticket)
}

func (a *App) plan8KubernetesExec(ctx context.Context, env *E2EEnvironment, cluster string, namespace uuid.UUID, pod corev1.Pod) (uuid.UUID, error) {
	ticket, err := a.plan8KubernetesStreamTicket(ctx, env, cluster, kubernetesworkbench.StreamInput{NamespaceID: namespace, Type: "pod", Name: pod.Name, UID: string(pod.UID), Container: "http", Mode: "exec", Shell: "sh", Reason: "Plan8 isolated actual terminal acceptance"})
	if err != nil {
		return uuid.Nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(bounded, ticket.WebsocketURL, &websocket.DialOptions{HTTPClient: &http.Client{Transport: env.Endpoints.Transport()}, HTTPHeader: http.Header{"Origin": []string{env.EnterpriseOrigin()}}})
	if err != nil {
		return uuid.Nil, errors.New("Kubernetes terminal connection failed")
	}
	defer ws.CloseNow()
	hello, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "client_hello", Sequence: 1, Ticket: ticket.Ticket, Cols: 100, Rows: 30})
	if err = ws.Write(bounded, websocket.MessageText, hello); err != nil {
		clear(hello)
		return uuid.Nil, err
	}
	clear(hello)
	ticket.Ticket = ""
	kind, raw, err := ws.Read(bounded)
	var ready kubernetesstreamgateway.ServerFrame
	if err != nil || kind != websocket.MessageText || json.Unmarshal(raw, &ready) != nil || ready.Type != "server_ready" || ready.Mode != "exec" || ready.Nonce == "" {
		return uuid.Nil, errors.New("Kubernetes terminal did not become active")
	}
	command, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "input", Sequence: 2, Nonce: ready.Nonce, Data: "printf 'plan8-exec-%s\\n' 'actual-result'\n"})
	if err = ws.Write(bounded, websocket.MessageText, command); err != nil {
		return uuid.Nil, err
	}
	output := ""
	for !strings.Contains(output, "plan8-exec-actual-result") {
		kind, raw, err = ws.Read(bounded)
		if err != nil {
			return uuid.Nil, errors.New("actual terminal command produced no verified output")
		}
		var frame kubernetesstreamgateway.ServerFrame
		if kind != websocket.MessageText || json.Unmarshal(raw, &frame) != nil || frame.Type != "output" {
			return uuid.Nil, errors.New("actual terminal ended before verification")
		}
		decoded, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			return uuid.Nil, err
		}
		output += string(decoded)
		if len(output) > 64<<10 {
			return uuid.Nil, errors.New("actual terminal output exceeded its bounded acceptance window")
		}
	}
	closeFrame, _ := json.Marshal(interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "close", Sequence: 3, Nonce: ready.Nonce})
	if err = ws.Write(bounded, websocket.MessageText, closeFrame); err != nil {
		return uuid.Nil, err
	}
	for {
		_, raw, err = ws.Read(bounded)
		if err != nil {
			return uuid.Nil, errors.New("actual terminal close was not acknowledged")
		}
		var frame kubernetesstreamgateway.ServerFrame
		if json.Unmarshal(raw, &frame) == nil && frame.Type == "session_state" {
			if frame.Status != "closed" {
				return uuid.Nil, errors.New("terminal did not close normally")
			}
			break
		}
	}
	return ticket.Session.ID, nil
}

func (a *App) plan8KubernetesForward(ctx context.Context, env *E2EEnvironment, cluster string, namespace uuid.UUID, pod corev1.Pod) (uuid.UUID, error) {
	return a.plan8KubernetesForwardPath(ctx, env, cluster, namespace, pod, "/plan8-forward-probe")
}
func (a *App) plan8KubernetesForwardPath(ctx context.Context, env *E2EEnvironment, cluster string, namespace uuid.UUID, pod corev1.Pod, path string) (uuid.UUID, error) {
	ticket, err := a.plan8KubernetesStreamTicket(ctx, env, cluster, kubernetesworkbench.StreamInput{NamespaceID: namespace, Type: "pod", Name: pod.Name, UID: string(pod.UID), Port: 8080, Mode: "port_forward", Reason: "Plan8 isolated actual HTTP forwarding acceptance"})
	if err != nil {
		return uuid.Nil, err
	}
	listener, err := kubernetesstreamclient.Listen("127.0.0.1:0")
	if err != nil {
		return uuid.Nil, err
	}
	defer listener.Close()
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- kubernetesstreamclient.Forward(bounded, listener, kubernetesstreamclient.Options{URL: ticket.WebsocketURL, Ticket: ticket.Ticket, HTTPClient: &http.Client{Transport: env.Endpoints.Transport()}, Ready: func() { close(ready) }})
	}()
	select {
	case <-ready:
	case <-done:
		return uuid.Nil, errors.New("actual forwarding session did not become ready")
	case <-bounded.Done():
		return uuid.Nil, bounded.Err()
	}
	request, err := http.NewRequestWithContext(bounded, http.MethodGet, "http://"+listener.Addr().String()+path, nil)
	if err != nil {
		return uuid.Nil, err
	}
	response, err := (&http.Client{Transport: &http.Transport{}, Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return uuid.Nil, errors.New("forwarded HTTP request failed")
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != 200 || string(body) != "plan8-forward-actual-result" {
		return uuid.Nil, errors.New("forwarded bytes did not come from the owned target")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err = client.JSON(ctx, "plan8-k8s-forward-close", "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/"+cluster+"/stream-sessions/"+ticket.Session.ID.String()+"/close", 200, nil, enterpriseHeaders(env, "")); err != nil {
		return uuid.Nil, err
	}
	select {
	case <-done:
	case <-bounded.Done():
		return uuid.Nil, errors.New("closed forward retained an active client")
	}
	return ticket.Session.ID, nil
}

func (a *App) plan8KubernetesPodLogs(ctx context.Context, env *E2EEnvironment, cluster string, namespace uuid.UUID, pod corev1.Pod) (uuid.UUID, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return uuid.Nil, err
	}
	base := "/enterprise/kubernetes-clusters/" + cluster + "/log-sessions"
	value, err := client.JSON(ctx, "plan8-k8s-log-create", "enterprise", http.MethodPost, base, 201, kubernetesworkbench.LogSessionInput{NamespaceID: namespace, Pod: pod.Name, PodUID: string(pod.UID), Container: "http", TailLines: 100, SinceSeconds: 300}, enterpriseHeaders(env, ""))
	if err != nil {
		return uuid.Nil, err
	}
	raw, _ := json.Marshal(value)
	var ticket kubernetesworkbench.LogTicket
	if json.Unmarshal(raw, &ticket) != nil || ticket.Session.PodUID != string(pod.UID) {
		return uuid.Nil, errors.New("log ticket lost the actual Pod identity")
	}
	base += "/" + ticket.Session.ID.String()
	if _, err = client.JSON(ctx, "plan8-k8s-log-attach", "enterprise", http.MethodPost, base+"/attach", 200, map[string]string{"ticket": ticket.Ticket}, enterpriseHeaders(env, "")); err != nil {
		return uuid.Nil, err
	}
	ticket.Ticket = ""
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cursor := ""
	observed := ""
	for !strings.Contains(observed, "GET /plan8-forward-probe") {
		body := map[string]any{}
		if cursor != "" {
			body["cursor"] = cursor
		}
		value, err = client.JSON(bounded, "plan8-k8s-log-read", "enterprise", http.MethodPost, base+"/read", 200, body, enterpriseHeaders(env, ""))
		if err != nil {
			return uuid.Nil, err
		}
		raw, _ = json.Marshal(value)
		var batch kubernetesworkbench.LogBatch
		if json.Unmarshal(raw, &batch) != nil {
			return uuid.Nil, errors.New("invalid log batch")
		}
		observed += batch.Content
		cursor = batch.NextCursor
		if len(observed) > 64<<10 {
			return uuid.Nil, errors.New("owned Pod log acceptance exceeded its bounded window")
		}
		if err = waitContext(bounded, 250*time.Millisecond); err != nil {
			return uuid.Nil, errors.New("owned HTTP access log was not observed")
		}
	}
	if _, err = client.JSON(ctx, "plan8-k8s-log-close", "enterprise", http.MethodPost, base+"/close", 200, nil, enterpriseHeaders(env, "")); err != nil {
		return uuid.Nil, err
	}
	return ticket.Session.ID, nil
}

func (a *App) plan8KubernetesSessions(ctx context.Context, env *E2EEnvironment, cluster string, namespace kubernetesworkbench.Namespace, pod corev1.Pod) error {
	execID, err := a.plan8KubernetesExec(ctx, env, cluster, namespace.ID, pod)
	if err != nil {
		return err
	}
	forwardID, err := a.plan8KubernetesForward(ctx, env, cluster, namespace.ID, pod)
	if err != nil {
		return err
	}
	logsID, err := a.plan8KubernetesPodLogs(ctx, env, cluster, namespace.ID, pod)
	if err != nil {
		return err
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "verification_level": "enrolled_connector_gateway_actual_pod_sessions", "cluster_id": cluster, "namespace_id": namespace.ID, "namespace_uid": namespace.UID, "pod": pod.Name, "pod_uid": pod.UID, "exec_session_id": execID, "forward_session_id": forwardID, "log_session_id": logsID, "terminal_output_verified": true, "forwarded_http_bytes_verified": true, "pod_access_log_verified": true}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-kubernetes-sessions.json"), proof)
}
