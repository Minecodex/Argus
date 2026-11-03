package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
)

func (a *App) preparePlan8MySQLTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (plan8ExtraTarget, error) {
	return a.preparePlan8SingleDatabaseTarget(ctx, env, host, "mysql")
}
func (a *App) preparePlan8SQLServerTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (plan8ExtraTarget, error) {
	return a.preparePlan8SingleDatabaseTarget(ctx, env, host, "sqlserver")
}

// Reuse the existing private credential/least-privilege/owned lifecycle rather
// than maintaining a second SQL Server creation path for discovery acceptance.
func (a *App) preparePlan8SingleDatabaseTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, engine string) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) || engine != "mysql" && engine != "sqlserver" {
		return row, errors.New("owned database target environment missing")
	}
	rows, err := a.preparePlan8DatabaseSet(ctx, env, host, "dedicated-"+engine, []string{engine})
	if len(rows) > 0 {
		row.plan8DatabaseFixture = rows[0]
	}
	if err != nil {
		return row, err
	}
	if len(rows) != 1 {
		return row, errors.New("single database fixture cohort invalid")
	}
	port := "3306"
	row.metric = "mysql_uptime"
	if engine == "sqlserver" {
		port = "1433"
		row.metric = "sqlserver_user_connection_count"
	}
	row.endpoint = row.name + ":" + port
	row.config = map[string]any{"interval": "5s", "timeout": "5s", "tls": map[string]any{"enabled": false}}
	proof, err := json.MarshalIndent(map[string]any{"owner": row.owner, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version, "monitoring_login_verified": true, "database_creation_explicitly_denied": true}, "", "  ")
	if err == nil {
		err = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-"+engine+"-ownership.json"), proof)
	}
	return row, err
}
