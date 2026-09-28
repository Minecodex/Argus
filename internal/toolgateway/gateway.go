// Package toolgateway discovers and invokes Argus-owned business capabilities.
// Customer MCP tools and sandbox builtins are separate toolruntime providers.
package toolgateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var Categories = []string{"host", "k8s", "metric", "trace", "log", "connector", "workflow", "dashboard"}

type Gateway struct {
	Scope     func(context.Context, toolruntime.Invocation) (func(string) bool, error)
	cache     discoveryCache
	Registry  *mcp.Registry
	manifests []Manifest
	byName    map[string]Manifest
	schemas   map[string]*jsonschema.Schema
	Revision  string
}

func (g *Gateway) BuildTools(context.Context, toolruntime.Principal) (toolruntime.Contribution, error) {
	return toolruntime.Contribution{Tools: g.CoreTools(), NativeCatalog: g.Revision}, nil
}

func New(registry *mcp.Registry, implementationRevision string) (*Gateway, error) {
	if implementationRevision == "" {
		return nil, fmt.Errorf("native implementation revision is required")
	}
	g := &Gateway{Registry: registry, byName: map[string]Manifest{}, schemas: map[string]*jsonschema.Schema{}}
	if registry == nil {
		return nil, fmt.Errorf("native registry is required")
	}
	for _, metadata := range registry.ModelCatalog() {
		category, name, ok := identity(metadata.ID)
		if !ok {
			return nil, fmt.Errorf("tool %s has no native category", metadata.ID)
		}
		discovery, discoveryHash, err := mcp.NormalizeDiscovery(metadata.Discovery)
		if err != nil {
			return nil, fmt.Errorf("tool %s discovery: %w", metadata.ID, err)
		}
		version := metadata.InputVersion + ":" + metadata.OutputVersion + ":" + discoveryHash
		if metadata.Template != nil {
			version += ":" + metadata.Template.Hash
		}
		manifest := Manifest{SchemaVersion: "argus.tool_manifest/v1", Category: category, Name: name, Version: version, Discovery: discovery, DiscoveryHash: discoveryHash,
			Risk: metadata.Risk, Required: append([]string{}, metadata.Required...), AnyRequired: append([]string{}, metadata.AnyRequired...),
			InputSchema: metadata.InputSchema, OutputSchema: metadata.OutputSchema, ToolID: metadata.ID,
			RequiresConfirmation: strings.HasSuffix(metadata.ID, ".preview")}
		if err := validateManifest(manifest); err != nil {
			return nil, fmt.Errorf("tool %s manifest: %w", metadata.ID, err)
		}
		key := category + "/" + name
		if _, exists := g.byName[key]; exists {
			return nil, fmt.Errorf("duplicate native tool %s", key)
		}
		schema, err := toolruntime.CompileInputSchema(manifest.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool schema %s: %w", key, err)
		}
		g.schemas[key] = schema
		g.byName[key] = manifest
		g.manifests = append(g.manifests, manifest)
	}
	sort.Slice(g.manifests, func(i, j int) bool {
		a, b := g.manifests[i], g.manifests[j]
		return a.Category+"/"+a.Name < b.Category+"/"+b.Name
	})
	encoded, _ := json.Marshal([]any{implementationRevision, g.manifests})
	hash := sha256.Sum256(encoded)
	g.Revision = hex.EncodeToString(hash[:])
	return g, nil
}

