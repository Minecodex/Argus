package kuberneteshelm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"text/template"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartv2util "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/engine"
	release "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlreader "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

const markerAnnotation = "argus.io/helm-operation"

func renderChart(ctx context.Context, cfg *rest.Config, r Request, m Material) (frozen, error) {
	var f frozen
	d, e := discovery.NewDiscoveryClientForConfig(cfg)
	if e != nil {
		return f, ErrUnavailable
	}
	version, e := d.ServerVersion()
	if e != nil {
		return f, nativeError(e)
	}
	versions, e := action.GetVersionSet(d)
	if e != nil {
		return f, ErrUnavailable
	}
	caps := common.DefaultCapabilities.Copy()
	caps.KubeVersion = common.KubeVersion{Version: version.GitVersion, Major: version.Major, Minor: version.Minor}
	caps.APIVersions = versions

	f, e = renderIsolated(ctx, r, m, caps)
	if e != nil {
		return f, e
	}
	f.SchemaVersion, f.Request, f.Marker = SchemaVersion, r, keyedDigest(m.Key, []byte("helm-operation/"+r.OperationID))[:48]
	return f, nil
}
func renderLocal(r Request, m Material, caps *common.Capabilities) (frozen, error) {
	var f frozen
	ch, e := loadChart(m.Chart)
	if e != nil {
		return f, e
	}
	values, e := parseValues(m.Values)
	if e != nil {
		return f, e
	}
	if e = chartv2util.ProcessDependencies(ch, values); e != nil {
		return f, ErrInvalid
	}
	for _, dep := range ch.Metadata.Dependencies {
		found := false
		for _, loaded := range ch.Dependencies() {
			if loaded.Name() == dep.Name || loaded.Name() == dep.Alias {
				found = true
			}
		}
		if !found {
			return f, ErrInvalid
		}
	}
	options := common.ReleaseOptions{Name: r.Name, Namespace: r.Namespace.Name, Revision: r.ExpectedRevision + 1, IsInstall: r.Operation == "install", IsUpgrade: r.Operation == "upgrade"}
	prepared, e := chartutil.ToRenderValuesWithSchemaValidation(ch, values, options, caps, true)
	if e != nil {
		return f, ErrInvalid
	}
	if e = validateOfflineSchemas(ch, prepared["Values"]); e != nil {
		return f, e
	}
	renderer := engine.Engine{CustomTemplateFuncs: template.FuncMap{"lookup": func(...any) (map[string]any, error) { return nil, ErrDenied }, "getHostByName": func(...any) (string, error) { return "", ErrDenied }}}
	files, e := renderer.Render(ch, prepared)
	if e != nil {
		return f, ErrInvalid
	}
	for name := range files {
		if strings.HasSuffix(name, "NOTES.txt") {
			delete(files, name)
		}
	}
	hooks, manifests, e := releaseutil.SortManifests(files, nil, releaseutil.InstallOrder)
	if e != nil {
		return f, ErrInvalid
	}
	for _, part := range manifests {
		f.Manifest += "---\n" + part.Content + "\n"
	}
	for _, h := range hooks {
		events := []string{}
		for _, event := range h.Events {
			events = append(events, string(event))
		}
		f.Hooks = append(f.Hooks, hook{Name: h.Name, Manifest: h.Manifest, Events: events})
	}
	for _, crd := range ch.CRDObjects() {
		f.CRDs = append(f.CRDs, string(crd.File.Data))
	}
	f.ChartName, f.ChartVersion = ch.Metadata.Name, ch.Metadata.Version
	if len(f.Manifest) > MaxManifestBytes {
		return f, ErrBudget
	}
	return f, nil
}
func releaseFrozen(r Request, m Material, current *release.Release) frozen {
	f := frozen{SchemaVersion: SchemaVersion, Request: r, Marker: keyedDigest(m.Key, []byte("helm-operation/"+r.OperationID))[:48], Manifest: current.Manifest}
	if current.Chart != nil && current.Chart.Metadata != nil {
		f.ChartName, f.ChartVersion = current.Chart.Metadata.Name, current.Chart.Metadata.Version
	}
	for _, h := range current.Hooks {
		events := []string{}
		for _, event := range h.Events {
			events = append(events, string(event))
		}
		f.Hooks = append(f.Hooks, hook{Name: h.Name, Manifest: h.Manifest, Events: events})
	}
	return f
}
func parseObjects(raw string) ([]*unstructured.Unstructured, error) {
	if len(raw) > MaxManifestBytes {
		return nil, ErrBudget
	}
	out := []*unstructured.Unstructured{}
	decoder := yamlreader.NewYAMLReader(bufio.NewReader(strings.NewReader(raw)))
	for i := 0; i < MaxObjects*3; i++ {
		var value map[string]any
		document, e := decoder.Read()
		if e == io.EOF {
			return out, nil
		}
		if e != nil {
			return nil, ErrInvalid
		}
		encoded, e := yaml.YAMLToJSONStrict(document)
		if e != nil || json.Unmarshal(encoded, &value) != nil {
			return nil, ErrInvalid
		}
		if len(value) == 0 {
			continue
		}
		if len(out) >= MaxObjects {
			return nil, ErrBudget
		}
		out = append(out, &unstructured.Unstructured{Object: value})
	}
	return nil, ErrBudget
}
func manifest(objects []*unstructured.Unstructured) (string, error) {
	var b bytes.Buffer
	for _, o := range objects {
		raw, e := json.Marshal(o.Object)
		if e != nil {
			return "", ErrInvalid
		}
		b.WriteString("---\n")
		b.Write(raw)
		b.WriteByte('\n')
	}
	if b.Len() > MaxManifestBytes {
		return "", ErrBudget
	}
	return b.String(), nil
}
func freezeObject(o *unstructured.Unstructured, r Request, marker string) {
	a := o.GetAnnotations()
	if a == nil {
		a = map[string]string{}
	}
	a[markerAnnotation], a["meta.helm.sh/release-name"], a["meta.helm.sh/release-namespace"] = marker, r.Name, r.Namespace.Name
	o.SetAnnotations(a)
	l := o.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	l["app.kubernetes.io/managed-by"] = "Helm"
	o.SetLabels(l)
}
func activeHook(operation string, events []string) bool {
	prefix := map[string]string{"install": "install", "upgrade": "upgrade", "rollback": "rollback", "uninstall": "delete"}[operation]
	return slices.Contains(events, "pre-"+prefix) || slices.Contains(events, "post-"+prefix)
}
func staticChart(f frozen) *chart.Chart {
	var b strings.Builder
	b.WriteString(f.Manifest)
	for _, h := range f.Hooks {
		b.WriteString("\n---\n")
		b.WriteString(h.Manifest)
	}
	ch := &chart.Chart{Metadata: &chart.Metadata{APIVersion: "v2", Name: f.ChartName, Version: f.ChartVersion, Type: "application"}, Values: map[string]any{}, Templates: []*common.File{{Name: "templates/argus.yaml", Data: []byte(`{{ .Files.Get "argus/frozen.yaml" }}`)}}, Files: []*common.File{{Name: "argus/frozen.yaml", Data: []byte(b.String())}}}
	for i, raw := range f.CRDs {
		ch.Files = append(ch.Files, &common.File{Name: "crds/argus-" + string(rune('a'+i)) + ".yaml", Data: []byte(raw)})
	}
	return ch
}
