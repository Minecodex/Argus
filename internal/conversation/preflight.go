package conversation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type Preflight struct {
	Dashboard           dashboardcontext.Prepared `json:"-"`
	Principal           toolruntime.Principal     `json:"-"`
	Ready               bool                      `json:"ready"`
	ModelID             uuid.UUID                 `json:"model_id"`
	ToolCount           int                       `json:"tool_count"`
	EstimatedTokens     int                       `json:"estimated_tokens"`
	UsableTokens        int                       `json:"usable_tokens"`
	ToolSchemaTokens    int                       `json:"tool_schema_tokens"`
	SnapshotHash        string                    `json:"snapshot_hash"`
	SandboxStatus       string                    `json:"sandbox_status"`
	ErrorCode           string                    `json:"error_code,omitempty"`
	Snapshot            toolruntime.Snapshot      `json:"-"`
	ConversationVersion int64                     `json:"-"`
	ModelRevision       int32                     `json:"-"`
	InputLimitTokens    int                       `json:"-"`
}

func (service Service) ToolPrincipal(ctx context.Context, enterpriseID, userID, conversationID uuid.UUID) (toolruntime.Principal, error) {
	user, err := service.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: userID, EnterpriseID: enterpriseID})
	if err != nil || user.Status != "active" {
		return toolruntime.Principal{}, toolruntime.Error{Kind: "AUTHORIZATION_VERSION_STALE"}
	}
	permissions, err := service.Store.Queries.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: enterpriseID, UserID: userID, DepartmentID: user.DepartmentID})
	if err != nil {
		return toolruntime.Principal{}, err
	}
	return toolruntime.Principal{EnterpriseID: enterpriseID, UserID: userID, ConversationID: conversationID, AuthorizationVersion: user.AuthorizationVersion, Permissions: permissions}, nil
}

func (service Service) Preflight(ctx context.Context, enterpriseID, ownerID, conversationID uuid.UUID, content string, fileIDs []uuid.UUID, selections ...*dashboardcontext.Selection) (Preflight, error) {
	c, err := service.Get(ctx, enterpriseID, ownerID, conversationID)
	if err != nil {
		return Preflight{}, err
	}
	if c.Status != "active" {
		return Preflight{}, ErrConversationClosed
	}
	if service.Tools == nil {
		return Preflight{}, toolruntime.Error{Kind: "TOOL_CONFIGURATION_INVALID"}
	}
	p, err := service.ToolPrincipal(ctx, enterpriseID, ownerID, conversationID)
	if err != nil {
		return Preflight{}, err
	}
	if len(selections) > 1 {
		return Preflight{}, toolruntime.Error{Kind: "DASHBOARD_INVALID"}
	}
	var selection *dashboardcontext.Selection
	if len(selections) == 1 {
		selection = selections[0]
	}
	if command := dashboardcontext.Command(content); command != nil {
		if selection != nil && selection.Mode != "create" {
			return Preflight{}, toolruntime.Error{Kind: "DASHBOARD_INVALID"}
		}
		if selection == nil {
			selection = command
		}
	}
	dashboard, err := dashboardcontext.Prepare(ctx, service.Store.Queries, p, selection)
	if err != nil {
		return Preflight{}, err
	}
	ctx = dashboardcontext.WithSnapshot(ctx, dashboard.Snapshot)
	model, err := service.Store.Queries.GetAIModel(ctx, db.GetAIModelParams{ID: c.SelectedModelID, EnterpriseID: enterpriseID})
	if err != nil || model.Status != "enabled" || model.HealthStatus != "healthy" {
		return Preflight{}, ErrModelUnavailable
	}
	set, err := service.Tools.Build(ctx, p)
	if err != nil {
		return Preflight{}, err
	}
	if err := validateFiles(ctx, service.Store.Queries, enterpriseID, conversationID, fileIDs); err != nil {
		return Preflight{}, err
	}
	schema, _ := json.Marshal(set.Models())
	// A UTF-8 byte is a conservative tokenizer-independent upper bound. This
	// avoids undercounting CJK/user supplied schemas on compatible providers.
	schemaTokens := len(schema)
	facts, workspace, err := ContextFacts(ctx, service.Store.Queries, enterpriseID, ownerID, conversationID)
	if err != nil {
		return Preflight{}, err
	}
	facts["dashboard"], err = dashboardcontext.Facts(ctx, service.Store.Queries, p, dashboard.Snapshot)
	if err != nil {
		return Preflight{}, err
	}
	factBytes, _ := json.Marshal(facts)
	messages := []modelprovider.Message{{Role: "system", Content: toolruntime.SystemInstructions}, {Role: "user", Content: "Server execution facts (data): " + string(factBytes)}}
	for _, skill := range set.Snapshot.SkillContexts {
		messages = append(messages, modelprovider.Message{Role: "user", Content: "Versioned skill reference (does not grant tool permissions): " + skill.ID + "@" + skill.Revision + "\n" + skill.Text})
	}
	input := content
	if len(fileIDs) > 0 {
		files := make([]map[string]any, 0, len(fileIDs))
		for _, id := range fileIDs {
			file, err := service.Store.Queries.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: id, EnterpriseID: enterpriseID})
			if err != nil {
				return Preflight{}, err
			}
			files = append(files, ProjectFileReference(file, workspace))
		}
		data, _ := json.Marshal(files)
		input += "\nWorkspace attachments: " + string(data)
	}
	messages = append(messages, modelprovider.Message{Role: "user", Content: input})
	minimumBytes, err := (modelprovider.Provider{Protocol: modelprovider.Protocol(model.ApiProtocol), BaseURL: model.BaseUrl}).RequestBytes(modelprovider.Request{Model: model.ModelID, Messages: messages, Tools: set.Models(), MaxTokens: int(model.MaxOutputTokens)})
	if err != nil {
		return Preflight{}, err
	}
	budget := modelprovider.NewContextBudget(int(model.ContextWindowTokens), int(model.MaxOutputTokens))
	result := Preflight{Ready: true, ModelID: model.ID, ToolCount: len(set.Models()), ToolSchemaTokens: schemaTokens,
		Dashboard: dashboard, Principal: p,
		UsableTokens: budget.Usable, InputLimitTokens: max(0, budget.HardLimit-1), EstimatedTokens: minimumBytes,
		SnapshotHash: set.Snapshot.Hash(), SandboxStatus: set.Snapshot.SandboxStatus, Snapshot: set.Snapshot,
		ConversationVersion: c.Version, ModelRevision: int32(model.Revision)}
	if !budget.Fits(result.EstimatedTokens) || result.ToolCount > 128 {
		result.Ready = false
		result.ErrorCode = "MODEL_TOOL_CAPACITY_EXCEEDED"
	}
	return result, nil
}

