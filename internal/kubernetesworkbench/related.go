package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/pagination"
	"slices"
	"time"
)

type RelatedInput struct {
	ObjectInput
	View   string `json:"view"`
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}
type RelatedPage struct {
	Items      []SnapshotResource `json:"items"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Complete   bool               `json:"complete"`
	Errors     map[string]string  `json:"errors"`
	ObservedAt time.Time          `json:"observed_at"`
}
type relatedPosition struct {
	Type   int    `json:"type"`
	Cursor string `json:"cursor"`
}

// Related reads one bounded native page at a time. Cursors bind the actor,
// authorization version, source UID, selected view and scope. No snapshot-wide
// browser filtering or unbounded scan is necessary to continue reading.
func (s Service) Related(ctx context.Context, a Actor, clusterID uuid.UUID, in RelatedInput) (RelatedPage, error) {
	out := RelatedPage{Items: []SnapshotResource{}, Errors: map[string]string{}, Complete: true, ObservedAt: time.Now().UTC()}
	if in.Limit == 0 {
		in.Limit = 30
	}
	if in.Limit < 1 || in.Limit > 100 || len(in.Cursor) > 65536 || (in.View != "relations" && in.View != "events") {
		return out, kubernetesinventory.ErrInvalid
	}
	source, err := s.Get(ctx, a, clusterID, in.ObjectInput)
	if err != nil {
		return out, err
	}
	_, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	signature := scopeSignature(scope, scope.Namespaces)
	selection := Selection{Mode: "all_authorized"}
	if source.Definition.Namespaced {
		selection = Selection{Mode: "explicit", IDs: []uuid.UUID{in.NamespaceID}}
	}
	if in.Type == "namespace" {
		refs, e := s.observeNamespaces(ctx, a.EnterpriseID, clusterID, []kubernetesinventory.Object{source})
		if e != nil {
			return out, e
		}
		selection = Selection{Mode: "explicit", IDs: []uuid.UUID{refs[source.UID]}}
	}
	types := relatedTypes(source, in.View)
	filter := in
	filter.Cursor = ""
	binding := pagination.Binding{Audience: "kubernetes_related/v1", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(struct {
		Input      RelatedInput
		Scope      string
		Types      []string
		Generation int64
	}{filter, signature, types, source.Generation}), Sort: "type-native-snapshot"}
	signer := pagination.Signer{Key: s.CursorKey}
	next := relatedPosition{}
	if in.Cursor != "" {
		decoded, e := signer.Decode(in.Cursor, binding)
		if e != nil {
			return out, e
		}
		if json.Unmarshal([]byte(decoded.ID), &next) != nil || next.Type < 0 || next.Type >= len(types) {
			return out, kubernetesinventory.ErrInvalid
		}
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for scans := 0; next.Type < len(types) && scans < 8 && len(out.Items) == 0; scans++ {
		key := types[next.Type]
		selected := relatedNamespaceSelection(source, key, in.View, selection)
		def, ok := kubernetesinventory.Lookup(key)
		if ok && !def.Namespaced {
			if !scope.ClusterWide {
				next.Type++
				next.Cursor = ""
				continue
			}
			selected = Selection{}
		}
		page, e := s.List(ctx, a, clusterID, ListInput{Type: key, Namespaces: selected, Limit: in.Limit, Cursor: next.Cursor})
		if e != nil {
			if errors.Is(e, authorization.ErrAccessDenied) {
				return RelatedPage{}, e
			}
			out.Errors[key] = kubernetesinventory.ErrorCode(e)
			out.Complete = false
			next.Type++
			next.Cursor = ""
			continue
		}
		for _, candidate := range page.Items {
			if !isRelated(source, candidate, in.View) {
				continue
			}
			ref := ResourceReference{Type: candidate.Definition.Key, Name: candidate.Name, UID: candidate.UID, Namespace: candidate.Namespace, NamespaceUID: candidate.NamespaceUID}
			if id, ok := page.NamespaceReferences[candidate.NamespaceUID]; ok {
				ref.NamespaceID = &id
			}
			out.Items = append(out.Items, SnapshotResource{ResourceReference: ref, Summary: kubernetesinventory.Summarize(candidate), Actions: candidate.Actions, CreatedAt: candidate.CreatedAt})
		}
		next.Cursor = page.NextCursor
		if next.Cursor == "" {
			next.Type++
			if !page.Complete {
				out.Complete = false
				out.Errors[key] = "KUBERNETES_RESULT_BUDGET_EXCEEDED"
			}
		}
	}
	if next.Type < len(types) {
		raw, _ := json.Marshal(next)
		out.NextCursor, err = signer.Encode(binding, pagination.Position{Time: out.ObservedAt, ID: string(raw)})
		if err != nil {
			return out, err
		}
	}
	_, latest, err := s.scope(parent, a, clusterID)
	if err != nil {
		return RelatedPage{}, err
	}
	if signature != scopeSignature(latest, latest.Namespaces) {
		return RelatedPage{}, authorization.ErrAccessDenied
	}
	if _, err = s.Get(parent, a, clusterID, in.ObjectInput); err != nil {
		return RelatedPage{}, err
	}
	return out, nil
}

func relatedTypes(source kubernetesinventory.Object, view string) []string {
	if view == "events" {
		return []string{"event"}
	}
	if source.Definition.Key == "crd" {
		var data struct {
			Spec struct {
				Group    string
				Names    struct{ Plural string }
				Versions []struct {
					Name            string
					Served, Storage bool
				}
			}
		}
		if json.Unmarshal(source.Data, &data) != nil {
			return nil
		}
		version := ""
		for _, v := range data.Spec.Versions {
			if v.Served && (version == "" || v.Storage) {
				version = v.Name
			}
		}
		if version == "" {
			return nil
		}
		return []string{"custom/" + data.Spec.Group + "/" + version + "/" + data.Spec.Names.Plural + "@" + source.UID}
	}
	return append(slices.DeleteFunc(slices.Clone(snapshotTypes), func(key string) bool { return key == "event" }), "role", "rolebinding", "clusterrole", "clusterrolebinding", "controllerrevision", "resourcequota", "limitrange")
}
func isRelated(source, candidate kubernetesinventory.Object, view string) bool {
	if source.UID == candidate.UID {
		return false
	}
	if view == "events" {
		return kubernetesinventory.Summarize(candidate).Fields["involved_uid"] == source.UID
	}
	if source.Definition.Key == "namespace" {
		return candidate.NamespaceUID == source.UID
	}
	if source.Definition.Key == "crd" {
		return slices.Contains(relatedTypes(source, view), candidate.Definition.Key)
	}
	return len(resourceRelations([]kubernetesinventory.Object{source, candidate})) > 0
}

// Only RBAC subject references can cross namespaces here. List still applies the
// caller's current authorized namespaces before matching the explicit reference.
func relatedNamespaceSelection(source kubernetesinventory.Object, target, view string, local Selection) Selection {
	if view == "relations" && ((source.Definition.Key == "serviceaccount" && target == "rolebinding") || (source.Definition.Key == "rolebinding" && target == "serviceaccount")) {
		return Selection{Mode: "all_authorized"}
	}
	return local
}
