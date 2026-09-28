package argusdev

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	argusopenapi "github.com/kakj-go/Argus/api/openapi"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	corev1 "k8s.io/api/core/v1"
)

func TestPlanV2DashboardAndSelfCollectorUsePublishedSourceContracts(t *testing.T) {
	spec, err := planV2SelfDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Panels) != 18 {
		t.Fatal("missing per-source APM/Trace coverage")
	}
	for _, panel := range spec.Panels {
		if len(panel.Drilldowns) == 0 || len(panel.DetailQueryTargets) == 0 {
			t.Fatalf("missing published drilldown for %s", panel.ID)
		}
	}
	input := configbundle.RenderInput{CollectorID: uuid.NewString(), ResourceID: uuid.NewString(), SourceGeneration: uuid.NewString(), ConfigRevision: 2, ResourceType: "kubernetes_cluster", Role: "direct", Platform: "linux_amd64", RouteKind: "direct_argus", Transport: "direct", ProfileKeys: []string{"k8s-node-container", "otlp-receiver", "skywalking-receiver", "jaeger-receiver"}, EnrollmentEndpoint: "https://example.test/enroll", IngestGRPCEndpoint: "grpcs://ingest.example.test:4317", IngestHTTPEndpoint: "https://ingest.example.test:4318"}
	raw, err := configbundle.Render(input)
	if err != nil {
		t.Fatal(err)
	}
	config, err := planV2SelfCollector(raw)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err = json.Unmarshal(config, &parsed); err != nil {
		t.Fatal(err)
	}
	receivers := parsed["receivers"].(map[string]any)
	if len(receivers) != 3 || receivers["skywalking"] == nil || receivers["jaeger"] == nil {
		t.Fatal("native receivers missing")
	}
	seen := map[string]bool{}
	for name, value := range parsed["service"].(map[string]any)["pipelines"].(map[string]any) {
		p := value.(map[string]any)
		if p["receivers"].([]any)[0] == "otlp" {
			signal, _, _ := strings.Cut(name, "/")
			seen[signal] = true
		}
	}
	if !seen["metrics"] || !seen["logs"] || !seen["traces"] {
		t.Fatal("registered OTLP receiver lost one signal")
	}
	if len(parsed["extensions"].(map[string]any)) != 0 {
		t.Fatal("self-check sidecar must not compete for Collector identity rotation")
	}
	sources, err := configbundle.Sources(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.Type == "skywalking" || source.Type == "jaeger" {
			if len(source.Signals) != 1 || source.Signals[0] != "traces" {
				t.Fatal("unverified native JVM metrics exposed")
			}
		}
	}
}

func TestPlanV2FileFixtureHasExecutablePublishedThreeSignalShapes(t *testing.T) {
	spec, err := planV2ThreeSignals()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Panels) != 3 || spec.Panels[0].Targets[0].QueryMode != "range" {
		t.Fatal("file proof needs three actual signals and a range for historical metrics")
	}
	for _, panel := range spec.Panels {
		if panel.Signal != panel.ID || panel.SourceBinding.SourceType != "otlp" {
			t.Fatal("file fixture changed signal/source coverage")
		}
	}
}

func TestPlanV2ParametersUseCascadeLocalAndExplicitSignalMapping(t *testing.T) {
	spec, err := planV2ParameterDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Panels) != 4 || spec.Variables[1].Default.Values[0] != "blue-220" || spec.Panels[3].Targets[0].ParameterBindings[0].ValueMap["green"] != "ERROR" {
		t.Fatal("lost boundary fixture contract")
	}
}

func TestPlanV2MetricGalleryHasValidQueriesAndBucketInput(t *testing.T) {
	spec, err := planV2MetricGallery()
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Panels) != 11 {
		t.Fatal("missing metric chart shapes")
	}
	for _, panel := range spec.Panels {
		if panel.Type == "histogram" && !strings.Contains(panel.Targets[0].SourceDefinition.DSL.Expression, "_bucket[") {
			t.Fatal("histogram must use cumulative buckets rather than gauge values")
		}
	}
}

func TestPlanV2BrowserRegexSelectorRunsOnWindows(t *testing.T) {
	for _, selector := range []string{"e2e/planv2-real.spec.ts", `e2e/planv2-.*\.spec\.ts`} {
		if !planV2BrowserSelector(selector) {
			t.Fatalf("real browser selector silently skipped: %s", selector)
		}
	}
	if planV2BrowserSelector("e2e/m4-real.spec.ts") {
		t.Fatal("baseline portal suite unexpectedly included")
	}
}

func TestSelfTracingTargetsQueryProgramRatherThanDeploymentName(t *testing.T) {
	pod := corev1.PodSpec{Containers: []corev1.Container{{Name: "telemetry", Command: []string{"/usr/local/bin/argus-telemetry"}, Args: []string{"--mode=query"}}, {Name: "collector", Command: []string{"/usr/local/bin/argus-otelcol"}}}}
	if err := enablePlanV2SelfTracing(&pod, "argus-telemetry-query"); err != nil {
		t.Fatal(err)
	}
	if len(pod.Containers[0].Env) != 3 || len(pod.Containers[1].Env) != 0 {
		t.Fatal("tracing configured the wrong process")
	}
	if err := enablePlanV2SelfTracing(&pod, "argus-server"); err == nil {
		t.Fatal("missing target silently accepted")
	}
	pod.Containers = append(pod.Containers, pod.Containers[0])
	if err := enablePlanV2SelfTracing(&pod, "argus-telemetry-query"); err == nil {
		t.Fatal("ambiguous target silently accepted")
	}
}

func TestPlanV2FixtureUsesActualPublicHTTPContract(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromData(argusopenapi.BundledJSON)
	if err != nil {
		t.Fatal(err)
	}
	requestBytes, _ := json.Marshal(planV2ToolCatalogInput(uuid.NewString(), time.Now().UTC()))
	var catalogRequest any
	if err := json.Unmarshal(requestBytes, &catalogRequest); err != nil {
		t.Fatal(err)
	}
	if err := document.Components.Schemas["DashboardCatalogInput"].Value.VisitJSON(catalogRequest); err != nil {
		t.Fatal(err)
	}
	if err := document.Components.Schemas["DashboardLifecycleInput"].Value.VisitJSON(planV2LifecycleInput("archive", float64(1))); err != nil {
		t.Fatal(err)
	}
	path := document.Paths.Value(planV2DraftPath)
	if path == nil || path.Post == nil || document.Paths.Value(planV2DashboardPath) == nil {
		t.Fatal("fixture route is not part of the public API")
	}
	for _, fixture := range []func() (dashboard.Spec, error){planV2SelfDashboard, planV2ParameterDashboard, planV2DepthSpec, planV2NativeMappingSpec, planV2CapacitySpec} {
		spec, err := fixture()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(planV2DraftRequest(spec))
		var request any
		if err = json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		if err = path.Post.RequestBody.Value.Content["application/json"].Schema.Value.VisitJSON(request); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlanV2DataEvidenceRejectsMixedSources(t *testing.T) {
	if planV2SourcesMatch(map[string]any{"data": []any{map[string]any{"sourceId": "foreign"}}}, map[string]bool{"own": true}) {
		t.Fatal("metadata-only source assertion accepted foreign data")
	}
	if !planV2SourcesMatch(map[string]any{"data": []any{map[string]any{"sourceId": "own"}}}, map[string]bool{"own": true}) {
		t.Fatal("owned data was rejected")
	}
}
