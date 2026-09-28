import type { DashboardDomains, DashboardSchemas } from "../dashboard";
import type { BaseContext } from "./context";
import {
  copyDashboard as copy,
  dashboardState,
  dashboardPermission,
  dashboardObjectGrant,
  dashboardFailure as fail,
  mockDashboard,
} from "./dashboard-state";
import { mockDashboardExecution } from "./dashboard-runtime";
import { resolvePermissions } from "./permissions";
import { mockFileDrilldown } from "./dashboard-query-drilldown";

export function assertDashboardQuerySources(
  ctx: BaseContext,
  conversation: string,
) {
  const workspace = ctx.db.planv5.workspaces[conversation];
  if (!workspace || workspace.status === "deleted") return;
  const jobs = dashboardState(ctx).queryJobs ?? [];
  for (const job of jobs)
    if (
      job.conversation === conversation &&
      job.value.files.some((f) => f.workspace_id === workspace.id)
    )
      check(ctx, conversation, job.value.dashboard_id, job.execution.resources);
}
function check(
  ctx: BaseContext,
  conversation: string,
  dashboard: string,
  resources: Array<{ id: string; type: string }> = [],
) {
  dashboardPermission(ctx);
  const permissions = resolvePermissions(
    ctx.db,
    ctx.enterpriseId(),
    ctx.actor().id,
    ctx.nowIso(),
  );
  if (!permissions.has("*") && !permissions.has("workspace.use"))
    fail("DASHBOARD_DENIED", 403);
  if (
    !ctx.db.conversations.some(
      (c) =>
        c.id === conversation &&
        c.enterpriseId === ctx.enterpriseId() &&
        c.createdBy === ctx.actor().id &&
        c.status === "active",
    )
  )
    fail("DASHBOARD_DENIED", 403);
  if (mockDashboard(ctx, dashboard).lifecycle !== "active")
    fail("DASHBOARD_ARCHIVED", 409);
  if (resources.some((r) => !dashboardObjectGrant(ctx, r.type, r.id)))
    fail("DASHBOARD_DENIED", 403);
}
export function createMockDashboardQueries(
  ctx: BaseContext,
): DashboardDomains["dashboardQueries"] {
  const jobs = () => (dashboardState(ctx).queryJobs ??= []);
  const get = (conversation: string, id: string) => {
    const job =
      jobs().find(
        (j) =>
          j.value.id === id &&
          j.enterprise === ctx.enterpriseId() &&
          j.owner === ctx.actor().id &&
          j.conversation === conversation,
      ) ?? fail("DASHBOARD_NOT_FOUND", 404);
    check(ctx, conversation, job.value.dashboard_id, job.execution.resources);
    return job;
  };
  return {
    async start(conversation, input, key) {
      check(ctx, conversation, input.dashboard_id);
      if (!key || input.run_id || input.parameters.candidates_only) fail(); // Mock has no server Run identities.
      const encoded = JSON.stringify(input),
        prior = jobs().find(
          (j) =>
            j.key === key &&
            j.conversation === conversation &&
            j.owner === ctx.actor().id &&
            j.enterprise === ctx.enterpriseId(),
        );
      if (prior) {
        if (prior.input !== encoded) fail("DASHBOARD_VERSION_CONFLICT", 409);
        return copy(get(conversation, prior.value.id).value);
      }
      const item = mockDashboard(ctx, input.dashboard_id);
      const parent = input.drilldown
        ? get(conversation, input.drilldown.parent_job_id)
        : undefined;
      const revision =
        dashboardState(ctx).revisions.find(
          (r) =>
            r.id === (parent?.value.revision_id ?? item.active_revision_id),
        ) ?? fail();
      const detail = parent ? mockFileDrilldown(parent, input) : undefined;
      const execution =
        detail?.execution ??
        mockDashboardExecution(
          ctx,
          revision.spec,
          input.parameters,
          item.id,
          revision.id,
        );
      const value: DashboardSchemas["DashboardQueryJobView"] = {
        id: crypto.randomUUID(),
        dashboard_id: item.id,
        revision_id: revision.id,
        status: "queued",
        version: 1,
        files: [],
      };
      jobs().push({
        enterprise: ctx.enterpriseId(),
        owner: ctx.actor().id,
        conversation,
        key,
        input: encoded,
        execution,
        spec: copy(revision.spec),
        drilldown: detail?.context,
        value,
      });
      ctx.save();
      return copy(value);
    },
    async get(conversation, id) {
      const job = get(conversation, id);
      if (job.value.status !== "queued") return copy(job.value);
      job.value.status = "fetching";
      job.value.version++;
      const attempt = crypto.randomUUID(),
        source = `dashboard-query:${id}/${attempt}`,
        execution = copy(job.execution);
      const pending: Array<{
        ref: DashboardSchemas["DashboardQueryJobView"]["files"][number];
        data: string;
      }> = [];
      async function file(
        panel: string,
        target: string,
        kind: "data" | "manifest",
        value: unknown,
      ) {
        const data = JSON.stringify(value),
          bytes = new TextEncoder().encode(data),
          hash = await crypto.subtle.digest("SHA-256", bytes);
        const fileId = crypto.randomUUID(),
          name = kind === "manifest" ? "manifest.json" : `${fileId}.json`;
        pending.push({
          ref: {
            id: fileId,
            panel_id: panel,
            target_id: target,
            kind,
            bytes: bytes.length,
            sha256: Array.from(new Uint8Array(hash), (b) =>
              b.toString(16).padStart(2, "0"),
            ).join(""),
            source_ref: source,
            path: `/workspace/queries/${id}/${attempt}/${name}`,
          },
          data: `data:application/json;base64,${btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join(""))}`,
        });
      }
      for (const panel of execution.panels)
        for (const target of panel.targets) {
          if (target.data != null)
            await file(panel.id, target.id, "data", target.data);
          target.data = null;
        }
      const manifest = {
        schema_version: "argus.dashboard_query_manifest/v1",
        job_id: id,
        attempt_id: attempt,
        compiler_version: "mock",
        execution,
        definition: job.spec,
        files: pending.map((p) => copy(p.ref)),
        complete: !execution.partial,
        analysis_status: "not_analyzed",
        ...(job.drilldown ? { drilldown: job.drilldown } : {}),
        warnings: ["MOCK_DATA"],
      };
      await file("", "", "manifest", manifest);
      // A cancel while hashing prevents every write, as in the real lease path.
      if ((job.value.status as string) !== "fetching") return copy(job.value);
      check(ctx, conversation, job.value.dashboard_id, job.execution.resources);
      const state = ctx.db.planv5;
      if (
        !state.workspaces[conversation] ||
        state.workspaces[conversation].status === "deleted"
      )
        state.workspaces[conversation] = {
          id: crypto.randomUUID(),
          conversation_id: conversation,
          status: "ready",
          capacity_bytes: 2 * 1024 ** 3,
          version: 1,
          created_at: ctx.nowIso(),
        };
      const workspace = state.workspaces[conversation];
      for (const { ref, data } of pending) {
        ref.workspace_id = workspace.id;
        ref.workspace_file_id = crypto.randomUUID();
        (state.files[conversation] ??= []).push({
          id: ref.workspace_file_id,
          name: ref.path.split("/").at(-1)!,
          path: ref.path,
          byte_size: ref.bytes,
          content_hash: ref.sha256,
          media_type: "application/json",
          created_at: ctx.nowIso(),
        });
        state.contents[ref.workspace_file_id] = data;
      }
      job.value.manifest = manifest;
      job.value.files = pending.map((p) => p.ref);
      job.value.status = execution.partial ? "partial" : "complete";
      job.value.version++;
      ctx.save();
      return copy(job.value);
    },
    async cancel(conversation, id) {
      const job = get(conversation, id);
      if (
        !["complete", "partial", "cancelled", "failed"].includes(
          job.value.status,
        )
      ) {
        job.value.status = "cancelled";
        job.value.version++;
        ctx.save();
      }
      return copy(job.value);
    },
    async resume(conversation, id, version) {
      const job = get(conversation, id);
      if (job.value.status !== "failed" || job.value.version !== version)
        fail("DASHBOARD_VERSION_CONFLICT", 409);
      job.value.status = "queued";
      job.value.version++;
      ctx.save();
      return copy(job.value);
    },
  };
}
