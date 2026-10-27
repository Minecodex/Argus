package collectormanager

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const collectorPublishLease = "argus-otelcol-publish"
const configSlotBase = "argus-otelcol-config"
const configSlotNext = "argus-otelcol-config-next"
const secretSlotBase = "argus-otelcol-monitoring"
const secretSlotNext = "argus-otelcol-monitoring-next"

type kubernetesPublishLease struct {
	client      kubernetes.Interface
	holder      string
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	annotations map[string]string
}

func acquireKubernetesPublish(ctx context.Context, client kubernetes.Interface, collector string, revision uint64, hash string) (*kubernetesPublishLease, error) {
	holder := collector + "/" + uuid.NewString()
	duration := int32(120)
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		current, err := client.CoordinationV1().Leases(KubernetesNamespace).Get(deadline, collectorPublishLease, metav1.GetOptions{})
		missing := apierrors.IsNotFound(err)
		if err != nil && !missing {
			return nil, err
		}
		if missing {
			current = &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: collectorPublishLease, Namespace: KubernetesNamespace, Labels: map[string]string{"argus.io/collector-id": collector}}}
		}
		if owner := current.Labels["argus.io/collector-id"]; owner != "" && owner != collector {
			return nil, ErrInvalidCommand
		}
		active := current.Spec.HolderIdentity != nil && *current.Spec.HolderIdentity != "" && current.Spec.RenewTime != nil && current.Spec.LeaseDurationSeconds != nil && time.Now().Before(current.Spec.RenewTime.Add(time.Duration(*current.Spec.LeaseDurationSeconds)*time.Second))
		if !active {
			if current.Annotations == nil {
				current.Annotations = map[string]string{}
			}
			prior, _ := strconv.ParseUint(current.Annotations["argus.io/desired-revision"], 10, 64)
			if prior > revision || (prior == revision && current.Annotations["argus.io/desired-hash"] != "" && current.Annotations["argus.io/desired-hash"] != hash) {
				return nil, ErrInvalidCommand
			}
			current.Annotations["argus.io/desired-revision"], current.Annotations["argus.io/desired-hash"] = strconv.FormatUint(revision, 10), hash
			now := metav1.NewMicroTime(time.Now().UTC())
			current.Spec.HolderIdentity = &holder
			current.Spec.LeaseDurationSeconds = &duration
			current.Spec.RenewTime = &now
			if missing {
				current, err = client.CoordinationV1().Leases(KubernetesNamespace).Create(deadline, current, metav1.CreateOptions{})
			} else {
				current, err = client.CoordinationV1().Leases(KubernetesNamespace).Update(deadline, current, metav1.UpdateOptions{})
			}
			if err == nil {
				runCtx, runCancel := context.WithCancel(ctx)
				lease := &kubernetesPublishLease{client: client, holder: holder, ctx: runCtx, cancel: runCancel, annotations: current.Annotations}
				go lease.renew()
				return lease, nil
			}
			if !apierrors.IsConflict(err) && !apierrors.IsAlreadyExists(err) {
				return nil, err
			}
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-deadline.Done():
			timer.Stop()
			return nil, deadline.Err()
		case <-timer.C:
		}
	}
}

func (lease *kubernetesPublishLease) mutate(ctx context.Context, fn func(*coordinationv1.Lease)) error {
	lease.mu.Lock()
	defer lease.mu.Unlock()
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := lease.client.CoordinationV1().Leases(KubernetesNamespace).Get(ctx, collectorPublishLease, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.Spec.HolderIdentity == nil || *current.Spec.HolderIdentity != lease.holder {
			return errors.New("Collector publication lease was lost")
		}
		fn(current)
		_, err = lease.client.CoordinationV1().Leases(KubernetesNamespace).Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
}

func (lease *kubernetesPublishLease) renew() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-lease.ctx.Done():
			return
		case <-ticker.C:
			if err := lease.mutate(lease.ctx, func(value *coordinationv1.Lease) {
				now := metav1.NewMicroTime(time.Now().UTC())
				value.Spec.RenewTime = &now
			}); err != nil {
				lease.cancel()
				return
			}
		}
	}
}

func (lease *kubernetesPublishLease) finish(slot string, revision uint64, hash string) error {
	return lease.mutate(lease.ctx, func(value *coordinationv1.Lease) {
		if value.Annotations == nil {
			value.Annotations = map[string]string{}
		}
		value.Annotations["argus.io/effective-slot"], value.Annotations["argus.io/effective-revision"], value.Annotations["argus.io/effective-hash"] = slot, strconv.FormatUint(revision, 10), hash
	})
}

func (lease *kubernetesPublishLease) release() {
	lease.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = lease.mutate(ctx, func(value *coordinationv1.Lease) { empty := ""; value.Spec.HolderIdentity = &empty })
}

func inactiveConfigSlot(effective string) string {
	if effective == configSlotBase {
		return configSlotNext
	}
	return configSlotBase
}
func privateSlot(config string) string {
	if config == configSlotNext {
		return secretSlotNext
	}
	return secretSlotBase
}
