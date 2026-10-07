package argusctl

import (
	"context"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// CompatibleStrimziOwner checks the exact pinned chart rules and CRD schemas.
// A second namespace-local operator shares only definitions, not its lifecycle.
func CompatibleStrimziOwner(ctx context.Context, contextName, root string) (string, error) {
	clients, err := clientsFor(contextName)
	if err != nil {
		return "", err
	}
	ch, err := (helmManager{cacheDir: filepath.Join(root, "deploy", ".cache", "charts"), log: io.Discard}).loadRemoteChart(ctx, "strimzi-1.1.0", strimziChartURL)
	if err != nil {
		return "", err
	}
	cfg := &InstallConfig{}
	cfg.Spec.ReleaseID = "argus-dependency-probe"
	cfg.Spec.Namespaces.Observability = "argus-dependency-probe"
	return configureSharedStrimzi(ctx, cfg, clients, ch, map[string]any{})
}

func operatorObjects(ch *chart.Chart, cfg *InstallConfig, values map[string]any) ([]*unstructured.Unstructured, error) {
	renderValues, err := chartutil.ToRenderValues(ch, values, common.ReleaseOptions{Name: cfg.upstreamReleaseName("st"), Namespace: cfg.Spec.Namespaces.Observability, IsInstall: true}, common.DefaultCapabilities)
	if err != nil {
		return nil, err
	}
	files, err := engine.Render(ch, renderValues)
	if err != nil {
		return nil, err
	}
	objects := []*unstructured.Unstructured{}
	for name, manifest := range files {
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
		for {
			var object unstructured.Unstructured
			err = decoder.Decode(&object)
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if object.GetKind() != "" {
				objects = append(objects, &object)
			}
		}
	}
	return objects, nil
}

func configureSharedStrimzi(ctx context.Context, cfg *InstallConfig, clients *kubeClients, ch *chart.Chart, values map[string]any) (string, error) {
	objects, err := operatorObjects(ch, cfg, values)
	if err != nil {
		return "", err
	}
	owner := ""
	wanted, found, foreign := 0, 0, 0
	for _, object := range objects {
		if object.GetKind() != "ClusterRole" {
			continue
		}
		wanted++
		live, err := clients.typed.RbacV1().ClusterRoles().Get(ctx, object.GetName(), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		found++
		ns, release := live.Annotations["meta.helm.sh/release-namespace"], live.Annotations["meta.helm.sh/release-name"]
		if ns == cfg.Spec.Namespaces.Observability && release == cfg.upstreamReleaseName("st") {
			continue
		}
		if ns == "" || release == "" || live.DeletionTimestamp != nil {
			return "", fmt.Errorf("Strimzi role %s has no reusable owner", live.Name)
		}
		if owner != "" && owner != ns+"/"+release {
			return "", fmt.Errorf("Strimzi global definitions have mixed owners")
		}
		owner = ns + "/" + release
		foreign++
		actual, err := runtime.DefaultUnstructuredConverter.ToUnstructured(live)
		if err != nil {
			return "", err
		}
		if !reflect.DeepEqual(object.Object["rules"], actual["rules"]) || !reflect.DeepEqual(object.Object["aggregationRule"], actual["aggregationRule"]) {
			return "", fmt.Errorf("Strimzi role %s differs from pinned 1.1.0 chart", live.Name)
		}
	}
	if owner == "" {
		return "", nil
	}
	if found != wanted || foreign != wanted || wanted == 0 {
		return "", fmt.Errorf("partial Strimzi global definitions cannot be reused")
	}
	if shared, err := configureSharedDataCRDs(ctx, cfg, clients, ch); err != nil {
		return "", err
	} else if !shared {
		return "", fmt.Errorf("shared Strimzi roles require a complete compatible CRD inventory")
	}
	values["createGlobalResources"] = false
	// The upstream switch omits both the shared roles and operator bindings.
	// Recreate bindings with release-scoped names, targeting this operator only.
	for _, object := range objects {
		if object.GetKind() != "ClusterRoleBinding" {
			continue
		}
		object.SetName(cfg.upstreamReleaseName("st") + "-" + object.GetName())
		raw, err := object.MarshalJSON()
		if err != nil {
			return "", err
		}
		ch.Templates = append(ch.Templates, &common.File{Name: "templates/argus-" + object.GetName() + ".yaml", Data: raw})
	}
	return owner, nil
}

// Existing CRDs are a read-only shared dependency. Never relabel or upgrade them
// from a temporary install; reject an incomplete or incompatible inventory.
func configureSharedDataCRDs(ctx context.Context, cfg *InstallConfig, clients *kubeClients, ch *chart.Chart) (bool, error) {
	gvr := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	foreign, total, found := 0, 0, 0
	for _, file := range ch.CRDObjects() {
		decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(string(file.File.Data)), 4096)
		for {
			var wanted unstructured.Unstructured
			err := decoder.Decode(&wanted)
			if err == io.EOF {
				break
			}
			if err != nil {
				return false, err
			}
			if wanted.GetKind() != "CustomResourceDefinition" {
				continue
			}
			total++
			live, err := clients.dynamic.Resource(gvr).Get(ctx, wanted.GetName(), metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return false, err
			}
			found++
			if live.GetLabels()["argus.io/owner-release"] == cfg.Spec.ReleaseID {
				continue
			}
			// The API server defaults names omitted from the upstream manifest.
			// Compare the same identity after defaulting, without relaxing schemas.
			names, _, _ := unstructured.NestedMap(wanted.Object, "spec", "names")
			kind, _ := names["kind"].(string)
			if names["listKind"] == nil {
				names["listKind"] = kind + "List"
			}
			if names["singular"] == nil {
				names["singular"] = strings.ToLower(kind)
			}
			if err := unstructured.SetNestedMap(wanted.Object, names, "spec", "names"); err != nil {
				return false, err
			}
			if err := compatibleOpenSandboxCRD(&wanted, live); err != nil {
				return false, fmt.Errorf("shared data CRD %s: %w", wanted.GetName(), err)
			}
			foreign++
			if cfg.sharedDataCRDs == nil {
				cfg.sharedDataCRDs = map[string]bool{}
			}
			cfg.sharedDataCRDs[wanted.GetName()] = true
		}
	}
	if foreign > 0 && (found != total || foreign != total) {
		return false, fmt.Errorf("partial/mixed shared %s CRDs cannot be reused", ch.Name())
	}
	return foreign > 0, nil
}
