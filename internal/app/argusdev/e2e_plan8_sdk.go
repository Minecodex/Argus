package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func (a *App) plan8DeployedSDK(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if _, err := a.plan6InspectDocker(ctx, h.Docker.Host, h.Docker.Owner); err != nil {
		return err
	}
	executable := filepath.Join(env.WorkDir, "plan8-actual-sdk-linux")
	if err := a.runner.Run(ctx, map[string]string{"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"}, "go", "build", "-trimpath", "-o", executable, "./internal/app/argusdev/fixtures/sdk"); err != nil {
		return err
	}
	remote := "/tmp/argus-plan8-actual-sdk"
	if err := a.runner.Run(ctx, nil, "docker", "cp", executable, h.Docker.Host+":"+remote); err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	cases := []map[string]any{}
	modelKeyDelivery := false
	for _, test := range []struct {
		name, protocol, environment, port string
		signals                           []string
	}{
		{"otlp_http", "otlp", "production", "24318", []string{"traces", "logs", "metrics"}},
		{"otlp_grpc", "otlp", "testing", "24317", []string{"traces"}},
		{"jaeger", "jaeger", "production", "24250", []string{"traces"}},
	} {
		if (env.Options.Plan8Phase == "plan11-metadata" || env.Options.Plan8Phase == "plan11-metrics" || plan11SignalPhase(env.Options.Plan8Phase)) && test.name != "otlp_http" {
			continue
		}
		app, err := a.plan8ServiceCommit(ctx, env, "sdk-app-"+test.name, map[string]any{"kind": "application", "operation": "create", "application": map[string]any{"name": "Plan8 SDK " + test.name, "environment": test.environment}})
		if err != nil {
			return err
		}
		endpoint, err := a.plan8ServiceCommit(ctx, env, "sdk-endpoint-"+test.name, map[string]any{"kind": "endpoint", "operation": "create", "endpoint": map[string]any{"name": "Actual SDK " + test.name, "application_id": app, "collector_id": h.CollectorID, "protocol": test.protocol, "signals": test.signals, "enabled": true}})
		if err != nil {
			return err
		}
		if _, err = a.waitP4HostCollector(ctx, env, h.HostID, "converged"); err != nil {
			return err
		}
		settings, err := client.JSON(ctx, "sdk-connection-"+test.name, "enterprise", http.MethodGet, "/monitoring/ingest-endpoints/"+endpoint.String()+"/connection", 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		encoded, _ := json.Marshal(settings)
		var connection monitoring.IngestConnection
		if json.Unmarshal(encoded, &connection) != nil || connection.ApplicationID != app || connection.CollectorID.String() != h.CollectorID || connection.Environment != test.environment || connection.ConfigurationStatus != "effective" || !connection.TLS.Required || connection.TLS.TrustBundleEpoch < 1 {
			return errors.New("SDK guide does not match the actual application and effective receiver")
		}
		transport := "grpc"
		if test.name == "otlp_http" {
			transport = "http"
		}
		address := ""
		for _, listener := range connection.Listeners {
			if listener.Transport == transport {
				parsed, parseErr := url.Parse(listener.LocalEndpoint)
				if parseErr != nil || parsed.Hostname() != "127.0.0.1" {
					return errors.New("SDK guide has an invalid same-host listener")
				}
				address = parsed.Host
			}
		}
		if address == "" {
			return errors.New("SDK guide omitted the actual tested transport")
		}
		makeKey := func(label string) (uuid.UUID, string, error) {
			id, e := a.plan8ServiceCommit(ctx, env, label, map[string]any{"kind": "ingest_key", "operation": "create", "ingest_key": map[string]any{"endpoint_id": endpoint, "expires_at": time.Now().UTC().Add(time.Hour)}})
			if e != nil {
				return uuid.Nil, "", e
			}
			value, e := client.JSON(ctx, label+"-reveal", "enterprise", http.MethodPost, "/monitoring/ingest-keys/"+id.String()+"/reveal", 200, nil, enterpriseHeaders(env, ""))
			if e != nil {
				return uuid.Nil, "", e
			}
			key, e := stringField(value, "key")
			return id, key, e
		}
		var keyID uuid.UUID
		var key string
		if plan8SDKModelKeySelected(env.Options.Plan8Phase, test.name) {
			keyID, key, err = a.plan8ModelSDKKey(ctx, env, app, endpoint)
			modelKeyDelivery = err == nil
		} else {
			keyID, key, err = makeKey("sdk-key-" + test.name)
		}
		if err != nil {
			return err
		}
		send := func(label, token string, accept bool) (map[string]any, error) {
			// Private data is streamed only to the explicitly owned SDK process. It
			// does not enter Docker arguments, files, model messages or evidence.
			input := map[string]any{"Endpoint": address, "Key": token, "CA": connection.TLS.CABundlePEM, "ServerName": connection.TLS.ServerName, "Protocol": test.name, "Service": "api", "Namespace": "orders", "Instance": "replica-one", "Marker": "plan8-sdk-" + test.name + "-" + label, "ExpectedAccept": accept}
			input["MetricSemantics"] = env.Options.Plan8Phase == "plan11-metrics" || plan11SignalPhase(env.Options.Plan8Phase)
			input["SignalAcceptance"] = plan11SignalPhase(env.Options.Plan8Phase)
			raw, _ := json.Marshal(input)
			defer clear(raw)
			var output bytes.Buffer
			e := a.runner.RunIO(ctx, nil, bytes.NewReader(raw), &output, io.Discard, "docker", "exec", "-i", h.Docker.Host, remote)
			if e != nil {
				return nil, errors.New("owned SDK process did not meet the expected transport outcome")
			}
			result := map[string]any{}
			if json.Unmarshal(output.Bytes(), &result) != nil || result["accepted"] != accept || result["expectation_met"] != true {
				return nil, errors.New("invalid actual SDK transport evidence")
			}
			if accept && (env.Options.Plan8Phase == "plan11-metrics" || plan11SignalPhase(env.Options.Plan8Phase)) && result["metric_semantics_generated"] != true {
				return nil, errors.New("actual SDK omitted the declared metric kinds")
			}

			if accept && plan11SignalPhase(env.Options.Plan8Phase) && result["signal_acceptance_generated"] != true {
				return nil, errors.New("actual SDK omitted the explicit native/injection fixture observations")
			}
			return result, nil
		}
		initial, err := send("initial", key, true)
		if err != nil {
			return err
		}
		appSQL := fmt.Sprintf("SELECT count(*) FROM monitoring_application_services WHERE enterprise_id='%s' AND application_id='%s' AND service_name='api' AND service_namespace='orders'", env.State.Values["enterprise_id"], app)
		if err = a.waitPostgresValue(ctx, env, appSQL, "1", time.Minute); err != nil {
			return err
		}
		if _, err = a.plan8ServiceCommit(ctx, env, "sdk-revoke-"+test.name, map[string]any{"kind": "ingest_key", "operation": "revoke", "id": keyID, "expected_version": 1, "ingest_key": map[string]any{}}); err != nil {
			return err
		}
		if _, err = send("revoked", key, false); err != nil {
			return err
		}
		_, replacement, err := makeKey("sdk-rotated-" + test.name)
		if err != nil {
			return err
		}
		rotated, err := send("rotated", replacement, true)
		if err != nil {
			return err
		}
		if err = a.waitPostgresValue(ctx, env, appSQL, "1", time.Minute); err != nil {
			return err
		}
		row, err := a.plan8WaitSDKAPM(ctx, env, app, test.name)
		if err != nil {
			return err
		}
		if test.name == "otlp_http" {
			if err = a.plan8WaitSDKLogs(ctx, env, app, "plan8-sdk-otlp_http-initial"); err != nil {
				return err
			}
			if err = a.waitM7Metric(ctx, env, app.String(), "argus_plan8_sdk_requests"); err != nil {
				return err
			}
			if env.Options.Plan8Phase == "plan11-metrics" || plan11SignalPhase(env.Options.Plan8Phase) {
				for _, name := range []string{"argus_plan11_sdk_completed", "argus_plan11_sdk_queue", "argus_plan11_sdk_latency_count"} {
					if err = a.waitM7Metric(ctx, env, app.String(), name); err != nil {
						return err
					}
				}
			}
		}
		if _, err = a.plan8ServiceCommit(ctx, env, "sdk-disable-"+test.name, map[string]any{"kind": "endpoint", "operation": "set_enabled", "id": endpoint, "expected_version": 1, "endpoint": map[string]any{"enabled": false}}); err != nil {
			return err
		}
		if _, err = a.waitP4HostCollector(ctx, env, h.HostID, "converged"); err != nil {
			return err
		}
		// Keep only non-secret observed identities. Credentials are never copied
		// into this proof, even for the negative/revoked case.
		cases = append(cases, map[string]any{"protocol": test.name, "application_id": app, "environment": test.environment, "endpoint_id": endpoint, "initial": initial, "rotated": rotated, "apm": row, "key_revoked_rejected": true, "stable_service": true})
		if env.Options.Plan8Phase == "plan11-metadata" {
			env.State.Values["p11_metadata_application_id"] = app.String()
			env.State.Values["p11_metadata_endpoint_id"] = endpoint.String()
		}
		if env.Options.Plan8Phase == "plan11-metrics" {
			env.State.Values["p11_metric_application_id"] = app.String()
		}
		if plan11SignalPhase(env.Options.Plan8Phase) {
			env.State.Values["p11_signal_application_id"] = app.String()
			env.State.Values["p11_signal_endpoint_id"] = endpoint.String()
			if err = a.plan11WaitNativeSDKHistogram(ctx, env, app.String()); err != nil {
				return err
			}
		}

		key = ""
		replacement = ""
	}
	if err = a.plan8FreshHostSample(ctx, env, h); err != nil {
		return err
	}
	if _, err = a.plan6HostMetric(ctx, env, h, "after-sdk-endpoints-off", h.CPUExpression); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(map[string]any{"passed": true, "enrollment_fixture": false, "cases": cases, "sdk_key_in_model_or_argv": false, "real_model_key_delivery_tested": modelKeyDelivery}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-sdk.json"), raw)
}

func (a *App) plan8WaitSDKAPM(ctx context.Context, env *E2EEnvironment, app uuid.UUID, protocol string) (map[string]any, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(time.Minute)
	query, err := skywalking.ApplicationAPMQuery("services", 10)
	if err != nil {
		return nil, err
	}
	for {
		value, err := client.JSON(ctx, "sdk-services-"+protocol, "enterprise", http.MethodPost, "/enterprise/traces/graphql", 200, map[string]any{"query": query, "operation_name": "ApplicationAPM", "variables": map[string]any{"serviceIds": nil}, "resource_ids": []string{app.String()}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		data, ok := value["data"].(map[string]any)
		if !ok {
			return nil, errors.New("actual application APM data missing")
		}
		services, ok := data["queryAPMServices"].(map[string]any)
		if !ok {
			return nil, errors.New("actual application service view missing")
		}
		rows, ok := services["rows"].([]any)
		if !ok || len(rows) > 1 {
			return nil, errors.New("actual application APM service identity mismatch")
		}
		if len(rows) == 1 {
			row, ok := rows[0].(map[string]any)
			if !ok || row["serviceName"] != "api" || row["serviceNamespace"] != "orders" {
				return nil, errors.New("actual protocol service view lost SDK identity")
			}
			if count, ok := row["sampleCount"].(float64); ok && count == 2 {
				return row, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, errors.New("two actual native protocol requests did not reach the application APM view")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return nil, err
		}
	}
}

func (a *App) plan8WaitSDKLogs(ctx context.Context, env *E2EEnvironment, app uuid.UUID, marker string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Minute)
	for attempt := 1; ; attempt++ {
		value, err := client.JSON(ctx, "sdk-logs-"+marker, "enterprise", http.MethodPost, "/enterprise/logs/query", 200, map[string]any{"query": "body : " + strconv.Quote(marker), "resource_ids": []string{app.String()}, "time_range": telemetryTimeRange(5 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if plan11SignalPhase(env.Options.Plan8Phase) {
			raw, marshalErr := json.MarshalIndent(value, "", "  ")
			if marshalErr != nil {
				return marshalErr
			}
			if e := writePrivate(filepath.Join(env.Options.Artifacts, fmt.Sprintf("plan11-sdk-log-%s-attempt-%02d.json", marker, attempt)), raw); e != nil {
				return e
			}
		}
		if assertKQLResponse(value, marker, 1) == nil {
			rows := value["data"].([]any)
			row, _ := rows[0].(map[string]any)
			if row["resource_id"] != app.String() {
				return errors.New("SDK log escaped its trusted application")
			}
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("actual SDK business log did not reach the application query")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
}
