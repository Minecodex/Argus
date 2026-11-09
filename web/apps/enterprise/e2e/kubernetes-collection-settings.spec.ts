import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`collection choices ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1100 });
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
        await page.goto("/kubernetes/k8s-prod-east#otlp-collector");
        const panel = page.getByRole("tabpanel", {
          name: text("采集配置", "Collection"),
          exact: true,
        });
        await expect(
          panel.getByText(text("采集内容", "What to collect"), { exact: true }),
        ).toBeVisible();
        await expect(
          panel.getByRole("table").getByRole("columnheader"),
        ).toHaveCount(4);
        await expect(panel.getByRole("switch")).toHaveCount(8);
        await expect(panel.getByRole("textbox")).toHaveCount(0);
        await expect(
          panel.getByText(/Claim|Revision|Digest|Profile/),
        ).toHaveCount(0);
        const node = panel.getByRole("row").filter({
          has: page.getByText(
            text("节点与容器指标", "Node and container metrics"),
            { exact: true },
          ),
        });
        await node.getByRole("switch").focus();
        await node.getByRole("switch").press("Space");
        const confirmation = page.getByRole("dialog", {
          name: text("自身监控配置预览", "Preview own resource monitoring"),
          exact: true,
        });
        await expect(confirmation).toBeVisible();
        await confirmation
          .getByRole("button", { name: /^(确认|Confirm)/ })
          .click();
        await expect(confirmation).not.toBeVisible();
        await expect(node.getByRole("switch")).toBeChecked();
        await panel
          .getByRole("button", {
            name: text("采集配置 · 更多操作", "Collection · More actions"),
            exact: true,
          })
          .click();
        await page
          .getByRole("menuitem", {
            name: text("收集器管理", "Collector management"),
            exact: true,
          })
          .click();
        const settings = page.getByRole("dialog", {
          name: text("收集器管理", "Collector management"),
          exact: true,
        });
        await expect(
          settings.getByLabel(text("内网镜像地址", "Internal image address")),
        ).toBeVisible();
        await page.keyboard.press("Escape");
        await expect(settings).not.toBeVisible();
        await expect(panel.getByRole("textbox")).toHaveCount(0);
        expect(
          await panel.evaluate(
            (element) => element.scrollWidth <= element.clientWidth,
          ),
        ).toBe(true);
        expect(
          (await new AxeBuilder({ page }).analyze()).violations.filter(
            (item) => item.impact === "critical" || item.impact === "serious",
          ),
        ).toEqual([]);
        await page.getByRole("heading", { level: 1 }).click();
        await panel.screenshot({
          path: info.outputPath("collection-choices.png"),
        });
        expect(errors).toEqual([]);
      });
    }
