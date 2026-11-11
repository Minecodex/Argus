package kubernetespodlogs

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func readNativeLogs(ctx context.Context, api kubernetes.Interface, q Query, options *corev1.PodLogOptions) ([]byte, error) {
	stream, err := api.CoreV1().Pods(q.Pin.Namespace).GetLogs(q.Pin.Pod, options).Stream(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	defer stream.Close()
	raw, err := io.ReadAll(io.LimitReader(stream, MaxBytes+1))
	if err != nil {
		clear(raw)
		return nil, kubernetesinventory.ErrUnavailable
	}
	return raw, nil
}

// The first read uses the native tail, so a quiet container still exposes its
// latest retained entries. Re-read its boundary second to count the full prefix:
// a tail can start midway through repeated entries sharing the same timestamp.
func completeInitialTail(ctx context.Context, api kubernetes.Interface, q Query, raw []byte, initial Batch) (Batch, error) {
	if initial.Truncated || initial.Next.Count == 0 {
		return initial, nil
	}
	end := len(raw)
	if !q.Pin.Previous && raw[end-1] != '\n' {
		end = bytes.LastIndexByte(raw, '\n') + 1
	}
	cut := 0
	for cut < end {
		length := bytes.IndexByte(raw[cut:end], '\n') + 1
		if length == 0 {
			length = end - cut
		}
		line := raw[cut : cut+length]
		separator := bytes.IndexByte(line, ' ')
		if separator < 0 {
			return Batch{}, ErrGap
		}
		at, err := time.Parse(time.RFC3339Nano, string(line[:separator]))
		if err != nil {
			return Batch{}, ErrGap
		}
		if at.Truncate(time.Second).Equal(initial.Next.Second) {
			break
		}
		cut += length
	}
	limit := int64(MaxBytes + 1)
	options := &corev1.PodLogOptions{Container: q.Pin.Container, Previous: q.Pin.Previous, Timestamps: true, SinceTime: &metav1.Time{Time: initial.Next.Second}, LimitBytes: &limit}
	boundary, err := readNativeLogs(ctx, api, q, options)
	if err != nil {
		return Batch{}, err
	}
	defer clear(boundary)
	if err = ValidatePin(ctx, api, q.Pin); err != nil {
		return Batch{}, err
	}
	if len(boundary) > MaxBytes || cut+len(boundary) > MaxBytes {
		initial.Truncated = true
		return initial, nil
	}
	// Rotation or changed data between the two snapshots cannot be accepted as
	// an empty success. Later appends are included in this new bounded snapshot.
	if !bytes.Contains(boundary, raw[cut:end]) {
		return Batch{}, ErrGap
	}
	combined := make([]byte, 0, cut+len(boundary))
	combined = append(combined, raw[:cut]...)
	combined = append(combined, boundary...)
	defer clear(combined)
	return Continue(combined, q, time.Now().UTC())
}
