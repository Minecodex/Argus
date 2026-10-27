package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/remoteaccess/revocation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrBuiltinRoleImmutable = errors.New("builtin role is immutable")

var (
	ErrInvalidRoleAssignment = errors.New("role does not belong to enterprise or is disabled")
	ErrLastIAMAdminRequired  = errors.New("enterprise must retain an active IAM administrator")
)

type Service struct {
	Store       *postgres.Store
	Idempotency postgres.Idempotency
}

type RoleRecord struct {
	Role        db.Role
	Permissions []string
}
type BindingRecord struct {
	Binding db.RoleBinding
}

type InheritedRoleAssignment struct {
	RoleID     uuid.UUID
	SourceID   uuid.UUID
	SourceName string
}

type UserRoleAssignments struct {
	DirectRoleIDs        []uuid.UUID
	InheritedRoles       []InheritedRoleAssignment
	EffectiveRoleIDs     []uuid.UUID
	AuthorizationVersion int64
}

type UserRoleAssignmentsUpdate struct {
	RoleIDs                      []uuid.UUID
	DepartmentID                 uuid.UUID
	ExpectedUserVersion          int64
	ExpectedAuthorizationVersion int64
}

type RoleInput struct {
	Name, Description string
	Permissions       []string
	Status            *string
	ExpectedVersion   int64
}
type BindingInput struct {
	SubjectType           string
	SubjectID, RoleID     uuid.UUID
	ValidFrom, ValidUntil *time.Time
	SetValidFrom          bool
	SetValidUntil         bool
	Status                *string
	ExpectedVersion       int64
}

type GrantResource struct {
	Environment  string
	ParentID     uuid.NullUUID
	ParentName   string
	SourceUID    string
	ResourceType string
	ResourceID   uuid.UUID
	Name         string
	Direct       bool
	Inherited    bool
	Sources      []string
}

type GrantResourcePage struct {
	Items                []GrantResource
	AuthorizationVersion int64
	AffectedMemberCount  int64
}

type GrantBatchInput struct {
	SubjectType     string
	SubjectID       uuid.UUID
	ResourceType    string
	ResourceIDs     []uuid.UUID
	Remove          bool
	ExpectedVersion int64
}

func (service Service) CurrentAuthorizationVersion(ctx context.Context, enterpriseID, subjectID uuid.UUID, subjectType string) (int64, error) {
	return service.currentSubjectVersion(ctx, service.Store.Queries, enterpriseID, subjectType, subjectID)
}

func (service Service) AffectedMemberCount(ctx context.Context, enterpriseID, subjectID uuid.UUID, subjectType string) (int64, error) {
	if err := service.validateSubject(ctx, service.Store.Queries, enterpriseID, subjectType, subjectID); err != nil {
		return 0, err
	}
	switch subjectType {
	case "user", "service_account":
		return 1, nil
	case "department":
		return service.Store.Queries.CountDepartmentUsers(ctx, db.CountDepartmentUsersParams{EnterpriseID: enterpriseID, DepartmentID: subjectID})
	case "role":
		return service.Store.Queries.CountRoleMembers(ctx, db.CountRoleMembersParams{EnterpriseID: enterpriseID, RoleID: subjectID})
	default:
		return 0, errors.New("invalid subject type")
	}
}

var (
	ErrInvalidResource       = errors.New("resource does not belong to enterprise")
	ErrAuthorizationConflict = errors.New("authorization version conflict")
	ErrInheritedGrant        = errors.New("inherited authorization must be changed on its role or department")
)

func (service Service) validateSubject(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, subjectType string, subjectID uuid.UUID) error {
	var err error
	switch subjectType {
	case "user":
		_, err = q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: subjectID, EnterpriseID: enterpriseID})
	case "department":
		_, err = q.GetDepartment(ctx, db.GetDepartmentParams{ID: subjectID, EnterpriseID: enterpriseID})
	case "role":
		_, err = q.GetRole(ctx, db.GetRoleParams{ID: subjectID, EnterpriseID: enterpriseID})
	case "service_account":
		_, err = q.GetServiceAccount(ctx, db.GetServiceAccountParams{ID: subjectID, EnterpriseID: enterpriseID})
	default:
		return errors.New("invalid subject type")
	}
	if err != nil {
		return errors.New("subject does not belong to enterprise")
	}
	return nil
}

