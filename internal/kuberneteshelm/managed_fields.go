package kuberneteshelm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Helm migrates its own client-side field manager before applying an object.
// Only its two metadata replacements are accepted; arbitrary JSON Patch is not
// a second path around the frozen object/admission checks.
func (g *writeGuard) managedFields(base http.RoundTripper, request *http.Request, w Write, raw []byte) (*http.Response, error) {
	p, ok := g.find(w)
	if !ok || (p.Object.Action != "apply" && p.Object.Action != "hook") {
		return nil, ErrDenied
	}
	if e := verifyNamespace(g.ctx, g.api, g.request.Namespace); e != nil {
		return nil, e
	}
	current, e := g.verifyPin(g.ctx, w, p)
	if e != nil {
		return nil, e
	}
	if current == nil {
		return nil, ErrConflict
	}
	var patch []map[string]any
	if json.Unmarshal(raw, &patch) != nil || len(patch) != 2 {
		return nil, ErrDenied
	}
	seen := map[string]bool{}
	for _, op := range patch {
		path, _ := op["path"].(string)
		if op["op"] != "replace" || len(op) != 3 || seen[path] {
			return nil, ErrDenied
		}
		seen[path] = true
		switch path {
		case "/metadata/managedFields":
			if _, ok := op["value"].([]any); !ok {
				return nil, ErrDenied
			}
		case "/metadata/resourceVersion":
			if op["value"] != current.GetResourceVersion() {
				return nil, ErrConflict
			}
		default:
			return nil, ErrDenied
		}
	}
	if !seen["/metadata/managedFields"] || !seen["/metadata/resourceVersion"] {
		return nil, ErrDenied
	}
	patch = append([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": string(current.GetUID())}}, patch...)
	body, _ := json.Marshal(patch)
	defer clear(body)
	probe := request.Clone(g.ctx)
	u := *request.URL
	q := u.Query()
	q.Set("dryRun", "All")
	u.RawQuery = q.Encode()
	probe.URL = &u
	probe.Body = io.NopCloser(bytes.NewReader(body))
	probe.ContentLength = int64(len(body))
	response, e := base.RoundTrip(probe)
	if e != nil {
		return nil, nativeError(e)
	}
	data, e := limitedRead(response.Body, 2<<20)
	response.Body.Close()
	defer clear(data)
	if e != nil {
		return nil, e
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrConflict
	}
	var admitted unstructured.Unstructured
	if admitted.UnmarshalJSON(data) != nil || !sameManagedFieldsObject(current, &admitted) {
		return nil, ErrDenied
	}
	if g.authorize == nil || g.authorize(g.ctx, g.request, w) != nil {
		return nil, ErrDenied
	}
	write := request.Clone(g.ctx)
	write.Body = io.NopCloser(bytes.NewReader(body))
	write.ContentLength = int64(len(body))
	response, e = base.RoundTrip(write)
	if e != nil {
		return nil, ErrUnknown
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		actual, e := limitedRead(response.Body, 2<<20)
		response.Body.Close()
		if e != nil {
			return nil, ErrUnknown
		}
		var object unstructured.Unstructured
		if object.UnmarshalJSON(actual) != nil {
			return nil, ErrUnknown
		}
		g.mu.Lock()
		g.written[writeKey(w)] = livePin{UID: string(object.GetUID()), Version: object.GetResourceVersion(), Generation: object.GetGeneration()}
		g.writes++
		g.mu.Unlock()
		response.Body = io.NopCloser(bytes.NewReader(actual))
		response.ContentLength = int64(len(actual))
	}
	return response, nil
}
func sameManagedFieldsObject(a, b *unstructured.Unstructured) bool {
	left, right := a.DeepCopy(), b.DeepCopy()
	for _, o := range []*unstructured.Unstructured{left, right} {
		o.SetManagedFields(nil)
		o.SetResourceVersion("")
	}
	x, _ := json.Marshal(left.Object)
	y, _ := json.Marshal(right.Object)
	defer clear(x)
	defer clear(y)
	return bytes.Equal(x, y)
}
