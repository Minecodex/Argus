package tooltemplatefixture

import (
	"strings"

	"github.com/kakj-go/Argus/internal/hostinventory"
)

// These maps mirror the public DTOs built by ResourceTools, rather than using
// one invented host row for clusters, connectors and Kubernetes objects.
func resourceFixture(tool, entry string) map[string]any {
	var row map[string]any
	switch entry {
	case "host-list", "host-detail":
		row = map[string]any{"id": host, "name": "db-prod-01", "hostname": "db-prod-01.internal", "address": "10.0.10.24", "platform": "linux", "role": "managed_host", "control_path": "direct", "connection_status": "connected", "labels": map[string]string{"env": "production"}, "resource_version": 3}
		if entry == "host-detail" {
			row["inventory"] = hostinventory.Snapshot{SchemaVersion: hostinventory.SchemaVersion, CollectedAt: now, Hostname: "db-prod-01.internal", OS: "linux", OSVersion: "Ubuntu 24.04", Scope: "container", CPU: &hostinventory.CPU{LogicalCores: 8}, Memory: &hostinventory.Memory{TotalBytes: 16 << 30}, Disks: []hostinventory.Disk{}, Networks: []hostinventory.Network{}, GPUs: []hostinventory.GPU{}, Issues: []hostinventory.Issue{}}
		}
	case "cluster-list", "cluster-detail":
		row = map[string]any{"id": cluster, "name": "orders-production", "api_server": "https://kubernetes.example.test", "connection_mode": "direct", "status": "ready", "labels": map[string]string{"env": "production"}, "resource_version": 3}
	case "connector-list", "connector-detail":
		row = map[string]any{"id": connectorID, "name": "db-prod-connector", "role": "host", "status": "online", "connection_epoch": 7, "last_heartbeat_at": now, "host_id": host, "related_resources": []any{map[string]any{"id": host, "name": "db-prod-01", "type": "host", "status": "available"}}}
		row["software_version"] = "v1.2.3"
		if entry == "connector-detail" {
			row["capabilities"] = []string{"host_inventory_probe"}
			row["host_id"] = host
		}
	case "kubernetes-resource-list":
		kind := strings.Split(tool, ".")[1]
		row = map[string]any{"resource_type": kind, "name": "orders-api", "namespace": "orders", "labels": map[string]string{"app": "orders"}, "summary": map[string]any{"phase": "Running", "ready": "2/2"}}
	default:
		return nil
	}
	if strings.HasSuffix(entry, "-detail") {
		if entry == "connector-detail" {
			connectorContext(row)
		}
		return row
	}
	data := map[string]any{"items": []any{row}}
	if entry == "connector-list" {
		connectorContext(data)
	}
	return data
}
