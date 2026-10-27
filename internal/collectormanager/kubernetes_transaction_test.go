package collectormanager

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesPublicationLeaseSerializesAndFencesRevisions(t *testing.T) {
	client := fake.NewClientset()
	id := uuid.NewString()
	hash := strings.Repeat("a", 64)
	lease, err := acquireKubernetesPublish(t.Context(), client, id, 2, hash)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	if second, err := acquireKubernetesPublish(ctx, client, id, 3, hash); !errors.Is(err, context.DeadlineExceeded) {
		if second != nil {
			second.release()
		}
		lease.release()
		t.Fatalf("concurrent publication admitted: %v", err)
	}
	cancel()
	if err = lease.finish(configSlotBase, 2, hash); err != nil {
		t.Fatal(err)
	}
	lease.release()
	if old, err := acquireKubernetesPublish(t.Context(), client, id, 1, hash); !errors.Is(err, ErrInvalidCommand) {
		if old != nil {
			old.release()
		}
		t.Fatalf("stale revision admitted: %v", err)
	}
	latest, err := acquireKubernetesPublish(t.Context(), client, id, 3, hash)
	if err != nil {
		t.Fatal(err)
	}
	defer latest.release()
	if latest.annotations["argus.io/effective-slot"] != configSlotBase || inactiveConfigSlot(configSlotBase) != configSlotNext {
		t.Fatal("last effective slot lost")
	}
}

func TestKubernetesRollbackPreservesConcurrentSpecificationChange(t *testing.T) {
	id := uuid.NewString()
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-otelcol-gateway", Namespace: KubernetesNamespace, UID: types.UID(uuid.NewString()), Generation: 1, Labels: map[string]string{"argus.io/collector-id": id}}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "collector", Image: "old"}}}}}}
	client := fake.NewClientset(old)
	snapshot, err := snapshotKubernetesRuntime(t.Context(), client, id)
	if err != nil {
		t.Fatal(err)
	}
	newVersion := old.DeepCopy()
	newVersion.Spec.Template.Spec.Containers[0].Image = "candidate"
	if err = snapshot.apply(t.Context(), client, newVersion, nil); err != nil {
		t.Fatal(err)
	}
	changed, err := client.AppsV1().Deployments(KubernetesNamespace).Get(t.Context(), old.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	changed.Generation++
	changed.Spec.Template.Spec.Containers[0].Image = "other-authorized-change"
	if _, err = client.AppsV1().Deployments(KubernetesNamespace).Update(t.Context(), changed, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.rollback(t.Context(), client); !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("rollback overwrote newer generation: %v", err)
	}
	actual, err := client.AppsV1().Deployments(KubernetesNamespace).Get(t.Context(), old.Name, metav1.GetOptions{})
	if err != nil || actual.Spec.Template.Spec.Containers[0].Image != "other-authorized-change" {
		t.Fatal("concurrent specification was lost")
	}
}

func TestKubernetesRollbackRestoresPriorTemplate(t *testing.T) {
	id := uuid.NewString()
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argus-otelcol-gateway", Namespace: KubernetesNamespace, UID: types.UID(uuid.NewString()), Generation: 1, Labels: map[string]string{"argus.io/collector-id": id}}, Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "collector", Image: "old"}}}}}}
	client := fake.NewClientset(old)
	snapshot, err := snapshotKubernetesRuntime(t.Context(), client, id)
	if err != nil {
		t.Fatal(err)
	}
	next := old.DeepCopy()
	next.Spec.Template.Spec.Containers[0].Image = "candidate"
	if err = snapshot.apply(t.Context(), client, next, nil); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.rollback(t.Context(), client); err != nil {
		t.Fatal(err)
	}
	actual, err := client.AppsV1().Deployments(KubernetesNamespace).Get(t.Context(), old.Name, metav1.GetOptions{})
	if err != nil || actual.Spec.Template.Spec.Containers[0].Image != "old" {
		t.Fatal("previous template was not restored")
	}
}
