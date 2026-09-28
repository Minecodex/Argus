import { describe, expect, it } from "vitest";
import { createMockApiClient } from "./index";
import { emptyDashboardSpec } from "../dashboard";

describe("dashboard mock contract", () => {
  it("keeps invalid drafts editable and does not fabricate a successful sample", async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    await api.auth.login({ username: "root", password: "123456" });
    const spec = emptyDashboardSpec();
    spec.layout.columns = 13;
    const draft = await api.dashboards.createDraft({
      name: "Invalid layout",
      description: "",
      spec,
      proposed_bindings: [],
    });
    const result = await api.dashboards.sample(draft.id, draft.draft_version);
    expect(result.validation.valid).toBe(false);
    expect(result.validation.issues.length).toBeGreaterThan(0);
    expect(result.sample).toEqual({
      status: "not_executed",
      reason: "configuration_invalid",
    });
    await expect(
      api.dashboards.preview(draft.id, draft.draft_version),
    ).rejects.toMatchObject({ code: "DASHBOARD_INVALID" });
    expect((await api.dashboards.draft(draft.id)).status).toBe("editing");
  });
  it("keeps drafts private to publication and rejects stale publication previews", async () => {
    const api = createMockApiClient({ persist: false, delay: 0 });
    await api.auth.login({ username: "root", password: "123456" });
    const draft = await api.dashboards.createDraft({
      name: "Test",
      description: "",
      proposed_bindings: [],
      spec: emptyDashboardSpec(),
    });
    expect(await api.dashboards.list()).toHaveLength(0);
    const preview = await api.dashboards.preview(draft.id, draft.draft_version);
    const saved = await api.dashboards.saveDraft(draft.id, {
      name: "New name",
      description: "",
      spec: draft.spec,
      proposed_bindings: [],
      expected_version: draft.draft_version,
    });
    await expect(
      api.approvals.confirm(preview.action_ref),
    ).rejects.toMatchObject({ code: "DASHBOARD_VERSION_CONFLICT" });
    const fresh = await api.dashboards.preview(draft.id, saved.draft_version);
    await api.approvals.confirm(fresh.action_ref);
    const items = await api.dashboards.list();
    expect(items).toHaveLength(1);
    expect(items[0]?.name).toBe("New name");
    const edit = await api.dashboards.createDraft({
      dashboard_id: items[0]!.id,
      name: "ignored",
      description: "",
      spec: emptyDashboardSpec(),
      proposed_bindings: [],
    });
    await api.dashboards.saveDraft(edit.id, {
      name: "Unpublished",
      description: "",
      spec: edit.spec,
      proposed_bindings: [],
      expected_version: edit.draft_version,
    });
    expect((await api.dashboards.get(items[0]!.id)).dashboard.name).toBe(
      "New name",
    );
  });
});
