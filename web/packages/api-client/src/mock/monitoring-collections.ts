import type { MonitoringCollection, MonitoringMutation } from "../monitoring";
import type { MockMonitoringState } from "./monitoring";
import type { BaseContext } from "./context";
import { monitoringIntegrations } from "../generated/monitoring-registry";
import {
  isBuiltinCollection,
  validateBuiltinCollection,
} from "./monitoring-builtins";
import { dashboardObjectGrant } from "./dashboard-state";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";
import {
  isLogRule,
  logRuleTarget,
  validateLogRule,
  canReadMonitoringCollection,
  canUseMonitoringCollector,
} from "./monitoring-logs";

function fail(code = "MONITORING_INVALID", status = 400): never {
  throw new ApiError(
    {
      code,
      message_key: `monitoring.errors.${code.replace("MONITORING_", "").toLowerCase()}`,
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}
function permit(ctx: BaseContext, permission: string) {
  const allowed = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (!allowed.has("*") && !allowed.has(permission))
    fail("MONITORING_DENIED", 403);
}
function current(ctx: BaseContext, data: MockMonitoringState, id?: string) {
  const row = data.collections?.find((v) => v.id === id);
  if (!row || !canReadMonitoringCollection(ctx, row))
    fail("MONITORING_DENIED", 403);
  return row;
}

export function validateMonitoringCollection(
  ctx: BaseContext,
  m: MonitoringMutation,
  data: MockMonitoringState,
  credentialPrepared?: boolean,
) {
  if (isBuiltinCollection(m.collection?.integration)) {
    validateBuiltinCollection(ctx, m, data);
    return;
  }
  permit(ctx, "monitoring.manage");
  permit(ctx, "collector.use");
  if (!m.collection || m.application || m.binding) fail();
  const existing =
    m.operation === "create" ? undefined : current(ctx, data, m.id);
  if (
    existing &&
    (existing.version !== m.expected_version || existing.status !== "active")
  )
    fail("MONITORING_VERSION_CONFLICT", 409);
  if (m.operation === "set_enabled" || m.operation === "archive") {
    const collector = ctx.db.collectors.find(
      (item) =>
        item.id === existing?.collector_id &&
        item.enterprise_id === ctx.enterpriseId(),
    );
    if (
      !collector ||
      !(existing?.integration === "k8slog"
        ? canUseMonitoringCollector(
            ctx,
            collector,
            existing.config.workload?.namespace_id,
          )
        : dashboardObjectGrant(
            ctx,
            collector.resource_type,
            collector.resource_id,
          ))
    )
      fail("MONITORING_DENIED", 403);
    if (existing && isLogRule(existing.integration))
      validateLogRule(
        ctx,
        {
          ...existing,
          target: existing.target,
          enabled: m.operation === "archive" ? false : m.collection.enabled,
        },
        data,
        existing,
        fail,
        (p) => permit(ctx, p),
      );
    return;
  }
  if (m.operation !== "create" && m.operation !== "update") fail();
  const input = m.collection;
  const entry = monitoringIntegrations.find((v) => v.key === input.integration);
  if (!entry || !entry.adapter_ready || entry.selection !== "candidate")
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  if (
    !input.name?.trim() ||
    input.name.length > 200 ||
    !input.collector_id ||
    !input.config
  )
    fail();
  const collector = ctx.db.collectors.find(
    (v) =>
      v.id === input.collector_id && v.enterprise_id === ctx.enterpriseId(),
  );
  if (
    !collector ||
    !(input.integration === "k8slog"
      ? canUseMonitoringCollector(
          ctx,
          collector,
          input.config?.workload?.namespace_id,
        )
      : dashboardObjectGrant(
          ctx,
          collector.resource_type,
          collector.resource_id,
        ))
  )
    fail("MONITORING_DENIED", 403);
  if (["uninstalled", "uninstalling"].includes(collector.status))
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  if (isLogRule(input.integration)) {
    if (m.credentials) fail("MONITORING_CREDENTIAL_INVALID");
    validateLogRule(ctx, input, data, existing, fail, (p) => permit(ctx, p));
    return;
  }
  let identity = input.target_identity;
  if (input.target) {
    const target = data.targets?.find((v) => v.id === input.target?.id);
    if (
      !target ||
      input.target.kind !== "monitoring_target" ||
      !dashboardObjectGrant(ctx, "monitoring_target", target.id)
    )
      fail("MONITORING_DENIED", 403);
    if (target.engine !== input.integration || input.target_identity) fail();
    identity = target.identity;
  } else if (!input.target_name?.trim()) fail();
  if (!identity?.endpoint || /[$\r\n\0]/.test(identity.endpoint)) fail();
  const form = "form" in entry ? entry.form : undefined;
  if (!form) fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  if (form.endpoint_kind === "http_url") {
    let url: URL;
    try {
      url = new URL(identity.endpoint);
    } catch {
      fail();
    }
    if (
      !["http:", "https:"].includes(url!.protocol) ||
      url!.username ||
      url!.password ||
      (url!.search &&
        !(input.integration === "apache" && url!.search === "?auto")) ||
      url!.hash
    )
      fail();
    if (input.integration === "apache" && url!.search !== "?auto") fail();
  } else if (form.endpoint_kind === "local") {
    if (identity.endpoint !== "local") fail();
  } else if (
    form.endpoint_kind === "host_port" &&
    !/^.+:\d{1,5}$/.test(identity.endpoint)
  )
    fail();
  if (form.database_required && !identity.database) fail();
  if (form.hostnames && !identity.hostnames?.length) fail();
  if (
    existing &&
    (existing.integration !== input.integration ||
      input.target?.id !== existing.target.id)
  )
    fail();
  for (const binding of [
    ...(input.bindings ?? []),
    ...(input.application_id
      ? [{ application_id: input.application_id, service_id: input.service_id }]
      : []),
  ]) {
    permit(ctx, "application.manage");
    if (
      !data.applications.some(
        (a) => a.id === binding.application_id && a.status === "active",
      ) ||
      !dashboardObjectGrant(ctx, "application", binding.application_id)
    )
      fail("MONITORING_DENIED", 403);
    if (
      binding.service_id &&
      !data.services.some(
        (s) =>
          s.id === binding.service_id &&
          s.application_id === binding.application_id,
      )
    )
      fail();
  }
  const replacing = input.credential_update === "replace";
  const prepared =
    credentialPrepared ??
    Boolean(
      m.credentials &&
      (input.integration === "redis" || m.credentials.username) &&
      (["mysql", "redis"].includes(input.integration ?? "") ||
        m.credentials.password),
    );
  if (replacing && !prepared) fail("MONITORING_CREDENTIAL_INVALID");
  const hasCredential = replacing
    ? prepared
    : input.credential_update === "remove"
      ? false
      : Boolean(existing?.credential_configured);
  if (
    (form.authentication === "required" && !hasCredential) ||
    (form.authentication === "none" && hasCredential)
  )
    fail("MONITORING_CREDENTIAL_INVALID");
  if (m.credentials && !replacing) fail("MONITORING_CREDENTIAL_INVALID");
}

export function commitMonitoringCollection(
  ctx: BaseContext,
  m: MonitoringMutation,
  data: MockMonitoringState,
  credentialPrepared: boolean,
) {
  validateMonitoringCollection(ctx, m, data, credentialPrepared);
  const input = m.collection!;
  if (m.operation === "create") {
    const entry = monitoringIntegrations.find(
      (v) => v.key === input.integration,
    )!;
    const target =
      input.target ??
      (isLogRule(input.integration)
        ? logRuleTarget(ctx, input)
        : {
            kind: "monitoring_target" as const,
            id: crypto.randomUUID(),
          });
    if (!input.target && !isLogRule(input.integration)) {
      (data.targets ??= []).push({
        id: target.id,
        name: input.target_name!,
        kind: entry.family as "database" | "cache" | "web",
        engine: entry.key,
        identity: input.target_identity!,
        status: "active",
        version: 1,
      });
      (ctx.db.dataAuthorizationGrants ??= []).push({
        subject_type: "user",
        subject_id: ctx.actor().id,
        resource_type: "monitoring_target",
        resource_id: target.id,
        active: true,
      });
    }
    const row: MonitoringCollection = {
      id: m.id!,
      name: input.name!,
      integration: input.integration!,
      target,
      collector_id: input.collector_id!,
      application_id: input.application_id,
      service_id: input.service_id,
      config: input.config!,
      credential_configured: credentialPrepared,
      desired_enabled: input.enabled ?? true,
      desired_revision: 1,
      effective_revision: 0,
      assignment_epoch: 1,
      apply_status: "pending",
      health: "unknown",
      freshness: "no_data",
      error_code: "",
      status: "active",
      version: 1,
    };
    (data.collections ??= []).push(row);
    if (target.kind !== "application") {
      for (const binding of [
        ...(input.bindings ?? []),
        ...(input.application_id
          ? [
              {
                application_id: input.application_id,
                service_id: input.service_id,
              },
            ]
          : []),
      ])
        data.bindings.push({
          ...binding,
          target: { kind: target.kind, id: target.id },
          id: crypto.randomUUID(),
          version: 1,
          active: true,
        });
    }
  } else {
    const row = current(ctx, data, m.id);
    if (m.operation === "archive") {
      row.status = "archived";
      row.desired_enabled = false;
      row.credential_configured = false;
    } else if (m.operation === "set_enabled")
      row.desired_enabled = input.enabled ?? false;
    else {
      row.name = input.name!;
      row.config = input.config!;
      row.desired_enabled = input.enabled ?? true;
      if (row.collector_id !== input.collector_id) row.assignment_epoch++;
      row.collector_id = input.collector_id!;
      if (input.credential_update === "replace")
        row.credential_configured = credentialPrepared;
      if (input.credential_update === "remove")
        row.credential_configured = false;
    }
    row.version++;
    row.desired_revision++;
    row.apply_status = "pending";
  }
}
