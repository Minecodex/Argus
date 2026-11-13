import { readFileSync } from "node:fs";
import { expect, test, request } from "@playwright/test";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";
import { selectRowAction } from "./helpers/action-menu";
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
const root = process.env.ARGUS_KWR_ARTIFACTS ?? "",
  mapping = root
    ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
    : {};
async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_KWR_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/login/);
}
test.describe("KWR actual lifecycle completion", () => {
  test.skip(
    process.env.ARGUS_KWR_REAL !== "1",
    "Requires owned real resources",
  );
  test("dedicated image form rolls the actual deployment", async ({
    page,
  }, info) => {
    test.setTimeout(120000);
    await login(page);
    await page.goto("/kubernetes/" + mapping.cluster + "?tab=resources");
    await selectWorkbenchNamespace(page, "kwr-workloads");
    const row = page
      .getByRole("row")
      .filter({
        has: page.getByRole("button", { name: "kwr-api", exact: true }),
      });
    await selectRowAction(row, "更新镜像");
    const dialog = page.getByRole("dialog", {
      name: "更新镜像 · kwr-api",
      exact: true,
    });
    await expect(dialog.getByRole("textbox", { name: /main/ })).toHaveValue(
      "busybox:1.36",
    );
    await dialog.getByRole("textbox", { name: /main/ }).fill("busybox:1.37");
    await dialog
      .getByRole("button", { name: "验证并预览差异", exact: true })
      .click();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
      "已生效",
      { timeout: 90000 },
    );
    await expect(row).toContainText("busybox:1.37");
    await page.screenshot({
      path: info.outputPath("actual-image-rollout.png"),
      fullPage: true,
    });
  });
  test("dedicated PVC expansion waits for actual capacity", async ({
    page,
  }, info) => {
    test.setTimeout(180000);
    const pvc = JSON.parse(readFileSync(root + "/pvc-fixture.json", "utf8"));
    await login(page);
    await page.goto("/kubernetes/" + pvc.cluster + "?tab=resources");
    await selectWorkbenchNamespace(page, pvc.namespace_name);
    await page.getByRole("tab", { name: "存储", exact: true }).click();
    await page
      .getByRole("tab", { name: "PersistentVolumeClaim", exact: true })
      .click();
    const row = page
      .getByRole("row")
      .filter({
        has: page.getByRole("button", { name: pvc.claim, exact: true }),
      });
    await selectRowAction(row, "扩容");
    const dialog = page.getByRole("dialog", {
      name: "扩容 · " + pvc.claim,
      exact: true,
    });
    await expect(dialog.getByRole("textbox", { name: /目标容量/ })).toHaveValue(
      "32Mi",
    );
    await dialog.getByRole("textbox", { name: /目标容量/ }).fill("64Mi");
    await dialog
      .getByRole("button", { name: "验证并预览差异", exact: true })
      .click();
    await dialog.getByRole("button", { name: "确认执行", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.locator(".argus-k8s-operation-feedback")).toContainText(
      "已生效",
      { timeout: 120000 },
    );
    await expect(row).toContainText("64Mi");
    await page.screenshot({
      path: info.outputPath("actual-pvc-expansion.png"),
      fullPage: true,
    });
  });
  test("revoking the actual namespace grant removes displayed data", async ({
    page,
  }, info) => {
    test.setTimeout(90000);
    const credentials = JSON.parse(
        readFileSync(root + "/private/reader-login.json", "utf8"),
      ),
      admin = JSON.parse(
        readFileSync(root + "/private/enterprise-session.json", "utf8"),
      ),
      reader = JSON.parse(
        readFileSync(root + "/reader-resources.json", "utf8"),
      );
    await page.goto("/login");
    await page
      .locator('input[autocomplete="username"]')
      .fill(credentials.username);
    await page
      .locator('input[autocomplete="current-password"]')
      .fill(credentials.password);
    await page.locator('form button[type="submit"]').click();
    await expect(page).not.toHaveURL(/login/);
    await page.goto("/kubernetes/" + mapping.cluster);
    await expect(page.locator(".argus-resource-summary")).toBeVisible();
    const api = await request.newContext({
      baseURL: "https://argus.dev",
      storageState: {
        cookies: admin.cookies.map((c: object) => ({
          ...c,
          httpOnly: true,
          sameSite: "Lax" as const,
          expires: -1,
        })),
        origins: [],
      },
      extraHTTPHeaders: {
        Origin: "https://argus.dev",
        "X-CSRF-Token": admin.csrf,
      },
    });
    try {
      const path = "/api/v1/enterprise/data-authorizations/user/" + reader.user;
      const grants = await (
        await api.get(path + "?resource_type=kubernetes_namespace")
      ).json();
      const response = await api.post(path, {
        headers: { "Idempotency-Key": crypto.randomUUID() },
        data: {
          resource_type: "kubernetes_namespace",
          resource_ids: [mapping.namespace],
          remove: true,
          expected_version: grants.authorization_version,
        },
      });
      expect(response.status()).toBe(204);
      await page.getByRole("button", { name: "刷新", exact: true }).click();
      await expect(page.locator(".argus-resource-summary")).toHaveCount(0, {
        timeout: 15000,
      });
      await expect(page.locator(".argus-node-allocation")).toHaveCount(0);
      await page.screenshot({
        path: info.outputPath("actual-grant-revoked.png"),
        fullPage: true,
      });
    } finally {
      await api.dispose();
    }
  });
});
