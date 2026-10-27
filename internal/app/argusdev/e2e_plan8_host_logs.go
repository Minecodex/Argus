package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
)

func plan8DeclaredServiceBody(applicationID uuid.UUID) map[string]any {
	return map[string]any{"kind": "service", "operation": "create", "service": monitoring.ApplicationServiceInput{ApplicationID: applicationID, Namespace: "acceptance", Name: "file-api"}}
}

func (a *App) plan8WaitScopedLog(ctx context.Context, env *E2EEnvironment, resource uuid.UUID, marker string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	for {
		value, err := client.JSON(bounded, "plan8-log-query-"+marker, "enterprise", http.MethodPost, "/enterprise/logs/query", 200, map[string]any{"query": "body : " + strconv.Quote(marker), "resource_ids": []string{resource.String()}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		matchErr := plan8ScopedLogMatches(value, resource, marker)
		if matchErr == nil {
			return value, nil
		}
		if err = waitContext(bounded, time.Second); err != nil {
			return nil, fmt.Errorf("actual scoped log verification failed: %w", matchErr)
		}
	}
}

func (a *App) plan8DeployedHostLogs(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if _, err := a.plan6InspectDocker(ctx, h.Docker.Host, h.Docker.Owner); err != nil {
		return err
	}
	collector, err := uuid.Parse(h.CollectorID)
	if err != nil {
		return err
	}
	host, err := uuid.Parse(h.HostID)
	if err != nil {
		return err
	}
	app, err := a.plan8ServiceCommit(ctx, env, "file-log-application", map[string]any{"kind": "application", "operation": "create", "application": map[string]any{"name": "Plan8 file log application", "environment": "testing"}})
	if err != nil {
		return err
	}
	service, err := a.plan8ServiceCommit(ctx, env, "file-log-service", plan8DeclaredServiceBody(app))
	if err != nil {
		return err
	}
	directory := "/tmp/argus-plan8-logs-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "mkdir", "-p", directory); err != nil {
		return err
	}
	path := directory + "/application.log"
	write := func(marker string) error {
		body, _ := json.Marshal(map[string]string{"message": marker})
		return a.runner.RunIO(ctx, nil, strings.NewReader(string(body)+"\n"), io.Discard, io.Discard, "docker", "exec", "-i", h.Docker.Host, "sh", "-c", "cat >> \"$1\"", "plan8-log-write", path)
	}
	initial := "plan8-file-initial-" + uuid.NewString()
	if err = write(initial); err != nil {
		return err
	}
	fileID, err := a.plan8ServiceCommit(ctx, env, "file-log-rule", map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": "Actual application JSON files", "integration": "filelog", "target": map[string]any{"kind": "application", "id": app}, "collector_id": collector, "application_id": app, "service_id": service, "enabled": true, "credential_update": "keep", "config": map[string]any{"scope": "business", "include": []string{directory + "/*.log"}, "start_at": "beginning", "format": "json"}}})
	if err != nil {
		return err
	}
	row, err := a.plan8ServiceEffective(ctx, env, fileID, collector, true, nil)
	if err != nil {
		return err
	}
	first, err := a.plan8WaitScopedLog(ctx, env, app, initial)
	if err != nil {
		return err
	}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "mv", path, path+".1"); err != nil {
		return err
	}
	rotated := "plan8-file-rotated-" + uuid.NewString()
	if err = write(rotated); err != nil {
		return err
	}
	second, err := a.plan8WaitScopedLog(ctx, env, app, rotated)
	if err != nil {
		return err
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "file-log-disable", row, collector, false)
	if err != nil {
		return err
	}
	resumed := "plan8-file-resumed-" + uuid.NewString()
	if err = write(resumed); err != nil {
		return err
	}
	row, err = a.plan8ServiceUpdate(ctx, env, "file-log-resume", row, collector, true)
	if err != nil {
		return err
	}
	if _, err = a.plan8WaitScopedLog(ctx, env, app, rotated); err != nil {
		return err
	}
	third, err := a.plan8WaitScopedLog(ctx, env, app, resumed)
	if err != nil {
		return err
	}
	row, err = a.plan8StopCollectionAcrossGatewayLoss(ctx, env, h, row)
	if err != nil {
		return err
	}
	unit := "argus-plan8-journal-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	journalID, err := a.plan8ServiceCommit(ctx, env, "journal-rule", map[string]any{"kind": "collection", "operation": "create", "collection": map[string]any{"name": "Actual system journal unit", "integration": "journald", "target": map[string]any{"kind": "host", "id": host}, "collector_id": collector, "enabled": true, "credential_update": "keep", "config": map[string]any{"scope": "system", "units": []string{unit}, "start_at": "beginning"}}})
	if err != nil {
		return err
	}
	if _, err = a.waitP4HostCollector(ctx, env, h.HostID, "converged"); err != nil {
		return err
	}
	journalMarker := "plan8-journal-" + uuid.NewString()
	// A minimal enrolled host can manage installed units through systemctl's
	// private manager socket without running a separate system D-Bus daemon.
	unitBody := "[Unit]\nDescription=Argus Plan8 owned journal fixture\n[Service]\nType=oneshot\nExecStart=/bin/echo " + journalMarker + "\n"
	unitPath := "/etc/systemd/system/" + unit + ".service"
	if err = a.runner.RunIO(ctx, nil, strings.NewReader(unitBody), io.Discard, io.Discard, "docker", "exec", "-i", h.Docker.Host, "sh", "-c", "cat > \"$1\"", "plan8-journal-write", unitPath); err != nil {
		return err
	}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "exec", h.Docker.Host, "systemctl", "start", unit+".service"); err != nil {
		return fmt.Errorf("actual bounded journal unit failed: %w", err)
	}
	journal, err := a.plan8ServiceEffective(ctx, env, journalID, collector, true, nil)
	if err != nil {
		return err
	}
	system, err := a.plan8WaitScopedLog(ctx, env, host, journalMarker)
	if err != nil {
		return err
	}
	journal, err = a.plan8ServiceUpdate(ctx, env, "journal-final-stop", journal, collector, false)
	if err != nil {
		return err
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "enrolled_linux_collector_actual_application_file_rotation_checkpoint_and_system_journal", "application_id": app, "service_id": service, "file_rule": row, "journal_rule": journal, "initial": first, "rotated": second, "resumed": third, "system_journal": system}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-host-logs.json"), proof)
}
