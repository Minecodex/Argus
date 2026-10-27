import type { BaseContext, MockContext } from "./context";
import type {
  KubernetesHelmRelease,
  KubernetesHelmPreviewInput,
  KubernetesWorkbenchDomains,
  KubernetesInventoryScope,
} from "../kubernetes-workbench";
import type { PendingActionPublic, ConfirmActionResult } from "../types";
import type { MockKubernetesInventory } from "./kubernetes-workbench";
import { resolvePermissions } from "./permissions";
import { dashboardObjectGrant } from "./dashboard-state";
import { ApiError } from "../transport/errors";

function fail(status = 400): never {
  throw new ApiError(
    {
      code:
        status === 403
          ? "KUBERNETES_HELM_FORBIDDEN"
          : "KUBERNETES_HELM_CONFLICT",
      message_key: `kubernetesWorkbench.errors.${status === 403 ? "forbidden" : "mutationConflict"}`,
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}
function authorize(
  ctx: BaseContext,
  cluster: string,
  namespace: string,
  write: boolean,
) {
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  for (const permission of [
    "kubernetes.read",
    ...(write
      ? ["kubernetes.helm.manage", "kubernetes.configuration.manage"]
      : []),
  ])
    if (!permissions.has("*") && !permissions.has(permission)) fail(403);
  const data = ctx.db.kubernetesInventory?.[cluster],
    ns = data?.namespaces.find((n) => n.id === namespace);
  if (
    !data ||
    !ns ||
    !ctx.db.clusters.some(
      (c) => c.id === cluster && c.enterpriseId === ctx.enterpriseId(),
    ) ||
    (!dashboardObjectGrant(ctx, "kubernetes_cluster", cluster) &&
      !dashboardObjectGrant(ctx, "kubernetes_namespace", namespace))
  )
    fail(403);
  return { data, ns };
}
export function createKubernetesHelmDomains(
  ctx: MockContext,
  state: (id: string) => MockKubernetesInventory,
  scope: (id: string) => KubernetesInventoryScope,
): Pick<
  KubernetesWorkbenchDomains["kubernetesWorkbench"],
  "helmReleases" | "helmHistory" | "previewHelm"
> {
  const rows = (id: string, namespace: string) => {
    scope(id);
    const { ns } = authorize(ctx, id, namespace, false);
    return (state(id).helm ?? []).filter((r) => r.namespace === ns.name);
  };
  return {
    async helmReleases(id, namespace) {
      await ctx.pause();
      const history = rows(id, namespace),
        latest = new Map<string, KubernetesHelmRelease>();
      for (const r of history)
        if ((latest.get(r.name)?.revision ?? 0) < r.revision)
          latest.set(r.name, r);
      return { items: structuredClone([...latest.values()]) };
    },
    async helmHistory(id, namespace, name) {
      await ctx.pause();
      return {
        items: structuredClone(
          rows(id, namespace)
            .filter((r) => r.name === name)
            .sort((a, b) => b.revision - a.revision),
        ),
      };
    },
    async previewHelm(cluster, input) {
      await ctx.pause();
      scope(cluster);
      const { data, ns } = authorize(ctx, cluster, input.namespace_id, true);
      if (
        !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(input.name) ||
        input.name.length > 53 ||
        !input.reason.trim() ||
        input.reason.length > 500 ||
        input.timeout_seconds < 10 ||
        input.timeout_seconds > 240
      )
        fail();
      const history = (data.helm ?? []).filter(
          (r) => r.name === input.name && r.namespace === ns.name,
        ),
        latest = history.reduce<KubernetesHelmRelease | undefined>(
          (a, b) => (!a || a.revision < b.revision ? b : a),
          undefined,
        );
      if (
        (latest?.revision ?? 0) !== input.expected_revision ||
        (input.operation === "install" &&
          latest &&
          latest.status !== "uninstalled") ||
        (input.operation !== "install" && !latest)
      )
        fail(409);
      if (
        input.operation === "rollback" &&
        !history.some(
          (r) =>
            r.revision === input.rollback_revision &&
            r.revision < input.expected_revision,
        )
      )
        fail(409);
      const needsChart = ["install", "upgrade"].includes(input.operation);
      if (needsChart) {
        if (!input.chart_archive === !input.source) fail();
        if (input.chart_archive && input.chart_archive.length > 524288) fail();
      } else if (
        input.chart_archive ||
        input.values ||
        input.source ||
        input.source_credentials
      )
        fail();
      if (input.source) {
        const s = input.source;
        if (
          !["repository", "oci"].includes(s.kind) ||
          !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(
            s.version,
          ) ||
          (s.digest && !/^sha256:[a-f0-9]{64}$/.test(s.digest)) ||
          Object.keys(s).some(
            (k) =>
              ![
                "kind",
                "url",
                "chart",
                "version",
                "digest",
                "plain_http",
                "auth_url",
              ].includes(k),
          )
        )
          fail();
        let u: URL;
        try {
          u = new URL(s.url);
        } catch {
          fail();
        }
        if (
          u!.username ||
          u!.password ||
          u!.search ||
          u!.hash ||
          (s.kind === "oci"
            ? u!.protocol !== "oci:"
            : u!.protocol !== "https:" &&
              !(s.plain_http && u!.protocol === "http:")) ||
          (s.kind === "repository" && !s.chart?.trim())
        )
          fail();
        const c = input.source_credentials;
        if (
          c &&
          (!!c.username !== !!c.password ||
            (c.token && (c.username || c.password)))
        )
          fail();
      } else if (input.source_credentials) fail();
      if (new TextEncoder().encode(input.values ?? "").length > 96 * 1024)
        fail();
      // Mock mode simulates metadata only; protected bytes never enter localStorage.
      const {
        chart_archive: _chart,
        values: _values,
        source_credentials: _credentials,
        ...safe
      } = input;
      void _chart;
      void _values;
      void _credentials;
      const action = ctx.createPendingAction({
        tool: `kubernetes.helm.${input.operation}`,
        title: `Helm ${input.operation} ${input.name}`,
        risk: input.operation === "uninstall" ? "dangerous" : "write",
        input_data: {
          ...safe,
          cluster_id: cluster,
          namespace_uid: ns.uid,
          operation_id: crypto.randomUUID(),
        },
      });
      action.preview = {
        ...(input.source
          ? {
              source: {
                kind: input.source.kind,
                origin: input.source.url,
                transport:
                  input.source.kind === "oci"
                    ? input.source.plain_http
                      ? "http"
                      : "https"
                    : new URL(input.source.url).protocol.replace(":", ""),
                chart: input.source.chart ?? input.source.url.split("/").at(-1),
                version: input.source.version,
                digest: input.source.digest ?? "not-verified-in-mock",
                archive_digest: "not-verified-in-mock",
                ...(input.source.auth_url
                  ? { auth_url: input.source.auth_url }
                  : {}),
              },
            }
          : {}),
        operation: input.operation,
        name: input.name,
        namespace: ns.name,
        expected_revision: input.expected_revision,
        keep_history: true,
        objects: [
          {
            definition: { kind: "ConfigMap" },
            namespace: ns.name,
            name: `${input.name}-settings`,
            action: input.operation === "uninstall" ? "delete" : "apply",
            keep: false,
            validation: "server",
            changes: ["/data/*"],
          },
        ],
      };
      action.diff = [
        {
          kind: "note",
          text: `ConfigMap ${ns.name}/${input.name}-settings /data/* = [protected]`,
        },
      ];
      ctx.save();
      return structuredClone(action);
    },
  };
}
export function commitKubernetesHelm(
  ctx: BaseContext,
  action: PendingActionPublic,
): ConfirmActionResult | undefined {
  const plan = ctx.db.actionPlans[action.action_ref];
  if (!plan?.tool.startsWith("kubernetes.helm.")) return;
  const actor = ctx.db.users.find(
    (u) => u.id === plan.created_by && u.status === "active",
  );
  if (!actor || plan.enterprise_id !== ctx.enterpriseId()) fail(403);
  const input = plan.input_data as unknown as KubernetesHelmPreviewInput & {
    cluster_id: string;
    namespace_uid: string;
    operation_id: string;
  };
  const { data, ns } = authorize(
    { ...ctx, actor: () => actor },
    input.cluster_id,
    input.namespace_id,
    true,
  );
  if (ns.uid !== input.namespace_uid) fail(409);
  data.helm ??= [];
  const history = data.helm.filter(
      (r) => r.name === input.name && r.namespace === ns.name,
    ),
    latest = history.reduce<KubernetesHelmRelease | undefined>(
      (a, b) => (!a || a.revision < b.revision ? b : a),
      undefined,
    );
  if ((latest?.revision ?? 0) !== input.expected_revision) fail(409);
  const target =
    input.operation === "rollback"
      ? history.find((r) => r.revision === input.rollback_revision)
      : latest;
  if (latest)
    latest.status =
      input.operation === "uninstall" ? "uninstalled" : "superseded";
  const next: KubernetesHelmRelease = {
    name: input.name,
    namespace: ns.name,
    revision:
      input.operation === "uninstall"
        ? input.expected_revision
        : input.expected_revision + 1,
    status: input.operation === "uninstall" ? "uninstalled" : "deployed",
    chart:
      input.source?.chart ??
      input.source?.url.split("/").at(-1) ??
      target?.chart ??
      "uploaded-chart",
    chart_version: input.source?.version ?? target?.chart_version ?? "1.0.0",
    updated_at: ctx.nowIso(),
  };
  if (input.operation !== "uninstall") data.helm.push(next);
  data.operations ??= [];
  data.operations.push({
    id: input.operation_id,
    cluster_id: input.cluster_id,
    namespace_id: ns.id,
    namespace_uid: ns.uid,
    name: input.name,
    type: "helm_release",
    operation: `helm_${input.operation}`,
    uid: "",
    version: 1,
    status: "succeeded",
    error_code: "",
    result: { accepted: true, applied: true, ready: true, release: next },
    created_at: ctx.nowIso(),
    updated_at: ctx.nowIso(),
    finished_at: ctx.nowIso(),
  });
  action.status = "succeeded";
  action.available_actions = [];
  action.updated_at = ctx.nowIso();
  ctx.audit(plan.tool, {
    summary: action.title,
    resourceType: "kubernetes_operation",
    resourceId: input.operation_id,
  });
  ctx.save();
  return { pending_action: structuredClone(action) };
}
