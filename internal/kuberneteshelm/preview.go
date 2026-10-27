package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	common "helm.sh/helm/v4/pkg/release/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func (e Engine) Preview(ctx context.Context, r Request, m Material) (Result, error) {
	out := Result{SchemaVersion: SchemaVersion, DryRun: true, Objects: []Object{}}
	if len(m.Key) != 32 || !slices.Contains([]string{"install", "upgrade", "rollback", "uninstall"}, r.Operation) {
		return out, ErrInvalid
	}
	cfg, err := e.config(ctx, r, func(base http.RoundTripper) http.RoundTripper {
		return transportFunc(func(q *http.Request) (*http.Response, error) {
			if q.Method == http.MethodDelete && q.Body != nil {
				raw, e := limitedRead(q.Body, 1<<20)
				if e != nil {
					return nil, e
				}
				var options metav1.DeleteOptions
				if json.Unmarshal(raw, &options) != nil || len(options.DryRun) != 1 || options.DryRun[0] != metav1.DryRunAll {
					return nil, ErrDenied
				}
				q = q.Clone(ctx)
				q.Body = io.NopCloser(bytes.NewReader(raw))
				return base.RoundTrip(q)
			}
			if q.Method != http.MethodGet && q.URL.Query().Get("dryRun") != "All" {
				return nil, ErrDenied
			}
			return base.RoundTrip(q)
		})
	})
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
	current, err := readCurrent(configuration, r.Name)
	if err != nil {
		return out, err
	}
	if current == nil && r.ExpectedRevision != 0 || current != nil && current.Version != r.ExpectedRevision {
		return out, ErrConflict
	}
	if r.Operation == "install" && current != nil && current.Info.Status != common.StatusUninstalled {
		return out, ErrConflict
	}
	if r.Operation != "install" && current == nil {
		return out, ErrConflict
	}
	if current != nil && (current.Info == nil || current.Info.Status.IsPending()) && r.Operation != "uninstall" {
		return out, ErrConflict
	}
	var f frozen
	if r.Operation == "install" || r.Operation == "upgrade" {
		if ValidateSourceCredentials(m.SourceCredentials) != nil || r.Source == nil && m.SourceCredentials != nil {
			return out, ErrInvalid
		}
		if r.Source != nil {
			if len(m.Chart) != 0 {
				return out, ErrInvalid
			}
			m.Chart, out.Source, err = e.resolveChart(ctx, *r.Source, m.SourceCredentials)
			if err != nil {
				return out, err
			}
			defer clear(m.Chart)
			out.EncryptedChart, err = sealDownloadedChart(r, m.Key, m.Chart)
			if err != nil {
				return out, err
			}
		}
		m.SourceCredentials = nil
		f, err = renderChart(ctx, cfg, r, m)
		if err == nil {
			if out.Source == nil {
				out.Source = &ChartProvenance{Kind: "upload", Chart: f.ChartName, Version: f.ChartVersion, Digest: archiveDigest(m.Chart), ArchiveDigest: archiveDigest(m.Chart)}
			}
			f.Source, f.MaterialDigest = out.Source, materialDigest(m)
		}
	} else if r.Operation == "rollback" {
		if r.RollbackRevision < 1 || r.RollbackRevision >= r.ExpectedRevision {
			return out, ErrInvalid
		}
		old, e := configuration.Releases.Get(r.Name, r.RollbackRevision)
		if e != nil {
			return out, ErrConflict
		}
		target, e := releaseV1(old)
		if e != nil {
			return out, e
		}
		f = releaseFrozen(r, m, target)
	} else {
		f = releaseFrozen(r, m, current)
	}
	if err != nil {
		return out, err
	}
	// Source resolution and isolated rendering can take time. Do not continue
	// the object reads/dry-runs against a replacement Namespace with the same name.
	if err = verifyNamespace(ctx, api, r.Namespace); err != nil {
		return out, err
	}
	dyn, err := dynamicAPI(cfg)
	if err != nil {
		return out, err
	}
	getter := &clientGetter{config: cfg, namespace: r.Namespace.Name}
	mapper, err := getter.ToRESTMapper()
	if err != nil {
		return out, ErrUnavailable
	}
	state := &preflight{request: r, key: m.Key, api: api, dynamic: dyn, mapper: mapper, pending: map[schema.GroupVersionKind]kubernetesinventory.Definition{}, namespaces: map[string]string{r.Namespace.Name: r.Namespace.UID}}
	for _, n := range r.Access.Namespaces {
		state.namespaces[n.Name] = n.UID
	}
	// CRDs in crds/ are install-only and never upgraded or removed by Helm.
	var newCRDs []string
	for _, raw := range f.CRDs {
		objects, e := parseObjects(raw)
		if e != nil {
			return out, e
		}
		for _, o := range objects {
			if o.GetKind() != "CustomResourceDefinition" || o.GetAPIVersion() != "apiextensions.k8s.io/v1" {
				return out, ErrInvalid
			}
			_, e = dyn.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(ctx, o.GetName(), metav1.GetOptions{})
			if e == nil {
				continue
			}
			if !apierrors.IsNotFound(e) {
				return out, nativeError(e)
			}
			if r.Operation != "install" {
				continue
			}
			if e = state.addPendingCRD(o); e != nil {
				return out, e
			}
			freezeObject(o, r, f.Marker)
			p, e := state.object(ctx, o, "apply", "", true)
			if e != nil {
				return out, e
			}
			f.Pins = append(f.Pins, p)
			encoded, e := manifest([]*unstructured.Unstructured{o})
			if e != nil {
				return out, e
			}
			newCRDs = append(newCRDs, encoded)
		}
	}
	f.CRDs = newCRDs
	desired, err := parseObjects(f.Manifest)
	if err != nil {
		return out, err
	}
	for _, o := range desired {
		if o.GetKind() == "Namespace" && o.GetAPIVersion() == "v1" {
			if !r.Access.ClusterWide {
				return out, ErrDenied
			}
			if _, e := api.CoreV1().Namespaces().Get(ctx, o.GetName(), metav1.GetOptions{}); apierrors.IsNotFound(e) {
				state.namespaces[o.GetName()] = ""
			}
		}
	}
	seen := map[string]bool{}
	for _, o := range desired {
		freezeObject(o, r, f.Marker)
		action := "apply"
		if r.Operation == "uninstall" {
			action = "delete"
		}
		p, e := state.object(ctx, o, action, "", false)
		if e != nil {
			return out, e
		}
		if seen[p.Key] {
			return out, ErrInvalid
		}
		seen[p.Key] = true
		f.Pins = append(f.Pins, p)
	}
	f.Manifest, err = manifest(desired)
	if err != nil {
		return out, err
	}
	for index := range f.Hooks {
		h := &f.Hooks[index]
		if !activeHook(r.Operation, h.Events) {
			continue
		}
		objects, e := parseObjects(h.Manifest)
		if e != nil {
			return out, e
		}
		for _, o := range objects {
			freezeObject(o, r, f.Marker)
			p, e := state.object(ctx, o, "hook", strings.Join(h.Events, ","), false)
			if e != nil {
				return out, e
			}
			if seen[p.Key] {
				return out, ErrInvalid
			}
			seen[p.Key] = true
			f.Pins = append(f.Pins, p)
		}
		h.Manifest, e = manifest(objects)
		if e != nil {
			return out, e
		}
	}
	if current != nil && r.Operation != "uninstall" {
		old, e := parseObjects(current.Manifest)
		if e != nil {
			return out, e
		}
		for _, o := range old {
			key := objectKey(o)
			if seen[key] {
				continue
			}
			p, e := state.object(ctx, o, "delete", "", false)
			if e != nil {
				return out, e
			}
			seen[p.Key] = true
			f.Pins = append(f.Pins, p)
		}
	}
	if len(f.Pins) > MaxObjects {
		return out, ErrBudget
	}
	if current != nil {
		p, e := getStoragePin(ctx, api, r.Namespace.Name, storageName(r.Name, current.Version))
		if e != nil {
			return out, e
		}
		f.Storage = append(f.Storage, p)
		out.Release = releaseDTO(current)
	}
	if r.Operation != "uninstall" {
		p, e := getStoragePin(ctx, api, r.Namespace.Name, storageName(r.Name, r.ExpectedRevision+1))
		if e != nil {
			return out, e
		}
		if p.UID != "" {
			return out, ErrConflict
		}
		f.Storage = append(f.Storage, p)
	}
	for _, p := range f.Pins {
		out.Objects = append(out.Objects, publicObject(p.Object))
	}
	out.Release = Release{Name: r.Name, Namespace: r.Namespace.Name, Revision: r.ExpectedRevision + 1, Chart: f.ChartName, ChartVersion: f.ChartVersion}
	if err = verifyNamespace(ctx, api, r.Namespace); err != nil {
		return Result{}, err
	}
	out.SealedPlan, err = sealPlan(r, m, f)
	if raw, e := json.Marshal(out); e != nil || len(raw) > 768<<10 {
		return Result{}, ErrBudget
	}
	return out, err
}

