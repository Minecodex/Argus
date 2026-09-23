package argusctl

import (
	"context"
	"fmt"
	storagev1 "k8s.io/api/storage/v1"
	"path"
	"slices"
	"strings"
	"time"

	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const rawfileChartURL = "https://github.com/openebs/rawfile-localpv/releases/download/v0.15.1/rawfile-localpv-0.15.1.tgz"
const rawfileChartHash = "3107fbe473f64de9ae84aae3677d395daa19d22049dd12ade0e5cda037b90169"

type WorkspaceInstall struct {
	Enabled           bool   `json:"enabled"`
	StorageNamespace  string `json:"storageNamespace"`
	StorageClass      string `json:"storageClass"`
	DataDirectory     string `json:"dataDirectory"`
	MetadataDirectory string `json:"metadataDirectory"`
	DefaultBytes      int64  `json:"defaultBytes"`
	EnterpriseBytes   int64  `json:"enterpriseBytes"`
	MaxFileBytes      int64  `json:"maxFileBytes"`
}

func (value *WorkspaceInstall) validate() error {
	if !value.Enabled {
		return nil
	}
	if value.StorageNamespace == "" {
		value.StorageNamespace = "argus-workspace-storage"
	}
	if value.StorageClass == "" {
		value.StorageClass = "argus-workspace"
	}
	if value.DataDirectory == "" {
		value.DataDirectory = "/var/local/argus-workspaces/data"
	}
	if value.MetadataDirectory == "" {
		value.MetadataDirectory = "/var/local/argus-workspaces/meta"
	}
	if value.DefaultBytes == 0 {
		value.DefaultBytes = 2 << 30
	}
	if value.EnterpriseBytes == 0 {
		value.EnterpriseBytes = 20 << 30
	}
	if value.MaxFileBytes == 0 {
		value.MaxFileBytes = 100 << 20
	}
	if !dnsLabel.MatchString(value.StorageNamespace) || !dnsLabel.MatchString(value.StorageClass) || value.DefaultBytes <= 0 || value.EnterpriseBytes < value.DefaultBytes || value.MaxFileBytes <= 0 || value.MaxFileBytes > value.DefaultBytes {
		return fmt.Errorf("invalid Workspace namespace, StorageClass or capacity")
	}
	for _, directory := range []string{value.DataDirectory, value.MetadataDirectory} {
		if !strings.HasPrefix(directory, "/") || path.Clean(directory) != directory || directory == "/" {
			return fmt.Errorf("Workspace storage paths must be explicit absolute Linux directories")
		}
	}
	if value.DataDirectory == value.MetadataDirectory {
		return fmt.Errorf("Workspace data and metadata directories must differ")
	}
	return nil
}

func (a *App) installWorkspaceStorage(ctx context.Context, cfg *InstallConfig, clients *kubeClients, helm helmManager) error {
	if !cfg.Spec.Workspace.Enabled {
		return nil
	}
	settings := cfg.Spec.Workspace
	class, err := clients.typed.StorageV1().StorageClasses().Get(ctx, settings.StorageClass, metav1.GetOptions{})
	classExists := err == nil
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if classExists && (class.Provisioner != "rawfile.csi.openebs.io" || class.Parameters["thinProvision"] != "false" || class.Parameters["csi.storage.k8s.io/fstype"] != "ext4" || class.AllowVolumeExpansion != nil && *class.AllowVolumeExpansion || class.VolumeBindingMode == nil || *class.VolumeBindingMode != storagev1.VolumeBindingWaitForFirstConsumer || !slices.Contains(class.MountOptions, "nodiscard")) {
		return fmt.Errorf("existing Workspace StorageClass does not enforce the approved hard quota")
	}
	driver, driverErr := clients.typed.StorageV1().CSIDrivers().Get(ctx, "rawfile.csi.openebs.io", metav1.GetOptions{})
	if driverErr == nil {
		if err := validateExistingRawfileDriver(ctx, clients.typed, driver); err != nil {
			return err
		}
		if classExists {
			return nil
		}
		return createWorkspaceStorageClass(ctx, cfg, clients)
	}
	if !apierrors.IsNotFound(driverErr) {
		return driverErr
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: settings.StorageNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": cfg.Spec.ReleaseID, "argus.io/plane": "workspace-storage"}}}
	if _, err := clients.typed.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	archive, err := downloadFile(ctx, helm.cacheDir, "rawfile-localpv-0.15.1.tgz", rawfileChartURL)
	if err != nil {
		return err
	}
	digest, err := fileDigest(archive)
	if err != nil {
		return err
	}
	if digest != rawfileChartHash {
		return fmt.Errorf("RawFile v0.15.1 chart checksum mismatch")
	}
	chart, err := loader.Load(archive)
	if err != nil {
		return err
	}
	if err := helm.installOrUpgrade(ctx, cfg.upstreamReleaseName("workspace-storage"), settings.StorageNamespace, chart, workspaceStorageValues(settings)); err != nil {
		return err
	}
	if classExists {
		return nil
	}
	return createWorkspaceStorageClass(ctx, cfg, clients)
}

func workspaceStorageValues(settings WorkspaceInstall) map[string]any {
	resources := map[string]any{"requests": map[string]any{"cpu": "25m", "memory": "64Mi"}, "limits": map[string]any{"cpu": "500m", "memory": "256Mi"}}
	return map[string]any{
		"analytics": map[string]any{"enabled": false}, "global": map[string]any{"analytics": map[string]any{"enabled": false}},
		"auth": map[string]any{"enabled": true}, "image": map[string]any{"tag": "v0.15.1"},
		"node": map[string]any{"metadataDirPath": settings.MetadataDirectory, "defaultPool": "default", "storagePools": map[string]any{"default": map[string]any{"path": settings.DataDirectory}}, "resources": resources,
			"tolerations":         []any{map[string]any{"key": "node-role.kubernetes.io/control-plane", "operator": "Exists", "effect": "NoSchedule"}},
			"externalProvisioner": map[string]any{"resources": resources}, "driverRegistrar": map[string]any{"resources": resources}},
		"capabilities": map[string]any{"snapshots": map[string]any{"enabled": false}, "resize": map[string]any{"enabled": false}, "apiServer": map[string]any{"enabled": false}},
		"crds":         map[string]any{"enabled": false}, "snapshotClasses": []any{}, "metrics": map[string]any{"enabled": true},
		"storageClasses": []any{},
	}
}

func removeWorkspaceStorage(ctx context.Context, cfg *InstallConfig, clients *kubeClients, helm helmManager) error {
	if !cfg.Spec.Workspace.Enabled {
		return nil
	}
	namespace, err := clients.typed.CoreV1().Namespaces().Get(ctx, cfg.Spec.Workspace.StorageNamespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if namespace.Labels["argus.io/release-id"] != cfg.Spec.ReleaseID {
		return nil
	}
	for {
		volumes, err := clients.typed.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		pending := false
		for _, volume := range volumes.Items {
			if volume.Spec.CSI == nil || volume.Spec.CSI.Driver != "rawfile.csi.openebs.io" {
				continue
			}
			if volume.Spec.ClaimRef == nil || volume.Spec.ClaimRef.Namespace != cfg.Spec.Namespaces.Sandbox {
				return fmt.Errorf("RawFile serves another namespace; retaining shared Workspace storage")
			}
			pending = true
		}
		if !pending {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	class, classErr := clients.typed.StorageV1().StorageClasses().Get(ctx, cfg.Spec.Workspace.StorageClass, metav1.GetOptions{})
	if classErr == nil && class.Labels["argus.io/release-id"] == cfg.Spec.ReleaseID {
		if err := clients.typed.StorageV1().StorageClasses().Delete(ctx, class.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &class.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	if err := helm.uninstall(cfg.upstreamReleaseName("workspace-storage"), cfg.Spec.Workspace.StorageNamespace); err != nil {
		return err
	}
	err = clients.typed.CoreV1().Namespaces().Delete(ctx, namespace.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID}})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func createWorkspaceStorageClass(ctx context.Context, cfg *InstallConfig, clients *kubeClients) error {
	no := false
	binding := storagev1.VolumeBindingWaitForFirstConsumer
	reclaim := corev1.PersistentVolumeReclaimDelete
	_, err := clients.typed.StorageV1().StorageClasses().Create(ctx, &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: cfg.Spec.Workspace.StorageClass, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/release-id": cfg.Spec.ReleaseID}}, Provisioner: "rawfile.csi.openebs.io", AllowVolumeExpansion: &no, VolumeBindingMode: &binding, ReclaimPolicy: &reclaim, MountOptions: []string{"nodiscard"}, Parameters: map[string]string{"csi.storage.k8s.io/fstype": "ext4", "thinProvision": "false", "formatOptions": "-E nodiscard", "copyOnWrite": "false", "storagePool": "default"}}, metav1.CreateOptions{})
	return err
}
