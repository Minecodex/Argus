package presentation

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestRelatedResourceAccessRequiresPermissionAndTypedGrant(t *testing.T) {
	id := uuid.New()
	host := toolruntime.ResourceRef{Type: "host", ID: id.String()}
	cluster := toolruntime.ResourceRef{Type: "kubernetes_cluster", ID: id.String()}
	access := Access{Permissions: []string{"connector.read", "host.read", "kubernetes.read"}, ResourceIDs: map[string][]uuid.UUID{"host": {id}}}
	if !access.AllowsResource(host) || access.AllowsResource(cluster) {
		t.Fatal("resource grant crossed its type")
	}
	access.Permissions = []string{"connector.read"}
	if access.AllowsResource(host) {
		t.Fatal("connector permission exposed an associated resource name")
	}
	if (Access{}).AllowsResource(host) || access.AllowsResource(toolruntime.ResourceRef{Type: "host", ID: "bad"}) {
		t.Fatal("missing authorization or invalid resource accepted")
	}
}
