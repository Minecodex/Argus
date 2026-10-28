import type {
  MonitoringApplication,
  MonitoringDomains,
  MonitoringListInput,
  MonitoringMutation,
  MonitoringService,
  MonitoringSchemas,
  MonitoringCollection,
} from "../monitoring";
import { monitoringIntegrations } from "../generated/monitoring-registry";
import { monitoringRequest } from "../monitoring-request";
import { createBuiltinDomains } from "./monitoring-builtins";
import {
  canReadMonitoringCollection,
  canUseMonitoringCollector,
} from "./monitoring-logs";
import {
  collectorComponents,
  collectorVersion,
  collectorDistributionVersion,
} from "../generated/collector-registry";
import {
  validateMonitoringCollection,
  commitMonitoringCollection,
} from "./monitoring-collections";
import type { ConfirmActionResult, PendingActionPublic } from "../types";
import { ApiError } from "../transport/errors";
import type { BaseContext, MockContext } from "./context";
import { dashboardObjectGrant } from "./dashboard-state";
import { resolvePermissions } from "./permissions";
import {
  createIngestDomains,
  validateMonitoringIngest,
  commitMonitoringIngest,
  type IngestHelpers,
} from "./monitoring-ingest";

export type MockMonitoringState = {
  applications: MonitoringApplication[];
  services: MonitoringService[];
  targets?: MonitoringSchemas["MonitoringTarget"][];
  collections?: MonitoringCollection[];
  endpoints?: MonitoringSchemas["MonitoringIngestEndpoint"][];
  ingestKeys?: Array<
    MonitoringSchemas["MonitoringIngestKey"] & { creator_id: string }
  >;
  bindings: Array<
    NonNullable<MonitoringMutation["binding"]> & {
      id: string;
      version: number;
      active: boolean;
    }
  >;
};
const copy = <T>(value: T): T => structuredClone(value);
function state(ctx: BaseContext): MockMonitoringState {
  const all = (ctx.db.monitoring ??= {});
  return (all[ctx.enterpriseId()] ??= {
    applications: [],
    services: [],
    bindings: [],
  });
}
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
function permission(ctx: BaseContext, name: string) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (!permissions.has("*") && !permissions.has(name))
    fail("MONITORING_DENIED", 403);
}
function app(ctx: BaseContext, id: string) {
  const found = state(ctx).applications.find((v) => v.id === id);
  if (!found || !dashboardObjectGrant(ctx, "application", id))
    fail("MONITORING_DENIED", 403);
  return found;
}
function validate(
  ctx: BaseContext,
  m: MonitoringMutation,
  credentialPrepared?: boolean,
) {
  if (m.kind === "endpoint" || m.kind === "ingest_key") {
    validateMonitoringIngest(ctx, ingestHelpers(ctx), m);
    return;
  }
  if (m.kind === "collection") {
    validateMonitoringCollection(ctx, m, state(ctx), credentialPrepared);
    return;
  }
  if (m.kind === "service") {
    permission(ctx, "application.manage");
    if (
      m.operation !== "create" ||
      !m.service ||
      m.application ||
      m.binding ||
      m.collection ||
      m.credentials
    )
      fail();
    const s = m.service;
    const parent = app(ctx, s.application_id);
    if (
      parent.status !== "active" ||
      !s.service_name.trim() ||
      s.service_name !== s.service_name.trim() ||
      s.service_name.length > 200 ||
      s.service_name.toLowerCase().startsWith("unknown_service") ||
      (s.service_namespace &&
        s.service_namespace.trim() !== s.service_namespace)
    )
      fail();
    if (
      state(ctx).services.some(
        (v) =>
          v.application_id === s.application_id &&
          v.service_namespace === (s.service_namespace ?? "") &&
          v.service_name === s.service_name,
      )
    )
      fail("MONITORING_VERSION_CONFLICT", 409);
    if (
      state(ctx).services.filter((v) => v.application_id === s.application_id)
        .length >= 1000
    )
      fail("MONITORING_SERVICE_LIMIT", 409);
    return;
  }
  if (m.kind !== "application" && m.kind !== "binding")
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  if (m.credentials !== undefined || m.collection !== undefined) fail();
  permission(ctx, "application.manage");
  if (m.kind === "application") {
    if (m.operation === "create") {
      if (
        !m.application?.name.trim() ||
        !/^[a-z][a-z0-9_-]{0,63}$/.test(m.application.environment ?? "")
      )
        fail();
    } else {
      const row = app(ctx, m.id ?? "");
      if (row.version !== m.expected_version || row.status !== "active")
        fail("MONITORING_VERSION_CONFLICT", 409);
      if (m.operation !== "archive" && m.operation !== "update") fail();
      if (
        m.operation === "update" &&
        (!m.application?.name.trim() ||
          (m.application.environment &&
            m.application.environment !== row.environment) ||
          (m.application.application_key &&
            m.application.application_key !== row.application_key))
      )
        fail();
    }
  } else {
    if (!m.binding) fail();
    app(ctx, m.binding.application_id);
    if (!dashboardObjectGrant(ctx, m.binding.target.kind, m.binding.target.id))
      fail("MONITORING_DENIED", 403);
    if (
      m.binding.service_id &&
      !state(ctx).services.some(
        (s) =>
          s.id === m.binding!.service_id &&
          s.application_id === m.binding!.application_id,
      )
    )
      fail();
  }
}
function page<T>(
  ctx: BaseContext,
  values: T[],
  input: MonitoringListInput = {},
) {
  const limit = input.limit ?? 50;
  if (limit < 1 || limit > 200) fail();
  const binding = JSON.stringify({
    tenant: ctx.enterpriseId(),
    actor: ctx.actor().id,
    version: ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1,
    search: input.search?.trim() ?? "",
    status: input.status ?? "active",
    environment: input.environment ?? "",
    applications:
      input.application_ids === undefined
        ? "all_authorized"
        : [...input.application_ids].sort(),
    limit,
    integration: input.integration ?? "",
    desired_enabled: input.desired_enabled ?? null,
    apply_status: input.apply_status ?? "",
    health: input.health ?? "",
    freshness: input.freshness ?? "",
  });
  let offset = 0;
  if (input.cursor) {
    try {
      const c = JSON.parse(
        new TextDecoder().decode(
          Uint8Array.from(atob(input.cursor), (x) => x.charCodeAt(0)),
        ),
      ) as { binding: string; offset: number };
      if (c.binding !== binding || !Number.isInteger(c.offset) || c.offset < 0)
        fail("MONITORING_CURSOR_INVALID");
      offset = c.offset;
    } catch {
      fail("MONITORING_CURSOR_INVALID");
    }
  }
  const end = Math.min(offset + limit, values.length);
  const next =
    end < values.length
      ? btoa(
          String.fromCharCode(
            ...new TextEncoder().encode(
              JSON.stringify({ binding, offset: end }),
            ),
          ),
        )
      : undefined;
  return {
    items: copy(values.slice(offset, end)),
    ...(next ? { next_cursor: next } : {}),
  };
}
export function createMonitoringDomains(ctx: MockContext): MonitoringDomains {
  return {
    monitoring: {
      ...createBuiltinDomains(ctx, () => state(ctx)),
      ...createIngestDomains(ctx, ingestHelpers(ctx)),
      async collectors(input = {}) {
        permission(ctx, "collector.use");
        await ctx.pause();
        const rows = ctx.db.collectors
          .filter(
            (item) =>
              item.enterprise_id === ctx.enterpriseId() &&
              !["uninstalled", "uninstalling"].includes(item.status) &&
              canUseMonitoringCollector(ctx, item),
          )
          .map((item) => ({
            id: item.id,
            resource: { kind: item.resource_type, id: item.resource_id },
            name:
              (item.resource_type === "host"
                ? ctx.db.hosts.find((v) => v.id === item.resource_id)?.name
                : ctx.db.clusters.find((v) => v.id === item.resource_id)
                    ?.name) ?? item.resource_id,
            platform: item.platform,
            status: item.status,
            version: item.version,
            collector_version: collectorVersion,
            distribution_version: collectorDistributionVersion,
            components: [
              ...(collectorComponents[
                item.platform as keyof typeof collectorComponents
              ] ?? []),
            ],
            support_status: "supported" as const,
          }))
          .filter((row) =>
            row.name.toLowerCase().includes(input.search?.toLowerCase() ?? ""),
          );
        return page(ctx, rows, input);
      },
      async integrations() {
        permission(ctx, "monitoring.read");
        await ctx.pause();
        return structuredClone(
          monitoringIntegrations,
        ) as unknown as MonitoringSchemas["MonitoringIntegration"][];
      },
      async collections(input = {}) {
        permission(ctx, "monitoring.read");
        await ctx.pause();
        return page(
          ctx,
          (state(ctx).collections ?? []).filter(
            (item) =>
              item.status === "active" &&
              canReadMonitoringCollection(ctx, item) &&
              (!input.integration || item.integration === input.integration) &&
              (input.desired_enabled === undefined ||
                item.desired_enabled === input.desired_enabled) &&
              (!input.apply_status ||
                item.apply_status === input.apply_status) &&
              (!input.health || item.health === input.health) &&
              (!input.freshness || item.freshness === input.freshness) &&
              item.name
                .toLowerCase()
                .includes(input.search?.toLowerCase() ?? ""),
          ),
          input,
        );
      },
      async collection(id) {
        permission(ctx, "monitoring.read");
        await ctx.pause();
        const item = state(ctx).collections?.find((row) => row.id === id);
        if (!item || !canReadMonitoringCollection(ctx, item))
          fail("MONITORING_DENIED", 403);
        return copy(item);
      },
      async targets(input = {}) {
        permission(ctx, "monitoring.read");
        await ctx.pause();
        return page(
          ctx,
          (state(ctx).targets ?? []).filter(
            (item) =>
              item.status === "active" &&
              dashboardObjectGrant(ctx, "monitoring_target", item.id) &&
              (!input.integration || item.engine === input.integration) &&
              item.name
                .toLowerCase()
                .includes(input.search?.toLowerCase() ?? ""),
          ),
          input,
        );
      },
      async applications(input = {}) {
        permission(ctx, "application.read");
        if (
          input.environment &&
          !/^[a-z][a-z0-9_-]{0,63}$/.test(input.environment)
        )
          fail();
        await ctx.pause();
        const search = input.search?.trim().toLowerCase() ?? "";
        return page(
          ctx,
          state(ctx)
            .applications.filter(
              (a) =>
                a.status === (input.status ?? "active") &&
                (!input.environment || a.environment === input.environment) &&
                dashboardObjectGrant(ctx, "application", a.id) &&
                (a.name.toLowerCase().includes(search) ||
                  a.application_key.toLowerCase().includes(search)),
            )
            .sort((a, b) => a.id.localeCompare(b.id)),
          input,
        );
      },
      async application(id) {
        permission(ctx, "application.read");
        await ctx.pause();
        return copy(app(ctx, id));
      },
      async services(input = {}) {
        permission(ctx, "application.read");
        await ctx.pause();
        for (const id of input.application_ids ?? []) app(ctx, id);
        const search = input.search?.trim().toLowerCase() ?? "";
        return page(
          ctx,
          state(ctx)
            .services.filter(
              (s) =>
                s.status === "active" &&
                dashboardObjectGrant(ctx, "application", s.application_id) &&
                (input.application_ids === undefined ||
                  input.application_ids.includes(s.application_id)) &&
                `${s.service_namespace}/${s.service_name}`
                  .toLowerCase()
                  .includes(search),
            )
            .sort((a, b) => a.id.localeCompare(b.id)),
          input,
        );
      },
      async preview(request) {
        const input = monitoringRequest(request);
        await ctx.pause();
        validate(ctx, input);
        const m = copy(input);
        const credentialPrepared =
          m.kind === "collection" &&
          m.collection?.credential_update === "replace";
        delete m.credentials;
        if (m.operation === "create") m.id = crypto.randomUUID();
        return ctx.createPendingAction({
          tool: `monitoring.${m.kind}.${m.operation}`,
          title:
            m.application?.name ??
            m.service?.service_name ??
            m.collection?.name ??
            m.kind,
          input_data: {
            ...m,
            credential_prepared: credentialPrepared,
          } as unknown as Record<string, unknown>,
        });
      },
    },
  };
}
function ingestHelpers(ctx: BaseContext): IngestHelpers {
  return {
    state: () => state(ctx),
    application: (id) => app(ctx, id),
    permission: (name) => permission(ctx, name),
    page: (rows, input) => page(ctx, rows, input),
  };
}
export function commitMonitoringAction(
  ctx: BaseContext,
  action: PendingActionPublic,
): ConfirmActionResult | undefined {
  const plan = ctx.db.actionPlans[action.action_ref];
  if (!plan?.tool.startsWith("monitoring.")) return undefined;
  const m = plan.input_data as unknown as MonitoringMutation;
  validate(ctx, m, Boolean(plan.input_data.credential_prepared));
  const data = state(ctx);
  if (m.kind === "service") {
    data.services.push({
      id: m.id!,
      application_id: m.service!.application_id,
      service_name: m.service!.service_name,
      service_namespace: m.service!.service_namespace ?? "",
      runtime: "",
      status: "active",
      version: 1,
    });
  } else if (m.kind === "endpoint" || m.kind === "ingest_key") {
    commitMonitoringIngest(ctx, ingestHelpers(ctx), m);
  } else if (m.kind === "collection") {
    commitMonitoringCollection(
      ctx,
      m,
      data,
      Boolean(plan.input_data.credential_prepared),
    );
  } else if (m.kind === "application") {
    if (m.operation === "create") {
      const input = m.application!;
      if (
        data.applications.some(
          (a) =>
            a.id === m.id ||
            (input.application_key &&
              a.application_key === input.application_key),
        )
      )
        fail("MONITORING_VERSION_CONFLICT", 409);
      data.applications.push({
        id: m.id!,
        application_key: input.application_key ?? m.id!,
        name: input.name,
        environment: input.environment!,
        description: input.description ?? "",
        owner_name: input.owner_name ?? "",
        status: "active",
        version: 1,
        created_at: ctx.nowIso(),
        updated_at: ctx.nowIso(),
      });
      (ctx.db.dataAuthorizationGrants ??= []).push({
        subject_type: "user",
        subject_id: ctx.actor().id,
        resource_type: "application",
        resource_id: m.id!,
        active: true,
      });
      const versions = (ctx.db.dataAuthorizationVersions ??= {});
      const key = `user:${ctx.actor().id}`;
      versions[key] = (versions[key] ?? 1) + 1;
    } else {
      const row = app(ctx, m.id!);
      if (m.operation === "archive") row.status = "archived";
      else {
        row.name = m.application!.name;
        row.description = m.application!.description ?? "";
        row.owner_name = m.application!.owner_name ?? "";
      }
      row.version++;
      row.updated_at = ctx.nowIso();
    }
  } else if (m.operation === "create")
    data.bindings.push({ ...m.binding!, id: m.id!, version: 1, active: true });
  else {
    const binding = data.bindings.find((v) => v.id === m.id && v.active);
    if (!binding || binding.version !== m.expected_version)
      fail("MONITORING_VERSION_CONFLICT", 409);
    binding.active = false;
    binding.version++;
  }
  action.status = "succeeded";
  action.available_actions = [];
  action.updated_at = ctx.nowIso();
  ctx.audit(plan.tool, {
    summary: action.title,
    resourceType: m.kind,
    resourceId: m.id,
  });
  ctx.save();
  return { pending_action: copy(action) };
}
