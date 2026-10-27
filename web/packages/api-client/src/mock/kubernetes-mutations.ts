import type { BaseContext, MockContext } from "./context";
import type { MockKubernetesInventory } from "./kubernetes-workbench";
import type {
  KubernetesMutationInput,
  KubernetesWorkbenchDomains,
  KubernetesInventoryScope,
  KubernetesInventoryObject,
} from "../kubernetes-workbench";
import type { PendingActionPublic, ConfirmActionResult } from "../types";
import { ApiError } from "../transport/errors";
import { resolvePermissions } from "./permissions";
import { dashboardObjectGrant } from "./dashboard-state";
import { kubernetesInventoryTypes } from "../generated/kubernetes-inventory-registry";
import { kubernetesRollbackReference } from "../kubernetes-workbench";
import { mockDrainProgress, mockNodeMaintenanceData } from "./kubernetes-drain";

function rollbackTemplate(
  input: KubernetesMutationInput,
  existing: KubernetesInventoryObject | undefined,
  state: MockKubernetesInventory,
) {
  const history = state.objects.find((row) => row.uid === input.rollback?.uid);
  const reference =
    existing && history
      ? kubernetesRollbackReference(existing, history)
      : undefined;
  if (
    !reference ||
    reference.name !== input.rollback?.name ||
    reference.resource_version !== input.rollback.resource_version ||
    reference.revision !== input.rollback.revision
  )
    fail("KUBERNETES_MUTATION_CONFLICT", 409);
  if (input.object || input.replicas !== undefined) fail();
  const spec = (
    history!.definition.key === "replicaset"
      ? history!.data.spec
      : (history!.data.data as Record<string, unknown>)?.spec
  ) as Record<string, unknown> | undefined;
  const template = structuredClone(spec?.template) as
    Record<string, unknown> | undefined;
  if (!template) fail();
  delete template.$patch;
  const metadata = template.metadata as Record<string, unknown> | undefined;
  if (history!.definition.key === "replicaset" && metadata?.labels)
    delete (metadata.labels as Record<string, unknown>)["pod-template-hash"];
  return template;
}

function fail(code = "KUBERNETES_MUTATION_INVALID", status = 400): never {
  throw new ApiError(
    {
      code,
      message_key:
        "kubernetesWorkbench.errors." +
        (status === 403
          ? "forbidden"
          : status === 409
            ? "mutationConflict"
            : "mutationInvalid"),
      request_id: crypto.randomUUID(),
      retryable: false,
    },
    status,
  );
}

