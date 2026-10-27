package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
)

type StreamInput struct {
	NamespaceID uuid.UUID `json:"namespace_id"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	UID         string    `json:"uid"`
	Container   string    `json:"container,omitempty"`
	Mode        string    `json:"mode"`
	Shell       string    `json:"shell,omitempty"`
	Port        int32     `json:"port,omitempty"`
	Reason      string    `json:"reason"`
}
type StreamRoute struct {
	Mode              string    `json:"mode"`
	ConnectorID       uuid.UUID `json:"connector_id"`
	Epoch             int64     `json:"epoch"`
	CredentialID      uuid.UUID `json:"credential_id"`
	CredentialVersion int64     `json:"credential_version"`
	SecretVersionID   uuid.UUID `json:"secret_version_id"`
}
type StreamBinding struct {
	Target kubernetesstream.Target `json:"target"`
	Route  StreamRoute             `json:"route"`
	Reason string                  `json:"reason"`
}
type StreamConnection struct {
	Row     db.KubernetesStreamSession
	Binding StreamBinding
	Cluster db.KubernetesCluster
}

func streamPermission(mode string) string {
	if mode == "exec" {
		return "kubernetes.exec"
	}
	if mode == "port_forward" {
		return "kubernetes.port_forward"
	}
	return ""
}

func currentStreamRoute(ctx context.Context, q *db.Queries, cluster db.KubernetesCluster) (StreamRoute, error) {
	return currentClusterRoute(ctx, q, cluster, kubernetesstream.SchemaVersion)
}
func currentClusterRoute(ctx context.Context, q *db.Queries, cluster db.KubernetesCluster, capability string) (StreamRoute, error) {
	result := StreamRoute{Mode: cluster.ConnectionMode}
	if cluster.CredentialID.Valid {
		credential, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: cluster.EnterpriseID, ID: cluster.CredentialID.UUID})
		if err != nil || credential.Status != "active" || credential.Protocol != "kubernetes" {
			return result, kubernetesinventory.ErrUnavailable
		}
		secret, err := q.GetSecret(ctx, db.GetSecretParams{EnterpriseID: cluster.EnterpriseID, ID: credential.SecretID})
		if err != nil || secret.Status != "active" {
			return result, kubernetesinventory.ErrUnavailable
		}
		version, err := q.GetCurrentSecretVersion(ctx, db.GetCurrentSecretVersionParams{EnterpriseID: cluster.EnterpriseID, SecretID: credential.SecretID})
		if err != nil {
			return result, err
		}
		result.CredentialID, result.CredentialVersion, result.SecretVersionID = credential.ID, credential.Version, version.ID
	}
	if cluster.ConnectionMode == "direct" {
		if result.CredentialID == uuid.Nil {
			return result, kubernetesinventory.ErrUnavailable
		}
		return result, nil
	}
	connectorID := cluster.ConnectorID
	if cluster.ConnectionMode == "via_bastion" {
		if !cluster.BastionScopeID.Valid {
			return result, kubernetesinventory.ErrUnavailable
		}
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{EnterpriseID: cluster.EnterpriseID, ID: cluster.BastionScopeID.UUID})
		if err != nil || !scope.ActiveConnectorID.Valid {
			return result, kubernetesinventory.ErrUnavailable
		}
		connectorID = scope.ActiveConnectorID
	}
	if !connectorID.Valid {
		return result, kubernetesinventory.ErrUnavailable
	}
	connector, err := q.GetConnector(ctx, db.GetConnectorParams{EnterpriseID: cluster.EnterpriseID, ID: connectorID.UUID})
	if err != nil || connector.Status != "online" || connector.ConnectionEpoch < 1 {
		return result, kubernetesinventory.ErrUnavailable
	}
	session, err := q.GetConnectorSession(ctx, db.GetConnectorSessionParams{ConnectorID: connector.ID, ConnectionEpoch: connector.ConnectionEpoch})
	if err != nil || !slices.Contains(session.Capabilities, capability) {
		return result, kubernetesinventory.ErrUnavailable
	}
	if cluster.ConnectionMode == "in_cluster" && (connector.Role != "kubernetes" || !connector.KubernetesClusterID.Valid || connector.KubernetesClusterID.UUID != cluster.ID) {
		return result, kubernetesinventory.ErrUnavailable
	}
	if cluster.ConnectionMode == "via_bastion" && (connector.Role != "bastion" || !connector.BastionScopeID.Valid || connector.BastionScopeID != cluster.BastionScopeID) {
		return result, kubernetesinventory.ErrUnavailable
	}
	result.ConnectorID, result.Epoch = connector.ID, connector.ConnectionEpoch
	return result, nil
}

func (s Service) resolveStream(ctx context.Context, a Actor, clusterID uuid.UUID, in StreamInput) (StreamBinding, db.KubernetesCluster, error) {
	var binding StreamBinding
	if in.NamespaceID == uuid.Nil || in.Name == "" || in.UID == "" || strings.TrimSpace(in.Reason) == "" || len([]rune(in.Reason)) > 500 || streamPermission(in.Mode) == "" || in.Type != "pod" && in.Type != "service" || in.Type == "service" && in.Mode != "port_forward" {
		return binding, db.KubernetesCluster{}, kubernetesinventory.ErrInvalid
	}
	if in.Mode == "exec" && (in.Port != 0 || !slices.Contains([]string{"sh", "bash", "powershell", "cmd"}, in.Shell)) || in.Mode == "port_forward" && (in.Shell != "" || in.Port < 1 || in.Port > 65535) {
		return binding, db.KubernetesCluster{}, kubernetesinventory.ErrInvalid
	}
	cluster, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return binding, cluster, err
	}
	if err = authorization.Authorize(ctx, s.Store.Queries, a, streamPermission(in.Mode)); err != nil {
		return binding, cluster, err
	}
	source, err := s.Get(ctx, a, clusterID, ObjectInput{NamespaceID: in.NamespaceID, Type: in.Type, Name: in.Name, UID: in.UID})
	if err != nil {
		return binding, cluster, err
	}
	target := kubernetesstream.Target{SchemaVersion: kubernetesstream.SchemaVersion, APIServer: cluster.ApiServer, Mode: in.Mode, Shell: in.Shell, Port: in.Port, ClusterWide: scope.ClusterWide}
	if in.Type == "service" {
		var service corev1.Service
		if json.Unmarshal(source.Data, &service) != nil {
			return binding, cluster, kubernetesinventory.ErrProtocol
		}
		page, err := s.List(ctx, a, clusterID, ListInput{Type: "endpointslice", Namespaces: Selection{Mode: "explicit", IDs: []uuid.UUID{in.NamespaceID}}, Labels: "kubernetes.io/service-name=" + source.Name, Limit: 200})
		if err != nil {
			return binding, cluster, err
		}
		if !page.Complete {
			return binding, cluster, kubernetesinventory.ErrBudget
		}
		items := []discoveryv1.EndpointSlice{}
		for _, item := range page.Items {
			var value discoveryv1.EndpointSlice
			if json.Unmarshal(item.Data, &value) != nil {
				return binding, cluster, kubernetesinventory.ErrProtocol
			}
			items = append(items, value)
		}
		endpoints, err := kubernetesstream.ResolveEndpoints(source.Namespace, &service, in.Port, items)
		if err != nil {
			return binding, cluster, err
		}
		keys := []string{}
		for key := range endpoints {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			return binding, cluster, kubernetesinventory.ErrUnavailable
		}
		parts := strings.SplitN(keys[0], "/", 2)
		source, err = s.Get(ctx, a, clusterID, ObjectInput{NamespaceID: in.NamespaceID, Type: "pod", Name: parts[0], UID: parts[1]})
		if err != nil {
			return binding, cluster, err
		}
		target.Port = endpoints[keys[0]]
		target.Service = &kubernetesstream.ServicePin{Name: service.Name, UID: string(service.UID), Port: in.Port}
	}
	var pod corev1.Pod
	if json.Unmarshal(source.Data, &pod) != nil {
		return binding, cluster, kubernetesinventory.ErrProtocol
	}
	container := in.Container
	if container == "" && len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}
	target.Pin, err = kubernetespodlogs.PinPod(source.NamespaceUID, &pod, container, false)
	if err != nil {
		return binding, cluster, err
	}
	target.Pin.PreviousContainerID = ""
	// The previous-container field is irrelevant to exec/forward identity. Native
	// verification normalizes it in the same way before comparing the target.
	target.SecurityFingerprint = kubernetesstream.SecurityFingerprint(&pod)
	requiredPriv := kubernetesstream.ValidatePodPolicy(&pod, true, kubernetesinventory.MutationPolicy{ServiceAccountUse: true}) != nil
	requiredSA := in.Mode == "exec" && kubernetesstream.ValidatePodPolicy(&pod, true, kubernetesinventory.MutationPolicy{PrivilegedWorkloads: true}) != nil
	if requiredPriv {
		if !scope.ClusterWide {
			return binding, cluster, authorization.ErrAccessDenied
		}
		if err = authorization.Authorize(ctx, s.Store.Queries, a, "kubernetes.workload.privileged"); err != nil {
			return binding, cluster, err
		}
	}
	if requiredSA {
		if err = authorization.Authorize(ctx, s.Store.Queries, a, "kubernetes.service_account.use"); err != nil {
			return binding, cluster, err
		}
	}
	target.Policy = kubernetesinventory.MutationPolicy{PrivilegedWorkloads: requiredPriv, ServiceAccountUse: requiredSA || in.Mode == "port_forward"}
	if err = kubernetesstream.ValidatePodPolicy(&pod, scope.ClusterWide, target.Policy); err != nil {
		return binding, cluster, err
	}
	if err = target.Validate(); err != nil {
		return binding, cluster, err
	}
	route, err := currentStreamRoute(ctx, s.Store.Queries, cluster)
	if err != nil {
		return binding, cluster, err
	}
	return StreamBinding{Target: target, Route: route, Reason: in.Reason}, cluster, nil
}
