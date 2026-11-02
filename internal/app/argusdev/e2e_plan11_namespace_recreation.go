package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// This is an owned actual-API identity gate after the frozen model cohort.
// It never edits identity rows or substitutes fabricated telemetry/UIDs.
func (a *App) plan11NamespaceRecreation(ctx context.Context, env *E2EEnvironment) error {
	if !plan11SignalPhase(env.Options.Plan8Phase) {
		return errors.New("namespace recreation requires explicit signal phase")
	}
	name, oldUID, oldID, cluster := env.State.Values["p11_signal_namespace_name"], env.State.Values["p11_signal_namespace_uid"], env.State.Values["p11_signal_namespace_id"], env.State.Values["p11_signal_cluster_id"]
	oldPod, oldPodName := env.State.Values["p11_signal_pod_uid"], env.State.Values["p11_signal_pod_name"]
	if name == "" || name != env.State.Values["p7_target_namespace"] || oldUID == "" || oldUID != env.State.Values["p7_target_namespace_uid"] || uuid.Validate(oldID) != nil || uuid.Validate(cluster) != nil || oldPod == "" || oldPodName == "" {
		return errors.New("owned Namespace identity prerequisites missing")
	}
	live, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil || string(live.UID) != oldUID || live.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.Join(err, errors.New("namespace deletion ownership changed"))
	}
	roles, err := env.Kube.Client.RbacV1().Roles(name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	bindings, err := env.Kube.Client.RbacV1().RoleBindings(name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, role := range roles.Items {
		if role.Labels["argus.io/release-id"] != env.ReleaseID {
			return errors.New("foreign Role in owned namespace; recreation refused")
		}
	}
	for _, binding := range bindings.Items {
		if binding.Labels["argus.io/release-id"] != env.ReleaseID {
			return errors.New("foreign RoleBinding in owned namespace; recreation refused")
		}
	}
	before, _ := json.MarshalIndent(map[string]any{"namespace": live, "old_resource_id": oldID, "old_pod_uid": oldPod, "old_pod_name": oldPodName, "roles": roles.Items, "role_bindings": bindings.Items}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-namespace-before-recreation.json"), before); err != nil {
		return err
	}
	uid := types.UID(oldUID)
	if err = env.Kube.Client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		_, e := env.Kube.Client.CoreV1().Namespaces().Get(bounded, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			break
		}
		if e != nil {
			return e
		}
		if e = waitContext(bounded, time.Second); e != nil {
			return e
		}
	}
	next, err := env.Kube.Client.CoreV1().Namespaces().Create(bounded, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e"}}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if string(next.UID) == oldUID || next.Labels["argus.io/release-id"] != env.ReleaseID {
		return errors.New("new Namespace ownership not established")
	}
	// The cleanup precondition changes only after the actual new owner is checked.
	env.State.Values["p7_target_namespace_uid"] = string(next.UID)
	for _, role := range roles.Items {
		_, e := env.Kube.Client.RbacV1().Roles(name).Create(bounded, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: role.Name, Namespace: name, Labels: role.Labels}, Rules: role.Rules}, metav1.CreateOptions{})
		if e != nil {
			return e
		}
	}
	for _, binding := range bindings.Items {
		_, e := env.Kube.Client.RbacV1().RoleBindings(name).Create(bounded, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: binding.Name, Namespace: name, Labels: binding.Labels}, RoleRef: binding.RoleRef, Subjects: binding.Subjects}, metav1.CreateOptions{})
		if e != nil {
			return e
		}
	}
	no, yes := false, true
	user := int64(1000)
	pod, err := env.Kube.Client.CoreV1().Pods(name).Create(bounded, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: oldPodName, Namespace: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID}}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, RestartPolicy: corev1.RestartPolicyNever, SecurityContext: &corev1.PodSecurityContext{RunAsUser: &user, RunAsGroup: &user, RunAsNonRoot: &yes, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Containers: []corev1.Container{{Name: "identity", Image: env.State.FixtureImages["remote-runtime"], ImagePullPolicy: corev1.PullNever, Command: []string{"/bin/bash", "-c", "exec sleep infinity"}, SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &no, Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}}}}}}, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	if string(pod.UID) == oldPod {
		return errors.New("Pod UID did not change")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	selected := kubernetesworkbench.Namespace{}
	for {
		query := url.Values{"type": {"namespace"}, "name": {name}, "namespace_mode": {"all_authorized"}, "limit": {"30"}}
		result, e := client.JSON(bounded, "plan11-new-namespace-directory", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/inventory?"+query.Encode(), 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(result)
		var page kubernetesworkbench.Page
		if json.Unmarshal(raw, &page) != nil {
			return errors.New("new Namespace directory malformed")
		}
		for _, object := range page.Items {
			if object.Name == name && object.UID == string(next.UID) {
				selected = kubernetesworkbench.Namespace{ID: page.NamespaceReferences[object.UID], Name: name, UID: object.UID}
			}
		}
		if selected.ID != uuid.Nil {
			break
		}
		if e = waitContext(bounded, time.Second); e != nil {
			return e
		}
	}
	if selected.ID.String() == oldID {
		return errors.New("same name inherited old Namespace resource identity")
	}
	if _, err = a.plan8KubernetesObject(bounded, env, cluster, selected.ID, "pod", pod.Name, string(pod.UID)); err != nil {
		return err
	}
	oldQuery := url.Values{"type": {"pod"}, "name": {pod.Name}, "namespace_id": {selected.ID.String()}, "uid": {oldPod}}
	negative, err := client.JSON(bounded, "plan11-old-pod-uid-denied", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster+"/inventory/object?"+oldQuery.Encode(), 409, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if negative["code"] != kubernetesinventory.ErrIdentityChanged.Error() {
		return errors.New("old Pod UID was rejected for a different cause")
	}
	identity, err := a.postgresQuery(bounded, env, `SELECT json_build_object('old_status',(SELECT status FROM kubernetes_namespace_resources WHERE id='`+oldID+`'),'old_pod_namespace_uid',(SELECT namespace_uid FROM kubernetes_namespace_object_identities WHERE cluster_id='`+cluster+`' AND object_uid='`+oldPod+`' LIMIT 1),'new_id','`+selected.ID.String()+`','new_uid','`+string(next.UID)+`')`)
	if err != nil {
		return err
	}
	var state struct {
		OldStatus       string  `json:"old_status"`
		OldPodNamespace *string `json:"old_pod_namespace_uid"`
	}
	if json.Unmarshal([]byte(identity), &state) != nil || state.OldStatus != "deleted" || state.OldPodNamespace != nil && *state.OldPodNamespace == string(next.UID) {
		return errors.New("old Namespace/Pod metadata inherited recreated identity")
	}
	proof, err := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual owned Namespace same-name delete/recreate and same-name Pod UID replacement through real Kubernetes API/Connector inventory; no identity row edits", "old_namespace_id": oldID, "old_namespace_uid": oldUID, "new_namespace": selected, "old_pod_uid": oldPod, "new_pod_uid": pod.UID, "old_pod_request_http": 409, "old_pod_request_code": negative["code"], "actual_identity_projection": json.RawMessage(identity), "cleanup_current_uid": env.State.Values["p7_target_namespace_uid"], "whole_plan_complete": false}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal namespace proof: %w", err)
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-namespace-recreation.json"), proof)
}
