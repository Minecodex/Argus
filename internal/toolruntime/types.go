// Package toolruntime defines the model-visible execution boundary. It has no
// business services, network clients, sandbox lifecycle, or renderer selection.
package toolruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Principal struct {
	EnterpriseID         uuid.UUID `json:"enterprise_id"`
	UserID               uuid.UUID `json:"user_id"`
	ConversationID       uuid.UUID `json:"conversation_id"`
	AuthorizationVersion int64     `json:"authorization_version"`
	Permissions          []string  `json:"-"`
}

func (p Principal) Allows(permissions ...string) bool {
	if slices.Contains(p.Permissions, "*") {
		return true
	}
	for _, permission := range permissions {
		if !slices.Contains(p.Permissions, permission) {
			return false
		}
	}
	return true
}

type Invocation struct {
	BeforeDispatch                 func(context.Context) error `json:"-"`
	Principal                      Principal
	RunID, StepID, ModelCallID, ID uuid.UUID
	CallID                         string
	Arguments                      map[string]any
	ReadOnly                       bool
}

type Presentation struct {
	AuthorizationScope string
	Status             string
	Template           string
	Hash               string
	Version            string
	Data               map[string]any
	Resources          []ResourceRef
}

type TemplateAsset struct {
	Source  string `json:"-"`
	Hash    string `json:"hash"`
	Version string `json:"version"`
	Runtime string `json:"runtime"`
}

type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Result struct {
	DataIsSchema bool
	Data         map[string]any
	Partial      bool
	ActionRef    string
	ToolID       string
	Presentation *Presentation
	Unknown      bool
}

type Definition struct {
	Model        modelprovider.Tool `json:"model"`
	Source       string             `json:"source"`
	Version      string             `json:"version"`
	ReadOnly     bool               `json:"read_only"`
	ConnectionID string             `json:"connection_id,omitempty"`
	SchemaHash   string             `json:"schema_hash,omitempty"`
}

type Tool struct {
	Definition
	Invoke func(context.Context, Invocation) (Result, error)
}

type Snapshot struct {
	SkillContexts []SkillContext       `json:"skill_contexts,omitempty"`
	Version       string               `json:"version"`
	NativeCatalog string               `json:"native_catalog"`
	WorkspaceID   string               `json:"workspace_id,omitempty"`
	SandboxStatus string               `json:"sandbox_status"`
	Definitions   []Definition         `json:"tools"`
	Connections   []ConnectionSnapshot `json:"connections"`
}

type ConnectionSnapshot struct {
	ID                string `json:"id"`
	Revision          int32  `json:"revision"`
	CredentialVersion int64  `json:"credential_version"`
	SchemaHash        string `json:"schema_hash"`
}

type Set struct {
	Snapshot      Snapshot
	tools         map[string]Tool
	schemas       map[string]*jsonschema.Schema
	Validate      func(context.Context, Principal) error
	AuthorizeTool func(context.Context, Principal, uuid.UUID, Definition) error
}

func NewSet(snapshot Snapshot, tools []Tool) (*Set, error) {
	set := &Set{Snapshot: snapshot, tools: make(map[string]Tool, len(tools)), schemas: make(map[string]*jsonschema.Schema, len(tools))}
	set.Snapshot.Version = "argus.model_tool_set/v1"
	set.Snapshot.Definitions = nil
	wireNames := map[string]bool{}
	sort.SliceStable(tools, func(i, j int) bool {
		a, b := sourceOrder(tools[i].Source), sourceOrder(tools[j].Source)
		if a != b {
			return a < b
		}
		return tools[i].Model.Name < tools[j].Model.Name
	})
	for _, tool := range tools {
		name := tool.Model.Name
		wire := modelprovider.WireToolName(name)
		if name == "" || tool.Invoke == nil || tool.Model.Schema == nil || wireNames[wire] {
			return nil, Error{Kind: "TOOL_CONFIGURATION_INVALID"}
		}
		wireNames[wire] = true
		schema, err := CompileInputSchema(tool.Model.Schema)
		if err != nil {
			return nil, err
		}
		set.schemas[name] = schema
		set.tools[name] = tool
		set.Snapshot.Definitions = append(set.Snapshot.Definitions, tool.Definition)
	}
	return set, nil
}

func sourceOrder(source string) int {
	switch source {
	case "argus":
		return 0
	case "sandbox":
		return 1
	default:
		return 2
	}
}

func (set *Set) Models() []modelprovider.Tool {
	models := make([]modelprovider.Tool, 0, len(set.Snapshot.Definitions))
	for _, definition := range set.Snapshot.Definitions {
		models = append(models, definition.Model)
	}
	return models
}

func (set *Set) Lookup(name string) (Tool, bool) { tool, ok := set.tools[name]; return tool, ok }

func (set *Set) Invoke(ctx context.Context, name string, call Invocation) (Result, error) {
	if set.Validate != nil {
		if err := set.Validate(ctx, call.Principal); err != nil {
			return Result{}, err
		}
	}
	tool, ok := set.tools[name]
	if !ok {
		return Result{}, Error{Kind: "TOOL_NOT_FOUND"}
	}
	if set.AuthorizeTool != nil {
		if err := set.AuthorizeTool(ctx, call.Principal, call.RunID, tool.Definition); err != nil {
			return Result{}, err
		}
	}
	if call.ReadOnly && !tool.ReadOnly {
		return Result{}, Error{Kind: "TOOL_READ_ONLY_REQUIRED"}
	}
	if err := set.schemas[name].Validate(call.Arguments); err != nil {
		return Result{}, Error{Kind: "TOOL_INPUT_INVALID", Message: "arguments do not match the tool schema"}
	}
	return tool.Invoke(ctx, call)
}

