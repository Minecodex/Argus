package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Exercise stale reviews through the same ingress/confirmation API as the
// browser. Only this run's PostgreSQL identities are changed by fault controls.
func (a *App) verifyPlan6StalePreviews(ctx context.Context, env *E2EEnvironment, base map[string]any) (failure error) {
	enterprise, account := env.State.Values["enterprise_id"], env.State.Values["p6_evaluator_id"]
	if uuid.Validate(enterprise) != nil || uuid.Validate(account) != nil {
		return fmt.Errorf("missing owned Plan6 preview fixture identity")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(base)
	spec := map[string]any{}
	if err = json.Unmarshal(raw, &spec); err != nil {
		return err
	}
	spec["enabled"], spec["channel_ids"] = false, []string{}
	name := "Plan6 stale preview policy"
	row, err := a.plan6Config(ctx, env, "stale-policy", map[string]any{"kind": "policy", "operation": "create", "name": name, "spec": spec})
	if err != nil {
		return err
	}
	policy, _ := stringField(row, "id")
	if uuid.Validate(policy) != nil {
		return fmt.Errorf("invalid stale review policy identity")
	}
	cases := []map[string]any{}
	defer func() {
		report, _ := json.MarshalIndent(map[string]any{"passed": failure == nil, "scope": "real ingress API, stale preview confirmation and PostgreSQL rollback", "cases": cases}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-stale-previews.json"), report))
	}()
	for _, kind := range []string{"version-conflict", "evaluator-revoked", "expired"} {
		current, e := client.JSON(ctx, "p6-stale-read-"+kind, "enterprise", http.MethodGet, "/alerts/configurations/"+policy, 200, nil, enterpriseHeaders(env, ""))
		if e != nil {
			return e
		}
		// Creation assigns immutable rule IDs. Updates must use the committed
		// snapshot, never the original ID-less create fixture.
		currentSpec, ok := current["spec"].(map[string]any)
		if !ok {
			return fmt.Errorf("stale review fixture has no committed policy spec")
		}
		input := map[string]any{"kind": "policy", "operation": "update", "id": policy, "name": name, "expected_version": current["version"], "spec": currentSpec}
		preview, e := client.JSON(ctx, "p6-stale-preview-"+kind, "enterprise", http.MethodPost, "/alerts/configurations/preview", 202, input, enterpriseHeaders(env, "p6-stale-preview-"+kind))
		if e != nil {
			return e
		}
		ref, e := stringField(preview, "action_ref")
		if e != nil || !strings.HasPrefix(ref, "act_") || strings.ContainsAny(ref, "'\\;") {
			return fmt.Errorf("invalid owned preview reference: %v", e)
		}
		restore := func(context.Context) error { return nil }
		switch kind {
		case "version-conflict":
			changed := map[string]any{}
			b, _ := json.Marshal(currentSpec)
			_ = json.Unmarshal(b, &changed)
			changed["description"] = "a later confirmed review"
			update := map[string]any{"kind": "policy", "operation": "update", "id": policy, "name": name, "expected_version": current["version"], "spec": changed}
			if _, e = a.plan6Config(ctx, env, "stale-conflicting-update", update); e != nil {
				return e
			}
		case "evaluator-revoked":
			restore = func(c context.Context) error {
				_, e := a.postgresQuery(c, env, fmt.Sprintf("UPDATE service_accounts SET status='active',authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';", account, enterprise))
				return e
			}
			defer func() {
				c, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
				defer cancel()
				failure = errors.Join(failure, restore(c))
			}()
			_, e = a.postgresQuery(ctx, env, fmt.Sprintf("UPDATE service_accounts SET status='disabled',authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';", account, enterprise))
		case "expired":
			_, e = a.postgresQuery(ctx, env, "UPDATE pending_actions SET expires_at=now()-interval '1 minute' WHERE action_ref='"+ref+"' AND enterprise_id='"+enterprise+"';")
		}
		if e != nil {
			return e
		}
		before, e := a.plan6ConfigurationFacts(ctx, env)
		if e != nil {
			return e
		}
		denied, e := client.JSON(ctx, "p6-stale-confirm-"+kind, "enterprise", http.MethodPost, "/enterprise/pending-actions/"+ref+"/confirm", 409, nil, enterpriseHeaders(env, "p6-stale-confirm-"+kind))
		if e != nil || denied["code"] != "PENDING_ACTION_INVALIDATED" {
			return fmt.Errorf("stale %s was not rejected: %v %v", kind, denied, e)
		}
		after, e := a.plan6ConfigurationFacts(ctx, env)
		if e != nil || after != before {
			return fmt.Errorf("stale %s changed configuration: %v", kind, e)
		}
		fact, e := a.postgresQuery(ctx, env, "SELECT json_build_object('status',p.status,'token_status',t.status,'confirmations',(SELECT count(*) FROM user_confirmations WHERE pending_action_id=p.id),'executions',(SELECT count(*) FROM executions WHERE pending_action_id=p.id)) FROM pending_actions p JOIN pending_action_tokens t ON t.pending_action_id=p.id WHERE p.action_ref='"+ref+"' AND p.enterprise_id='"+enterprise+"';")
		if e != nil {
			return e
		}
		var state struct {
			TokenStatus               string `json:"token_status"`
			Confirmations, Executions int
		}
		if json.Unmarshal([]byte(fact), &state) != nil || state.TokenStatus != "active" || state.Confirmations != 0 || state.Executions != 0 {
			return fmt.Errorf("stale %s consumed confirmation/token: %s", kind, fact)
		}
		cases = append(cases, map[string]any{"case": kind, "passed": true, "action_ref": ref, "response": denied, "database": json.RawMessage(strings.TrimSpace(fact))})
		if e = restore(ctx); e != nil {
			return e
		}
	}
	return nil
}
