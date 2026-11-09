import { expect, test } from "@playwright/test";
import type { MockDb } from "../../../packages/api-client/src/mock/store";
import { selectWorkbenchNamespace } from "./helpers/kubernetes-workbench";

test.beforeEach(async ({ page }) => {
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/kubernetes/k8s-prod-east");
  await expect(
    page.getByText("deployment-example", { exact: true }),
  ).toHaveCount(2);
});

test("Namespace dropdown loads candidates beyond the first page", async ({
  page,
}) => {
  await page.evaluate(() => {
    const key = "argus-mock:db-v14";
    const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
    const inventory = db.kubernetesInventory!["k8s-prod-east"]!;
    const template = inventory.objects.find(
      (row) => row.definition.key === "namespace",
    )!;
    for (let index = 0; index < 37; index++) {
      const namespace = {
        id: crypto.randomUUID(),
        uid: crypto.randomUUID(),
        name: `namespace-${String(index).padStart(3, "0")}`,
        version: 1,
      };
      inventory.namespaces.push(namespace);
      inventory.objects.push({
        ...structuredClone(template),
        uid: namespace.uid,
        name: namespace.name,
      });
    }
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.reload();
  const select = page.getByRole("combobox", {
    name: "命名空间范围",
    exact: true,
  });
  await select.fill("");
  const list = page.getByRole("listbox");
  await expect(
    page.getByRole("option", { name: "namespace-027", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: "namespace-036", exact: true }),
  ).toHaveCount(0);
  await list.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(
    page.getByRole("option", { name: "namespace-036", exact: true }),
  ).toBeAttached();
  await page
    .getByRole("option", { name: "namespace-036", exact: true })
    .click();
  await expect(select).toHaveValue("namespace-036");
  await expect(
    page.getByRole("button", { name: "新建对象", exact: true }),
  ).toBeEnabled();
  await expect(page.getByText("当前页没有资源", { exact: true })).toBeVisible();
  await select.fill("missing-namespace");
  await expect(list.getByRole("option")).toHaveCount(2);
  await expect(
    page.getByRole("option", { name: "namespace-036", exact: true }),
  ).toBeAttached();
  await page.keyboard.press("Escape");
  await expect(select).toHaveValue("namespace-036");
});

test("Namespace dropdown respects namespace-only read grants", async ({
  page,
}) => {
  await page.evaluate(() => {
    const key = "argus-mock:db-v14";
    const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
    const namespace = db.kubernetesInventory!["k8s-prod-east"]!.namespaces.find(
      (row) => row.name === "production",
    )!;
    for (const role of db.roles) role.permissions = ["kubernetes.read"];
    db.dataAuthorizationGrants = [
      {
        subject_type: "user",
        subject_id: "u-root",
        resource_type: "kubernetes_namespace",
        resource_id: namespace.id,
        active: true,
      },
    ];
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.reload();
  await expect(
    page.getByText("当前授权仅包含指定命名空间。", { exact: true }),
  ).toBeVisible();
  await selectWorkbenchNamespace(page, "production");
  const select = page.getByRole("combobox", {
    name: "命名空间范围",
    exact: true,
  });
  await select.fill("staging");
  await expect(page.getByRole("listbox").getByRole("option")).toHaveCount(2);
  await expect(
    page.getByRole("option", { name: "staging", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(select).toHaveValue("production");
  await expect(
    page.getByText("deployment-example", { exact: true }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "新建对象", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("tab", { name: "Node", exact: true }),
  ).toHaveCount(0);
});
