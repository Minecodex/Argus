package kuberneteshelm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kubernetesaccess"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"helm.sh/helm/v4/pkg/action"
	ri "helm.sh/helm/v4/pkg/release"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func validateRequest(r Request) error {
	if ValidateChartSource(r.Source) != nil || r.Source != nil && r.Operation != "install" && r.Operation != "upgrade" {
		return ErrInvalid
	}
	if r.SchemaVersion != SchemaVersion || !kubernetesaccess.ValidServer(r.APIServer) || r.Namespace.Name == "" || r.Namespace.UID == "" || len(validation.IsDNS1123Label(r.Name)) > 0 || len(r.Name) > 53 || r.ExpectedRevision < 0 || r.RollbackRevision < 0 || r.TimeoutSeconds < 10 || r.TimeoutSeconds > 240 {
		return ErrInvalid
	}
	if !slices.Contains([]string{"install", "upgrade", "rollback", "uninstall", "list", "history", "status", "observe"}, r.Operation) {
		return ErrInvalid
	}
	if r.Operation != "list" && r.Operation != "history" && r.Operation != "status" && r.Operation != "observe" {
		if id, e := uuid.Parse(r.OperationID); e != nil || id == uuid.Nil {
			return ErrInvalid
		}
	}
	return nil
}

// ErrorCode never exposes raw SDK errors, rendered values or API response bodies.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return nativeError(err).Error()
}
func ErrorFromCode(code string) error {
	if code == "" {
		return nil
	}
	for _, err := range []error{ErrInvalid, ErrConflict, ErrUnavailable, ErrDenied, ErrBudget, ErrUnknown, ErrSourceAuth, ErrSourceIntegrity, ErrSourceUnavailable, kubernetesinventory.ErrCRDHasInstances} {
		if err.Error() == code {
			return err
		}
	}
	return ErrUnavailable
}
func nativeError(e error) error {
	if e == nil {
		return nil
	}
	if apierrors.IsForbidden(e) || apierrors.IsUnauthorized(e) {
		return ErrDenied
	}
	if apierrors.IsConflict(e) || apierrors.IsAlreadyExists(e) {
		return ErrConflict
	}
	for _, known := range []error{ErrInvalid, ErrConflict, ErrDenied, ErrBudget, ErrUnknown, ErrSourceAuth, ErrSourceIntegrity, ErrSourceUnavailable, kubernetesinventory.ErrCRDHasInstances} {
		if errors.Is(e, known) {
			return known
		}
	}
	return ErrUnavailable
}
func (e Engine) config(ctx context.Context, r Request, wrapper func(http.RoundTripper) http.RoundTripper) (*rest.Config, error) {
	if validateRequest(r) != nil || e.Config == nil || e.Config.Host != r.APIServer || e.Config.Insecure {
		return nil, ErrInvalid
	}
	cfg := rest.CopyConfig(e.Config)
	cfg.Timeout = 0
	cfg.ContentType, cfg.AcceptContentTypes = "application/json", "application/json"
	canonical, err := url.Parse(r.APIServer)
	if err != nil {
		return nil, ErrInvalid
	}
	cfg.Wrap(func(base http.RoundTripper) http.RoundTripper {
		if wrapper != nil {
			base = wrapper(base)
		}
		return transportFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.Scheme != canonical.Scheme || request.URL.Host != canonical.Host {
				return nil, ErrDenied
			}
			response, err := base.RoundTrip(request.Clone(ctx))
			if err == nil && response.StatusCode >= 300 && response.StatusCode < 400 {
				response.Body.Close()
				return nil, ErrDenied
			}
			if err == nil && request.Method == http.MethodGet && response.Body != nil {
				if path, ok := writePath(request.URL.Path); ok && path.Group == "" && path.Resource == "secrets" && request.URL.Query().Get("watch") != "true" {
					raw, e := limitedRead(response.Body, 8<<20)
					response.Body.Close()
					if e != nil {
						return nil, e
					}
					if e = validateStorageResponse(raw); e != nil {
						clear(raw)
						return nil, e
					}
					response.Body = io.NopCloser(bytes.NewReader(raw))
				}
				// Watches are streaming GETs. Bound their reads without buffering
				// the whole body, which would prevent Helm from observing readiness.
				response.Body = &budgetBody{ReadCloser: response.Body, remaining: 8 << 20}
			}
			return response, err
		})
	})
	return cfg, nil
}

type transportFunc func(*http.Request) (*http.Response, error)

type budgetBody struct {
	io.ReadCloser
	remaining int64
}

