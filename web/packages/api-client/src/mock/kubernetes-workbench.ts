import { dashboardState, dashboardAccess } from "./dashboard-state";
import { mockOverview, validateOverviewChoice } from "./kubernetes-overview";
import { createDashboardDomains } from "./dashboard";
import { mockResourceView } from "./kubernetes-resource-view";
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
      async overviewCandidates(id, input) {
        const access = scope(id);
        if (!access.cluster_wide) fail("KUBERNETES_API_FORBIDDEN", 403);
        mockOverview(ctx, access);
        const data = dashboardState(ctx);
        const items = data.items
          .filter(
            (row) =>
              row.enterprise === ctx.enterpriseId() &&
              row.value.lifecycle === "active" &&
              row.value.name
                .toLocaleLowerCase()
                .includes((input.search ?? "").toLocaleLowerCase()),
          )
          .filter((row) => {
            try {
              dashboardAccess(ctx, row.value.id);
              return true;
            } catch {
              return false;
            }
          })
          .filter((row) => {
            const revision = data.revisions.find(
              (item) => item.id === row.value.active_revision_id,
            );
            return (
              revision?.spec.panels.length &&
              revision.spec.panels.every((panel) =>
                panel.applicable_resource_types.some(
                  (kind) =>
                    kind === "kubernetes_cluster" ||
                    kind === "kubernetes_namespace",
                ),
              )
            );
          })
          .map((row) => row.value);
        const start = input.cursor ? Number(input.cursor) : 0,
          limit = input.limit ?? 50;
        return {
          items: items.slice(start, start + limit),
          next_cursor:
            start + limit < items.length ? String(start + limit) : undefined,
        };
      },
      async overview(id) {
        await ctx.pause();
        return mockOverview(ctx, scope(id));
      },
      async previewOverview(id, input) {
        const access = scope(id);
        const { board } = validateOverviewChoice(ctx, access, input);
        const action = ctx.createPendingAction({
          tool:
            "telemetry.dashboard.overview." +
            (input.reset ? "reset" : "select"),
          title: board.name,
          risk: "write",
          input_data: { ...input, cluster_id: id },
        });
        ctx.save();
        return structuredClone(action);
      },
      async executeOverview(id, input) {
        const current = mockOverview(ctx, scope(id));
        if (current.dashboard?.id !== input.expected_dashboard_id)
          fail("DASHBOARD_SELECTION_STALE", 409);
        if (input.kubernetes_scope && input.kubernetes_scope.cluster_id !== id)
          fail("KUBERNETES_API_FORBIDDEN", 403);
        return createDashboardDomains(ctx).dashboards.execute(
          current.dashboard.id,
          {
            ...input,
            resource_mode: "explicit",
            resource_ids: [id],
            resource_types: ["kubernetes_cluster"],
            kubernetes_scope: { ...input.kubernetes_scope, cluster_id: id },
          },
        );
      },
      async related(id, input, signal) {
        const source = await this.object(id, input, signal);
        const snapshot = await this.snapshot(id, { namespace_mode: input.namespace_id ? "explicit" : "all_authorized", namespace_ids: input.namespace_id ? [input.namespace_id] : undefined, types: input.view === "events" ? ["event"] : ["pod", "deployment", "cronjob", "job", "pvc", "pdb"] }, signal);
        const related = snapshot.resources.filter(row => row.uid !== source.uid && (input.view === "events" ? row.summary.fields.involved_uid === source.uid : source.definition.key === "namespace" ? row.namespace_uid === source.uid : source.owners.some(owner => owner.uid === row.uid) || row.summary.fields.owner?.includes(source.definition.kind + "/" + source.name)));
        const offset = Number(input.cursor ?? 0), limit = input.limit ?? 30;
        return { items: related.slice(offset, offset + limit), next_cursor: offset + limit < related.length ? String(offset + limit) : undefined, complete: true, errors: {}, observed_at: ctx.nowIso() };
      },
      async snapshot(id, input, signal) {
        const access = scope(id);
        const resources: import("../kubernetes-workbench").KubernetesResourceSnapshot["resources"] =
          [];
        const counts: Record<string, Record<string, number>> = {};
        for (const type of input.types ?? [
          "pod",
          "deployment",
          "cronjob",
          "pvc",
        ]) {
          const page = await this.list(
            id,
            {
              type,
              namespace_mode: input.namespace_mode,
              namespace_ids: input.namespace_ids,
              limit: 100,
            },
            signal,
          );
          for (const row of page.items) {
            const summary = row.summary ?? {
              state: "unknown" as const,
              fields: {},
            };
            resources.push({
              type,
              name: row.name,
              uid: row.uid,
              namespace: row.namespace,
              namespace_uid: row.namespace_uid,
              namespace_id: row.namespace_uid
                ? page.namespace_references[row.namespace_uid]
                : undefined,
              summary,
              actions: row.actions ?? [],
              created_at: row.created_at,
            });
            counts[type] ??= { total: 0 };
            counts[type]!.total = (counts[type]!.total ?? 0) + 1;
            counts[type]![summary.state] =
              (counts[type]![summary.state] ?? 0) + 1;
          }
        }
        return {
          cluster_id: id,
          cluster_wide: access.cluster_wide,
          resources,
          relations: [],
          counts,
          complete: true,
          errors: {},
          observed_at: ctx.nowIso(),
          scope_signature: JSON.stringify(access),
        };
      },
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
        const { access, data, rows: unfiltered } = visible(id, input),
          rows = unfiltered.filter((row) => {
            const state = mockResourceView(row).summary?.state;
            return (
              !input.state ||
              (input.state === "attention"
                ? ["failed", "warning", "pending", "unknown"].includes(
                    state ?? "unknown",
                  )
                : input.state === state)
            );
          }),
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
          items: rows.slice(offset, offset + limit).map(mockResourceView),
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
        return mockResourceView(row);
      },
    },
  };
}
