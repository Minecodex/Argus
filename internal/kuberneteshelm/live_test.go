package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const nativeChart = `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-settings
data:
  message: {{ .Values.message | quote }}
  build: {{ randAlphaNum 12 | quote }}
---
apiVersion: v1
kind: Secret
metadata:
  name: {{ .Release.Name }}-secret
type: Opaque
data:
  password: {{ .Values.password | b64enc | quote }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-service
spec:
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: {{ .Release.Name }}
---
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-hook
  annotations:
    helm.sh/hook: pre-install,pre-upgrade,pre-rollback,pre-delete
    helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded
spec:
  template:
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      containers:
        - name: main
          image: alpine:3.21
          imagePullPolicy: IfNotPresent
          command: ["/bin/sh", "-c", "sleep 1; exit 0"]
          resources:
            requests: {cpu: 10m, memory: 8Mi}
            limits: {cpu: 100m, memory: 32Mi}
`

func TestLiveHelmNativeLifecycleFrozenValuesHooksAndVersionConflict(t *testing.T) {
	ctx, cfg, api, ns := liveHelmEnvironment(t)
	owner := ns.Name
	permissions := []string{}
	for _, d := range kubernetesinventory.Definitions() {
		if d.ManagePermission != "" {
			permissions = append(permissions, d.ManagePermission)
		}
	}
	engine := Engine{Config: cfg, AuthorizeWrite: func(context.Context, Request, Write) error { return nil }}
	request := Request{SchemaVersion: SchemaVersion, APIServer: cfg.Host, OperationID: uuid.NewString(), Namespace: Namespace{Name: owner, UID: string(ns.UID)}, Name: "sample", Operation: "install", TimeoutSeconds: 90, Access: Access{Namespaces: []Namespace{{Name: owner, UID: string(ns.UID)}}, Permissions: permissions}}
	material := Material{Chart: packagedChart(t, "1.0.0", nativeChart), Values: []byte("message: one\npassword: plan8-private-helm-canary\n"), Key: bytes.Repeat([]byte{5}, 32)}
	preview, e := engine.Preview(ctx, request, material)
	if e != nil {
		t.Fatal("install preview", e)
	}
	public, _ := json.Marshal(preview)
	if bytes.Contains(public, []byte("plan8-private-helm-canary")) {
		t.Fatal("private values in preview")
	}
	if len(preview.Objects) != 4 {
		t.Fatal("objects and hook not all authorized", len(preview.Objects))
	}
	request.SealedPlan = preview.SealedPlan
	installed, e := engine.Execute(ctx, request, material)
	if e != nil || !installed.Ready {
		t.Fatalf("install: %v result=%+v", e, installed)
	}
	cm, e := api.CoreV1().ConfigMaps(owner).Get(ctx, "sample-settings", metav1.GetOptions{})
	if e != nil || cm.Data["message"] != "one" {
		t.Fatal("installed value", e)
	}
	firstBuild := cm.Data["build"]
	secret, e := api.CoreV1().Secrets(owner).Get(ctx, "sample-secret", metav1.GetOptions{})
	if e != nil || string(secret.Data["password"]) != "plan8-private-helm-canary" {
		t.Fatal("private secret did not reach target", e)
	}
	observed, e := engine.Observe(ctx, request, material)
	if e != nil || !observed.Ready || !observed.Applied {
		t.Fatal("observed install", e, observed.Release)
	}
	request.Operation, request.OperationID, request.ExpectedRevision, request.SealedPlan = "upgrade", uuid.NewString(), 1, nil
	material.Values = []byte("message: two\npassword: plan8-private-helm-canary\n")
	preview, e = engine.Preview(ctx, request, material)
	if e != nil {
		t.Fatal("upgrade preview", e)
	}
	// A concurrent edit invalidates the whole approved plan before any Helm
	// storage or workload write. A fresh preview is required to continue.
	cm.Annotations["plan8.concurrent-edit"] = "observed"
	if _, e = api.CoreV1().ConfigMaps(owner).Update(ctx, cm, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	request.SealedPlan = preview.SealedPlan
	if _, e = engine.Execute(ctx, request, material); !errors.Is(e, ErrConflict) {
		t.Fatal("stale object preview executed", e)
	}
	request.SealedPlan = nil
	preview, e = engine.Preview(ctx, request, material)
	if e != nil {
		t.Fatal("fresh upgrade preview", e)
	}
	request.SealedPlan = preview.SealedPlan
	upgraded, e := engine.Execute(ctx, request, material)
	if e != nil || !upgraded.Ready || upgraded.Release.Revision != 2 {
		t.Fatalf("upgrade: %v result=%+v", e, upgraded)
	}
	request.Operation, request.OperationID, request.ExpectedRevision, request.RollbackRevision, request.SealedPlan = "rollback", uuid.NewString(), 2, 1, nil
	preview, e = engine.Preview(ctx, request, material)
	if e != nil {
		t.Fatal("rollback preview", e)
	}
	request.SealedPlan = preview.SealedPlan
	rolled, e := engine.Execute(ctx, request, material)
	if e != nil || !rolled.Ready || rolled.Release.Revision != 3 {
		t.Fatalf("rollback: %v result=%+v", e, rolled)
	}
	cm, e = api.CoreV1().ConfigMaps(owner).Get(ctx, "sample-settings", metav1.GetOptions{})
	if e != nil || cm.Data["message"] != "one" || cm.Data["build"] != firstBuild {
		t.Fatal("rollback rerendered random values or failed to restore revision", e)
	}
	request.Operation, request.OperationID, request.ExpectedRevision, request.RollbackRevision, request.SealedPlan = "uninstall", uuid.NewString(), 3, 0, nil
	preview, e = engine.Preview(ctx, request, material)
	if e != nil {
		t.Fatal("uninstall preview", e)
	}
	request.SealedPlan = preview.SealedPlan
	removed, e := engine.Execute(ctx, request, material)
	if e != nil || !removed.Ready {
		t.Fatalf("uninstall: %v result=%+v", e, removed)
	}
	if _, e = api.CoreV1().ConfigMaps(owner).Get(ctx, "sample-settings", metav1.GetOptions{}); !apierrors.IsNotFound(e) {
		t.Fatal("uninstall left chart ConfigMap", e)
	}
	request.Operation = "history"
	history, e := engine.Releases(ctx, request)
	if e != nil || len(history) != 3 {
		t.Fatal("Helm history not retained", e, len(history))
	}
	t.Log("native Helm install, upgrade, rollback, uninstall, private values, frozen random values and lifecycle hooks passed")
}

func liveHelmEnvironment(t *testing.T) (context.Context, *rest.Config, kubernetes.Interface, *corev1.Namespace) {
	t.Helper()
	selected := os.Getenv("ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	if selected == "" {
		t.Skip("requires explicitly selected Kubernetes context")
	}
	cfg, e := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{CurrentContext: selected}).ClientConfig()
	if e != nil {
		t.Fatal(e)
	}
	cfg.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	api, e := kubernetes.NewForConfig(cfg)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	t.Cleanup(cancel)
	owner := "plan8-helm-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	ns, e := api.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: owner, Labels: map[string]string{"argus.owner": owner}}}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		current, e := api.CoreV1().Namespaces().Get(cleanup, owner, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			return
		}
		if e != nil || current.UID != ns.UID || current.Labels["argus.owner"] != owner {
			t.Error("namespace cleanup ownership mismatch", e)
			return
		}
		if e = api.CoreV1().Namespaces().Delete(cleanup, owner, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &ns.UID}}); e != nil {
			t.Error(e)
			return
		}
		if e = wait.PollUntilContextCancel(cleanup, 300*time.Millisecond, true, func(c context.Context) (bool, error) {
			_, e := api.CoreV1().Namespaces().Get(c, owner, metav1.GetOptions{})
			return apierrors.IsNotFound(e), nil
		}); e != nil {
			t.Error(e)
		}
		t.Logf("owned namespace %s UID %s deleted", owner, ns.UID)
	})
	return ctx, cfg, api, ns
}
