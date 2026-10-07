package argusctl

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func namespaceFixture(name, plane string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, UID: types.UID(name + "-uid"), ResourceVersion: "1", Labels: map[string]string{"argus.io/release-id": "test", "app.kubernetes.io/part-of": "argus", "argus.io/plane": plane}}}
}

func TestConsolidatedNamespaceTeardownPreservesCSIUntilReclaim(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprint(failed), func(t *testing.T) {
			cfg := &InstallConfig{}
			cfg.Spec.ReleaseID = "test"
			cfg.Spec.Namespaces = Namespaces{System: "system", Observability: "system", Sandbox: "sandbox"}
			client := fake.NewClientset(namespaceFixture("system", "system"), namespaceFixture("sandbox", "sandbox"))
			err := teardownNamespaces(context.Background(), client, cfg, func() error {
				if _, err := client.CoreV1().Namespaces().Get(context.Background(), "sandbox", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
					t.Fatal("sandbox must be removed before reclaim")
				}
				if _, err := client.CoreV1().Namespaces().Get(context.Background(), "system", metav1.GetOptions{}); err != nil {
					t.Fatal("CSI host namespace was removed before reclaim")
				}
				if failed {
					return fmt.Errorf("reclaim blocked")
				}
				return nil
			})
			if (err != nil) != failed {
				t.Fatalf("teardown error: %v", err)
			}
			_, getErr := client.CoreV1().Namespaces().Get(context.Background(), "system", metav1.GetOptions{})
			if failed && getErr != nil {
				t.Fatal("failed reclaim removed storage namespace")
			}
			if !failed && !apierrors.IsNotFound(getErr) {
				t.Fatal("successful teardown retained system")
			}
			deletes := 0
			for _, a := range client.Actions() {
				if a.GetVerb() == "delete" {
					deletes++
				}
			}
			want := 2
			if failed {
				want = 1
			}
			if deletes != want {
				t.Fatalf("duplicate or premature deletes: %d", deletes)
			}
		})
	}
}

func TestWorkspaceRemovalOnlyDeletesDedicatedOwnedNamespace(t *testing.T) {
	for _, scenario := range []string{"merged", "dedicated", "foreign", "unmarked"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := &InstallConfig{}
			cfg.Spec.ReleaseID = "test"
			cfg.Spec.Namespaces = Namespaces{System: "system", Observability: "system", Sandbox: "sandbox"}
			ns := namespaceFixture("storage", "workspace-storage")
			if scenario == "merged" {
				ns = namespaceFixture("system", "workspace-storage")
			}
			if scenario == "foreign" {
				ns.Labels["argus.io/release-id"] = "other"
			}
			if scenario == "unmarked" {
				delete(ns.Labels, "argus.io/plane")
			}
			client := fake.NewClientset(ns)
			if err := removeDedicatedWorkspaceNamespace(context.Background(), client, cfg, ns); err != nil {
				t.Fatal(err)
			}
			_, err := client.CoreV1().Namespaces().Get(context.Background(), ns.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) != (scenario == "dedicated") {
				t.Fatalf("namespace retention mismatch: %v", err)
			}
		})
	}
}

func TestWorkspaceCleanupPreservesExternalDriverInMergedLayout(t *testing.T) {
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "test"
	cfg.Spec.Namespaces = Namespaces{System: "system", Observability: "system", Sandbox: "sandbox"}
	cfg.Spec.Workspace = WorkspaceInstall{Enabled: true, StorageNamespace: "system", StorageClass: "argus-workspace"}
	driver := &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: "rawfile.csi.openebs.io", Annotations: map[string]string{"meta.helm.sh/release-name": "external", "meta.helm.sh/release-namespace": "external-storage"}}}
	client := fake.NewClientset(namespaceFixture("system", "system"), driver)
	if err := removeWorkspaceStorage(context.Background(), cfg, &kubeClients{typed: client}, helmManager{}); err != nil {
		t.Fatal(err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "get" {
			t.Fatalf("external storage touched: %s", action.GetVerb())
		}
	}
}
