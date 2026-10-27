// Package kubernetespodlogs reads bounded logs using an immutable API identity.
// Direct and Connector adapters share these checks and continuation semantics.
package kubernetespodlogs

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const SchemaVersion = "argus.kubernetes_pod_logs/v1"
const MaxBytes = 256 << 10

var ErrGap = errors.New("KUBERNETES_LOG_CONTINUITY_LOST")

func ErrorCode(err error) string {
	if errors.Is(err, ErrGap) {
		return ErrGap.Error()
	}
	return kubernetesinventory.ErrorCode(err)
}
func ErrorFromCode(code string) error {
	if code == ErrGap.Error() {
		return ErrGap
	}
	return kubernetesinventory.ErrorFromCode(code)
}

type Pin struct {
	Namespace           string `json:"namespace"`
	NamespaceUID        string `json:"namespace_uid"`
	Pod                 string `json:"pod"`
	PodUID              string `json:"pod_uid"`
	Container           string `json:"container"`
	ContainerID         string `json:"container_id"`
	RestartCount        int32  `json:"restart_count"`
	Previous            bool   `json:"previous"`
	PreviousContainerID string `json:"previous_container_id,omitempty"`
}
type Query struct {
	APIServer string `json:"api_server"`
	// Private per-session HMAC key; never projected into a browser cursor.
	ContinuityKey []byte     `json:"continuity_key"`
	Pin           Pin        `json:"pin"`
	Since         time.Time  `json:"since"`
	Resume        *Watermark `json:"resume,omitempty"`
	TailLines     int        `json:"tail_lines"`
}
type Batch struct {
	Content     string    `json:"content"`
	Next        Watermark `json:"next"`
	ObservedAt  time.Time `json:"observed_at"`
	Truncated   bool      `json:"truncated"`
	PendingLine bool      `json:"pending_line"`
	Redacted    bool      `json:"redacted"`
}

func (q Query) Validate() error {
	p := q.Pin
	if len(q.ContinuityKey) != 32 || !kubernetesaccess.ValidServer(q.APIServer) {
		return kubernetesinventory.ErrInvalid
	}
	if p.Namespace == "" || p.NamespaceUID == "" || p.Pod == "" || p.PodUID == "" || p.Container == "" || p.ContainerID == "" || p.RestartCount < 0 || p.Previous && (p.RestartCount == 0 || p.PreviousContainerID == "") || q.TailLines < 1 || q.TailLines > 1000 || q.Since.IsZero() || q.Since.Nanosecond() != 0 {
		return kubernetesinventory.ErrInvalid
	}
	if q.Resume != nil && (q.Resume.Second.Before(q.Since) || q.Resume.Second.Nanosecond() != 0 || q.Resume.Count < 0 || q.Resume.Count > MaxBytes || len(q.Resume.Hash) != 64) {
		return kubernetesinventory.ErrInvalid
	}
	return nil
}

func PinPod(namespaceUID string, pod *corev1.Pod, container string, previous bool) (Pin, error) {
	p := Pin{Namespace: pod.Namespace, NamespaceUID: namespaceUID, Pod: pod.Name, PodUID: string(pod.UID), Container: container, Previous: previous}
	if pod.DeletionTimestamp != nil {
		return p, kubernetesinventory.ErrIdentityChanged
	}
	for _, statuses := range [][]corev1.ContainerStatus{pod.Status.ContainerStatuses, pod.Status.InitContainerStatuses, pod.Status.EphemeralContainerStatuses} {
		for _, c := range statuses {
			if c.Name == container {
				p.ContainerID, p.RestartCount = c.ContainerID, c.RestartCount
				if c.LastTerminationState.Terminated != nil {
					p.PreviousContainerID = c.LastTerminationState.Terminated.ContainerID
				}
				if p.ContainerID == "" || previous && (p.RestartCount < 1 || p.PreviousContainerID == "") {
					return p, kubernetesinventory.ErrUnavailable
				}
				return p, nil
			}
		}
	}
	return p, kubernetesinventory.ErrIdentityChanged
}

func ValidatePin(ctx context.Context, api kubernetes.Interface, pin Pin) error {
	ns, err := api.CoreV1().Namespaces().Get(ctx, pin.Namespace, metav1.GetOptions{})
	if err != nil {
		return apiError(err)
	}
	if string(ns.UID) != pin.NamespaceUID || ns.DeletionTimestamp != nil {
		return kubernetesinventory.ErrIdentityChanged
	}
	pod, err := api.CoreV1().Pods(pin.Namespace).Get(ctx, pin.Pod, metav1.GetOptions{})
	if err != nil {
		return apiError(err)
	}
	current, err := PinPod(pin.NamespaceUID, pod, pin.Container, pin.Previous)
	if err != nil || current != pin {
		return kubernetesinventory.ErrIdentityChanged
	}
	return nil
}

func Read(ctx context.Context, api kubernetes.Interface, q Query) (Batch, error) {
	if err := q.Validate(); err != nil {
		return Batch{}, err
	}
	if err := ValidatePin(ctx, api, q.Pin); err != nil {
		return Batch{}, err
	}
	since := q.Since
	if q.Resume != nil {
		since = q.Resume.Second
	}
	limit := int64(MaxBytes + 1)
	options := &corev1.PodLogOptions{Container: q.Pin.Container, Previous: q.Pin.Previous, Timestamps: true, SinceTime: &metav1.Time{Time: since}, LimitBytes: &limit}
	stream, err := api.CoreV1().Pods(q.Pin.Namespace).GetLogs(q.Pin.Pod, options).Stream(ctx)
	if err != nil {
		return Batch{}, apiError(err)
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, limit))
	defer clear(raw)
	if err != nil {
		return Batch{}, kubernetesinventory.ErrUnavailable
	}
	if err = ValidatePin(ctx, api, q.Pin); err != nil {
		return Batch{}, err
	}
	batch, err := Continue(raw, q, time.Now().UTC())
	if err != nil {
		return Batch{}, err
	}
	projected, changed, err := datapolicy.Project(batch.Content)
	if err != nil {
		return Batch{}, kubernetesinventory.ErrUnavailable
	}
	batch.Content, batch.Redacted = projected.(string), changed
	return batch, nil
}

func apiError(err error) error {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return kubernetesinventory.ErrForbidden
	}
	if apierrors.IsNotFound(err) {
		return kubernetesinventory.ErrIdentityChanged
	}
	return kubernetesinventory.ErrUnavailable
}
