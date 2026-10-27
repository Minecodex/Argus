// Package resourceref is the storage-independent identity contract shared by
// telemetry scope resolution, transport, query snapshots and caches.
package resourceref

import (
	"errors"
	"github.com/google/uuid"
	"slices"
	"sort"
)

var ErrInvalid = errors.New("invalid typed telemetry resource scope")

type Ref struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

func ValidType(kind string) bool {
	return ValidTelemetryRoot(kind) || kind == "kubernetes_namespace"
}

// Namespace is a selection authority; telemetry keeps its observed physical
// application/cluster identity. It must be resolved before an engine request.
func ValidTelemetryRoot(kind string) bool {
	return slices.Contains([]string{"host", "kubernetes_cluster", "application", "monitoring_target"}, kind)
}
func Normalize(values []Ref) ([]Ref, error) {
	if len(values) > 1000 {
		return nil, ErrInvalid
	}
	out := append([]Ref{}, values...)
	seen := map[uuid.UUID]string{}
	for _, ref := range out {
		if ref.ID == uuid.Nil || !ValidType(ref.Type) {
			return nil, ErrInvalid
		}
		if kind, exists := seen[ref.ID]; exists && kind != ref.Type {
			return nil, ErrInvalid
		}
		seen[ref.ID] = ref.Type
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return slices.Compact(out), nil
}
func IDs(values []Ref) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(values))
	for _, ref := range values {
		out = append(out, ref.ID)
	}
	return out
}
func MatchIDs(values []Ref, ids []uuid.UUID) bool {
	if len(values) != len(ids) {
		return false
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, ref := range values {
		if !seen[ref.ID] {
			return false
		}
		delete(seen, ref.ID)
	}
	return len(seen) == 0
}
func Subset(values []Ref, ids []uuid.UUID) ([]Ref, error) {
	out := []Ref{}
	for _, ref := range values {
		if slices.Contains(ids, ref.ID) {
			out = append(out, ref)
		}
	}
	normalized, err := Normalize(out)
	if err != nil || !MatchIDs(normalized, ids) {
		return nil, ErrInvalid
	}
	return normalized, nil
}
