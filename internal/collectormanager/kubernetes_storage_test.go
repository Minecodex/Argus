package collectormanager

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestKubernetesGatewayStateClaimOwnershipAndPodReplacement(t *testing.T) {
	client := fake.NewClientset()
	collector := uuid.NewString()
	if err := ensureKubernetesGatewayState(t.Context(), client, collector); err != nil {
		t.Fatal(err)
	}
	claim, err := client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Get(t.Context(), KubernetesGatewayStateClaim, metav1.GetOptions{})
	if err != nil || claim.Labels["argus.io/collector-id"] != collector {
		t.Fatal("dedicated state claim missing", err)
	}
	if err = ensureKubernetesGatewayState(t.Context(), client, collector); err != nil {
		t.Fatal("same owner could not reuse storage", err)
	}
	if err = ensureKubernetesGatewayState(t.Context(), client, uuid.NewString()); !errors.Is(err, ErrInvalidCommand) {
		t.Fatal("another Collector adopted persisted state", err)
	}
	for _, scenario := range []string{"deleting", "lost", "block", "too_small", "readonly", "unlabelled"} {
		t.Run(scenario, func(t *testing.T) {
			changed := claim.DeepCopy()
			switch scenario {
			case "deleting":
				now := metav1.Now()
				changed.DeletionTimestamp = &now
			case "lost":
				changed.Status.Phase = corev1.ClaimLost
			case "block":
				mode := corev1.PersistentVolumeBlock
				changed.Spec.VolumeMode = &mode
			case "too_small":
				changed.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("64Mi")
			case "readonly":
				changed.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}
			case "unlabelled":
				delete(changed.Labels, "argus.io/collector-id")
			}
			isolated := fake.NewClientset(changed)
			if err := ensureKubernetesGatewayState(t.Context(), isolated, collector); !errors.Is(err, ErrInvalidCommand) {
				t.Fatal("unsafe state claim accepted", err)
			}
		})
	}
	command := collectorCommand(t, testConfigBundle(t), "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	first := collectorPodTemplate(command, "collector:test", configSlotBase, "argus-otelcol-gateway")
	second := collectorPodTemplate(command, "collector:test", configSlotNext, "argus-otelcol-gateway")
	for _, template := range []corev1.PodTemplateSpec{first, second} {
		if !slices.ContainsFunc(template.Spec.Volumes, func(v corev1.Volume) bool {
			return v.Name == "identity-state" && v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == KubernetesGatewayStateClaim && v.EmptyDir == nil
		}) {
			t.Fatal("Gateway replacement does not mount its persistent identity/queue/ledger")
		}
	}
	agent := collectorPodTemplate(command, "collector:test", configSlotBase, "argus-otelcol-agent")
	if slices.ContainsFunc(agent.Spec.Volumes, func(v corev1.Volume) bool { return v.PersistentVolumeClaim != nil }) {
		t.Fatal("node agents inherited a shared single-writer Gateway claim")
	}
}

func TestKubernetesCandidateValidationDoesNotMountLiveStateClaim(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("create", "jobs", func(action ktesting.Action) (bool, runtime.Object, error) {
		job := action.(ktesting.CreateAction).GetObject().(*batchv1.Job)
		if slices.ContainsFunc(job.Spec.Template.Spec.Volumes, func(v corev1.Volume) bool { return v.PersistentVolumeClaim != nil }) {
			t.Fatal("validation attempted to attach the active single-writer volume")
		}
		if !slices.ContainsFunc(job.Spec.Template.Spec.Volumes, func(v corev1.Volume) bool { return v.Name == "identity-state" && v.EmptyDir != nil }) {
			t.Fatal("validation lacks isolated temporary state")
		}
		job.Status.Succeeded = 1
		return false, nil, nil
	})
	command := collectorCommand(t, testConfigBundle(t), "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	if err := validateKubernetesCandidate(t.Context(), client, command, "collector:test", configSlotBase); err != nil {
		t.Fatal(err)
	}
}

func TestKubernetesGatewayStateForeignOwnerStopsUninstallBeforeWrites(t *testing.T) {
	client := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: KubernetesNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus"}}})
	command := collectorCommand(t, testConfigBundle(t), "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	command.ResourceType, command.KubernetesImage = "kubernetes_cluster", "collector:test"
	identity := testKubernetesIdentity(t, command.GetCollectorId())
	if _, err := (Manager{}).ApplyKubernetes(t.Context(), command, KubernetesOptions{Client: client, Image: "collector:test", IdentityMaterial: &identity}); err != nil {
		t.Fatal(err)
	}
	claim, err := client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Get(t.Context(), KubernetesGatewayStateClaim, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	claim.Labels["argus.io/collector-id"] = uuid.NewString()
	if _, err = client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Update(t.Context(), claim, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	command.Operation, command.DesiredRevision = "uninstall", 2
	if _, err = (Manager{}).ApplyKubernetes(t.Context(), command, KubernetesOptions{Client: client, Image: "collector:test"}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatal("uninstall accepted foreign persisted state", err)
	}
	if _, err = client.AppsV1().Deployments(KubernetesNamespace).Get(t.Context(), "argus-otelcol-gateway", metav1.GetOptions{}); err != nil {
		t.Fatal("uninstall partially removed workloads before checking all owners", err)
	}
	claim.Labels["argus.io/collector-id"] = command.GetCollectorId()
	if _, err = client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Update(t.Context(), claim, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = (Manager{}).ApplyKubernetes(t.Context(), command, KubernetesOptions{Client: client, Image: "collector:test"}); err != nil {
		t.Fatal("owned state uninstall failed", err)
	}
	if _, err = client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Get(t.Context(), KubernetesGatewayStateClaim, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatal("owned state was retained after explicit Collector uninstall", err)
	}
}
