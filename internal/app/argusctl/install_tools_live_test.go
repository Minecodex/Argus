package argusctl

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/yaml"
)

// Execute the real upstream Hook command with a dedicated ServiceAccount and
// a uniquely owned CRD. Its input replaces business CRDs, and deterministic
// denial uses a no-retry Job; production Hook commands/RBAC/retries are intact.
func TestLiveInstallationToolsHookUsesServiceAccountAndRejectsRevocation(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_INSTALL_TOOLS_LIVE") != "1" {
		t.Skip("explicit owned installation-tools integration required")
	}
	contextName, image := os.Getenv("ARGUS_PLAN8_INSTALL_TOOLS_CONTEXT"), os.Getenv("ARGUS_PLAN8_INSTALL_TOOLS_IMAGE")
	if contextName == "" {
		t.Fatal("explicit installation-tools Kubernetes context required")
	}
	repository, tag, err := pinnedCRDHookImage(image)
	if err != nil {
		t.Fatal(err)
	}
	command := renderedInstallationHookCommand(t, repository, tag)
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	owner := "plan8-install-" + uuid.NewString()[:8]
	label := "argus.io/installation-tools-test-owner"
	namespace, err := clients.typed.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{label: owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	group := owner + ".argus.invalid"
	crdName := "checks." + group
	crds := clients.dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})
	instances := clients.dynamic.Resource(schema.GroupVersionResource{Group: group, Version: "v1", Resource: "checks"}).Namespace(owner)
	var role *rbacv1.ClusterRole
	var binding *rbacv1.ClusterRoleBinding
	var instance *unstructured.Unstructured
	passed := false
	imageIDs := map[string]string{}
	t.Cleanup(func() {
		clean, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		cleaned := true
		if instance != nil {
			uid := instance.GetUID()
			if e := instances.Delete(clean, instance.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil && !apierrors.IsNotFound(e) {
				t.Error("owned instance cleanup", e)
				cleaned = false
			}
		}
		if current, e := crds.Get(clean, crdName, metav1.GetOptions{}); e == nil {
			if current.GetLabels()[label] != owner {
				t.Error("refuse foreign CRD cleanup")
				cleaned = false
			} else {
				uid := current.GetUID()
				if e = crds.Delete(clean, crdName, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
					cleaned = false
				}
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error(e)
			cleaned = false
		}
		if binding != nil {
			if e := clients.typed.RbacV1().ClusterRoleBindings().Delete(clean, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &binding.UID}}); e != nil && !apierrors.IsNotFound(e) {
				t.Error(e)
				cleaned = false
			}
		}
		if role != nil {
			if e := clients.typed.RbacV1().ClusterRoles().Delete(clean, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &role.UID}}); e != nil && !apierrors.IsNotFound(e) {
				t.Error(e)
				cleaned = false
			}
		}
		if e := clients.typed.CoreV1().Namespaces().Delete(clean, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID}}); e != nil {
			t.Error(e)
			cleaned = false
		}
		if e := wait.PollUntilContextCancel(clean, 250*time.Millisecond, true, func(check context.Context) (bool, error) {
			_, nsErr := clients.typed.CoreV1().Namespaces().Get(check, owner, metav1.GetOptions{})
			_, crdErr := crds.Get(check, crdName, metav1.GetOptions{})
			return apierrors.IsNotFound(nsErr) && apierrors.IsNotFound(crdErr), nil
		}); e != nil {
			t.Error("owned Hook resources did not retire", e)
			cleaned = false
		}
		if path := os.Getenv("ARGUS_PLAN8_INSTALL_TOOLS_EVIDENCE"); path != "" {
			if _, e := os.Stat(path); e == nil {
				t.Error("preserve previous installer evidence")
				return
			}
			raw, _ := json.MarshalIndent(map[string]any{"passed": passed && cleaned && !t.Failed(), "context": contextName, "namespace_uid": namespace.UID, "requested_image": image, "actual_image_ids": imageIDs, "original_hook_command": command, "service_account_authentication": passed, "revoked_rbac_rejected": passed, "existing_instance_preserved_after_crd_update": passed, "owned_resources_removed": cleaned}, "", "  ")
			if e := writePrivate(path, raw); e != nil {
				t.Error(e)
			}
		}
	})
	if _, err = clients.typed.CoreV1().ServiceAccounts(owner).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "installer", Labels: map[string]string{label: owner}}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	role, err = clients.typed.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{label: owner}}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "create", "patch", "update"}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	subjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: "installer", Namespace: owner}}
	binding, err = clients.typed.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{label: owner}}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: owner}, Subjects: subjects}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	setManifest := func(expanded bool) {
		properties := map[string]any{"message": map[string]any{"type": "string"}}
		if expanded {
			properties["revision"] = map[string]any{"type": "integer"}
		}
		value := map[string]any{"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition", "metadata": map[string]any{"name": crdName, "labels": map[string]string{label: owner}}, "spec": map[string]any{"group": group, "scope": "Namespaced", "names": map[string]any{"plural": "checks", "singular": "check", "kind": "InstallerCheck"}, "versions": []any{map[string]any{"name": "v1", "served": true, "storage": true, "schema": map[string]any{"openAPIV3Schema": map[string]any{"type": "object", "properties": map[string]any{"spec": map[string]any{"type": "object", "properties": properties}}}}}}}}
		raw, e := yaml.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		cm, e := clients.typed.CoreV1().ConfigMaps(owner).Get(ctx, "crds", metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			_, e = clients.typed.CoreV1().ConfigMaps(owner).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "crds", Labels: map[string]string{label: owner}}, Data: map[string]string{"owned.yaml": string(raw)}}, metav1.CreateOptions{})
		} else if e == nil {
			cm.Data = map[string]string{"owned.yaml": string(raw)}
			_, e = clients.typed.CoreV1().ConfigMaps(owner).Update(ctx, cm, metav1.UpdateOptions{})
		}
		if e != nil {
			t.Fatal(e)
		}
	}
	waitAuthorization := func(allowed bool) {
		if e := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 10*time.Second, true, func(check context.Context) (bool, error) {
			review, e := clients.typed.AuthorizationV1().SubjectAccessReviews().Create(check, &authorizationv1.SubjectAccessReview{Spec: authorizationv1.SubjectAccessReviewSpec{User: "system:serviceaccount:" + owner + ":installer", Groups: []string{"system:serviceaccounts", "system:serviceaccounts:" + owner, "system:authenticated"}, ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "patch", Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions", Name: crdName}}}, metav1.CreateOptions{})
			return e == nil && review.Status.Allowed == allowed, e
		}); e != nil {
			t.Fatal("actual Hook authorization did not converge", e)
		}
	}
	runHook := func(name string, success bool) {
		zero := int32(0)
		job, e := clients.typed.BatchV1().Jobs(owner).Create(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{label: owner}}, Spec: batchv1.JobSpec{BackoffLimit: &zero, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{ServiceAccountName: "installer", RestartPolicy: corev1.RestartPolicyNever, Containers: []corev1.Container{{Name: "crd-install", Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Command: command, VolumeMounts: []corev1.VolumeMount{{Name: "crds", MountPath: "/crds/owned", ReadOnly: true}}}}, Volumes: []corev1.Volume{{Name: "crds", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "crds"}}}}}}}}}, metav1.CreateOptions{})
		if e != nil {
			t.Fatal(e)
		}
		e = wait.PollUntilContextTimeout(ctx, 250*time.Millisecond, time.Minute, true, func(check context.Context) (bool, error) {
			current, e := clients.typed.BatchV1().Jobs(owner).Get(check, name, metav1.GetOptions{})
			return e == nil && (current.Status.Succeeded > 0 || current.Status.Failed > 0), e
		})
		pods, listErr := clients.typed.CoreV1().Pods(owner).List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + name})
		if listErr != nil || len(pods.Items) != 1 || metav1.GetControllerOf(&pods.Items[0]) == nil || metav1.GetControllerOf(&pods.Items[0]).UID != job.UID {
			t.Fatal("actual Hook Pod identity missing", listErr)
		}
		pod := pods.Items[0]
		logs, logErr := clients.typed.CoreV1().Pods(owner).GetLogs(pod.Name, &corev1.PodLogOptions{Container: "crd-install"}).DoRaw(ctx)
		if e != nil || logErr != nil {
			t.Fatalf("owned Hook did not finish: %v %v; %s", e, logErr, logs)
		}
		current, e := clients.typed.BatchV1().Jobs(owner).Get(ctx, name, metav1.GetOptions{})
		if e != nil || (current.Status.Succeeded == 1) != success || (!success && !strings.Contains(strings.ToLower(string(logs)), "forbidden")) {
			t.Fatalf("unexpected actual Hook outcome: success=%t error=%v logs=%s", success, e, logs)
		}
		if len(pod.Status.ContainerStatuses) != 1 || !strings.Contains(pod.Status.ContainerStatuses[0].ImageID, "sha256:") {
			t.Fatal("actual installer image identity absent")
		}
		imageIDs[name] = pod.Status.ContainerStatuses[0].ImageID
	}
	setManifest(false)
	waitAuthorization(true)
	runHook("initial", true)
	if err = clients.waitResource(ctx, schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, "", crdName, func(value map[string]any) bool {
		conditions, _, _ := unstructured.NestedSlice(value, "status", "conditions")
		for _, item := range conditions {
			row, _ := item.(map[string]any)
			if row["type"] == "Established" && row["status"] == "True" {
				return true
			}
		}
		return false
	}, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	initial, err := crds.Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	instance, err = instances.Create(ctx, &unstructured.Unstructured{Object: map[string]any{"apiVersion": group + "/v1", "kind": "InstallerCheck", "metadata": map[string]any{"name": "preserved", "labels": map[string]string{label: owner}}, "spec": map[string]any{"message": "original-value"}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	setManifest(true)
	binding.Subjects = nil
	updatedBinding, err := clients.typed.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	binding = updatedBinding
	waitAuthorization(false)
	runHook("denied", false)
	binding.Subjects = subjects
	updatedBinding, err = clients.typed.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	binding = updatedBinding
	waitAuthorization(true)
	runHook("upgrade", true)
	updated, err := crds.Get(ctx, crdName, metav1.GetOptions{})
	if err != nil || updated.GetUID() != initial.GetUID() || updated.GetGeneration() <= initial.GetGeneration() {
		t.Fatal("actual CRD definition did not update without replacement", err)
	}
	preserved, err := instances.Get(ctx, "preserved", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	message, _, _ := unstructured.NestedString(preserved.Object, "spec", "message")
	if preserved.GetUID() != instance.GetUID() || message != "original-value" {
		t.Fatal("actual existing instance was not preserved", err)
	}
	passed = true
	t.Log("actual original Hook command authenticated using ServiceAccount, denied revoked RBAC, updated its owned CRD and preserved an existing instance")
}

func renderedInstallationHookCommand(t *testing.T, repository, tag string) []string {
	t.Helper()
	chart, err := loader.Load(filepath.Join("..", "..", "..", "deploy", ".cache", "charts", "altinity-0.27.3.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	configuration := action.NewConfiguration(action.ConfigurationSetLogger(slog.NewTextHandler(io.Discard, nil)))
	install := action.NewInstall(configuration)
	install.ReleaseName, install.Namespace, install.DryRunStrategy = "owned-install-tools", "owned-install-tools", action.DryRunClient
	result, err := install.Run(chart, map[string]any{"crdHook": map[string]any{"enabled": true, "image": map[string]any{"repository": repository, "tag": tag, "pullPolicy": "IfNotPresent"}}})
	if err != nil {
		t.Fatal(err)
	}
	accessor, err := release.NewAccessor(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range accessor.Hooks() {
		value, e := release.NewHookAccessor(hook)
		if e != nil {
			t.Fatal(e)
		}
		var job batchv1.Job
		if yaml.Unmarshal([]byte(value.Manifest()), &job) == nil && job.Kind == "Job" && len(job.Spec.Template.Spec.Containers) == 1 {
			container := job.Spec.Template.Spec.Containers[0]
			if container.Name == "crd-install" && container.Image == repository+":"+tag && strings.Contains(strings.Join(container.Command, " "), "kubectl apply --server-side=true --force-conflicts") {
				return container.Command
			}
		}
	}
	t.Fatal("pinned upstream CRD Hook command missing")
	return nil
}
