// Package enterprisemcp owns enterprise connections and member grants. Remote
// tools do not enter the Argus-owned Registry or PendingAction workflow.
package enterprisemcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/integration/remotemcp"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

type Service struct {
	Store       *postgres.Store
	Credentials secret.Service
	Idempotency postgres.Idempotency
	Policy      remotemcp.EndpointPolicy
	HTTP        *http.Client
}

type Input struct {
	Name            string      `json:"name"`
	Endpoint        string      `json:"endpoint"`
	AuthType        string      `json:"auth_type"`
	Value           *string     `json:"credential_value,omitempty"`
	Members         []uuid.UUID `json:"member_ids"`
	ExpectedVersion int64       `json:"expected_version"`
}

type View struct {
	ID        uuid.UUID   `json:"id"`
	Name      string      `json:"name"`
	Transport string      `json:"transport"`
	Endpoint  string      `json:"endpoint"`
	AuthType  string      `json:"auth_type"`
	Status    string      `json:"status"`
	Health    string      `json:"health_status"`
	Version   int64       `json:"version"`
	Revision  int32       `json:"revision"`
	ToolCount int         `json:"tool_count"`
	Members   []uuid.UUID `json:"member_ids"`
	ErrorCode string      `json:"last_error_code,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func (service Service) Admin(ctx context.Context, enterprise, userID uuid.UUID) error {
	user, err := service.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: userID, EnterpriseID: enterprise})
	if err != nil || user.Status != "active" {
		return toolruntime.Error{Kind: "MCP_CONNECTION_FORBIDDEN"}
	}
	admin, err := service.Store.Queries.IsUserEnterpriseAdmin(ctx, db.IsUserEnterpriseAdminParams{EnterpriseID: enterprise, UserID: userID, DepartmentID: user.DepartmentID})
	if err != nil {
		return err
	}
	if !admin {
		return toolruntime.Error{Kind: "MCP_CONNECTION_FORBIDDEN"}
	}
	return nil
}

func (service Service) List(ctx context.Context, enterprise, userID uuid.UUID, admin bool) ([]View, error) {
	var records []db.McpConnection
	var err error
	if admin {
		if err := service.Admin(ctx, enterprise, userID); err != nil {
			return nil, err
		}
		records, err = service.Store.Queries.ListMCPConnections(ctx, enterprise)
	} else {
		records, err = service.Store.Queries.ListGrantedMCPConnections(ctx, db.ListGrantedMCPConnectionsParams{EnterpriseID: enterprise, UserID: userID})
	}
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(records))
	for _, record := range records {
		view, err := service.view(ctx, record, admin)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (service Service) Get(ctx context.Context, enterprise, userID, id uuid.UUID, admin bool) (View, error) {
	if admin {
		if err := service.Admin(ctx, enterprise, userID); err != nil {
			return View{}, err
		}
	} else {
		allowed, err := service.Store.Queries.HasMCPConnectionGrant(ctx, db.HasMCPConnectionGrantParams{ConnectionID: id, EnterpriseID: enterprise, UserID: userID})
		if err != nil {
			return View{}, err
		}
		if !allowed {
			return View{}, toolruntime.Error{Kind: "MCP_CONNECTION_FORBIDDEN"}
		}
	}
	record, err := service.Store.Queries.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: enterprise})
	if err != nil {
		return View{}, err
	}
	return service.view(ctx, record, admin)
}

func (service Service) Save(ctx context.Context, enterprise, userID, id uuid.UUID, input Input, idempotencyKey string) (View, error) {
	if err := service.Admin(ctx, enterprise, userID); err != nil {
		return View{}, err
	}
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 128 || len(input.Members) > 10000 {
		return View{}, toolruntime.Error{Kind: "MCP_CONNECTION_INVALID"}
	}
	if _, err := service.Policy.Validate(ctx, input.Endpoint); err != nil {
		return View{}, err
	}
	var old db.McpConnection
	var revision db.McpConnectionRevision
	var credentialID uuid.NullUUID
	var credentialVersion pgtype.Int8
	var auth string
	var err error
	creating := id == uuid.Nil
	if creating {
		id = uuid.New()
	} else {
		old, err = service.Store.Queries.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: enterprise})
		if err != nil {
			return View{}, err
		}
		if old.Version != input.ExpectedVersion {
			return View{}, toolruntime.Error{Kind: "MCP_CONNECTION_CONFLICT"}
		}
		revision, err = service.Store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: id, EnterpriseID: enterprise, Revision: old.CurrentRevision})
		if err != nil {
			return View{}, err
		}
	}
	if input.AuthType == "none" {
		auth = ""
	} else if input.Value != nil {
		auth, err = authorization(input.AuthType, *input.Value)
	} else if !creating && input.AuthType == revision.AuthType {
		auth, err = service.authorization(ctx, enterprise, userID, old, true)
		credentialID, credentialVersion = revision.CredentialID, revision.CredentialVersion
	} else {
		err = toolruntime.Error{Kind: "MCP_CREDENTIAL_REQUIRED"}
	}
	if err != nil {
		return View{}, err
	}
	tools, hash, err := service.discover(ctx, input.Endpoint, auth)
	auth = ""
	if err != nil {
		return View{}, err
	}
	var record db.McpConnection
	write := func(q *db.Queries) (View, error) {
		version := input.ExpectedVersion
		nextRevision := old.CurrentRevision + 1
		status := old.Status
		if creating {
			var err error
			record, err = q.CreateMCPConnection(ctx, db.CreateMCPConnectionParams{ID: id, EnterpriseID: enterprise, Name: input.Name, CreatedBy: userID})
			if err != nil {
				return View{}, err
			}
			version = record.Version
			status = "enabled"
		}
		if input.AuthType != "none" && input.Value != nil {
			kind := "api_token"
			if input.AuthType == "basic" {
				kind = "basic_auth"
			}
			credential, err := service.Credentials.CreateMCPWithQueries(ctx, q, enterprise, userID, id, kind, *input.Value)
			if err != nil {
				return View{}, err
			}
			credentialID = uuid.NullUUID{UUID: credential.ID, Valid: true}
			credentialVersion = pgtype.Int8{Int64: credential.Version, Valid: true}
		}
		if _, err := q.CreateMCPConnectionRevision(ctx, db.CreateMCPConnectionRevisionParams{ConnectionID: id, EnterpriseID: enterprise, Revision: nextRevision, Endpoint: input.Endpoint,
			AuthType: input.AuthType, CredentialID: credentialID, CredentialVersion: credentialVersion}); err != nil {
			return View{}, err
		}
		encoded, _ := json.Marshal(tools)
		snapshot, err := q.CreateMCPToolSnapshot(ctx, db.CreateMCPToolSnapshotParams{ID: uuid.New(), ConnectionID: id, EnterpriseID: enterprise, ConnectionRevision: nextRevision, SchemaHash: hash, Tools: encoded})
		if err != nil {
			return View{}, err
		}
		record, err = q.UpdateMCPConnection(ctx, db.UpdateMCPConnectionParams{ID: id, EnterpriseID: enterprise, Name: input.Name, Status: status, CurrentRevision: nextRevision,
			CurrentToolSnapshotID: uuid.NullUUID{UUID: snapshot.ID, Valid: true}, HealthStatus: "healthy", Version: version})
		if err != nil {
			return View{}, err
		}
		if err := replaceMembers(ctx, q, enterprise, id, input.Members); err != nil {
			return View{}, err
		}
		if err := appendAudit(ctx, q, enterprise, userID, id, "mcp_connection.save"); err != nil {
			return View{}, err
		}
		return View{ID: record.ID, Name: record.Name, Transport: "streamable_http", Endpoint: input.Endpoint, AuthType: input.AuthType, Status: record.Status, Health: record.HealthStatus,
			Version: record.Version, Revision: record.CurrentRevision, ToolCount: len(tools), Members: input.Members, CreatedAt: record.CreatedAt.Time, UpdatedAt: record.UpdatedAt.Time}, nil
	}
	if creating {
		return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", userID.String(), "mcp_connection.create", idempotencyKey, input, 201, write)
	}
	var view View
	err = service.Store.InTx(ctx, func(q *db.Queries) error { var err error; view, err = write(q); return err })
	return view, err
}

func (service Service) SetMembers(ctx context.Context, enterprise, userID, id uuid.UUID, version int64, members []uuid.UUID) (View, error) {
	if err := service.Admin(ctx, enterprise, userID); err != nil {
		return View{}, err
	}
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		if _, err := q.BumpMCPAuthorization(ctx, db.BumpMCPAuthorizationParams{ID: id, EnterpriseID: enterprise, Version: version}); err != nil {
			return err
		}
		if err := replaceMembers(ctx, q, enterprise, id, members); err != nil {
			return err
		}
		return appendAudit(ctx, q, enterprise, userID, id, "mcp_connection.members")
	})
	if err != nil {
		return View{}, err
	}
	return service.Get(ctx, enterprise, userID, id, true)
}

func (service Service) SetState(ctx context.Context, enterprise, userID, id uuid.UUID, version int64, status string) (View, error) {
	if err := service.Admin(ctx, enterprise, userID); err != nil {
		return View{}, err
	}
	if status != "enabled" && status != "disabled" {
		return View{}, toolruntime.Error{Kind: "MCP_CONNECTION_INVALID"}
	}
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		old, err := q.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: enterprise})
		if err != nil {
			return err
		}
		if status == "enabled" && (old.HealthStatus != "healthy" || !old.CurrentToolSnapshotID.Valid) {
			return toolruntime.Error{Kind: "MCP_CONNECTION_UNAVAILABLE"}
		}
		if _, err = q.UpdateMCPConnection(ctx, db.UpdateMCPConnectionParams{ID: id, EnterpriseID: enterprise, Name: old.Name, Status: status, CurrentRevision: old.CurrentRevision,
			CurrentToolSnapshotID: old.CurrentToolSnapshotID, HealthStatus: old.HealthStatus, LastErrorCode: old.LastErrorCode, Version: version}); err != nil {
			return err
		}
		return appendAudit(ctx, q, enterprise, userID, id, "mcp_connection."+status)
	})
	if err != nil {
		return View{}, err
	}
	return service.Get(ctx, enterprise, userID, id, true)
}

func (service Service) Test(ctx context.Context, enterprise, userID, id uuid.UUID) (View, error) {
	if err := service.Admin(ctx, enterprise, userID); err != nil {
		return View{}, err
	}
	old, err := service.Store.Queries.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: id, EnterpriseID: enterprise})
	if err != nil {
		return View{}, err
	}
	revision, err := service.Store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: id, EnterpriseID: enterprise, Revision: old.CurrentRevision})
	if err != nil {
		return View{}, err
	}
	auth, err := service.authorization(ctx, enterprise, userID, old, true)
	if err != nil {
		return View{}, err
	}
	tools, hash, testErr := service.discover(ctx, revision.Endpoint, auth)
	auth = ""
	err = service.Store.InTx(ctx, func(q *db.Queries) error {
		snapshotID := old.CurrentToolSnapshotID
		health := "healthy"
		code := pgtype.Text{}
		if testErr != nil {
			health = "unhealthy"
			code = pgtype.Text{String: "MCP_CONNECTION_UNAVAILABLE", Valid: true}
		} else {
			encoded, _ := json.Marshal(tools)
			snapshot, err := q.CreateMCPToolSnapshot(ctx, db.CreateMCPToolSnapshotParams{ID: uuid.New(), ConnectionID: id, EnterpriseID: enterprise, ConnectionRevision: old.CurrentRevision, SchemaHash: hash, Tools: encoded})
			if err != nil {
				return err
			}
			snapshotID = uuid.NullUUID{UUID: snapshot.ID, Valid: true}
		}
		_, err := q.UpdateMCPConnection(ctx, db.UpdateMCPConnectionParams{ID: id, EnterpriseID: enterprise, Name: old.Name, Status: old.Status, CurrentRevision: old.CurrentRevision, CurrentToolSnapshotID: snapshotID, HealthStatus: health, LastErrorCode: code, Version: old.Version})
		return err
	})
	if err != nil {
		return View{}, err
	}
	if testErr != nil {
		return View{}, testErr
	}
	return service.Get(ctx, enterprise, userID, id, true)
}

func (service Service) view(ctx context.Context, record db.McpConnection, admin bool) (View, error) {
	revision, err := service.Store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: record.ID, EnterpriseID: record.EnterpriseID, Revision: record.CurrentRevision})
	if err != nil {
		return View{}, err
	}
	view := View{ID: record.ID, Name: record.Name, Transport: "streamable_http", Endpoint: revision.Endpoint, AuthType: revision.AuthType, Status: record.Status, Health: record.HealthStatus,
		Version: record.Version, Revision: record.CurrentRevision, Members: []uuid.UUID{}, CreatedAt: record.CreatedAt.Time, UpdatedAt: record.UpdatedAt.Time, ErrorCode: record.LastErrorCode.String}
	if admin {
		view.Members, err = service.Store.Queries.ListMCPConnectionMembers(ctx, db.ListMCPConnectionMembersParams{ConnectionID: record.ID, EnterpriseID: record.EnterpriseID})
		if err != nil {
			return View{}, err
		}
	}
	if record.CurrentToolSnapshotID.Valid {
		snapshot, err := service.Store.Queries.GetMCPToolSnapshot(ctx, db.GetMCPToolSnapshotParams{ID: record.CurrentToolSnapshotID.UUID, EnterpriseID: record.EnterpriseID})
		if err != nil {
			return View{}, err
		}
		var tools []remotemcp.Tool
		_ = json.Unmarshal(snapshot.Tools, &tools)
		view.ToolCount = len(tools)
	}
	return view, nil
}

func (service Service) discover(ctx context.Context, endpoint, auth string) ([]remotemcp.Tool, string, error) {
	if _, err := service.Policy.Validate(ctx, endpoint); err != nil {
		return nil, "", err
	}
	client := service.client(endpoint, auth)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		client.Close(closeCtx)
		client.Authorization = ""
	}()
	if err := client.Initialize(ctx); err != nil {
		return nil, "", err
	}
	tools, hash, err := client.ListTools(ctx)
	if err != nil {
		return nil, "", err
	}
	for _, tool := range tools {
		if _, err := toolruntime.CompileInputSchema(tool.InputSchema); err != nil {
			return nil, "", err
		}
	}
	return tools, hash, nil
}

func (service Service) client(endpoint, auth string) *remotemcp.Client {
	httpClient := service.HTTP
	if httpClient == nil {
		httpClient = service.Policy.Client()
	}
	return &remotemcp.Client{Endpoint: endpoint, HTTP: httpClient, Authorization: auth}
}

func (service Service) authorization(ctx context.Context, enterprise, userID uuid.UUID, connection db.McpConnection, admin bool) (string, error) {
	revision, err := service.Store.Queries.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: connection.ID, EnterpriseID: enterprise, Revision: connection.CurrentRevision})
	if err != nil {
		return "", err
	}
	if revision.AuthType == "none" {
		return "", nil
	}
	prefix := "mcp-call/"
	if admin {
		prefix = "mcp-test/"
	}
	issued, err := service.Credentials.IssueLease(ctx, userID.String(), enterprise, secret.LeaseRequest{CredentialID: revision.CredentialID.UUID, OperationRef: prefix + uuid.NewString(),
		TargetResourceType: "mcp_connection", TargetResourceID: connection.ID, RecipientType: "mcp_adapter", RecipientID: userID.String(), Protocol: "http", TTL: time.Minute})
	if err != nil {
		return "", err
	}
	defer clear(issued.Value)
	return authorization(revision.AuthType, string(issued.Value))
}

func authorization(kind, value string) (string, error) {
	switch kind {
	case "none":
		return "", nil
	case "bearer":
		if value != "" && !strings.ContainsAny(value, "\r\n") {
			return "Bearer " + value, nil
		}
	case "basic":
		var data struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if json.Unmarshal([]byte(value), &data) == nil && data.Username != "" && !strings.Contains(data.Username, ":") {
			return "Basic " + base64.StdEncoding.EncodeToString([]byte(data.Username+":"+data.Password)), nil
		}
	}
	return "", toolruntime.Error{Kind: "MCP_CREDENTIAL_INVALID"}
}

func replaceMembers(ctx context.Context, q *db.Queries, enterprise, id uuid.UUID, members []uuid.UUID) error {
	if len(members) > 10000 {
		return toolruntime.Error{Kind: "MCP_MEMBER_INVALID"}
	}
	if err := q.DeleteMCPConnectionGrants(ctx, db.DeleteMCPConnectionGrantsParams{ConnectionID: id, EnterpriseID: enterprise}); err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	for _, member := range members {
		if seen[member] {
			return toolruntime.Error{Kind: "MCP_MEMBER_INVALID"}
		}
		seen[member] = true
		user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: member, EnterpriseID: enterprise})
		if err != nil || user.Status != "active" {
			return toolruntime.Error{Kind: "MCP_MEMBER_INVALID"}
		}
		if err := q.GrantMCPConnection(ctx, db.GrantMCPConnectionParams{ConnectionID: id, EnterpriseID: enterprise, UserID: member}); err != nil {
			return err
		}
	}
	return nil
}

func appendAudit(ctx context.Context, q *db.Queries, enterprise, user, id uuid.UUID, action string) error {
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, ActorType: "enterprise_user", ActorID: user.String(), Action: action,
		ResourceType: "mcp_connection", ResourceID: id.String(), Result: "success", Details: map[string]any{"summary": "enterprise MCP configuration updated"}})
	return err
}
