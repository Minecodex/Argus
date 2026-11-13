package collectormanager

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func TestKubernetesPublicNodeCAIsInTheCandidateConfigVolume(t *testing.T) {
	var bundle configbundle.Bundle
	if err := json.Unmarshal(testConfigBundle(t), &bundle); err != nil {
		t.Fatal(err)
	}
	identity := testKubernetesIdentity(t, bundle.CollectorID)
	config, _ := json.Marshal(map[string]any{"interval": "10s", "component_tls": map[string]string{"ca_pem": string(identity.CABundlePEM)}})
	bundle.Collections = []configbundle.Collection{{ID: uuid.NewString(), Integration: "kubernetes_node_metrics", Enabled: true, Config: config}}
	raw, _ := json.Marshal(bundle)
	command := collectorCommand(t, raw, "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	command.ResourceType = "kubernetes_cluster"
	command.KubernetesImage = "registry.example/collector:test"
	client := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: KubernetesNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus"}}})
	if _, err := (Manager{}).ApplyKubernetes(context.Background(), command, KubernetesOptions{Client: client, Image: command.KubernetesImage, IdentityMaterial: &identity}); err != nil {
		t.Fatal(err)
	}
	maps, err := client.CoreV1().ConfigMaps(KubernetesNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, cm := range maps.Items {
		if cm.Data[configbundle.KubeletCAFile] == string(identity.CABundlePEM) {
			found = true
		}
	}
	if !found {
		t.Fatal("public node CA was not materialized in a candidate configuration volume")
	}
}
