package sandbox

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
)

func TestWorkspaceIdentityCannotAliasDifferentRuntimeOrRevision(t *testing.T) {
	base := WorkspaceIdentity{ProfileID: uuid.New(), ProfileRevision: 1, BackendID: uuid.New(), BackendVersion: 1, ImageID: uuid.New(), ImageVersion: 1, ImageDigest: "sha256:original"}
	for _, change := range []func(*WorkspaceIdentity){
		func(i *WorkspaceIdentity) { i.ProfileID = uuid.New() }, func(i *WorkspaceIdentity) { i.ProfileRevision++ },
		func(i *WorkspaceIdentity) { i.BackendID = uuid.New() }, func(i *WorkspaceIdentity) { i.BackendVersion++ },
		func(i *WorkspaceIdentity) { i.ImageID = uuid.New() }, func(i *WorkspaceIdentity) { i.ImageVersion++ }, func(i *WorkspaceIdentity) { i.ImageDigest = "sha256:changed" },
	} {
		next := base
		change(&next)
		if next.Version() == base.Version() || next.Label() == base.Label() {
			t.Fatal("runtime identity or revision aliased the frozen snapshot")
		}
	}
	if len(validation.IsValidLabelValue(base.Label())) != 0 {
		t.Fatal("identity label is not a valid Kubernetes label")
	}
	if strings.Contains(base.Version(), "endpoint") || strings.Contains(base.Version(), "credential") {
		t.Fatal("private configuration leaked into snapshot")
	}
}
