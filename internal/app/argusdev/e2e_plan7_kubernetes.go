package argusdev

import (
	"context"
	"fmt"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

func (a *App) plan7KubernetesControlFlow(ctx context.Context, env *E2EEnvironment) error {
	targetNS, err := a.plan7TargetNamespace(ctx, env)
	if err != nil {
		return err
	}
	cluster, err := a.createM3InCluster(ctx, env, targetNS)
	if err != nil {
		return err
	}
	if err = a.startM3KubernetesConnector(ctx, env); err != nil {
		return err
	}
	env.State.Values["p7_cluster_id"] = cluster
	no := false
	yes := true
	uid := int64(65532)
	pod, err := env.Kube.Client.CoreV1().Pods(targetNS).Create(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "argus-plan7-target", Namespace: targetNS, Labels: map[string]string{"argus.io/release-id": env.ReleaseID}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever, SecurityContext: &corev1.PodSecurityContext{RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: "target", Image: env.State.FixtureImages["remote-runtime"], ImagePullPolicy: corev1.PullNever, Command: []string{"/bin/bash", "-c", "exec sleep infinity"}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Minute)
	for {
		current, e := env.Kube.Client.CoreV1().Pods(targetNS).Get(ctx, pod.Name, metav1.GetOptions{})
		if e != nil || current.UID != pod.UID {
			return fmt.Errorf("Plan7 target Pod changed")
		}
		if current.Status.Phase == corev1.PodRunning {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("Plan7 target Pod not ready")
		}
		if e = waitContext(ctx, 300*time.Millisecond); e != nil {
			return e
		}
	}
	conversation, err := a.p5Conversation(ctx, env, "Plan7 Kubernetes controlled client and Pod exec")
	if err != nil {
		return err
	}
	kubectl := map[string]any{"description": "Create an owned marker using the enrolled API identity", "target": map[string]any{"resource_type": "kubernetes_cluster", "resource_id": cluster, "namespace": targetNS}, "interpreter": "kubectl", "script": "kubectl create configmap plan7-marker -n " + targetNS + " --from-literal=value=approved; kubectl get configmap plan7-marker -n " + targetNS + " -o json > \"$ARGUS_OUTPUT_DIR/marker.json\"", "timeout_seconds": 60}
	exec := map[string]any{"description": "Inspect only the fixed Pod instance and container", "target": map[string]any{"resource_type": "kubernetes_cluster", "resource_id": cluster, "namespace": targetNS, "pod": pod.Name, "pod_uid": string(pod.UID), "container": "target"}, "interpreter": "pod_bash", "script": "id -u; printf 'pod evidence' > \"$ARGUS_OUTPUT_DIR/pod.txt\"", "timeout_seconds": 60}
	run, err := a.p5StartRun(ctx, env, conversation, "plan7-kubernetes", []p5ToolStep{{Tool: "tool.search", Arguments: map[string]any{"category": "workflow"}}, {Tool: "tool.describe", Arguments: map[string]any{"category": "workflow", "name": "remote_plan.preview"}}, plan7PreviewTool("plan", kubectl, exec)}, []string{})
	if err != nil {
		return err
	}
	view, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool { return v.Status == "awaiting_confirmation" }, run)
	if err != nil {
		return err
	}
	if err = a.plan7NoDispatchBeforeApproval(ctx, env, view); err != nil {
		return err
	}
	if err = a.plan7Approve(ctx, env, view); err != nil {
		return err
	}
	result, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool {
		return v.ID == view.ID && v.Status != "awaiting_confirmation" && v.Status != "ready" && v.Status != "running"
	})
	if err != nil {
		return err
	}
	if err = a.plan7CompletedEvidence(ctx, env, conversation, "kubernetes-control", result); err != nil {
		return err
	}
	marker, err := env.Kube.Client.CoreV1().ConfigMaps(targetNS).Get(ctx, "plan7-marker", metav1.GetOptions{})
	if err != nil || marker.Data["value"] != "approved" {
		return fmt.Errorf("Plan7 Kubernetes effect missing")
	}
	client, _ := scenarioHTTP(env)
	if _, err = client.JSON(ctx, "p7-kubernetes-permissions", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster, 200, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	env.State.Values["p7_kubernetes_conversation"] = conversation
	env.State.Values["p7_target_pod"] = pod.Name
	env.State.Values["p7_target_pod_uid"] = string(pod.UID)
	env.State.Values["p7_target_namespace"] = targetNS
	return a.waitRunTerminal(ctx, env, run)
}
