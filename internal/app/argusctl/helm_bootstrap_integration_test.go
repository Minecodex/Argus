package argusctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// A runtime Pod cannot start until the installer supplies a prerequisite after
// Helm returns. A successful migration Job alone must allow that handoff.
// The ordinary installation path still waits and rolls back an unready Pod.
func TestLiveHelmBootstrapApplySeparatesRuntimeReadiness(t *testing.T) {
	if os.Getenv("ARGUS_PLAN8_HELM_BOOTSTRAP") != "1" {
		t.Skip("explicit owned Helm bootstrap integration required")
	}
	contextName := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if contextName == "" {
		t.Fatal("explicit Kubernetes context required")
	}
	clients, err := clientsFor(contextName)
	if err != nil {
		t.Fatal(err)
	}
	for _, bootstrap := range []bool{true, false} {
		name := "ordinary_wait"
		if bootstrap {
			name = "bootstrap"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			owner := "argus-bootstrap-" + uuid.NewString()[:8]
			namespace, e := clients.typed.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.io/helm-bootstrap-owner": owner}}}, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				clean, stop := context.WithTimeout(context.Background(), time.Minute)
				defer stop()
				current, e := clients.typed.CoreV1().Namespaces().Get(clean, owner, metav1.GetOptions{})
				if e != nil || current.UID != namespace.UID || current.Labels["argus.io/helm-bootstrap-owner"] != owner {
					t.Error("refuse to remove a changed bootstrap test namespace")
					return
				}
				if e = clients.typed.CoreV1().Namespaces().Delete(clean, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID}}); e != nil {
					t.Error(e)
					return
				}
				if e = wait.PollUntilContextCancel(clean, 250*time.Millisecond, true, func(check context.Context) (bool, error) {
					_, e := clients.typed.CoreV1().Namespaces().Get(check, owner, metav1.GetOptions{})
					return apierrors.IsNotFound(e), nil
				}); e != nil {
					t.Error("owned bootstrap namespace cleanup", e)
				}
			})
			manager := helmManager{contextName: contextName, log: io.Discard}
			ch := &chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: "bootstrap-order", Version: "0.0.1"}, Templates: []*common.File{{Name: "templates/runtime.yaml", Data: []byte(helmBootstrapPrerequisiteChart)}}}
			apply, stop := context.WithTimeout(ctx, 15*time.Second)
			if bootstrap {
				e = manager.applyBeforeRuntimeBootstrap(apply, owner, owner, ch, map[string]any{})
			} else {
				e = manager.installOrUpgrade(apply, owner, owner, ch, map[string]any{})
			}
			stop()
			if !bootstrap {
				if e == nil {
					t.Fatal("ordinary installation returned success before the runtime was ready")
				}
				if e = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(check context.Context) (bool, error) {
					_, getErr := clients.typed.AppsV1().Deployments(owner).Get(check, "runtime", metav1.GetOptions{})
					return apierrors.IsNotFound(getErr), nil
				}); e != nil {
					t.Fatal("ordinary failed install did not roll back its owned runtime", e)
				}
				return
			}
			if e != nil {
				t.Fatal("bootstrap apply waited on a runtime prerequisite that can only be supplied after it returns", e)
			}
			if e = waitForJob(ctx, clients, owner, "migration", 30*time.Second); e != nil {
				t.Fatal("independent migration completion", e)
			}
			runtime, e := clients.typed.AppsV1().Deployments(owner).Get(ctx, "runtime", metav1.GetOptions{})
			if e != nil || runtime.Status.AvailableReplicas != 0 {
				t.Fatal("runtime unexpectedly satisfied the missing prerequisite")
			}
			_, e = clients.typed.CoreV1().ConfigMaps(owner).Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "runtime-prerequisite"}, Data: map[string]string{"ready": "yes"}}, metav1.CreateOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if e = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(check context.Context) (bool, error) {
				current, getErr := clients.typed.AppsV1().Deployments(owner).Get(check, "runtime", metav1.GetOptions{})
				return getErr == nil && current.Status.AvailableReplicas == 1, getErr
			}); e != nil {
				t.Fatal("runtime did not become ready after the installer supplied its prerequisite", e)
			}
			t.Log(fmt.Sprintf("actual Helm handoff -> completed migration -> supplied prerequisite -> available runtime, context=%s namespace=%s", contextName, owner))
		})
	}
}

const helmBootstrapPrerequisiteChart = `apiVersion: batch/v1
kind: Job
metadata:
  name: migration
spec:
  template:
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      containers:
        - name: migration
          image: busybox:1.36
          imagePullPolicy: IfNotPresent
          command: ["sh", "-ec", "exit 0"]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: runtime
spec:
  replicas: 1
  selector:
    matchLabels: {app: bootstrap-runtime}
  template:
    metadata:
      labels: {app: bootstrap-runtime}
    spec:
      automountServiceAccountToken: false
      containers:
        - name: runtime
          image: busybox:1.36
          imagePullPolicy: IfNotPresent
          command: ["sh", "-ec", "test \"$READY\" = yes; exec sleep 3600"]
          env:
            - name: READY
              valueFrom:
                configMapKeyRef: {name: runtime-prerequisite, key: ready}
`
