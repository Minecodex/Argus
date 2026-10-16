// Package tooltemplatefixture builds controlled UI fixtures from the production
// registry and its exact immutable sources. It does not execute a model or query.
package tooltemplatefixture

import (
	"fmt"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
)

type Fixture struct {
	Tool, Entry, Mode, Hash, Version, DataType string
	Reads                                      []string
	Data                                       map[string]any
	Expected                                   []string
}
type Catalog struct {
	Assets    map[string]string                    `json:"assets"`
	Bindings  []Fixture                            `json:"bindings"`
	Cases     map[string]map[string]map[string]any `json:"cases"`
	Scenarios map[string]Fixture                   `json:"scenarios"`
}

func Build() (Catalog, error) {
	registry := mcp.NewRegistry()
	store := &postgres.Store{}
	base := toolgateway.ResourceTools{Store: store}
	for _, register := range []func(*mcp.Registry) error{base.Register, (toolgateway.LifecycleTools{Base: base}).Register, (toolgateway.CollectorPreviewTools{Base: base}).Register, (telemetry.Tools{Service: telemetry.Service{Store: store}}).Register, (workspace.Service{}).RegisterTools, (toolgateway.DashboardTools{Service: dashboard.Service{Store: store}}).Register, (alerting.Tools{Service: alerting.Service{Store: store}}).Register} {
		if err := register(registry); err != nil {
			return Catalog{}, err
		}
	}
	result := Catalog{Assets: map[string]string{}, Cases: map[string]map[string]map[string]any{}, Scenarios: map[string]Fixture{}}
	add := func(id, title, dataType string, asset *toolruntime.TemplateAsset) error {
		binding, err := presentation.ToolBinding(id)
		if err != nil {
			return err
		}
		if binding.Mode == "template" && asset == nil {
			return fmt.Errorf("%s missing declared template", id)
		}
		if binding.Mode == "template" && dataType == "" {
			return fmt.Errorf("%s missing result type/version declaration", id)
		}
		if err = presentation.Validate(asset); err != nil {
			return err
		}
		fixture := Fixture{Tool: id, Entry: binding.Entry, Mode: binding.Mode, DataType: dataType, Reads: binding.Reads, Data: presentation.BindData(id, title, NormalForTool(id, binding.Entry))}
		fixture.Data["observed_at"] = now
		fixture.Expected = Expected(binding.Entry, fixture.Data)
		if asset != nil {
			fixture.Hash, fixture.Version = asset.Hash, asset.Version
			result.Assets[asset.Hash] = asset.Source
			result.Cases[binding.Entry] = Variants(binding.Entry, fixture.Data)
		}
		result.Bindings = append(result.Bindings, fixture)
		return nil
	}
	for _, metadata := range registry.ModelCatalog() {
		if err := add(metadata.ID, metadata.Discovery.Title, metadata.OutputVersion, metadata.Template); err != nil {
			return result, err
		}
	}
	for _, id := range []string{"tool.search", "tool.describe", "tool.invoke"} {
		dataType := id + "/v2"
		if id == "tool.describe" {
			dataType = "argus.tool_manifest/v1"
		}
		if err := add(id, id, dataType, toolgateway.CoreTemplate(id)); err != nil {
			return result, err
		}
	}
	for name, scenario := range BusinessScenarios() {
		for _, binding := range result.Bindings {
			if binding.Tool == scenario.Tool {
				value := binding
				value.Data = presentation.BindData(value.Tool, value.Tool, scenario.Data)
				value.Expected = scenario.Expected
				result.Scenarios[name] = value
				break
			}
		}
	}
	return result, nil
}
