package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/api/schemas"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Tools struct{ Service Service }

func (t Tools) actor(ctx context.Context, call mcp.Call) (Actor, error) {
	enterprise, err := uuid.Parse(call.Enterprise)
	if err != nil {
		return Actor{}, ErrDenied
	}
	id, err := uuid.Parse(call.Subject)
	if err != nil {
		return Actor{}, ErrDenied
	}
	actor := Actor{EnterpriseID: enterprise, SubjectID: id, SubjectType: call.SubjectType}
	if call.SubjectType == "service_account" {
		account, e := t.Service.Store.Queries.GetServiceAccount(ctx, db.GetServiceAccountParams{EnterpriseID: enterprise, ID: id})
		if e != nil || account.Status != "active" || !slices.Contains(account.AllowedToolIds, call.ToolID) {
			return actor, ErrDenied
		}
		actor.AuthorizationVersion = account.AuthorizationVersion
	} else {
		user, e := t.Service.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: enterprise, ID: id})
		if e != nil || user.Status != "active" {
			return actor, ErrDenied
		}
		actor.SubjectType = "user"
		actor.AuthorizationVersion = user.AuthorizationVersion
	}
	if call.RunID != "" {
		actor.RunID, _ = uuid.Parse(call.RunID)
	}
	return actor, nil
}
func toolInput[T any](call mcp.Call) (T, error) {
	var value T
	data, err := json.Marshal(call.Input)
	if err != nil {
		return value, ErrInvalid
	}
	if json.Unmarshal(data, &value) != nil {
		return value, ErrInvalid
	}
	return value, nil
}
func objectResult(key string, value any) (mcp.Result, error) {
	return mcp.Result{Structured: map[string]any{key: value}}, nil
}
func (t Tools) Register(registry *mcp.Registry) error {
	if registry == nil || t.Service.Store == nil {
		return ErrUnavailable
	}
	for _, kind := range []string{"policy", "silence", "inhibition", "channel", "route", "settings", "grouping", "template"} {
		kind := kind
		var operations []string
		// Enterprise account/channel/template configuration remains a portal
		// capability. Models may select existing configuration, not maintain it.
		if slices.Contains([]string{"policy", "silence", "inhibition", "route"}, kind) {
			operations = []string{"create", "update", "delete", "enable", "disable"}
		}
		for _, operation := range operations {
			operation := operation
			id := "alert." + kind + "." + operation + ".preview"
			schema := map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "name": map[string]any{"type": "string", "maxLength": 200}, "spec": map[string]any{"type": "object"}, "expected_version": map[string]any{"type": "integer", "minimum": 0}}, "additionalProperties": false}
			if operation == "create" {
				schema["required"] = []string{"name", "spec"}
			} else {
				schema["required"] = []string{"id", "expected_version"}
				if operation == "update" {
					schema["required"] = []string{"id", "name", "spec", "expected_version"}
				}
			}
			var contracts map[string]any
			_ = json.Unmarshal(schemas.AlertContracts, &contracts)
			schema["$defs"] = reachableDefinitions(configSchema(kind), contracts["$defs"].(map[string]any))
			if kind == "policy" && operation == "update" {
				rule := schema["$defs"].(map[string]any)["AlertRule"].(map[string]any)
				rule["required"] = append(rule["required"].([]any), "id")
			}
			schema["properties"].(map[string]any)["spec"] = map[string]any{"$ref": "#/$defs/" + configSchema(kind)}
			if operation != "create" && operation != "update" {
				delete(schema["properties"].(map[string]any), "name")
				delete(schema["properties"].(map[string]any), "spec")
				delete(schema, "$defs")
			}
			run := func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
				a, err := t.actor(ctx, call)
				if err != nil {
					return mcp.Result{}, err
				}
				input, err := toolInput[Mutation](call)
				if err != nil {
					return mcp.Result{}, err
				}
				input.Kind = kind
				input.Operation = operation
				action, err := t.Service.Preview(ctx, a, input, call.InvocationID)
				if err != nil {
					return mcp.Result{}, err
				}
				return mcp.Result{Structured: publicPending(action)}, nil
			}
			if err := t.register(registry, id, requiredPermission(kind), "write", schema, run); err != nil {
				return err
			}
			if err := registerHiddenCommit(registry, strings.TrimSuffix(id, ".preview")+".commit"); err != nil {
				return err
			}
		}
		id := "alert." + kind + ".list"
		if err := t.register(registry, id, ConfigurationReadPermission(kind), "read", map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "additionalProperties": false}, func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
			a, err := t.actor(ctx, call)
			if err != nil {
				return mcp.Result{}, err
			}
			rows, err := t.Service.Configurations(ctx, a, kind)
			if err != nil {
				return mcp.Result{}, err
			}
			query, _ := call.Input["query"].(string)
			if query = strings.ToLower(strings.TrimSpace(query)); query != "" {
				rows = slices.DeleteFunc(rows, func(row Configuration) bool {
					return !strings.Contains(strings.ToLower(row.Name+" "+string(row.Spec)), query)
				})
			}
			return objectResult("items", rows)
		}); err != nil {
			return err
		}
	}
	for _, entry := range []struct {
		id      string
		history bool
	}{{"alert.event.query", false}, {"alert.history.query", true}} {
		entry := entry
		if err := t.register(registry, entry.id, "alert.read", "read", eventQueryToolSchema(entry.history), func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
			a, err := t.actor(ctx, call)
			if err != nil {
				return mcp.Result{}, err
			}
			filter, err := toolInput[ListFilter](call)
			if err != nil {
				return mcp.Result{}, err
			}
			page, err := t.Service.Instances(ctx, a, filter, entry.history)
			if err != nil {
				return mcp.Result{}, err
			}
			return objectResult("page", page)
		}); err != nil {
			return err
		}
	}
	if err := t.register(registry, "alert.notification.query", "alert.read", "read", queryToolSchema(), func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		a, err := t.actor(ctx, call)
		if err != nil {
			return mcp.Result{}, err
		}
		filter, err := toolInput[ListFilter](call)
		if err != nil {
			return mcp.Result{}, err
		}
		page, err := t.Service.Deliveries(ctx, a, filter)
		if err != nil {
			return mcp.Result{}, err
		}
		return objectResult("page", page)
	}); err != nil {
		return err
	}
	if err := t.register(registry, "alert.event.get", "alert.read", "read", idToolSchema(), func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		a, err := t.actor(ctx, call)
		if err != nil {
			return mcp.Result{}, err
		}
		input, err := toolInput[struct {
			ID uuid.UUID `json:"id"`
		}](call)
		if err != nil {
			return mcp.Result{}, err
		}
		instance, events, err := t.Service.Instance(ctx, a, input.ID)
		if err != nil {
			return mcp.Result{}, err
		}
		return mcp.Result{Structured: map[string]any{"instance": instance, "events": events}}, nil
	}); err != nil {
		return err
	}
	if err := t.register(registry, "alert.policy.get", "alert.read", "read", idToolSchema(), func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		a, err := t.actor(ctx, call)
		if err != nil {
			return mcp.Result{}, err
		}
		input, err := toolInput[struct {
			ID uuid.UUID `json:"id"`
		}](call)
		if err != nil {
			return mcp.Result{}, err
		}
		item, err := t.Service.Configuration(ctx, a, input.ID)
		if err != nil {
			return mcp.Result{}, err
		}
		return objectResult("configuration", item)
	}); err != nil {
		return err
	}
	for _, operation := range []string{"event.acknowledge", "event.assign", "event.comment", "notification.test", "notification.resend"} {
		operation := operation
		permission := "alert.notification.manage"
		if strings.HasPrefix(operation, "event.") {
			permission = "alert.handle"
		}
		id := "alert." + operation + ".preview"
		schema := map[string]any{"type": "object", "required": []string{"id", "expected_version"}, "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}, "expected_version": map[string]any{"type": "integer", "minimum": 1}, "handling": map[string]any{"type": "object"}}, "additionalProperties": false}
		if strings.HasPrefix(operation, "event.") {
			props := schema["properties"].(map[string]any)
			delete(props, "expected_version")
			props["expected_handling_version"] = map[string]any{"type": "integer", "minimum": 1}
			schema["required"] = []string{"id", "expected_handling_version"}
		}
		if err := t.register(registry, id, permission, "write", schema, func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
			a, err := t.actor(ctx, call)
			if err != nil {
				return mcp.Result{}, err
			}
			input, err := toolInput[Command](call)
			if err != nil {
				return mcp.Result{}, err
			}
			input.Operation = operation
			action, err := t.Service.PreviewCommand(ctx, a, input, call.InvocationID)
			if err != nil {
				return mcp.Result{}, err
			}
			return mcp.Result{Structured: publicPending(action)}, nil
		}); err != nil {
			return err
		}
		if err := registerHiddenCommit(registry, strings.TrimSuffix(id, ".preview")+".commit"); err != nil {
			return err
		}
	}
	return nil
}
func registerHiddenCommit(registry *mcp.Registry, id string) error {
	return registry.Register(mcp.Metadata{ID: id, ToolFamily: id, Risk: "dangerous", Visibility: mcp.Hidden, ExecutionMode: mcp.Sequential, Required: []string{"internal.action_executor"}, InputVersion: "argus.private_action_plan/v1", OutputVersion: "argus.execution/v1", MaxResultBytes: 1024, InputSchema: map[string]any{"type": "object", "additionalProperties": false}, Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{}, errors.New("commit is performed deterministically by the internal Action Executor")
	}})
}
func (t Tools) register(registry *mcp.Registry, id, permission, risk string, schema map[string]any, run func(context.Context, mcp.Call) (mcp.Result, error)) error {
	mode := mcp.Sequential
	if risk == "read" {
		mode = mcp.ParallelSafe
	}
	return registry.Register(t.presentationMetadata(mcp.Metadata{ID: id, ToolFamily: id, Risk: risk, Visibility: mcp.Visible, ExecutionMode: mode, Required: []string{permission}, InputVersion: "argus.alert_input/v1", OutputVersion: "argus.alert_result/v1", ProjectionSchema: "argus.tool_result_projection/v1", MaxResultBytes: 2 << 20, InputSchema: schema, OutputSchema: map[string]any{"type": "object", "additionalProperties": true}, Discovery: alertDiscovery(id), Authorize: func(ctx context.Context, call mcp.Call) error {
		a, err := t.actor(ctx, call)
		if err != nil {
			return err
		}
		return authorize(ctx, t.Service.Store.Queries, a, permission)
	}, Execute: run}))
}
func idToolSchema() map[string]any {
	return map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": map[string]any{"type": "string", "format": "uuid"}}, "additionalProperties": false}
}
func queryToolSchema() map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"query", "severity", "lifecycle", "notification_state", "metric", "cursor", "from", "to", "policy_id", "resource_id", "rule_id", "channel_id"} {
		properties[key] = map[string]any{"type": "string"}
	}
	properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 500}
	properties["acknowledged"] = map[string]any{"type": "boolean"}
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

