import { ensureKubernetesInventory } from "./kubernetes-seed";
import { createKubernetesLogSessionDomains } from "./kubernetes-log-sessions";
import { createKubernetesStreamDomains } from "./kubernetes-stream-sessions";
import { createKubernetesHelmDomains } from "./kubernetes-helm";
import type { KubernetesHelmRelease } from "../kubernetes-workbench";
import type {
  KubernetesWorkbenchDomains,
  KubernetesInventoryObject,
  KubernetesInventoryDefinition,
  KubernetesInventoryScope,
  KubernetesInventoryInput,
} from "../kubernetes-workbench";
import { kubernetesInventoryTypes } from "../generated/kubernetes-inventory-registry";
import type { MockContext } from "./context";
import { dashboardObjectGrant } from "./dashboard-state";
import { resolvePermissions } from "./permissions";
import { ApiError } from "../transport/errors";
import { createKubernetesMutationDomains } from "./kubernetes-mutations";
import type { KubernetesOperation } from "../kubernetes-workbench";

export type MockKubernetesInventory = {
  helm?: KubernetesHelmRelease[];
  operations?: KubernetesOperation[];
  namespaces: KubernetesInventoryScope["namespaces"];
  objects: KubernetesInventoryObject[];
};
function fail(code = "KUBERNETES_INVENTORY_INVALID", status = 400): never {
  throw new ApiError(
    {
      code,
      message_key:
        "kubernetesWorkbench.errors." +
        ({
          KUBERNETES_API_FORBIDDEN: "forbidden",
          KUBERNETES_OBJECT_IDENTITY_CHANGED: "identityChanged",
        }[code] ?? "invalid"),
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}
export function createKubernetesWorkbenchDomains(
  ctx: MockContext,
): KubernetesWorkbenchDomains {
  const watches = new Map<
    string,
    {
      cluster: string;
      actor: string;
      version: number;
      scope: string;
      signature: string;
      expires: number;
    }
  >();
  const definitions = structuredClone(
    kubernetesInventoryTypes,
  ) as unknown as KubernetesInventoryDefinition[];
  const state = (id: string) => ensureKubernetesInventory(ctx, id);
  const scope = (id: string): KubernetesInventoryScope => {
    const permissions = resolvePermissions(
      ctx.db,
      ctx.enterpriseId(),
      ctx.actor().id,
      ctx.nowIso(),
    );
    if (!permissions.has("*") && !permissions.has("kubernetes.read"))
      fail("KUBERNETES_API_FORBIDDEN", 403);
    const cluster = ctx.db.clusters.find(
      (v) => v.id === id && v.enterpriseId === ctx.enterpriseId(),
    );
    if (!cluster) fail("KUBERNETES_API_FORBIDDEN", 403);
    const clusterWide = dashboardObjectGrant(ctx, "kubernetes_cluster", id),
      namespaces = clusterWide
        ? []
        : state(id).namespaces.filter((v) =>
            dashboardObjectGrant(ctx, "kubernetes_namespace", v.id),
          );
    if (!clusterWide && !namespaces.length)
      fail("KUBERNETES_API_FORBIDDEN", 403);
    return {
      cluster_id: id,
      cluster_name: cluster.name,
      cluster_version: cluster.resourceVersion ?? 1,
      cluster_wide: clusterWide,
      namespaces,
    };
  };
  const visible = (id: string, input: KubernetesInventoryInput) => {
    const access = scope(id),
      data = state(id),
      definition = definitions.find((v) => v.key === input.type);
    if (!definition) fail();
    if (
      !definition.namespaced &&
      definition.key !== "namespace" &&
      !access.cluster_wide
    )
      fail("KUBERNETES_API_FORBIDDEN", 403);
    const allowed = access.cluster_wide ? data.namespaces : access.namespaces;
    const mode =
      input.namespace_mode ??
      (input.namespace_ids !== undefined ? "explicit" : "all_authorized");
    if (mode === "all_authorized" && input.namespace_ids?.length) fail();
    for (const namespace of input.namespace_ids ?? [])
      if (!allowed.some((v) => v.id === namespace))
        fail("KUBERNETES_API_FORBIDDEN", 403);
    const selected =
      mode === "explicit"
        ? allowed.filter((v) => input.namespace_ids?.includes(v.id))
        : allowed;
    return {
      access,
      data,
      rows: data.objects.filter(
        (row) =>
          row.definition.key === input.type &&
          (!input.name || row.name === input.name) &&
          (definition.namespaced
            ? selected.some((v) => v.uid === row.namespace_uid)
            : definition.key === "namespace" && !access.cluster_wide
              ? allowed.some((v) => v.uid === row.uid)
              : true),
      ),
    };
  };
  return {
    kubernetesWorkbench: {
      ...createKubernetesHelmDomains(ctx, state, scope),
      ...createKubernetesMutationDomains(ctx, state, scope),
      ...createKubernetesLogSessionDomains(ctx, state, scope),
      ...createKubernetesStreamDomains(ctx, state, scope),
      async watch(id, token, signal) {
        await ctx.pause();
        if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
        const entry = watches.get(token),
          access = scope(id);
        if (
          !entry ||
          entry.cluster !== id ||
          entry.actor !== ctx.actor().id ||
          entry.version !==
            (ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ??
              1) ||
          entry.scope !== JSON.stringify(access)
        )
          fail("KUBERNETES_API_FORBIDDEN", 403);
        if (entry.expires < Date.now()) fail();
        return {
          events: [],
          watch_token: token,
          scope_signature: entry.signature,
          observed_at: ctx.nowIso(),
          more: false,
        };
      },
      async scope(id) {
        await ctx.pause();
        return scope(id);
      },
      async types(id) {
        await ctx.pause();
        const access = scope(id);
        return {
          complete: true,
          items: definitions.filter(
            (v) => access.cluster_wide || v.namespaced || v.key === "namespace",
          ),
        };
      },
      async list(id, input) {
        await ctx.pause();
        const { access, data, rows } = visible(id, input),
          limit = input.limit ?? 50;
        if (limit < 1 || limit > 200) fail();
        const binding = JSON.stringify({
          id,
          actor: ctx.actor().id,
          version:
            ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1,
          scope: access,
          input: { ...input, cursor: undefined },
        });
        let offset = 0;
        if (input.cursor) {
          try {
            const parsed = JSON.parse(atob(input.cursor)) as {
              binding: string;
              offset: number;
            };
            if (
              parsed.binding !== binding ||
              !Number.isInteger(parsed.offset) ||
              parsed.offset < 0
            )
              fail();
            offset = parsed.offset;
          } catch {
            fail();
          }
        }
        const next =
          offset + limit < rows.length
            ? btoa(JSON.stringify({ binding, offset: offset + limit }))
            : undefined;
        const hash = await crypto.subtle.digest(
          "SHA-256",
          new TextEncoder().encode(binding),
        );
        const signature = [...new Uint8Array(hash)]
          .map((v) => v.toString(16).padStart(2, "0"))
          .join("");
        for (const [token, value] of watches)
          if (value.expires < Date.now()) watches.delete(token);
        if (watches.size >= 128) watches.delete(watches.keys().next().value!);
        const watchToken = crypto.randomUUID();
        watches.set(watchToken, {
          cluster: id,
          actor: ctx.actor().id,
          version:
            ctx.db.dataAuthorizationVersions?.[`user:${ctx.actor().id}`] ?? 1,
          scope: JSON.stringify(access),
          signature,
          expires: Date.now() + 15 * 60000,
        });
        return {
          watch_token: watchToken,
          items: structuredClone(rows.slice(offset, offset + limit)),
          next_cursor: next,
          complete: !next,
          observed_at: ctx.nowIso(),
          scope_signature: signature,
          namespace_references: Object.fromEntries(
            data.namespaces
              .filter(
                (v) =>
                  access.cluster_wide ||
                  access.namespaces.some((n) => n.id === v.id),
              )
              .map((v) => [v.uid, v.id]),
          ),
        };
      },
      async object(id, input) {
        await ctx.pause();
        const { rows } = visible(id, {
          type: input.type,
          name: input.name,
          namespace_mode: input.namespace_id ? "explicit" : "all_authorized",
          namespace_ids: input.namespace_id ? [input.namespace_id] : undefined,
        });
        const row = rows.find((v) => v.uid === input.uid);
        if (!row) fail("KUBERNETES_OBJECT_IDENTITY_CHANGED", 409);
        if (row.definition.namespaced && !input.namespace_id) fail();
        return structuredClone(row);
      },
    },
  };
}
