package argusdev

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestInstallationTopicCleanupRequiresNamespaceAndTopicOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, release, cluster string
		allowed                bool
	}{
		{"owned", "test-release", "argus-kafka", true},
		{"foreign-namespace", "other-release", "argus-kafka", false},
		{"foreign-topic", "test-release", "foreign-kafka", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kube := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "owned", UID: "namespace-uid", Labels: map[string]string{"argus.io/release-id": tc.release}}})
			topic := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "kafka.strimzi.io/v1beta2", "kind": "KafkaTopic",
				"metadata": map[string]any{"name": "argus-installation-check", "namespace": "owned", "uid": "topic-uid", "resourceVersion": "9", "labels": map[string]any{"strimzi.io/cluster": tc.cluster, "app.kubernetes.io/managed-by": "Helm"}},
			}}
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), topic)
			deleted := false
			client.PrependReactor("delete", "kafkatopics", func(action kubetesting.Action) (bool, runtime.Object, error) {
				deleted = true
				options := action.(kubetesting.DeleteAction).GetDeleteOptions()
				if !tc.allowed || action.GetNamespace() != "owned" || options.Preconditions == nil || options.Preconditions.UID == nil || *options.Preconditions.UID != "topic-uid" || options.Preconditions.ResourceVersion == nil || *options.Preconditions.ResourceVersion != "9" {
					t.Fatal("topic cleanup omitted concrete ownership/identity")
				}
				return false, nil, nil
			})
			err := deleteOwnedInstallationTopic(context.Background(), kube, client, "owned", "test-release")
			if tc.allowed && (err != nil || !deleted) || !tc.allowed && (err == nil || deleted) {
				t.Fatal("topic cleanup ownership boundary", deleted, err)
			}
		})
	}
}
