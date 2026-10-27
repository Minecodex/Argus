package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type plan8DatabaseFixture struct {
	owner, name, id, engine, password string
	imageID, version                  string
}

// Passwords travel through private env files/stdin, never Docker arguments or
// evidence. Targets are attached only to this acceptance run's customer network.
func (a *App) preparePlan8Databases(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) ([]plan8DatabaseFixture, error) {
	return a.preparePlan8DatabaseSet(ctx, env, h, "services", []string{"mysql", "mysql", "sqlserver"})
}

func (a *App) preparePlan8DatabaseSet(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, suffix string, engines []string) ([]plan8DatabaseFixture, error) {
	network := ""
	for _, row := range h.Docker.Networks {
		if row.Internal {
			network = row.Name
		}
	}
	if network == "" {
		return nil, errors.New("owned customer network is missing")
	}
	owner := "plan8-" + suffix + "-" + env.Options.RunID
	rows := []plan8DatabaseFixture{}
	// Persist ownership after every create, so partial setup can be recovered.
	save := func() error {
		public := []map[string]string{}
		for _, row := range rows {
			public = append(public, map[string]string{"name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "target_version": row.version})
		}
		body, _ := json.MarshalIndent(map[string]any{"owner": owner, "network": network, "containers": public}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-"+suffix+"-ownership.json"), body)
	}
	for index, engine := range engines {
		rootPassword := "Root-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "!x"
		row := plan8DatabaseFixture{owner: owner, name: kubernetesNameForDev(fmt.Sprintf("%s-%s-%d", owner, engine, index)), engine: engine, password: "Monitor-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "") + "!x"}
		material := "MYSQL_ROOT_PASSWORD=" + rootPassword + "\n"
		image := "mysql:8.4"
		options := []string{"--memory=768m", "--cpus=1"}
		command := []string{"--innodb-buffer-pool-size=64M", "--max-connections=40"}
		if engine == "sqlserver" {
			material = "ACCEPT_EULA=Y\nMSSQL_PID=Developer\nMSSQL_MEMORY_LIMIT_MB=1536\nMSSQL_SA_PASSWORD=" + rootPassword + "\n"
			image = "mcr.microsoft.com/mssql/server:2022-CU18-ubuntu-22.04"
			options = []string{"--memory=2560m", "--cpus=2"}
			command = nil
		}
		privateFile := filepath.Join(env.WorkDir, fmt.Sprintf("plan8-%s-database-%d.env", suffix, index))
		if err := os.WriteFile(privateFile, []byte(material), 0600); err != nil {
			return rows, err
		}
		args := []string{"run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run=" + owner, "--network", network, "--env-file", privateFile}
		args = append(args, options...)
		args = append(args, image)
		args = append(args, command...)
		rows = append(rows, row)
		if err := save(); err != nil {
			return rows, err
		}
		id, err := a.runner.Output(ctx, nil, "docker", args...)
		if err != nil {
			return rows, fmt.Errorf("owned %s target start failed", engine)
		}
		row.id = strings.TrimSpace(id)
		rows[len(rows)-1] = row
		if err := save(); err != nil {
			return rows, err
		}
		imageID, inspectErr := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
		if inspectErr != nil {
			return rows, errors.New("owned target image identity not observable")
		}
		row.imageID = strings.TrimSpace(imageID)
		rows[len(rows)-1] = row
		if err := save(); err != nil {
			return rows, err
		}
		bootstrap := func(sql string) error {
			variables := map[string]string{"MYSQL_PWD": rootPassword}
			cmd := []string{"exec", "-i", "--env", "MYSQL_PWD", row.id, "mysql", "-uroot"}
			if engine == "sqlserver" {
				variables = map[string]string{"SQLCMDPASSWORD": rootPassword}
				cmd = []string{"exec", "-i", "--env", "SQLCMDPASSWORD", row.id, "/opt/mssql-tools18/bin/sqlcmd", "-C", "-b", "-S", "localhost", "-U", "sa"}
			}
			return a.runner.RunIO(ctx, variables, strings.NewReader(sql), io.Discard, io.Discard, "docker", cmd...)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for {
			if bootstrap("SELECT 1;\n") == nil {
				break
			}
			if time.Now().After(deadline) {
				return rows, fmt.Errorf("owned %s target readiness timed out", engine)
			}
			if err := waitContext(ctx, time.Second); err != nil {
				return rows, err
			}
		}
		var version bytes.Buffer
		versionEnv := map[string]string{"MYSQL_PWD": rootPassword}
		versionArgs := []string{"exec", "-i", "--env", "MYSQL_PWD", row.id, "mysql", "-N", "-B", "-uroot", "-e", "SELECT VERSION()"}
		if engine == "sqlserver" {
			versionEnv = map[string]string{"SQLCMDPASSWORD": rootPassword}
			versionArgs = []string{"exec", "-i", "--env", "SQLCMDPASSWORD", row.id, "/opt/mssql-tools18/bin/sqlcmd", "-C", "-b", "-h", "-1", "-W", "-S", "localhost", "-U", "sa", "-Q", "SET NOCOUNT ON; SELECT CONVERT(varchar(128),SERVERPROPERTY('ProductVersion'))"}
		}
		if err := a.runner.RunIO(ctx, versionEnv, nil, &version, io.Discard, "docker", versionArgs...); err != nil {
			return rows, errors.New("owned target runtime version query failed")
		}
		row.version = strings.TrimSpace(version.String())
		if row.version == "" || len(row.version) > 200 {
			return rows, errors.New("owned target runtime version invalid")
		}
		rows[len(rows)-1] = row
		if err := save(); err != nil {
			return rows, err
		}
		statement := "CREATE USER 'monitor'@'%' IDENTIFIED BY '" + row.password + "'; GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'monitor'@'%'; GRANT SELECT ON performance_schema.* TO 'monitor'@'%';\n"
		if engine == "sqlserver" {
			statement = "CREATE DATABASE plan8; CREATE LOGIN monitor WITH PASSWORD='" + row.password + "',CHECK_POLICY=OFF; GRANT VIEW ANY DATABASE TO monitor; GRANT VIEW SERVER PERFORMANCE STATE TO monitor;\n"
		}
		if bootstrap(statement) != nil {
			return rows, fmt.Errorf("owned %s least-privilege account setup failed", engine)
		}
		variables := map[string]string{"MYSQL_PWD": row.password}
		cmd := []string{"exec", "-i", "--env", "MYSQL_PWD", row.id, "mysql", "-umonitor"}
		if engine == "sqlserver" {
			variables = map[string]string{"SQLCMDPASSWORD": row.password}
			cmd = []string{"exec", "-i", "--env", "SQLCMDPASSWORD", row.id, "/opt/mssql-tools18/bin/sqlcmd", "-C", "-b", "-S", "localhost", "-U", "monitor"}
		}
		if a.runner.RunIO(ctx, variables, strings.NewReader("CREATE DATABASE forbidden;\n"), io.Discard, io.Discard, "docker", cmd...) == nil {
			return rows, fmt.Errorf("%s monitoring account can create databases", engine)
		}
	}
	return rows, nil
}

func (a *App) cleanupPlan8Databases(ctx context.Context, rows []plan8DatabaseFixture) error {
	var failures []error
	for _, row := range rows {
		reference := row.id
		if reference == "" {
			reference = row.name
		}
		raw, err := a.runner.Output(ctx, nil, "docker", "inspect", reference)
		if err != nil {
			if row.id == "" {
				remaining, lookupErr := a.runner.Output(ctx, nil, "docker", "ps", "--all", "--filter", "label=io.argus.e2e.run="+row.owner, "--filter", "name=^/"+row.name+"$", "--format", "{{.ID}}")
				if lookupErr == nil && strings.TrimSpace(remaining) == "" {
					continue
				}
			}
			failures = append(failures, errors.New("cannot verify service target ownership"))
			continue
		}
		var values []struct {
			ID     string
			Config struct{ Labels map[string]string }
		}
		if json.Unmarshal([]byte(raw), &values) != nil || len(values) != 1 || (row.id != "" && values[0].ID != row.id) || values[0].Config.Labels["io.argus.e2e.run"] != row.owner {
			failures = append(failures, errors.New("service target ownership changed"))
			continue
		}
		var output bytes.Buffer
		if err := a.runner.RunIO(ctx, nil, nil, &output, &output, "docker", "rm", "--force", "--volumes", values[0].ID); err != nil {
			failures = append(failures, errors.New("owned service target cleanup failed"))
		}
	}
	return errors.Join(failures...)
}
