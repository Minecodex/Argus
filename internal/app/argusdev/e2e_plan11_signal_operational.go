package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SDK business logs stay Application-owned. This separate actual CRI fixture
// uses the existing component scope and trusted Namespace workload identity.
func (a *App) plan11KubernetesOperationalFixture(ctx context.Context, env *E2EEnvironment, cluster string, namespace kubernetesworkbench.Namespace, deployment kubernetesinventory.Object, collector uuid.UUID, metric map[string]any) error {
	if !plan11SignalPhase(env.Options.Plan8Phase) {
		return errors.New("operational fixture requires explicit signal phase")
	}
	config := plan8KubernetesLogConfig(namespace.ID, deployment.Name, deployment.UID)
	config.Scope = "component"
	id, err := a.plan8ServiceCommit(ctx, env, "plan11-operational-log-create", map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": "Plan11 actual Namespace operational logs", "integration": "k8slog", "collector_id": collector, "enabled": true, "credential_update": "keep", "config": config}})
	if err != nil {
		return err
	}
	if _, err = a.plan8WaitKubernetesCollector(ctx, env, cluster); err != nil {
		return err
	}
	pods, err := env.Kube.Client.CoreV1().Pods(namespace.Name).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment.Name})
	if err != nil || len(pods.Items) != 2 {
		return errors.Join(err, errors.New("actual operational workload identities unavailable"))
	}
	marker := "/plan11-operational-log-" + uuid.NewString()
	if _, err = a.plan8KubernetesForwardPath(ctx, env, cluster, namespace.ID, pods.Items[0], marker); err != nil {
		return err
	}
	row, err := a.plan8ServiceEffective(ctx, env, id, collector, true, nil)
	if err != nil {
		return err
	}
	logs, err := a.plan11WaitNamespaceLog(ctx, env, cluster, namespace.ID, strings.TrimPrefix(marker, "/"))
	if err != nil {
		return err
	}
	env.State.Values["p11_signal_cluster_id"] = cluster
	env.State.Values["p11_signal_namespace_id"] = namespace.ID.String()
	env.State.Values["p11_signal_namespace_name"] = namespace.Name
	env.State.Values["p11_signal_namespace_uid"] = namespace.UID
	env.State.Values["p11_signal_pod_uid"] = string(pods.Items[0].UID)
	env.State.Values["p11_signal_pod_name"] = pods.Items[0].Name
	env.State.Values["p11_signal_operational_collection_id"] = id.String()
	env.State.Values["p11_signal_operational_marker"] = strings.TrimPrefix(marker, "/")
	raw, err := json.MarshalIndent(map[string]any{"passed": true, "collection": row, "cluster_id": cluster, "namespace": namespace, "pod_uid": pods.Items[0].UID, "scope": "component", "application_id": nil, "marker": marker, "logs": logs, "actual_cluster_metric": metric, "synthetic_otlp_injection": false, "collector_left_installed_until_owned_cleanup": true}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-operational-log-prerequisite.json"), raw)
}

func (a *App) plan11WaitNamespaceLog(ctx context.Context, env *E2EEnvironment, cluster string, namespace uuid.UUID, marker string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for attempt := 1; ; attempt++ {
		result, e := client.JSON(bounded, "plan11-operational-log-query", "enterprise", http.MethodPost, "/enterprise/logs/query", 200, map[string]any{"query": "body : " + strconv.Quote(marker), "scope": map[string]any{"mode": "explicit", "resources": []map[string]any{{"type": "kubernetes_namespace", "id": namespace}}}, "time_range": telemetryTimeRange(10 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if e != nil {
			return nil, e
		}
		raw, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			return nil, marshalErr
		}
		if e = writePrivate(filepath.Join(env.Options.Artifacts, fmt.Sprintf("plan11-operational-log-attempt-%02d.json", attempt)), raw); e != nil {
			return nil, e
		}
		matchErr := plan8ScopedLogMatches(result, uuid.MustParse(cluster), marker)
		if matchErr == nil {
			return result, nil
		}

		if e = waitContext(bounded, time.Second); e != nil {
			return nil, errors.Join(e, fmt.Errorf("actual Namespace-scoped operational log verification failed: %w", matchErr))
		}
	}
}
