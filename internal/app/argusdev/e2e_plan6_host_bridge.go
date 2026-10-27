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

// Local Kubernetes runs in a Docker network. This ownership-verified transport
// exposes temporary NodePorts to the Bastion's outbound network;
// it forwards opaque TLS bytes and never supplies telemetry or model results.
func (a *App) plan6DockerIngressBridge(ctx context.Context, env *E2EEnvironment, f *plan6DockerFixture) (string, error) {
	outside := ""
	for _, n := range f.Networks {
		if !n.Internal {
			outside = n.Name
		}
	}
	nodes, err := env.Kube.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list local bridge nodes: %w", err)
	}
	candidates, err := localBridgeNodeCandidates(env.Options.KubeContext, nodes.Items)
	if err != nil {
		return "", err
	}
	if outside == "" {
		return "", fmt.Errorf("owned Bastion outbound network missing")
	}
	node := candidates[0]
	actualNode, err := a.localE2EDockerNode(ctx, env, node.Name)
	if err != nil {
		return "", err
	}
	shared, ip, err := localNodeSharedNetwork(node, actualNode)
	if err != nil {
		return "", err
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
	proof, _ := json.MarshalIndent(map[string]any{"container_id": cid, "owner": f.Owner, "node_name": node.Name, "node_uid": node.UID, "node_container_id": actualNode.ID, "kubernetes_context": env.Options.KubeContext, "shared_network_id": shared, "shared_network_delete_allowed": false, "node_ports": ports, "outbound_address": bridge, "tls_termination": false, "synthetic_metric_generation": false}, "", "  ")
	return bridge, writePrivate(filepath.Join(env.Options.Artifacts, "host-docker-ingress-bridge.json"), proof)
}
