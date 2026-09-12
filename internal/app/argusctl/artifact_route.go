package argusctl

import (
	"context"
	"fmt"
	"net"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Discover the HTTPS Service of the selected ingress controller rather than
// freezing a workstation/LAN address into every runtime Pod.
func httpsInternalAddress(ctx context.Context, clients *kubeClients, cfg *InstallConfig) (string, error) {
	if address := cfg.Spec.Exposure.HTTPSInternalAddress; address != "" {
		host, port, err := net.SplitHostPort(address)
		number, portErr := strconv.Atoi(port)
		if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 {
			return "", fmt.Errorf("spec.exposure.httpsInternalAddress must be host:port")
		}
		return address, nil
	}
	class, err := clients.typed.NetworkingV1().IngressClasses().Get(ctx, cfg.Spec.Exposure.IngressClassName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	if class.Spec.Controller != "k8s.io/ingress-nginx" {
		return "", fmt.Errorf("configure spec.exposure.httpsInternalAddress for ingress controller %s", class.Spec.Controller)
	}
	selector := "app.kubernetes.io/name=ingress-nginx,app.kubernetes.io/component=controller"
	if instance := class.Labels["app.kubernetes.io/instance"]; instance != "" {
		selector += ",app.kubernetes.io/instance=" + instance
	}
	namespace := class.Annotations["meta.helm.sh/release-namespace"]
	services, err := clients.typed.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", err
	}
	var addresses []string
	for _, service := range services.Items {
		for _, port := range service.Spec.Ports {
			if port.Name == "https" && port.TargetPort.StrVal != "webhook" {
				addresses = append(addresses, net.JoinHostPort(service.Name+"."+service.Namespace+".svc", strconv.Itoa(int(port.Port))))
			}
		}
	}
	if len(addresses) != 1 {
		return "", fmt.Errorf("found %d ingress HTTPS entries; configure spec.exposure.httpsInternalAddress", len(addresses))
	}
	return addresses[0], nil
}
