package kubernetesworkbench

import (
	"context"
	"errors"

	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func mutationPermissions(ctx context.Context, q *db.Queries, a Actor, clusterWide bool) (kubernetesinventory.MutationPolicy, error) {
	p := kubernetesinventory.MutationPolicy{}
	for _, item := range []struct {
		permission string
		value      *bool
		cluster    bool
	}{
		{"kubernetes.workload.privileged", &p.PrivilegedWorkloads, true},
		{"kubernetes.rbac.escalate", &p.RBACDelegation, true},
		{"kubernetes.service_account.use", &p.ServiceAccountUse, false},
	} {
		if item.cluster && !clusterWide {
			continue
		}
		err := authorization.Authorize(ctx, q, a, item.permission)
		if err != nil && !errors.Is(err, authorization.ErrAccessDenied) {
			return p, err
		}
		*item.value = err == nil
	}
	return p, nil
}

func checkMutationPermissions(ctx context.Context, q *db.Queries, a Actor, clusterWide bool, p kubernetesinventory.MutationPolicy) error {
	if !clusterWide && (p.PrivilegedWorkloads || p.RBACDelegation) {
		return authorization.ErrAccessDenied
	}
	for _, item := range []struct {
		permission string
		enabled    bool
	}{
		{"kubernetes.workload.privileged", p.PrivilegedWorkloads},
		{"kubernetes.rbac.escalate", p.RBACDelegation},
		{"kubernetes.service_account.use", p.ServiceAccountUse},
	} {
		if item.enabled {
			if err := authorization.Authorize(ctx, q, a, item.permission); err != nil {
				return err
			}
		}
	}
	return nil
}