func (service Service) UpdateGrantBatch(ctx context.Context, actorID string, enterpriseID uuid.UUID, input GrantBatchInput) error {
	if input.SubjectType != "user" && input.SubjectType != "department" && input.SubjectType != "role" && input.SubjectType != "service_account" {
		return errors.New("invalid subject type")
	}
	if !ValidResourceType(input.ResourceType) {
		return errors.New("invalid resource type")
	}
	if len(input.ResourceIDs) == 0 {
		return nil
	}
	actor, err := uuid.Parse(actorID)
	if err != nil {
		return errors.New("invalid actor id")
	}
	return service.Store.InTx(ctx, func(q *db.Queries) error {
		if err := service.validateSubject(ctx, q, enterpriseID, input.SubjectType, input.SubjectID); err != nil {
			return err
		}
		if input.ExpectedVersion > 0 {
			current, versionErr := service.currentSubjectVersion(ctx, q, enterpriseID, input.SubjectType, input.SubjectID)
			if versionErr != nil {
				return versionErr
			}
			if current != input.ExpectedVersion {
				return ErrAuthorizationConflict
			}
		}
		for _, resourceID := range input.ResourceIDs {
			if err := service.validateResource(ctx, q, enterpriseID, input.ResourceType, resourceID); err != nil {
				return err
			}
			if input.Remove {
				if input.SubjectType != "role" || input.ResourceType == "kubernetes_namespace" {
					grants, listErr := q.ListDataAuthorizationGrants(ctx, db.ListDataAuthorizationGrantsParams{EnterpriseID: enterpriseID, SubjectType: input.SubjectType, SubjectID: input.SubjectID, ResourceType: input.ResourceType})
					if listErr != nil {
						return listErr
					}
					direct := false
					for _, grant := range grants {
						if grant.ResourceID == resourceID && grant.Status == "active" {
							direct = true
							break
						}
					}
					inherited, containsErr := service.effectiveResourceContains(ctx, q, enterpriseID, input.SubjectID, input.SubjectType, input.ResourceType, resourceID)
					if containsErr != nil {
						return containsErr
					}
					if !direct && inherited {
						return ErrInheritedGrant
					}
				}
				if err := q.DisableDataAuthorizationGrant(ctx, db.DisableDataAuthorizationGrantParams{EnterpriseID: enterpriseID, SubjectType: input.SubjectType, SubjectID: input.SubjectID, ResourceType: input.ResourceType, ResourceID: resourceID}); err != nil {
					return err
				}
				continue
			}
			if _, err := q.AddDataAuthorizationGrant(ctx, db.AddDataAuthorizationGrantParams{ID: newID(), EnterpriseID: enterpriseID, SubjectType: input.SubjectType, SubjectID: input.SubjectID, ResourceType: input.ResourceType, ResourceID: resourceID, CreatedBy: uuid.NullUUID{UUID: actor, Valid: true}}); err != nil {
				return err
			}
		}
		if err := service.invalidateSubject(ctx, q, enterpriseID, input.SubjectType, input.SubjectID); err != nil {
			return err
		}
		_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID, Action: "data_authorization.update", ResourceType: input.ResourceType, ResourceID: input.SubjectID.String(), Result: "success", Details: map[string]any{"subject_type": input.SubjectType, "removed": input.Remove, "count": len(input.ResourceIDs)}})
		return err
	})
}