func identity(id string) (string, string, bool) {
	switch id {
	case "telemetry.promql.query":
		return "metric", "query", true
	case "telemetry.kql.query":
		return "log", "query", true
	case "telemetry.skywalking.trace":
		return "trace", "query", true
	case "telemetry.overview":
		return "metric", "overview", true
	case "telemetry.collector.list":
		return "connector", "collector.list", true
	case "telemetry.collector.get":
		return "connector", "collector.get", true
	}
	for _, prefix := range []struct{ prefix, category string }{
		{"telemetry.dashboard.", "dashboard"},
		{"host.", "host"}, {"kubernetes.", "k8s"}, {"connector.", "connector"},
		{"telemetry.metric.", "metric"}, {"telemetry.metrics.", "metric"}, {"metric.", "metric"},
		{"telemetry.trace.", "trace"}, {"telemetry.traces.", "trace"}, {"trace.", "trace"},
		{"telemetry.log.", "log"}, {"telemetry.logs.", "log"}, {"log.", "log"},
		{"pending_action.", "workflow"}, {"workflow.", "workflow"},
	} {
		if strings.HasPrefix(id, prefix.prefix) {
			return prefix.category, strings.TrimPrefix(id, prefix.prefix), true
		}
	}
	return "", "", false
}

func (g *Gateway) CoreTools() []toolruntime.Tool {
	category := map[string]any{"type": "string", "enum": Categories}
	name := map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
	schemas := map[string]map[string]any{
		"tool.search":   object([]string{"category"}, map[string]any{"category": category, "query": map[string]any{"type": "string", "maxLength": 256}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}, "cursor": map[string]any{"type": "string", "maxLength": 1024}}),
		"tool.describe": object([]string{"category", "name"}, map[string]any{"category": category, "name": name}),
		"tool.invoke":   object([]string{"category", "name", "arguments"}, map[string]any{"category": category, "name": name, "arguments": map[string]any{"type": "object"}}),
	}
	result := make([]toolruntime.Tool, 0, 3)
	for _, name := range []string{"tool.search", "tool.describe", "tool.invoke"} {
		name := name
		result = append(result, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: name, Schema: schemas[name], Description: coreDescription(name)}, Source: "argus", Version: "1", ReadOnly: true},
			Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
				return g.execute(ctx, name, call)
			}})
	}
	return result
}

func object(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}

func coreDescription(name string) string {
	switch name {
	case "tool.search":
		return "Search available Argus tools in an explicit category. Returns summaries only."
	case "tool.describe":
		return "Describe an Argus tool's input schema and result. Describe before its first invocation."
	default:
		return "Invoke a described Argus tool. Business mutations return a preview for user confirmation; commit is never accessible."
	}
}

func (g *Gateway) execute(ctx context.Context, tool string, call toolruntime.Invocation) (toolruntime.Result, error) {
	allow := func(string) bool { return true }
	if g.Scope != nil {
		var err error
		allow, err = g.Scope(ctx, call)
		if err != nil {
			return toolruntime.Result{}, err
		}
	}
	category, _ := call.Arguments["category"].(string)
	if !slices.Contains(Categories, category) {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_CATEGORY_UNKNOWN"}
	}
	if tool == "tool.search" {
		key := discoveryKey(g.Revision, tool, call)
		if cached, ok := g.cache.get(key); ok {
			return cached, nil
		}
		value, err := g.search(call, category, allow)
		if err == nil {
			g.cache.put(key, value)
		}
		return value, err
	}
	name, _ := call.Arguments["name"].(string)
	key := category + "/" + name
	manifest, ok := g.byName[key]
	if !ok || !manifest.allowed(call.Principal) || !allow(manifest.ToolID) {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_NOT_FOUND"}
	}
	if tool == "tool.describe" {
		key := discoveryKey(g.Revision, tool, call)
		if cached, ok := g.cache.get(key); ok {
			return cached, nil
		}
		encoded, _ := json.Marshal(manifest)
		var data map[string]any
		_ = json.Unmarshal(encoded, &data)
		value := toolruntime.Result{Data: data, ToolID: tool, DataIsSchema: true}
		g.cache.put(key, value)
		return value, nil
	}
	args, err := toolruntime.DecodeObject(call.Arguments["arguments"])
	if err != nil {
		return toolruntime.Result{}, err
	}
	if err := g.schemas[key].Validate(args); err != nil {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID", Message: "arguments do not match the tool schema"}
	}
	if call.ReadOnly && manifest.Risk != "read" {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_READ_ONLY_REQUIRED"}
	}
	if manifest.Risk != "read" && !manifest.RequiresConfirmation && !slices.Contains([]string{"pending_action.cancel", "workflow.import_result", "workflow.publish_file", "telemetry.dashboard.query.cancel", "telemetry.dashboard.query.resume", "telemetry.dashboard.draft.create", "telemetry.dashboard.draft.save", "telemetry.dashboard.draft.drilldowns"}, manifest.ToolID) {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_NOT_FOUND"}
	}
	native := mcp.Call{ToolID: manifest.ToolID, CallID: call.CallID, Caller: "model", Enterprise: call.Principal.EnterpriseID.String(),
		Subject: call.Principal.UserID.String(), SubjectType: "user", RunID: call.RunID.String(), InvocationID: call.ID.String(), Input: args}
	result, err := g.Registry.Call(ctx, native)
	if err != nil {
		if errors.Is(err, mcp.ErrPermissionDenied) {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_PERMISSION_DENIED"}
		}
		if errors.Is(err, mcp.ErrInputInvalid) {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
		}
		if errors.Is(err, mcp.ErrToolNotAvailable) {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_NOT_FOUND"}
		}
		return toolruntime.Result{}, err
	}
	actionRef := ""
	if manifest.RequiresConfirmation {
		actionRef, _ = result.Structured["action_ref"].(string)
	}
	output := toolruntime.Result{Data: result.Structured, Partial: result.Partial, ActionRef: actionRef, ToolID: manifest.ToolID}
	if metadata, ok := g.Registry.Lookup(manifest.ToolID); ok && metadata.Present != nil {
		output.Presentation, _ = metadata.Present(ctx, native, result)
	}
	return output, nil
}

