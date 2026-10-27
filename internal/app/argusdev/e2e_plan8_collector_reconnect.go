package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoring"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan8StopCollectionAcrossGatewayLoss(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, row monitoring.Collection) (result monitoring.Collection, failure error) {
	collector, err := uuid.Parse(h.CollectorID)
	if err != nil {
		return row, err
	}
	if _, err = a.plan6InspectDocker(ctx, h.Docker.Host, h.Docker.Owner); err != nil {
		return row, err
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	directory := "/etc/systemd/system/argus-otelcol.service.d"
	unit := directory + "/90-plan8-reconnect-" + id + ".conf"
	counter := "/var/lib/argus-otelcol/plan8-reconnect-" + id + ".count"
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := a.plan6InspectDocker(cleanup, h.Docker.Host, h.Docker.Owner); err != nil {
			failure = errors.Join(failure, err)
			return
		}
		failure = errors.Join(failure, a.runner.RunIO(cleanup, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "rm", "-f", "--", unit, counter))
		failure = errors.Join(failure, a.runner.RunIO(cleanup, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "systemctl", "daemon-reload"))
	}()
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "mkdir", "-p", directory); err != nil {
		return row, err
	}
	// A fixture-only service drop-in opens a deterministic disconnect window.
	// It also counts actual process starts, independently of command receipts.
	body := "[Service]\nExecStartPre=/bin/sh -c 'umask 077; echo start >> " + counter + "'\nExecStartPre=/bin/sleep 20\n"
	if err = a.runner.RunIO(ctx, nil, strings.NewReader(body), io.Discard, io.Discard, "docker", "exec", "-i", h.Docker.Host, "sh", "-c", "cat > \"$1\"", "owned-plan8-dropin", unit); err != nil {
		return row, err
	}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "systemctl", "daemon-reload"); err != nil {
		return row, err
	}
	previous, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT desired_revision FROM collector_instances WHERE id='%s'", collector))
	if err != nil {
		return row, err
	}
	previousRevision, err := strconv.ParseInt(strings.TrimSpace(previous), 10, 64)
	if err != nil || previousRevision < 1 {
		return row, errors.New("Collector fault requires a known preceding configuration revision")
	}
	if _, err = a.plan8ServiceCommit(ctx, env, "file-log-final-stop-with-gateway-loss", plan8ServiceUpdateBody(row, collector, false)); err != nil {
		return row, err
	}
	var command struct {
		ID        string    `json:"command_id"`
		Connector uuid.UUID `json:"connector_id"`
		Epoch     int64     `json:"connection_epoch"`
		Status    string    `json:"status"`
		Revision  int64     `json:"revision"`
	}
	servicePhase, startCount, readyObserved := "", 0, false
	defer func() {
		readiness := map[string]any{"passed": readyObserved, "previous_desired_revision": previousRevision, "command": command, "service_substate": servicePhase, "actual_collector_starts": startCount}
		if failure != nil {
			capture, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			query := fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT command_id,status,connection_epoch,created_at,to_jsonb(c)->'error_code' AS error_code,to_jsonb(c)->'result'->'error_code' AS result_error_code FROM connector_commands c WHERE c.command_type='collector_management' AND coalesce(c.payload->>'collector_id',c.payload->>'collectorId')='%s' ORDER BY created_at DESC LIMIT 4)t`, collector)
			raw, err := a.postgresQuery(capture, env, query)
			readiness["failure_snapshot_available"] = err == nil && json.Valid([]byte(raw))
			if err == nil && json.Valid([]byte(raw)) {
				readiness["command_history"] = json.RawMessage(raw)
			}
		}
		raw, _ := json.MarshalIndent(readiness, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan8-collector-reconnect-readiness.json"), raw))
	}()
	// Commit records an Outbox; configuration compilation is asynchronous.
	// Until it advances, the preceding successful command still matches the
	// Collector's desired revision. It is not this operation's fault window.
	query := fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT c.command_id,c.connector_id,c.connection_epoch,c.status,i.desired_revision AS revision FROM connector_commands c JOIN collector_instances i ON i.id::text=coalesce(c.payload->>'collector_id',c.payload->>'collectorId') AND i.enterprise_id=c.enterprise_id WHERE i.id='%s' AND i.desired_revision>%d AND c.command_type='collector_management' AND i.desired_revision::text=coalesce(c.payload->>'desired_revision',c.payload->>'desiredRevision') ORDER BY c.created_at DESC LIMIT 1) t`, collector, previousRevision)
	deadline := time.Now().Add(12 * time.Second)
	for {
		raw, err := a.postgresQuery(ctx, env, query)
		if err != nil {
			return row, err
		}
		var rows []json.RawMessage
		if json.Unmarshal([]byte(raw), &rows) != nil {
			return row, errors.New("invalid Collector fault readiness response")
		}
		if len(rows) == 1 && json.Unmarshal(rows[0], &command) == nil {
			var phase, count bytes.Buffer
			phaseErr := a.runner.RunIO(ctx, nil, nil, &phase, io.Discard, "docker", "exec", h.Docker.Host, "systemctl", "show", "--property=SubState", "--value", "argus-otelcol.service")
			countErr := a.runner.RunIO(ctx, nil, nil, &count, io.Discard, "docker", "exec", h.Docker.Host, "sh", "-c", "test ! -f \"$1\" || wc -l < \"$1\"", "owned-plan8-start-observation", counter)
			servicePhase = strings.TrimSpace(phase.String())
			startCount, _ = strconv.Atoi(strings.TrimSpace(count.String()))
			if phaseErr == nil && countErr == nil && plan8CollectorFaultReady(command.Status, servicePhase, startCount) {
				readyObserved = true
				break
			}
		}
		if time.Now().After(deadline) || command.Status == "succeeded" {
			return row, fmt.Errorf("Collector fault injection window was not observed after revision %d (new command status=%s, revision=%d)", previousRevision, command.Status, command.Revision)
		}
		if err = waitContext(ctx, 100*time.Millisecond); err != nil {
			return row, err
		}
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return row, errors.New("gateway fault namespace ownership unavailable")
	}
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-connector-gateway", metav1.GetOptions{})
	if err != nil {
		return row, err
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return row, err
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return row, err
	}
	ready := []corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = append(ready, pod)
			}
		}
	}
	if len(ready) != 1 {
		return row, errors.New("fault requires exactly one ready owned gateway Pod")
	}
	proof := map[string]any{"passed": false, "scope": "formal_enrolled_collector_update_across_owned_gateway_process_loss", "previous_desired_revision": previousRevision, "command": command, "gateway_pod": ready[0].Name, "gateway_pod_uid": ready[0].UID, "namespace_uid": ns.UID, "fixture_start_delay_seconds": 20, "synthetic_telemetry_injection": false}
	write := func() error {
		raw, _ := json.MarshalIndent(proof, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-collector-reconnect.json"), raw)
	}
	if err = write(); err != nil {
		return row, err
	}
	zero := int64(0)
	uid := ready[0].UID
	if err = env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, ready[0].Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return row, err
	}
	result, err = a.plan8WaitServiceEffective(ctx, env, row.ID, collector, false, nil, true)
	if err != nil {
		return result, err
	}
	stateQuery := fmt.Sprintf(`SELECT json_build_object('status',c.status,'old_epoch',c.connection_epoch,'current_epoch',x.connection_epoch,'effective_revision',i.effective_revision,'matching_commands',(SELECT count(*) FROM connector_commands d WHERE d.connector_id=c.connector_id AND d.command_type='collector_management' AND coalesce(d.payload->>'collector_id',d.payload->>'collectorId')=i.id::text AND coalesce(d.payload->>'desired_revision',d.payload->>'desiredRevision')=i.desired_revision::text)) FROM connector_commands c JOIN connectors x ON x.id=c.connector_id AND x.enterprise_id=c.enterprise_id JOIN collector_instances i ON i.id='%s' WHERE c.command_id='%s'`, collector, command.ID)
	// command.ID comes from the owned database but is still bounded before SQL.
	if strings.ContainsAny(command.ID, "'\\\r\n") || len(command.ID) > 128 {
		return result, errors.New("invalid command identity in fault receipt")
	}
	raw, err := a.postgresQuery(ctx, env, stateQuery)
	if err != nil {
		return result, err
	}
	var state struct {
		Status       string `json:"status"`
		OldEpoch     int64  `json:"old_epoch"`
		CurrentEpoch int64  `json:"current_epoch"`
		Effective    int64  `json:"effective_revision"`
		Commands     int64  `json:"matching_commands"`
	}
	if json.Unmarshal([]byte(raw), &state) != nil || state.Status != "succeeded" || state.OldEpoch != command.Epoch || state.CurrentEpoch <= command.Epoch || state.Effective != command.Revision || state.Commands != 1 {
		return result, errors.New("current connection did not reconcile the original single Collector command")
	}
	var count bytes.Buffer
	if err = a.runner.RunIO(ctx, nil, nil, &count, io.Discard, "docker", "exec", h.Docker.Host, "sh", "-c", "wc -l < \"$1\"", "owned-plan8-start-count", counter); err != nil {
		return result, err
	}
	starts, err := strconv.Atoi(strings.TrimSpace(count.String()))
	if err != nil || starts != 1 {
		return result, errors.New("Collector management was executed more than once during reconciliation")
	}
	if err = a.plan8WaitCollectorPaths(ctx, env); err != nil {
		return result, err
	}
	host, err := uuid.Parse(h.HostID)
	if err != nil {
		return result, err
	}
	after := time.Now().UTC()
	freshHost, err := a.plan8ServiceEffective(ctx, env, builtincollection.StableID("host", host, "host_metrics"), collector, true, &after)
	if err != nil {
		return result, err
	}
	proof["passed"], proof["reconciled"], proof["actual_collector_starts"], proof["collection"] = true, state, starts, result
	proof["fresh_host_after_route_recovery"] = freshHost
	return result, write()
}

func plan8CollectorFaultReady(status, phase string, starts int) bool {
	if phase != "start-pre" || starts != 1 {
		return false
	}
	// A persisted unknown status is not completion. Require independent process
	// evidence of this exact restart; final reconciliation still must succeed
	// on the original command with one process start and a new connection epoch.
	switch status {
	case "acknowledged", "running", "result_unknown":
		return true
	default:
		return false
	}
}
