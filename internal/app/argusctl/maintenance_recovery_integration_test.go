package argusctl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/trustbundle"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveRestoreRetainsManagedIssuerAndClientServerChains(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_PKI_RECOVERY") != "1" {
		t.Skip("explicit owned cert-manager restore integration required")
	}
	contextName := os.Getenv("ARGUS_KUBERNETES_POLICY_CONTEXT")
	if !strings.HasPrefix(contextName, "kind-plan8-network-") {
		t.Fatal("refuse PKI restore integration outside the owned disposable context")
	}
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	nodes, err := clients.typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil || len(nodes.Items) != 2 {
		t.Fatal("owned two-node network cluster required")
	}
	for _, node := range nodes.Items {
		if !strings.HasPrefix(node.Name, "plan8-network-") {
			t.Fatal("foreign node in PKI restore test context")
		}
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "local-hardening.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	owner := "plan8-pki-restore-" + uuid.NewString()[:8]
	cfg.Spec.ReleaseID, cfg.Spec.KubeContext, cfg.Spec.Namespaces.System = owner, contextName, owner+"-system"
	state := testBackupRecoveryState(t)
	material, err := trustbundle.Parse([]byte(state.BundlePEM), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := clients.typed.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cfg.Spec.Namespaces.System, Labels: map[string]string{"argus.io/release-id": owner}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 45*time.Second)
		defer stop()
		issuers := clients.dynamic.Resource(pkiIssuerGVR)
		issuer, e := issuers.Get(cleanup, cfg.globalIssuerName(), metav1.GetOptions{})
		if e == nil {
			if issuer.GetLabels()["argus.io/release-id"] != owner {
				t.Error("refuse cleanup of foreign restore Issuer")
			} else {
				uid := issuer.GetUID()
				if e = issuers.Delete(cleanup, issuer.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error(e)
		}
		secret, e := clients.typed.CoreV1().Secrets("cert-manager").Get(cleanup, owner+"-root-ca", metav1.GetOptions{})
		if e == nil {
			if validateManagedRootCA(secret, owner) != nil || !bytes.Equal(secret.Data[corev1.TLSCertKey], state.RootCertificate) {
				t.Error("refuse cleanup of changed restore root")
			} else {
				uid := secret.UID
				if e = clients.typed.CoreV1().Secrets("cert-manager").Delete(cleanup, secret.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error("remove owned restore root failed")
				}
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error(e)
		}
		trust, e := clients.typed.CoreV1().ConfigMaps("cert-manager").Get(cleanup, cfg.trustSourceName(), metav1.GetOptions{})
		if e == nil {
			if trust.Labels["argus.io/release-id"] != owner || trust.Annotations["argus.io/trust-bundle-sha256"] != material.SHA256 {
				t.Error("refuse cleanup of changed restore public trust")
			} else {
				uid := trust.UID
				if e = clients.typed.CoreV1().ConfigMaps("cert-manager").Delete(cleanup, trust.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
					t.Error(e)
				}
			}
		} else if !apierrors.IsNotFound(e) {
			t.Error(e)
		}
		current, e := clients.typed.CoreV1().Namespaces().Get(cleanup, namespace.Name, metav1.GetOptions{})
		if e != nil || current.UID != namespace.UID || current.Labels["argus.io/release-id"] != owner {
			t.Errorf("restore test namespace cleanup identity: %v", e)
			return
		}
		uid := namespace.UID
		if e = clients.typed.CoreV1().Namespaces().Delete(cleanup, namespace.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextTimeout(cleanup, time.Second, 35*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := clients.typed.CoreV1().Namespaces().Get(ctx, namespace.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}); e != nil {
			t.Error(e)
		}
	})
	if err = installBackupRecoveryState(ctx, clients, cfg, state); err != nil {
		t.Fatal(err)
	}
	if err = ensurePKITrustSource(ctx, clients, cfg); err != nil {
		t.Fatal(err)
	}
	issuer := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "ClusterIssuer", "metadata": map[string]any{"name": cfg.globalIssuerName(), "labels": map[string]any{"argus.io/release-id": owner}}, "spec": map[string]any{"ca": map[string]any{"secretName": owner + "-root-ca"}}}}
	if _, err = clients.dynamic.Resource(pkiIssuerGVR).Create(ctx, issuer, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = waitForIssuerReady(ctx, clients, cfg.globalIssuerName(), time.Minute); err != nil {
		t.Fatal(err)
	}
	if err = probeClusterIssuer(ctx, clients, cfg, cfg.globalIssuerName(), state.IssuerGeneration, material); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"runtime": map[string]any{}}
	current, err := preserveRuntimePKIState(ctx, clients, cfg, values)
	if err != nil || current.TrustBundleEpoch != state.BundleEpoch || current.IssuerGeneration != state.IssuerGeneration {
		t.Fatal("live restored epochs changed")
	}
	t.Log("actual cert-manager issued independent serverAuth/clientAuth leaves from restored original root in a new owned namespace; root, Bundle epoch, issuer generation and runtime keys retained")
}
