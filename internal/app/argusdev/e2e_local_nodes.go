package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type localE2ENetwork struct {
	NetworkID string `json:"NetworkID"`
	IPAddress string `json:"IPAddress"`
}

type localE2ENode struct {
	ID     string `json:"Id"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	NetworkSettings struct {
		Networks map[string]localE2ENetwork `json:"Networks"`
	} `json:"NetworkSettings"`
}

type localKindReceipt struct {
	Schema    string `json:"schema_version"`
	RunID     string `json:"run_id"`
	Workspace string `json:"workspace"`
	Nodes     []struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"nodes"`
}

func (a *App) localE2EDockerNode(ctx context.Context, env *E2EEnvironment, name string) (localE2ENode, error) {
	var result localE2ENode
	var receipt *localKindReceipt
	if env.Options.KubeContext == "docker-desktop" {
		if name != "desktop-control-plane" {
			return result, errors.New("Docker Desktop node differs from the approved local node")
		}
	} else {
		cluster := strings.TrimPrefix(env.Options.KubeContext, "kind-")
		if env.Options.Suite != "plan8" || !regexp.MustCompile(`^plan8-[a-z0-9-]{3,63}$`).MatchString(cluster) || !strings.HasPrefix(name, cluster+"-") {
			return result, errors.New("local node transport requires an owned Plan8 Kind cluster")
		}
		path := filepath.Join(a.root, "artifacts", cluster, "owner.json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return result, errors.New("owned Kind node receipt unavailable")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return result, errors.New("owned Kind node receipt invalid")
		}
		parsed, err := decodeLocalKindReceipt(raw)
		if err != nil {
			return result, err
		}
		receipt = &parsed
	}
	raw, err := a.runner.Output(ctx, nil, "docker", "inspect", name)
	if err != nil {
		return result, err
	}
	var inspected []localE2ENode
	if json.Unmarshal([]byte(raw), &inspected) != nil || len(inspected) != 1 {
		return result, errors.New("local node Docker identity missing")
	}
	result = inspected[0]
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(result.ID) {
		return result, errors.New("local node Docker ID invalid")
	}
	if receipt != nil {
		if err = validateOwnedKindNode(a.root, env.Options.KubeContext, name, result, *receipt); err != nil {
			return result, err
		}
	}
	return result, nil
}

func decodeLocalKindReceipt(raw []byte) (localKindReceipt, error) {
	var result localKindReceipt
	if len(raw) > 64<<10 || json.Unmarshal(bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf}), &result) != nil {
		return result, errors.New("owned Kind node receipt invalid")
	}
	return result, nil
}

func validateOwnedKindNode(root, contextName, name string, actual localE2ENode, receipt localKindReceipt) error {
	cluster := strings.TrimPrefix(contextName, "kind-")
	if receipt.Schema != "argus.plan8_network_owner/v1" || receipt.RunID != cluster || !strings.EqualFold(filepath.Clean(receipt.Workspace), filepath.Clean(root)) || actual.Config.Labels["io.x-k8s.kind.cluster"] != cluster {
		return errors.New("Kind receipt or runtime belongs to another environment")
	}
	matches := 0
	for _, node := range receipt.Nodes {
		if node.Name == name {
			matches++
			if node.ID != actual.ID || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(node.ID) {
				return errors.New("Kind node Docker identity changed")
			}
		}
	}
	if matches != 1 {
		return errors.New("Kind node is absent or duplicated in the ownership receipt")
	}
	return nil
}

func localBridgeNodeCandidates(contextName string, nodes []corev1.Node) ([]corev1.Node, error) {
	if contextName == "docker-desktop" {
		if len(nodes) != 1 || nodes[0].Name != "desktop-control-plane" {
			return nil, errors.New("Docker Desktop bridge requires its single approved local node")
		}
	} else if !strings.HasPrefix(contextName, "kind-plan8-") {
		return nil, errors.New("Docker bridge context is not an approved local environment")
	}
	result := []corev1.Node{}
	for _, node := range nodes {
		if node.UID == "" || node.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				result = append(result, node)
			}
		}
	}
	if len(result) == 0 {
		return nil, errors.New("local Docker bridge has no verified ready Kubernetes node")
	}
	sort.Slice(result, func(i, j int) bool {
		_, leftControl := result[i].Labels["node-role.kubernetes.io/control-plane"]
		_, rightControl := result[j].Labels["node-role.kubernetes.io/control-plane"]
		if leftControl != rightControl {
			return !leftControl
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func localNodeSharedNetwork(node corev1.Node, actual localE2ENode) (string, string, error) {
	for _, network := range actual.NetworkSettings.Networks {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && address.Address == network.IPAddress && net.ParseIP(address.Address) != nil {
				if network.NetworkID == "" {
					return "", "", errors.New("local node shared network ID missing")
				}
				return network.NetworkID, network.IPAddress, nil
			}
		}
	}
	return "", "", fmt.Errorf("local node %s IP/network differs between Kubernetes and Docker", node.Name)
}
