package argusdev

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Contract fixtures create synthetic pending Runs. Keep the real scheduler
// from executing those fixtures, then restore it before real tool-chain tests.
func (a *App) pauseTemplateContractWorker(ctx context.Context, env *E2EEnvironment) (func() error, error) {
	client := env.Kube.Client.AppsV1().Deployments(env.SystemNS)
	deployment, err := client.Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if deployment.Labels["argus.io/release-id"] != env.ReleaseID {
		return nil, fmt.Errorf("contract Worker ownership mismatch")
	}
	uid := deployment.UID
	original := int32(1)
	if deployment.Spec.Replicas != nil {
		original = *deployment.Spec.Replicas
	}
	set := func(c context.Context, replicas int32) error {
		current, e := client.Get(c, "argus-worker", metav1.GetOptions{})
		if e != nil {
			return e
		}
		if current.UID != uid || current.Labels["argus.io/release-id"] != env.ReleaseID {
			return fmt.Errorf("contract Worker identity changed")
		}
		scale, e := client.GetScale(c, "argus-worker", metav1.GetOptions{})
		if e != nil {
			return e
		}
		if scale.UID != uid {
			return fmt.Errorf("contract Worker scale identity mismatch")
		}
		scale.Spec.Replicas = replicas
		_, e = client.UpdateScale(c, "argus-worker", scale, metav1.UpdateOptions{})
		return e
	}
	if err = set(ctx, 0); err != nil {
		return nil, err
	}
	restore := func() error {
		recovery, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if e := set(recovery, original); e != nil {
			return e
		}
		if e := env.Kube.WaitDeployment(recovery, env.SystemNS, "argus-worker", 2*time.Minute); e != nil {
			return e
		}
		_, _ = fmt.Fprintln(a.stdout, "Contract Worker restored to original replicas and Ready")
		return nil
	}
	deadline := time.NewTimer(time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, e := client.Get(ctx, "argus-worker", metav1.GetOptions{})
		if e != nil {
			_ = restore()
			return nil, e
		}
		if current.Status.Replicas == 0 {
			break
		}
		select {
		case <-ctx.Done():
			_ = restore()
			return nil, ctx.Err()
		case <-deadline.C:
			_ = restore()
			return nil, fmt.Errorf("contract Worker did not stop")
		case <-ticker.C:
		}
	}
	_, _ = fmt.Fprintln(a.stdout, "Owned Worker paused for synthetic PostgreSQL contract fixtures only")
	return restore, nil
}
