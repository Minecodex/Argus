import { expect, test, type Page } from "@playwright/test";
import { dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import {
  confirmDashboardAction,
  dashboardAPI,
  objectGrant,
} from "./helpers/planv2-api";

const d = dashboardsZh.dashboards;
const login = createMfaLogin("enterprise"),
  editorLogin = createMfaLogin("enterprise", "EDITOR");
test.beforeEach(async ({ page }) => {
  test.skip(
    process.env.ARGUS_PLANV2_E2E !== "1",
    "Requires PlanV2 Kubernetes environment",
  );
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await login(
    page,
    "/login",
    process.env.ARGUS_PLANV2_USERNAME!,
    process.env.ARGUS_PLANV2_PASSWORD!,
  );
  await page.goto("/dashboards");
  await expect(
    page.getByRole("heading", { name: d.title, exact: true }),
  ).toBeVisible();
});

test("folder restoration retains personal drafts but never revives an old publication", async ({
  page,
}, info) => {
  test.setTimeout(180000);
  const prepared = await (
    await dashboardAPI(
      page,
      "/dashboard-folders/preview",
      "POST",
      {
        operation: "folder.create",
        name: "PlanV2 retained folder",
        description: "",
        sort_order: 0,
        expected_version: 0,
      },
      201,
    )
  ).json();
  await confirmDashboardAction(page, prepared.action_ref);
  const folder = (
    await (await dashboardAPI(page, "/dashboard-folders")).json()
  ).find((f: { name: string }) => f.name === "PlanV2 retained folder");
  const draft = await newDraft(page, "PlanV2 retained folder draft", folder.id);
  await page.goto(`/dashboard-drafts/${draft.id}`);
  const old = await preview(page);
  await page.keyboard.press("Escape");
  await folderAction(page, folder.id, "folder.archive");
  expect(
    (await (await dashboardAPI(page, `/dashboard-drafts/${draft.id}`)).json())
      .status,
  ).toBe("editing");
  await dashboardAPI(
    page,
    `/enterprise/pending-actions/${old}/confirm`,
    "POST",
    undefined,
    409,
  );
  await dashboardAPI(
    page,
    `/dashboard-drafts/${draft.id}/preview`,
    "POST",
    { expected_version: draft.draft_version },
    409,
  );
  await folderAction(page, folder.id, "folder.restore");
  // Restoring the folder changes its lifecycle version even though no draft content changed.
  const stale = await (
    await dashboardAPI(
      page,
      `/enterprise/pending-actions/${old}/confirm`,
      "POST",
      undefined,
      409,
    )
  ).json();
  expect(stale.code).toBe("DASHBOARD_VERSION_CONFLICT");
  await page.reload();
  await preview(page);
  await page
    .getByRole("dialog", { name: d.previewTitle, exact: true })
    .getByRole("button", { name: d.publish, exact: true })
    .click();
  await expect(page).toHaveURL(/\/dashboards\//);
  const board = page.url().split("/").at(-1)!;
  const revisions = await (
    await dashboardAPI(page, `/dashboards/${board}/revisions`)
  ).json();
  expect(revisions).toHaveLength(1);
  const audit = await (
    await dashboardAPI(
      page,
      `/enterprise/audit-events?${new URLSearchParams({ resource_id: folder.id })}`,
    )
  ).json();
  expect(audit.items.map((e: { action: string }) => e.action)).toEqual(
    expect.arrayContaining([
      "dashboard.folder.created",
      "dashboard.folder.archived",
      "dashboard.folder.restored",
    ]),
  );
  await page.screenshot({
    path: info.outputPath("folder-restored-publication.png"),
    fullPage: true,
  });
});

test("bindings require both current grants and disappear after their owned resource is deleted", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(360000);
  const cluster = process.env.ARGUS_PLANV2_DISPOSABLE_CLUSTER_ID!;
  const subject = process.env.ARGUS_PLANV2_EDITOR_ID!;
  const resource = await (
    await dashboardAPI(page, `/enterprise/kubernetes-clusters/${cluster}`)
  ).json();
  expect(resource.name).toBe("PlanV2 disposable shortcut");
  const draft = await newDraft(page, "PlanV2 binding lifecycle");
  const publish = await (
    await dashboardAPI(
      page,
      `/dashboard-drafts/${draft.id}/preview`,
      "POST",
      { expected_version: draft.draft_version },
      201,
    )
  ).json();
  await confirmDashboardAction(page, publish.action_ref);
  const board = (
    await (await dashboardAPI(page, `/dashboard-drafts/${draft.id}`)).json()
  ).dashboard_id;
  const path = `/kubernetes/clusters/${cluster}/dashboard-bindings`;
  let entry = await (await dashboardAPI(page, path)).json();
  const dashboard = (
    await (await dashboardAPI(page, `/dashboards/${board}`)).json()
  ).dashboard;
  const attached = await (
    await dashboardAPI(
      page,
      `${path}/preview`,
      "POST",
      {
        operation: "attach",
        dashboard_id: board,
        expected_dashboard_version: dashboard.version,
        expected_resource_version: entry.resource_version,
      },
      201,
    )
  ).json();
  await confirmDashboardAction(page, attached.action_ref);
  entry = await (await dashboardAPI(page, path)).json();
  const binding = entry.items.find(
    (b: { dashboard: { id: string } }) => b.dashboard.id === board,
  );
  const detach = {
    operation: "detach",
    dashboard_id: board,
    expected_dashboard_version: dashboard.version,
    expected_resource_version: entry.resource_version,
    binding_id: binding.id,
    expected_binding_version: binding.version,
  };
  await objectGrant(page, subject, "dashboard", [board], false);
  await objectGrant(page, subject, "kubernetes_cluster", [cluster], false);
  const peerContext = await browser.newContext({
    baseURL: process.env.ARGUS_E2E_ENTERPRISE_ORIGIN,
    ignoreHTTPSErrors: true,
  });
  try {
    await peerContext.addInitScript(() =>
      localStorage.setItem("argus.locale", "zh-CN"),
    );
    const peer = await peerContext.newPage();
    await peerSession(peer, `/kubernetes/${cluster}`);
    const links = peer.getByRole("region", {
      name: d.resourceLinks,
      exact: true,
    });
    await expect(
      links.getByRole("link", { name: dashboard.name, exact: true }),
    ).toBeVisible();
    const old = await (
      await dashboardAPI(peer, `${path}/preview`, "POST", detach, 201)
    ).json();
    await objectGrant(page, subject, "dashboard", [board], true);
    await peerSession(peer, `/kubernetes/${cluster}`);
    await expect(
      links.getByRole("link", { name: dashboard.name, exact: true }),
    ).toHaveCount(0);
    expect((await (await dashboardAPI(peer, path)).json()).items).toEqual([]);
    await dashboardAPI(
      peer,
      `/enterprise/pending-actions/${old.action_ref}/confirm`,
      "POST",
      undefined,
      409,
    );
    await objectGrant(page, subject, "dashboard", [board], false);
    await objectGrant(page, subject, "kubernetes_cluster", [cluster], true);
    await peerSession(peer, `/dashboards/${board}`);
    expect(
      await (await dashboardAPI(peer, `/dashboards/${board}/bindings`)).json(),
    ).toEqual([]);
    await dashboardAPI(peer, `${path}/preview`, "POST", detach, 403);
    await objectGrant(page, subject, "kubernetes_cluster", [cluster], false);
    await peerSession(peer, `/kubernetes/${cluster}`);
    await expect(
      links.getByRole("link", { name: dashboard.name, exact: true }),
    ).toBeVisible();
    await dashboardAPI(
      peer,
      `/enterprise/pending-actions/${old.action_ref}/confirm`,
      "POST",
      undefined,
      409,
    );

    const oldRoot = await (
      await dashboardAPI(page, `${path}/preview`, "POST", detach, 201)
    ).json();
    const removed = await (
      await dashboardAPI(
        page,
        `/enterprise/kubernetes-clusters/${cluster}/actions/preview-delete`,
        "POST",
        { expected_version: resource.resource_version },
        201,
      )
    ).json();
    await confirmDashboardAction(page, removed.action_ref);
    expect(
      await (await dashboardAPI(page, `/dashboards/${board}/bindings`)).json(),
    ).toEqual([]);
    await dashboardAPI(
      page,
      `/enterprise/pending-actions/${oldRoot.action_ref}/confirm`,
      "POST",
      undefined,
      409,
    );
    await dashboardAPI(
      page,
      `/dashboards/${board}/execute`,
      "POST",
      { resource_ids: [cluster] },
      403,
    );
    await page.goto(`/dashboards/${board}`);
    await page.getByRole("button", { name: "更多", exact: true }).click();
    await page.getByRole("menuitem", { name: "关联资源", exact: true }).click();
    await expect(page.getByText("暂无关联资源", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("heading", { name: dashboard.name, exact: true }),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("deleted-resource-retained-dashboard.png"),
      fullPage: true,
    });
  } finally {
    await peerContext.close();
  }
});

async function newDraft(page: Page, name: string, folder_id?: string) {
  const current = await (
    await dashboardAPI(
      page,
      `/dashboards/${process.env.ARGUS_PLANV2_GALLERY_ID!}`,
    )
  ).json();
  return (
    await dashboardAPI(
      page,
      "/dashboard-drafts",
      "POST",
      {
        name,
        folder_id,
        description: "Retained contents",
        spec: { ...current.revision.spec, panels: [], variables: [] },
        proposed_bindings: [],
      },
      201,
    )
  ).json();
}
async function folderAction(page: Page, id: string, operation: string) {
  const folder = (
    await (await dashboardAPI(page, "/dashboard-folders")).json()
  ).find((f: { id: string }) => f.id === id);
  const action = await (
    await dashboardAPI(
      page,
      "/dashboard-folders/preview",
      "POST",
      {
        id,
        operation,
        expected_version: folder.version,
        name: folder.name,
        description: folder.description,
        sort_order: folder.sort_order,
      },
      201,
    )
  ).json();
  await confirmDashboardAction(page, action.action_ref);
}
async function preview(page: Page) {
  const response = page.waitForResponse(
    (r) =>
      r.url().includes("/dashboard-drafts/") && r.url().endsWith("/preview"),
  );
  await page.getByRole("button", { name: d.preview, exact: true }).click();
  const result = await response;
  expect(result.status()).toBe(201);
  await expect(
    page.getByRole("dialog", { name: d.previewTitle, exact: true }),
  ).toBeVisible();
  return (await result.json()).action_ref as string;
}
async function peerSession(page: Page, path: string) {
  await editorLogin(
    page,
    "/login",
    process.env.ARGUS_PLANV2_EDITOR_USERNAME!,
    process.env.ARGUS_PLANV2_EDITOR_PASSWORD!,
  );
  await page.goto(path);
  await expect(page.getByRole("main")).toBeVisible();
}
