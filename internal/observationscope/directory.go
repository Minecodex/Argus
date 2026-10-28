package observationscope

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type DirectoryInput struct {
	Kind, Search, Cursor string
	Limit                int
}
type DirectoryItem struct {
	ID          uuid.UUID  `json:"id"`
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Environment string     `json:"environment"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	ParentName  string     `json:"parent_name,omitempty"`
	SourceUID   string     `json:"source_uid,omitempty"`
}
type DirectoryPage struct {
	Items      []DirectoryItem `json:"items"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

// ScopeDirectory is a paginated selection catalog, independent of opening the
// host, Kubernetes or application management menus. Every item needs a grant.
func (r Resolver) ResourceDirectory(ctx context.Context, a authorization.Actor, key []byte, in DirectoryInput) (DirectoryPage, error) {
	out := DirectoryPage{Items: []DirectoryItem{}}
	if r.Queries == nil {
		return out, ErrInvalid
	}
	q := r.Queries
	permission := r.Permission
	if permission == "" {
		permission = RootPermission(in.Kind)
	}
	if permission == "" {
		return out, ErrInvalid
	}
	if err := authorization.Authorize(ctx, q, a, permission); err != nil {
		return out, err
	}
	in.Search = strings.TrimSpace(in.Search)
	if in.Limit == 0 {
		in.Limit = 30
	}
	if !resourceref.ValidType(in.Kind) || in.Limit < 1 || in.Limit > 100 || len(in.Search) > 200 || len(in.Cursor) > 4096 {
		return out, ErrInvalid
	}
	binding := pagination.Binding{Audience: "resource_scope_directory/v1/" + permission, EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(struct{ Kind, Search string }{in.Kind, in.Search}), Sort: "id:asc"}
	signer := pagination.Signer{Key: key}
	after := uuid.Nil
	if in.Cursor != "" {
		position, e := signer.Decode(in.Cursor, binding)
		if e != nil {
			if errors.Is(e, pagination.ErrAuthorizationVersionStale) {
				return out, ErrDenied
			}
			return out, ErrInvalid
		}
		after, e = uuid.Parse(position.ID)
		if e != nil {
			return out, ErrInvalid
		}
	}
	ids, err := authorization.AuthorizedResourceIDs(ctx, q, a, in.Kind)
	if err != nil {
		return out, err
	}
	rows, err := q.ListObservationResourceDirectoryPage(ctx, db.ListObservationResourceDirectoryPageParams{EnterpriseID: a.EnterpriseID, Kind: in.Kind, AuthorizedIds: ids, AfterID: after, Search: in.Search, PageLimit: int32(in.Limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > in.Limit {
		out.NextCursor, err = signer.Encode(binding, pagination.Position{ID: rows[in.Limit-1].ID.String(), Time: time.Now().UTC()})
		if err != nil {
			return out, err
		}
		rows = rows[:in.Limit]
	}
	current, err := authorization.AuthorizedResourceIDs(ctx, q, a, in.Kind)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		if !slices.Contains(current, row.ID) {
			return DirectoryPage{}, ErrDenied
		}
		item := DirectoryItem{ID: row.ID, Type: row.Kind, Name: row.Name, Environment: row.Environment, ParentName: row.ParentName, SourceUID: row.NamespaceUid}
		if row.ParentID.Valid {
			id := row.ParentID.UUID
			item.ParentID = &id
		}
		out.Items = append(out.Items, item)
	}
	return out, authorization.Authorize(ctx, q, a, permission)
}
