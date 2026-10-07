package argusdev

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/kakj-go/Argus/internal/app/argusctl"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type E2EOptions struct {
	SharedController    *argusctl.SharedSandboxController
	Suite               string
	KubeContext         string
	RunID               string
	Artifacts           string
	UnitOnly            bool
	Argusctl            string
	BaselinesRun        bool
	PKIMode             string
	BootstrapTLS        string
	RealModel           *p5RealModelConfig
	PlanV2RuntimeOnly   bool
	PlanV2BrowserGrep   string
	PlanV2RealModelOnly bool
	PlanV2ModelGroups   []string
}

type E2EEnvironment struct {
	IngressNS, IngressClass string
	Forwards                []*KubeForward
	Paused                  []pausedWorkload
	Options                 E2EOptions
	Root                    string
	WorkDir                 string
	ConfigPath              string
	Profile                 string
	ReleaseID               string
	SystemNS                string
	SandboxNS               string
	ObservNS                string
	ImageTag                string
	ImagePlatform           string
	Argusctl                string
	Kube                    *E2EKube
	Endpoints               *E2EEndpoints
	Processes               []*Process
	ManagedNamespaces       []string
	ManagedClusterRBAC      []string
	ExternalPKI             *e2eExternalPKI
	State                   *ScenarioState
	CollectorArtifacts      *E2ECollectorArtifacts
	ArtifactSigning         *E2EArtifactSigning
	ArtifactTLS             fixtureCertificate
	installed               bool
	imagesAttempted         bool
	installAttempted        bool
	leaseAcquired           bool
	fixtureAttempted        bool
	fixtureReady            bool
}

var suiteDependencies = map[string][]string{
	"m2":                  {"m2"},
	"m3":                  {"m2", "m3"},
	"m4":                  {"m2", "m4"},
	"p5":                  {"m2", "p5"},
	"m6":                  {"m2", "m3", "m6"},
	"m7":                  {"m2", "m3", "m4", "p5-native", "m7"},
	"m10-query":           {"m2", "m3", "m4", "p5-native", "m7", "m10-query"},
	"planv2":              {"m2", "m3", "m4", "p5-native", "m7", "m10-query", "planv2"},
	"m8":                  {"m6", "m7", "m8"},
	"p4":                  {"m2", "p4"},
	"tls":                 {"m2", "tls"},
	"tls-managed-strict":  {"m2", "tls"},
	"tls-existing-strict": {"m2", "tls"},
}