func (service Service) effectiveResourceContains(ctx context.Context, q *db.Queries, enterpriseID, subjectID uuid.UUID, subjectType, resourceType string, resourceID uuid.UUID) (bool, error) {
	if resourceType == "kubernetes_namespace" {
		namespace, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: enterpriseID, ID: resourceID})
		if err != nil {
			return false, err
		}
		if inherited, err := service.effectiveResourceContains(ctx, q, enterpriseID, subjectID, subjectType, "kubernetes_cluster", namespace.ClusterID); err != nil || inherited {
			return inherited, err
		}
	}
	var values []uuid.UUID
	var err error
	switch subjectType {
	case "user":
		values, err = q.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: enterpriseID, UserID: subjectID, ResourceType: resourceType})
	case "department":
		values, err = q.ListDepartmentAuthorizedResourceIDs(ctx, db.ListDepartmentAuthorizedResourceIDsParams{EnterpriseID: enterpriseID, DepartmentID: subjectID, ResourceType: resourceType})
	case "service_account":
		values, err = q.ListServiceAccountAuthorizedResourceIDs(ctx, db.ListServiceAccountAuthorizedResourceIDsParams{EnterpriseID: enterpriseID, ServiceAccountID: subjectID, ResourceType: resourceType})
	case "role":
		grants, e := q.ListDataAuthorizationGrants(ctx, db.ListDataAuthorizationGrantsParams{EnterpriseID: enterpriseID, SubjectType: subjectType, SubjectID: subjectID, ResourceType: resourceType})
		err = e
		for _, grant := range grants {
			if grant.Status == "active" {
				values = append(values, grant.ResourceID)
			}
		}
	}
	if err != nil {
		return false, err
	}
	for _, value := range values {
		if value == resourceID {
			return true, nil
		}
	}
	return false, nil
}

func (service Service) currentSubjectVersion(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, subjectType string, subjectID uuid.UUID) (int64, error) {
	switch subjectType {
	case "user":
		value, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: subjectID, EnterpriseID: enterpriseID})
		return value.AuthorizationVersion, err
	case "service_account":
		value, err := q.GetServiceAccount(ctx, db.GetServiceAccountParams{ID: subjectID, EnterpriseID: enterpriseID})
		return value.AuthorizationVersion, err
	default:
		return q.GetAuthorizationVersion(ctx, db.GetAuthorizationVersionParams{EnterpriseID: enterpriseID, SubjectType: subjectType, SubjectID: subjectID})
	}
}

func (service Service) validateResource(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, resourceType string, resourceID uuid.UUID) error {
	switch resourceType {
	case "host":
		_, err := q.GetHost(ctx, db.GetHostParams{ID: resourceID, EnterpriseID: enterpriseID})
		if err != nil {
			return ErrInvalidResource
		}
	case "kubernetes_cluster":
		_, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: resourceID, EnterpriseID: enterpriseID})
		if err != nil {
			return ErrInvalidResource
		}
	case "dashboard":
		_, err := q.GetDashboard(ctx, db.GetDashboardParams{ID: resourceID, EnterpriseID: enterpriseID})
		if err != nil {
			return ErrInvalidResource
		}
	case "application", "monitoring_target", "kubernetes_namespace":
		found, err := q.MonitoringGrantResourceExists(ctx, db.MonitoringGrantResourceExistsParams{EnterpriseID: enterpriseID, ID: resourceID, ResourceType: resourceType})
		if err != nil || !found {
			return ErrInvalidResource
		}
	default:
		return ErrInvalidResource
	}
	return nil
}

