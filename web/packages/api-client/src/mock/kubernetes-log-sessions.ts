import type {
  KubernetesInventoryScope,
  KubernetesLogSession,
  KubernetesWorkbenchDomains,
} from "../kubernetes-workbench";
import type { MockContext } from "./context";
import type { MockKubernetesInventory } from "./kubernetes-workbench";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";

type Entry = {
  session: KubernetesLogSession;
  actor: string;
  http: string;
  scope: string;
  version: number;
  containerId: string;
  previousId: string;
  ticket?: string;
  ticketExpiry: number;
  cursors: Map<string, number>;
};
type LogMethods = Pick<
  KubernetesWorkbenchDomains["kubernetesWorkbench"],
  | "createLogSession"
  | "attachLogSession"
  | "readLogSession"
  | "closeLogSession"
  | "getLogSession"
>;
const record = (v: unknown): Record<string, unknown> =>
  v && typeof v === "object" ? (v as Record<string, unknown>) : {};
export function createKubernetesLogSessionDomains(
  ctx: MockContext,
  state: (id: string) => MockKubernetesInventory,
  scope: (id: string) => KubernetesInventoryScope,
): LogMethods {
  const sessions = new Map<string, Entry>();
  function fail(key = "forbidden", status = 403): never {
    throw new ApiError(
      {
        code:
          key === "identityChanged"
            ? "KUBERNETES_OBJECT_IDENTITY_CHANGED"
            : "KUBERNETES_LOG_SESSION_REJECTED",
        message_key: `kubernetesWorkbench.errors.${key}`,
        request_id: crypto.randomUUID(),
        retryable: false,
      },
      status,
    );
  }
  function permissions() {
    if (!ctx.db.session.id || !ctx.db.session.userId) fail();
    const p = resolvePermissions(
      ctx.db,
      ctx.enterpriseId(),
      ctx.actor().id,
      ctx.nowIso(),
    );
    if (!p.has("*") && !p.has("kubernetes.operational_logs.read")) fail();
  }
  function generation(
    id: string,
    namespace: string,
    pod: string,
    uid: string,
    container: string,
  ) {
    const data = state(id),
      ns = data.namespaces.find((n) => n.id === namespace),
      object = data.objects.find(
        (o) =>
          o.definition.key === "pod" &&
          o.uid === uid &&
          o.name === pod &&
          o.namespace_uid === ns?.uid,
      );
    if (!ns || !object) fail("identityChanged", 409);
    const status = record(object.data.status);
    const statuses = [
      status.containerStatuses,
      status.initContainerStatuses,
      status.ephemeralContainerStatuses,
    ].flatMap((v) => (Array.isArray(v) ? v : []));
    const c = record(statuses.find((v) => record(v).name === container));
    if (typeof c.containerID !== "string" || typeof c.restartCount !== "number")
      fail("identityChanged", 409);
    return {
      ns,
      containerId: c.containerID as string,
      restarts: c.restartCount as number,
      previousId: String(
        record(record(c.lastState).terminated).containerID ?? "",
      ),
    };
  }
  function current(cluster: string, id: string) {
    permissions();
    const entry = sessions.get(id),
      access = scope(cluster);
    if (
      !entry ||
      entry.actor !== ctx.actor().id ||
      entry.http !== ctx.db.session.id ||
      entry.session.cluster_id !== cluster ||
      entry.session.expires_at <= ctx.nowIso() ||
      entry.version !==
        (ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1) ||
      entry.scope !== JSON.stringify(access)
    )
      fail();
    const c = generation(
      cluster,
      entry.session.namespace_id,
      entry.session.pod,
      entry.session.pod_uid,
      entry.session.container,
    );
    if (
      c.containerId !== entry.containerId ||
      c.restarts !== entry.session.restart_count ||
      c.ns.uid !== entry.session.namespace_uid ||
      (entry.session.previous && c.previousId !== entry.previousId)
    ) {
      entry.session.status = "invalidated";
      fail("identityChanged", 409);
    }
    return entry;
  }
  return {
    async getLogSession(cluster, id) {
      await ctx.pause();
      const e = current(cluster, id);
      if (e.session.status !== "active") fail();
      return structuredClone(e.session);
    },
    async createLogSession(cluster, input) {
      await ctx.pause();
      permissions();
      const access = scope(cluster),
        g = generation(
          cluster,
          input.namespace_id,
          input.pod,
          input.pod_uid,
          input.container,
        );
      if (
        !access.cluster_wide &&
        !access.namespaces.some((n) => n.id === input.namespace_id)
      )
        fail();
      if (
        input.tail_lines < 1 ||
        input.tail_lines > 1000 ||
        input.since_seconds < 1 ||
        input.since_seconds > 86400 ||
        (input.previous && (g.restarts < 1 || !g.previousId))
      )
        fail("invalid", 400);
      for (const [id, e] of sessions)
        if (e.session.expires_at <= ctx.nowIso()) sessions.delete(id);
      if (
        [...sessions.values()].filter(
          (e) =>
            e.actor === ctx.actor().id &&
            ["pending", "active"].includes(e.session.status),
        ).length >= 10
      )
        fail("budget", 413);
      const ticket = btoa(
        String.fromCharCode(...crypto.getRandomValues(new Uint8Array(32))),
      )
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replaceAll("=", "");
      const now = Date.now(),
        session: KubernetesLogSession = {
          id: crypto.randomUUID(),
          cluster_id: cluster,
          namespace_id: input.namespace_id,
          namespace: g.ns.name,
          namespace_uid: g.ns.uid,
          pod: input.pod,
          pod_uid: input.pod_uid,
          container: input.container,
          restart_count: g.restarts,
          previous: input.previous,
          status: "pending",
          version: 1,
          expires_at: new Date(now + 15 * 60000).toISOString(),
        };
      const entry: Entry = {
        session,
        actor: ctx.actor().id,
        http: ctx.db.session.id ?? "",
        version:
          ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1,
        scope: JSON.stringify(access),
        containerId: g.containerId,
        previousId: g.previousId,
        ticket,
        ticketExpiry: now + 30000,
        cursors: new Map(),
      };
      sessions.set(session.id, entry);
      return {
        session: structuredClone(session),
        ticket,
        ticket_expires_at: new Date(entry.ticketExpiry).toISOString(),
      };
    },
    async attachLogSession(cluster, id, ticket) {
      await ctx.pause();
      const e = current(cluster, id);
      if (
        e.session.status !== "pending" ||
        e.ticket !== ticket ||
        Date.now() >= e.ticketExpiry
      )
        fail("mutationConflict", 409);
      delete e.ticket;
      e.session.status = "active";
      e.session.version++;
      return structuredClone(e.session);
    },
    async readLogSession(cluster, id, cursor, signal) {
      await ctx.pause();
      if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
      const e = current(cluster, id);
      if (e.session.status !== "active") fail();
      const count = cursor ? e.cursors.get(cursor) : 0;
      if (count === undefined) fail("invalid", 400);
      const next = crypto.randomUUID();
      e.cursors.set(next, count + 1);
      if (e.cursors.size > 128)
        e.cursors.delete(e.cursors.keys().next().value!);
      return {
        content: `${ctx.nowIso()} mock ${e.session.container} ${e.session.previous ? "previous" : "current"} log ${count + 1}; token=[REDACTED]\n`,
        next_cursor: next,
        observed_at: ctx.nowIso(),
        truncated: false,
        pending_line: false,
        redacted: true,
        complete: e.session.previous,
      };
    },
    async closeLogSession(cluster, id) {
      await ctx.pause();
      const e = current(cluster, id);
      e.session.status = "closed";
      e.session.version++;
      delete e.ticket;
      e.cursors.clear();
      return structuredClone(e.session);
    },
  };
}
