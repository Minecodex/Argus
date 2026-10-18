package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Docker Desktop runs Kubernetes in its own Docker network. This owned
// transport exposes temporary NodePorts to the Bastion's outbound network;
// it forwards opaque TLS bytes and never supplies telemetry or model results.
func (a *App) plan6DockerIngressBridge(ctx context.Context, env *E2EEnvironment, f *plan6DockerFixture) (string, error) {
	outside := ""
	for _, n := range f.Networks {
		if !n.Internal {
			outside = n.Name
		}
	}
	nodes, err := env.Kube.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil || len(nodes.Items) != 1 {
		return "", fmt.Errorf("Docker host bridge requires the single owned desktop node: %w", err)
	}
	node := nodes.Items[0]
	raw, err := a.runner.Output(ctx, nil, "docker", "inspect", node.Name)
	if err != nil {
		return "", err
	}
	var peers []struct {
		ID              string `json:"Id"`
		NetworkSettings struct {
			Networks map[string]struct {
				NetworkID string `json:"NetworkID"`
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if json.Unmarshal([]byte(raw), &peers) != nil || len(peers) != 1 {
		return "", fmt.Errorf("desktop node Docker identity missing")
	}
	shared, ip := "", ""
	for _, v := range peers[0].NetworkSettings.Networks {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && address.Address == v.IPAddress {
				shared, ip = v.NetworkID, v.IPAddress
			}
		}
	}
	if shared == "" || net.ParseIP(ip) == nil {
		return "", fmt.Errorf("desktop node IP/network does not match Kubernetes")
	}
	ports := map[int32]int32{}
	ingressService := "argus-e2e-ingress"
	if configured := env.State.Values["host_ingress_service"]; configured != "" {
		ingressService = configured
	}
	for _, item := range []struct {
		namespace, name string
		port            int32
	}{{env.IngressNS, ingressService, 443}, {env.SystemNS, "argus-connector-gateway-public", 9443}, {env.ObservNS, "argus-telemetry-ingest", 4317}, {env.ObservNS, "argus-telemetry-ingest", 4318}} {
		original, err := env.Kube.Client.CoreV1().Services(item.namespace).Get(ctx, item.name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		var target corev1.ServicePort
		for _, p := range original.Spec.Ports {
			if p.Port == item.port {
				target = p
			}
		}
		if target.Port == 0 {
			return "", fmt.Errorf("temporary service port missing")
		}
		target.NodePort = 0
		name := kubernetesNameForDev(fmt.Sprintf("docker-host-%s-%s-%d", env.Options.RunID, item.name, item.port))
		created, err := env.Kube.Client.CoreV1().Services(item.namespace).Create(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID}}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, Selector: original.Spec.Selector, Ports: []corev1.ServicePort{target}}}, metav1.CreateOptions{})
		if err != nil {
			return "", err
		}
		ports[item.port] = created.Spec.Ports[0].NodePort
	}
	name := kubernetesNameForDev("argus-host-bridge-" + env.Options.RunID)
	script := ""
	for _, port := range []int32{443, 9443, 4317, 4318} {
		script += fmt.Sprintf("socat TCP-LISTEN:%d,fork,reuseaddr TCP:%s:%d & ", port, ip, ports[port])
	}
	script += "wait"
	image := f.RuntimeImage
	if image == "" {
		image = "argus-host-loop-os:20261011-01"
	}
	cid, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", name, "--label", "io.argus.e2e.run="+f.Owner, "--network", outside, image, "/bin/bash", "-lc", script)
	if err != nil {
		return "", err
	}
	cid = strings.TrimSpace(cid)
	env.State.Values["host_docker_bridge_id"] = cid
	env.State.Values["host_docker_bridge_owner"] = f.Owner
	if err = a.runner.Run(ctx, nil, "docker", "network", "connect", shared, cid); err != nil {
		return "", err
	}
	inspection, err := a.plan6InspectDocker(ctx, cid, f.Owner)
	if err != nil {
		return "", err
	}
	bridge := inspection.NetworkSettings.Networks[outside].IP
	if net.ParseIP(bridge) == nil {
		return "", fmt.Errorf("bridge has no owned outbound address")
	}
	proof, _ := json.MarshalIndent(map[string]any{"container_id": cid, "owner": f.Owner, "desktop_node_uid": node.UID, "desktop_node_container_id": peers[0].ID, "shared_network_id": shared, "shared_network_delete_allowed": false, "node_ports": ports, "outbound_address": bridge, "tls_termination": false, "synthetic_metric_generation": false}, "", "  ")
	return bridge, writePrivate(filepath.Join(env.Options.Artifacts, "host-docker-ingress-bridge.json"), proof)
}
