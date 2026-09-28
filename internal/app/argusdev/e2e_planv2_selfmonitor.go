package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (a *App) configurePlanV2SelfMonitoring(ctx context.Context, env *E2EEnvironment) error {
	distribution, _, profiles, err := a.verifyM7Catalog(ctx, env)
	if err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	catalog, err := client.JSONArray(ctx, "p2-native-profiles", "enterprise", http.MethodGet, "/enterprise/telemetry/profiles", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	for _, key := range []string{"skywalking-receiver", "jaeger-receiver"} {
		found := false
		for _, entry := range catalog {
			if entry["key"] == key {
				id, _ := entry["id"].(string)
				profiles = append(profiles, id)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("native trace profile %s unavailable", key)
		}
	}
	clusterID := env.State.Values["m3_cluster_id"]
	if _, err = uuid.Parse(clusterID); err != nil {
		return err
	}
	if err = a.applyM7CollectorAction(ctx, env, "kubernetes-cluster", clusterID, "configure", distribution, profiles); err != nil {
		return err
	}
	if err = env.Kube.WaitDaemonSet(ctx, m7CollectorNamespace, "argus-otelcol-agent", 5*time.Minute); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, m7CollectorNamespace, "argus-otelcol-gateway", 5*time.Minute); err != nil {
		return err
	}
	raw, err := a.postgresQuery(ctx, env, `SELECT r.rendered_config::text FROM collector_config_revisions r JOIN collector_instances c ON c.id=r.collector_id WHERE c.resource_type='kubernetes_cluster' AND c.resource_id='`+clusterID+`' AND r.status='effective' ORDER BY r.revision DESC LIMIT 1;`)
	if err != nil {
		return err
	}
	config, err := planV2SelfCollector([]byte(strings.TrimSpace(raw)))
	if err != nil {
		return err
	}
	identity, err := env.Kube.Client.CoreV1().Secrets(m7CollectorNamespace).Get(ctx, "argus-otelcol-identity", metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, workload := range []struct{ ns, name string }{{env.SystemNS, "argus-server"}, {env.ObservNS, "argus-telemetry-query"}} {
		labels := map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e"}
		_, err = env.Kube.Client.CoreV1().Secrets(workload.ns).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "argus-p2-self-identity", Labels: labels}, Data: identity.Data}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		_, err = env.Kube.Client.CoreV1().ConfigMaps(workload.ns).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "argus-p2-self-collector", Labels: labels}, Data: map[string]string{"collector.json": string(config)}}, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		deployment, e := env.Kube.Client.AppsV1().Deployments(workload.ns).Get(ctx, workload.name, metav1.GetOptions{})
		if e != nil {
			return e
		}
		if deployment.Labels["argus.io/release-id"] != env.ReleaseID {
			return fmt.Errorf("self-monitoring requires an owned deployment")
		}
		if err = enablePlanV2SelfTracing(&deployment.Spec.Template.Spec, workload.name); err != nil {
			return err
		}
		mode := int32(0440)
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: "p2-self-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "argus-p2-self-collector"}}}}, corev1.Volume{Name: "p2-self-identity", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "argus-p2-self-identity", DefaultMode: &mode}}})
		if deployment.Spec.Template.Spec.SecurityContext == nil {
			deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
		}
		deployment.Spec.Template.Spec.SecurityContext.FSGroup = int64PointerValue(65532)
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, corev1.Container{Name: "self-collector", Image: env.State.FixtureImages["otelcol"], ImagePullPolicy: corev1.PullNever, Args: []string{"--config=/etc/argus-self/collector.json"}, VolumeMounts: []corev1.VolumeMount{{Name: "p2-self-config", MountPath: "/etc/argus-self", ReadOnly: true}, {Name: "p2-self-identity", MountPath: "/var/lib/argus-otelcol/identity", ReadOnly: true}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("96Mi"), corev1.ResourceCPU: resource.MustParse("50m")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("384Mi")}}, SecurityContext: &corev1.SecurityContext{RunAsUser: int64PointerValue(65532), RunAsNonRoot: boolPointer(true), AllowPrivilegeEscalation: boolPointer(false)}})
		if _, err = env.Kube.Client.AppsV1().Deployments(workload.ns).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
			return err
		}
		port := intstr.FromInt32(4317)
		tcp := corev1.ProtocolTCP
		err = supplementPlanV2TracePolicy(ctx, env.Kube, workload.ns, deployment.Spec.Template.Labels, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "argus-p2-self-egress", Labels: labels}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": workload.name}}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress}, Egress: []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": m7CollectorNamespace}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port}}}}}})
		if err != nil {
			return err
		}
		if err = env.Kube.WaitDeployment(ctx, workload.ns, workload.name, 5*time.Minute); err != nil {
			return err
		}
	}
	port := intstr.FromInt32(4317)
	tcp := corev1.ProtocolTCP
	gateway, err := env.Kube.Client.AppsV1().Deployments(m7CollectorNamespace).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{})
	if err != nil {
		return err
	}
	err = supplementPlanV2TracePolicy(ctx, env.Kube, m7CollectorNamespace, gateway.Spec.Template.Labels, &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "argus-p2-self-ingress", Labels: map[string]string{"argus.io/release-id": env.ReleaseID}}, Spec: networkingv1.NetworkPolicySpec{PodSelector: *gateway.Spec.Selector, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}, Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"argus.io/release-id": env.ReleaseID}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port}}}}}})
	return err
}

