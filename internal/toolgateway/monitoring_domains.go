package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// MonitoringTools is a transport adapter. Decisions, scope and action plans
// remain in the same services used by the HTTP API and background workers.
type MonitoringTools struct {
	Base       ResourceTools
	Monitoring monitoring.Service
	Kubernetes kubernetesworkbench.Service
}

func (t MonitoringTools) actor(ctx context.Context, c mcp.Call) (authorization.Actor, error) {
	a := authorization.Actor{SubjectType: c.SubjectType}
	var err error
	if t.Base.Store == nil {
		return a, monitoring.ErrUnavailable
	}
	if a.EnterpriseID, err = uuid.Parse(c.Enterprise); err != nil {
		return a, monitoring.ErrDenied
	}
	if a.SubjectID, err = uuid.Parse(c.Subject); err != nil {
		return a, monitoring.ErrDenied
	}
	if c.RunID != "" {
		if c.Caller != "model" {
			return a, monitoring.ErrDenied
		}
		if a.RunID, err = uuid.Parse(c.RunID); err != nil {
			return a, monitoring.ErrDenied
		}
	}
	switch c.SubjectType {
	case "service_account":
		account, e := t.Base.Store.Queries.GetServiceAccount(ctx, db.GetServiceAccountParams{EnterpriseID: a.EnterpriseID, ID: a.SubjectID})
		if e != nil || account.Status != "active" || !slices.Contains(account.AllowedToolIds, c.ToolID) {
			return a, monitoring.ErrDenied
		}
		a.AuthorizationVersion = account.AuthorizationVersion
	case "", "user":
		user, e := t.Base.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: a.EnterpriseID, ID: a.SubjectID})
		if e != nil || user.Status != "active" {
			return a, monitoring.ErrDenied
		}
		a.SubjectType, a.AuthorizationVersion = "user", user.AuthorizationVersion
	default:
		return a, monitoring.ErrDenied
	}
	return a, nil
}