type preflight struct {
	request    Request
	key        []byte
	api        kubernetes.Interface
	dynamic    dynamic.Interface
	mapper     meta.RESTMapper
	pending    map[schema.GroupVersionKind]kubernetesinventory.Definition
	namespaces map[string]string
}

func objectKey(o *unstructured.Unstructured) string {
	return o.GetAPIVersion() + "/" + o.GetKind() + "/" + o.GetNamespace() + "/" + o.GetName()
}
func (p *preflight) definition(ctx context.Context, o *unstructured.Unstructured) (kubernetesinventory.Definition, schema.GroupVersionResource, error) {
	gv, e := schema.ParseGroupVersion(o.GetAPIVersion())
	if e != nil || o.GetKind() == "" {
		return kubernetesinventory.Definition{}, schema.GroupVersionResource{}, ErrInvalid
	}
	gk := gv.WithKind(o.GetKind())
	if d, ok := p.pending[gk]; ok {
		return d, schema.GroupVersionResource{Group: d.Group, Version: d.Version, Resource: d.Resource}, nil
	}
	m, e := p.mapper.RESTMapping(gk.GroupKind(), gv.Version)
	if e != nil {
		return kubernetesinventory.Definition{}, schema.GroupVersionResource{}, ErrInvalid
	}
	for _, d := range kubernetesinventory.Definitions() {
		if d.Group == gv.Group && d.Resource == m.Resource.Resource && d.Kind == o.GetKind() {
			d.Version = gv.Version
			return d, m.Resource, nil
		}
	}
	crd, e := p.dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(ctx, m.Resource.Resource+"."+gv.Group, metav1.GetOptions{})
	if e != nil {
		return kubernetesinventory.Definition{}, m.Resource, nativeError(e)
	}
	key := fmt.Sprintf("custom/%s/%s/%s@%s", gv.Group, gv.Version, m.Resource.Resource, crd.GetUID())
	d, e := (kubernetesinventory.Client{Dynamic: p.dynamic}).Definition(ctx, key)
	if e != nil {
		return d, m.Resource, ErrInvalid
	}
	return d, m.Resource, nil
}
func (p *preflight) addPendingCRD(o *unstructured.Unstructured) error {
	if !p.request.Access.ClusterWide || !slices.Contains(p.request.Access.Permissions, "kubernetes.crd.manage") {
		return ErrDenied
	}
	group, _, _ := unstructured.NestedString(o.Object, "spec", "group")
	plural, _, _ := unstructured.NestedString(o.Object, "spec", "names", "plural")
	kind, _, _ := unstructured.NestedString(o.Object, "spec", "names", "kind")
	scope, _, _ := unstructured.NestedString(o.Object, "spec", "scope")
	versions, _, _ := unstructured.NestedSlice(o.Object, "spec", "versions")
	if group == "" || plural == "" || kind == "" || scope != "Namespaced" && scope != "Cluster" {
		return ErrInvalid
	}
	for _, raw := range versions {
		v, _ := raw.(map[string]any)
		name, _ := v["name"].(string)
		if name != "" && v["served"] == true {
			p.pending[schema.GroupVersionKind{Group: group, Version: name, Kind: kind}] = kubernetesinventory.Definition{Key: "pending-crd/" + o.GetName() + "/" + name, Group: group, Version: name, Resource: plural, Kind: kind, Namespaced: scope == "Namespaced", Category: "extension", ManagePermission: "kubernetes.custom_resource.manage"}
		}
	}
	return nil
}
func (p *preflight) object(ctx context.Context, o *unstructured.Unstructured, action, hook string, crd bool) (pin, error) {
	var out pin
	d, gvr, e := p.definition(ctx, o)
	if e != nil {
		return out, e
	}
	if d.ManagePermission == "" || !slices.Contains(p.request.Access.Permissions, d.ManagePermission) {
		return out, ErrDenied
	}
	if o.GetName() == "" || strings.HasPrefix(o.GetName(), "sh.helm.release.v1.") {
		return out, ErrInvalid
	}
	var client dynamic.ResourceInterface = p.dynamic.Resource(gvr)
	nsuid := ""
	validation := "server"
	if d.Namespaced {
		if o.GetNamespace() == "" {
			o.SetNamespace(p.request.Namespace.Name)
		}
		expected, known := p.namespaces[o.GetNamespace()]
		if !p.request.Access.ClusterWide && !known {
			return out, ErrDenied
		}
		if known && expected == "" {
			validation = "deferred_namespace"
		} else {
			ns, e := p.api.CoreV1().Namespaces().Get(ctx, o.GetNamespace(), metav1.GetOptions{})
			if e != nil {
				return out, nativeError(e)
			}
			if known && expected != string(ns.UID) {
				return out, ErrConflict
			}
			nsuid = string(ns.UID)
		}
		client = p.dynamic.Resource(gvr).Namespace(o.GetNamespace())
	} else if !p.request.Access.ClusterWide || o.GetNamespace() != "" {
		return out, ErrDenied
	}
	if e = kubernetesinventory.ValidateMutationResult(d, kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Data: mustJSON(o.Object)}}, p.request.Access.ClusterWide, p.request.Access.Policy); e != nil {
		return out, ErrDenied
	}
	out = pin{Key: objectKey(o), GVR: gvr, Desired: o.DeepCopy(), Object: Object{Definition: d, Namespace: o.GetNamespace(), NamespaceUID: nsuid, Name: o.GetName(), Action: action, Hook: hook, Keep: o.GetAnnotations()["helm.sh/resource-policy"] == "keep", CRD: crd, Validation: validation}}
	if strings.HasPrefix(d.Key, "pending-crd/") {
		out.Object.Validation = "deferred_crd"
		v, e := project(d, *o, nsuid)
		if e != nil {
			return out, e
		}
		out.Object.After = &v
		return out, nil
	}
	if validation == "deferred_namespace" {
		v, e := project(d, *o, nsuid)
		if e != nil {
			return out, e
		}
		out.Object.After = &v
		return out, nil
	}
	current, e := client.Get(ctx, o.GetName(), metav1.GetOptions{})
	if e != nil && !apierrors.IsNotFound(e) {
		return out, nativeError(e)
	}
	if current != nil {
		out.Object.UID, out.Object.Version = string(current.GetUID()), current.GetResourceVersion()
		if !owned(current, p.request) && !crd {
			return out, ErrConflict
		}
		before, e := project(d, *current, nsuid)
		if e != nil {
			return out, e
		}
		out.Object.Before = &before
	}
	if action == "delete" {
		if current == nil {
			out.Object.Action = "absent"
			return out, nil
		}
		if out.Object.Keep {
			out.Object.Action = "retain"
			return out, nil
		}
		if e = (kubernetesinventory.Client{Dynamic: p.dynamic}).CheckDeletionDependencies(ctx, d, current); e != nil {
			return out, e
		}
		uid := current.GetUID()
		version := current.GetResourceVersion()
		if e = client.Delete(ctx, o.GetName(), metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}, Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}}); e != nil {
			return out, nativeError(e)
		}
		return out, nil
	}
	if current != nil {
		o.SetUID(current.GetUID())
		o.SetResourceVersion(current.GetResourceVersion())
	}
	var after *unstructured.Unstructured
	if current == nil {
		after, e = client.Create(ctx, o, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}, FieldManager: "helm", FieldValidation: "Strict"})
	} else {
		after, e = client.Patch(ctx, o.GetName(), types.ApplyPatchType, mustJSON(o.Object), metav1.PatchOptions{DryRun: []string{metav1.DryRunAll}, FieldManager: "helm", FieldValidation: "Strict"})
	}
	if e != nil {
		return out, nativeError(e)
	}
	if e = kubernetesinventory.ValidateMutationResult(d, kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Data: mustJSON(after.Object)}}, p.request.Access.ClusterWide, p.request.Access.Policy); e != nil {
		return out, ErrDenied
	}
	projected, e := project(d, *after, nsuid)
	if e != nil {
		return out, e
	}
	out.Object.After = &projected
	out.Digest = resourceDigest(p.key, after, out)
	return out, nil
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func owned(o *unstructured.Unstructured, r Request) bool {
	a := o.GetAnnotations()
	return o.GetLabels()["app.kubernetes.io/managed-by"] == "Helm" && a["meta.helm.sh/release-name"] == r.Name && a["meta.helm.sh/release-namespace"] == r.Namespace.Name
}
func resourceDigest(key []byte, o *unstructured.Unstructured, p pin) string {
	return keyedDigest(key, []byte(kubernetesinventory.StableDesiredDigest(o, p.Desired, p.Object.Definition.Key, p.Object.UID == "")))
}
