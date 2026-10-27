package collectormanager

import (
	"context"
	"slices"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const collectorValidationJob = "argus-otelcol-validate"

func validateKubernetesCandidate(ctx context.Context, client kubernetes.Interface, command *connectorv1.CollectorManagementCommand, image, slot string) error {
	previous, err := client.BatchV1().Jobs(KubernetesNamespace).Get(ctx, collectorValidationJob, metav1.GetOptions{})
	if err == nil {
		if previous.Labels["argus.io/collector-id"] != command.GetCollectorId() {
			return ErrInvalidCommand
		}
		uid := previous.UID
		propagation := metav1.DeletePropagationBackground
		if err = client.BatchV1().Jobs(KubernetesNamespace).Delete(ctx, previous.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}, PropagationPolicy: &propagation}); err != nil {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	template := collectorPodTemplate(command, image, slot, "argus-otelcol-gateway")
	// Validation never starts collection and must not mount the active single-writer PVC.
	for i := range template.Spec.Volumes {
		if template.Spec.Volumes[i].Name == "identity-state" {
			template.Spec.Volumes[i].VolumeSource = corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
		}
	}
	template.Labels["app.kubernetes.io/name"] = collectorValidationJob
	container := template.Spec.Containers[0]
	container.Name = "validate-gateway"
	container.Args = []string{"validate", "--config=/etc/argus-otelcol/gateway.yaml", "--config=/var/run/argus-monitoring/config.json"}
	container.Ports = nil
	container.LivenessProbe = nil
	container.ReadinessProbe = nil
	container.StartupProbe = nil
	agent := *container.DeepCopy()
	agent.Env = append(agent.Env, corev1.EnvVar{Name: "K8S_NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}})
	agent.Name = "validate-agent"
	agent.Args = []string{"validate", "--config=/etc/argus-otelcol/agent.yaml"}
	agent.VolumeMounts = slices.DeleteFunc(agent.VolumeMounts, func(mount corev1.VolumeMount) bool { return mount.Name == "monitoring-private" })
	template.Spec.InitContainers = []corev1.Container{agent}
	template.Spec.Containers = []corev1.Container{container}
	template.Spec.RestartPolicy = corev1.RestartPolicyNever
	deadline := int64(90)
	retries := int32(0)
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: collectorValidationJob, Namespace: KubernetesNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/collector-id": command.GetCollectorId()}}, Spec: batchv1.JobSpec{Template: template, BackoffLimit: &retries, ActiveDeadlineSeconds: &deadline}}
	if _, err = client.BatchV1().Jobs(KubernetesNamespace).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		current, err := client.BatchV1().Jobs(KubernetesNamespace).Get(ctx, collectorValidationJob, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if current.Status.Failed > 0 {
			return false, ErrConfigInvalid
		}
		return current.Status.Succeeded == 1, nil
	})
}