func (service Service) invalidateSubject(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, subjectType string, subjectID uuid.UUID) error {
	var affected []uuid.UUID
	var err error
	switch subjectType {
	case "user":
		_, err = q.BumpUserAuthorizationVersion(ctx, db.BumpUserAuthorizationVersionParams{ID: subjectID, EnterpriseID: enterpriseID})
		if err == nil {
			affected = []uuid.UUID{subjectID}
		}
	case "service_account":
		_, err = q.BumpServiceAccountAuthorizationVersion(ctx, db.BumpServiceAccountAuthorizationVersionParams{ID: subjectID, EnterpriseID: enterpriseID})
	case "department":
		if err := q.BumpAuthorizationVersionRecord(ctx, db.BumpAuthorizationVersionRecordParams{EnterpriseID: enterpriseID, SubjectType: subjectType, SubjectID: subjectID}); err != nil {
			return err
		}
		affected, err = q.BumpDepartmentUsersAuthorizationVersion(ctx, db.BumpDepartmentUsersAuthorizationVersionParams{EnterpriseID: enterpriseID, DepartmentID: subjectID})
	case "role":
		if err := q.BumpAuthorizationVersionRecord(ctx, db.BumpAuthorizationVersionRecordParams{EnterpriseID: enterpriseID, SubjectType: subjectType, SubjectID: subjectID}); err != nil {
			return err
		}
		var userIDs, departmentUserIDs, serviceAccountIDs []uuid.UUID
		userIDs, err = q.ListUserIDsForRole(ctx, db.ListUserIDsForRoleParams{EnterpriseID: enterpriseID, RoleID: subjectID})
		if err == nil {
			departmentUserIDs, err = q.ListUserIDsForDepartmentRole(ctx, db.ListUserIDsForDepartmentRoleParams{EnterpriseID: enterpriseID, RoleID: subjectID})
		}
		if err == nil {
			serviceAccountIDs, err = q.ListServiceAccountIDsForRole(ctx, db.ListServiceAccountIDsForRoleParams{EnterpriseID: enterpriseID, RoleID: subjectID})
		}
		if err == nil {
			seen := make(map[uuid.UUID]struct{})
			for _, id := range append(userIDs, departmentUserIDs...) {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					if _, bumpErr := q.BumpUserAuthorizationVersion(ctx, db.BumpUserAuthorizationVersionParams{ID: id, EnterpriseID: enterpriseID}); bumpErr != nil {
						return bumpErr
					}
					affected = append(affected, id)
				}
			}
			for _, id := range serviceAccountIDs {
				if _, bumpErr := q.BumpServiceAccountAuthorizationVersion(ctx, db.BumpServiceAccountAuthorizationVersionParams{ID: id, EnterpriseID: enterpriseID}); bumpErr != nil {
					return bumpErr
				}
			}
		}
	default:
		return errors.New("invalid subject type")
	}
	if err != nil {
		return err
	}
	if err := revocation.Users(ctx, q, enterpriseID, affected, subjectType+"_authorization_changed"); err != nil {
		return err
	}
	return queriesOutbox(ctx, q, enterpriseID, subjectType, subjectID, len(affected))
}

func queriesOutbox(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, subjectType string, subjectID uuid.UUID, affected int) error {
	payload, _ := json.Marshal(map[string]any{"enterprise_id": enterpriseID.String(), "subject_type": subjectType, "subject_id": subjectID.String(), "affected_user_count": affected})
	return q.InsertOutboxEvent(ctx, db.InsertOutboxEventParams{ID: newID(), Topic: "authorization.changed", AggregateType: subjectType, AggregateID: subjectID.String(), Payload: payload})
}

func (service Service) Permissions(ctx context.Context) ([]string, error) {
	return service.Store.Queries.ListPermissions(ctx)
}

func (service Service) ListRoles(ctx context.Context, enterpriseID uuid.UUID) ([]RoleRecord, error) {
	roles, err := service.Store.Queries.ListRoles(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	result := make([]RoleRecord, 0, len(roles))
	for _, role := range roles {
		permissions, err := service.Store.Queries.ListRolePermissions(ctx, role.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, RoleRecord{Role: role, Permissions: permissions})
	}
	return result, nil
}

func (service Service) CreateRole(ctx context.Context, actorID string, enterpriseID uuid.UUID, input RoleInput, idempotencyKey string) (RoleRecord, error) {
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", actorID, "role.create", idempotencyKey, input, 201, func(queries *db.Queries) (RoleRecord, error) {
		role, err := queries.CreateRole(ctx, db.CreateRoleParams{ID: newID(), EnterpriseID: enterpriseID, Name: input.Name, Description: input.Description})
		if err != nil {
			return RoleRecord{}, err
		}
		for _, permission := range input.Permissions {
			if _, exists := PermissionRegistry[permission]; !exists {
				return RoleRecord{}, errors.New("unknown permission")
			}
			if err := queries.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: role.ID, PermissionID: permission}); err != nil {
				return RoleRecord{}, err
			}
		}
		if _, err := audit.Append(ctx, queries, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID,
			Action: "role.create", ResourceType: "role", ResourceID: role.ID.String(), Result: "success", Details: map[string]any{"summary": "custom role created"}}); err != nil {
			return RoleRecord{}, err
		}
		return RoleRecord{Role: role, Permissions: append([]string(nil), input.Permissions...)}, nil
	})
}

