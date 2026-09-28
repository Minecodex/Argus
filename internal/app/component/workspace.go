package component

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/sandbox"
	"github.com/kakj-go/Argus/internal/storage/objectstore"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/tlsmaterial"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func Workspace(ctx context.Context, cfg config.Server, store *postgres.Store, idempotency postgres.Idempotency, runtime sandbox.Service) (workspace.Service, error) {
	settings := config.LoadWorkspace()
	service := workspace.Service{Store: store, Idempotency: idempotency, Config: settings, Sandbox: runtime, Kubernetes: workspace.Kubernetes{Config: settings}}
	objects, err := objectstore.Open(ctx, cfg.ObjectStoreURL, settings.FilesBucket, cfg.ObjectStoreAccess, cfg.ObjectStoreSecret)
	if err != nil {
		return service, err
	}
	service.Objects = objects
	queries := dashboard.QueryJobs{Runtime: dashboard.Runtime{Store: store}}
	service.ExternalSource = func(ctx context.Context, p toolruntime.Principal, ref string) (bool, error) {
		handled, err := queries.AuthorizeSource(ctx, p, ref)
		if handled {
			err = dashboardWorkspaceSourceError(err)
		}
		return handled, err
	}
	if !settings.Enabled {
		return service, nil
	}
	if settings.DefaultBytes > settings.EnterpriseBytes || settings.MaxFileBytes > settings.DefaultBytes || settings.IOImage == "" || settings.EgressImage == "" || settings.ClientCert == "" || settings.ClientKey == "" || settings.CAPath == "" {
		return service, errors.New("invalid Workspace platform configuration")
	}
	var kubeConfig *rest.Config
	if cfg.KubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
	} else {
		kubeConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return service, err
	}
	service.Kubernetes.Client, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return service, err
	}
	return service, nil
}

// Translate the source domain at the composition boundary. Workspace does not
// depend on Dashboard, and a denied/archived source is not an internal IO error.
func dashboardWorkspaceSourceError(err error) error {
	if errors.Is(err, dashboard.ErrDenied) || errors.Is(err, dashboard.ErrArchived) || errors.Is(err, dashboard.ErrNotFound) {
		return toolruntime.Error{Kind: "WORKSPACE_FILE_FORBIDDEN"}
	}
	return err
}

// StartWorkspaceAdmission shares argus-server ownership but uses an isolated
// TLS listener, so tenant HTTP routes cannot reach the admission protocol.
func StartWorkspaceAdmission(ctx context.Context, service workspace.Service) (<-chan error, error) {
	result := make(chan error, 1)
	if !service.Config.Enabled {
		return result, nil
	}
	material, err := tlsmaterial.Load(tlsmaterial.Options{CertificatePath: service.Config.AdmissionCert, PrivateKeyPath: service.Config.AdmissionKey, CABundlePath: service.Config.CAPath, Usage: x509.ExtKeyUsageServerAuth})
	if err != nil {
		return nil, err
	}
	tlsConfig, err := material.ServerConfig(tls.NoClientCert, nil)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", ":8448")
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: http.HandlerFunc(service.Admission), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		TLSConfig: tlsConfig}
	go material.Watch(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	go func() {
		err := server.Serve(tls.NewListener(listener, server.TLSConfig))
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		result <- err
	}()
	return result, nil
}