func enablePlanV2SelfTracing(pod *corev1.PodSpec, service string) error {
	index := -1
	for i, container := range pod.Containers {
		matches := slices.Contains(container.Command, "/usr/local/bin/argus-server") && service == "argus-server"
		matches = matches || (service == "argus-telemetry-query" && slices.Contains(container.Command, "/usr/local/bin/argus-telemetry") && slices.Contains(container.Args, "--mode=query"))
		if matches {
			if index >= 0 {
				return fmt.Errorf("ambiguous self-monitoring container for %s", service)
			}
			index = i
		}
	}
	if index < 0 {
		return fmt.Errorf("self-monitoring program entrypoint not found for %s", service)
	}
	pod.Containers[index].Env = append(pod.Containers[index].Env, corev1.EnvVar{Name: "ARGUS_SELF_TRACE_ENDPOINT", Value: "http://127.0.0.1:4318"}, corev1.EnvVar{Name: "ARGUS_SELF_TRACE_SAMPLE_RATIO", Value: "1"}, corev1.EnvVar{Name: "ARGUS_SELF_TRACE_ENVIRONMENT", Value: "planv2-e2e"})
	return nil
}

func planV2SelfCollector(raw []byte) ([]byte, error) {
	agent, err := configbundle.Extract(raw, "kubernetes_agent")
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err = json.Unmarshal(agent, &config); err != nil {
		return nil, err
	}
	receivers := config["receivers"].(map[string]any)
	for name := range receivers {
		if name != "otlp" && name != "skywalking" && name != "jaeger" {
			delete(receivers, name)
		}
	}
	if len(receivers) != 3 {
		return nil, fmt.Errorf("expected three registered native/OTLP receivers")
	}
	service := config["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	for name := range pipelines {
		pipeline := pipelines[name].(map[string]any)
		inputs, ok := pipeline["receivers"].([]any)
		if !ok || len(inputs) != 1 || receivers[fmt.Sprint(inputs[0])] == nil {
			delete(pipelines, name)
		}
	}
	if len(pipelines) != 5 {
		return nil, fmt.Errorf("expected OTLP three-signal and two native trace pipelines")
	}
	config["extensions"] = map[string]any{}
	service["extensions"] = []string{}
	service["telemetry"] = map[string]any{"metrics": map[string]any{"readers": []any{map[string]any{"pull": map[string]any{"exporter": map[string]any{"prometheus": map[string]any{"host": "127.0.0.1", "port": 8888}}}}}}}
	return json.Marshal(config)
}