func (service Service) UpdateRole(ctx context.Context, actorID string, enterpriseID, roleID uuid.UUID, input RoleInput) (RoleRecord, error) {
	var result RoleRecord
	err := service.Store.InTx(ctx, func(queries *db.Queries) error {
		current, err := queries.GetRole(ctx, db.GetRoleParams{ID: roleID, EnterpriseID: enterpriseID})
		if err != nil {
			return err
		}
		if current.Builtin {
			return ErrBuiltinRoleImmutable
		}
		role, err := queries.UpdateRole(ctx, db.UpdateRoleParams{ID: roleID, EnterpriseID: enterpriseID, ExpectedVersion: input.ExpectedVersion,
			Name: optionalText(input.Name), Description: optionalText(input.Description), Status: textValue(input.Status)})
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.ErrVersionConflict
		}
		if err != nil {
			return err
		}
		if input.Permissions != nil {
			if err := queries.DeleteRolePermissions(ctx, roleID); err != nil {
				return err
			}
			for _, permission := range input.Permissions {
				if _, exists := PermissionRegistry[permission]; !exists {
					return errors.New("unknown permission")
				}
				if err := queries.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: roleID, PermissionID: permission}); err != nil {
					return err
				}
			}
		}
		permissions, err := queries.ListRolePermissions(ctx, roleID)
		if err != nil {
			return err
		}
		if err := service.invalidateSubject(ctx, queries, enterpriseID, "role", roleID); err != nil {
			return err
		}
		_, err = audit.Append(ctx, queries, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID,
			Action: "role.update", ResourceType: "role", ResourceID: roleID.String(), Result: "success", Details: map[string]any{"summary": "custom role updated"}})
		result = RoleRecord{Role: role, Permissions: permissions}
		return err
	})
	return result, err
}

func (service Service) ListBindings(ctx context.Context, enterpriseID uuid.UUID) ([]BindingRecord, error) {
	bindings, err := service.Store.Queries.ListRoleBindings(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	result := make([]BindingRecord, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, BindingRecord{Binding: binding})
	}
	return result, nil
}

func (service Service) GetUserRoleAssignments(ctx context.Context, enterpriseID, userID uuid.UUID) (UserRoleAssignments, error) {
	return service.userRoleAssignments(ctx, service.Store.Queries, enterpriseID, userID)
}

