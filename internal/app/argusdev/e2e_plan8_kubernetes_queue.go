package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/collectormanager"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type plan8GatewayQueueObservation struct {
	AcceptedLogs     float64            `json:"accepted_logs"`
	QueuedRequests   float64            `json:"queued_requests"`
	AcceptedObserved bool               `json:"accepted_observed"`
	QueueObserved    bool               `json:"queue_observed"`
	QueuesBySignal   map[string]float64 `json:"queues_by_signal"`
	SentLogs         float64            `json:"sent_logs"`
	FailedLogs       float64            `json:"failed_logs"`
}

var errPlan8KubernetesQueueRecovery = errors.New("Plan8 Kubernetes pending queue recovery gate failed")

func plan8ReadGatewayQueue(ctx context.Context, forward *KubeForward) (plan8GatewayQueueObservation, error) {
	observed := plan8GatewayQueueObservation{QueuesBySignal: map[string]float64{}}
	if forward == nil || len(forward.Ports) != 1 || forward.Ports[0].Local == 0 {
		return observed, errors.New("owned Gateway metric forward missing")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/metrics", forward.Ports[0].Local), nil)
	if err != nil {
		return observed, err
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("native metric redirect refused") }}
	response, err := client.Do(request)
	if err != nil {
		return observed, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return observed, errors.New("native Gateway metrics unavailable")
	}
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return observed, errors.New("native Gateway metrics invalid")
	}
	for name, family := range families {
		for _, metric := range family.Metric {
			exporter, receiver, signal := "", "", ""
			for _, label := range metric.Label {
				if label.GetName() == "exporter" {
					exporter = label.GetValue()
				}
				if label.GetName() == "receiver" {
					receiver = label.GetValue()
				}
				if label.GetName() == "data_type" {
					signal = label.GetValue()
				}
			}
			value := float64(0)
			if metric.Counter != nil {
				value = metric.Counter.GetValue()
			} else if metric.Gauge != nil {
				value = metric.Gauge.GetValue()
			} else {
				continue
			}
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return observed, errors.New("native Gateway counter invalid")
			}
			if receiver == "otlp/downstream" && (name == "otelcol_receiver_accepted_log_records" || name == "otelcol_receiver_accepted_log_records_total") {
				observed.AcceptedLogs += value
				observed.AcceptedObserved = true
			}
			if exporter == "otlp/argus" && name == "otelcol_exporter_queue_size" {
				observed.QueuedRequests += value
				observed.QueueObserved = true
				observed.QueuesBySignal[signal] += value
			}
			if exporter == "otlp/argus" && (name == "otelcol_exporter_sent_log_records" || name == "otelcol_exporter_sent_log_records_total") {
				observed.SentLogs += value
			}
			if exporter == "otlp/argus" && (name == "otelcol_exporter_send_failed_log_records" || name == "otelcol_exporter_send_failed_log_records_total") {
				observed.FailedLogs += value
			}
		}
	}
	return observed, nil
}

