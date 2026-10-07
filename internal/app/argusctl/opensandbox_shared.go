package argusctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// CompatibleOpenSandboxOwner is a read-only ownership probe shared with the E2E
// preflight. Installation repeats the checks before using the dependency.
func CompatibleOpenSandboxOwner(ctx context.Context, contextName, root string, pins ...*SharedSandboxController) (string, error) {
	clients, err := clientsFor(contextName)
	if err != nil {
		return "", err
	}
	ch, err := (helmManager{cacheDir: filepath.Join(root, "deploy", ".cache", "charts"), log: io.Discard}).loadOpenSandboxControllerChart(ctx)
	if err != nil {
		return "", err
	}
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "argus-dependency-probe"
	cfg.Spec.Namespaces.Sandbox = "argus-dependency-probe"
	if len(pins) > 1 {
		return "", fmt.Errorf("one shared controller identity is required")
	}
	if len(pins) == 1 {
		cfg.Spec.OpenSandbox.SharedController = pins[0]
		if err := pins[0].validate(); err != nil {
			return "", err
		}
	}
	shared, err := sharedOpenSandboxController(ctx, cfg, clients, ch)
	if err != nil || shared == "" {
		return "", err
	}
	namespace, name, _ := strings.Cut(shared, "/")
	deployment, err := clients.typed.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return namespace + "/" + deployment.Labels["app.kubernetes.io/instance"], nil
}

// OpenSandbox v0.2.0 watches the entire cluster and uses fixed ClusterRole names.
// A compatible existing installation is a shared dependency, never something
// this release should adopt, relabel, upgrade or delete.
func sharedOpenSandboxController(ctx context.Context, cfg *InstallConfig, clients *kubeClients, ch *chart.Chart) (string, error) {
	expected, err := openSandboxCRDs(ch, openSandboxControllerValues(cfg))
	if err != nil {
		return "", err
	}
	if len(expected) != 3 {
		return "", fmt.Errorf("unexpected OpenSandbox CRD inventory")
	}
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	found, foreign := 0, 0
	ownerNS, ownerRelease := "", ""
	for name, wanted := range expected {
		live, e := clients.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		found++
		annotations := live.GetAnnotations()
		ns, release := annotations["meta.helm.sh/release-namespace"], annotations["meta.helm.sh/release-name"]
		if ns == cfg.Spec.Namespaces.Sandbox && release == cfg.upstreamReleaseName("os") {
			continue
		}
		foreign++
		if ns == "" || release == "" || live.GetDeletionTimestamp() != nil {
			return "", fmt.Errorf("OpenSandbox CRD %s has no reusable active owner", name)
		}
		if ownerNS != "" && (ownerNS != ns || ownerRelease != release) {
			return "", fmt.Errorf("OpenSandbox CRDs have conflicting owners")
		}
		ownerNS, ownerRelease = ns, release
		if err = compatibleOpenSandboxCRD(wanted, live); err != nil {
			return "", fmt.Errorf("shared OpenSandbox CRD %s is incompatible: %w", name, err)
		}
	}
	if foreign == 0 {
		if cfg.Spec.OpenSandbox.SharedController != nil {
			return "", fmt.Errorf("configured shared OpenSandbox controller was not found")
		}
		return "", nil
	}
	if foreign != len(expected) || found != len(expected) {
		return "", fmt.Errorf("partial/mixed OpenSandbox installation cannot be reused")
	}
	deployment, err := clients.typed.AppsV1().Deployments(ownerNS).Get(ctx, "opensandbox-controller-manager", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("shared OpenSandbox controller unavailable: %w", err)
	}
	if deployment.DeletionTimestamp != nil || deployment.Labels["app.kubernetes.io/instance"] != ownerRelease || deployment.Status.AvailableReplicas < 1 || deployment.Status.ObservedGeneration < deployment.Generation {
		return "", fmt.Errorf("shared OpenSandbox controller owner/version/readiness mismatch")
	}
	if pin := cfg.Spec.OpenSandbox.SharedController; pin != nil {
		if err := verifySharedSandboxController(ctx, clients, deployment, pin); err != nil {
			return "", err
		}
		return ownerNS + "/" + deployment.Name, nil
	}
	if deployment.Labels["app.kubernetes.io/version"] != "0.2.0" {
		return "", fmt.Errorf("shared OpenSandbox controller version does not match v0.2.0")
	}
	compatibleImage := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		image, _, _ := strings.Cut(container.Image, "@")
		if image == "opensandbox/controller:v0.2.0" || strings.HasSuffix(image, "/opensandbox/controller:v0.2.0") {
			compatibleImage = true
		}
	}
	if !compatibleImage {
		return "", fmt.Errorf("shared OpenSandbox controller image does not match v0.2.0")
	}
	return ownerNS + "/" + deployment.Name, nil
}