func (service Service) ReplaceUserRoleAssignments(ctx context.Context, actorID string, enterpriseID, userID uuid.UUID, input UserRoleAssignmentsUpdate, idempotencyKey string) (UserRoleAssignments, error) {
	roleIDs := uniqueUUIDs(input.RoleIDs)
	request := map[string]any{"user_id": userID.String(), "department_id": input.DepartmentID.String(), "role_ids": roleIDs, "expected_user_version": input.ExpectedUserVersion, "expected_authorization_version": input.ExpectedAuthorizationVersion}
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", actorID, "role_assignment.replace", idempotencyKey, request, 200, func(q *db.Queries) (UserRoleAssignments, error) {
		if _, err := q.LockEnterpriseForAccessUpdate(ctx, enterpriseID); err != nil {
			return UserRoleAssignments{}, err
		}
		user, err := q.LockEnterpriseUserForAccessUpdate(ctx, db.LockEnterpriseUserForAccessUpdateParams{ID: userID, EnterpriseID: enterpriseID})
		if err != nil {
			return UserRoleAssignments{}, err
		}
		if user.Version != input.ExpectedUserVersion || user.AuthorizationVersion != input.ExpectedAuthorizationVersion {
			return UserRoleAssignments{}, ErrAuthorizationConflict
		}
		if _, err := q.GetDepartment(ctx, db.GetDepartmentParams{ID: input.DepartmentID, EnterpriseID: enterpriseID}); err != nil {
			return UserRoleAssignments{}, err
		}
		current, err := service.userRoleAssignments(ctx, q, enterpriseID, userID)
		if err != nil {
			return UserRoleAssignments{}, err
		}
		for _, roleID := range roleIDs {
			role, roleErr := q.GetRole(ctx, db.GetRoleParams{ID: roleID, EnterpriseID: enterpriseID})
			if roleErr != nil || role.Status != "active" {
				return UserRoleAssignments{}, ErrInvalidRoleAssignment
			}
		}
		departmentChanged := user.DepartmentID != input.DepartmentID
		rolesChanged := !sameUUIDSet(current.DirectRoleIDs, roleIDs)
		if !departmentChanged && !rolesChanged {
			return current, nil
		}
		if rolesChanged {
			for _, roleID := range roleIDs {
				if _, err := q.UpsertPermanentUserRoleBinding(ctx, db.UpsertPermanentUserRoleBindingParams{ID: newID(), EnterpriseID: enterpriseID, UserID: userID, RoleID: roleID}); err != nil {
					return UserRoleAssignments{}, err
				}
			}
			if err := q.DisableUserRoleBindingsExcept(ctx, db.DisableUserRoleBindingsExceptParams{EnterpriseID: enterpriseID, UserID: userID, RoleIds: roleIDs}); err != nil {
				return UserRoleAssignments{}, err
			}
		}
		version := user.AuthorizationVersion
		if departmentChanged {
			updated, updateErr := q.UpdateEnterpriseUser(ctx, db.UpdateEnterpriseUserParams{ID: userID, EnterpriseID: enterpriseID, DepartmentID: uuid.NullUUID{UUID: input.DepartmentID, Valid: true}, ExpectedVersion: input.ExpectedUserVersion})
			if updateErr != nil {
				return UserRoleAssignments{}, updateErr
			}
			version = updated.AuthorizationVersion
		} else {
			version, err = q.BumpUserAuthorizationVersionExpected(ctx, db.BumpUserAuthorizationVersionExpectedParams{ID: userID, EnterpriseID: enterpriseID, ExpectedAuthorizationVersion: input.ExpectedAuthorizationVersion})
			if errors.Is(err, pgx.ErrNoRows) {
				return UserRoleAssignments{}, ErrAuthorizationConflict
			}
			if err != nil {
				return UserRoleAssignments{}, err
			}
		}
		managerCount, err := q.CountEnterpriseIAMManagers(ctx, enterpriseID)
		if err != nil {
			return UserRoleAssignments{}, err
		}
		if managerCount == 0 {
			return UserRoleAssignments{}, ErrLastIAMAdminRequired
		}
		if err := revocation.Users(ctx, q, enterpriseID, []uuid.UUID{userID}, "user_role_assignment_changed"); err != nil {
			return UserRoleAssignments{}, err
		}
		if err := queriesOutbox(ctx, q, enterpriseID, "user", userID, 1); err != nil {
			return UserRoleAssignments{}, err
		}
		_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID,
			Action: "role_assignment.replace", ResourceType: "enterprise_user", ResourceID: userID.String(), Result: "success", Details: map[string]any{"direct_role_count": len(roleIDs), "authorization_version": version}})
		if err != nil {
			return UserRoleAssignments{}, err
		}
		return service.userRoleAssignments(ctx, q, enterpriseID, userID)
	})
}

func (service Service) userRoleAssignments(ctx context.Context, q *db.Queries, enterpriseID, userID uuid.UUID) (UserRoleAssignments, error) {
	user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: userID, EnterpriseID: enterpriseID})
	if err != nil {
		return UserRoleAssignments{}, err
	}
	direct, err := q.ListEffectiveRoleBindingsForSubject(ctx, db.ListEffectiveRoleBindingsForSubjectParams{EnterpriseID: enterpriseID, SubjectType: "user", SubjectID: userID})
	if err != nil {
		return UserRoleAssignments{}, err
	}
	inherited, err := q.ListEffectiveRoleBindingsForSubject(ctx, db.ListEffectiveRoleBindingsForSubjectParams{EnterpriseID: enterpriseID, SubjectType: "department", SubjectID: user.DepartmentID})
	if err != nil {
		return UserRoleAssignments{}, err
	}
	department, err := q.GetDepartment(ctx, db.GetDepartmentParams{ID: user.DepartmentID, EnterpriseID: enterpriseID})
	if err != nil {
		return UserRoleAssignments{}, err
	}
	result := UserRoleAssignments{AuthorizationVersion: user.AuthorizationVersion}
	effective := make(map[uuid.UUID]struct{}, len(direct)+len(inherited))
	for _, binding := range direct {
		result.DirectRoleIDs = append(result.DirectRoleIDs, binding.RoleID)
		effective[binding.RoleID] = struct{}{}
	}
	for _, binding := range inherited {
		result.InheritedRoles = append(result.InheritedRoles, InheritedRoleAssignment{RoleID: binding.RoleID, SourceID: department.ID, SourceName: department.Name})
		effective[binding.RoleID] = struct{}{}
	}
	for roleID := range effective {
		result.EffectiveRoleIDs = append(result.EffectiveRoleIDs, roleID)
	}
	sortUUIDs(result.DirectRoleIDs)
	sort.Slice(result.InheritedRoles, func(i, j int) bool {
		return result.InheritedRoles[i].RoleID.String() < result.InheritedRoles[j].RoleID.String()
	})
	sortUUIDs(result.EffectiveRoleIDs)
	return result, nil
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sortUUIDs(result)
	return result
}

