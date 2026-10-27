import type {
  KubernetesInventoryScope,
  KubernetesStreamSession,
  KubernetesWorkbenchDomains,
} from "../kubernetes-workbench";
import type { MockContext } from "./context";
import type { MockKubernetesInventory } from "./kubernetes-workbench";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";

type Methods = Pick<
  KubernetesWorkbenchDomains["kubernetesWorkbench"],
  | "createStreamSession"
  | "getStreamSession"
  | "closeStreamSession"
  | "issueStreamTicket"
>;
type Entry = {
  session: KubernetesStreamSession;
  actor: string;
  http?: string;
  scope: string;
};
const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" ? (value as Record<string, unknown>) : {};
function fail(): never {
  throw new ApiError(
    {
      code: "KUBERNETES_API_FORBIDDEN",
      message_key: "kubernetesWorkbench.errors.forbidden",
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    403,
  );
}
export function createKubernetesStreamDomains(
  ctx: MockContext,
  state: (id: string) => MockKubernetesInventory,
  scope: (id: string) => KubernetesInventoryScope,
): Methods {
  const entries = new Map<string, Entry>();
  const permission = (mode: string) => {
    const p = resolvePermissions(
      ctx.db,
      ctx.enterpriseId(),
      ctx.actor().id,
      ctx.nowIso(),
    );
    if (
      !p.has("*") &&
      !p.has(mode === "exec" ? "kubernetes.exec" : "kubernetes.port_forward")
    )
      fail();
  };
  function current(cluster: string, id: string) {
    const entry = entries.get(id);
    if (
      !entry ||
      entry.actor !== ctx.actor().id ||
      entry.http !== ctx.db.session.id ||
      entry.session.cluster_id !== cluster
    )
      fail();
    permission(entry.session.mode);
    if (entry.scope !== JSON.stringify(scope(cluster))) fail();
    return entry;
  }
  function ticket(session: KubernetesStreamSession) {
    const bytes = crypto.getRandomValues(new Uint8Array(32));
    return {
      session: structuredClone(session),
      ticket: btoa(String.fromCharCode(...bytes))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replaceAll("=", ""),
      ticket_expires_at: new Date(Date.now() + 30000).toISOString(),
      protocol_version: "argus.kubernetes_stream/v1" as const,
      websocket_url: `wss://argus-mock.invalid/v1/kubernetes/sessions/${session.id}`,
    };
  }
  return {
    async createStreamSession(cluster, input) {
      await ctx.pause();
      permission(input.mode);
      const access = scope(cluster),
        ns = state(cluster).namespaces.find((v) => v.id === input.namespace_id);
      if (
        !ns ||
        (!access.cluster_wide &&
          !access.namespaces.some((v) => v.id === ns.id)) ||
        !input.reason.trim()
      )
        fail();
      const object = state(cluster).objects.find(
        (v) =>
          v.definition.key === input.type &&
          v.uid === input.uid &&
          v.name === input.name &&
          v.namespace_uid === ns.uid,
      );
      if (!object) fail();
      const pod =
        input.type === "pod"
          ? object
          : state(cluster).objects.find(
              (v) => v.definition.key === "pod" && v.namespace_uid === ns.uid,
            );
      if (!pod) fail();
      const containers = record(pod.data.status).containerStatuses;
      const statuses = Array.isArray(containers) ? containers : [];
      const c = record(
        statuses.find(
          (v) => !input.container || record(v).name === input.container,
        ),
      );
      if (!c.containerID) fail();
      if (entries.size >= 10) fail();
      const session: KubernetesStreamSession = {
        id: crypto.randomUUID(),
        cluster_id: cluster,
        namespace_id: ns.id,
        namespace: ns.name,
        namespace_uid: ns.uid,
        pod: pod.name,
        pod_uid: pod.uid,
        container: String(c.name),
        restart_count: Number(c.restartCount ?? 0),
        mode: input.mode,
        status: "pending",
        version: 1,
        expires_at: new Date(Date.now() + 900000).toISOString(),
        bytes_in: 0,
        bytes_out: 0,
        ...(input.mode === "exec"
          ? { shell: input.shell ?? "sh" }
          : {
              port: input.port,
              service: input.type === "service" ? object.name : undefined,
            }),
      };
      entries.set(session.id, {
        session,
        actor: ctx.actor().id,
        http: ctx.db.session.id,
        scope: JSON.stringify(access),
      });
      return ticket(session);
    },
    async getStreamSession(cluster, id) {
      await ctx.pause();
      return structuredClone(current(cluster, id).session);
    },
    async closeStreamSession(cluster, id) {
      await ctx.pause();
      const e = current(cluster, id);
      e.session.status = "closed";
      e.session.version++;
      return structuredClone(e.session);
    },
    async issueStreamTicket(cluster, id) {
      await ctx.pause();
      const e = current(cluster, id);
      if (e.session.status !== "pending" || e.session.expires_at < ctx.nowIso())
        fail();
      return ticket(e.session);
    },
  };
}