func eventQueryToolSchema(history bool) map[string]any {
	schema := queryToolSchema()
	properties := schema["properties"].(map[string]any)
	states := []string{"normal", "pending", "firing", "recovering"}
	description := "Current instances only. Resolved/closed episodes are excluded; use alert history.query for recovery or closed history. Empty current results do not prove absence of history."
	if history {
		states = []string{"resolved", "closed"}
		description = "Historical resolved/closed episodes only, including restored query-health events. Current active instances require alert event.query."
	}
	properties["lifecycle"] = map[string]any{"type": "string", "enum": states, "description": description}
	return schema
}
func configSchema(kind string) string {
	return "Alert" + map[string]string{"policy": "Policy", "silence": "Silence", "inhibition": "Inhibition", "channel": "Channel", "route": "Route", "settings": "Settings", "grouping": "Grouping", "template": "Template"}[kind] + "Spec"
}

func reachableDefinitions(root string, defs map[string]any) map[string]any {
	out := map[string]any{}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "$ref" {
					text, _ := child.(string)
					name := strings.TrimPrefix(text, "#/$defs/")
					if _, ok := out[name]; !ok {
						if definition, found := defs[name]; found {
							out[name] = definition
							walk(definition)
						}
					}
				} else {
					walk(child)
				}
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	out[root] = defs[root]
	walk(defs[root])
	return out
}