function cronJobManifest(
  input: KubernetesMutationInput,
  namespace: KubernetesInventoryScope["namespaces"][number] | undefined,
  state: MockKubernetesInventory,
) {
  if (!input.cronjob_source) return undefined;
  if (
    input.operation !== "create" ||
    input.type !== "job" ||
    input.object ||
    input.rollback ||
    input.replicas !== undefined ||
    !namespace
  )
    fail();
  const ref = input.cronjob_source;
  const source = state.objects.find(
    (row) =>
      row.definition.key === "cronjob" &&
      row.namespace_uid === namespace.uid &&
      row.name === ref.name,
  );
  if (
    !source ||
    source.uid !== ref.uid ||
    source.resource_version !== ref.resource_version
  )
    fail("KUBERNETES_MUTATION_CONFLICT", 409);
  const jobTemplate = (source.data.spec as Record<string, unknown>)
    ?.jobTemplate as Record<string, unknown> | undefined;
  if (!jobTemplate?.spec) fail();
  const metadata = jobTemplate.metadata as Record<string, unknown> | undefined;
  return {
    apiVersion: "batch/v1",
    kind: "Job",
    metadata: {
      name: input.name,
      namespace: namespace.name,
      labels: metadata?.labels ?? {},
      annotations: {
        ...(metadata?.annotations as object),
        "cronjob.kubernetes.io/instantiate": "manual",
      },
      ownerReferences: [
        {
          apiVersion: "batch/v1",
          kind: "CronJob",
          name: source.name,
          uid: source.uid,
          controller: true,
        },
      ],
    },
    spec: structuredClone(jobTemplate.spec),
  };
}
function validate(
  ctx: BaseContext,
  id: string,
  input: KubernetesMutationInput,
  state: MockKubernetesInventory,
) {
  const definition = kubernetesInventoryTypes.find((d) => d.key === input.type),
    permissions = resolvePermissions(
      ctx.db,
      ctx.enterpriseId(),
      ctx.actor().id,
      ctx.nowIso(),
    );
  if (
    !definition?.manage_permission ||
    (!permissions.has("*") && !permissions.has(definition.manage_permission))
  )
    fail("KUBERNETES_API_FORBIDDEN", 403);
  const cluster = ctx.db.clusters.find(
    (v) => v.id === id && v.enterpriseId === ctx.enterpriseId(),
  );
  if (!cluster) fail("KUBERNETES_API_FORBIDDEN", 403);
  if (
    ["drain", "cordon", "uncordon"].includes(input.operation) &&
    input.type !== "node"
  )
    fail();
  if (
    input.operation === "drain" &&
    (input.type !== "node" ||
      (!permissions.has("*") && !permissions.has("kubernetes.node.drain")))
  )
    fail("KUBERNETES_API_FORBIDDEN", 403);
  const clusterWide = dashboardObjectGrant(ctx, "kubernetes_cluster", id),
    namespace = state.namespaces.find((n) => n.id === input.namespace_id);
  if (
    definition.namespaced
      ? !namespace ||
        (!clusterWide &&
          !dashboardObjectGrant(ctx, "kubernetes_namespace", namespace.id))
      : !clusterWide || !!input.namespace_id
  )
    fail("KUBERNETES_API_FORBIDDEN", 403);
  const existing = state.objects.find(
    (o) =>
      o.definition.key === input.type &&
      o.name === input.name &&
      o.namespace === (namespace?.name ?? ""),
  );
  if (
    input.operation === "create"
      ? !!existing
      : !existing ||
        existing.uid !== input.uid ||
        existing.resource_version !== input.expected_resource_version
  )
    fail("KUBERNETES_MUTATION_CONFLICT", 409);
  if (
    input.type === "secret" &&
    input.object &&
    (input.object.stringData ||
      (input.object.data &&
        (typeof input.object.data !== "object" ||
          Array.isArray(input.object.data) ||
          Object.values(input.object.data).some(
            (v) => v !== null && v !== "[REDACTED]",
          ))))
  )
    fail();
  if (
    input.operation === "scale" &&
    (!Number.isInteger(input.replicas) ||
      input.replicas! < 0 ||
      input.replicas! > 10000)
  )
    fail();
  const template =
    input.operation === "rollback"
      ? rollbackTemplate(input, existing, state)
      : undefined;
  if (input.operation !== "rollback" && input.rollback) fail();
  return {
    definition,
    namespace,
    existing,
    template,
    derived: cronJobManifest(input, namespace, state),
  };
}
function merged(
  before: Record<string, unknown>,
  patch: Record<string, unknown>,
): Record<string, unknown> {
  const result = structuredClone(before);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete result[key];
    else
      result[key] =
        value && typeof value === "object" && !Array.isArray(value)
          ? merged(
              (result[key] ?? {}) as Record<string, unknown>,
              value as Record<string, unknown>,
            )
          : value;
  }
  return result;
}
export function createKubernetesMutationDomains(
  ctx: MockContext,
  state: (id: string) => MockKubernetesInventory,
  scope: (id: string) => KubernetesInventoryScope,
): Pick<
  KubernetesWorkbenchDomains["kubernetesWorkbench"],
  "preview" | "operations" | "operation" | "cancelDrain"
