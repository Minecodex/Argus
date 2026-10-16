package argusdev

import (
	"errors"
	"testing"

	"k8s.io/client-go/tools/portforward"
)

func TestPlan6GatewayReplacementKeepsEndpointAfterPodRetirement(t *testing.T) {
	old := &KubeForward{Ports: []portforward.ForwardedPort{{Local: 58900, Remote: 9443}}, stop: make(chan struct{}), done: make(chan error, 1)}
	old.done <- errors.New("lost connection to retired Pod")
	current := &KubeForward{Ports: append([]portforward.ForwardedPort{}, old.Ports...)}
	replacement, err := replacePlan6ConnectorForward(old, func(ports []string) (*KubeForward, error) {
		if len(ports) != 1 || ports[0] != "58900:9443" {
			t.Fatal("accepted endpoint changed", ports)
		}
		select {
		case <-old.stop:
		default:
			t.Fatal("retired stream was not stopped")
		}
		return current, nil
	})
	if err != nil || replacement != current {
		t.Fatal(replacement, err)
	}
	old = &KubeForward{Ports: []portforward.ForwardedPort{{Local: 58900, Remote: 9443}}, stop: make(chan struct{}), done: make(chan error, 1)}
	old.done <- nil
	wrong := &KubeForward{Ports: []portforward.ForwardedPort{{Local: 58901, Remote: 9443}}, stop: make(chan struct{}), done: make(chan error, 1)}
	wrong.done <- nil
	if _, err := replacePlan6ConnectorForward(old, func([]string) (*KubeForward, error) { return wrong, nil }); err == nil {
		t.Fatal("replacement allowed a different local port")
	}
}
