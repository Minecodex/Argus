package kubernetesinventory

import (
	"encoding/json"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWorkloadIdentityAndPrivilegeAreIndependentOfClusterScope(t *testing.T) {
	definition, _ := Lookup("deployment")
	base := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "app", "image": "fixture"}}}}}}}
	defaultWorkloadIdentity(definition, base, MutationPolicy{})
	for _, tc := range []struct {
		name    string
		cluster bool
		policy  MutationPolicy
		change  func(map[string]any)
		denied  bool
	}{
		{"ordinary namespace", false, MutationPolicy{}, func(map[string]any) {}, false},
		{"cluster grant is not host privilege", true, MutationPolicy{}, func(p map[string]any) { p["hostNetwork"] = true }, true},
		{"explicit host privilege", true, MutationPolicy{PrivilegedWorkloads: true}, func(p map[string]any) { p["hostNetwork"] = true }, false},
		{"privilege cannot leave namespace scope", false, MutationPolicy{PrivilegedWorkloads: true}, func(map[string]any) {}, true},
		{"automatic token denied", true, MutationPolicy{}, func(p map[string]any) { p["automountServiceAccountToken"] = true }, true},
		{"automatic token permitted explicitly", false, MutationPolicy{ServiceAccountUse: true}, func(p map[string]any) { p["automountServiceAccountToken"] = true }, false},
		{"nondefault service account denied", false, MutationPolicy{}, func(p map[string]any) { p["serviceAccountName"] = "operator" }, true},
		{"projected token denied", true, MutationPolicy{}, func(p map[string]any) {
			p["volumes"] = []any{map[string]any{"name": "api", "projected": map[string]any{"sources": []any{map[string]any{"serviceAccountToken": map[string]any{"path": "token"}}}}}}
		}, true},
		{"host privilege is not API identity", true, MutationPolicy{PrivilegedWorkloads: true}, func(p map[string]any) { p["automountServiceAccountToken"] = true }, true},
		{"windows host process denied", true, MutationPolicy{}, func(p map[string]any) {
			p["securityContext"] = map[string]any{"windowsOptions": map[string]any{"hostProcess": true}}
		}, true},
		{"unmasked proc denied", true, MutationPolicy{}, func(p map[string]any) {
			p["containers"].([]any)[0].(map[string]any)["securityContext"] = map[string]any{"procMount": "Unmasked"}
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := base.DeepCopy()
			pod, _, _ := unstructured.NestedMap(value.Object, workloadPodPath(definition)...)
			tc.change(pod)
			_ = unstructured.SetNestedMap(value.Object, pod, workloadPodPath(definition)...)
			err := validateMutationPolicy(definition, value, tc.cluster, tc.policy)
			if tc.denied != errors.Is(err, ErrForbidden) {
				t.Fatalf("policy error=%v denied=%v", err, tc.denied)
			}
		})
	}
	// An ordinary API default must appear in the effective manifest, and an
	// admission webhook cannot silently replace it with a token-bearing Pod.
	mount, found, _ := unstructured.NestedBool(base.Object, "spec", "template", "spec", "automountServiceAccountToken")
	if !found || mount {
		t.Fatal("new workload did not explicitly disable API token")
	}
	_ = unstructured.SetNestedField(base.Object, true, "spec", "template", "spec", "automountServiceAccountToken")
	raw, _ := json.Marshal(base.Object)
	if err := ValidateMutationResult(definition, MutationResult{After: &Object{Data: raw}}, true, MutationPolicy{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("admission's effective identity escaped policy")
	}
}

func TestRBACDelegationRequiresIndependentClusterPermission(t *testing.T) {
	definition, _ := Lookup("rolebinding")
	value := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "roleRef": map[string]any{"kind": "ClusterRole", "name": "cluster-admin"}}}
	for _, tc := range []struct {
		cluster bool
		policy  MutationPolicy
		denied  bool
	}{{true, MutationPolicy{}, true}, {false, MutationPolicy{RBACDelegation: true}, true}, {true, MutationPolicy{PrivilegedWorkloads: true}, true}, {true, MutationPolicy{RBACDelegation: true}, false}} {
		if err := validateMutationPolicy(definition, value, tc.cluster, tc.policy); tc.denied != errors.Is(err, ErrForbidden) {
			t.Fatalf("RBAC policy %+v: %v", tc, err)
		}
	}
	secret, _ := Lookup("secret")
	token := &unstructured.Unstructured{Object: map[string]any{"type": "kubernetes.io/service-account-token"}}
	if err := validateMutationPolicy(secret, token, true, MutationPolicy{}); !errors.Is(err, ErrForbidden) {
		t.Fatal("token Secret created without API identity permission")
	}
}
