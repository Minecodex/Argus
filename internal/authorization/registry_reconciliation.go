package authorization

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type RegistryReconciliation struct {
	Version      int32
	CreatedRoles int
	UpdatedRoles int
}

// SyncPermissionCatalog must run inside the caller's transaction. Enterprise
// creation and startup share this lock, ordering and downgrade fence.
func SyncPermissionCatalog(ctx context.Context, q *db.Queries) (int32, error) {
	if err := q.LockPermissionRegistry(ctx); err != nil {
		return 0, err
	}
	previous, err := q.CurrentPermissionRegistryRevision(ctx)
	if err != nil {
		return 0, err
	}
	if previous > PermissionRegistryVersion {
		return 0, fmt.Errorf("permission registry revision %d is newer than this service revision %d", previous, PermissionRegistryVersion)
	}
	for _, id := range registryKeys() {
		if err := q.UpsertPermission(ctx, db.UpsertPermissionParams{ID: id, Description: PermissionRegistry[id], RegistryVersion: PermissionRegistryVersion}); err != nil {
			return 0, err
		}
	}
	return previous, nil
}

// ReconcileBuiltinRegistry is an internal migration/startup operation, not a
// user mutation. It uses the same immutable built-in definitions as enterprise
// creation. Custom role permissions and every resource grant remain untouched.
func (service Service) ReconcileBuiltinRegistry(ctx context.Context) (RegistryReconciliation, error) {
	result := RegistryReconciliation{Version: PermissionRegistryVersion}
	if service.Store == nil {
		return result, errors.New("permission registry store is required")
	}
	err := service.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		previous, err := SyncPermissionCatalog(ctx, q)
		if err != nil {
			return err
		}
		enterprises, err := q.ListAllEnterprises(ctx)
		if err != nil {
			return err
		}
		for _, enterprise := range enterprises {
			if _, err = q.LockEnterpriseForAccessUpdate(ctx, enterprise.ID); err != nil {
				return err
			}
			roles, err := q.ListRoles(ctx, enterprise.ID)
			if err != nil {
				return err
			}
			existing := map[string]db.Role{}
			for _, role := range roles {
				if role.Builtin && role.IdentityKey.Valid {
					existing[role.IdentityKey.String] = role
				}
			}
			for _, definition := range BuiltinRoles {
				expected := append([]string(nil), definition.Permissions...)
				slices.Sort(expected)
				for _, permission := range expected {
					if _, ok := PermissionRegistry[permission]; !ok {
						return errors.New("built-in role references an unregistered permission")
					}
				}
				role, found := existing[definition.Key]
				var current []string
				action := "role.registry.create"
				if found {
					current, err = q.ListRolePermissions(ctx, role.ID)
					if err != nil {
						return err
					}
					slices.Sort(current)
					if slices.Equal(current, expected) {
						continue
					}
					_, err = q.BumpBuiltinRoleRegistryVersion(ctx, db.BumpBuiltinRoleRegistryVersionParams{ID: role.ID, EnterpriseID: enterprise.ID, IdentityKey: definition.Key, ExpectedVersion: role.Version})
					if err != nil {
						return err
					}
					if err = q.DeleteRolePermissions(ctx, role.ID); err != nil {
						return err
					}
					action = "role.registry.upgrade"
					result.UpdatedRoles++
				} else {
					role, err = q.CreateRole(ctx, db.CreateRoleParams{ID: newID(), EnterpriseID: enterprise.ID, IdentityKey: pgtype.Text{String: definition.Key, Valid: true}, Name: definition.Name, Description: definition.Description, Builtin: true})
					if err != nil {
						return err
					}
					result.CreatedRoles++
				}
				for _, permission := range expected {
					if err = q.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: role.ID, PermissionID: permission}); err != nil {
						return err
					}
				}
				if found {
					if err = service.invalidateSubject(ctx, q, enterprise.ID, "role", role.ID); err != nil {
						return err
					}
				}
				_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterprise.ID, Valid: true}, ActorType: "system", ActorID: "permission-registry", Action: action, ResourceType: "role", ResourceID: role.ID.String(), Result: "success", Details: map[string]any{"summary": "built-in role synchronized to permission registry", "resource_name": definition.Key, "before": map[string]any{"version": previous, "actions": current}, "after": map[string]any{"version": PermissionRegistryVersion, "actions": expected}}})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return RegistryReconciliation{}, err
	}
	return result, nil
}
