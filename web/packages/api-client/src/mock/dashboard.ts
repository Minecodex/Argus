import type {
  DashboardDomains,
  DashboardDraft,
  DashboardRevision,
} from "../dashboard";
import type { BaseContext, MockContext } from "./context";
import type { ConfirmActionResult, PendingActionPublic } from "../types";
import {
  copyDashboard as copy,
  dashboardAccess,
  dashboardFailure as fail,
  dashboardPermission,
  dashboardState,
  draftBaseline,
  mockDashboard,
  mockDraft,
  validateMockDashboard,
} from "./dashboard-state";
import {
  mockDashboardExecution,
  mockDashboardCatalog,
  mockDashboardDrilldown,
  mockGenerateDrilldowns,
} from "./dashboard-runtime";
import {
  createDashboardBindingMethods,
  commitDashboardBinding,
} from "./dashboard-bindings";

import { mockConvertPanel } from "./dashboard-conversion";
import { createMockDashboardQueries } from "./dashboard-queries";

export function createDashboardDomains(ctx: MockContext): DashboardDomains {
  const revision = (id: string) =>
    dashboardState(ctx).revisions.find((r) => r.id === id) ??
    fail("DASHBOARD_NOT_FOUND", 404);
  const preview = (
    tool: string,
    data: Record<string, unknown>,
    title: string,
  ) =>
    ctx.createPendingAction({ tool, title, risk: "write", input_data: data });
  return {
    dashboardQueries: createMockDashboardQueries(ctx),
    dashboards: {
      async convertPanel(input) {
        dashboardPermission(ctx, true);
        return mockConvertPanel(input);
      },
      ...createDashboardBindingMethods(ctx),
      async list() {
        await ctx.pause();
        dashboardPermission(ctx);
        return copy(
          dashboardState(ctx)
            .items.filter(
              (r) =>
                r.enterprise === ctx.enterpriseId() &&
                dashboardAccess(ctx, r.value.id),
            )
            .map((r) => r.value),
        );
      },
      async get(id) {
        const item = mockDashboard(ctx, id);
        return copy({
          dashboard: item,
          revision: revision(item.active_revision_id),
        });
      },
      async revisions(id) {
        mockDashboard(ctx, id);
        return copy(
          dashboardState(ctx)
            .revisions.filter((r) => r.dashboard_id === id)
            .reverse(),
        );
      },
      async drafts() {
        dashboardPermission(ctx, true);
        return copy(
          dashboardState(ctx)
            .drafts.filter(
              (r) =>
                r.enterprise === ctx.enterpriseId() &&
                r.editor === ctx.actor().id &&
                r.value.status === "editing" &&
                (!r.value.dashboard_id ||
                  dashboardAccess(ctx, r.value.dashboard_id)),
            )
            .map((r) => r.value),
        );
      },
      async draft(id) {
        return copy(mockDraft(ctx, id));
      },
      async createDraft(input) {
        dashboardPermission(ctx, true);
        const item = input.dashboard_id
          ? mockDashboard(ctx, input.dashboard_id)
          : undefined;
        if (item?.lifecycle === "archived") fail("DASHBOARD_ARCHIVED", 409);
        const existing =
          item &&
          dashboardState(ctx).drafts.find(
            (r) =>
              r.editor === ctx.actor().id &&
              r.enterprise === ctx.enterpriseId() &&
              r.value.dashboard_id === item.id &&
              r.value.status === "editing",
          );
        if (existing) return copy(existing.value);
        const published = item ? revision(item.active_revision_id) : undefined;
        const value: DashboardDraft = {
          id: crypto.randomUUID(),
          dashboard_id: item?.id,
          base_revision_id: item?.active_revision_id,
          base_object_version: item?.version ?? 0,
          draft_version: 1,
          status: "editing",
          name: item?.name ?? input.name,
          description: item?.description ?? input.description,
          folder_id: item?.folder_id ?? input.folder_id,
          spec: copy(published?.spec ?? input.spec),
          proposed_bindings: copy(input.proposed_bindings),
          updated_at: ctx.nowIso(),
        };
        dashboardState(ctx).drafts.push({
          enterprise: ctx.enterpriseId(),
          editor: ctx.actor().id,
          value,
        });
        ctx.save();
        return copy(value);
      },
      async saveDraft(id, input) {
        const d = mockDraft(ctx, id);
        if (
          d.status !== "editing" ||
          input.expected_version !== d.draft_version
        )
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        Object.assign(
          d,
          copy({
            name: input.name,
            description: input.description,
            folder_id: input.folder_id,
            spec: input.spec,
            proposed_bindings: input.proposed_bindings,
          }),
          { draft_version: d.draft_version + 1, updated_at: ctx.nowIso() },
        );
        ctx.save();
        return copy(d);
      },
      async discardDraft(id, version) {
        const d = mockDraft(ctx, id);
        if (d.draft_version !== version || d.status !== "editing")
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        d.status = "discarded";
        ctx.save();
      },
      async rebase(id, input) {
        const d = mockDraft(ctx, id);
        if (!d.dashboard_id || d.draft_version !== input.expected_version)
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        const item = mockDashboard(ctx, d.dashboard_id);
        if (item.lifecycle !== "active") fail("DASHBOARD_ARCHIVED", 409);
        if (
          input.revision_id !== item.active_revision_id ||
          input.object_version !== item.version
        )
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        d.base_revision_id = item.active_revision_id;
        d.base_object_version = item.version;
        d.draft_version++;
        ctx.save();
        return copy(d);
      },
      async validate(spec) {
        dashboardPermission(ctx, true);
        return validateMockDashboard(spec);
      },
      async sample(id, version) {
        const d = mockDraft(ctx, id);
        if (d.draft_version !== version)
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        const validation = validateMockDashboard(d.spec);
        if (!validation.valid)
          return {
            draft_id: id,
            draft_version: version,
            validation,
            sample: { status: "not_executed", reason: "configuration_invalid" },
          };
        return {
          draft_id: id,
          draft_version: version,
          validation,
          sample: {
            status: "mock",
            execution: mockDashboardExecution(
              ctx,
              d.spec,
              {},
              d.dashboard_id,
              d.base_revision_id,
            ),
          },
        };
      },
      async preview(id, version) {
        const d = mockDraft(ctx, id);
        if (d.draft_version !== version)
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        draftBaseline(ctx, d);
        if (!validateMockDashboard(d.spec).valid) fail();
        const folderVersion =
          dashboardState(ctx).folders.find((f) => f.value.id === d.folder_id)
            ?.value.version ?? 0;
        const action = preview(
          "telemetry.dashboard.publish",
          {
            draft_id: id,
            draft_version: version,
            base_revision_id: d.base_revision_id ?? "",
            base_object_version: d.base_object_version,
            folder_version: folderVersion,
          },
          d.name,
        );
        const before = d.base_revision_id
          ? revision(d.base_revision_id).spec
          : null;
        action.preview = {
          before_json: JSON.stringify(before, null, 2),
          after_json: JSON.stringify(
            {
              name: d.name,
              description: d.description,
              folder_id: d.folder_id,
              spec: d.spec,
              bindings: d.proposed_bindings,
            },
            null,
            2,
          ),
          validation: "mock_structure_only",
        };
        action.diff = [
          {
            kind: before ? "change" : "add",
            text: `${d.name} · ${d.spec.panels.length} panels · ${d.spec.variables.length} variables`,
          },
        ];
        ctx.save();
        return copy(action);
      },
      async execute(id, input) {
        const item = mockDashboard(ctx, id);
        if (item.lifecycle !== "active") fail("DASHBOARD_ARCHIVED", 409);
        return mockDashboardExecution(
          ctx,
          revision(item.active_revision_id).spec,
          input,
          id,
          item.active_revision_id,
        );
      },
      async catalog(input) {
        dashboardPermission(ctx);
        return mockDashboardCatalog(input);
      },
      async drilldown(id, input) {
        mockDashboard(ctx, id);
        return mockDashboardDrilldown(ctx, id, input);
      },
      async generateDrilldowns(id, input) {
        const d = mockDraft(ctx, id);
        if (d.draft_version !== input.expected_version)
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        const added = mockGenerateDrilldowns(d.spec, input.panel_id);
        if (added.length) d.draft_version++;
        ctx.save();
        return { draft: copy(d), added, issues: [] };
      },
      async folders() {
        dashboardPermission(ctx);
        return copy(
          dashboardState(ctx)
            .folders.filter((f) => f.enterprise === ctx.enterpriseId())
            .map((f) => f.value),
        );
      },
      async previewFolder(input) {
        dashboardPermission(ctx, true);
        return copy(
          preview(
            "telemetry.dashboard.folder",
            { ...input },
            input.name || input.operation,
          ),
        );
      },
      async previewLifecycle(id, input) {
        dashboardPermission(ctx, true);
        const item = mockDashboard(ctx, id);
        if (item.version !== input.expected_version)
          fail("DASHBOARD_VERSION_CONFLICT", 409);
        return copy(
          preview("telemetry.dashboard.lifecycle", { ...input, id }, item.name),
        );
      },
    },
  };
}

