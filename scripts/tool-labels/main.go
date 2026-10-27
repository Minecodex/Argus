// Generate UI names from the same public native registrations used by the
// server and worker. No business handler is executed and no credentials load.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/workspace"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--catalog" {
		catalog, err := publicCatalog()
		if err != nil {
			panic(err)
		}
		rows := make([]map[string]any, 0, len(catalog))
		for _, metadata := range catalog {
			binding, e := presentation.ToolBinding(metadata.ID)
			if e != nil {
				panic(e)
			}
			row := map[string]any{"id": metadata.ID, "title": metadata.Discovery.Title, "risk": metadata.Risk, "has_template": metadata.Template != nil}
			row["entry"], row["mode"], row["read_operations"] = binding.Entry, binding.Mode, binding.Reads
			if metadata.Template != nil {
				row["template_version"] = metadata.Template.Version
				row["template_hash"] = metadata.Template.Hash
				row["runtime"] = metadata.Template.Runtime
			}
			rows = append(rows, row)
		}
		encoded, _ := json.MarshalIndent(rows, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	path := "web/packages/api-client/src/generated/tool-labels.ts"
	check := len(os.Args) > 1 && os.Args[1] == "--check"
	if len(os.Args) > 1 && !check {
		path = os.Args[1]
	}
	text, err := generate()
	if err != nil {
		panic(err)
	}
	if check {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(bytes.ReplaceAll(current, []byte("\r\n"), []byte("\n")), text) {
			panic("generated tool labels are stale; run go run ./scripts/tool-labels")
		}
		return
	}
	if err := os.WriteFile(path, text, 0644); err != nil {
		panic(err)
	}
}

func publicCatalog() ([]mcp.Metadata, error) {
	registry := mcp.NewRegistry()
	store := &postgres.Store{}
	base := toolgateway.ResourceTools{Store: store}
	registers := []func(*mcp.Registry) error{
		(toolgateway.MonitoringTools{Base: base, Monitoring: monitoring.Service{Store: store}, Kubernetes: kubernetesworkbench.Service{Store: store}}).Register,
		base.Register,
		(toolgateway.LifecycleTools{Base: base}).Register,
		(toolgateway.CollectorPreviewTools{Base: base}).Register,
		(telemetry.Tools{Service: telemetry.Service{Store: store}}).Register,
		(workspace.Service{}).RegisterTools,
		(toolgateway.DashboardTools{Service: dashboard.Service{Store: store}}).Register,
		(alerting.Tools{Service: alerting.Service{Store: store}}).Register,
	}
	for _, register := range registers {
		if err := register(registry); err != nil {
			return nil, err
		}
	}
	catalog := registry.ModelCatalog()
	for _, metadata := range catalog {
		binding, err := presentation.ToolBinding(metadata.ID)
		if err != nil {
			return nil, err
		}
		if binding.Mode == "template" && (metadata.Template == nil || metadata.Present == nil) {
			return nil, fmt.Errorf("tool %s lacks its declared template", metadata.ID)
		}
		if binding.Mode != "template" && metadata.Template != nil {
			return nil, fmt.Errorf("tool %s duplicates a %s delivery", metadata.ID, binding.Mode)
		}
	}
	return catalog, nil
}

func generate() ([]byte, error) {
	catalog, err := publicCatalog()
	if err != nil {
		return nil, err
	}
	zh, en := map[string]string{}, map[string]string{}
	for _, metadata := range catalog {
		left, right, ok := strings.Cut(metadata.Discovery.Title, " / ")
		if !ok || strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
			return nil, fmt.Errorf("public tool %s needs a Chinese / English discovery title: %s", metadata.ID, metadata.Discovery.Title)
		}
		// Slashes keep dotted protocol IDs as one i18next leaf. Query and
		// query.get therefore coexist without a translation tree collision.
		key := strings.ReplaceAll(metadata.ID, ".", "/")
		zh[key], en[key] = strings.TrimSpace(left), strings.TrimSpace(right)
	}
	zhJSON, _ := json.MarshalIndent(zh, "", "  ")
	enJSON, _ := json.MarshalIndent(en, "", "  ")
	return []byte(fmt.Sprintf("/** Generated from public native Tool Discovery titles. DO NOT EDIT. */\nexport const nativeToolNamesZh: Record<string, string> = %s;\nexport const nativeToolNamesEn: Record<string, string> = %s;\n", zhJSON, enJSON)), nil
}
