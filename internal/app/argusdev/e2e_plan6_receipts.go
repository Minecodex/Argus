package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Reconcile accepted database batches with durable protocol receiver records.
// A delivery state, HTTP 200, or SMTP connection alone cannot pass this gate.
func (a *App) verifyPlan6Receipts(ctx context.Context, env *E2EEnvironment, policyID string, expectedEvents ...string) error {
	prefix := "plan6-protocol"
	if len(expectedEvents) == 0 {
		expectedEvents = []string{"first", "recovery"}
	} else {
		prefix += "-" + strings.Join(expectedEvents, "-")
	}
	facts, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT COALESCE(json_agg(row_to_json(item)), '[]'::json) FROM (
		SELECT b.id, b.status, b.owner_task_id, b.fence, b.payload, c.spec->>'provider' AS provider,
		(SELECT json_agg(json_build_object('id',d.id,'status',d.status,'event_type',d.payload->>'event_type','attempts',
		(SELECT json_agg(json_build_object('status',t.status,'result_code',t.result_code)) FROM alert_delivery_attempts t WHERE t.delivery_id=d.id)))
		FROM alert_deliveries d WHERE d.batch_id=b.id) AS members
		FROM alert_delivery_batches b JOIN alert_configurations c ON c.id=b.channel_id
		WHERE b.enterprise_id='%s' AND b.status='accepted' AND EXISTS
		(SELECT 1 FROM alert_deliveries d JOIN alert_instances i ON i.id=d.instance_id WHERE d.batch_id=b.id AND i.policy_id='%s')
		ORDER BY b.created_at,b.id) item;`, env.State.Values["enterprise_id"], policyID))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, prefix+"-batches.json"), []byte(strings.TrimSpace(facts)+"\n")); err != nil {
		return err
	}

	forward, err := env.Kube.PortForwardService(ctx, env.ReplayNamespace(), "argus-replay-model", []string{"0:8081"}, io.Discard)
	if err != nil {
		return err
	}
	defer forward.Stop()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/fixtures/notifications/", forward.Ports[0].Local), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil || response.StatusCode != http.StatusOK {
		return fmt.Errorf("Plan6 receiver records unavailable: status %d: %w", response.StatusCode, err)
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, prefix+"-receipts.json"), raw); err != nil {
		return err
	}
	var received struct {
		Items []map[string]any `json:"items"`
	}
	if err = json.Unmarshal(raw, &received); err != nil {
		return err
	}
	var batches []struct {
		ID, Status, Provider string
		OwnerTaskID          string `json:"owner_task_id"`
		Fence                int64  `json:"fence"`
		Payload              struct {
			Text string `json:"text"`
		}
		Members []struct {
			ID, Status string
			EventType  string `json:"event_type"`
			Attempts   []struct {
				Status     string
				ResultCode string `json:"result_code"`
			}
		}
	}
	if err = json.Unmarshal([]byte(strings.TrimSpace(facts)), &batches); err != nil {
		return err
	}
	seen := map[string]map[string]bool{}
	for _, batch := range batches {
		if uuid.Validate(batch.OwnerTaskID) != nil || batch.Fence < 1 {
			return fmt.Errorf("Plan6 batch %s lacks a fenced sending owner", batch.ID)
		}
		matches := []map[string]any{}
		for _, record := range received.Items {
			body, _ := record["body"].(string)
			if record["batch_id"] == batch.ID || batch.Provider == "smtp" && strings.Contains(body, "X-Argus-Notification-Batch: "+batch.ID+"\r\n") {
				matches = append(matches, record)
			}
		}
		if len(matches) != 1 || matches[0]["provider"] != batch.Provider {
			return fmt.Errorf("Plan6 batch %s expected one %s receipt, got %d", batch.ID, batch.Provider, len(matches))
		}
		record := matches[0]
		if batch.Provider == "smtp" {
			body, _ := record["body"].(string)
			to, _ := record["recipients"].([]any)
			if record["from"] != "<alerts@example.test>" || len(to) != 1 || to[0] != "<ops@example.test>" || record["queue_id"] == "" || !strings.Contains(strings.ReplaceAll(body, "\r\n", "\n"), batch.Payload.Text) {
				return fmt.Errorf("Plan6 SMTP envelope/body mismatch for %s", batch.ID)
			}
		} else {
			body, _ := record["body"].(map[string]any)
			encoded, _ := json.Marshal(body)
			if record["signature_valid"] != true || !strings.Contains(string(encoded), "fixture-user") {
				return fmt.Errorf("Plan6 %s signature or mention mismatch for %s", batch.Provider, batch.ID)
			}
			text := ""
			if batch.Provider == "feishu" {
				content, _ := body["content"].(map[string]any)
				text, _ = content["text"].(string)
			} else {
				content, _ := body["text"].(map[string]any)
				text, _ = content["content"].(string)
			}
			if !strings.Contains(text, batch.Payload.Text) {
				return fmt.Errorf("Plan6 webhook payload differs from frozen batch %s", batch.ID)
			}
		}
		if len(batch.Members) == 0 {
			return fmt.Errorf("Plan6 batch %s has no delivery members", batch.ID)
		}
		for _, member := range batch.Members {
			accepted := false
			for _, attempt := range member.Attempts {
				accepted = accepted || attempt.Status == "accepted" && attempt.ResultCode != ""
			}
			if member.Status != "accepted" || !accepted {
				return fmt.Errorf("Plan6 delivery %s lacks accepted protocol attempt", member.ID)
			}
			if seen[batch.Provider] == nil {
				seen[batch.Provider] = map[string]bool{}
			}
			seen[batch.Provider][member.EventType] = true
		}
	}
	for _, provider := range []string{"dingtalk", "wecom", "feishu", "smtp"} {
		for _, event := range expectedEvents {
			if !seen[provider][event] {
				return fmt.Errorf("Plan6 %s missing %s reconciliation", provider, event)
			}
		}
	}
	result, _ := json.MarshalIndent(map[string]any{"accepted_batches": len(batches), "receipt_count": len(received.Items), "reconciled": true, "providers": seen}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, prefix+"-reconciliation.json"), result)
}
