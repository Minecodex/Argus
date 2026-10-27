package collectormanager

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	corev1 "k8s.io/api/core/v1"
)

func TestKubernetesLogCheckpointMountIsCollectorScoped(t *testing.T) {
	command := collectorCommand(t, testConfigBundle(t), "linux_amd64", "https://artifacts.example/collector.tar.gz", []byte("artifact"))
	var bundle configbundle.Bundle
	if err := json.Unmarshal(command.RenderedConfig, &bundle); err != nil {
		t.Fatal(err)
	}
	bundle.Collections = []configbundle.Collection{{ID: uuid.NewString(), Integration: "k8slog", Enabled: true}}
	command.RenderedConfig, _ = json.Marshal(bundle)
	agent := collectorPodTemplate(command, "collector:test", configSlotBase, "argus-otelcol-agent")
	container := agent.Spec.Containers[0]
	if *agent.Spec.SecurityContext.RunAsUser != 0 || *agent.Spec.SecurityContext.RunAsNonRoot {
		t.Fatal("node log reader cannot access root-owned CRI files")
	}
	if container.SecurityContext == nil || *container.SecurityContext.AllowPrivilegeEscalation || !*container.SecurityContext.ReadOnlyRootFilesystem || !slices.Equal(container.SecurityContext.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Fatal("node reader retained excessive process privileges")
	}
	if !slices.ContainsFunc(agent.Spec.Volumes, func(v corev1.Volume) bool {
		return v.Name == "log-checkpoints" && v.HostPath != nil && v.HostPath.Path == "/var/lib/argus-otelcol/"+command.GetCollectorId()+"/log-checkpoints"
	}) {
		t.Fatal("checkpoint is not per-Collector persistent state")
	}
	for _, mount := range container.VolumeMounts {
		if slices.Contains([]string{"hostfs", "podlogs", "kubelet-pki"}, mount.Name) && !mount.ReadOnly {
			t.Fatalf("host mount %s became writable", mount.Name)
		}
	}
	gateway := collectorPodTemplate(command, "collector:test", configSlotBase, "argus-otelcol-gateway")
	if *gateway.Spec.SecurityContext.RunAsUser == 0 || slices.ContainsFunc(gateway.Spec.Volumes, func(v corev1.Volume) bool { return v.Name == "log-checkpoints" }) {
		t.Fatal("Gateway inherited node access")
	}
	bundle.Collections[0].Enabled = false
	command.RenderedConfig, _ = json.Marshal(bundle)
	stopped := collectorPodTemplate(command, "collector:test", configSlotBase, "argus-otelcol-agent")
	if *stopped.Spec.SecurityContext.RunAsUser == 0 || slices.ContainsFunc(stopped.Spec.Volumes, func(v corev1.Volume) bool { return v.Name == "log-checkpoints" }) {
		t.Fatal("metrics-only agent retained log access")
	}
}
