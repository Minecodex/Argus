package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var errPlan8TargetCleanup = errors.New("owned target cleanup incomplete")

// Each new integration is exercised on the enrolled Host after independent
// model scenarios. Actual API/Collector/Kafka/Writer/Query results are retained;
// component captures alone do not satisfy this deployed stage.
func (a *App) plan8DeployedSupportedTargets(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if host == nil {
		return errors.New("supported target stage requires enrolled Host")
	}
	collector, err := uuid.Parse(host.CollectorID)
	if err != nil {
		return err
	}
	records := []map[string]any{}
	defer func() {
		proof, _ := json.MarshalIndent(map[string]any{"passed": failure == nil, "scope": "formally_enrolled_dedicated_receivers_actual_api_private_credentials_kafka_writer_query_default_panels_authorization_and_lifecycle", "resources": records, "support_matrix_complete": false}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-additional-admissions.json"), proof))
	}()
	one := func(prepare func(context.Context, *E2EEnvironment, *plan6HostLoop) (plan8ExtraTarget, error)) func() ([]plan8ExtraTarget, error) {
		return func() ([]plan8ExtraTarget, error) {
			target, e := prepare(ctx, env, host)
			return []plan8ExtraTarget{target}, e
		}
	}
	groups := []struct {
		name    string
		prepare func() ([]plan8ExtraTarget, error)
	}{
		{"postgresql", one(a.preparePlan8PostgresTarget)},
		{"cache-web-coordination", func() ([]plan8ExtraTarget, error) { return a.preparePlan8LightTargets(ctx, env, host) }},
		{"rabbitmq", one(a.preparePlan8RabbitTarget)},
		{"mongodb", func() ([]plan8ExtraTarget, error) {
			target, e := a.preparePlan8DocumentTarget(ctx, env, host, "mongodb")
			return []plan8ExtraTarget{target}, e
		}},
		{"elasticsearch", func() ([]plan8ExtraTarget, error) {
			target, e := a.preparePlan8DocumentTarget(ctx, env, host, "elasticsearch")
			return []plan8ExtraTarget{target}, e
		}},
		{"oracle", one(a.preparePlan8OracleTarget)},
		{"kafka", one(a.preparePlan8KafkaTarget)},
		{"http-tcp-tls-dns-icmp", func() ([]plan8ExtraTarget, error) {
			probes, e := a.preparePlan8ProbeTargets(ctx, env, host)
			targets := make([]plan8ExtraTarget, 0, len(probes))
			for _, probe := range probes {
				targets = append(targets, probe.Target)
			}
			return targets, e
		}},
	}
	for _, group := range groups {
		if len(env.Options.Plan8Integrations) > 0 {
			selected := false
			for _, name := range env.Options.Plan8Integrations {
				selected = selected || name == group.name
			}
			if !selected {
				continue
			}
		}
		fmt.Fprintf(a.stdout, "Plan8 supported target group %s started\n", group.name)
		started := time.Now().UTC()
		err := func() (problem error) {
			targets, e := group.prepare()
			defer func() {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
				defer cancel()
				if problem != nil {
					// Capture the receiver failure while the exact owned target is
					// still present. End-of-run logs otherwise show cleanup DNS errors.
					if _, err := a.plan6InspectDocker(cleanup, host.Docker.Host, host.Docker.Owner); err == nil {
						var out, diagnosticError bytes.Buffer
						if err = a.runner.RunIO(cleanup, nil, nil, &out, &diagnosticError, "docker", "exec", host.Docker.Host, "journalctl", "-u", "argus-otelcol", "--since", started.Format(time.RFC3339), "--no-pager", "-n", "200", "-o", "short-iso"); err == nil {
							body := redactDiagnostic(out.Bytes())
							for _, target := range targets {
								if target.password != "" {
									body = bytes.ReplaceAll(body, []byte(target.password), []byte("[REDACTED]"))
								}
							}
							problem = errors.Join(problem, writePrivate(filepath.Join(env.Options.Artifacts, "plan8-admission-"+group.name+"-receiver.log"), body))
						}
					}
				}
				owned := []plan8DatabaseFixture{}
				for _, target := range targets {
					if target.owner != "" {
						owned = append(owned, target.plan8DatabaseFixture)
					}
				}
				if err := a.cleanupPlan8Databases(cleanup, owned); err != nil {
					problem = errors.Join(problem, errPlan8TargetCleanup, err)
				}
			}()
			if e != nil {
				return e
			}
			for _, target := range targets {
				record, e := a.plan8VerifyDedicatedTarget(ctx, env, collector, target, true)
				if e != nil {
					return e
				}
				records = append(records, record)
			}
			return nil
		}()
		if err != nil {
			records = append(records, map[string]any{"group": group.name, "passed": false, "error": string(redactDiagnostic([]byte(err.Error())))})
			failure = errors.Join(failure, fmt.Errorf("supported target group %s: %w", group.name, err))
			if errors.Is(err, errPlan8TargetCleanup) || ctx.Err() != nil {
				return failure
			}
			fmt.Fprintf(a.stdout, "Plan8 supported target group %s failed; independent groups continue\n", group.name)
			continue
		}
		fmt.Fprintf(a.stdout, "Plan8 supported target group %s passed\n", group.name)
	}
	return failure
}