> {
  const visible = (id: string) => {
    const access = scope(id);
    return (state(id).operations ?? []).filter(
      (o) =>
        access.cluster_wide ||
        access.namespaces.some(
          (n) => n.id === o.namespace_id && n.uid === o.namespace_uid,
        ),
    );
  };
  return {
    async cancelDrain(cluster, id, expectedVersion) {
      await ctx.pause();
      const permissions = resolvePermissions(
        ctx.db,
        ctx.enterpriseId(),
        ctx.actor().id,
        ctx.nowIso(),
      );
      if (
        !scope(cluster).cluster_wide ||
        (!permissions.has("*") &&
          (!permissions.has("kubernetes.node.manage") ||
            !permissions.has("kubernetes.node.drain")))
      )
        fail("KUBERNETES_API_FORBIDDEN", 403);
      const row = visible(cluster).find((o) => o.id === id);
      if (!row || row.operation !== "drain")
        fail("KUBERNETES_API_FORBIDDEN", 403);
      if (row.status === "cancelled") return structuredClone(row);
      if (
        row.version !== expectedVersion ||
        !["queued", "running", "observing", "result_unknown"].includes(
          row.status,
        )
      )
        fail("KUBERNETES_MUTATION_CONFLICT", 409);
      row.status = "cancelled";
      row.version++;
      row.updated_at = ctx.nowIso();
      row.finished_at = ctx.nowIso();
      ctx.audit("kubernetes.drain.cancel", {
        summary: row.name,
        resourceType: "kubernetes_operation",
        resourceId: row.id,
      });
      ctx.save();
      return structuredClone(row);
    },
    async preview(id, input) {
      await ctx.pause();
      const publicInput = structuredClone(input),
        privateValues = publicInput.secret_values;
      delete publicInput.secret_values;
      if (privateValues) {
        if (
          input.type !== "secret" ||
          !["create", "patch"].includes(input.operation) ||
          Object.keys(privateValues).length > 128 ||
          JSON.stringify(privateValues).length > 65536
        )
          fail();
        const keys = Object.keys(privateValues);
        if (
          keys.some(
            (key) => !key || key.length > 253 || !/^[a-zA-Z0-9._-]+$/.test(key),
          )
        )
          fail();
        validate(ctx, id, publicInput, state(id));
        publicInput.object ??= {};
        const data = {
          ...((publicInput.object.data as Record<string, unknown>) ?? {}),
        };
        for (const key of keys) data[key] = "[REDACTED]";
        publicInput.object.data = data;
      }
      input = publicInput;
      const checked = validate(ctx, id, input, state(id));
      const operationId = crypto.randomUUID();
      const action = ctx.createPendingAction({
        tool: `kubernetes.object.${input.operation}`,
        title: `${input.operation} ${checked.definition.kind}/${input.name}`,
        risk:
          input.operation === "delete" || input.operation === "drain"
            ? "dangerous"
            : "write",
        input_data: {
          ...structuredClone(input),
          cluster_id: id,
          operation_id: operationId,
        },
      });
      action.diff = publicDiff(
        checked.existing?.data,
        input.operation === "delete"
          ? undefined
          : input.operation === "drain" ||
              input.operation === "cordon" ||
              input.operation === "uncordon"
            ? mockNodeMaintenanceData(checked.existing!.data, input.operation)
            : checked.derived
              ? checked.derived
              : checked.template
                ? {
                    ...checked.existing!.data,
                    spec: {
                      ...(checked.existing!.data.spec as object),
                      template: checked.template,
                    },
                  }
                : input.object
                  ? merged(checked.existing?.data ?? {}, input.object)
                  : { operation: input.operation, replicas: input.replicas },
      );
      if (input.operation === "drain" && checked.existing) {
        const progress = mockDrainProgress(state(id), checked.existing);
        action.preview = {
          ...(typeof action.preview === "object" && action.preview !== null
            ? action.preview
            : {}),
          drain: progress,
        };
        action.diff.push(
          ...progress.pods.map((p) => ({
            kind: "note" as const,
            text: JSON.stringify(p),
          })),
        );
      }
      ctx.save();
      return structuredClone(action);
    },
    async operations(id, input) {
      await ctx.pause();
      const rows = visible(id).sort((a, b) => a.id.localeCompare(b.id)),
        start = input?.cursor
          ? rows.findIndex((o) => o.id === input.cursor) + 1
          : 0,
        limit = input?.limit ?? 50;
      if (input?.cursor && start === 0) fail();
      return {
        items: structuredClone(rows.slice(start, start + limit)),
        next_cursor:
          rows.length > start + limit ? rows[start + limit - 1]?.id : undefined,
      };
    },
    async operation(id, operationId) {
      await ctx.pause();
      const row = visible(id).find((o) => o.id === operationId);
      if (!row) fail("KUBERNETES_API_FORBIDDEN", 403);
      return structuredClone(row);
    },
  };
}

