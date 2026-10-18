package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func (a *App) plan7Engineering(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	password, err := dataCredentialValue(ctx, env, "postgresql-password")
	if err != nil {
		return err
	}
	forward, err := env.Kube.PortForwardService(ctx, env.SystemNS, "argus-postgresql", []string{"0:5432"}, io.Discard)
	if err != nil {
		return err
	}
	defer forward.Stop()
	connection := url.URL{Scheme: "postgres", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forward.Ports[0].Local))), Path: "argus", User: url.UserPassword("argus", password), RawQuery: "sslmode=disable"}
	password = ""
	admin, err := pgx.Connect(ctx, connection.String())
	if err != nil {
		return fmt.Errorf("connect owned acceptance PostgreSQL: %w", err)
	}
	defer admin.Close(context.Background())
	// A separate database prevents live workers from consuming unit-test tasks.
	database := "plan7_gate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{database}.Sanitize()); err != nil {
		return err
	}
	connection.Path = database
	if err = postgres.RunMigrations(ctx, connection.String(), filepath.Join(env.Root, "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		return err
	}
	testEnv := map[string]string{"ARGUS_P5_TEST_DATABASE_URL": connection.String(), "ARGUS_P7_TEST_DATABASE_URL": connection.String(), "ARGUS_P7_KUBE_LIVE": "0", "ARGUS_P7_STORAGE_LIVE": "0"}
	for _, gate := range []struct {
		name, command string
		args          []string
		variables     map[string]string
	}{
		{"go-test", "go", []string{"test", "./...", "-json", "-p", "1", "-count=1"}, testEnv},
		{"active-compaction", "go", []string{"test", "-tags", "m4e2e", "./internal/agent", "-run", "^TestHardActiveTurnCompactionResumesWithPairedHistoryAndUserConstraints$", "-json", "-count=1"}, testEnv},
		{"replay-protocol", "go", []string{"test", "-tags", "m4e2e", "./cmd/argus-replay-model", "-json", "-count=1"}, nil},
		{"contracts", "go", []string{"run", "./cmd/argus-dev", "contracts", "check"}, map[string]string{"CI": "1", "NO_UPDATE_NOTIFIER": "1"}},
		{"typecheck", "pnpm", []string{"typecheck"}, nil},
		{"web-test", "pnpm", []string{"test"}, nil},
		{"web-lint", "pnpm", []string{"lint"}, nil},
		{"real-build", "pnpm", []string{"build:real"}, nil},
	} {
		if err = a.plan7CommandEvidence(ctx, env, gate.name, gate.variables, gate.command, gate.args...); err != nil {
			return err
		}
	}
	if err = a.plan7PlatformUnitEvidence(ctx, env, host); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "source_fingerprint": env.State.Values["p6_source_fingerprint"], "status": "passed", "test_database": database}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-engineering.json"), raw)
}

func (a *App) plan7CommandEvidence(ctx context.Context, env *E2EEnvironment, name string, variables map[string]string, command string, args ...string) error {
	_, _ = fmt.Fprintln(a.stdout, "Plan7 engineering", name, "started")
	path := filepath.Join(env.Options.Artifacts, "plan7-"+name+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = a.runner.RunIO(ctx, variables, nil, file, file, command, args...)
	err = errors.Join(err, file.Close())
	if err != nil {
		return fmt.Errorf("Plan7 engineering %s failed; see %s: %w", name, path, err)
	}
	_, _ = fmt.Fprintln(a.stdout, "Plan7 engineering", name, "passed")
	return nil
}

func (a *App) plan7PlatformUnitEvidence(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	storage := map[string]string{"ARGUS_P7_STORAGE_LIVE": "1", "ARGUS_P7_STORAGE_RUN_ID": env.Options.RunID, "ARGUS_P7_STORAGE_ARTIFACTS": filepath.Join(env.Options.Artifacts, "storage-live"), "ARGUS_P7_STORAGE_IMAGE": env.State.FixtureImages["remote-runtime"]}
	if err := a.plan7CommandEvidence(ctx, env, "workspace-storage-tests", storage, "go", "test", "./internal/app/argusdev", "-run", "^TestPlan7LiveWorkspaceStorageBeyondEightVolumes$", "-json", "-count=1", "-timeout=8m"); err != nil {
		return err
	}
	helperRoot := filepath.Join(env.WorkDir, "plan7-live-helpers")
	if err := os.MkdirAll(helperRoot, 0700); err != nil {
		return err
	}
	buildEnv := map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}
	if err := a.runner.Run(ctx, buildEnv, "go", "build", "-trimpath", "-o", filepath.Join(helperRoot, "linux-amd64"), "./cmd/argus-remote-runtime"); err != nil {
		return err
	}
	live := map[string]string{"ARGUS_P7_KUBE_LIVE": "1", "ARGUS_P7_KUBE_CONTEXT": env.Options.KubeContext, "ARGUS_P7_REMOTE_IMAGE": env.State.FixtureImages["remote-runtime"], "ARGUS_REMOTE_HELPER_DIRECTORY": helperRoot}
	if err := a.plan7CommandEvidence(ctx, env, "kubernetes-adapter-tests", live, "go", "test", "./internal/remoteexecution/kubernetesruntime", "-run", "TestLive", "-json", "-count=1", "-timeout=10m"); err != nil {
		return err
	}
	for _, pkg := range []string{"runner", "runtimewire"} {
		binary := filepath.Join(env.WorkDir, "plan7-"+pkg+".test")
		if err := a.runner.Run(ctx, buildEnv, "go", "test", "-c", "-o", binary, "./internal/remoteexecution/"+pkg); err != nil {
			return err
		}
		if _, err := a.plan6InspectDocker(ctx, host.Docker.Host, host.Docker.Owner); err != nil {
			return err
		}
		target := "/tmp/plan7-" + pkg + "-" + env.Options.RunID + ".test"
		if err := a.runner.Run(ctx, nil, "docker", "cp", binary, host.Docker.Host+":"+target); err != nil {
			return err
		}
		if err := a.plan7CommandEvidence(ctx, env, "linux-"+pkg+"-tests", nil, "docker", "exec", host.Docker.Host, target, "-test.v", "-test.count=1", "-test.timeout=3m"); err != nil {
			return err
		}
	}
	return nil
}