func sameUUIDSet(left, right []uuid.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := make(map[uuid.UUID]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := leftSet[value]; !ok {
			return false
		}
	}
	return true
}

func sortUUIDs(values []uuid.UUID) {
	sort.Slice(values, func(i, j int) bool { return values[i].String() < values[j].String() })
}

func (service Service) CreateBinding(ctx context.Context, actorID string, enterpriseID uuid.UUID, input BindingInput, idempotencyKey string) (BindingRecord, error) {
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", actorID, "role_binding.create", idempotencyKey, input, 201, func(queries *db.Queries) (BindingRecord, error) {
		if input.SubjectType != "user" && input.SubjectType != "department" && input.SubjectType != "service_account" {
			return BindingRecord{}, ErrInvalidRoleAssignment
		}
		if err := service.validateSubject(ctx, queries, enterpriseID, input.SubjectType, input.SubjectID); err != nil {
			return BindingRecord{}, err
		}
		if _, err := queries.GetRole(ctx, db.GetRoleParams{EnterpriseID: enterpriseID, ID: input.RoleID}); err != nil {
			return BindingRecord{}, ErrInvalidRoleAssignment
		}
		binding, err := queries.CreateRoleBinding(ctx, db.CreateRoleBindingParams{ID: newID(), EnterpriseID: enterpriseID, SubjectType: input.SubjectType,
			SubjectID: input.SubjectID, RoleID: input.RoleID, ValidFrom: timeValue(input.ValidFrom), ValidUntil: timeValue(input.ValidUntil)})
		if err != nil {
			return BindingRecord{}, err
		}
		if err := service.invalidateSubject(ctx, queries, enterpriseID, binding.SubjectType, binding.SubjectID); err != nil {
			return BindingRecord{}, err
		}
		_, err = audit.Append(ctx, queries, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID,
			Action: "role_binding.create", ResourceType: "role_binding", ResourceID: binding.ID.String(), Result: "success", Details: map[string]any{"summary": "role binding created"}})
		if err != nil {
			return BindingRecord{}, err
		}
		return BindingRecord{Binding: binding}, nil
	})
}

func (service Service) UpdateBinding(ctx context.Context, actorID string, enterpriseID, bindingID uuid.UUID, input BindingInput) (BindingRecord, error) {
	var result BindingRecord
	err := service.Store.InTx(ctx, func(queries *db.Queries) error {
		binding, err := queries.UpdateRoleBinding(ctx, db.UpdateRoleBindingParams{ID: bindingID, EnterpriseID: enterpriseID, ExpectedVersion: input.ExpectedVersion,
			ValidFrom: timeValue(input.ValidFrom), SetValidFrom: input.SetValidFrom, ValidUntil: timeValue(input.ValidUntil), SetValidUntil: input.SetValidUntil, Status: textValue(input.Status)})
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.ErrVersionConflict
		}
		if err != nil {
			return err
		}
		if err := service.invalidateSubject(ctx, queries, enterpriseID, binding.SubjectType, binding.SubjectID); err != nil {
			return err
		}
		_, err = audit.Append(ctx, queries, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterpriseID, Valid: true}, ActorType: identity.ActorTypeFromContext(ctx), ActorID: actorID,
			Action: "role_binding.update", ResourceType: "role_binding", ResourceID: bindingID.String(), Result: "success", Details: map[string]any{"summary": "role binding updated"}})
		result = BindingRecord{Binding: binding}
		return err
	})
	return result, err
}

func newID() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id
}
func textValue(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
func optionalText(value string) pgtype.Text { return pgtype.Text{String: value, Valid: value != ""} }
func timeValue(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
