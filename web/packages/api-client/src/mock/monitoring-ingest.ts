import type {
  MonitoringApplication,
  MonitoringDomains,
  MonitoringListInput,
  MonitoringMutation,
  MonitoringSchemas,
} from "../monitoring";
import type { MockMonitoringState } from "./monitoring";
import type { BaseContext, MockContext } from "./context";
import { ApiError } from "../transport/errors";
import { dashboardObjectGrant } from "./dashboard-state";

export type IngestHelpers = {
  state: () => MockMonitoringState;
  application: (id: string) => MonitoringApplication;
  permission: (name: string) => void;
  page: <T extends { id: string }>(
    rows: T[],
    input: MonitoringListInput,
  ) => { items: T[]; next_cursor?: string };
};
type Endpoint = MonitoringSchemas["MonitoringIngestEndpoint"];
// Mock mode keeps unrevealed values only in memory, never localStorage. A reload
// discards them; real mode uses the authenticated server's one-time reveal API.
const unrevealed = new Map<string, string>();
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
function endpoint(h: IngestHelpers, id?: string): Endpoint {
  const row = h.state().endpoints?.find((item) => item.id === id);
  if (!row) fail("MONITORING_NOT_FOUND", 404);
  h.application(row.application_id);
  return row;
}
export function validateMonitoringIngest(
  ctx: BaseContext,
  h: IngestHelpers,
  m: MonitoringMutation,
) {
  h.permission("application.manage");
  if (m.application || m.binding || m.collection || m.credentials) fail();
  if (m.kind === "ingest_key") {
    if (!m.ingest_key || m.endpoint) fail();
    if (m.operation === "revoke") {
      const row = h.state().ingestKeys?.find((v) => v.id === m.id);
      if (!row || row.version !== m.expected_version || row.revoked_at)
        fail("MONITORING_VERSION_CONFLICT", 409);
      endpoint(h, row.endpoint_id);
      return;
    }
    if (m.operation !== "create") fail();
    const row = endpoint(h, m.ingest_key.endpoint_id),
      expiry = Date.parse(m.ingest_key.expires_at ?? "");
    if (
      !row.enabled ||
      !Number.isFinite(expiry) ||
      expiry < Date.now() + 60000 ||
      expiry > Date.now() + 366 * 86400000
    )
      fail();
    if (
      (h.state().ingestKeys ?? []).filter(
        (v) =>
          v.endpoint_id === row.id &&
          !v.revoked_at &&
          Date.parse(v.expires_at ?? "") > Date.now(),
      ).length >= 4
    )
      fail("MONITORING_DEPENDENCIES_EXIST", 409);
    return;
  }
  if (!m.endpoint || m.ingest_key) fail();
  const current = m.operation === "create" ? undefined : endpoint(h, m.id);
  if (current && current.version !== m.expected_version)
    fail("MONITORING_VERSION_CONFLICT", 409);
  if (m.operation === "set_enabled") return;
  if (!["create", "update"].includes(m.operation)) fail();
  const input = m.endpoint;
  if (input.protocol === "skywalking" && input.enabled)
    fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
  if (
    !input.name?.trim() ||
    !input.application_id ||
    !input.collector_id ||
    !input.protocol ||
    !input.signals?.length
  )
    fail();
  if (h.application(input.application_id).status !== "active") fail();
  h.permission("collector.use");
  const collector = ctx.db.collectors.find(
    (v) =>
      v.id === input.collector_id && v.enterprise_id === ctx.enterpriseId(),
  );
  if (
    !collector ||
    !dashboardObjectGrant(ctx, collector.resource_type, collector.resource_id)
  )
    fail("MONITORING_DENIED", 403);
  if (input.protocol !== "otlp" && input.signals.some((v) => v !== "traces"))
    fail();
  if (
    current &&
    (current.application_id !== input.application_id ||
      current.collector_id !== input.collector_id ||
      current.protocol !== input.protocol)
  )
    fail();
}
export function commitMonitoringIngest(
  ctx: BaseContext,
  h: IngestHelpers,
  m: MonitoringMutation,
) {
  const data = h.state();
  if (m.kind === "endpoint") {
    if (m.operation === "create")
      (data.endpoints ??= []).push({
        ...m.endpoint,
        id: m.id!,
        version: 1,
        last_error_code: "",
        service_allowlist: m.endpoint?.service_allowlist ?? [],
      } as Endpoint);
    else {
      const row = endpoint(h, m.id);
      if (m.operation === "update") {
        row.name = m.endpoint!.name!;
        row.service_allowlist = m.endpoint!.service_allowlist ?? [];
      }
      row.enabled = m.endpoint!.enabled ?? false;
      row.version++;
    }
  } else if (m.operation === "revoke") {
    const row = data.ingestKeys!.find((v) => v.id === m.id)!;
    row.revoked_at = ctx.nowIso();
    row.version++;
    unrevealed.delete(row.id);
  } else {
    const id = m.id!,
      bytes = crypto.getRandomValues(new Uint8Array(32));
    const secret = btoa(String.fromCharCode(...bytes))
      .replaceAll("+", "-")
      .replaceAll("/", "_")
      .replaceAll("=", "");
    unrevealed.set(id, `argus_app.${id}.${secret}`);
    (data.ingestKeys ??= []).push({
      id,
      endpoint_id: m.ingest_key!.endpoint_id!,
      prefix: `argus_app.${id}`,
      expires_at: m.ingest_key!.expires_at,
      revealed: false,
      created_at: ctx.nowIso(),
      version: 1,
      creator_id: ctx.actor().id,
    });
  }
}
export function createIngestDomains(
  ctx: MockContext,
  h: IngestHelpers,
): Pick<
  MonitoringDomains["monitoring"],
  "ingestEndpoints" | "ingestKeys" | "revealIngestKey" | "ingestConnection"
