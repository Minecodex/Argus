package argusdev

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) preparePlan8OracleTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) {
		return row, errors.New("owned Oracle environment missing")
	}
	network := ""
	for _, n := range host.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return row, errors.New("owned customer network missing")
	}
	passwordBytes := make([]byte, 16)
	if _, err := rand.Read(passwordBytes); err != nil {
		return row, err
	}
	// The pinned Oracle receiver's 30-byte authentication combination was
	// verified. A 31-byte password succeeds in SQLPlus but fails in this native
	// driver; retain that explicit unsupported combination in the report.
	password := "P8!" + base64.RawURLEncoding.EncodeToString(passwordBytes)
	clear(passwordBytes)
	row.plan8DatabaseFixture = plan8DatabaseFixture{owner: "plan8-oracle-" + env.Options.RunID, name: kubernetesNameForDev("p8-oracle-" + env.Options.RunID), engine: "oracle", password: password}
	row.endpoint, row.database, row.metric = row.name+":1521", "FREEPDB1", "oracledb_sessions_usage"
	rootPassword := "RootP8" + strings.ReplaceAll(uuid.NewString(), "-", "")
	private := filepath.Join(env.WorkDir, "plan8-oracle.env")
	if err := os.WriteFile(private, []byte("ORACLE_PASSWORD="+rootPassword+"\n"), 0600); err != nil {
		return row, err
	}
	save := func() error {
		body, _ := json.MarshalIndent(map[string]any{"owner": row.owner, "network": network, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-oracle-ownership.json"), body)
	}
	if err := save(); err != nil {
		return row, err
	}
	id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run="+row.owner, "--network", network, "--memory=2560m", "--cpus=2", "--env-file", private, "gvenzl/oracle-free:23.5-slim-faststart")
	if err != nil {
		return row, errors.New("owned Oracle start failed")
	}
	row.id = strings.TrimSpace(id)
	if err = save(); err != nil {
		return row, err
	}
	deadline := time.Now().Add(4 * time.Minute)
	for a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", row.id, "healthcheck.sh") != nil {
		if time.Now().After(deadline) {
			return row, errors.New("owned Oracle readiness timed out")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return row, err
		}
	}
	run := func(root bool, sql string, output io.Writer) error {
		setup := "WHENEVER SQLERROR EXIT FAILURE;\nSET HEADING OFF FEEDBACK OFF PAGESIZE 0 VERIFY OFF ECHO OFF\n"
		args := []string{"exec", "-i", row.id, "sqlplus", "-s", "/nolog"}
		if root {
			args = []string{"exec", "-i", row.id, "sqlplus", "-s", "/", "as", "sysdba"}
			setup += "ALTER SESSION SET CONTAINER=FREEPDB1;\n"
		} else {
			setup += "CONNECT monitor/\"" + row.password + "\"@//127.0.0.1:1521/FREEPDB1\n"
		}
		return a.runner.RunIO(ctx, nil, strings.NewReader(setup+sql+"\nEXIT;\n"), output, io.Discard, "docker", args...)
	}
	statements := "CREATE USER monitor IDENTIFIED BY \"" + row.password + "\";\nGRANT CREATE SESSION TO monitor;\n"
	for _, view := range []string{"V_$INSTANCE", "V_$DATABASE", "V_$DATAFILE", "V_$PDBS", "CDB_SERVICES", "V_$SESSION", "V_$SYSSTAT", "V_$RESOURCE_LIMIT", "V_$OSSTAT", "DBA_TABLESPACES", "DBA_DATA_FILES", "DBA_TABLESPACE_USAGE_METRICS", "V_$SGAINFO"} {
		statements += "GRANT SELECT ON SYS." + view + " TO monitor;\n"
	}
	if err = run(true, statements, io.Discard); err != nil {
		return row, errors.New("Oracle monitoring role setup failed")
	}
	var version bytes.Buffer
	if err = run(false, "SELECT VERSION_FULL FROM V$INSTANCE;", &version); err != nil {
		return row, errors.New("Oracle monitoring login or actual server version unavailable")
	}
	row.version = strings.TrimSpace(version.String())
	if row.version == "" || len(row.version) > 200 || strings.Contains(row.version, row.password) || strings.Contains(row.version, rootPassword) {
		return row, errors.New("Oracle runtime version missing or invalid")
	}
	var denied bytes.Buffer
	if err = run(false, "CREATE TABLE forbidden (id NUMBER);", &denied); err == nil || !strings.Contains(denied.String(), "ORA-01031") {
		return row, errors.New("Oracle did not explicitly deny monitor table creation")
	}
	image, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(image), "sha256:") {
		return row, errors.New("Oracle immutable image identity missing")
	}
	row.imageID = strings.TrimSpace(image)
	return row, save()
}