export function commitDashboardAction(
  ctx: BaseContext,
  action: PendingActionPublic,
): ConfirmActionResult | undefined {
  const plan = ctx.db.actionPlans[action.action_ref];
  if (!plan?.tool.startsWith("telemetry.dashboard.")) return undefined;
  dashboardPermission(
    ctx,
    !plan.tool.startsWith("telemetry.dashboard.binding."),
  );
  if (
    plan.enterprise_id !== ctx.enterpriseId() ||
    plan.created_by !== ctx.actor().id
  )
    fail("DASHBOARD_DENIED", 403);
  const state = dashboardState(ctx),
    input = plan.input_data;
  if (plan.tool.startsWith("telemetry.dashboard.binding.")) {
    commitDashboardBinding(ctx, plan.input_data);
  } else if (plan.tool === "telemetry.dashboard.publish") {
    const d = mockDraft(ctx, String(input.draft_id));
    draftBaseline(ctx, d);
    const folderVersion =
      state.folders.find((f) => f.value.id === d.folder_id)?.value.version ?? 0;
    if (
      d.draft_version !== input.draft_version ||
      (d.base_revision_id ?? "") !== input.base_revision_id ||
      d.base_object_version !== input.base_object_version ||
      folderVersion !== input.folder_version
    )
      fail("DASHBOARD_VERSION_CONFLICT", 409);
    const id = d.dashboard_id ?? crypto.randomUUID(),
      revId = crypto.randomUUID();
    const item = d.dashboard_id
      ? mockDashboard(ctx, id)
      : {
          id,
          name: d.name,
          description: d.description,
          active_revision_id: revId,
          version: 0,
          lifecycle: "active" as const,
          updated_at: ctx.nowIso(),
        };
    const value: DashboardRevision = {
      id: revId,
      dashboard_id: id,
      revision_number:
        state.revisions.filter((r) => r.dashboard_id === id).length + 1,
      name: d.name,
      description: d.description,
      spec: copy(d.spec),
      spec_hash: revId,
      validation: validateMockDashboard(d.spec),
      sample: { status: "requires_resource_authorization" },
      created_at: ctx.nowIso(),
    };
    if (!d.dashboard_id) {
      state.items.push({
        enterprise: ctx.enterpriseId(),
        creator: ctx.actor().id,
        value: item,
      });
      (ctx.db.dataAuthorizationGrants ??= []).push({
        subject_type: "user",
        subject_id: ctx.actor().id,
        resource_type: "dashboard",
        resource_id: id,
        active: true,
      });
    }
    Object.assign(item, {
      name: d.name,
      description: d.description,
      folder_id: d.folder_id,
      active_revision_id: revId,
      version: item.version + 1,
      updated_at: ctx.nowIso(),
    });
    state.revisions.push(value);
    d.status = "published";
    d.dashboard_id = id;
  } else if (plan.tool === "telemetry.dashboard.lifecycle") {
    const item = mockDashboard(ctx, String(input.id));
    if (item.version !== input.expected_version)
      fail("DASHBOARD_VERSION_CONFLICT", 409);
    item.lifecycle = input.operation === "archive" ? "archived" : "active";
    item.version++;
  } else if (plan.tool === "telemetry.dashboard.folder") {
    if (input.operation === "folder.create") {
      state.folders.push({
        enterprise: ctx.enterpriseId(),
        value: {
          id: crypto.randomUUID(),
          name: String(input.name),
          description: String(input.description),
          sort_order: Number(input.sort_order),
          status: "active",
          version: 1,
        },
      });
    } else {
      const f = state.folders.find(
        (f) => f.enterprise === ctx.enterpriseId() && f.value.id === input.id,
      )?.value;
      if (!f || f.version !== input.expected_version)
        fail("DASHBOARD_VERSION_CONFLICT", 409);
      if (
        input.operation === "folder.archive" &&
        state.items.some((r) => r.value.folder_id === f.id)
      )
        fail("DASHBOARD_INVALID");
      f.status = input.operation === "folder.archive" ? "archived" : "active";
      if (input.operation === "folder.update") {
        f.name = String(input.name);
        f.description = String(input.description);
      }
      f.version++;
    }
  }
  action.status = "succeeded";
  action.available_actions = [];
  action.updated_at = ctx.nowIso();
  const execution = {
    execution_id: crypto.randomUUID(),
    action_ref: action.action_ref,
    status: "succeeded" as const,
    one_time_result_state: "unavailable" as const,
    created_at: ctx.nowIso(),
    updated_at: ctx.nowIso(),
  };
  action.execution_ref = execution.execution_id;
  ctx.db.executions.unshift(execution);
  ctx.save();
  return { pending_action: copy(action), execution };
}
