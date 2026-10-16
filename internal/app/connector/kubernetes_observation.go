package connector

import (
	"context"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"time"
)

func observeKubernetesRuntime(parent context.Context) *connectorv1.KubernetesRuntimeObservation {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	configuration, err := rest.InClusterConfig()
	if err != nil {
		return nil
	}
	configuration.Timeout = 2 * time.Second
	client, err := kubernetes.NewForConfig(configuration)
	if err != nil {
		return nil
	}
	version, err := client.Discovery().ServerVersion()
	if err != nil {
		return nil
	}
	observation := &connectorv1.KubernetesRuntimeObservation{ServerVersion: version.GitVersion}
	continuation := ""
	for {
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 500, Continue: continuation})
		if err != nil {
			return nil
		}
		for _, node := range nodes.Items {
			observation.NodeCount++
			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					observation.ReadyNodeCount++
					break
				}
			}
		}
		if observation.NodeCount > 10000 {
			return nil
		}
		continuation = nodes.Continue
		if continuation == "" {
			break
		}
	}
	observation.ObservedAt = timestamppb.Now()
	return observation
}