func openSandboxCRDs(ch *chart.Chart, values map[string]any) (map[string]*unstructured.Unstructured, error) {
	renderValues, err := chartutil.ToRenderValues(ch, values, common.ReleaseOptions{Name: "compatibility-check", Namespace: "compatibility-check", IsInstall: true}, common.DefaultCapabilities)
	if err != nil {
		return nil, err
	}
	files, err := engine.Render(ch, renderValues)
	if err != nil {
		return nil, err
	}
	result := map[string]*unstructured.Unstructured{}
	for name, manifest := range files {
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
		for {
			var value map[string]any
			err = decoder.Decode(&value)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			object := &unstructured.Unstructured{Object: value}
			if object.GetKind() == "CustomResourceDefinition" {
				result[object.GetName()] = object
			}
		}
	}
	return result, nil
}

func compatibleOpenSandboxCRD(wanted, live *unstructured.Unstructured) error {
	for _, field := range []string{"group", "scope", "names"} {
		a, _, _ := unstructured.NestedFieldNoCopy(wanted.Object, "spec", field)
		b, _, _ := unstructured.NestedFieldNoCopy(live.Object, "spec", field)
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("%s differs", field)
		}
	}
	strategy, _, _ := unstructured.NestedString(live.Object, "spec", "conversion", "strategy")
	if strategy != "" && strategy != "None" {
		return fmt.Errorf("conversion webhook is not supported")
	}
	versions, _, _ := unstructured.NestedSlice(wanted.Object, "spec", "versions")
	actual, _, _ := unstructured.NestedSlice(live.Object, "spec", "versions")
	for _, raw := range versions {
		version := raw.(map[string]any)
		match := false
		for _, other := range actual {
			candidate := other.(map[string]any)
			if candidate["name"] != version["name"] {
				continue
			}
			match = true
			for _, field := range []string{"served", "storage", "schema", "subresources"} {
				if field == "schema" {
					a, _ := version[field].(map[string]any)
					b, _ := candidate[field].(map[string]any)
					if !compatibleSandboxSchema(a["openAPIV3Schema"], b["openAPIV3Schema"]) {
						return fmt.Errorf("version %v schema differs", version["name"])
					}
					continue
				}
				a, _ := json.Marshal(version[field])
				b, _ := json.Marshal(candidate[field])
				if string(a) != string(b) {
					return fmt.Errorf("version %v %s differs", version["name"], field)
				}
			}
		}
		if !match {
			return fmt.Errorf("required version %v missing", version["name"])
		}
	}
	conditions, _, _ := unstructured.NestedSlice(live.Object, "status", "conditions")
	for _, raw := range conditions {
		condition, _ := raw.(map[string]any)
		if condition["type"] == "Established" && condition["status"] == "True" {
			return nil
		}
	}
	return fmt.Errorf("CRD is not established")
}

// Accept additive optional fields and documentation-only changes, while retaining
// identical types, defaults, constraints and required fields for the API we use.
func compatibleSandboxSchema(wanted, actual any) bool {
	a, ok := wanted.(map[string]any)
	if !ok {
		return sandboxJSONEqual(wanted, actual)
	}
	b, ok := actual.(map[string]any)
	if !ok {
		return false
	}
	doc := func(key string) bool {
		return key == "description" || key == "title" || key == "externalDocs" || key == "example"
	}
	for key, value := range a {
		if doc(key) {
			continue
		}
		other, found := b[key]
		if !found {
			return false
		}
		switch key {
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				return false
			}
			current, ok := other.(map[string]any)
			if !ok {
				return false
			}
			for name, property := range properties {
				if !compatibleSandboxSchema(property, current[name]) {
					return false
				}
			}
			for name, property := range current {
				if _, known := properties[name]; known {
					continue
				}
				node, _ := property.(map[string]any)
				if _, hasDefault := node["default"]; hasDefault {
					return false
				}
			}
		case "items", "not", "additionalProperties":
			if !compatibleSandboxSchema(value, other) {
				return false
			}
		case "allOf", "anyOf", "oneOf":
			left, ok := value.([]any)
			if !ok {
				return false
			}
			right, ok := other.([]any)
			if !ok || len(left) != len(right) {
				return false
			}
			for i := range left {
				if !compatibleSandboxSchema(left[i], right[i]) {
					return false
				}
			}
		default:
			if !sandboxJSONEqual(value, other) {
				return false
			}
		}
	}
	for key := range b {
		if _, found := a[key]; !found && !doc(key) {
			return false
		}
	}
	return true
}

func sandboxJSONEqual(a, b any) bool {
	left, e1 := json.Marshal(a)
	right, e2 := json.Marshal(b)
	return e1 == nil && e2 == nil && string(left) == string(right)
}