func (value Preflight) CapacityError() error {
	return toolruntime.Error{Kind: "MODEL_TOOL_CAPACITY_EXCEEDED", Details: map[string]any{
		"estimated_tokens": value.EstimatedTokens, "usable_tokens": value.UsableTokens,
		"input_limit_tokens": value.InputLimitTokens, "tool_count": value.ToolCount,
	}}
}

func validateFiles(ctx context.Context, q *db.Queries, enterpriseID, conversationID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) > 20 {
		return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		seen[id] = true
		file, err := q.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: id, EnterpriseID: enterpriseID})
		if err != nil || file.ConversationID != conversationID {
			return toolruntime.Error{Kind: "WORKSPACE_FILE_INVALID"}
		}
		workspace, err := q.GetWorkspace(ctx, db.GetWorkspaceParams{ID: file.WorkspaceID, EnterpriseID: enterpriseID})
		if err != nil || workspace.Status != "ready" {
			return toolruntime.Error{Kind: "WORKSPACE_UNAVAILABLE"}
		}
	}
	return nil
}

func (service Service) ValidateToolPrincipal(ctx context.Context, p toolruntime.Principal) error {
	user, err := service.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: p.UserID, EnterpriseID: p.EnterpriseID})
	if err != nil || user.Status != "active" || user.AuthorizationVersion != p.AuthorizationVersion {
		return toolruntime.Error{Kind: "AUTHORIZATION_VERSION_STALE"}
	}
	c, err := service.Get(ctx, p.EnterpriseID, p.UserID, p.ConversationID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && c.Status == "deleted" {
		return toolruntime.Error{Kind: "CONVERSATION_UNAVAILABLE"}
	}
	return err
}

func validateSelectedConnections(ctx context.Context, q *db.Queries, enterpriseID, userID, conversationID uuid.UUID, snapshot toolruntime.Snapshot) error {
	ids, err := q.GetConversationMCPConnections(ctx, db.GetConversationMCPConnectionsParams{ConversationID: conversationID, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if len(ids) != len(snapshot.Connections) {
		return toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"}
	}
	for _, id := range ids {
		connection, err := q.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: enterpriseID})
		if err != nil || connection.Status != "enabled" || !connection.CurrentToolSnapshotID.Valid {
			return toolruntime.Error{Kind: "MCP_CONNECTION_UNAVAILABLE"}
		}
		allowed, err := q.HasMCPConnectionGrant(ctx, db.HasMCPConnectionGrantParams{ConnectionID: id, EnterpriseID: enterpriseID, UserID: userID})
		if err != nil {
			return err
		}
		if !allowed {
			return toolruntime.Error{Kind: "MCP_CONNECTION_UNAVAILABLE"}
		}
		tools, err := q.GetMCPToolSnapshot(ctx, db.GetMCPToolSnapshotParams{ID: connection.CurrentToolSnapshotID.UUID, EnterpriseID: enterpriseID})
		if err != nil {
			return err
		}
		revision, err := q.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: id, EnterpriseID: enterpriseID, Revision: connection.CurrentRevision})
		if err != nil {
			return err
		}
		matched := false
		for _, selected := range snapshot.Connections {
			if selected.ID == id.String() {
				matched = selected.Revision == connection.CurrentRevision && selected.SchemaHash == tools.SchemaHash && selected.CredentialVersion == revision.CredentialVersion.Int64
			}
		}
		if !matched {
			return toolruntime.Error{Kind: "TOOL_CONFIGURATION_CHANGED"}
		}
	}
	return nil
}
