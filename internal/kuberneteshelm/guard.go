package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

type livePin struct {
	UID, Version string
	Generation   int64
}
type writeGuard struct {
	ctx       context.Context
	request   Request
	plan      frozen
	key       []byte
	api       kubernetes.Interface
	dynamic   dynamic.Interface
	authorize func(context.Context, Request, Write) error
	mu        sync.Mutex
	written   map[string]livePin
	writes    int
	outcomes  []WriteOutcome
}

func writePath(path string) (Write, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	var w Write
	if len(parts) >= 2 && parts[0] == "api" {
		w.Version = parts[1]
		parts = parts[2:]
	} else if len(parts) >= 3 && parts[0] == "apis" {
		w.Group, w.Version = parts[1], parts[2]
		parts = parts[3:]
	} else {
		return w, false
	}
	if len(parts) >= 3 && parts[0] == "namespaces" {
		w.Namespace = parts[1]
		parts = parts[2:]
	}
	if len(parts) < 1 || len(parts) > 2 {
		return w, false
	}
	w.Resource = parts[0]
	if len(parts) == 2 {
		w.Name = parts[1]
	}
	return w, true
}
func pinPath(gvr schema.GroupVersionResource, namespace, name string) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource + "/" + namespace + "/" + name
}
func writeKey(w Write) string {
	return pinPath(schema.GroupVersionResource{Group: w.Group, Version: w.Version, Resource: w.Resource}, w.Namespace, w.Name)
}
func (g *writeGuard) find(w Write) (pin, bool) {
	key := writeKey(w)
	for _, p := range g.plan.Pins {
		if pinPath(p.GVR, p.Object.Namespace, p.Object.Name) == key {
			return p, true
		}
	}
	return pin{}, false
}
func (g *writeGuard) storage(w Write) (storagePin, bool) {
	if w.Group != "" || w.Version != "v1" || w.Resource != "secrets" || w.Namespace != g.request.Namespace.Name {
		return storagePin{}, false
	}
	for _, p := range g.plan.Storage {
		if p.Name == w.Name {
			return p, true
		}
	}
	return storagePin{}, false
}
func (g *writeGuard) readCurrent(ctx context.Context, w Write) (*unstructured.Unstructured, error) {
	var c dynamic.ResourceInterface = g.dynamic.Resource(schema.GroupVersionResource{Group: w.Group, Version: w.Version, Resource: w.Resource})
	if w.Namespace != "" {
		c = g.dynamic.Resource(schema.GroupVersionResource{Group: w.Group, Version: w.Version, Resource: w.Resource}).Namespace(w.Namespace)
	}
	o, e := c.Get(ctx, w.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(e) {
		return nil, nil
	}
	if e != nil {
		return nil, nativeError(e)
	}
	return o, nil
}
func (g *writeGuard) verifyPin(ctx context.Context, w Write, p pin) (*unstructured.Unstructured, error) {
	if strings.HasPrefix(p.Object.Definition.Key, "custom/") {
		d, e := (kubernetesinventory.Client{Dynamic: g.dynamic}).Definition(ctx, p.Object.Definition.Key)
		if e != nil || d != p.Object.Definition {
			return nil, ErrConflict
		}
	}
	if strings.HasPrefix(p.Object.Definition.Key, "pending-crd/") {
		parts := strings.Split(p.Object.Definition.Key, "/")
		if len(parts) != 3 {
			return nil, ErrInvalid
		}
		g.mu.Lock()
		created := g.written[pinPath(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}, "", parts[1])]
		g.mu.Unlock()
		if created.UID == "" {
			return nil, ErrConflict
		}
		crd, e := g.dynamic.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}).Get(ctx, parts[1], metav1.GetOptions{})
		if e != nil || string(crd.GetUID()) != created.UID || crd.GetGeneration() != created.Generation {
			return nil, ErrConflict
		}
	}
	if p.Object.NamespaceUID != "" {
		if e := verifyNamespace(ctx, g.api, Namespace{Name: p.Object.Namespace, UID: p.Object.NamespaceUID}); e != nil {
			return nil, e
		}
	}
	if p.Object.Namespace != "" && p.Object.NamespaceUID == "" {
		g.mu.Lock()
		owned := g.written[pinPath(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, "", p.Object.Namespace)]
		g.mu.Unlock()
		if owned.UID == "" {
			return nil, ErrConflict
		}
		if e := verifyNamespace(ctx, g.api, Namespace{Name: p.Object.Namespace, UID: owned.UID}); e != nil {
			return nil, e
		}
	}
	o, e := g.readCurrent(ctx, w)
	if e != nil {
		return nil, e
	}
	g.mu.Lock()
	runtime, own := g.written[writeKey(w)]
	g.mu.Unlock()
	if own {
		if runtime.UID == "" {
			if o != nil {
				return nil, ErrConflict
			}
			return nil, nil
		}
		if o == nil || string(o.GetUID()) != runtime.UID {
			return nil, ErrConflict
		}
		if p.Object.Hook != "" {
			if o.GetGeneration() != runtime.Generation {
				return nil, ErrConflict
			}
		} else if o.GetResourceVersion() != runtime.Version {
			return nil, ErrConflict
		}
		return o, nil
	}
	if p.Object.UID == "" {
		if o != nil {
			return nil, ErrConflict
		}
		return nil, nil
	}
	if o == nil || string(o.GetUID()) != p.Object.UID || o.GetResourceVersion() != p.Object.Version {
		return nil, ErrConflict
	}
	return o, nil
}
func (g *writeGuard) verifyStorage(ctx context.Context, w Write, p storagePin) (*unstructured.Unstructured, error) {
	o, e := g.readCurrent(ctx, w)
	if e != nil {
		return nil, e
	}
	g.mu.Lock()
	runtime, own := g.written[writeKey(w)]
	g.mu.Unlock()
	if own {
		if o == nil || string(o.GetUID()) != runtime.UID || o.GetResourceVersion() != runtime.Version {
			return nil, ErrConflict
		}
		return o, nil
	}
	if p.UID == "" {
		if o != nil {
			return nil, ErrConflict
		}
		return nil, nil
	}
	if o == nil || string(o.GetUID()) != p.UID || o.GetResourceVersion() != p.Version {
		return nil, ErrConflict
	}
	return o, nil
}
func (g *writeGuard) initial(ctx context.Context) error {
	if e := verifyNamespace(ctx, g.api, g.request.Namespace); e != nil {
		return e
	}
	for _, p := range g.plan.Pins {
		if strings.HasPrefix(p.Object.Validation, "deferred_") {
			continue
		}
		w := Write{Group: p.GVR.Group, Version: p.GVR.Version, Resource: p.GVR.Resource, Namespace: p.Object.Namespace, Name: p.Object.Name}
		if _, e := g.verifyPin(ctx, w, p); e != nil {
			return e
		}
	}
	for _, p := range g.plan.Storage {
		w := Write{Version: "v1", Resource: "secrets", Namespace: g.request.Namespace.Name, Name: p.Name}
		if _, e := g.verifyStorage(ctx, w, p); e != nil {
			return e
		}
	}
	return nil
}
func (g *writeGuard) wrap(base http.RoundTripper) http.RoundTripper {
	return transportFunc(func(request *http.Request) (response *http.Response, err error) {
		if request.Method == http.MethodGet || request.Method == http.MethodHead {
			return base.RoundTrip(request)
		}
		w, ok := writePath(request.URL.Path)
		if !ok {
			return nil, ErrDenied
		}
		w.Verb = request.Method
		stage := "decode"
		defer func() {
			state, code := "applied", ""
			if err != nil {
				state, code = "rejected", ErrorCode(err)
			} else if response != nil && response.StatusCode >= 300 {
				state, code = "rejected", ErrUnavailable.Error()
				if request.Method == http.MethodDelete && response.StatusCode == 404 {
					state, code = "absent", ""
				}
			}
			g.mu.Lock()
			if len(g.outcomes) < 1000 {
				g.outcomes = append(g.outcomes, WriteOutcome{Write: w, Stage: stage, State: state, ErrorCode: code})
			}
			g.mu.Unlock()
		}()
		if request.Method != http.MethodDelete {
			u := *request.URL
			q := u.Query()
			q.Set("fieldManager", "helm")
			q.Set("fieldValidation", "Strict")
			u.RawQuery = q.Encode()
			request = request.Clone(g.ctx)
			request.URL = &u
		}
		var raw []byte
		var e error
		if request.Body != nil {
			raw, e = limitedRead(request.Body, 2<<20)
		}
		if e != nil {
			return nil, e
		}
		defer clear(raw)
		if request.Method == http.MethodPatch && strings.HasPrefix(request.Header.Get("Content-Type"), "application/json-patch+json") {
			stage = "managed_fields"
			return g.managedFields(base, request, w, raw)
		}
		value := map[string]any{}
		if len(raw) > 0 {
			encoded, e := yaml.YAMLToJSONStrict(raw)
			if e != nil || json.Unmarshal(encoded, &value) != nil {
				return nil, ErrDenied
			}
		}
		if request.Method == http.MethodPost && w.Name == "" {
			metadata, _ := value["metadata"].(map[string]any)
			w.Name, _ = metadata["name"].(string)
		}
		if w.Name == "" {
			return nil, ErrDenied
		}
		p, object := g.find(w)
		stage = "plan"
		store, control := g.storage(w)
		if !object && !control {
			return nil, ErrDenied
		}
		if object && p.Object.Action == "retain" {
			return nil, ErrDenied
		}
		if object && request.Method == http.MethodDelete && p.Object.Action != "delete" && p.Object.Action != "hook" && p.Object.Action != "absent" {
			return nil, ErrDenied
		}
		if object && request.Method != http.MethodDelete && p.Object.Action != "apply" && p.Object.Action != "hook" {
			return nil, ErrDenied
		}
		if control && (request.Method == http.MethodDelete || value["type"] != "helm.sh/release.v1") {
			return nil, ErrDenied
		}
		if e = verifyNamespace(g.ctx, g.api, g.request.Namespace); e != nil {
			return nil, e
		}
		var current *unstructured.Unstructured
		stage = "identity"
		if object {
			current, e = g.verifyPin(g.ctx, w, p)
		} else {
			current, e = g.verifyStorage(g.ctx, w, store)
		}
		if e != nil {
			return nil, e
		}
		if request.Method == http.MethodDelete {
			if current == nil {
				return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)), Request: request}, nil
			}
			if object {
				stage = "dependencies"
				if e = (kubernetesinventory.Client{Dynamic: g.dynamic}).CheckDeletionDependencies(g.ctx, p.Object.Definition, current); e != nil {
					return nil, e
				}
			}
			value["preconditions"] = map[string]any{"uid": string(current.GetUID()), "resourceVersion": current.GetResourceVersion()}
		} else {
			metadata, _ := value["metadata"].(map[string]any)
			if metadata == nil {
				metadata = map[string]any{}
				value["metadata"] = metadata
			}
			if current != nil {
				metadata["uid"], metadata["resourceVersion"] = string(current.GetUID()), current.GetResourceVersion()
			} else {
				delete(metadata, "uid")
				delete(metadata, "resourceVersion")
			}
		}
		body, e := json.Marshal(value)
		if e != nil {
			return nil, ErrInvalid
		}
		defer clear(body)
		if object && request.Method != http.MethodDelete {
			stage = "admission"
			// Admission runs on exactly the body and field-manager used by Helm.
			probe := request.Clone(g.ctx)
			u := *request.URL
			query := u.Query()
			query.Set("dryRun", "All")
			u.RawQuery = query.Encode()
			probe.URL = &u
			probe.Body = io.NopCloser(bytes.NewReader(body))
			probe.ContentLength = int64(len(body))
			response, e := base.RoundTrip(probe)
			if e != nil {
				return nil, nativeError(e)
			}
			data, e := limitedRead(response.Body, 2<<20)
			response.Body.Close()
			if e != nil {
				return nil, e
			}
			defer clear(data)
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return nil, ErrConflict
			}
			var after unstructured.Unstructured
			if after.UnmarshalJSON(data) != nil {
				return nil, ErrInvalid
			}
			if e = kubernetesinventory.ValidateMutationResult(p.Object.Definition, kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Data: data}}, g.request.Access.ClusterWide, g.request.Access.Policy); e != nil {
				return nil, ErrDenied
			}
			if p.Digest != "" && resourceDigest(g.key, &after, p) != p.Digest {
				return nil, ErrConflict
			}
			if p.Digest == "" && p.Desired != nil {
				if e = validateDeferredBody(p, &unstructured.Unstructured{Object: value}); e != nil {
					return nil, e
				}
			}
		}
		if g.authorize == nil {
			return nil, ErrDenied
		}
		stage = "authorization"
		if e = g.authorize(g.ctx, g.request, w); e != nil {
			return nil, ErrDenied
		}
		stage = "write"
		write := request.Clone(g.ctx)
		write.Body = io.NopCloser(bytes.NewReader(body))
		write.ContentLength = int64(len(body))
		response, e = base.RoundTrip(write)
		if e != nil {
			return nil, ErrUnknown
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			data, e := limitedRead(response.Body, 2<<20)
			response.Body.Close()
			if e != nil {
				return nil, ErrUnknown
			}
			var result unstructured.Unstructured
			if request.Method != http.MethodDelete && result.UnmarshalJSON(data) == nil {
				g.mu.Lock()
				g.written[writeKey(w)] = livePin{UID: string(result.GetUID()), Version: result.GetResourceVersion(), Generation: result.GetGeneration()}
				g.writes++
				g.mu.Unlock()
			} else if request.Method == http.MethodDelete {
				g.mu.Lock()
				g.written[writeKey(w)] = livePin{}
				g.writes++
				g.mu.Unlock()
			}
			response.Body = io.NopCloser(bytes.NewReader(data))
			response.ContentLength = int64(len(data))
		}
		return response, nil
	})
}
func validateDeferredBody(p pin, actual *unstructured.Unstructured) error {
	desired := p.Desired.DeepCopy()
	a := actual.DeepCopy()
	for _, o := range []*unstructured.Unstructured{desired, a} {
		o.SetUID("")
		o.SetResourceVersion("")
	}
	// The immutable manifest is the baseline for resources whose Namespace or
	// CRD becomes available only during this approved operation.
	if !slices.Equal(mustJSON(desired.Object), mustJSON(a.Object)) {
		return ErrConflict
	}
	return nil
}
