package argusdev

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var installationTopicResource = schema.GroupVersionResource{Group: "kafka.strimzi.io", Version: "v1beta2", Resource: "kafkatopics"}

// The installation probe topic must finish deletion while its Topic Operator
// and Kafka broker are still live. Deleting Kafka first strands its finalizer.
func cleanupInstallationTopic(ctx context.Context, env *E2EEnvironment) error {
	if env.Kube == nil || env.Kube.Config == nil || env.ObservNS == "" {
		return nil
	}
	client, err := dynamic.NewForConfig(env.Kube.Config)
	if err != nil {
		return err
	}
	return deleteOwnedInstallationTopic(ctx, env.Kube.Client, client, env.ObservNS, env.ReleaseID)
}

func deleteOwnedInstallationTopic(ctx context.Context, kube kubernetes.Interface, client dynamic.Interface, namespace, release string) error {
	owner, err := kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if release == "" || owner.UID == "" || owner.Labels["argus.io/release-id"] != release {
		return fmt.Errorf("installation topic namespace is not owned by %s", release)
	}
	topics := client.Resource(installationTopicResource).Namespace(namespace)
	topic, err := topics.Get(ctx, "argus-installation-check", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	labels := topic.GetLabels()
	if topic.GetUID() == "" || labels["strimzi.io/cluster"] != "argus-kafka" || labels["app.kubernetes.io/managed-by"] != "Helm" {
		return fmt.Errorf("refusing foreign installation topic in %s", namespace)
	}
	uid, version := topic.GetUID(), topic.GetResourceVersion()
	if err = topics.Delete(ctx, topic.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	deadline := time.Now().Add(time.Minute)
	for {
		current, err := kube.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.UID != owner.UID || current.Labels["argus.io/release-id"] != release {
			return fmt.Errorf("installation topic namespace ownership changed")
		}
		currentTopic, err := topics.Get(ctx, topic.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if currentTopic.GetUID() != uid {
			return fmt.Errorf("installation topic identity changed")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("installation topic did not finish deletion before Kafka teardown")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
}