func (g *Gateway) search(call toolruntime.Invocation, category string, scopes ...func(string) bool) (toolruntime.Result, error) {
	query, _ := call.Arguments["query"].(string)
	query = normalizeSearch(query)
	limit := 10
	if value, ok := call.Arguments["limit"].(float64); ok {
		limit = int(value)
	}
	if limit < 1 || limit > 20 {
		return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
	}
	scopeBytes, _ := json.Marshal([]any{g.Revision, call.Principal.EnterpriseID, call.Principal.UserID, call.Principal.AuthorizationVersion, call.RunID, category, query})
	scopeHash := sha256.Sum256(scopeBytes)
	scope := hex.EncodeToString(scopeHash[:])
	offset := 0
	if cursor, _ := call.Arguments["cursor"].(string); cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(decoded), ":")
		if err != nil || len(parts) != 2 || parts[0] != scope {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
		}
		offset, err = strconv.Atoi(parts[1])
		if err != nil || offset < 0 {
			return toolruntime.Result{}, toolruntime.Error{Kind: "TOOL_INPUT_INVALID"}
		}
	}
	type hit struct {
		manifest Manifest
		score    int
	}
	hits := []hit{}
	for _, manifest := range g.manifests {
		if len(scopes) > 0 && !scopes[0](manifest.ToolID) {
			continue
		}
		if manifest.Category != category || !manifest.allowed(call.Principal) {
			continue
		}
		score, matched := discoveryScore(manifest, query)
		if !matched {
			continue
		}
		hits = append(hits, hit{manifest, score})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].manifest.Name < hits[j].manifest.Name
	})
	items := []map[string]any{}
	end := min(len(hits), offset+limit)
	for i := min(offset, len(hits)); i < end; i++ {
		m := hits[i].manifest
		items = append(items, map[string]any{"name": m.Name, "title": m.Title, "summary": discoverySummary(m.Description), "risk": m.Risk, "requires_confirmation": m.RequiresConfirmation, "version": m.Version})
	}
	var next any
	if end < len(hits) {
		next = base64.RawURLEncoding.EncodeToString([]byte(scope + ":" + strconv.Itoa(end)))
	}
	return toolruntime.Result{ToolID: "tool.search", Data: map[string]any{"category": category, "items": items, "next_cursor": next, "catalog_revision": g.Revision}}, nil
}
