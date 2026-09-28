package argusdev

import (
	"context"
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) installE2EIngress(ctx context.Context, env *E2EEnvironment) error {
	if env.IngressNS == "" {
		return nil
	}
	if _, err := env.Kube.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: env.IngressNS, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": env.ReleaseID, "argus.io/plane": "e2e-ingress"}}}, metav1.CreateOptions{}); err != nil {
		return err
	}
	env.ManagedNamespaces = append(env.ManagedNamespaces, env.IngressNS)
	env.ManagedClusterRBAC = append(env.ManagedClusterRBAC, env.IngressClass)
	file := filepath.Join(env.WorkDir, "ingress.yaml")
	if err := writePrivate(file, []byte(e2eIngressManifest(env))); err != nil {
		return err
	}
	return a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "apply", "--filename", file, "--field-manager=argus-e2e")
}

func e2eIngressManifest(env *E2EEnvironment) string {
	// The system namespace is created later by the installer. Selecting this
	// release's namespaces avoids a crash loop while retaining run isolation.
	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata: {name: argus-e2e-ingress, namespace: %[1]s}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %[2]s
  labels: {argus.io/release-id: %[3]s}
rules:
  - apiGroups: [""]
    resources: [configmaps, endpoints, nodes, pods, secrets, services, namespaces]
    verbs: [list, watch, get]
  - apiGroups: [networking.k8s.io]
    resources: [ingresses, ingressclasses]
    verbs: [get, list, watch]
  - apiGroups: [networking.k8s.io]
    resources: [ingresses/status]
    verbs: [update]
  - apiGroups: [discovery.k8s.io]
    resources: [endpointslices]
    verbs: [get, list, watch]
  - apiGroups: [coordination.k8s.io]
    resources: [leases]
    verbs: [get, list, watch, create, update]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %[2]s
  labels: {argus.io/release-id: %[3]s}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: %[2]s}
subjects: [{kind: ServiceAccount, name: argus-e2e-ingress, namespace: %[1]s}]
---
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: %[2]s
  labels: {argus.io/release-id: %[3]s}
spec: {controller: k8s.io/%[2]s}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: argus-e2e-ingress, namespace: %[1]s}
data: {allow-snippet-annotations: "false"}
---
apiVersion: v1
kind: Service
metadata: {name: argus-e2e-ingress, namespace: %[1]s}
spec:
  selector: {app.kubernetes.io/name: argus-e2e-ingress}
  ports: [{name: https, port: 443, targetPort: 443}, {name: http, port: 80, targetPort: 80}]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: argus-e2e-ingress, namespace: %[1]s}
spec:
  replicas: 1
  selector: {matchLabels: {app.kubernetes.io/name: argus-e2e-ingress}}
  template:
    metadata: {labels: {app.kubernetes.io/name: argus-e2e-ingress}}
    spec:
      serviceAccountName: argus-e2e-ingress
      containers:
        - name: controller
          image: registry.k8s.io/ingress-nginx/controller:v1.15.1@sha256:594ceea76b01c592858f803f9ff4d2cb40542cae2060410b2c95f75907d659e1
          args: [/nginx-ingress-controller, --controller-class=k8s.io/%[2]s, --ingress-class=%[2]s, --election-id=%[2]s, --watch-namespace-selector=argus.io/release-id=%[3]s, --publish-service=%[1]s/argus-e2e-ingress, --configmap=%[1]s/argus-e2e-ingress]
          env:
            - {name: POD_NAME, valueFrom: {fieldRef: {fieldPath: metadata.name}}}
            - {name: POD_NAMESPACE, valueFrom: {fieldRef: {fieldPath: metadata.namespace}}}
          securityContext:
            runAsUser: 101
            runAsNonRoot: true
            allowPrivilegeEscalation: false
            capabilities: {drop: [ALL], add: [NET_BIND_SERVICE]}
          resources: {requests: {cpu: 50m, memory: 128Mi}, limits: {cpu: 500m, memory: 256Mi}}
          readinessProbe: {httpGet: {path: /healthz, port: 10254}, periodSeconds: 2}
`, env.IngressNS, env.IngressClass, env.ReleaseID)
}
