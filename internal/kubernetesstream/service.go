package kubernetesstream

import (
	"context"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ServiceEndpoints accepts only controller-owned EndpointSlices and explicit
// Pod references. ExternalName/IP-only endpoints cannot become TCP targets.
func ServiceEndpoints(ctx context.Context, api kubernetes.Interface, namespace string, service *corev1.Service, port int32) (map[string]int32, error) {
	slices, err := api.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{LabelSelector: "kubernetes.io/service-name=" + service.Name, Limit: 201})
	if err != nil {
		return nil, accessError(err)
	}
	if len(slices.Items) > 200 || slices.Continue != "" {
		return nil, kubernetesinventory.ErrBudget
	}
	return ResolveEndpoints(namespace, service, port, slices.Items)
}
func ResolveEndpoints(namespace string, service *corev1.Service, port int32, items []discoveryv1.EndpointSlice) (map[string]int32, error) {
	selected := ""
	found := false
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		return nil, kubernetesinventory.ErrUnsupported
	}
	for _, p := range service.Spec.Ports {
		if p.Port == port && (p.Protocol == "" || p.Protocol == corev1.ProtocolTCP) {
			selected = p.Name
			found = true
		}
	}
	if !found {
		return nil, kubernetesinventory.ErrInvalid
	}
	out := map[string]int32{}
	for _, slice := range items {
		owned := false
		for _, owner := range slice.OwnerReferences {
			if owner.Kind == "Service" && owner.APIVersion == "v1" && owner.UID == service.UID && owner.Name == service.Name && owner.Controller != nil && *owner.Controller {
				owned = true
			}
		}
		if !owned {
			continue
		}
		var native int32
		for _, p := range slice.Ports {
			name := ""
			if p.Name != nil {
				name = *p.Name
			}
			if p.Port != nil && name == selected && (p.Protocol == nil || *p.Protocol == corev1.ProtocolTCP) {
				native = *p.Port
			}
		}
		if native < 1 || native > 65535 {
			continue
		}
		for _, e := range slice.Endpoints {
			ref := e.TargetRef
			if e.Conditions.Ready != nil && !*e.Conditions.Ready || e.Conditions.Terminating != nil && *e.Conditions.Terminating || ref == nil || ref.Kind != "Pod" || ref.Namespace != namespace || ref.UID == "" || ref.Name == "" {
				continue
			}
			out[ref.Name+"/"+string(ref.UID)] = native
		}
	}
	return out, nil
}
func validateService(ctx context.Context, api kubernetes.Interface, target Target) error {
	service, err := api.CoreV1().Services(target.Pin.Namespace).Get(ctx, target.Service.Name, metav1.GetOptions{})
	if err != nil {
		return accessError(err)
	}
	if string(service.UID) != target.Service.UID || service.DeletionTimestamp != nil {
		return kubernetesinventory.ErrIdentityChanged
	}
	endpoints, err := ServiceEndpoints(ctx, api, target.Pin.Namespace, service, target.Service.Port)
	if err != nil {
		return err
	}
	if endpoints[target.Pin.Pod+"/"+target.Pin.PodUID] != target.Port {
		return kubernetesinventory.ErrIdentityChanged
	}
	return nil
}
