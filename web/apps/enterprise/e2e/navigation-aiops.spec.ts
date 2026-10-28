import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import type { MockDb } from "../../../packages/api-client/src/mock/store";

async function login(page: Page, english = false, theme = "light") {
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.getByLabel(english ? "Username" : "用户名").fill("root");
  await page.getByLabel(english ? "Password" : "密码").fill("123456");
  await page
    .getByRole("button", { name: english ? "Sign in" : "登录", exact: true })
    .click();
  await expect(page).not.toHaveURL(/login/);
}

for (const width of [1280, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`model governance control alignment ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        await login(page, english, theme);
        await page.goto("/settings/aiops?tab=models");
        await page
          .getByRole("button", {
            name: english ? "View governance dashboard" : "查看治理仪表盘",
            exact: true,
          })
          .click();
        const month = page.getByRole("textbox", {
          name: english ? "Month" : "月份",
          exact: true,
        });
        const model = page.getByRole("button", {
          name: english ? "Model" : "模型",
          exact: true,
        });
        const refresh = page.getByRole("button", {
          name: english ? "Refresh" : "刷新",
          exact: true,
        });
        await expect(month).toBeVisible();
        await expect(refresh).toBeEnabled();
        const controls = await Promise.all([
          month.boundingBox(),
          model.boundingBox(),
          refresh.boundingBox(),
        ]);
        const boxes = controls.map((box) => box!);
        expect(
          Math.max(...boxes.map((box) => box.y)) -
            Math.min(...boxes.map((box) => box.y)),
        ).toBeLessThanOrEqual(1);
        expect(
          Math.max(...boxes.map((box) => box.y + box.height)) -
            Math.min(...boxes.map((box) => box.y + box.height)),
        ).toBeLessThanOrEqual(1);
        await refresh.click();
        await expect(refresh).toBeEnabled();
        await page.screenshot({
          path: info.outputPath("controls-aligned.png"),
          animations: "disabled",
        });
      });
    }

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`navigation and AIOps ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        await page.setViewportSize({ width, height: 1000 });
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        const tx = (zh: string, en: string) => (english ? en : zh);
        await login(page, english, theme);
        await page.goto("/hosts");
        const nav = page.getByRole("navigation", {
          name: tx("主导航", "Main navigation"),
        });
        const groups = nav.locator(".argus-nav-section");
        await expect(groups.locator(".argus-nav-section__label")).toHaveText([
          tx("资源", "Resources"),
          tx("观测", "Observability"),
          tx("执行治理", "Execution Governance"),
          tx("企业设置", "Enterprise Settings"),
        ]);
        expect(
          await groups
            .nth(0)
            .getByRole("link")
            .evaluateAll((links) =>
              links.map((link) => link.getAttribute("href")),
            ),
        ).toEqual(["/hosts", "/kubernetes", "/monitoring", "/applications"]);
        expect(
          await groups
            .nth(1)
            .getByRole("link")
            .evaluateAll((links) =>
              links.map((link) => link.getAttribute("href")),
            ),
        ).toEqual(["/dashboards", "/alerts", "/observe"]);
        await expect(
          nav.getByRole("link", {
            name: tx("模型设置", "Model Settings"),
            exact: true,
          }),
        ).toHaveCount(0);
        await expect(
          nav.getByRole("link", {
            name: tx("MCP 连接", "MCP connections"),
            exact: true,
          }),
        ).toHaveCount(0);

        await nav
          .getByRole("link", {
            name: tx("中间件（监控）", "Middleware Monitoring"),
            exact: true,
          })
          .click();
        await expect(
          page.getByRole("heading", {
            name: tx("中间件（监控）", "Middleware Monitoring"),
            exact: true,
          }),
        ).toBeVisible();
        await nav
          .getByRole("link", { name: tx("查询", "Queries"), exact: true })
          .click();
        await expect(page).toHaveURL(/\/observe/);
        await expect(
          page.getByRole("heading", {
            name: tx("观测查询", "Observability queries"),
            exact: true,
          }),
        ).toBeVisible();

        const aiops = nav.getByRole("link", {
          name: tx("AIOps 设置", "AIOps Settings"),
          exact: true,
        });
        await aiops.click();
        await expect(page.getByRole("heading", { level: 1 })).toHaveText(
          tx("AIOps 设置", "AIOps Settings"),
        );
        const models = page.getByRole("tab", {
          name: tx("模型设置", "Model Settings"),
          exact: true,
        });
        const mcp = page.getByRole("tab", {
          name: tx("MCP 连接", "MCP connections"),
          exact: true,
        });
        await expect(models).toHaveAttribute("aria-selected", "true");
        await expect(
          page.getByRole("button", {
            name: tx("添加模型", "Add model"),
            exact: true,
          }),
        ).toBeVisible();
        await models.focus();
        await models.press("ArrowRight");
        await expect(mcp).toBeFocused();
        await mcp.press("Enter");
        await expect(mcp).toHaveAttribute("aria-selected", "true");
        await expect(page).toHaveURL(/tab=mcp/);
        await expect(aiops).toHaveClass(/active/);
        await page.reload();
        await expect(mcp).toHaveAttribute("aria-selected", "true");
        await models.click();
        await expect(page).toHaveURL(/tab=models/);
        await page.goBack();
        await expect(mcp).toHaveAttribute("aria-selected", "true");
        await page.goForward();
        await expect(models).toHaveAttribute("aria-selected", "true");
        const accessibility = await new AxeBuilder({ page }).analyze();
        expect(
          accessibility.violations.filter((item) =>
            ["serious", "critical"].includes(item.impact ?? ""),
          ),
        ).toEqual([]);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth,
          ),
        ).toBe(false);
        await page.screenshot({
          path: info.outputPath("aiops-models.png"),
          animations: "disabled",
        });
        await mcp.click();
        const mcpAccessibility = await new AxeBuilder({ page }).analyze();
        expect(
          mcpAccessibility.violations.filter((item) =>
            ["serious", "critical"].includes(item.impact ?? ""),
          ),
        ).toEqual([]);
        await page.screenshot({
          path: info.outputPath("aiops-mcp.png"),
          animations: "disabled",
        });
        await page
          .getByRole("button", { name: tx("收起导航", "Collapse navigation") })
          .click();
        await expect(aiops).toBeVisible();
        await expect(
          nav.getByRole("link", { name: tx("查询", "Queries"), exact: true }),
        ).toBeVisible();
        expect(errors).toEqual([]);
      });
    }

