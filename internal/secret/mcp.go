package secret

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrMCPManagedSecret = errors.New("MCP credentials must be managed through their enterprise connection")

func (service Service) CreateMCPWithQueries(ctx context.Context, q *db.Queries, enterpriseID, actorID, connectionID uuid.UUID, kind, value string) (db.Credential, error) {
	if (kind != "api_token" && kind != "basic_auth") || value == "" {
		return db.Credential{}, ErrSecretValueRequired
	}
	id := newUUID()
	name := fmt.Sprintf("mcp-%s-%s", connectionID, id)
	envelope, err := service.Keyring.EncryptContext(ctx, []byte(value), secretAAD(enterpriseID, id, 1, kind))
	if err != nil {
		return db.Credential{}, err
	}
	record, err := q.CreateSecret(ctx, db.CreateSecretParams{ID: id, EnterpriseID: enterpriseID, Name: name, Type: kind, CreatedBy: actorID})
	if err != nil {
		return db.Credential{}, err
	}
	if _, err = q.CreateSecretVersion(ctx, envelopeParams(record, 1, envelope)); err != nil {
		return db.Credential{}, err
	}
	if err = q.MarkMCPManagedSecret(ctx, db.MarkMCPManagedSecretParams{ID: id, EnterpriseID: enterpriseID, OwnerID: uuid.NullUUID{UUID: connectionID, Valid: true}}); err != nil {
		return db.Credential{}, err
	}
	credential, err := q.CreateCredential(ctx, db.CreateCredentialParams{ID: newUUID(), EnterpriseID: enterpriseID, Name: name, Protocol: "http", SecretID: id})
	if err != nil {
		return db.Credential{}, err
	}
	return credential, appendAudit(ctx, q, actorID.String(), enterpriseID, "mcp_connection.credential.create", "mcp_connection", connectionID, map[string]any{"summary": "MCP connection credential created"})
}

func (service Service) assertPublicSecret(ctx context.Context, enterpriseID, id uuid.UUID) error {
	record, err := service.Store.Queries.GetSecret(ctx, db.GetSecretParams{ID: id, EnterpriseID: enterpriseID})
	if err != nil {
		return err
	}
	if record.OwnerType == "mcp_connection" {
		return ErrMCPManagedSecret
	}
	return nil
}

func validateMCPLease(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, actorID string, request LeaseRequest, credential db.Credential) error {
	if request.TargetResourceType != "mcp_connection" || request.Protocol != "http" || request.RecipientID != actorID {
		return ErrInvalidLease
	}
	connection, err := q.GetMCPConnection(ctx, db.GetMCPConnectionParams{ID: request.TargetResourceID, EnterpriseID: enterpriseID})
	if err != nil {
		return ErrInvalidLease
	}
	revision, err := q.GetMCPConnectionRevision(ctx, db.GetMCPConnectionRevisionParams{ConnectionID: connection.ID, EnterpriseID: enterpriseID, Revision: connection.CurrentRevision})
	if err != nil || !revision.CredentialID.Valid || revision.CredentialID.UUID != credential.ID || !revision.CredentialVersion.Valid || revision.CredentialVersion.Int64 != credential.Version {
		return ErrInvalidLease
	}
	userID, err := uuid.Parse(actorID)
	if err != nil {
		return ErrInvalidLease
	}
	user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: userID, EnterpriseID: enterpriseID})
	if err != nil || user.Status != "active" {
		return ErrInvalidLease
	}
	if len(request.OperationRef) > 9 && request.OperationRef[:9] == "mcp-test/" {
		admin, err := q.IsUserEnterpriseAdmin(ctx, db.IsUserEnterpriseAdminParams{EnterpriseID: enterpriseID, UserID: userID, DepartmentID: user.DepartmentID})
		if err != nil || !admin {
			return ErrInvalidLease
		}
		return nil
	}
	allowed, err := q.HasMCPConnectionGrant(ctx, db.HasMCPConnectionGrantParams{ConnectionID: connection.ID, EnterpriseID: enterpriseID, UserID: userID})
	if err != nil || !allowed || connection.Status != "enabled" {
		return ErrInvalidLease
	}
	return nil
}