func (a *App) runE2E(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "windows-host" {
		return a.runWindowsHostE2E(ctx, args[1:])
	}
	if len(args) == 0 || args[0] != "run" {
		return fmt.Errorf("%w: usage: argus-dev e2e run --suite SUITE [options]", errUsage)
	}
	flags := flag.NewFlagSet("e2e run", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	suite := flags.String("suite", envOr("ARGUS_E2E_SUITE", ""), "E2E suite")
	kubeContext := flags.String("kube-context", envOr("ARGUS_E2E_KUBE_CONTEXT", ""), "Kubernetes context")
	runID := flags.String("run-id", envOr("ARGUS_E2E_RUN_ID", ""), "run identifier")
	artifacts := flags.String("artifacts", envOr("ARGUS_E2E_ARTIFACTS", ""), "artifact directory")
	unitOnly := flags.Bool("unit-only", envBool("ARGUS_E2E_UNIT_ONLY"), "run the suite's non-cluster gate only")
	argusctl := flags.String("argusctl", envOr("ARGUS_E2E_ARGUSCTL", envOr("ARGUSCTL_BIN", "")), "existing argusctl binary")
	sharedControllerConfig := flags.String("shared-controller-config", "", "existing trusted Argus install config containing the shared controller namespace and immutable image digest; never modifies that controller")
	realModelPath := flags.String("real-model-config", "", "P5 or PlanV2 real-model benchmark JSON configuration (API key from named environment variable)")
	runtimeOnly := flags.Bool("planv2-runtime-only", false, "PlanV2 runtime/protocol/fault acceptance without browser tests; reported as partial scope")
	browserGrep := flags.String("planv2-browser-grep", "", "run matching PlanV2 browser cases plus runtime; records the selected scope")
	realModelOnly := flags.Bool("planv2-real-model-only", false, "run real model tasks with deployed telemetry and file prerequisites; excludes browser and fault suites")
	modelGroups := flags.String("planv2-model-groups", "", "explicit PlanV2 real-model groups: core,authoring,conditions,failures; omitted runs all")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("%w: %v", errUsage, err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%w: unexpected E2E arguments", errUsage)
	}
	if _, exists := suiteDependencies[*suite]; !exists {
		return fmt.Errorf("%w: unsupported suite %q", errUsage, *suite)
	}
	if *realModelOnly && (*suite != "planv2" || *unitOnly || *runtimeOnly || *browserGrep != "" || *realModelPath == "") {
		return fmt.Errorf("%w: --planv2-real-model-only requires PlanV2 cluster acceptance and a real-model configuration without other scope selectors", errUsage)
	}
	if *modelGroups != "" && !*realModelOnly {
		return fmt.Errorf("%w: model group selection requires --planv2-real-model-only", errUsage)
	}
	groups, groupErr := planV2ParseModelGroups(*modelGroups)
	if groupErr != nil {
		return groupErr
	}
	if *runtimeOnly && (*suite != "planv2" || *unitOnly || *realModelPath != "") {
		return fmt.Errorf("%w: --planv2-runtime-only requires PlanV2 cluster acceptance without a real-model configuration", errUsage)
	}
	if *browserGrep != "" {
		if *suite != "planv2" || *unitOnly || *runtimeOnly || *realModelPath != "" || len(*browserGrep) > 1024 {
			return fmt.Errorf("%w: --planv2-browser-grep requires PlanV2 cluster acceptance without runtime-only or real-model options", errUsage)
		}
		if _, err := regexp.Compile(*browserGrep); err != nil {
			return fmt.Errorf("%w: invalid browser selection", errUsage)
		}
	}
	if *runID == "" {
		*runID = time.Now().UTC().Format("20060102150405") + fmt.Sprintf("-%d", os.Getpid())
	}
	if *artifacts == "" {
		*artifacts = filepath.Join(a.root, "artifacts", *suite+"-e2e", *runID)
	} else if !filepath.IsAbs(*artifacts) {
		*artifacts = filepath.Join(a.root, *artifacts)
	}
	options := E2EOptions{Suite: *suite, KubeContext: *kubeContext, RunID: *runID, Artifacts: *artifacts, UnitOnly: *unitOnly, Argusctl: *argusctl, PlanV2RuntimeOnly: *runtimeOnly}
	if *sharedControllerConfig != "" {
		cfg, err := loadE2ESharedController(*sharedControllerConfig)
		if err != nil {
			return err
		}
		options.SharedController = cfg
	}
	options.PlanV2BrowserGrep = *browserGrep
	options.PlanV2RealModelOnly = *realModelOnly
	if *modelGroups != "" {
		options.PlanV2ModelGroups = groups
	}
	if *realModelPath != "" {
		if (*suite != "p5" && *suite != "planv2") || *unitOnly {
			return fmt.Errorf("%w: --real-model-config requires the P5 or PlanV2 cluster suite", errUsage)
		}
		var err error
		options.RealModel, err = loadP5RealModelConfig(*realModelPath)
		if err != nil {
			return err
		}
	}
	if *unitOnly {
		return a.runE2EUnitGate(ctx, options)
	}
	if options.Suite == "tls" {
		return a.runTLSE2EMatrix(ctx, options)
	}
	return a.runE2ECluster(ctx, options)
}

func (a *App) runE2EUnitGate(ctx context.Context, options E2EOptions) error {
	targets := []string{"./..."}
	if options.Suite == "m10-query" {
		targets = []string{"./internal/transport/httpapi", "./internal/telemetry", "./tests/contract"}
	}
	if options.Suite == "planv2" {
		targets = []string{"./internal/dashboard", "./internal/selfmonitor", "./internal/otelcol/configbundle", "./internal/telemetry", "./internal/transport/httpapi", "./tests/contract"}
	}
	for _, target := range targets {
		if err := a.runner.Run(ctx, nil, "go", "test", target); err != nil {
			return err
		}
	}
	if suiteFixtureFeatures(options.Suite).Replay {
		return a.runner.Run(ctx, nil, "go", "test", "-tags", "m4e2e", "./cmd/argus-replay-model")
	}
	return nil
}

func (a *App) runE2ECluster(ctx context.Context, options E2EOptions) (returnErr error) {
	if strings.TrimSpace(options.KubeContext) == "" {
		contextName, err := a.doctorOutput(ctx, "kubectl", "config", "current-context")
		if err != nil {
			return fmt.Errorf("%w: resolve Kubernetes context: %v", errCapability, err)
		}
		options.KubeContext = contextName
	}
	if report := a.doctorWithOptions(ctx, "e2e", doctorOptions{KubeContext: options.KubeContext, E2ESuite: options.Suite, SharedController: options.SharedController}); !report.Ready {
		_ = writeDoctor(a.stderr, "text", report)
		return fmt.Errorf("%w: doctor e2e found missing requirements", errCapability)
	}
	if options.Suite == "m8" && !options.BaselinesRun {
		for _, baseline := range []string{"m6", "m7"} {
			baselineOptions := options
			baselineOptions.Suite = baseline
			baselineOptions.RunID = options.RunID + "-" + baseline
			baselineOptions.Artifacts = filepath.Join(options.Artifacts, baseline+"-baseline")
			baselineOptions.BaselinesRun = true
			if err := a.runE2ECluster(ctx, baselineOptions); err != nil {
				return fmt.Errorf("M8 %s baseline: %w", baseline, err)
			}
		}
		options.BaselinesRun = true
	}
	if err := os.MkdirAll(options.Artifacts, 0o700); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "argus-dev-e2e-*")
	if err != nil {
		return err
	}
	env := &E2EEnvironment{Options: options, Root: a.root, WorkDir: workDir, State: NewScenarioState(options.RunID)}
	defer func() {
		if returnErr != nil {
			_, _ = fmt.Fprintf(a.stderr, "E2E failed before cleanup: %v\n", returnErr)
		}
		returnErr = errors.Join(returnErr, a.cleanupE2E(env))
		_ = os.RemoveAll(workDir)
	}()
	if err := a.prepareE2EEnvironment(ctx, env); err != nil {
		return err
	}
	if err := env.Kube.AcquireLease(ctx, "argus-global-e2e", options.RunID); err != nil {
		return err
	}
	env.leaseAcquired = true
	if err := a.installE2EIngress(ctx, env); err != nil {
		return err
	}
	if err := a.pauseFormalWorkloads(ctx, env); err != nil {
		return err
	}
	if err := a.invokeArgusctl(ctx, env, "preflight", "--config", env.ConfigPath); err != nil {
		return err
	}
	if err := a.invokeArgusctl(ctx, env, "plan", "--config", env.ConfigPath); err != nil {
		return err
	}
	env.imagesAttempted = true
	if err := a.prepareE2EImages(ctx, env); err != nil {
		return err
	}
	env.installAttempted = true
	if err := a.invokeArgusctl(ctx, env, "install", "--config", env.ConfigPath); err != nil {
		return err
	}
	// P5 installs agent-lite first, then enables OpenSandbox using the same
	// release and signing identity. The second install clears this key.
	if options.Suite != "p5" {
		env.clearArtifactSigningPrivateKey()
	}
	env.installed = true
	if err := configureRealModelEndpointDNS(ctx, env); err != nil {
		return err
	}
	env.fixtureAttempted = true
	if err := a.installE2EFixtures(ctx, env); err != nil {
		return err
	}
	env.fixtureReady = true
	if err := a.resolveE2EAccess(ctx, env); err != nil {
		return err
	}
	if err := a.runE2EScenarios(ctx, env); err != nil {
		return err
	}
	if err := a.invokeArgusctl(ctx, env, "verify", "--config", env.ConfigPath, "--artifacts", filepath.Join(options.Artifacts, "verify")); err != nil {
		return err
	}
	result := fmt.Sprintf("{\"run_id\":%q,\"suite\":%q,\"status\":\"passed\"}\n", options.RunID, options.Suite)
	if options.PlanV2RealModelOnly {
		result = fmt.Sprintf("{\"run_id\":%q,\"suite\":%q,\"scope\":\"real_model_with_telemetry_and_files\",\"browser_executed\":false,\"status\":\"passed\"}\n", options.RunID, options.Suite)
		if len(options.PlanV2ModelGroups) > 0 {
			raw, _ := json.Marshal(map[string]any{"run_id": options.RunID, "suite": options.Suite, "scope": "selected_real_model_groups_with_telemetry_and_files", "model_groups": options.PlanV2ModelGroups, "browser_executed": false, "status": "passed"})
			result = string(raw) + "\n"
		}
	}
	if options.PlanV2RuntimeOnly {
		result = fmt.Sprintf("{\"run_id\":%q,\"suite\":%q,\"scope\":\"runtime_protocol_faults\",\"browser_executed\":false,\"status\":\"passed\"}\n", options.RunID, options.Suite)
	}
	if options.PlanV2BrowserGrep != "" {
		raw, _ := json.Marshal(map[string]any{"run_id": options.RunID, "suite": options.Suite, "scope": "selected_browsers_and_runtime", "browser_filter": options.PlanV2BrowserGrep, "status": "passed"})
		result = string(raw) + "\n"
	}
	return writePrivate(filepath.Join(options.Artifacts, "result.json"), []byte(result))
}