func domainInput[T any](c mcp.Call) (T, error) {
	var out T
	raw, err := json.Marshal(c.Input)
	if err != nil || len(raw) > 256<<10 {
		return out, monitoring.ErrInvalid
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (t MonitoringTools) register(reg *mcp.Registry, id, permission, title, description string, schema map[string]any, run func(context.Context, authorization.Actor, mcp.Call) (any, error)) error {
	compiled, err := toolruntime.CompileInputSchema(schema)
	if err != nil {
		return err
	}
	preview := strings.HasSuffix(id, ".preview")
	risk, mode := "read", mcp.ParallelSafe
	if preview {
		risk, mode = "write", mcp.Sequential
	}
	meta := mcp.Metadata{ID: id, ToolFamily: id, Risk: risk, Visibility: mcp.Visible, ExecutionMode: mode,
		Required: []string{permission}, InputVersion: "argus.monitoring_tools/v1", OutputVersion: "argus.monitoring_result/v1",
		ProjectionSchema: "argus.tool_result_projection/v1", MaxResultBytes: 4 << 20, InputSchema: schema,
		OutputSchema: map[string]any{"type": "object", "additionalProperties": true},
		Discovery: mcp.Discovery{SchemaVersion: "argus.tool_discovery/v1", Revision: "1", Title: title, Description: description,
			Keywords:          []string{strings.Split(id, ".")[0], "application", "monitoring", "resource"},
			ResultDescription: "Authorized public records with bounded pages, or an immutable pending action requiring human confirmation. No credentials or SDK Key values.",
			Preconditions:     []string{"Use IDs and current versions from authorized directory results. Empty explicit selections stay empty.", "Secrets are entered only through monitoring.collection.form in the trusted conversation host or the portal. This tool cannot reveal Keys or submit private credential material."},
			Examples:          []mcp.DiscoveryExample{{Request: title, Guidance: "Describe this tool, then supply the exact public fields and authoritative IDs required by its schema."}}},
		Validate: func(in map[string]any) error {
			if err := rejectPrivateDomainInput(in); err != nil {
				return err
			}
			raw, _ := json.Marshal(in)
			var normalized any
			if json.Unmarshal(raw, &normalized) != nil {
				return monitoring.ErrInvalid
			}
			return compiled.Validate(normalized)
		},
		Authorize: func(ctx context.Context, c mcp.Call) error {
			a, e := t.actor(ctx, c)
			if e != nil {
				return e
			}
			return authorization.Authorize(ctx, t.Base.Store.Queries, a, permission)
		},
		Execute: func(ctx context.Context, c mcp.Call) (mcp.Result, error) {
			a, e := t.actor(ctx, c)
			if e != nil {
				return mcp.Result{}, e
			}
			value, e := run(ctx, a, c)
			if e != nil {
				return mcp.Result{}, e
			}
			raw, e := json.Marshal(value)
			if e != nil {
				return mcp.Result{}, e
			}
			out := map[string]any{}
			if json.Unmarshal(raw, &out) != nil {
				return mcp.Result{}, monitoring.ErrInvalid
			}
			delete(out, "watch_token")
			if cursor, ok := out["next_cursor"].(string); ok {
				out["complete"] = cursor == ""
			}
			public := presentation.PublicSnapshot(out)
			// The gateway extracts this public action reference for the host's
			// confirmation card. It is not the private executor commit token.
			if preview {
				if ref, ok := out["action_ref"].(string); ok && ref != "" {
					public["action_ref"] = ref
				}
			}
			return mcp.Result{Structured: public}, nil
		}}
	if id == "kubernetes.object.change.preview" {
		meta.AnyRequired = []string{"kubernetes.workload.manage", "kubernetes.configuration.manage", "kubernetes.namespace.manage", "kubernetes.node.manage", "kubernetes.network.manage", "kubernetes.rbac.manage", "kubernetes.storage.manage", "kubernetes.crd.manage", "kubernetes.custom_resource.manage"}
	}
	meta = t.presentation(meta)
	if err = reg.Register(meta); err != nil {
		return err
	}
	if !preview {
		return nil
	}
	return reg.Register(mcp.Metadata{ID: strings.TrimSuffix(id, ".preview") + ".commit", Risk: "dangerous", Visibility: mcp.Hidden, ExecutionMode: mcp.Sequential, Required: []string{"internal.action_executor"}, InputVersion: "argus.private_action_plan/v1", OutputVersion: "argus.execution/v1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{}, errors.New("commit uses the immutable internal action plan")
	}})
}

func (t MonitoringTools) Register(reg *mcp.Registry) error {
	if reg == nil || t.Base.Store == nil || t.Monitoring.Store == nil || t.Kubernetes.Store == nil {
		return monitoring.ErrUnavailable
	}
	if err := t.registerMonitoring(reg); err != nil {
		return err
	}
	if err := t.registerMonitoringHostForm(reg); err != nil {
		return err
	}
	if err := t.registerWorkbench(reg); err != nil {
		return err
	}
	if err := t.registerDrainCancellation(reg); err != nil {
		return err
	}
	return t.registerHelm(reg)
}

func domainID() map[string]any          { return map[string]any{"type": "string", "format": "uuid"} }
func domainText(max int) map[string]any { return map[string]any{"type": "string", "maxLength": max} }

// Reject private channels even when the public HTTP schema also defines them.
// A caller cannot manufacture another actor's encrypted draft reference.
func rejectPrivateDomainInput(value any) error {
	switch v := value.(type) {
	case map[string]any:
		if name, ok := v["name"].(string); ok && datapolicy.CredentialKey(name) && v["value"] != nil {
			return monitoring.ErrCredential
		}
		for k, child := range v {
			if datapolicy.CredentialKey(k) {
				return monitoring.ErrCredential
			}
			switch strings.ToLower(k) {
			case "credentials", "credential_draft_id", "secret_values", "password", "token", "private_key", "kubeconfig", "client_key", "chart_archive", "private_material_json", "private_input_id", "sealed_plan", "source_credentials", "encrypted_chart":
				return monitoring.ErrCredential
			}
			if err := rejectPrivateDomainInput(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := rejectPrivateDomainInput(child); err != nil {
				return err
			}
		}
	}
	return nil
}