function publicDiff(
  before: unknown,
  after: unknown,
): PendingActionPublic["diff"] {
  const lines: PendingActionPublic["diff"] = [];
  const append = (kind: "remove" | "add", path: string, value: unknown) => {
    lines.push({ kind: "note", text: path });
    for (const text of JSON.stringify(value, null, 2).split("\n")) {
      if (text.length > 2048 || lines.length >= 256)
        fail("KUBERNETES_RESULT_BUDGET_EXCEEDED", 413);
      lines.push({ kind, text });
    }
  };
  const walk = (path: string, left: unknown, right: unknown) => {
    if (JSON.stringify(left) === JSON.stringify(right)) return;
    if (
      left &&
      right &&
      typeof left === "object" &&
      typeof right === "object" &&
      !Array.isArray(left) &&
      !Array.isArray(right)
    ) {
      const a = left as Record<string, unknown>,
        b = right as Record<string, unknown>;
      for (const key of [
        ...new Set([...Object.keys(a), ...Object.keys(b)]),
      ].sort())
        walk(
          `${path}/${key.replaceAll("~", "~0").replaceAll("/", "~1")}`,
          a[key],
          b[key],
        );
      return;
    }
    if (left !== undefined) append("remove", path, left);
    if (right !== undefined) append("add", path, right);
  };
  walk("", before, after);
  return lines;
}
export function commitKubernetesMutation(
  ctx: BaseContext,
  action: PendingActionPublic,
): ConfirmActionResult | undefined {
  const plan = ctx.db.actionPlans[action.action_ref];
  if (!plan?.tool.startsWith("kubernetes.object.")) return undefined;
  const input = plan.input_data as unknown as KubernetesMutationInput & {
    cluster_id: string;
    operation_id: string;
  };
  const state = ctx.db.kubernetesInventory?.[input.cluster_id];
  if (!state) fail();
  const originalActor = ctx.db.users.find(
    (user) => user.id === plan.created_by && user.status === "active",
  );
  if (!originalActor || plan.enterprise_id !== ctx.enterpriseId())
    fail("KUBERNETES_API_FORBIDDEN", 403);
  const executionContext = { ...ctx, actor: () => originalActor };
  const { existing, definition, namespace, template, derived } = validate(
    executionContext,
    input.cluster_id,
    input,
    state,
  );
  let after: KubernetesInventoryObject | undefined;
  if (input.operation === "delete") {
    state.objects = state.objects.filter((o) => o.uid !== existing!.uid);
  } else {
    const now = ctx.nowIso();
    after = existing ?? {
      definition: structuredClone(definition),
      uid: crypto.randomUUID(),
      name: input.name,
      namespace: namespace?.name ?? "",
      namespace_uid: namespace?.uid,
      resource_version: "0",
      generation: 0,
      labels: {},
      owners: [],
      created_at: now,
      observed_at: now,
      redacted: false,
      data: {
        apiVersion: definition.group
          ? `${definition.group}/${definition.version}`
          : definition.version,
        kind: definition.kind,
        metadata: { name: input.name, namespace: namespace?.name },
      },
    };
    after.data = merged(
      after.data as Record<string, unknown>,
      input.object ?? derived ?? {},
    );
    if (derived) {
      after.owners = derived.metadata.ownerReferences;
      after.labels = derived.metadata.labels as Record<string, string>;
    }
    if (template)
      after.data.spec = { ...(after.data.spec as object), template };
    if (input.operation === "scale")
      after.data.spec = {
        ...((after.data.spec as object) ?? {}),
        replicas: input.replicas,
      };
    if (
      input.operation === "cordon" ||
      input.operation === "uncordon" ||
      input.operation === "drain"
    )
      after.data = mockNodeMaintenanceData(after.data, input.operation);
    after.resource_version = String(Number(after.resource_version) + 1);
    after.generation++;
    after.observed_at = now;
    after.data.metadata = {
      ...(after.data.metadata as object),
      uid: after.uid,
      resourceVersion: after.resource_version,
    };
    if (!existing) state.objects.push(after);
  }
  const drain =
    input.operation === "drain" && after
      ? mockDrainProgress(state, after)
      : undefined;
  const complete = !drain || drain.phase === "complete";
  (state.operations ??= []).push({
    id: input.operation_id,
    cluster_id: input.cluster_id,
    namespace_id: namespace?.id,
    namespace_uid: namespace?.uid ?? "",
    status: complete ? "succeeded" : "observing",
    operation: input.operation,
    type: input.type,
    name: input.name,
    uid: after?.uid ?? input.uid ?? "",
    error_code: "",
    version: 1,
    result: {
      accepted: true,
      deleted: input.operation === "delete",
      after: after?.data,
      ...(drain ? { drain } : {}),
    },
    created_at: ctx.nowIso(),
    updated_at: ctx.nowIso(),
    finished_at: complete ? ctx.nowIso() : undefined,
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
