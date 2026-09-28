package argusdev

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRealModelDNSOverrideKeepsOtherHostsAndRejectsForeignNamespace(t *testing.T) {
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "owned", false: "foreign"}[owned], func(t *testing.T) {
			owner := "run"
			if !owned {
				owner = "other"
			}
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test", Labels: map[string]string{"argus.io/release-id": owner}}}
			zero := int32(0)
			deployment := func(name string) *appsv1.Deployment {
				return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"}, Spec: appsv1.DeploymentSpec{Replicas: &zero, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{HostAliases: []corev1.HostAlias{{IP: "198.18.0.2", Hostnames: []string{"model.example", "another.example"}}}}}}}
			}
			client := fake.NewSimpleClientset(ns, deployment("argus-server"), deployment("argus-worker"), deployment("unrelated"))
			env := &E2EEnvironment{ReleaseID: "run", SystemNS: "test", Kube: &E2EKube{Client: client}, Options: E2EOptions{Artifacts: t.TempDir(), RealModel: &p5RealModelConfig{BaseURL: "https://model.example/v1", EndpointIP: "8.8.8.8"}}}
			err := configureRealModelEndpointDNS(t.Context(), env)
			if (err == nil) != owned {
				t.Fatalf("ownership gate: %v", err)
			}
			for _, name := range []string{"argus-server", "argus-worker", "unrelated"} {
				d, err := client.AppsV1().Deployments("test").Get(context.Background(), name, metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				want := "198.18.0.2"
				if owned && name != "unrelated" {
					want = "8.8.8.8"
				}
				if p4HostAliasAddress(d.Spec.Template.Spec.HostAliases, "model.example") != want || p4HostAliasAddress(d.Spec.Template.Spec.HostAliases, "another.example") != "198.18.0.2" {
					t.Fatalf("unexpected aliases for %s", name)
				}
			}
		})
	}
}
