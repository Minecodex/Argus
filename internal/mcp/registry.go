package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"slices"
	"sort"
	"strings"
	"sync"
)

var (
	ErrToolNotAvailable = errors.New("tool is not available")
	ErrCommitCaller     = errors.New("commit tool requires action executor identity")
	ErrPermissionDenied = errors.New("tool permission denied")
	ErrInputInvalid     = errors.New("tool input invalid")
)

type Visibility string
type ExecutionMode string

const (
	Visible Visibility = "visible"
	Hidden  Visibility = "hidden"

	Sequential   ExecutionMode = "sequential"
	ParallelSafe ExecutionMode = "parallel_safe"
)

type Metadata struct {
	Discovery                Discovery
	ID                       string
	ToolFamily               string
	Risk                     string
	Visibility               Visibility
	ExecutionMode            ExecutionMode
	Required                 []string
	InputVersion             string
	OutputVersion            string
	ProjectionSchema         string
	MaxResultBytes           int
	InputSchema              map[string]any
	OutputSchema             map[string]any
	OutputSchemaHash         string
	CompatibleOutputVersions []string
	SemanticFields           map[string]string
	FieldTypes               map[string]string
	Template                 *toolruntime.TemplateAsset
	Present                  func(context.Context, Call, Result) (*toolruntime.Presentation, error)
	Authorize                func(context.Context, Call) error
	Validate                 func(map[string]any) error
	Execute                  func(context.Context, Call) (Result, error)
}

type Call struct {
	ToolID       string
	CallID       string
	Caller       string
	Enterprise   string
	Subject      string
	SubjectType  string
	RunID        string
	InvocationID string
	Input        map[string]any
}

type Result struct {
	Structured map[string]any
	Private    map[string]any
	ResultRef  string
	Partial    bool
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Metadata
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Metadata{}} }

func (registry *Registry) Register(metadata Metadata) error {
	if metadata.ID == "" || metadata.Execute == nil || metadata.MaxResultBytes <= 0 {
		return errors.New("tool metadata is incomplete")
	}
	if strings.HasSuffix(metadata.ID, ".commit") {
		metadata.Visibility = Hidden
		metadata.ExecutionMode = Sequential
	}
	if metadata.OutputSchema != nil && metadata.OutputSchemaHash == "" {
		encoded, err := json.Marshal(metadata.OutputSchema)
		if err != nil {
			return fmt.Errorf("marshal output schema for %s: %w", metadata.ID, err)
		}
		hash := sha256.Sum256(encoded)
		metadata.OutputSchemaHash = hex.EncodeToString(hash[:])
	}
	if metadata.ToolFamily == "" {
		metadata.ToolFamily = metadata.ID
	}
	if metadata.OutputVersion != "" && !slices.Contains(metadata.CompatibleOutputVersions, metadata.OutputVersion) {
		metadata.CompatibleOutputVersions = append(metadata.CompatibleOutputVersions, metadata.OutputVersion)
	}
	sort.Strings(metadata.CompatibleOutputVersions)
	if err := presentation.Validate(metadata.Template); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.tools[metadata.ID]; exists {
		return fmt.Errorf("tool %s already registered", metadata.ID)
	}
	registry.tools[metadata.ID] = metadata
	return nil
}

func (registry *Registry) ModelCatalog() []Metadata {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]Metadata, 0, len(registry.tools))
	for _, metadata := range registry.tools {
		if metadata.Visibility == Visible && !strings.HasSuffix(metadata.ID, ".commit") {
			metadata.Execute = nil
			metadata.Authorize = nil
			metadata.Validate = nil
			result = append(result, metadata)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (registry *Registry) Call(ctx context.Context, call Call) (Result, error) {
	registry.mu.RLock()
	metadata, exists := registry.tools[call.ToolID]
	registry.mu.RUnlock()
	if !exists {
		return Result{}, ErrToolNotAvailable
	}
	if strings.HasSuffix(call.ToolID, ".commit") && call.Caller != "action_executor" {
		return Result{}, ErrCommitCaller
	}
	if metadata.Authorize != nil {
		if err := metadata.Authorize(ctx, call); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrPermissionDenied, err)
		}
	}
	if metadata.Validate != nil {
		if err := metadata.Validate(call.Input); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrInputInvalid, err)
		}
	}
	return metadata.Execute(ctx, call)
}

func (registry *Registry) Lookup(id string) (Metadata, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	metadata, ok := registry.tools[id]
	return metadata, ok
}

func (registry *Registry) ValidatePairs() error {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	for id := range registry.tools {
		if strings.HasSuffix(id, ".preview") {
			commit := strings.TrimSuffix(id, ".preview") + ".commit"
			if _, exists := registry.tools[commit]; !exists {
				return fmt.Errorf("preview tool %s has no commit pair", id)
			}
		}
	}
	return nil
}
