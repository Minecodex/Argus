package presentation

import (
	"embed"
	"encoding/json"
	"strings"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

// These local helpers are compiled into each immutable Tool-owned asset. The
// portal and network-isolated runtime do not select business fields or layouts.
//
//go:embed templates/common.js templates/common.css
var helpers embed.FS

func ToolAsset(id, source string) *toolruntime.TemplateAsset {
	binding, err := ToolBinding(id)
	if err != nil || binding.Mode != "template" {
		return nil
	}
	js, _ := helpers.ReadFile("templates/common.js")
	css, _ := helpers.ReadFile("templates/common.css")
	entry, _ := json.Marshal(binding.Entry)
	source = "<style>" + string(css) + "</style><script>" + string(js) + "\nwindow.ArgusView.entry=" + string(entry) + ";</script>" + source
	return AssetV2(source, "argus."+binding.Entry+"/v2", binding.Reads...)
}

func Bound(asset *toolruntime.TemplateAsset, data map[string]any, scope string, refs []toolruntime.ResourceRef) *toolruntime.Presentation {
	public := PublicSnapshot(data)
	return &toolruntime.Presentation{Template: asset.Source, Hash: asset.Hash, Version: asset.Version, Runtime: asset.Runtime, ReadOperations: asset.ReadOperations, Data: ViewData(public), Binding: map[string]any{"kind": "sealed_result", "data": public}, AuthorizationScope: scope, Status: "ready", Resources: refs}
}

func Title(value, locale string) string {
	left, right, ok := strings.Cut(value, " / ")
	if ok {
		if locale == "en-US" {
			return right
		}
		return left
	}
	return value
}