func (a *App) prepareE2EEnvironment(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.KubeContext == "" {
		contextName, err := a.runner.Output(ctx, nil, "kubectl", "config", "current-context")
		if err != nil {
			return err
		}
		env.Options.KubeContext = contextName
	}
	kube, err := NewE2EKube(env.Options.KubeContext, env.Options.Artifacts)
	if err != nil {
		return err
	}
	env.Kube = kube
	architecture, err := kube.NodeArchitecture(ctx)
	if err != nil {
		return err
	}
	if architecture != "arm64" && architecture != "amd64" {
		return fmt.Errorf("unsupported Kubernetes node architecture %s", architecture)
	}
	env.ImagePlatform = "linux/" + architecture
	release := releaseIDForDev(env.Options.Suite + "-" + env.Options.RunID)
	env.ReleaseID = release
	env.SystemNS = kubernetesNameForDev(release + "-system")
	env.SandboxNS = kubernetesNameForDev(release + "-sandbox")
	env.ObservNS = kubernetesNameForDev(release + "-observability")
	if envBool("ARGUS_E2E_ISOLATED_INGRESS") {
		env.IngressNS = kubernetesNameForDev(release + "-ingress")
		env.IngressClass = kubernetesNameForDev(release + "-nginx")
	}
	env.ImageTag = kubernetesNameForDev("e2e-" + env.Options.RunID)
	if env.Options.PKIMode == "existing-cluster-issuer" {
		if err := a.prepareE2EExternalIssuer(ctx, env); err != nil {
			return err
		}
	}
	if err := a.prepareE2EArtifactServer(env); err != nil {
		return err
	}
	if err := a.prepareE2ECollectorArtifacts(ctx, env); err != nil {
		return err
	}
	profile := "evaluation"
	if env.Options.Suite == "m8" {
		profile = "local-hardening"
	}
	configPath, err := a.writeE2EConfig(env, profile)
	if err != nil {
		return err
	}
	env.ConfigPath = configPath
	if env.Options.Argusctl != "" {
		env.Argusctl = env.Options.Argusctl
		return nil
	}
	binary := filepath.Join(env.WorkDir, "argusctl")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := a.runner.Run(ctx, nil, "go", "build", "-trimpath", "-o", binary, "./cmd/argusctl"); err != nil {
		return err
	}
	env.Argusctl = binary
	return nil
}

