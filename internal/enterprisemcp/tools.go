package enterprisemcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/integration/remotemcp"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (service Service) BuildTools(ctx context.Context, p toolruntime.Principal) (toolruntime.Contribution, error) {
	ids, err := service.Store.Queries.GetConversationMCPConnections(ctx, db.GetConversationMCPConnectionsParams{ConversationID: p.ConversationID, EnterpriseID: p.EnterpriseID})
	if err != nil {
		return toolruntime.Contribution{}, err
	}
	part := toolruntime.Contribution{}
	for _, id := range ids {
		connection, revision, err := service.authorized(ctx, p, id)
		if err != nil {
			return part, err
		}
		connection, err = service.refreshMemberCatalog(ctx, p, connection, revision)
		if err != nil {
			return part, err
		}
		if !connection.CurrentToolSnapshotID.Valid {
			return part, toolruntime.Error{Kind: "MCP_CONNECTION_UNAVAILABLE"}
		}
		snapshot, err := service.Store.Queries.GetMCPToolSnapshot(ctx, db.GetMCPToolSnapshotParams{ID: connection.CurrentToolSnapshotID.UUID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return part, err
		}
		var tools []remotemcp.Tool
		if err := json.Unmarshal(snapshot.Tools, &tools); err != nil {
			return part, toolruntime.Error{Kind: "MCP_PROTOCOL_ERROR"}
		}
		part.Connections = append(part.Connections, toolruntime.ConnectionSnapshot{ID: id.String(), Revision: connection.CurrentRevision, CredentialVersion: revision.CredentialVersion.Int64, SchemaHash: snapshot.SchemaHash})
		for _, remoteTool := range tools {
			name := remoteTool.Name
			sum := sha256.Sum256([]byte(name))
			alias := "mcp_" + strings.ReplaceAll(id.String(), "-", "") + "_" + hex.EncodeToString(sum[:6])
			connectionRevision, schemaHash, credentialVersion := connection.CurrentRevision, snapshot.SchemaHash, revision.CredentialVersion.Int64
			part.Tools = append(part.Tools, toolruntime.Tool{Definition: toolruntime.Definition{Model: modelprovider.Tool{Name: alias, Description: "Customer MCP " + connection.Name + " / " + remoteTool.Name + ": " + remoteTool.Description, Schema: remoteTool.InputSchema},
				Source: "external_mcp", Version: fmt.Sprintf("%d:%d:%s", connectionRevision, credentialVersion, schemaHash), ReadOnly: false, ConnectionID: id.String(), SchemaHash: schemaHash},
				Invoke: func(ctx context.Context, call toolruntime.Invocation) (toolruntime.Result, error) {
					return service.invoke(ctx, call, id, connectionRevision, credentialVersion, schemaHash, name)
				},
			})
		}
	}
	return part, nil
}

func (service Service) authorized(ctx context.Context, p toolruntime.Principal, id uuid.UUID) (db.McpConnection, db.McpConnectionRevision, error) {
	connection, err := service.Store.Queries.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: p.EnterpriseID})
	if err != nil || connection.Status != "enabled" || connection.HealthStatus != "healthy" {
		return connection, db.McpConnectionRevision{}, toolruntime.Error{Kind: "MCP_CONNECTION_UNAVAILABLE"}
	}
	allowed, err := service.Store.Queries.HasMCPConnectionGrant(ctx, db.HasMCPConnectionGrantParams{ConnectionID: id, EnterpriseID: p.EnterpriseID, UserID: p.UserID})
	if err != nil {
		return connection, db.McpConnectionRevision{}, err
	}
	if !allowed {
		return connection, db.McpConnectionRevision{}, toolruntime.Error{Kind: "MCP_CONNECTION_FORBIDDEN"}
	}
	revision, err := service.Store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: id, EnterpriseID: p.EnterpriseID, Revision: connection.CurrentRevision})
	return connection, revision, err
}

