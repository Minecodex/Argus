import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`KWR overview and resource detail ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90000);
        const errors: string[] = [];
        page.on("pageerror", (e) => errors.push(e.message));
        await page.setViewportSize({ width, height: 1080 });
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
        await page.goto("/kubernetes/k8s-prod-east");
        await expect(
          page.getByRole("tab", {
            name: english ? "Overview" : "大盘",
            exact: true,
          }),
        ).toHaveAttribute("aria-selected", "true");
        await expect(page.locator(".argus-resource-summary")).toBeVisible();
        await expect(
          page.getByRole("combobox", {
            name: english ? "Namespace scope" : "命名空间范围",
          }),
        ).toHaveValue(
          english ? "All authorized namespaces" : "全部已授权命名空间",
        );
        await expect(page.locator("iframe")).toHaveCount(0);
        await expect(page.locator(".argus-resource-summary")).not.toContainText(
          "undefined",
        );
        await expect(
          page.locator(".argus-dashboard-tile").first(),
        ).toBeVisible();
        const overviewAxe = await new AxeBuilder({ page }).analyze();
        expect(
          overviewAxe.violations.filter(
            (value) =>
              value.impact === "critical" || value.impact === "serious",
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("overview.png"),
          fullPage: true,
        });
        await page
          .getByRole("tab", {
            name: english ? "Resources" : "资源查询",
            exact: true,
          })
          .click();
        await expect(
          page.getByRole("columnheader", {
            name: "resourceVersion",
            exact: true,
          }),
        ).toHaveCount(0);
        await page
          .getByRole("button", { name: "deployment-example", exact: true })
          .first()
          .click();
        const drawer = page.getByRole("dialog", {
          name: "Deployment · deployment-example",
          exact: true,
        });
        await expect(drawer).toBeVisible();
        await expect(page).toHaveURL(/resource_uid=/);
        await drawer
          .getByRole("tab", {
            name: english ? "Technical details" : "技术信息",
            exact: true,
          })
          .click();
        await expect(
          drawer.getByText("resourceVersion", { exact: true }),
        ).toBeVisible();
        await page.screenshot({
          path: info.outputPath("resource-detail.png"),
          fullPage: true,
        });
        await page.keyboard.press("Escape");
        await expect(
          page
            .getByRole("button", { name: "deployment-example", exact: true })
            .first(),
        ).toBeVisible();
        const axe = await new AxeBuilder({ page }).analyze();
        expect(
          axe.violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        expect(errors).toEqual([]);
      });
    }
