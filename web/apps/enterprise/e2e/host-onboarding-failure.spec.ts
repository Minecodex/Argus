import { expect, test } from "@playwright/test";

test("failed onboarding callback shows a safe diagnosis and offers retry", async ({
  page,
}) => {
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.addInitScript(() => {
    const key = Object.keys(localStorage).find((key) =>
      key.startsWith("argus-mock:db-"),
    )!;
    const db = JSON.parse(localStorage.getItem(key)!);
    const host = db.hosts.find(
      (host: { name: string }) => host.name === "public-web-01",
    );
    Object.assign(host, {
      connectionStatus: "onboarding",
      onboardingState: "install_failed",
      onboardingErrorCode: "HOST_ONBOARDING_CALLBACK_TLS_FAILED",
      onboardingOperationId: "ssh-failed-operation",
      connectorId: undefined,
    });
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.goto("/hosts");
  const tile = page
    .locator(".argus-host-tile")
    .filter({ hasText: "public-web-01" });
  await expect(tile.getByText("安装失败", { exact: true })).toBeVisible();
  await expect(tile.getByText("接入中", { exact: true })).toHaveCount(0);
  await expect(
    tile.getByText("回调 TLS 校验失败，请检查证书、SNI 与信任链。"),
  ).toBeVisible();
  await expect(
    tile.getByText("错误码：HOST_ONBOARDING_CALLBACK_TLS_FAILED"),
  ).toBeVisible();
  await expect(tile.getByRole("button", { name: "重新安装" })).toBeVisible();
  await tile.getByRole("button", { name: "查看安装进度" }).click();
  const dialog = page.getByRole("dialog", { name: "查看安装进度" });
  await expect(dialog.getByText("下载并传输安装文件").first()).toBeVisible();
  await expect(
    dialog.getByText("回调 TLS 校验失败，请检查证书、SNI 与信任链。"),
  ).toHaveCount(2);
  await expect(
    dialog.getByText("错误码：HOST_ONBOARDING_CALLBACK_TLS_FAILED"),
  ).toHaveCount(2);
  await page.keyboard.press("Escape");
  await tile.getByRole("button", { name: "重新安装" }).click();
  await expect(page.getByRole("dialog", { name: "重新安装" })).toBeVisible();
});
