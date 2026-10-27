package kuberneteshelm

import (
	"context"
	"errors"
	"strings"
	"time"

	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	ri "helm.sh/helm/v4/pkg/release"
	common "helm.sh/helm/v4/pkg/release/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicclient "k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func (e Engine) Execute(ctx context.Context, r Request, m Material) (Result, error) {
	out := Result{SchemaVersion: SchemaVersion, Objects: []Object{}}
	if e.AuthorizeWrite == nil {
		return out, ErrDenied
	}
	f, err := openPlan(r, m)
	if err != nil {
		return out, err
	}
	if (r.Operation == "install" || r.Operation == "upgrade") && f.MaterialDigest != materialDigest(m) {
		return out, ErrConflict
	}
	out.Source = f.Source
	lifetime, cancel := context.WithTimeout(ctx, time.Duration(r.TimeoutSeconds)*time.Second)
	defer cancel()
	readConfig, err := e.config(lifetime, r, nil)
	if err != nil {
		return out, err
	}
	api, err := kubernetes.NewForConfig(readConfig)
	if err != nil {
		return out, ErrUnavailable
	}
	dynamic, err := dynamicAPI(readConfig)
	if err != nil {
		return out, err
	}
	guard := &writeGuard{ctx: lifetime, request: r, plan: f, key: m.Key, api: api, dynamic: dynamic, authorize: e.AuthorizeWrite, written: map[string]livePin{}}
	if err = guard.initial(lifetime); err != nil {
		return out, err
	}
	config, err := e.config(lifetime, r, guard.wrap)
	if err != nil {
		return out, err
	}
	configuration, err := newConfiguration(config, r.Namespace.Name)
	if err != nil {
		return out, err
	}
	var originalChart *chart.Chart
	if r.Operation == "install" || r.Operation == "upgrade" {
		originalChart, err = loadChart(m.Chart)
		if err != nil {
			return out, err
		}
	}
	configuration.Releases.MaxHistory = 0
	configuration.Releases.Driver = &releaseDriver{Driver: configuration.Releases.Driver, plan: f, original: originalChart}
	values, err := parseValues(m.Values)
	if err != nil {
		return out, err
	}
	var actual ri.Releaser
	timeout := time.Duration(r.TimeoutSeconds) * time.Second
	switch r.Operation {
	case "install":
		client := action.NewInstall(configuration)
		client.ReleaseName = r.Name
		client.Namespace = r.Namespace.Name
		client.Timeout = timeout
		client.WaitStrategy = kube.LegacyStrategy
		client.WaitForJobs = true
		client.Replace = r.ExpectedRevision > 0
		client.CreateNamespace = false
		client.RollbackOnFailure = false
		actual, err = client.RunWithContext(lifetime, staticChart(f), values)
	case "upgrade":
		client := action.NewUpgrade(configuration)
		client.Namespace = r.Namespace.Name
		client.Timeout = timeout
		client.WaitStrategy = kube.LegacyStrategy
		client.WaitForJobs = true
		client.ServerSideApply = "true"
		client.ForceConflicts = false
		client.CleanupOnFail = false
		client.MaxHistory = 0
		actual, err = client.RunWithContext(lifetime, r.Name, staticChart(f), values)
	case "rollback":
		client := action.NewRollback(configuration)
		client.Version = r.RollbackRevision
		client.Timeout = timeout
		client.WaitStrategy = kube.LegacyStrategy
		client.WaitForJobs = true
		client.ServerSideApply = "true"
		client.CleanupOnFail = false
		client.MaxHistory = 0
		err = client.Run(r.Name)
	case "uninstall":
		client := action.NewUninstall(configuration)
		client.Timeout = timeout
		client.WaitStrategy = kube.LegacyStrategy
		client.KeepHistory = true
		_, err = client.Run(r.Name)
	default:
		return out, ErrInvalid
	}
	for _, p := range f.Pins {
		out.Objects = append(out.Objects, publicObject(p.Object))
	}
	guard.mu.Lock()
	wrote := guard.writes > 0
	out.Writes = append([]WriteOutcome(nil), guard.outcomes...)
	guard.mu.Unlock()
	if err != nil {
		guardFailure := recordedWriteFailure(out.Writes)
		if errors.Is(err, ErrUnknown) || lifetime.Err() != nil || errors.Is(guardFailure, ErrUnknown) {
			return out, ErrUnknown
		}
		cause := nativeError(err)
		// Helm may aggregate failures as text and lose the original error
		// chain. Prefer our independently recorded, bounded rejection code.
		if errors.Is(cause, ErrUnavailable) && guardFailure != nil {
			cause = guardFailure
		}
		if !wrote {
			return out, cause
		}
		out.Accepted, out.Applied, out.ErrorCode = true, true, cause.Error()
		if current, e := readCurrent(configuration, r.Name); e == nil && current != nil {
			out.Release = releaseDTO(current)
		}
		return out, nil
	}
	if actual == nil {
		actual, err = configuration.Releases.Last(r.Name)
		if err != nil {
			return out, ErrUnknown
		}
	}
	v, err := releaseV1(actual)
	if err != nil {
		return out, err
	}
	out.Release = releaseDTO(v)
	out.Accepted, out.Applied = true, true
	out.Ready = v.Info != nil && (r.Operation == "uninstall" && v.Info.Status == common.StatusUninstalled || r.Operation != "uninstall" && v.Info.Status == common.StatusDeployed)
	return out, nil
}

func recordedWriteFailure(writes []WriteOutcome) error {
	var first error
	for _, write := range writes {
		if write.State != "rejected" || write.ErrorCode == "" {
			continue
		}
		cause := ErrorFromCode(write.ErrorCode)
		if errors.Is(cause, ErrUnknown) {
			return ErrUnknown
		}
		if first == nil && !errors.Is(cause, ErrUnavailable) {
			first = cause
		}
	}
	return first
}
func (e Engine) Observe(ctx context.Context, r Request, m Material) (Result, error) {
	out := Result{SchemaVersion: SchemaVersion, Objects: []Object{}}
	f, err := openPlan(r, m)
	if err != nil {
		return out, err
	}
	cfg, err := e.config(ctx, r, nil)
	if err != nil {
		return out, err
	}
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return out, ErrUnavailable
	}
	if err = verifyNamespace(ctx, api, r.Namespace); err != nil {
		return out, err
	}
	configuration, err := newConfiguration(cfg, r.Namespace.Name)
	if err != nil {
		return out, err
	}
	v, err := readCurrent(configuration, r.Name)
	if err != nil {
		return out, err
	}
	if v == nil {
		return out, nil
	}
	out.Release = releaseDTO(v)
	out.Applied = v.Labels[markerAnnotation] == f.Marker
	if !out.Applied {
		return out, nil
	}
	out.Accepted = true
	if v.Info.Status == common.StatusFailed {
		out.ErrorCode = "KUBERNETES_HELM_RELEASE_FAILED"
		return out, nil
	}
	if r.Operation == "uninstall" {
		out.Ready = v.Info.Status == common.StatusUninstalled
		return out, nil
	}
	if v.Info.Status != common.StatusDeployed {
		return out, nil
	}
	dynamic, err := dynamicAPI(cfg)
	if err != nil {
		return out, err
	}
	for _, p := range f.Pins {
		if p.Object.Action != "apply" || p.Object.CRD {
			continue
		}
		var c dynamicclient.ResourceInterface = dynamic.Resource(p.GVR)
		if p.Object.Namespace != "" {
			c = dynamic.Resource(p.GVR).Namespace(p.Object.Namespace)
		}
		current, err := c.Get(ctx, p.Object.Name, metav1.GetOptions{})
		if err != nil {
			return out, nativeError(err)
		}
		if current.GetAnnotations()[markerAnnotation] != f.Marker {
			return out, ErrConflict
		}
	}
	resources, err := configuration.KubeClient.Build(strings.NewReader(f.Manifest), false)
	if err != nil {
		return out, ErrUnavailable
	}
	waiter, err := configuration.KubeClient.GetWaiter(kube.LegacyStrategy)
	if err != nil {
		return out, ErrUnavailable
	}
	if err = waiter.WaitWithJobs(resources, time.Second); err == nil {
		out.Ready = true
	}
	return out, nil
}