func (snapshot Snapshot) Hash() string {
	data, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type Factory interface {
	Build(context.Context, Principal) (*Set, error)
	Restore(context.Context, Principal, Snapshot) (*Set, error)
}

type Contribution struct {
	Tools         []Tool
	NativeCatalog string
	WorkspaceID   string
	SandboxStatus string
	Connections   []ConnectionSnapshot
}

type Provider interface {
	BuildTools(context.Context, Principal) (Contribution, error)
}

type CompositeFactory struct {
	Providers      []Provider
	ContextSources []SkillContextSource
	Validate       func(context.Context, Principal) error
	Filter         func(context.Context, Principal, []Tool) ([]Tool, error)
	AuthorizeTool  func(context.Context, Principal, uuid.UUID, Definition) error
}

func (factory CompositeFactory) Build(ctx context.Context, principal Principal) (*Set, error) {
	if factory.Validate != nil {
		if err := factory.Validate(ctx, principal); err != nil {
			return nil, err
		}
	}
	snapshot := Snapshot{SandboxStatus: "not_configured"}
	for _, source := range factory.ContextSources {
		blocks, err := source.LoadContext(ctx, principal)
		if err != nil {
			return nil, err
		}
		snapshot.SkillContexts = append(snapshot.SkillContexts, blocks...)
	}
	if err := ValidateSkillContexts(snapshot.SkillContexts); err != nil {
		return nil, err
	}
	var tools []Tool
	for _, provider := range factory.Providers {
		part, err := provider.BuildTools(ctx, principal)
		if err != nil {
			return nil, err
		}
		tools = append(tools, part.Tools...)
		snapshot.Connections = append(snapshot.Connections, part.Connections...)
		if part.NativeCatalog != "" {
			snapshot.NativeCatalog = part.NativeCatalog
		}
		if part.WorkspaceID != "" {
			snapshot.WorkspaceID = part.WorkspaceID
		}
		if part.SandboxStatus != "" {
			snapshot.SandboxStatus = part.SandboxStatus
		}
	}
	if factory.Filter != nil {
		var err error
		tools, err = factory.Filter(ctx, principal, tools)
		if err != nil {
			return nil, err
		}
	}
	if len(tools) > 128 {
		return nil, Error{Kind: "MODEL_TOOL_CAPACITY_EXCEEDED", Details: map[string]any{"tool_count": len(tools), "maximum_tools": 128}}
	}
	set, err := NewSet(snapshot, tools)
	if err != nil {
		return nil, err
	}
	set.Validate = factory.Validate
	set.AuthorizeTool = factory.AuthorizeTool
	return set, nil
}

// Restore keeps the Run's schemas immutable while making lost capabilities
// controlled tool errors. A sandbox outage must not remove native capabilities
// or terminate an otherwise valid conversation before the model can respond.
func (factory CompositeFactory) Restore(ctx context.Context, principal Principal, snapshot Snapshot) (*Set, error) {
	if snapshot.Version != "argus.model_tool_set/v1" || len(snapshot.Definitions) < 3 {
		return nil, Error{Kind: "TOOL_CONFIGURATION_INVALID"}
	}
	if factory.Validate != nil {
		if err := factory.Validate(ctx, principal); err != nil {
			return nil, err
		}
	}
	current := map[string]Tool{}
	catalog := ""
	for _, provider := range factory.Providers {
		part, _ := provider.BuildTools(ctx, principal)
		if part.NativeCatalog != "" {
			catalog = part.NativeCatalog
		}
		for _, tool := range part.Tools {
			current[tool.Model.Name] = tool
		}
	}
	tools := make([]Tool, 0, len(snapshot.Definitions))
	for _, definition := range snapshot.Definitions {
		tool, exists := current[definition.Model.Name]
		code := ""
		if !exists {
			code = "TOOL_VERSION_UNAVAILABLE"
			if definition.Source == "sandbox" {
				code = "SANDBOX_UNAVAILABLE"
			}
			if definition.Source == "external_mcp" {
				code = "MCP_CONNECTION_UNAVAILABLE"
			}
		}
		if exists && (tool.Version != definition.Version || tool.Source != definition.Source || tool.SchemaHash != definition.SchemaHash) {
			code = "TOOL_VERSION_UNAVAILABLE"
			if definition.Source == "external_mcp" {
				code = "MCP_SCHEMA_CHANGED"
			}
		}
		if definition.Source == "argus" && catalog != snapshot.NativeCatalog {
			code = "TOOL_VERSION_UNAVAILABLE"
		}
		if code != "" {
			failure := code
			tool = Tool{Invoke: func(context.Context, Invocation) (Result, error) { return Result{}, Error{Kind: failure} }}
		}
		tool.Definition = definition
		tools = append(tools, tool)
	}
	set, err := NewSet(snapshot, tools)
	if err != nil {
		return nil, err
	}
	set.Validate = factory.Validate
	set.AuthorizeTool = factory.AuthorizeTool
	return set, nil
}

type Error struct {
	Kind    string
	Message string
	Details map[string]any
}

func (err Error) Error() string {
	if err.Message != "" {
		return err.Kind + ": " + err.Message
	}
	return err.Kind
}
func (err Error) Code() string { return err.Kind }

func DecodeObject(value any) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, Error{Kind: "TOOL_INPUT_INVALID", Message: "arguments must be an object"}
	}
	return object, nil
}

func (snapshot Snapshot) ValidateCurrent(current Snapshot) error {
	if snapshot.Hash() != current.Hash() {
		return Error{Kind: "TOOL_VERSION_UNAVAILABLE", Message: fmt.Sprintf("tool snapshot %s is no longer available", snapshot.Hash())}
	}
	return nil
}