test("former settings URLs open the corresponding AIOps tab", async ({
  page,
}) => {
  await login(page);
  for (const [path, tab, title] of [
    ["ai", "models", "模型设置"],
    ["mcp", "mcp", "MCP 连接"],
  ]) {
    await page.goto(`/settings/${path}`);
    await expect(page).toHaveURL(new RegExp(`/settings/aiops\\?tab=${tab}$`));
    await expect(
      page.getByRole("tab", { name: title, exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(
      "AIOps 设置",
    );
  }
});

for (const access of ["models", "quota", "usage", "mcp", "none"] as const) {
  test(`AIOps permission boundary: ${access}`, async ({ page }) => {
    await login(page);
    const permissions = {
      models: ["model.manage"],
      quota: ["model.quota.manage", "model.usage.read"],
      usage: ["model.usage.read"],
      mcp: ["mcp_connection.manage"],
      none: [],
    }[access];
    await page.evaluate((permissions) => {
      const key = "argus-mock:db-v14";
      const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
      for (const role of db.roles) role.permissions = permissions;
      localStorage.setItem(key, JSON.stringify(db));
    }, permissions);
    await page.goto(
      `/settings/aiops?tab=${access === "mcp" ? "models" : "mcp"}`,
    );
    const nav = page.getByRole("navigation", { name: "主导航" });
    if (access === "none") {
      await expect(nav.getByRole("link", { name: "AIOps 设置" })).toHaveCount(
        0,
      );
      await expect(
        page.getByText("无 AIOps 设置访问权限", { exact: true }),
      ).toBeVisible();
      await expect(page.getByRole("tab")).toHaveCount(0);
      return;
    }
    await expect(nav.getByRole("link", { name: "AIOps 设置" })).toBeVisible();
    const onlyMCP = access === "mcp";
    await expect(page).toHaveURL(
      new RegExp(`tab=${onlyMCP ? "mcp" : "models"}$`),
    );
    await expect(
      page.getByRole("tab", {
        name: onlyMCP ? "MCP 连接" : "模型设置",
        exact: true,
      }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(
      page.getByRole("tab", {
        name: onlyMCP ? "模型设置" : "MCP 连接",
        exact: true,
      }),
    ).toHaveCount(0);
    if (access === "quota" || access === "usage") {
      await expect(
        page.getByRole("tab", { name: "治理仪表盘", exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "查看治理仪表盘", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "添加模型", exact: true }),
      ).toHaveCount(0);
    }
  });
}
