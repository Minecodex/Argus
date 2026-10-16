import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const english of [false, true])
  for (const dark of [false, true])
    for (const width of [1280, 1440, 1920]) {
      test(`Resource lists ${english ? "en" : "zh"} ${dark ? "dark" : "light"} ${width}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        await page.setViewportSize({ width, height: 900 });
        await page.addInitScript(
          ({ english, dark }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", dark ? "dark" : "light");
          },
          { english, dark },
        );
        await page.goto("/login");
        await page.getByLabel(english ? "Username" : "用户名").fill("root");
        await page.getByLabel(english ? "Password" : "密码").fill("123456");
        await page
          .getByRole("button", {
            name: english ? "Sign in" : "登录",
            exact: true,
          })
          .click();
        await expect(page).not.toHaveURL(/login/);
        for (const route of ["/hosts", "/kubernetes", "/dashboards"]) {
          await page.goto(route);
          await expect(page.getByRole("table").first()).toBeVisible();
          await expect(page.locator(".argus-resource-card")).toHaveCount(0);
          await expect(
            page
              .getByRole("columnheader", {
                name: english ? "Name" : "名称",
                exact: true,
              })
              .first(),
          ).toBeVisible();
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth > innerWidth,
            ),
          ).toBe(false);
          const audit = await new AxeBuilder({ page }).analyze();
          expect(
            audit.violations.filter((v) =>
              ["serious", "critical"].includes(v.impact ?? ""),
            ),
          ).toEqual([]);
          await page.screenshot({
            path: info.outputPath(route.slice(1) + ".png"),
            fullPage: true,
          });
        }
        await page.goto("/hosts");
        await page
          .getByRole("link", { name: "public-web-01", exact: true })
          .click();
        await expect(
          page.getByText(english ? "Host inventory" : "主机资产信息", {
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          page.getByRole("button", {
            name: english ? "Collect again" : "重新采集",
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          page.getByText(
            english ? "Inventory has not been collected" : "尚未采集资产信息",
            { exact: true },
          ),
        ).toBeVisible();
        await expect(page.locator(".argus-stat-card")).toHaveCount(0);
        await page
          .getByRole("tab", {
            name: english ? "Dashboards" : "仪表盘",
            exact: true,
          })
          .click();
        await expect(page.locator(".argus-resource-card")).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath("host-dashboard-list.png"),
          fullPage: true,
        });
      });
    }