func (service Service) invoke(ctx context.Context, call toolruntime.Invocation, id uuid.UUID, expectedRevision int32, expectedCredential int64, expectedHash, name string) (toolruntime.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	connection, revision, err := service.authorized(ctx, call.Principal, id)
	if err != nil {
		return toolruntime.Result{}, err
	}
	if connection.CurrentRevision != expectedRevision || revision.CredentialVersion.Int64 != expectedCredential {
		return toolruntime.Result{}, toolruntime.Error{Kind: "MCP_SCHEMA_CHANGED"}
	}
	if _, err := service.Policy.Validate(ctx, revision.Endpoint); err != nil {
		return toolruntime.Result{}, err
	}
	auth, err := service.authorization(ctx, call.Principal.EnterpriseID, call.Principal.UserID, connection, false)
	if err != nil {
		return toolruntime.Result{}, err
	}
	client := service.client(revision.Endpoint, auth)
	auth = ""
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client.Close(closeCtx)
		client.Authorization = ""
	}()
	if err := client.Initialize(ctx); err != nil {
		return toolruntime.Result{}, err
	}
	tools, hash, err := client.ListTools(ctx)
	if err != nil {
		return toolruntime.Result{}, err
	}
	if hash != expectedHash || client.SchemaChanged {
		_ = service.recordSchema(ctx, connection, tools, hash)
		return toolruntime.Result{}, toolruntime.Error{Kind: "MCP_SCHEMA_CHANGED"}
	}
	// Discovery is not an authorization ticket: recheck after network work and
	// immediately before sending the customer's business invocation.
	latest, latestRevision, err := service.authorized(ctx, call.Principal, id)
	if err != nil {
		return toolruntime.Result{}, err
	}
	if latest.CurrentRevision != expectedRevision || latestRevision.CredentialVersion.Int64 != expectedCredential {
		return toolruntime.Result{}, toolruntime.Error{Kind: "MCP_SCHEMA_CHANGED"}
	}
	client.BeforeCall = call.BeforeDispatch
	data, err := client.Call(ctx, name, call.Arguments)
	if err != nil {
		var remoteError remotemcp.Error
		if errors.As(err, &remoteError) && remoteError.Unknown {
			return toolruntime.Result{Unknown: true, ToolID: name, Data: map[string]any{"error_code": "TOOL_RESULT_UNKNOWN", "cause": remoteError.Kind}}, nil
		}
		return toolruntime.Result{}, err
	}
	if failed, _ := data["is_error"].(bool); failed {
		return toolruntime.Result{}, toolruntime.Error{Kind: "MCP_TOOL_ERROR"}
	}
	return toolruntime.Result{Data: data, ToolID: name}, nil
}

func (service Service) recordSchema(ctx context.Context, connection db.McpConnection, tools []remotemcp.Tool, hash string) error {
	for _, tool := range tools {
		if _, err := toolruntime.CompileInputSchema(tool.InputSchema); err != nil {
			return err
		}
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		encoded, _ := json.Marshal(tools)
		snapshot, err := q.CreateMCPToolSnapshot(ctx, db.CreateMCPToolSnapshotParams{ID: uuid.New(), ConnectionID: connection.ID, EnterpriseID: connection.EnterpriseID, ConnectionRevision: connection.CurrentRevision, SchemaHash: hash, Tools: encoded})
		if err != nil {
			return err
		}
		_, err = q.UpdateMCPConnection(ctx, db.UpdateMCPConnectionParams{ID: connection.ID, EnterpriseID: connection.EnterpriseID, Name: connection.Name, Status: connection.Status, CurrentRevision: connection.CurrentRevision,
			CurrentToolSnapshotID: uuid.NullUUID{UUID: snapshot.ID, Valid: true}, HealthStatus: connection.HealthStatus, LastErrorCode: pgtype.Text{}, Version: connection.Version})
		return err
	})
}

// Every new tool set uses the selected connections' current catalog. A schema
// added between Runs must be visible without requiring an administrator test.
// Restore compares the refreshed snapshot and rejects a stale Run unchanged.
func (service Service) refreshMemberCatalog(ctx context.Context, p toolruntime.Principal, connection db.McpConnection, revision db.McpConnectionRevision) (db.McpConnection, error) {
	auth, err := service.authorization(ctx, p.EnterpriseID, p.UserID, connection, false)
	if err != nil {
		return connection, err
	}
	tools, hash, err := service.discover(ctx, revision.Endpoint, auth)
	auth = ""
	if err != nil {
		return connection, err
	}
	latest, latestRevision, err := service.authorized(ctx, p, connection.ID)
	if err != nil {
		return connection, err
	}
	if latest.CurrentRevision != connection.CurrentRevision || latestRevision.CredentialVersion != revision.CredentialVersion {
		return connection, toolruntime.Error{Kind: "MCP_SCHEMA_CHANGED"}
	}
	if latest.CurrentToolSnapshotID.Valid {
		stored, err := service.Store.Queries.GetMCPToolSnapshot(ctx, db.GetMCPToolSnapshotParams{ID: latest.CurrentToolSnapshotID.UUID, EnterpriseID: p.EnterpriseID})
		if err != nil {
			return connection, err
		}
		if stored.SchemaHash == hash {
			return latest, nil
		}
	}
	if err := service.recordSchema(ctx, latest, tools, hash); err != nil {
		return connection, err
	}
	latest, _, err = service.authorized(ctx, p, connection.ID)
	return latest, err
}
