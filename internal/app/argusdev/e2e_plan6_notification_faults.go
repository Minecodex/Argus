package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func (a *App) plan6Command(ctx context.Context, env *E2EEnvironment, key, operation, id string, version int64) (string, error) {
	client, _ := scenarioHTTP(env)
	preview, err := client.JSON(ctx, "p6-command-"+key, "enterprise", http.MethodPost, "/alerts/commands/preview", 202, map[string]any{"operation": operation, "id": id, "expected_version": version}, enterpriseHeaders(env, "p6-command-"+key))
	if err != nil {
		return "", err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return "", err
	}
	if _, err = a.confirmPendingAction(ctx, env, "p6-command-confirm-"+key, ref); err != nil {
		return "", err
	}
	result, err := a.postgresQuery(ctx, env, "SELECT result_resource_id FROM pending_actions WHERE action_ref='"+ref+"';")
	result = strings.TrimSpace(result)
	if err != nil || uuid.Validate(result) != nil {
		return "", fmt.Errorf("notification command has no durable result: %v", err)
	}
	return result, nil
}

func (a *App) verifyPlan6NotificationFaults(ctx context.Context, env *E2EEnvironment) (failure error) {
	client, _ := scenarioHTTP(env)
	rows := []map[string]any{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"passed": failure == nil, "cases": rows, "scope": "real TLS HTTP/SMTP adapters, delivery attempts, Unknown and controlled resend"}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-notification-faults.json"), raw))
	}()
	host := "argus-replay-model." + env.ReplayNamespace() + ".svc"
	cases := []struct {
		name, provider, fault, terminal string
		attempts                        int
	}{
		{"http-429", "feishu", "429", "accepted", 2},
		{"http-500", "feishu", "500", "accepted", 2},
		{"http-400", "feishu", "400", "failed", 1},
		{"business-rejected", "feishu", "business", "failed", 1},
		{"http-timeout", "feishu", "timeout", "unknown", 1},
		{"smtp-temporary", "smtp", "temporary", "accepted", 2},
		{"smtp-permanent", "smtp", "permanent", "failed", 1},
		{"smtp-unconfirmed-data", "smtp", "unknown", "unknown", 1},
		{"smtp-after-send-crash", "smtp", "hold", "unknown", 1},
		{"http-before-send-crash", "feishu", "pre-send", "accepted", 2},
	}
	for _, tc := range cases {
		caseID := uuid.NewString()
		credential := map[string]any{"webhook_url": "https://" + host + "/fixtures/notifications/" + tc.provider + "?fault=" + tc.fault + "&failures=1&case=" + caseID, "signing_secret": "plan6-fixture-signing"}
		if tc.fault == "pre-send" {
			credential["webhook_url"] = "https://" + host + "/fixtures/notifications/" + tc.provider + "?case=" + caseID
		}
		if tc.provider == "smtp" {
			credential = map[string]any{"smtp_host": host, "smtp_port": 465, "security": "tls", "username": "plan6-fault:" + tc.fault + ":" + caseID, "password": "plan6-fixture-password", "from": "alerts@example.test", "to": []string{"ops@example.test"}}
		}
		encoded, _ := json.Marshal(credential)
		secret, err := client.JSON(ctx, "p6-fault-secret-"+tc.name, "enterprise", http.MethodPost, "/enterprise/secrets", 201, map[string]any{"name": "Plan6 fault " + tc.name, "type": "notification", "value": string(encoded)}, enterpriseHeaders(env, "p6-fault-secret-"+tc.name))
		if err != nil {
			return err
		}
		secretID, _ := stringField(secret, "id")
		cred, err := client.JSON(ctx, "p6-fault-credential-"+tc.name, "enterprise", http.MethodPost, "/enterprise/credentials", 201, map[string]any{"name": "Plan6 fault " + tc.name, "protocol": "notification", "secret_id": secretID}, enterpriseHeaders(env, "p6-fault-credential-"+tc.name))
		if err != nil {
			return err
		}
		credentialID, _ := stringField(cred, "id")
		channel, err := a.plan6Config(ctx, env, "fault-channel-"+tc.name, map[string]any{"kind": "channel", "operation": "create", "name": "Plan6 fault " + tc.name, "spec": map[string]any{"enabled": true, "provider": tc.provider, "credential_id": credentialID, "display_address": "owned protocol fault receiver", "mention_ids": []string{}, "mention_all": false, "template": ""}})
		if err != nil {
			return err
		}
		channelID, _ := stringField(channel, "id")
		version, _ := numberField(channel, "version")
		barrierName := ""
		if tc.fault == "pre-send" {
			barrierName = "p6_" + strings.ReplaceAll(caseID, "-", "")
			_, err = a.postgresQuery(ctx, env, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.channel_id='%s'::uuid AND OLD.status='prepared' AND NEW.status='sending' THEN PERFORM pg_sleep(30); END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE UPDATE ON alert_delivery_batches FOR EACH ROW EXECUTE FUNCTION %s();`, barrierName, channelID, barrierName, barrierName))
			if err != nil {
				return err
			}
		}
		deliveryID, err := a.plan6Command(ctx, env, tc.name, "notification.test", channelID, version)
		if err != nil {
			return err
		}
		if tc.fault == "hold" || tc.fault == "pre-send" {
			if err = a.crashPlan6NotificationWorker(ctx, env, channelID, caseID, tc.fault == "hold"); err != nil {
				return err
			}
			if barrierName != "" {
				if _, err = a.postgresQuery(ctx, env, fmt.Sprintf("DROP TRIGGER %s ON alert_delivery_batches; DROP FUNCTION %s();", barrierName, barrierName)); err != nil {
					return err
				}
			}
		}
		terminal, err := a.plan6Wait(ctx, env, "fault-"+tc.name, "/alerts/deliveries/"+deliveryID, func(v map[string]any) bool { d, _ := v["delivery"].(map[string]any); return d["status"] == tc.terminal })
		if err != nil {
			// Keep the failed case and its credential-stage metadata. Neither
			// private credential values nor frozen notification payloads are read.
			fact, diagnosticErr := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('delivery_id',d.id,'delivery_status',d.status,'last_error',d.last_error,'batch_status',b.status,'send_started_at',b.send_started_at,'owner_task_id',b.owner_task_id,'task_status',t.status,'task_error',t.last_error_code,'leases',(SELECT json_agg(json_build_object('id',l.id,'status',l.status,'recipient_id',l.recipient_id,'created_at',l.created_at,'consumed_at',l.consumed_at)) FROM credential_leases l WHERE l.enterprise_id=d.enterprise_id AND l.operation_ref=d.id::text)) FROM alert_deliveries d LEFT JOIN alert_delivery_batches b ON b.id=d.batch_id LEFT JOIN runtime_tasks t ON t.id=b.owner_task_id WHERE d.id='%s';`, deliveryID))
			var diagnostic any
			if diagnosticErr == nil {
				diagnosticErr = json.Unmarshal([]byte(fact), &diagnostic)
			}
			rows = append(rows, map[string]any{"name": tc.name, "case": caseID, "channel_id": channelID, "api": terminal, "passed": false, "error": err.Error(), "diagnostic": diagnostic})
			if diagnosticErr != nil {
				return errors.Join(err, diagnosticErr)
			}
			return err
		}
		fact, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT json_build_object('id',d.id,'status',d.status,'version',d.version,'attempt',d.attempt,'batch_id',d.batch_id,'attempts',(SELECT json_agg(json_build_object('status',a.status,'result_code',a.result_code,'error_code',a.error_code) ORDER BY a.attempt) FROM alert_delivery_attempts a WHERE a.delivery_id=d.id)) FROM alert_deliveries d WHERE id='%s';`, deliveryID))
		if err != nil {
			return err
		}
		var facts map[string]any
		if json.Unmarshal([]byte(fact), &facts) != nil || facts["attempt"] != float64(tc.attempts) {
			return fmt.Errorf("notification fault %s violated retry classification: %s", tc.name, fact)
		}
		row := map[string]any{"name": tc.name, "case": caseID, "channel_id": channelID, "delivery": facts, "api": terminal, "passed": true}
		rows = append(rows, row)
		if tc.terminal == "unknown" {
			if tc.name == "smtp-unconfirmed-data" {
				env.State.Values["p6_unknown_delivery_id"] = deliveryID
			}
			resend, err := a.plan6Command(ctx, env, tc.name+"-resend", "notification.resend", deliveryID, int64(facts["version"].(float64)))
			if err != nil {
				return err
			}
			if resend == deliveryID {
				return fmt.Errorf("controlled resend replaced original unknown delivery")
			}
			if _, err = a.plan6Wait(ctx, env, "resend-"+tc.name, "/alerts/deliveries/"+resend, func(v map[string]any) bool { d, _ := v["delivery"].(map[string]any); return d["status"] == "accepted" }); err != nil {
				return err
			}
			proof, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT count(*) FROM alert_deliveries original JOIN alert_deliveries resend ON resend.resend_of_id=original.id WHERE original.id='%s' AND original.status='unknown' AND original.attempt=1 AND resend.id='%s' AND resend.status='accepted';`, deliveryID, resend))
			if err != nil || strings.TrimSpace(proof) != "1" {
				return fmt.Errorf("resend did not preserve unknown fact: %v", err)
			}
			row["resend_id"] = resend
		}
	}
	return nil
}
