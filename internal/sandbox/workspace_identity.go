package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
)

var ErrWorkspaceRuntimeChanged = errors.New("workspace runtime identity or revision changed")

// WorkspaceIdentity contains public identifiers only, never endpoints or keys.
// Version is persisted in the Run's immutable tool definitions.
type WorkspaceIdentity struct {
	ProfileID       uuid.UUID `json:"profile_id"`
	ProfileRevision int32     `json:"profile_revision"`
	BackendID       uuid.UUID `json:"backend_id"`
	BackendVersion  int64     `json:"backend_version"`
	ImageID         uuid.UUID `json:"image_id"`
	ImageVersion    int64     `json:"image_version"`
	ImageDigest     string    `json:"image_digest"`
}

func (r WorkspaceRuntime) Identity() WorkspaceIdentity {
	return WorkspaceIdentity{r.Profile.ID, r.Profile.Revision, r.Backend.ID, r.Backend.Version, r.Image.ID, r.Image.Version, r.Image.Digest}
}

func (i WorkspaceIdentity) Version() string {
	data, _ := json.Marshal(i)
	return "workspace/v1:" + string(data)
}

func (i WorkspaceIdentity) Label() string {
	hash := sha256.Sum256([]byte(i.Version()))
	// Kubernetes label values must start and end with an alphanumeric byte.
	return "r" + base64.RawURLEncoding.EncodeToString(hash[:]) + "v1"
}

func (service Service) WorkspaceRuntimeByID(ctx context.Context, id uuid.UUID) (WorkspaceRuntime, error) {
	profile, err := service.Store.Queries.GetSandboxProfile(ctx, id)
	if err != nil || profile.Status != "enabled" || profile.NetworkMode != "none" || !slices.Contains(profile.TaskKinds, "agent_workspace") {
		return WorkspaceRuntime{}, ErrUnavailable
	}
	image, err := service.Store.Queries.GetSandboxImage(ctx, profile.ImageID)
	if err != nil || image.Status != "enabled" || image.BackendID != profile.BackendID {
		return WorkspaceRuntime{}, ErrUnavailable
	}
	backend, client, err := service.client(ctx, profile.BackendID)
	if err != nil {
		return WorkspaceRuntime{}, err
	}
	if backend.HealthStatus != "healthy" {
		return WorkspaceRuntime{}, ErrUnavailable
	}
	return WorkspaceRuntime{Backend: backend, Profile: profile, Image: image, Client: client}, nil
}

func (service Service) ResolveWorkspaceRuntime(ctx context.Context, expected WorkspaceIdentity) (WorkspaceRuntime, error) {
	current, err := service.WorkspaceRuntimeByID(ctx, expected.ProfileID)
	if err != nil {
		return WorkspaceRuntime{}, ErrWorkspaceRuntimeChanged
	}
	if current.Identity() != expected {
		return WorkspaceRuntime{}, ErrWorkspaceRuntimeChanged
	}
	return current, nil
}
