package argusdev

import (
	"strings"
	"testing"
)

func TestPlan6CandidatesRejectPrerequisiteTestBuild(t *testing.T) {
	for _, build := range []string{"", "m4e2e", "replay"} {
		t.Run(build, func(t *testing.T) {
			env := &E2EEnvironment{State: NewScenarioState("owned-test-run")}
			env.State.Values["p6_backend_build"] = build
			// Rejection precedes any filesystem, registry or cluster action.
			if err := (&App{}).plan6Images(t.Context(), env, "127.0.0.1:5001/argus"); err == nil || !strings.Contains(err.Error(), "production backend") {
				t.Fatal("prerequisite build accepted as a release candidate", err)
			}
		})
	}
}

func TestPlan6ProductionCoverageRequiresTelemetryInItsActualNamespace(t *testing.T) {
	for _, observability := range []string{"system", "observability"} {
		t.Run(observability, func(t *testing.T) {
			env := &E2EEnvironment{SystemNS: "system", ObservNS: observability}
			changed := []plan6ProductionWorkload{}
			for _, name := range []string{"argus-server", "argus-worker", "argus-connector-gateway", "argus-direct-executor", "argus-pki-controller"} {
				changed = append(changed, plan6ProductionWorkload{Namespace: env.SystemNS, Name: name})
			}
			if err := validatePlan6ProductionCoverage(env, changed); err == nil {
				t.Fatal("system-only switch accepted without telemetry")
			}
			for _, name := range []string{"argus-telemetry-ingest", "argus-telemetry-writer", "argus-telemetry-query"} {
				changed = append(changed, plan6ProductionWorkload{Namespace: env.ObservNS, Name: name})
			}
			if err := validatePlan6ProductionCoverage(env, changed); err != nil {
				t.Fatal(err)
			}
			changed[len(changed)-1].Namespace = "unrelated"
			if err := validatePlan6ProductionCoverage(env, changed); err == nil {
				t.Fatal("query switch in a different namespace accepted")
			}
		})
	}
}

func TestPlan6ProductionSwitchRejectsUnpreparedMetrics(t *testing.T) {
	env := &E2EEnvironment{State: NewScenarioState("owned-run")}
	if err := (&App{}).usePlan6ProductionBackend(t.Context(), env); err == nil || !strings.Contains(err.Error(), "self-monitoring prerequisite") {
		t.Fatal("unprepared metrics allowed a production switch", err)
	}
}

func TestPlan6HTTPCollectorFixtureIsLimitedToItsOwnedHost(t *testing.T) {
	env := &E2EEnvironment{Options: E2EOptions{Suite: "plan6"}, State: NewScenarioState("owned-run")}
	env.State.Values["m7_host_id"] = "owned-host"
	env.State.Values["p6_identity_host_id"] = "owned-host"
	if fixture, err := plan6HTTPCollectorFixture(env, "host", "owned-host", "install", []string{"p6-identity"}); fixture || err != nil {
		t.Fatal("legacy Replay coverage was bypassed", fixture, err)
	}
	env.State.Values["p6_backend_build"], env.State.Values["p6_selfmonitor_ready"] = "production", "true"
	for _, action := range []string{"install", "uninstall"} {
		if fixture, err := plan6HTTPCollectorFixture(env, "host", "owned-host", action, []string{"p6-identity"}); !fixture || err != nil {
			t.Fatal("owned real HTTP fixture rejected", fixture, err)
		}
	}
	for _, tc := range []struct{ resourceType, resourceID, action, prefix string }{
		{"host", "unrelated-host", "install", "p6-identity"},
		{"kubernetes-cluster", "owned-host", "install", "p6-identity"},
		{"host", "owned-host", "repair", "p6-identity"},
		{"host", "owned-host", "install", "m7"},
	} {
		if _, err := plan6HTTPCollectorFixture(env, tc.resourceType, tc.resourceID, tc.action, []string{tc.prefix}); err == nil {
			t.Fatal("unrelated production fixture accepted", tc)
		}
	}
}
