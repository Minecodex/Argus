import { expect, test } from "@playwright/test";

// Helm lifecycle remains covered by domain/API tests. The resource page no
// longer offers a Helm management entry, including for enterprise admins.
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`Kubernetes resource page removes Helm ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }) => {
      await page.addInitScript(
        ({ english, theme }) => {
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.goto("/login");
      await page.locator('input[autocomplete="username"]').fill("root");
      await page
        .locator('input[autocomplete="current-password"]')
        .fill("123456");
      await page.locator('form button[type="submit"]').click();
      await expect(page).not.toHaveURL(/\/login/);
      await page.goto("/kubernetes/k8s-prod-east?tab=resources");
      await expect(
        page.getByRole("tab", { name: "Deployment", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", {
          name: english ? "Helm releases" : "Helm 发布管理",
          exact: true,
        }),
      ).toHaveCount(0);
      await page
        .getByRole("button", {
          name: english ? "Operation history" : "操作记录",
          exact: true,
        })
        .click();
      await expect(
        page.getByRole("dialog", {
          name: english ? "Operation history" : "操作记录",
          exact: true,
        }),
      ).toBeVisible();
    });
  }