func (a *App) writeE2EConfig(env *E2EEnvironment, profile string) (string, error) {
	source := filepath.Join(a.root, "deploy", "profiles", profile+".yaml")
	data, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return "", err
	}
	spec := nestedMap(document, "spec")
	env.Profile = profile
	document["metadata"] = map[string]any{"name": env.ReleaseID}
	spec["profile"] = profile
	spec["kubeContext"] = env.Options.KubeContext
	spec["releaseId"] = env.ReleaseID
	spec["namespaces"] = map[string]any{"system": env.SystemNS, "sandbox": env.SandboxNS, "observability": env.ObservNS}
	workspace := nestedMap(spec, "workspace")
	workspace["storageNamespace"] = env.SystemNS
	workspace["storageClass"] = "argus-workspace"
	workspace["defaultBytes"] = int64(256 << 20)
	workspace["maxFileBytes"] = int64(100 << 20)
	if env.Options.Suite == "p5" {
		nestedMap(spec, "openSandbox")["enabled"] = false
	}
	if env.Options.SharedController != nil {
		nestedMap(spec, "openSandbox")["sharedController"] = env.Options.SharedController
		nestedMap(spec, "openSandbox")["allowSharedRuntime"] = true
	}
	// Ingress-nginx rejects duplicate host/path pairs across namespaces. Give
	// every E2E release its own DNS suffix so a suite can coexist with a local
	// Argus installation (and with another suite) without mutating either one.
	exposure := nestedMap(spec, "exposure")
	if selected := os.Getenv("ARGUS_E2E_INGRESS_CLASS"); selected != "" {
		exposure["ingressClassName"] = selected
	}
	if address := os.Getenv("ARGUS_E2E_HTTPS_INTERNAL_ADDRESS"); address != "" {
		exposure["httpsInternalAddress"] = address
	}
	if env.IngressNS != "" {
		exposure["ingressClassName"] = env.IngressClass
		exposure["httpsInternalAddress"] = "argus-e2e-ingress." + env.IngressNS + ".svc:443"
	}
	e2eDomain := env.ReleaseID + ".argus.test"
	exposure["enterpriseHost"] = "enterprise." + e2eDomain
	exposure["platformHost"] = "platform." + e2eDomain
	exposure["connectorHost"] = "connector." + e2eDomain
	if env.Options.PKIMode != "" {
		pki := nestedMap(spec, "pki")
		pki["mode"] = env.Options.PKIMode
		pki["bootstrapTLSMode"] = env.Options.BootstrapTLS
		if env.ExternalPKI != nil {
			pki["issuerRef"] = map[string]any{"name": env.ExternalPKI.IssuerName, "group": "cert-manager.io"}
			pki["caBundle"] = map[string]any{"inlinePEM": env.ExternalPKI.CAPEM}
		}
	}
	images := nestedMap(spec, "images")
	images["tag"] = env.ImageTag
	if env.ArtifactSigning != nil {
		telemetry := nestedMap(spec, "telemetry")
		telemetry["signingKeyId"] = env.ArtifactSigning.KeyID
		telemetry["signingPublicKey"] = base64.RawStdEncoding.EncodeToString(env.ArtifactSigning.PublicKey)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	registryPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	images["registry"] = fmt.Sprintf("host.docker.internal:%d", registryPort)
	images["pullPolicy"] = "Never"
	if artifacts := env.CollectorArtifacts; artifacts != nil {
		spec["telemetry"] = map[string]any{
			"collectorVersion": artifacts.Version,
			"linuxArm64Uri":    artifacts.LinuxURI, "linuxArm64Sha256": artifacts.LinuxSHA256,
			"linuxArm64Signature": artifacts.LinuxSignature, "linuxArm64ByteSize": artifacts.LinuxByteSize,
			"linuxAmd64Uri": artifacts.LinuxAMD64URI, "linuxAmd64Sha256": artifacts.LinuxAMD64SHA256,
			"linuxAmd64Signature": artifacts.LinuxAMD64Signature, "linuxAmd64ByteSize": artifacts.LinuxAMD64ByteSize,
			"windowsAmd64Uri": artifacts.WindowsURI, "windowsAmd64Sha256": artifacts.WindowsSHA256,
			"windowsAmd64Signature": artifacts.WindowsSignature, "windowsAmd64ByteSize": artifacts.WindowsByteSize,
			"signingKeyId": artifacts.SigningKeyID, "signingPublicKey": artifacts.SigningPublicKey,
		}
	}
	if profile == "local-hardening" {
		security := nestedMap(spec, "security")
		security["platformMfaRequired"] = true
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return "", err
	}
	path := filepath.Join(env.Options.Artifacts, "install-config.yaml")
	if err := writePrivate(path, encoded); err != nil {
		return "", err
	}
	return path, nil
}