// Execute only within the normally installed, owned K8s collection stage.
func (a *App) plan8KubernetesPendingQueueRecovery(ctx context.Context, env *E2EEnvironment, cluster string, namespace kubernetesworkbench.Namespace, pod corev1.Pod, application, collector uuid.UUID) (failure error) {
	diagnostic := map[string]any{"passed": false, "phase": "ownership", "collector_id": collector, "application_id": application, "synthetic_otlp_injection": false}
	defer func() {
		diagnostic["passed"] = failure == nil
		body, _ := json.MarshalIndent(diagnostic, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-queue-diagnostic.json"), body))
	}()
	nativeNS := collectormanager.KubernetesNamespace
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, nativeNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID || string(ns.UID) != env.State.Values["p8_collector_namespace_uid"] {
		return errors.New("Gateway queue test namespace ownership differs")
	}
	defer func() {
		if failure == nil {
			return
		}
		capture, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		current, err := env.Kube.Client.CoreV1().Namespaces().Get(capture, nativeNS, metav1.GetOptions{})
		if err != nil || current.UID != ns.UID {
			return
		}
		pods, err := env.Kube.Client.CoreV1().Pods(nativeNS).List(capture, metav1.ListOptions{LabelSelector: "argus.io/collector-id=" + collector.String()})
		if err != nil {
			return
		}
		for _, p := range pods.Items {
			if !strings.HasPrefix(p.Name, "argus-otelcol-gateway-") {
				continue
			}
			tail := int64(200)
			body, err := env.Kube.Client.CoreV1().Pods(nativeNS).GetLogs(p.Name, &corev1.PodLogOptions{TailLines: &tail, Timestamps: true}).DoRaw(capture)
			if err == nil {
				failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan8-queue-gateway-"+string(p.UID)+".log"), redactDiagnostic(body)))
			}
		}
	}()
	observNS, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.ObservNS, metav1.GetOptions{})
	if err != nil || observNS.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.New("queue outage target namespace ownership differs")
	}
	ingest, err := env.Kube.Client.AppsV1().Deployments(env.ObservNS).Get(ctx, "argus-telemetry-ingest", metav1.GetOptions{})
	if err != nil || ingest.Spec.Replicas == nil || *ingest.Spec.Replicas < 1 {
		return errors.New("owned ingest is not initially available")
	}
	ingestUID, originalReplicas := ingest.UID, *ingest.Spec.Replicas
	scale := func(call context.Context, replicas int32) error {
		currentNS, e := env.Kube.Client.CoreV1().Namespaces().Get(call, env.ObservNS, metav1.GetOptions{})
		if e != nil || currentNS.UID != observNS.UID || currentNS.Labels["argus.io/release-id"] != env.ReleaseID {
			return errors.New("owned ingest namespace changed")
		}
		current, e := env.Kube.Client.AppsV1().Deployments(env.ObservNS).Get(call, ingest.Name, metav1.GetOptions{})
		if e != nil || current.UID != ingestUID {
			return errors.New("owned ingest deployment changed")
		}
		current.Spec.Replicas = &replicas
		_, e = env.Kube.Client.AppsV1().Deployments(env.ObservNS).Update(call, current, metav1.UpdateOptions{})
		return e
	}
	paused := false
	defer func() {
		if paused {
			restore, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			failure = errors.Join(failure, scale(restore, originalReplicas))
			failure = errors.Join(failure, env.Kube.WaitDeployment(restore, env.ObservNS, ingest.Name, time.Minute))
		}
	}()
	open := func() (*KubeForward, error) {
		service, e := env.Kube.Client.CoreV1().Services(nativeNS).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{})
		if e != nil || service.Labels["argus.io/collector-id"] != collector.String() {
			return nil, errors.New("owned Gateway service identity differs")
		}
		if e = env.Kube.WaitDeployment(ctx, nativeNS, "argus-otelcol-gateway", time.Minute); e != nil {
			return nil, e
		}
		return env.Kube.PortForwardService(ctx, nativeNS, service.Name, []string{"0:8888"}, io.Discard)
	}
	forward, err := open()
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, forward.Stop()) }()
	baseline, err := plan8ReadGatewayQueue(ctx, forward)
	if err != nil {
		return err
	}
	diagnostic["baseline"] = baseline
	diagnostic["phase"] = "ingest_outage"
	if err = scale(ctx, 0); err != nil {
		return err
	}
	paused = true
	if err = env.Kube.WaitDeployment(ctx, env.ObservNS, ingest.Name, time.Minute); err != nil {
		return err
	}
	outageStarted := time.Now().UTC()
	bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	markers := []string{}
	for i := 0; i < 16; i++ {
		marker := "/plan8-persistent-queue-" + uuid.NewString()
		if _, err = a.plan8KubernetesForwardPath(bounded, env, cluster, namespace.ID, pod, marker); err != nil {
			return err
		}
		markers = append(markers, strings.TrimPrefix(marker, "/"))
	}
	diagnostic["original_markers"] = markers
	diagnostic["phase"] = "observe_pending_gateway_queue"
	var pending plan8GatewayQueueObservation
	for {
		pending, err = plan8ReadGatewayQueue(bounded, forward)
		diagnostic["last_pending_observation"] = pending
		diagnostic["metric_observation_failed"] = err != nil
		if err == nil && pending.AcceptedObserved && pending.QueueObserved && pending.AcceptedLogs-baseline.AcceptedLogs >= float64(len(markers)) && pending.QueuedRequests > 0 {
			break
		}
		if err = waitContext(bounded, 500*time.Millisecond); err != nil {
			return errors.New("Gateway never demonstrated accepted logs and outstanding queue before restart")
		}
	}
	diagnostic["phase"] = "stop_upstream_agents"

	agent, err := env.Kube.Client.AppsV1().DaemonSets(nativeNS).Get(bounded, "argus-otelcol-agent", metav1.GetOptions{})
	if err != nil || agent.Labels["argus.io/collector-id"] != collector.String() {
		return errors.New("owned upstream Agent identity differs")
	}
	const holdKey = "argus.io/plan8-queue-hold"
	if _, present := agent.Spec.Template.Spec.NodeSelector[holdKey]; present {
		return errors.New("upstream Agent already has a test hold")
	}
	holdValue := uuid.NewString()
	nodes, err := env.Kube.Client.CoreV1().Nodes().List(bounded, metav1.ListOptions{LabelSelector: holdKey + "=" + holdValue})
	if err != nil || len(nodes.Items) != 0 {
		return errors.New("upstream hold does not select an empty Node set")
	}
	setHold := func(call context.Context, hold bool) error {
		actualNS, e := env.Kube.Client.CoreV1().Namespaces().Get(call, nativeNS, metav1.GetOptions{})
		if e != nil || actualNS.UID != ns.UID {
			return errors.New("upstream namespace identity changed")
		}
		current, e := env.Kube.Client.AppsV1().DaemonSets(nativeNS).Get(call, agent.Name, metav1.GetOptions{})
		if e != nil || current.UID != agent.UID || current.Labels["argus.io/collector-id"] != collector.String() {
			return errors.New("upstream Agent identity changed")
		}
		selector := current.Spec.Template.Spec.NodeSelector
		if selector == nil {
			selector = map[string]string{}
		}
		if hold {
			if value, present := selector[holdKey]; present && value != holdValue {
				return errors.New("upstream hold owned by another operation")
			}
			selector[holdKey] = holdValue
		} else {
			if value, present := selector[holdKey]; present && value != holdValue {
				return errors.New("upstream hold changed ownership")
			}
			if _, present := selector[holdKey]; !present {
				return nil
			}
			delete(selector, holdKey)
		}
		current.Spec.Template.Spec.NodeSelector = selector
		_, e = env.Kube.Client.AppsV1().DaemonSets(nativeNS).Update(call, current, metav1.UpdateOptions{})
		return e
	}
	held := true
	defer func() {
		if held {
			restore, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			failure = errors.Join(failure, setHold(restore, false))
			failure = errors.Join(failure, env.Kube.WaitDaemonSet(restore, nativeNS, agent.Name, time.Minute))
		}
	}()
	if err = setHold(bounded, true); err != nil {
		return err
	}
	agentSelector, err := metav1.LabelSelectorAsSelector(agent.Spec.Selector)
	if err != nil || agent.Spec.Selector == nil || agentSelector.Empty() {
		return errors.New("owned Agent selector missing")
	}
	for {
		pods, e := env.Kube.Client.CoreV1().Pods(nativeNS).List(bounded, metav1.ListOptions{LabelSelector: agentSelector.String()})
		if e != nil {
			return e
		}
		if len(pods.Items) == 0 {
			break
		}
		if err = waitContext(bounded, 500*time.Millisecond); err != nil {
			return errors.New("upstream Agents did not fully stop before Gateway replacement")
		}
	}
	if err = forward.Stop(); err != nil {
		return err
	}
	diagnostic["upstream_fully_stopped"] = true
	diagnostic["phase"] = "replace_gateway_pod"
	if err = a.plan8KubernetesStateRestart(bounded, env, collector); err != nil {
		return err
	}
	if err = scale(bounded, originalReplicas); err != nil {
		return err
	}
	diagnostic["phase"] = "recover_ingest_and_query_original_markers"
	if err = env.Kube.WaitDeployment(bounded, env.ObservNS, ingest.Name, time.Minute); err != nil {
		return err
	}
	paused = false
	replacement, err := open()
	if err != nil {
		return err
	}
	forward = replacement
	for _, marker := range markers {
		upstream, e := env.Kube.Client.CoreV1().Pods(nativeNS).List(bounded, metav1.ListOptions{LabelSelector: agentSelector.String()})
		if e != nil || len(upstream.Items) != 0 {
			return errors.New("upstream restarted before original queue evidence was queried")
		}
		if _, err = a.plan8WaitScopedLog(bounded, env, application, marker); err != nil {
			return err
		}
	}
	diagnostic["phase"] = "queue_drain"
	var recovered plan8GatewayQueueObservation
	for {
		recovered, err = plan8ReadGatewayQueue(bounded, forward)
		if err == nil {
			diagnostic["last_recovery_observation"] = recovered
		}
		diagnostic["recovery_observation_failed"] = err != nil
		if err == nil && recovered.QueueObserved && recovered.QueuedRequests == 0 {
			break
		}
		if err = waitContext(bounded, 500*time.Millisecond); err != nil {
			return errors.New("Gateway queue did not drain after actual ingress recovery")
		}
	}
	diagnostic["after_recovery"] = recovered
	diagnostic["phase"] = "restore_upstream_agents"
	if err = setHold(bounded, false); err != nil {
		return err
	}
	if err = env.Kube.WaitDaemonSet(bounded, nativeNS, agent.Name, time.Minute); err != nil {
		return err
	}
	held = false
	diagnostic["phase"] = "complete"
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "formal_enrolled_gateway_outstanding_export_queue_pvc_pod_replacement_and_original_cri_markers", "collector_id": collector, "application_id": application, "ingest_namespace_uid": observNS.UID, "ingest_uid": ingestUID, "original_replicas": originalReplicas, "outage_started_at": outageStarted, "outage_seconds": time.Since(outageStarted).Seconds(), "retry_limit_seconds": 300, "baseline": baseline, "pending_before_restart": pending, "after_recovery": recovered, "original_markers_received": markers, "synthetic_otlp_injection": false, "upstream_agent_uid": agent.UID, "upstream_fully_stopped_until_original_markers_received": true, "frozen_capacity_profile_pass_claimed": false}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-k8s-pending-queue-recovery.json"), proof)
}