func (b *budgetBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining < 0 {
		return 0, ErrBudget
	}
	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}
	n, e := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, ErrBudget
	}
	return n, e
}

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func verifyNamespace(ctx context.Context, api kubernetes.Interface, ns Namespace) error {
	v, e := api.CoreV1().Namespaces().Get(ctx, ns.Name, metav1.GetOptions{})
	if e != nil {
		return nativeError(e)
	}
	if string(v.UID) != ns.UID || v.DeletionTimestamp != nil {
		return ErrConflict
	}
	return nil
}
func releaseV1(v ri.Releaser) (*release.Release, error) {
	switch value := v.(type) {
	case *release.Release:
		return value, nil
	case release.Release:
		return &value, nil
	default:
		return nil, ErrInvalid
	}
}
func releaseDTO(v *release.Release) Release {
	r := Release{Name: v.Name, Namespace: v.Namespace, Revision: v.Version}
	if v.Info != nil {
		r.Status = string(v.Info.Status)
		r.UpdatedAt = v.Info.LastDeployed
	}
	if v.Chart != nil && v.Chart.Metadata != nil {
		r.Chart = v.Chart.Metadata.Name
		r.ChartVersion = v.Chart.Metadata.Version
	}
	return r
}
func readCurrent(cfg *action.Configuration, name string) (*release.Release, error) {
	v, e := cfg.Releases.Last(name)
	if errors.Is(e, driver.ErrReleaseNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, nativeError(e)
	}
	return releaseV1(v)
}
func storageName(name string, revision int) string {
	return "sh.helm.release.v1." + name + ".v" + strconv.Itoa(revision)
}
func getStoragePin(ctx context.Context, api kubernetes.Interface, namespace, name string) (storagePin, error) {
	v, e := api.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return storagePin{Name: name}, nil
	}
	if e != nil {
		return storagePin{}, nativeError(e)
	}
	if v.Type != corev1.SecretType("helm.sh/release.v1") || v.Labels["owner"] != "helm" {
		return storagePin{}, ErrConflict
	}
	return storagePin{Name: name, UID: string(v.UID), Version: v.ResourceVersion}, nil
}
func (e Engine) Releases(ctx context.Context, r Request) ([]Release, error) {
	cfg, e1 := e.config(ctx, r, func(base http.RoundTripper) http.RoundTripper {
		return transportFunc(func(q *http.Request) (*http.Response, error) {
			if q.Method != http.MethodGet {
				return nil, ErrDenied
			}
			return base.RoundTrip(q)
		})
	})
	if e1 != nil {
		return nil, e1
	}
	api, e1 := kubernetes.NewForConfig(cfg)
	if e1 != nil {
		return nil, ErrUnavailable
	}
	if e1 = verifyNamespace(ctx, api, r.Namespace); e1 != nil {
		return nil, e1
	}
	configuration, e1 := newConfiguration(cfg, r.Namespace.Name)
	if e1 != nil {
		return nil, e1
	}
	var rows []ri.Releaser
	if r.Operation == "list" {
		rows, e1 = configuration.Releases.List(func(ri.Releaser) bool { return true })
	} else {
		rows, e1 = configuration.Releases.History(r.Name)
	}
	if errors.Is(e1, driver.ErrReleaseNotFound) {
		return []Release{}, nil
	}
	if e1 != nil {
		return nil, nativeError(e1)
	}
	if len(rows) > 200 {
		return nil, ErrBudget
	}
	out := make([]Release, 0, len(rows))
	for _, row := range rows {
		v, e := releaseV1(row)
		if e != nil {
			return nil, e
		}
		out = append(out, releaseDTO(v))
	}
	slices.SortFunc(out, func(a, b Release) int {
		if a.Name != b.Name {
			return strings.Compare(a.Name, b.Name)
		}
		return b.Revision - a.Revision
	})
	if r.Operation == "list" {
		latest := make([]Release, 0, len(out))
		for _, v := range out {
			if len(latest) == 0 || latest[len(latest)-1].Name != v.Name {
				latest = append(latest, v)
			}
		}
		out = latest
	}
	return out, ValidateReleases(out, r)
}
func dynamicAPI(config *rest.Config) (dynamic.Interface, error) {
	d, e := dynamic.NewForConfig(config)
	if e != nil {
		return nil, ErrUnavailable
	}
	return d, nil
}

// Keeping the scanner bounded also prevents YAML streams with excessive empty
// documents from consuming an unbounded parser loop.
func limitedRead(r io.Reader, max int) ([]byte, error) {
	v, e := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if e != nil {
		return nil, ErrInvalid
	}
	if len(v) > max {
		return nil, ErrBudget
	}
	return v, nil
}
