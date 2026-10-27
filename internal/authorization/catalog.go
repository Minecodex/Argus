package authorization

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"sort"
	"unicode/utf8"
)

// GrantCatalogPage retrieves only one catalog page. The caller binds cursors to
// both the requesting administrator and the selected subject's grant version.
func (s Service) GrantCatalogPage(ctx context.Context, enterprise, subject uuid.UUID, subjectType, resourceType, search string, after uuid.UUID, limit int32) ([]GrantResource, error) {
	if !ValidResourceType(resourceType) || utf8.RuneCountInString(search) > 200 || limit < 1 || limit > 201 {
		return nil, ErrInvalidResource
	}
	if err := s.validateSubject(ctx, s.Store.Queries, enterprise, subjectType, subject); err != nil {
		return nil, err
	}
	rows, err := s.Store.Queries.ListDataAuthorizationCatalogPage(ctx, db.ListDataAuthorizationCatalogPageParams{EnterpriseID: enterprise, ResourceType: resourceType, Search: search, AfterID: after, PageLimit: limit})
	if err != nil {
		return nil, err
	}
	ids, parents := []uuid.UUID{}, []uuid.UUID{}
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.ParentID.Valid {
			parents = append(parents, row.ParentID.UUID)
		}
	}
	grants, err := s.Store.Queries.ListCatalogGrantsForResources(ctx, db.ListCatalogGrantsForResourcesParams{EnterpriseID: enterprise, SubjectType: subjectType, SubjectID: subject, ResourceType: resourceType, ResourceIds: ids})
	if err != nil {
		return nil, err
	}
	direct := map[uuid.UUID]bool{}
	for _, grant := range grants {
		direct[grant.ResourceID] = true
	}
	inherited, err := s.inheritedSources(ctx, enterprise, subject, subjectType, resourceType, ids)
	if err != nil {
		return nil, err
	}
	parentGrants := map[uuid.UUID]bool{}
	if resourceType == "kubernetes_namespace" {
		parentRows, e := s.Store.Queries.ListCatalogGrantsForResources(ctx, db.ListCatalogGrantsForResourcesParams{EnterpriseID: enterprise, SubjectType: subjectType, SubjectID: subject, ResourceType: "kubernetes_cluster", ResourceIds: parents})
		if e != nil {
			return nil, e
		}
		for _, grant := range parentRows {
			parentGrants[grant.ResourceID] = true
		}
		parentsInherited, e := s.inheritedSources(ctx, enterprise, subject, subjectType, "kubernetes_cluster", parents)
		if e != nil {
			return nil, e
		}
		for id := range parentsInherited {
			parentGrants[id] = true
		}
	}
	out := make([]GrantResource, 0, len(rows))
	for _, row := range rows {
		sources := append([]string{}, inherited[row.ID]...)
		if row.ParentID.Valid && parentGrants[row.ParentID.UUID] {
			sources = append(sources, "cluster:"+row.ParentName)
		}
		sort.Strings(sources)
		entry := GrantResource{ResourceType: resourceType, ResourceID: row.ID, Name: row.Name, Environment: row.Environment, ParentID: row.ParentID, ParentName: row.ParentName, SourceUID: row.SourceUid, Direct: direct[row.ID], Inherited: len(sources) > 0, Sources: sources}
		// Preserve inherited evidence alongside a direct grant so removing it
		// cannot be presented as revoking access that still comes from a parent.
		if entry.Direct {
			entry.Sources = append([]string{"direct"}, entry.Sources...)
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s Service) inheritedSources(ctx context.Context, enterprise, subject uuid.UUID, subjectType, resourceType string, ids []uuid.UUID) (map[uuid.UUID][]string, error) {
	out := map[uuid.UUID][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	appendGrants := func(kind string, id uuid.UUID, label string) error {
		rows, err := s.Store.Queries.ListCatalogGrantsForResources(ctx, db.ListCatalogGrantsForResourcesParams{EnterpriseID: enterprise, SubjectType: kind, SubjectID: id, ResourceType: resourceType, ResourceIds: ids})
		if err != nil {
			return err
		}
		for _, row := range rows {
			out[row.ResourceID] = append(out[row.ResourceID], label)
		}
		return nil
	}
	if subjectType == "user" {
		user, err := s.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: enterprise, ID: subject})
		if err != nil {
			return nil, err
		}
		department, err := s.Store.Queries.GetDepartment(ctx, db.GetDepartmentParams{EnterpriseID: enterprise, ID: user.DepartmentID})
		if err != nil {
			return nil, err
		}
		if department.Status == "active" {
			if err = appendGrants("department", department.ID, "department:"+department.Name); err != nil {
				return nil, err
			}
		}
	}
	roles, err := s.Store.Queries.ListCatalogSourceRoles(ctx, db.ListCatalogSourceRolesParams{EnterpriseID: enterprise, SubjectType: subjectType, SubjectID: subject})
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		if err = appendGrants("role", role.ID, "role:"+role.Name); err != nil {
			return nil, err
		}
	}
	return out, nil
}