func nestedMap(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	value := map[string]any{}
	parent[key] = value
	return value
}

func (a *App) invokeArgusctl(ctx context.Context, env *E2EEnvironment, args ...string) error {
	variables := map[string]string{}
	if env.Endpoints != nil {
		variables["ARGUSCTL_HTTPS_PROBE_ADDRESS"] = env.Endpoints.IngressDialAddress
		if env.IngressNS != "" {
			variables["ARGUSCTL_CONNECTOR_FORWARD_ADDRESS"] = env.Endpoints.ConnectorDialAddress
		}
	}
	if env.ArtifactSigning != nil && len(env.ArtifactSigning.PrivateKey) == ed25519.PrivateKeySize {
		variables["ARGUS_OTELCOL_SIGNING_PRIVATE_KEY"] = base64.RawStdEncoding.EncodeToString(env.ArtifactSigning.PrivateKey)
	}
	stdout := &diagnosticStream{destination: a.runner.Stdout}
	stderr := &diagnosticStream{destination: a.runner.Stderr}
	err := a.runner.RunIO(ctx, variables, nil, stdout, stderr, env.Argusctl, args...)
	return errors.Join(err, stdout.Flush(), stderr.Flush())
}

func (a *App) cleanupE2E(env *E2EEnvironment) error {
	env.clearArtifactSigningPrivateKey()
	var cleanupErrors []error
	record := func(operation string, err error) {
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", operation, err))
		}
	}
	for index := len(env.Processes) - 1; index >= 0; index-- {
		record("stop local process", env.Processes[index].Stop(5*time.Second))
	}
	for _, forward := range env.Forwards {
		record("stop E2E port forward", forward.Stop())
	}
	if env.Kube != nil {
		diagnosticCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		a.collectScenarioState(diagnosticCtx, env)
		record("collect redacted diagnostics", env.Kube.CollectDiagnostics(diagnosticCtx, env, filepath.Join(env.Options.Artifacts, "diagnostics")))
		cancel()
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if env.Kube != nil && env.fixtureAttempted {
		record("clean E2E fixtures", a.cleanupE2EFixtures(cleanupCtx, env))
	} else if env.imagesAttempted {
		record("clean local E2E fixture images", a.removeLocalFixtureImages(cleanupCtx, env.State.FixtureImages))
	}
	// The local registry may be owned by this E2E release and removed by
	// argusctl cleanup, so delete the exact run tags while it is reachable.
	if env.imagesAttempted && env.ConfigPath != "" {
		record("delete remote E2E image tags", a.removeRemoteE2EImages(cleanupCtx, env))
	}
	if env.installAttempted && env.Argusctl != "" && env.ConfigPath != "" {
		if env.IngressNS != "" && env.Kube != nil {
			record("clean unallocated local gateway service", env.Kube.CleanupUnallocatedLocalService(cleanupCtx, env.SystemNS, "argus-connector-gateway-public", env.ReleaseID))
		}
		record("uninstall Argus", a.invokeArgusctl(cleanupCtx, env, "uninstall", "--config", env.ConfigPath, "--delete-data", "--delete-owned-crds", "--yes"))
	} else if env.imagesAttempted && env.Argusctl != "" && env.ConfigPath != "" {
		record("clean E2E images", a.invokeArgusctl(cleanupCtx, env, "images", "clean", "--config", env.ConfigPath))
	}
	if env.Kube != nil {
		record("delete managed Collector RBAC", cleanupManagedCollectorRBAC(cleanupCtx, env))
		for _, namespace := range env.ManagedNamespaces {
			if namespace != "" {
				record("delete namespace "+namespace, env.Kube.DeleteNamespace(cleanupCtx, namespace, env.ReleaseID))
			}
		}
		for _, namespace := range []string{env.SystemNS, env.SandboxNS, env.ObservNS} {
			if namespace != "" {
				record("delete namespace "+namespace, env.Kube.DeleteNamespace(cleanupCtx, namespace, env.ReleaseID))
			}
		}
		for _, name := range env.ManagedClusterRBAC {
			err := env.Kube.Client.RbacV1().ClusterRoleBindings().Delete(cleanupCtx, name, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				record("delete ClusterRoleBinding "+name, err)
			}
			err = env.Kube.Client.RbacV1().ClusterRoles().Delete(cleanupCtx, name, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				record("delete ClusterRole "+name, err)
			}
		}
		if env.IngressClass != "" {
			err := env.Kube.Client.NetworkingV1().IngressClasses().Delete(cleanupCtx, env.IngressClass, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				record("delete E2E IngressClass", err)
			}
		}
		if env.leaseAcquired {
			record("restore original replicas", restoreFormalWorkloads(cleanupCtx, env))
			record("release E2E Lease", env.Kube.ReleaseLease(cleanupCtx, "argus-global-e2e", env.Options.RunID))
		}
		if env.ExternalPKI != nil {
			record("delete external E2E ClusterIssuer", a.deleteE2EExternalIssuer(cleanupCtx, env))
		}
	}
	return errors.Join(cleanupErrors...)
}

