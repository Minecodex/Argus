package kubernetesstream

import (
	"context"
	"errors"
	"github.com/kakj-go/Argus/internal/interactive"
	"io"
	"testing"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testPod() *corev1.Pod {
	no := false
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-1"}, Spec: corev1.PodSpec{AutomountServiceAccountToken: &no, Containers: []corev1.Container{{Name: "main", Image: "alpine:3.21"}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://one", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}}}
}

func TestTerminalStateSurvivesFullOutputBacklog(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &Session{ctx: ctx, cancel: cancel, events: make(chan interactive.Frame, 2), finished: make(chan struct{}), connections: map[uint32]io.ReadWriteCloser{}}
	s.events <- interactive.Frame{Type: "output", Data: []byte("one")}
	s.events <- interactive.Frame{Type: "output", Data: []byte("two")}
	s.finish("invalidated", "KUBERNETES_OBJECT_IDENTITY_CHANGED")
	for range 2 {
		f, e := s.Receive(t.Context())
		if e != nil || f.Type != "output" {
			t.Fatal("lost buffered output", e)
		}
	}
	f, e := s.Receive(t.Context())
	if e != nil || f.Status != "invalidated" {
		t.Fatal("lost terminal reason under backpressure", f, e)
	}
	if _, e = s.Receive(t.Context()); e != io.EOF {
		t.Fatal("terminal state delivered more than once", e)
	}
}
func testTarget(p *corev1.Pod) Target {
	pin, _ := kubernetespodlogs.PinPod("namespace-1", p, "main", false)
	pin.PreviousContainerID = ""
	return Target{SchemaVersion: SchemaVersion, APIServer: "https://kubernetes.example.test", Mode: "exec", Shell: "sh", Pin: pin, SecurityFingerprint: SecurityFingerprint(p)}
}
func TestTargetPinsGenerationSecurityAndNamespace(t *testing.T) {
	p := testPod()
	target := testTarget(p)
	api := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders", UID: "namespace-1"}}, p)
	if err := ValidateTarget(t.Context(), api, target); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*corev1.Pod)
	}{
		{"recreated", func(p *corev1.Pod) { p.UID = "pod-2" }},
		{"restarted", func(p *corev1.Pod) {
			p.Status.ContainerStatuses[0].RestartCount++
			p.Status.ContainerStatuses[0].ContainerID = "containerd://two"
		}},
		{"service_account", func(p *corev1.Pod) { p.Spec.ServiceAccountName = "admin" }},
		{"privilege", func(p *corev1.Pod) { p.Spec.HostNetwork = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := p.DeepCopy()
			tc.mutate(changed)
			_, _ = api.CoreV1().Pods("orders").Update(t.Context(), changed, metav1.UpdateOptions{})
			if err := ValidateTarget(t.Context(), api, target); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
				t.Fatalf("changed target accepted: %v", err)
			}
		})
	}
	_, _ = api.CoreV1().Pods("orders").Update(t.Context(), p, metav1.UpdateOptions{})
	_, _ = api.CoreV1().Namespaces().Update(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders", UID: "namespace-2"}}, metav1.UpdateOptions{})
	if err := ValidateTarget(context.Background(), api, target); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
		t.Fatal(err)
	}
}
func TestForwardPacketBoundsAndServiceOwnership(t *testing.T) {
	payload := []byte{0, 1, 255, 128}
	packet := Packet(ChannelData, 2, payload)
	kind, id, data, e := ParsePacket(packet)
	if e != nil || kind != ChannelData || id != 2 || string(data) != string(payload) {
		t.Fatal("binary bytes changed")
	}
	for _, bad := range [][]byte{nil, Packet(OpenChannel, 0, nil), Packet(OpenChannel, 1, []byte{1}), Packet(ChannelData, 1, nil), Packet(ChannelData, 1, make([]byte, MaxFrameBytes))} {
		if _, _, _, err := ParsePacket(bad); err == nil {
			t.Fatal("invalid packet accepted")
		}
	}
	port, name, yes := int32(18080), "tcp", true
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "orders", UID: "svc-1"}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: name, Port: 80, Protocol: corev1.ProtocolTCP}}}}
	slice := discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Service", Name: "backend", UID: "svc-1", Controller: &yes}}}, Ports: []discoveryv1.EndpointPort{{Name: &name, Port: &port}}, Endpoints: []discoveryv1.Endpoint{{TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "api", Namespace: "orders", UID: "pod-1"}, Conditions: discoveryv1.EndpointConditions{Ready: &yes}}}}
	got, err := ResolveEndpoints("orders", service, 80, []discoveryv1.EndpointSlice{slice})
	if err != nil || got["api/pod-1"] != 18080 {
		t.Fatal(got, err)
	}
	slice.OwnerReferences[0].UID = "replaced-service"
	got, err = ResolveEndpoints("orders", service, 80, []discoveryv1.EndpointSlice{slice})
	if err != nil || len(got) != 0 {
		t.Fatal("unowned endpoints accepted")
	}
	service.Spec.Type = corev1.ServiceTypeExternalName
	if _, err = ResolveEndpoints("orders", service, 80, nil); !errors.Is(err, kubernetesinventory.ErrUnsupported) {
		t.Fatal("external name accepted")
	}
}
