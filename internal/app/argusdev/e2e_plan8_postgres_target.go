package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) preparePlan8PostgresTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) {
		return row, errors.New("PostgreSQL acceptance requires its enrolled host")
	}
	network := ""
	for _, candidate := range host.Docker.Networks {
		if candidate.Internal {
			network = candidate.Name
		}
	}
	if network == "" {
		return row, errors.New("owned customer network missing")
	}
	row.plan8DatabaseFixture = plan8DatabaseFixture{owner: "plan8-postgres-" + env.Options.RunID, name: kubernetesNameForDev("p8-postgresql-" + env.Options.RunID), engine: "postgresql", password: "Monitor-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	row.endpoint, row.database, row.metric = row.name+":5432", "plan8", "postgresql_backends"
	rootPassword := "Root-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	private := filepath.Join(env.WorkDir, "plan8-postgresql.env")
	if err := os.WriteFile(private, []byte("POSTGRES_USER=bootstrap\nPOSTGRES_PASSWORD="+rootPassword+"\n"), 0600); err != nil {
		return row, err
	}
	save := func() error {
		body, _ := json.MarshalIndent(map[string]any{"owner": row.owner, "network": network, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-postgresql-ownership.json"), body)
	}
	if err := save(); err != nil {
		return row, err
	}
	id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run="+row.owner, "--network", network, "--memory=384m", "--cpus=1", "--env-file", private, "postgres:17-alpine")
	if err != nil {
		return row, errors.New("owned PostgreSQL start failed")
	}
	row.id = strings.TrimSpace(id)
	if err = save(); err != nil {
		return row, err
	}
	query := func(user, password, sql string, output io.Writer) error {
		return a.runner.RunIO(ctx, map[string]string{"PGPASSWORD": password}, strings.NewReader(sql), output, io.Discard, "docker", "exec", "-i", "--env", "PGPASSWORD", row.id, "psql", "-h", "127.0.0.1", "-U", user, "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-A", "-t")
	}
	deadline := time.Now().Add(time.Minute)
	for query("bootstrap", rootPassword, "SELECT 1;\n", io.Discard) != nil {
		if time.Now().After(deadline) {
			return row, errors.New("owned PostgreSQL readiness timed out")
		}
		if err = waitContext(ctx, 200*time.Millisecond); err != nil {
			return row, err
		}
	}
	statement := "CREATE DATABASE plan8;\nCREATE ROLE monitor LOGIN PASSWORD '" + row.password + "';\nGRANT pg_monitor TO monitor;\n\\connect plan8\nCREATE TABLE samples AS SELECT generate_series(1,1000) AS value;\nSELECT sum(value) FROM samples;\n"
	if err = query("bootstrap", rootPassword, statement, io.Discard); err != nil {
		return row, errors.New("PostgreSQL monitoring role setup failed")
	}
	if err = query("monitor", row.password, "CREATE DATABASE forbidden;\n", io.Discard); err == nil {
		return row, errors.New("PostgreSQL monitoring identity can create databases")
	}
	var version bytes.Buffer
	if err = query("monitor", row.password, "SHOW server_version;\n", &version); err != nil {
		return row, errors.New("PostgreSQL actual runtime version unavailable")
	}
	row.version = strings.TrimSpace(version.String())
	if row.version == "" || len(row.version) > 200 || strings.Contains(row.version, row.password) || strings.Contains(row.version, rootPassword) {
		return row, errors.New("PostgreSQL runtime version invalid")
	}
	image, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(image), "sha256:") {
		return row, errors.New("PostgreSQL target image identity unavailable")
	}
	row.imageID = strings.TrimSpace(image)
	return row, save()
}