func cleanupManagedCollectorRBAC(ctx context.Context, env *E2EEnvironment) error {
	managedNamespaces := make(map[string]struct{}, len(env.ManagedNamespaces))
	for _, namespace := range env.ManagedNamespaces {
		if namespace != "" {
			managedNamespaces[namespace] = struct{}{}
		}
	}
	if len(managedNamespaces) == 0 {
		return nil
	}
	bindings, err := env.Kube.Client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/part-of=argus"})
	if err != nil {
		return err
	}
	var cleanupErrors []error
	for _, binding := range bindings.Items {
		if !strings.HasPrefix(binding.Name, "argus-otelcol-") {
			continue
		}
		owned := false
		for _, subject := range binding.Subjects {
			if _, exists := managedNamespaces[subject.Namespace]; exists {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if err := env.Kube.Client.RbacV1().ClusterRoleBindings().Delete(ctx, binding.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete ClusterRoleBinding %s: %w", binding.Name, err))
		}
		if err := env.Kube.Client.RbacV1().ClusterRoles().Delete(ctx, binding.RoleRef.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete ClusterRole %s: %w", binding.RoleRef.Name, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func kubernetesNameForDev(value string) string {
	return boundedKubernetesNameForDev(value, 63)
}

func releaseIDForDev(value string) string {
	return boundedKubernetesNameForDev(value, 34)
}

func boundedKubernetesNameForDev(value string, maxLength int) string {
	value = strings.ToLower(value)
	var result strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			result.WriteRune(character)
		} else {
			result.WriteByte('-')
		}
	}
	name := strings.Trim(result.String(), "-")
	if len(name) > maxLength {
		digest := sha256.Sum256([]byte(name))
		suffix := hex.EncodeToString(digest[:])[:8]
		prefixLength := maxLength - len(suffix) - 1
		prefix := strings.TrimRight(name[:prefixLength], "-")
		name = prefix + "-" + suffix
	}
	return name
}

func openArtifact(path string) (io.WriteCloser, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
}
