package argusdev

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPlan7NamespaceOwnershipIncludesOriginalUIDAndRestrictedPolicy(t *testing.T) {
	for _, change := range []string{"uid", "owner", "policy"} {
		t.Run(change, func(t *testing.T) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "plan7-owned-target", UID: "original", Labels: map[string]string{"argus.io/release-id": "plan7-owned", "pod-security.kubernetes.io/enforce": "restricted"}}}
			client := fake.NewClientset(ns)
			env := &E2EEnvironment{ReleaseID: "plan7-owned", Kube: &E2EKube{Client: client}, State: NewScenarioState("plan7-owned")}
			env.State.Values["p7_target_namespace"], env.State.Values["p7_target_namespace_uid"] = ns.Name, string(ns.UID)
			if _, err := (&App{}).plan7TargetNamespace(t.Context(), env); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "uid":
				ns.UID = "replacement"
			case "owner":
				ns.Labels["argus.io/release-id"] = "different"
			case "policy":
				ns.Labels["pod-security.kubernetes.io/enforce"] = "privileged"
			}
			if _, err := client.CoreV1().Namespaces().Update(t.Context(), ns, metav1.UpdateOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := (&App{}).plan7TargetNamespace(t.Context(), env); err == nil {
				t.Fatal("changed namespace accepted")
			}
			if change != "policy" {
				if err := env.Kube.DeleteNamespace(t.Context(), ns.Name, env.ReleaseID, "original"); err == nil {
					t.Fatal("changed ownership accepted for deletion")
				}
				for _, action := range client.Actions() {
					if action.GetVerb() == "delete" {
						t.Fatal("unowned replacement was deleted")
					}
				}
			}
		})
	}
}
