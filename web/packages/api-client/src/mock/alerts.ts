import type {
  AlertConfiguration,
  AlertDomains,
  AlertFilter,
  AlertInstance,
  AlertDelivery,
  AlertMutation,
  AlertSchemas,
} from "../alerts";
import type { BaseContext, MockContext } from "./context";
import type { PendingActionPublic, ConfirmActionResult } from "../types";
import fixture from "./alerts-fixture.json";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";
import { ensureAlertResources } from "./alerts-resources";
export interface MockAlertState {
  configurations: AlertConfiguration[];
  instances: AlertInstance[];
  deliveries: AlertDelivery[];
  events: Record<string, AlertSchemas["AlertEvent"][]>;
  attempts: Record<string, AlertSchemas["AlertDeliveryAttempt"][]>;
}
const copy = <T>(v: T): T => structuredClone(v);
function fail(code = "ALERT_INVALID", status = 400): never {
  throw new ApiError(
    {
      code,
      message_key: `errors.alerts.${code.replace("ALERT_", "").toLowerCase()}`,
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}
function permission(ctx: BaseContext, code: string) {
  const p = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (!p.has("*") && !p.has(code)) fail("ALERT_DENIED", 403);
}
const required = (kind: string) =>
  kind === "policy"
    ? "alert.policy.manage"
    : ["silence", "inhibition"].includes(kind)
      ? "alert.silence.manage"
      : kind === "settings"
        ? "service_account.manage"
        : "alert.notification.manage";
function seed(ctx: BaseContext): MockAlertState {
  const zh =
    typeof localStorage === "undefined" ||
    localStorage.getItem("argus.locale") !== "en-US";
  const L = (v: string | string[]) => (Array.isArray(v) ? v[zh ? 0 : 1]! : v);
  const at = "2026-10-07T02:32:00Z";
  ensureAlertResources(ctx, at);
  const configurations: AlertConfiguration[] = [];
  const add = (
    id: string,
    kind: AlertConfiguration["kind"],
    name: string,
    spec: AlertConfiguration["spec"],
    version = 1,
    health = "normal",
    evaluatedAt = at,
  ) =>
    configurations.push({
      id,
      kind,
      name,
      spec,
      version,
      generation: 1,
      health,
      created_at: at,
      updated_at: at,
      last_evaluated_at: evaluatedAt,
    });
  const resourceId = (name: string) =>
    ctx.db.hosts.find(
      (h) => h.enterpriseId === ctx.enterpriseId() && h.name === name,
    )?.id ??
    ctx.db.clusters.find(
      (h) => h.enterpriseId === ctx.enterpriseId() && h.name === name,
    )?.id ??
    name;
  for (const c of fixture.channels) {
    const secretId = `sec-${c.credential}`;
    if (!ctx.db.secrets.some((entry) => entry.id === secretId))
      ctx.db.secrets.push({
        id: secretId,
        enterprise_id: ctx.enterpriseId(),
        name: c.credential,
        type: "notification",
        status: "active",
        current_version: 1,
        reference_count: 1,
        created_by: ctx.actor().id,
        version: 1,
        created_at: at,
        updated_at: at,
      });
    const credentials = (ctx.db.secretCredentials ??= []);
    if (!credentials.some((entry) => entry.id === c.credential))
      credentials.push({
        id: c.credential,
        enterprise_id: ctx.enterpriseId(),
        name: c.credential,
        protocol: "notification",
        secret_id: secretId,
        status: "active",
        version: 1,
        created_at: at,
        updated_at: at,
      });
    add(c.id, "channel", L(c.name), {
      enabled: c.enabled,
      default_for_new_policies: c.defaultForNewPolicies === true,
      provider:
        c.provider === "email"
          ? "smtp"
          : (c.provider as "dingtalk" | "wecom" | "feishu"),
      credential_id: c.credential,
      display_address: c.endpoint,
      mention_ids: [],
      mention_all: false,
      template: "",
    });
  }
  for (const p of fixture.policies)
    add(
      p.id,
      "policy",
      L(p.name),
      {
        description: "",
        enabled: p.enabled,
        resources: {
          mode: p.resource === "all" ? "dynamic" : "explicit",
          types: ["host", "kubernetes_cluster"],
          ids: p.resource === "all" ? [] : [resourceId(p.resource)],
          matchers: [],
          source_ids: [],
        },
        rules: p.rules.map((r, i) => ({
          id: `${p.id}-rule-${i}`,
          name: r.metric,
          expression: r.metric,
          query_mode: "numeric",
          operator: r.op as ">" | "<",
          threshold: r.threshold,
          recovery_threshold: r.recovery,
          severity: r.severity as "critical" | "warning",
          unit: r.metric === "collector_up" ? "" : "%",
        })),
        mode: p.mode as "independent" | "any",
        match_by: ["argus_resource_id", "argus_source_id"],
        interval_seconds: p.interval,
        for_seconds: p.duration * 60,
        recovery_seconds: p.recovery * 60,
        no_data_behavior: "report",
        error_behavior: "keep",
        channel_ids: p.channels,
        route_ids: [],
        labels: {},
      },
      p.revision,
      p.health,
      `2026-10-07T${p.lastEval}+08:00`,
    );
  for (const s of fixture.silences)
    add(s.id, "silence", L(s.name), {
      enabled: s.enabled,
      resource_ids: [resourceId(s.resource)],
      policy_ids: [s.policy],
      metric: "",
      matchers: [],
      starts_at: "2026-10-07T01:00:00Z",
      ends_at: s.until,
      reason: L(s.reason),
    });
  for (const s of fixture.inhibitions)
    add(s.id, "inhibition", L(s.name), {
      enabled: s.enabled,
      resource_ids: [],
      source: [{ key: "severity", operator: "=", value: s.source }],
      target: [{ key: "severity", operator: "=", value: s.target }],
      equal: s.equal.split(", "),
      reason: L(s.reason),
    });
  fixture.routes.forEach((r, i) =>
    add(r.id, "route", L(r.name), {
      enabled: true,
      matchers: [{ key: "severity", operator: "=", value: r.severity }],
      channel_ids: r.channels,
      continue: false,
      priority: i,
    }),
  );
  add("grouping", "grouping", zh ? "通知合并设置" : "Notification grouping", {
    group_by: [fixture.grouping.by],
    wait_seconds: fixture.grouping.wait,
    interval_seconds: fixture.grouping.interval,
    repeat_seconds: fixture.grouping.repeat,
    timing: fixture.grouping.timing as Array<
      "first" | "repeat" | "recovery" | "severity"
    >,
  });
  add("template", "template", zh ? "消息模板" : "Message template", {
    content: fixture.template,
  });
  for (const entry of fixture.audit) {
    const id = `alert-fixture-${entry.id}`;
    if (!ctx.db.auditEvents.some((record) => record.id === id)) {
      ctx.db.auditEvents.push({
        id,
        enterpriseId: ctx.enterpriseId(),
        actorUserId: entry.actor,
        actorName:
          entry.actor === "lin"
            ? L(["林晓", "Lin Xiao"])
            : L(["陈默", "Chen Mo"]),
        actorType: "enterprise_user",
        action:
          entry.id === "a1"
            ? "alert.handle.assign"
            : entry.id === "a2"
              ? "alert.silence.create"
              : "alert.policy.create",
        origin: entry.origin === "model" ? "admin_chatbox" : "admin_ui",
        resourceType: "alert",
        resourceId: entry.object,
        summary: L(entry.action),
        result: "success",
        createdAt: new Date(`2026-10-07T${entry.time}+08:00`).toISOString(),
      });
    }
  }
  const instances: AlertInstance[] = fixture.events.map((e) => {
    const p = configurations.find((p) => p.id === e.policy)!;
    const spec = p.spec as AlertSchemas["AlertPolicySpec"];
    const rule = spec.rules.find((r) => r.expression === e.metric)!;
    return {
      id: e.id,
      policy_id: e.policy,
      policy_version: e.revision,
      resource_ids: [resourceId(e.resource)],
      source_ids: [],
      state: {
        name: L(e.name),
        policy_name: p.name,
        rule_id: rule.id!,
        metric: e.metric,
        labels: {
          resource_name: e.resource,
          service: e.service,
          ...(e.owner !== "none"
            ? {
                owner_name:
                  (
                    {
                      han: zh ? "韩澈" : "Han Che",
                      lin: zh ? "林晓" : "Lin Xiao",
                    } as Record<string, string>
                  )[e.owner] ?? e.owner,
              }
            : {}),
        },
        value: e.value ?? undefined,
        threshold: e.threshold,
        recovery_threshold: rule.recovery_threshold,
        unit: e.metric === "collector_up" ? "" : "%",
        last_slot: at,
        policy_snapshot: {
          ...spec,
          rules: spec.rules.map((snapshotRule) =>
            snapshotRule.id === rule.id
              ? {
                  ...snapshotRule,
                  threshold: e.threshold,
                  severity: e.severity as AlertInstance["severity"],
                }
              : snapshotRule,
          ),
        },
        notification_state:
          e.notification === "received" ? "accepted" : e.notification,
        noise_reasons:
          e.notification === "muted"
            ? [L(fixture.silences[0]!.reason)]
            : e.notification === "inhibited"
              ? [L(fixture.inhibitions[0]!.reason)]
              : [],
        kind: "condition",
      },
      lifecycle: e.status as AlertInstance["lifecycle"],
      health: e.value === null ? "no_data" : "normal",
      severity:
        e.status === "resolved"
          ? (e.severity as AlertInstance["severity"])
          : rule.severity,
      handling_version: 1,
      acknowledged: e.ack,
      owner_id: e.owner === "none" ? undefined : e.owner,
      started_at: new Date(
        e.started.replace(" ", "T") + "+08:00",
      ).toISOString(),
      resolved_at: e.resolvedAt
        ? new Date(e.resolvedAt.replace(" ", "T") + "+08:00").toISOString()
        : undefined,
      last_evaluated_at: at,
      version: 1,
    };
  });
  const events: MockAlertState["events"] = {};
  instances.forEach(
    (e) =>
      (events[e.id] = [
        {
          id: crypto.randomUUID(),
          instance_id: e.id,
          event_type: "first",
          payload: {},
          origin: "evaluator",
          created_at: e.started_at!,
        },
      ]),
  );
  const deliveries: AlertDelivery[] = fixture.deliveries.map((d) => {
    const e = instances.find((e) => e.id === d.event)!;
    return {
      id: d.id,
      instance_id: e.id,
      channel_id: d.channel,
      resource_ids: e.resource_ids,
      message: {
        subject: e.state.name,
        text: e.state.metric,
        event_type: e.lifecycle === "resolved" ? "recovery" : "first",
        instance_ids: [e.id],
        policy_ids: [e.policy_id],
        labels: e.state.labels,
        test: false,
      },
      status:
        d.status === "received"
          ? "accepted"
          : (d.status as AlertDelivery["status"]),
      attempt: d.attempts,
      last_error: d.reason,
      last_attempt_at: `2026-10-07T${d.time}+08:00`,
      created_at: at,
      version: 1,
    };
  });
  return { configurations, instances, deliveries, events, attempts: {} };
}

function configurationWithHits(
  row: AlertConfiguration,
  state: MockAlertState,
): AlertConfiguration {
  if (!["silence", "inhibition"].includes(row.kind)) return row;
  const spec = row.spec as AlertSchemas["AlertSilenceSpec"] &
    AlertSchemas["AlertInhibitionSpec"];
  const resourceIds = spec.resource_ids ?? [];
  const active = state.instances.filter((e) =>
    ["firing", "recovering"].includes(e.lifecycle),
  );
  const labels = (e: AlertInstance) => ({
    ...e.state.labels,
    severity: e.severity,
    metric: e.state.metric,
    policy_id: e.policy_id,
    argus_resource_id: e.resource_ids[0] ?? "",
    resource_id: e.resource_ids[0] ?? "",
  });
  const match = (e: AlertInstance, rules: AlertSchemas["AlertMatcher"][]) =>
    rules.every((rule) => {
      const value = (labels(e) as Record<string, string>)[rule.key] ?? "";
      if (rule.operator === "=") return value === rule.value;
      if (rule.operator === "!=") return value !== rule.value;
      try {
        const matched = new RegExp(rule.value).test(value);
        return rule.operator === "=~" ? matched : !matched;
      } catch {
        return false;
      }
    });
  const hits = spec.enabled
    ? active.filter((e) => {
        if (
          resourceIds.length &&
          !e.resource_ids.every((id) => resourceIds.includes(id))
        )
          return false;
        if (row.kind === "silence")
          return (
            (!spec.policy_ids?.length ||
              spec.policy_ids.includes(e.policy_id)) &&
            (!spec.metric || e.state.metric === spec.metric) &&
            match(e, spec.matchers ?? [])
          );
        return (
          match(e, spec.target ?? []) &&
          active.some(
            (source) =>
              source.id !== e.id &&
              match(source, spec.source ?? []) &&
              spec.equal.every((key) => {
                const a = (labels(e) as Record<string, string>)[key],
                  b = (labels(source) as Record<string, string>)[key];
                return a !== undefined && a !== "" && a === b;
              }),
          )
        );
      }).length
    : 0;
  return { ...row, matched_instances: hits };
}
function state(ctx: BaseContext) {
  const all = (ctx.db.alerts ??= {});
  return (all[ctx.enterpriseId()] ??=
    ctx.enterpriseId() === "ent-acme"
      ? seed(ctx)
      : {
          configurations: [],
          instances: [],
          deliveries: [],
          events: {},
          attempts: {},
        });
}
function filtered<T extends AlertInstance | AlertDelivery>(
  items: T[],
  f: AlertFilter = {},
) {
  return items.filter((i) => {
    const e: AlertInstance | undefined =
      "lifecycle" in i ? (i as AlertInstance) : undefined;
    return (
      (!f.query ||
        JSON.stringify(i).toLowerCase().includes(f.query.toLowerCase())) &&
      (!f.severity || e?.severity === f.severity) &&
      (!f.lifecycle || e?.lifecycle === f.lifecycle) &&
      (!f.policy_id ||
        (e
          ? e.policy_id === f.policy_id
          : (i as AlertDelivery).message.policy_ids.includes(f.policy_id))) &&
      (!f.channel_id || (i as AlertDelivery).channel_id === f.channel_id) &&
      (!f.rule_id || e?.state.rule_id === f.rule_id) &&
      (!f.resource_id || i.resource_ids.includes(f.resource_id)) &&
      (!f.metric || e?.state.metric.includes(f.metric)) &&
      (!f.notification_state ||
        (e?.state.notification_state ?? ("status" in i ? i.status : "")) ===
          f.notification_state) &&
      (f.acknowledged === undefined || e?.acknowledged === f.acknowledged) &&
      (!f.from ||
        (e ? e.last_evaluated_at : (i as AlertDelivery).created_at) >=
          f.from) &&
      (!f.to ||
        (e ? e.last_evaluated_at : (i as AlertDelivery).created_at) <= f.to)
    );
  });
}
function page<T>(items: T[], f: AlertFilter = {}) {
  const offset = Number(f.cursor ?? 0),
    limit = f.limit ?? 50,
    end = Math.min(offset + limit, items.length);
  return {
    items: copy(items.slice(offset, end)),
    total: items.length,
    next_cursor: end < items.length ? String(end) : null,
    complete: end === items.length,
  };
}
export function createAlertDomains(ctx: MockContext): AlertDomains {
  const read = () => {
    permission(ctx, "alert.read");
    return state(ctx);
  };
  return {
    alerts: {
      async summary() {
        const s = read();
        await ctx.pause();
        const active = s.instances.filter(
          (e) => !["resolved", "closed", "normal"].includes(e.lifecycle),
        );
        return {
          current_total: active.length,
          active: active.filter(
            (e) => e.lifecycle === "firing" || e.lifecycle === "recovering",
          ).length,
          unacknowledged: active.filter((e) => !e.acknowledged).length,
          policies: s.configurations.filter((c) => c.kind === "policy").length,
          failed_deliveries: s.deliveries.filter((d) =>
            ["failed", "unknown"].includes(d.status),
          ).length,
          last_evaluated_at: s.instances[0]?.last_evaluated_at,
        };
      },
      async configurations(kind) {
        await ctx.pause();
        const s = read();
        return copy(
          s.configurations
            .filter((c) => c.kind === kind)
            .map((c) => configurationWithHits(c, s)),
        );
      },
      async configuration(id) {
        return copy(
          read().configurations.find((c) => c.id === id) ??
            fail("ALERT_NOT_FOUND", 404),
        );
      },
      async preview(input) {
        permission(ctx, required(input.kind));
        if (input.kind === "policy" && input.operation === "update") {
          const rules = (
            input.spec as AlertSchemas["AlertPolicySpec"] | undefined
          )?.rules;
          const ids = rules?.map((rule) => rule.id);
          if (
            !ids?.length ||
            ids.some(
              (id) => !id || id === "00000000-0000-0000-0000-000000000000",
            ) ||
            new Set(ids).size !== ids.length
          )
            fail();
        }
        if (input.operation !== "create") {
          const row = state(ctx).configurations.find((c) => c.id === input.id);
          if (!row) fail("ALERT_NOT_FOUND", 404);
          if (row.version !== input.expected_version)
            fail("ALERT_VERSION_CONFLICT", 409);
        }
        return ctx.createPendingAction({
          tool: `alert.${input.kind}.${input.operation}`,
          title:
            input.name ??
            state(ctx).configurations.find((c) => c.id === input.id)?.name ??
            input.kind,
          risk: "write",
          input_data: copy(input) as Record<string, unknown>,
        });
      },
      async previewCommand(input) {
        permission(
          ctx,
          input.operation.startsWith("event.")
            ? "alert.handle"
            : "alert.notification.manage",
        );
        return ctx.createPendingAction({
          tool: `alert.command.${input.operation}`,
          title: input.operation,
          risk: "write",
          input_data: copy(input) as unknown as Record<string, unknown>,
        });
      },
      async trial(input) {
        permission(ctx, "alert.policy.manage");
        if (input.rules.some((r) => !r.expression.trim() || !r.name.trim()))
          fail();
        return {
          observations: [],
          health: "no_data",
          error_code: "MOCK_TRIAL_NO_DATA",
          resource_ids: input.resources.ids ?? [],
          slot: ctx.nowIso(),
        };
      },
      async instances(f = {}, history = false) {
        await ctx.pause();
        const state = read();
        const result = page(
          filtered(
            state.instances.filter(
              (e) =>
                history ||
                !["resolved", "closed", "normal"].includes(e.lifecycle),
            ),
            f,
          ),
          f,
        );
        const policies = state.configurations.filter(
          (row) =>
            row.kind === "policy" && (!f.policy_id || row.id === f.policy_id),
        );
        return {
          ...result,
          resource_ids: [
            ...new Set(state.instances.flatMap((row) => row.resource_ids)),
          ],
          evaluations: policies.map((row) => ({
            policy_id: row.id,
            name: row.name,
            health: row.health,
            error_code: row.error_code,
            last_evaluated_at: row.last_evaluated_at ?? "",
            last_successful_at: row.last_successful_at ?? "",
          })),
        };
      },
      async instance(id) {
        const s = read();
        return copy({
          instance:
            s.instances.find((e) => e.id === id) ??
            fail("ALERT_NOT_FOUND", 404),
          events: s.events[id] ?? [],
        });
      },
      async handle(id, input) {
        permission(ctx, "alert.handle");
        const s = state(ctx),
          e =
            s.instances.find((e) => e.id === id) ??
            fail("ALERT_NOT_FOUND", 404);
        if (e.handling_version !== input.expected_handling_version)
          fail("ALERT_VERSION_CONFLICT", 409);
        if (input.operation === "acknowledge") e.acknowledged = true;
        if (input.operation === "assign") e.owner_id = input.owner_id;
        if (input.operation === "comment" && !input.comment?.trim()) fail();
        e.handling_version++;
        e.version++;
        (s.events[id] ??= []).push({
          id: crypto.randomUUID(),
          instance_id: id,
          event_type: input.operation,
          payload: input as unknown as Record<string, unknown>,
          actor_id: ctx.actor().id,
          origin: "admin_ui",
          created_at: ctx.nowIso(),
        });
        ctx.audit(`alert.handle.${input.operation}`, {
          summary: input.operation,
          resourceType: "alert_instance",
          resourceId: id,
        });
        ctx.save();
        return copy(e);
      },
      async deliveries(f = {}) {
        await ctx.pause();
        return page(filtered(read().deliveries, f), f);
      },
      async delivery(id) {
        const s = read();
        return copy({
          delivery:
            s.deliveries.find((d) => d.id === id) ??
            fail("ALERT_NOT_FOUND", 404),
          attempts: s.attempts[id] ?? [],
        });
      },
      async exportHistory(f = {}) {
        const rows = filtered(read().instances, f);
        return {
          filename: "alert-history.json",
          content_type: "application/json",
          content: JSON.stringify(rows, null, 2),
          count: rows.length,
        };
      },
    },
  };
}
export function commitAlertAction(
  ctx: BaseContext,
  action: PendingActionPublic,
): ConfirmActionResult | undefined {
  const plan = ctx.db.actionPlans[action.action_ref];
  if (!plan?.tool.startsWith("alert.")) return undefined;
  const s = state(ctx);
  const m = plan.input_data as unknown as AlertMutation;
  if (plan.tool.startsWith("alert.command.")) {
    const c = plan.input_data as unknown as AlertSchemas["AlertCommand"];
    permission(
      ctx,
      c.operation.startsWith("event.")
        ? "alert.handle"
        : "alert.notification.manage",
    );
    if (c.operation === "notification.test") {
      const ch =
        s.configurations.find((cg) => cg.id === c.id) ??
        fail("ALERT_NOT_FOUND", 404);
      s.deliveries.unshift({
        id: crypto.randomUUID(),
        channel_id: c.id,
        resource_ids: [],
        message: {
          subject: "Argus notification test",
          text: "Mock protocol test",
          event_type: "test",
          instance_ids: [],
          policy_ids: [],
          labels: {},
          test: true,
        },
        status: "accepted",
        attempt: 1,
        last_error: "",
        created_at: ctx.nowIso(),
        last_attempt_at: ctx.nowIso(),
        version: 1,
      });
      if (ch.version !== c.expected_version)
        fail("ALERT_VERSION_CONFLICT", 409);
    } else if (c.operation === "notification.resend") {
      const d =
        s.deliveries.find((d) => d.id === c.id) ?? fail("ALERT_NOT_FOUND", 404);
      if (d.version !== c.expected_version) fail("ALERT_VERSION_CONFLICT", 409);
      if (!["failed", "unknown"].includes(d.status)) fail();
      const id = crypto.randomUUID();
      const resend: AlertDelivery = {
        ...copy(d),
        id,
        resend_of_id: d.id,
        status: "accepted",
        attempt: 1,
        version: 1,
        last_error: "",
        created_at: ctx.nowIso(),
        last_attempt_at: ctx.nowIso(),
      };
      delete resend.batch_id;
      s.deliveries.unshift(resend);
      s.attempts[id] = [
        {
          id: crypto.randomUUID(),
          attempt: 1,
          status: "accepted",
          result_code: "0",
          error_code: "",
          started_at: ctx.nowIso(),
          finished_at: ctx.nowIso(),
        },
      ];
    }
  } else {
    permission(ctx, required(m.kind));
    const row = s.configurations.find((c) => c.id === m.id);
    if (
      m.operation !== "create" &&
      (!row || row.version !== m.expected_version)
    )
      fail("ALERT_VERSION_CONFLICT", 409);
    if (m.operation === "create") {
      if (!m.spec || !m.name?.trim()) fail();
      s.configurations.unshift({
        id: crypto.randomUUID(),
        kind: m.kind,
        name: m.name,
        spec: copy(m.spec),
        version: 1,
        generation: 1,
        health: "not_evaluated",
        created_at: ctx.nowIso(),
        updated_at: ctx.nowIso(),
      });
    } else if (m.operation === "delete")
      s.configurations = s.configurations.filter((c) => c.id !== m.id);
    else if (row) {
      if (m.operation === "update") {
        row.name = m.name!;
        row.spec = copy(m.spec!);
      } else
        (row.spec as { enabled: boolean }).enabled = m.operation === "enable";
      row.version++;
      row.updated_at = ctx.nowIso();
    }
  }
  action.status = "succeeded";
  action.available_actions = [];
  action.updated_at = ctx.nowIso();
  ctx.audit(plan.tool, {
    summary: action.title,
    resourceType: "alert_configuration",
    resourceId: m.id,
  });
  ctx.save();
  return { pending_action: copy(action) };
}
