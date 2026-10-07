import { expect, test, type Page } from "@playwright/test";
import { dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { selectTrigger } from "./helpers/select";
import {
  confirmDashboardAction,
  dashboardAPI,
  objectGrant,
} from "./helpers/planv2-api";

const d = dashboardsZh.dashboards;
const login = createMfaLogin("enterprise");
const editorLogin = createMfaLogin("enterprise", "EDITOR");
test.use({ actionTimeout: 15000, navigationTimeout: 30000 });
test.beforeEach(async ({ page }) => {
  test.skip(
    process.env.ARGUS_PLANV2_E2E !== "1",
    "Requires PlanV2 real Kubernetes environment",
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

test("two editors reconcile published conflicts and current object grants protect old previews", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(300000);
  const board = process.env.ARGUS_PLANV2_COLLABORATION_ID!;
  const subject = process.env.ARGUS_PLANV2_EDITOR_ID!;
  const cluster = process.env.ARGUS_PLANV2_CLUSTER_ID!;
  const peerContext = await browser.newContext({
    baseURL: process.env.ARGUS_E2E_ENTERPRISE_ORIGIN,
    ignoreHTTPSErrors: true,
  });
  try {
    await peerContext.addInitScript(() => {
      localStorage.setItem("argus.locale", "zh-CN");
      localStorage.setItem("argus.theme", "dark");
    });
    const peer = await peerContext.newPage();
    await editorLogin(
      peer,
      "/login",
      process.env.ARGUS_PLANV2_EDITOR_USERNAME!,
      process.env.ARGUS_PLANV2_EDITOR_PASSWORD!,
    );
    await peer.goto(`/dashboards/${board}`);
    await expect(
      peer.getByRole("heading", { name: "PlanV2 collaboration", exact: true }),
    ).toBeVisible();
    const session = await (
      await dashboardAPI(peer, "/enterprise/auth/session")
    ).json();
    expect(session.permissions).toContain("telemetry.dashboard.manage");
    expect(session.permissions).not.toContain("role.read");
    await dashboardAPI(peer, "/enterprise/roles", "GET", undefined, 403);
    // Object access does not imply access to the data behind that object.
    expect(
      (await (await dashboardAPI(peer, `/dashboards/${board}`)).json()).revision
        .spec.panels,
    ).toHaveLength(1);
    await dashboardAPI(peer, `/dashboards/${board}/execute`, "POST", {}, 403);
    await objectGrant(page, subject, "kubernetes_cluster", [cluster], false);
    await editorSession(peer, `/dashboards/${board}`);
    await expect(
      peer.getByRole("heading", { name: "PlanV2 collaboration", exact: true }),
    ).toBeVisible();
    const allowed = await (
      await dashboardAPI(peer, `/dashboards/${board}/execute`, "POST", {
        resource_ids: [cluster],
      })
    ).json();
    expect(allowed.panels[0].status).toBe("success");
    await page.goto(`/dashboards/${board}`);
    await page.getByRole("button", { name: d.edit, exact: true }).click();
    await peer.getByRole("button", { name: d.edit, exact: true }).click();
    await expect(page).toHaveURL(/dashboard-drafts/);
    await expect(peer).toHaveURL(/dashboard-drafts/);
    const ownDraft = page.url().split("/").at(-1)!;
    const peerDraft = peer.url().split("/").at(-1)!;
    expect(peerDraft).not.toBe(ownDraft);
    const a = await (
      await dashboardAPI(page, `/dashboard-drafts/${ownDraft}`)
    ).json();
    const b = await (
      await dashboardAPI(peer, `/dashboard-drafts/${peerDraft}`)
    ).json();
    expect(a.base_revision_id).toBe(b.base_revision_id);
    await dashboardAPI(
      peer,
      `/dashboard-drafts/${ownDraft}`,
      "GET",
      undefined,
      404,
    );
    await describe(page, "Editor A published");
    await describe(peer, "Editor B retained");
    await preview(peer);
    await publish(page);
    const rejected = peer.waitForResponse((r) => r.url().endsWith("/confirm"));
    await peer
      .getByRole("dialog", { name: d.previewTitle, exact: true })
      .getByRole("button", { name: d.publish, exact: true })
      .click();
    expect((await (await rejected).json()).code).toBe(
      "DASHBOARD_VERSION_CONFLICT",
    );
    const conflict = peer.getByRole("dialog", {
      name: d.conflict,
      exact: true,
    });
    await expect(conflict).toContainText("Editor A published");
    await expect(conflict).toContainText("Editor B retained");
    await conflict.screenshot({
      path: info.outputPath("two-editor-conflict.png"),
    });
    await conflict
      .getByRole("button", { name: d.acknowledge, exact: true })
      .click();
    await expect(conflict).not.toBeVisible();
    const committed = await publish(peer);
    const before = await (
      await dashboardAPI(peer, `/dashboards/${board}/revisions`)
    ).json();
    await confirmDashboardAction(peer, committed.ref, committed.key);
    expect(
      (
        await (
          await dashboardAPI(peer, `/dashboards/${board}/revisions`)
        ).json()
      ).length,
    ).toBe(before.length);

    await peer.getByRole("button", { name: d.edit, exact: true }).click();
    await expect(peer).toHaveURL(/dashboard-drafts/);
    const retained = peer.url().split("/").at(-1)!;
    await describe(peer, "Old preview text");
    const old = await preview(peer);
    await peer.keyboard.press("Escape");
    await describe(peer, "Retained across permission change");
    const edited = await (
      await dashboardAPI(
        peer,
        `/enterprise/pending-actions/${old}/confirm`,
        "POST",
        undefined,
        409,
      )
    ).json();
    expect(edited.code).toBe("DASHBOARD_VERSION_CONFLICT");
    const revokedPreview = await preview(peer);
    await peer.keyboard.press("Escape");
    await objectGrant(page, subject, "dashboard", [board], true);
    // Authorization changes invalidate the authenticated session as well as
    // the old publication. Prove that boundary, then inspect current grants
    // under a new normal MFA login, keeping the personal draft in storage.
    await peer.reload();
    await expect(peer).toHaveURL(/\/login/);
    await editorSession(peer, "/dashboards");
    await expect(
      peer.getByRole("heading", { name: d.title, exact: true }),
    ).toBeVisible();
    await dashboardAPI(peer, `/dashboards/${board}`, "GET", undefined, 403);
    await dashboardAPI(
      peer,
      `/dashboard-drafts/${retained}`,
      "GET",
      undefined,
      403,
    );
    await dashboardAPI(
      peer,
      `/dashboards/${board}/execute`,
      "POST",
      { resource_ids: [cluster] },
      403,
    );
    await dashboardAPI(
      peer,
      `/enterprise/pending-actions/${revokedPreview}/confirm`,
      "POST",
      undefined,
      409,
    );
    await objectGrant(page, subject, "dashboard", [board], false);
    await editorSession(peer, `/dashboard-drafts/${retained}`);
    await expect(
      peer.getByRole("button", { name: d.metadata, exact: true }),
    ).toBeVisible();
    const restored = await (
      await dashboardAPI(peer, `/dashboard-drafts/${retained}`)
    ).json();
    expect(restored.description).toBe("Retained across permission change");
    await dashboardAPI(
      peer,
      `/enterprise/pending-actions/${revokedPreview}/confirm`,
      "POST",
      undefined,
      409,
    );
    await peer.reload();
    await publish(peer);
    await expect(
      peer.getByText("Retained across permission change", { exact: false }),
    ).toBeVisible();
    await peer.screenshot({
      path: info.outputPath("two-editors-restored-access.png"),
      fullPage: true,
    });
  } finally {
    await peerContext.close();
  }
});

test("nonempty folders require published migration and preserve history across restoration", async ({
  page,
}, info) => {
  test.setTimeout(180000);
  const name = "PlanV2 folder migration";
  await page.goto("/dashboards");
  await page.getByRole("button", { name: d.createFolder, exact: true }).click();
  let dialog = page.getByRole("dialog", { name: d.createFolder, exact: true });
  await dialog.getByRole("textbox", { name: d.name, exact: true }).fill(name);
  await dialog.getByRole("button", { name: d.done, exact: true }).click();
  await confirmChange(page);
  const folders = await (await dashboardAPI(page, "/dashboard-folders")).json();
  const folder = folders.find((v: { name: string }) => v.name === name);
  expect(folder.status).toBe("active");
  await page.getByRole("button", { name: d.create, exact: true }).click();
  dialog = page.getByRole("dialog", { name: d.create, exact: true });
  await dialog
    .getByRole("textbox", { name: d.name, exact: true })
    .fill("Migrated dashboard");
  await selectTrigger(dialog, d.folder).click();
  await page.getByRole("option", { name, exact: true }).click();
  await dialog.getByRole("button", { name: d.done, exact: true }).click();
  await expect(page).toHaveURL(/dashboard-drafts/);
  await publish(page);
  const board = page.url().split("/").at(-1)!;
  await page.goto("/dashboards");
  await selectTrigger(page, d.folder).click();
  await page.getByRole("option", { name, exact: true }).click();
  const denied = page.waitForResponse((r) =>
    r.url().endsWith("/dashboard-folders/preview"),
  );
  await page
    .getByRole("button", { name: d.archiveFolder, exact: true })
    .click();
  expect((await (await denied).json()).code).toBe("DASHBOARD_VERSION_CONFLICT");
  await page.goto(`/dashboards/${board}`);
  await page.getByRole("button", { name: d.edit, exact: true }).click();
  await expect(page).toHaveURL(/dashboard-drafts/);
  await page.getByRole("button", { name: d.metadata, exact: true }).click();
  dialog = page.getByRole("dialog", { name: d.metadata, exact: true });
  await selectTrigger(dialog, d.folder).click();
  await page.getByRole("option", { name: d.ungrouped, exact: true }).click();
  const saved = page.waitForResponse(
    (r) =>
      r.request().method() === "PATCH" &&
      r.url().includes("/dashboard-drafts/"),
  );
  await dialog.getByRole("button", { name: d.done, exact: true }).click();
  expect((await saved).status()).toBe(200);
  await publish(page);
  expect(
    (await (await dashboardAPI(page, `/dashboards/${board}`)).json()).dashboard
      .folder_id ?? null,
  ).toBeNull();
  await page.goto("/dashboards");
  await selectTrigger(page, d.folder).click();
  await page.getByRole("option", { name, exact: true }).click();
  await page
    .getByRole("button", { name: d.archiveFolder, exact: true })
    .click();
  await confirmChange(page);
  await expect(
    page.getByRole("button", { name: d.restoreFolder, exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: d.restoreFolder, exact: true })
    .click();
  await confirmChange(page);
  await expect(
    page.getByRole("button", { name: d.archiveFolder, exact: true }),
  ).toBeVisible();
  expect(
    (await (await dashboardAPI(page, `/dashboards/${board}/revisions`)).json())
      .length,
  ).toBe(2);
  await page.screenshot({
    path: info.outputPath("folder-after-migration.png"),
    fullPage: true,
  });
});

test("Host and Kubernetes shortcuts preselect resources without narrowing the dashboard query scope", async ({
  page,
}, info) => {
  test.setTimeout(180000);
  const board = process.env.ARGUS_PLANV2_GALLERY_ID!;
  const cluster = process.env.ARGUS_PLANV2_CLUSTER_ID!;
  const hosts = await (await dashboardAPI(page, "/enterprise/hosts")).json();
  expect(hosts.items.length).toBeGreaterThan(0);
  for (const [route, id] of [
    ["hosts", hosts.items[0].id],
    ["kubernetes", cluster],
  ]) {
    await page.goto(`/${route}/${id}`);
    await page.getByRole("tab", { name: "仪表盘", exact: true }).click();
    const links = page.getByRole("region", {
      name: d.resourceLinks,
      exact: true,
    });
    await links
      .getByRole("button", { name: d.manageBindings, exact: true })
      .click();
    const manage = page.getByRole("dialog", {
      name: d.manageBindings,
      exact: true,
    });
    await manage
      .getByRole("button", {
        name: "关联 PlanV2 real Metrics gallery",
        exact: true,
      })
      .click();
    const review = page.getByRole("dialog", {
      name: d.bindingPreviewTitle,
      exact: true,
    });
    await review.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(review).not.toBeVisible();
    await manage.getByRole("button", { name: d.close, exact: true }).click();
    await expect(manage).not.toBeVisible();
    const result = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${board}/execute`) && r.status() === 200,
    );
    await links
      .getByRole("link", { name: "PlanV2 real Metrics gallery", exact: true })
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/dashboards/${board}\\?resource=${id}`),
    );
    const response = await result;
    expect(response.request().postDataJSON().resource_ids).toEqual([id]);
    const execution = await response.json();
    expect(execution.resources.map((r: { id: string }) => r.id)).toEqual([id]);
    if (route === "hosts") {
      // The marker is only sent to the cluster. A host shortcut cannot fetch it.
      expect(
        execution.panels.every((p: { targets: Array<{ data: unknown }> }) =>
          p.targets.every(
            (t) =>
              !JSON.stringify(t.data).includes("argus_m7_e2e_gauge_planv2"),
          ),
        ),
      ).toBe(true);
    }
    await page.screenshot({
      path: info.outputPath(`${route}-shortcut.png`),
      fullPage: true,
    });
    await page.goto(`/${route}/${id}`);
    await page.getByRole("tab", { name: "仪表盘", exact: true }).click();
    await links
      .getByRole("button", { name: d.manageBindings, exact: true })
      .click();
    await manage
      .getByRole("button", {
        name: "解除关联 PlanV2 real Metrics gallery",
        exact: true,
      })
      .click();
    await review.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(review).not.toBeVisible();
    await page.keyboard.press("Escape");
    await expect(
      links.getByRole("link", {
        name: "PlanV2 real Metrics gallery",
        exact: true,
      }),
    ).toHaveCount(0);
  }
  const unbound = await (
    await dashboardAPI(page, `/dashboards/${board}/execute`, "POST", {
      resource_ids: [cluster],
    })
  ).json();
  expect(
    unbound.panels.every((p: { status: string }) => p.status === "success"),
  ).toBe(true);
});

async function describe(page: Page, value: string) {
  await page.getByRole("button", { name: d.metadata, exact: true }).click();
  await page.getByLabel(d.descriptionField).fill(value);
  const saved = page.waitForResponse(
    (r) =>
      r.request().method() === "PATCH" &&
      r.url().includes("/dashboard-drafts/"),
  );
  await page
    .getByRole("dialog", { name: d.metadata, exact: true })
    .getByRole("button", { name: d.done, exact: true })
    .click();
  expect((await saved).status()).toBe(200);
}
async function editorSession(page: Page, path: string) {
  await editorLogin(
    page,
    "/login",
    process.env.ARGUS_PLANV2_EDITOR_USERNAME!,
    process.env.ARGUS_PLANV2_EDITOR_PASSWORD!,
  );
  await page.goto(path);
}
async function preview(page: Page) {
  const prepared = page.waitForResponse(
    (r) =>
      r.url().includes("/dashboard-drafts/") && r.url().endsWith("/preview"),
  );
  await page.getByRole("button", { name: d.preview, exact: true }).click();
  const response = await prepared;
  expect(response.status()).toBe(201);
  await expect(
    page.getByRole("dialog", { name: d.previewTitle, exact: true }),
  ).toBeVisible();
  return (await response.json()).action_ref as string;
}
async function publish(page: Page) {
  const ref = await preview(page);
  const confirmed = page.waitForResponse((r) =>
    r.url().endsWith(`/pending-actions/${ref}/confirm`),
  );
  await page
    .getByRole("dialog", { name: d.previewTitle, exact: true })
    .getByRole("button", { name: d.publish, exact: true })
    .click();
  const response = await confirmed;
  expect(response.status()).toBe(200);
  await expect(page).toHaveURL(/\/dashboards\//);
  return { ref, key: response.request().headers()["idempotency-key"]! };
}
async function confirmChange(page: Page) {
  const dialog = page.getByRole("dialog", {
    name: d.changePreviewTitle,
    exact: true,
  });
  await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
  await expect(dialog).not.toBeVisible();
}
