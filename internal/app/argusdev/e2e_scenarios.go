package argusdev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) runE2EScenarios(ctx context.Context, env *E2EEnvironment) error {
	if err := a.runM2Scenario(ctx, env); err != nil {
		return err
	}
	if env.Options.Suite == "m8" {
		return a.runM8Scenario(ctx, env)
	}
	for _, phase := range suiteDependencies[env.Options.Suite] {
		var err error
		if phase != "m2" {
			_, _ = fmt.Fprintf(a.stdout, "E2E phase %s started\n", phase)
		}
		switch phase {
		case "m2":
			continue
		case "m3":
			err = a.runM3Scenario(ctx, env)
		case "m4":
			err = a.runM4Scenario(ctx, env)
		case "p5":
			err = a.runP5Scenario(ctx, env)
		case "p5-native":
			err = a.verifyP5Native(ctx, env)
		case "m6":
			err = a.runM6Scenario(ctx, env)
		case "m7":
			err = a.runM7Scenario(ctx, env)
		case "m10-query":
			err = a.runM10QueryScenario(ctx, env)
		case "planv2":
			err = a.runPlanV2Scenario(ctx, env)
		case "p4":
			err = a.runP4Scenario(ctx, env)
		case "tls":
			err = a.runTLSScenario(ctx, env)
		default:
			err = fmt.Errorf("unsupported E2E dependency %q", phase)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", phase, err)
		}
		_, _ = fmt.Fprintf(a.stdout, "E2E phase %s passed\n", phase)
	}
	return nil
}
func (a *App) runPlaywright(ctx context.Context, env *E2EEnvironment, spec string, variables map[string]string) error {
	// PlanV2 depends on the setup/API checks from these suites, while its own
	// real browser cases exercise the dashboard. Baseline portal UI stays in
	// m2/m3/m4/m7 so focused runs do not repeat all MFA-driven browser flows.
	if env.Options.Suite == "planv2" && !planV2BrowserSelector(spec) {
		_, _ = fmt.Fprintf(a.stdout, "Baseline browser %s belongs to its own suite; PlanV2 retains the setup/API checks\n", spec)
		return nil
	}
	variables["ARGUS_E2E_EXTERNAL"] = "1"
	artifactDir := filepath.Join(env.Options.Artifacts, "playwright-"+env.Options.Suite)
	variables["ARGUS_E2E_ARTIFACTS"] = artifactDir
	variables["ARGUS_E2E_ENTERPRISE_ORIGIN"] = env.Endpoints.EnterpriseOrigin
	variables["ARGUS_E2E_PLATFORM_ORIGIN"] = env.Endpoints.PlatformOrigin
	variables["ARGUS_E2E_TEMPLATE_ORIGIN"] = env.Endpoints.TemplateOrigin
	variables["ARGUS_E2E_HOST_RESOLVER"] = env.Endpoints.HostResolver
	variables["ARGUS_E2E_ENTERPRISE_TOTP_SECRET"] = env.State.Values["enterprise_mfa_secret"]
	variables["ARGUS_E2E_ENTERPRISE_TOTP_LAST_CODE"] = env.State.Values["enterprise_mfa_last"]
	variables["ARGUS_E2E_PLATFORM_TOTP_SECRET"] = env.State.Values["platform_mfa_secret"]
	variables["ARGUS_E2E_PLATFORM_TOTP_LAST_CODE"] = env.State.Values["platform_mfa_last"]
	args := []string{"--filter", "@argus/enterprise", "exec", "playwright", "test", spec, "--workers=1"}
	if env.Options.Suite == "planv2" && env.Options.PlanV2BrowserGrep != "" {
		args = append(args, "--grep", env.Options.PlanV2BrowserGrep)
	}
	err := a.runner.Run(ctx, variables, "pnpm", args...)
	return errors.Join(err, syncPlaywrightMFAState(env, artifactDir))
}

// Playwright selectors use forward-slash paths and may contain regex escapes.
// filepath.Base on Windows treats a regex backslash as a directory separator.
func planV2BrowserSelector(spec string) bool {
	return strings.HasPrefix(path.Base(spec), "planv2-")
}

func syncPlaywrightMFAState(env *E2EEnvironment, artifactDir string) error {
	for audience, stateKey := range map[string]string{
		"enterprise":        "enterprise_mfa_last",
		"platform":          "platform_mfa_last",
		"enterprise-editor": "p2_editor_mfa_last",
	} {
		path := filepath.Join(artifactDir, ".argus-"+audience+"-totp-last-code")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read Playwright %s MFA state: %w", audience, err)
		}
		if code := strings.TrimSpace(string(data)); code != "" {
			env.State.Values[stateKey] = code
		}
	}
	return nil
}

func waitForNextTOTP(secret, previous string) (string, error) {
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		now := time.Now()
		code, err := generateTOTP(secret, now)
		if err != nil {
			return "", err
		}
		secondsRemaining := 30 - now.Unix()%30
		if code != previous && secondsRemaining >= 5 {
			return code, nil
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("TOTP period did not advance")
}
