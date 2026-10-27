package kubernetesworkbench

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// Real API/PG metadata gate with controlled Cluster registration/transport.
// This never claims that a Connector has been formally enrolled by this test.
func TestLiveNamespaceInventorySynchronizesActualUIDReplacement(t *testing.T) {
	name := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if name == "" {
		t.Skip("requires owned Kubernetes inventory acceptance context")
	}
	f := newMutationFixture(t)
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: name}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	api, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	f.service.Backend = liveOperationBackend{client: kubernetesinventory.Client{Dynamic: api}}
	owner := "plan8-namespace-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	namespaces := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	object := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": owner, "labels": map[string]any{"argus.owner": owner}}}}
	}
	current, err := namespaces.Create(t.Context(), object(), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found, err := namespaces.Get(ctx, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return
		}
		if err != nil || found.GetUID() != current.GetUID() || found.GetLabels()["argus.owner"] != owner {
			t.Error("namespace cleanup ownership changed", err)
			return
		}
		uid := found.GetUID()
		if err := namespaces.Delete(ctx, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
			t.Error(err)
			return
		}
		if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 25*time.Second, true, func(ctx context.Context) (bool, error) {
			_, err := namespaces.Get(ctx, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		}); err != nil {
			t.Error(err)
		}
	})
	sync := func() db.KubernetesNamespaceResource {
		t.Helper()
		f.exec("INSERT INTO kubernetes_namespace_sync_states(cluster_id,enterprise_id,lease_token,lease_expires_at,last_attempt_at,snapshot_started_at,observed_cluster_version,status) VALUES($1,$2,$3,now()+interval '1 minute',now(),clock_timestamp(),1,'syncing') ON CONFLICT(cluster_id) DO UPDATE SET lease_token=EXCLUDED.lease_token,lease_expires_at=EXCLUDED.lease_expires_at,snapshot_started_at=EXCLUDED.snapshot_started_at,status='syncing'", f.cluster, f.actor.EnterpriseID, uuid.New())
		claim, err := f.service.Store.Queries.GetKubernetesNamespaceSyncState(t.Context(), db.GetKubernetesNamespaceSyncStateParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.service.syncClaimedNamespaces(t.Context(), claim); err != nil {
			t.Fatal(err)
		}
		row, err := f.service.Store.Queries.ResolveKubernetesNamespaceResource(t.Context(), db.ResolveKubernetesNamespaceResourceParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, Name: owner})
		if err != nil || row.NamespaceUid != string(current.GetUID()) {
			t.Fatal("actual namespace UID was not synchronized", err)
		}
		return row
	}
	pods := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(owner)
	pod, err := pods.Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "identity-proof", "namespace": owner, "labels": map[string]any{"argus.owner": owner}}, "spec": map[string]any{"terminationGracePeriodSeconds": int64(0), "restartPolicy": "Never", "containers": []any{map[string]any{"name": "proof", "image": "busybox:1.37", "command": []any{"sleep", "3600"}}}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first := sync()
	identityQuery := db.ResolveKubernetesMetricObjectNamespaceParams{EnterpriseID: f.actor.EnterpriseID, ClusterID: f.cluster, NamespaceName: owner, ObjectType: "pod", ObjectUid: string(pod.GetUID())}
	if actual, err := f.service.Store.Queries.ResolveKubernetesMetricObjectNamespace(t.Context(), identityQuery); err != nil || actual != first.NamespaceUid {
		t.Fatal("actual Pod UID was not bound through the verified Namespace inventory", err)
	}
	if err := authorization.RequireResource(t.Context(), f.service.Store.Queries, f.actor, "kubernetes_namespace", first.ID); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("metadata discovery granted data access", err)
	}
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'kubernetes_namespace',$4)", uuid.New(), f.actor.EnterpriseID, f.actor.SubjectID, first.ID)
	if err := authorization.RequireResource(t.Context(), f.service.Store.Queries, f.actor, "kubernetes_namespace", first.ID); err != nil {
		t.Fatal("explicit first-generation Namespace grant was not usable", err)
	}
	uid := current.GetUID()
	if err := namespaces.Delete(t.Context(), owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(t.Context(), 100*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := namespaces.Get(ctx, owner, metav1.GetOptions{})
		return apierrors.IsNotFound(err), nil
	}); err != nil {
		t.Fatal(err)
	}
	current, err = namespaces.Create(t.Context(), object(), metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second := sync()
	if _, err := f.service.Store.Queries.ResolveKubernetesMetricObjectNamespace(t.Context(), identityQuery); err == nil {
		t.Fatal("old Pod identity inherited the recreated Namespace generation")
	}
	if first.ID == second.ID || first.NamespaceUid == second.NamespaceUid {
		t.Fatal("same-name replacement reused the old authority")
	}
	if err := authorization.RequireResource(t.Context(), f.service.Store.Queries, f.actor, "kubernetes_namespace", second.ID); !errors.Is(err, authorization.ErrAccessDenied) {
		t.Fatal("new Namespace inherited the old grant", err)
	}
	old, err := f.service.Store.Queries.GetKubernetesNamespaceResource(t.Context(), db.GetKubernetesNamespaceResourceParams{EnterpriseID: f.actor.EnterpriseID, ID: first.ID})
	if err != nil || old.Status != "deleted" {
		t.Fatal("deleted actual Namespace stayed active", err)
	}
	t.Log("actual Namespace create/list/delete/recreate produced distinct UID-bound directory records; discovery and replacement granted no access")
}