> {
  return {
    async ingestConnection(id) {
      h.permission("application.read");
      const row = endpoint(h, id),
        app = h.application(row.application_id);
      if (row.protocol === "skywalking")
        fail("MONITORING_UNSUPPORTED_INTEGRATION", 409);
      await ctx.pause();
      // Explicit mock mode provides configuration examples, never a real
      // trust anchor or evidence of an installed listener.
      const pem =
        "-----BEGIN CERTIFICATE-----\nMOCK_MODE_NOT_A_REAL_TRUST_ANCHOR\n-----END CERTIFICATE-----\n";
      const digest = await crypto.subtle.digest(
        "SHA-256",
        new TextEncoder().encode(pem),
      );
      const hash = Array.from(new Uint8Array(digest), (v) =>
        v.toString(16).padStart(2, "0"),
      ).join("");
      const serverName = `collector-${row.collector_id}.argus.telemetry`;
      const ports =
        row.protocol === "otlp"
          ? [
              { transport: "grpc" as const, port: 24317, path: "" },
              { transport: "http" as const, port: 24318, path: "" },
            ]
          : [
              { transport: "grpc" as const, port: 24250, path: "" },
              {
                transport: "thrift_http" as const,
                port: 24268,
                path: "/api/traces",
              },
            ];
      return {
        name: row.name,
        endpoint_id: row.id,
        application_id: app.id,
        collector_id: row.collector_id,
        environment: app.environment,
        protocol: row.protocol as "otlp" | "jaeger",
        signals: row.signals,
        desired_enabled: row.enabled,
        configuration_status: "unknown",
        collector_status: "mock",
        application_status: app.status,
        observation_state:
          row.last_rejected_at &&
          (!row.last_received_at || row.last_rejected_at > row.last_received_at)
            ? "rejected"
            : row.last_received_at
              ? "received"
              : "waiting",
        last_received_at: row.last_received_at,
        last_rejected_at: row.last_rejected_at,
        last_error_code: row.last_error_code,
        authentication_header: "authorization",
        authentication_scheme: "Bearer",
        service_name_required: true,
        network_prerequisite:
          "resolve_collector_dns_or_use_local_address_with_tls_server_name",
        listeners: ports.map((port) => ({
          ...port,
          endpoint: `${port.transport === "grpc" ? "grpcs" : "https"}://${serverName}:${port.port}${port.path}`,
          local_endpoint: `${port.transport === "grpc" ? "grpcs" : "https"}://127.0.0.1:${port.port}${port.path}`,
        })),
        tls: {
          required: true,
          minimum_version: "1.3",
          server_name: serverName,
          trust_bundle_epoch: 1,
          ca_bundle_pem: pem,
          ca_bundle_sha256: hash,
          ca_fingerprints: [hash],
        },
      };
    },
    async ingestEndpoints(id, input = {}) {
      h.permission("application.read");
      h.application(id);
      await ctx.pause();
      return h.page(
        (h.state().endpoints ?? []).filter((v) => v.application_id === id),
        input,
      );
    },
    async ingestKeys(id, input = {}) {
      h.permission("application.read");
      endpoint(h, id);
      await ctx.pause();
      return h.page(
        (h.state().ingestKeys ?? [])
          .filter((v) => v.endpoint_id === id)
          .map((value) => {
            const key = structuredClone(value);
            Reflect.deleteProperty(key, "creator_id");
            return key;
          }),
        input,
      );
    },
    async revealIngestKey(id) {
      h.permission("application.manage");
      const row = h.state().ingestKeys?.find((v) => v.id === id);
      if (!row) fail("MONITORING_NOT_FOUND", 404);
      endpoint(h, row.endpoint_id);
      if (
        row.creator_id !== ctx.actor().id ||
        row.revealed ||
        row.revoked_at ||
        Date.now() - Date.parse(row.created_at) > 600000 ||
        !unrevealed.has(id)
      )
        fail("MONITORING_VERSION_CONFLICT", 409);
      const key = unrevealed.get(id)!;
      unrevealed.delete(id);
      row.revealed = true;
      ctx.audit("application.ingest_key.reveal", {
        summary: "Application ingestion key revealed",
        resourceId: id,
        resourceType: "ingest_key",
      });
      ctx.save();
      return { key };
    },
  };
}
