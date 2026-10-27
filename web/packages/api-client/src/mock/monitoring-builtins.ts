import { builtinCollections } from "../generated/builtin-collections";
import { collectorComponents } from "../generated/collector-registry";
import type {
  MonitoringDomains,
  MonitoringMutation,
  MonitoringSchemas,
} from "../monitoring";
import type { MockMonitoringState } from "./monitoring";
import type { MockContext, BaseContext } from "./context";
import { dashboardObjectGrant } from "./dashboard-state";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";

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
const idFor = (kind: string, id: string, key: string) =>
  `builtin-${kind}-${id}-${key}`;
export const isBuiltinCollection = (key?: string) =>
  builtinCollections.some((entry) => entry.key === key);

export function validateBuiltinCollection(
  ctx: BaseContext,
  m: MonitoringMutation,
  data: MockMonitoringState,
) {
  const input = m.collection,
    definition = builtinCollections.find(
      (entry) => entry.key === input?.integration,
    );
  permit(ctx, "telemetry.collector.manage");
  if (
    !input ||
    !definition ||
    m.operation === "archive" ||
    !input.target ||
    input.application_id ||
    input.service_id ||
    input.bindings?.length ||
    input.target_identity ||
    m.credentials ||
    (input.credential_update && input.credential_update !== "keep")
  )
    fail();
  if (
    !definition.resource_types.some((type) => type === input.target!.kind) ||
    !dashboardObjectGrant(ctx, input.target.kind, input.target.id)
  )
    fail("MONITORING_DENIED", 403);
  const collector = ctx.db.collectors.find(
    (row) =>
      row.id === input.collector_id && row.enterprise_id === ctx.enterpriseId(),
  );
  if (
    !collector ||
    collector.resource_type !== input.target.kind ||
    collector.resource_id !== input.target.id ||
    ["uninstalled", "uninstalling"].includes(collector.status)
  )
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  const components =
    collectorComponents[collector.platform as keyof typeof collectorComponents];
  if (
    !definition.platforms.some((platform) => platform === collector.platform) ||
    !components.some((component) => component === definition.component)
  )
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  const existing = data.collections?.find(
    (row) =>
      row.id ===
      idFor(input.target!.kind, input.target!.id, input.integration!),
  );
  if (
    (m.operation === "create" && existing) ||
    (m.operation !== "create" &&
      (!existing || existing.version !== m.expected_version))
  )
    fail("MONITORING_VERSION_CONFLICT", 409);
  const interval = input.config?.interval ?? existing?.config.interval ?? "30s";
  const match = /^(\d+(?:\.\d+)?)(s|m|h)$/.exec(String(interval));
  const seconds = match
    ? Number(match[1]) * ({ s: 1, m: 60, h: 3600 }[match[2]!] ?? 0)
    : 0;
  if (
    seconds < 5 ||
    seconds > 3600 ||
    Object.keys(input.config ?? {}).some(
      (key) => key !== "interval" && key !== "component_tls",
    )
  )
    fail();
  const trust = input.config?.component_tls;
  if (trust !== undefined) {
    if (
      !definition.component_tls_supported ||
      !trust ||
      typeof trust !== "object" ||
      Array.isArray(trust)
    )
      fail();
    const fields = trust as Record<string, unknown>;
    if (
      Object.keys(fields).some(
        (key) => key !== "server_name" && key !== "ca_pem",
      )
    )
      fail();
    const serverName = fields.server_name,
      ca = fields.ca_pem;
    if (
      serverName !== undefined &&
      (typeof serverName !== "string" ||
        serverName.length > 253 ||
        /[\s/$\\]/.test(serverName))
    )
      fail();
    if (
      ca !== undefined &&
      (typeof ca !== "string" ||
        ca.length > 262144 ||
        ca.includes("PRIVATE KEY") ||
        (ca.trim() &&
          !/^-----BEGIN CERTIFICATE-----[\s\S]+-----END CERTIFICATE-----\s*$/.test(
            ca,
          )))
    )
      fail();
  }
}

export function createBuiltinDomains(
  ctx: MockContext,
  state: () => MockMonitoringState,
): Pick<MonitoringDomains["monitoring"], "builtins" | "previewBuiltin"> {
  return {
    async builtins(target) {
      permit(ctx, "telemetry.collector.read");
      if (
        !["host", "kubernetes_cluster"].includes(target.kind) ||
        !dashboardObjectGrant(ctx, target.kind, target.id)
      )
        fail("MONITORING_DENIED", 403);
      await ctx.pause();
      const collector = ctx.db.collectors.find(
        (row) =>
          row.resource_type === target.kind &&
          row.resource_id === target.id &&
          row.enterprise_id === ctx.enterpriseId() &&
          !["uninstalled", "uninstalling"].includes(row.status),
      );
      return builtinCollections
        .filter((entry) =>
          entry.resource_types.some((type) => type === target.kind),
        )
        .map((definition) => {
          const id = idFor(target.kind, target.id, definition.key),
            row = state().collections?.find((row) => row.id === id);
          const available =
            !!collector &&
            definition.platforms.some(
              (value) => value === collector.platform,
            ) &&
            collectorComponents[
              collector.platform as keyof typeof collectorComponents
            ].some((component) => component === definition.component);
          return {
            definition: {
              ...definition,
              resource_types: [...definition.resource_types],
              platforms: [...definition.platforms],
            },
            id,
            collector_id: collector?.id,
            collection: row ? structuredClone(row) : undefined,
            available,
            reason: available
              ? undefined
              : collector
                ? "component_unavailable"
                : "collector_not_installed",
          } as MonitoringSchemas["MonitoringBuiltin"];
        });
    },
    async previewBuiltin(input) {
      await ctx.pause();
      const data = state(),
        definition = builtinCollections.find(
          (entry) => entry.key === input.key,
        );
      if (!definition) fail();
      const collector = ctx.db.collectors.find(
        (row) =>
          row.resource_type === input.target.kind &&
          row.resource_id === input.target.id &&
          row.enterprise_id === ctx.enterpriseId(),
      );
      if (!collector) fail("MONITORING_UNAVAILABLE", 503);
      const id = idFor(input.target.kind, input.target.id, input.key),
        existing = data.collections?.find((row) => row.id === id);
      if (input.expected_version !== (existing?.version ?? 0))
        fail("MONITORING_VERSION_CONFLICT", 409);
      const mutation: MonitoringMutation = {
        kind: "collection",
        operation: existing ? "update" : "create",
        id,
        expected_version: existing?.version,
        collection: {
          name: definition.name,
          integration: input.key,
          target: input.target,
          collector_id: collector.id,
          config: input.config ?? existing?.config ?? { interval: "30s" },
          enabled: input.enabled,
          credential_update: "keep",
        },
      };
      validateBuiltinCollection(ctx, mutation, data);
      return ctx.createPendingAction({
        tool: `monitoring.collection.${mutation.operation}`,
        title: definition.name,
        input_data: mutation as unknown as Record<string, unknown>,
      });
    },
  };
}
