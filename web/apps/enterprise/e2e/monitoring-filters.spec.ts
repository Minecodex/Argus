import { expect, test, type Page } from "@playwright/test";
import { selectTrigger } from "./helpers/select";
import {
  builtinCollectionsEn,
  builtinCollectionsZh,
} from "../src/i18n/builtin-collections";

async function monitoringFixture(page: Page, english: boolean, theme: string) {
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
  await page.goto("/hosts/host-web-11");
  await page
    .getByRole("tab", {
      name: english ? "Components & Collection" : "组件与采集",
      exact: true,
    })
    .click();
  const builtinNames = (english ? builtinCollectionsEn : builtinCollectionsZh)
    .builtinCollections.names;
  for (const key of [
    "host_metrics",
    "collector_metrics",
    "host_process_metrics",
  ] as const) {
    const row = page
      .getByRole("row")
      .filter({ has: page.getByText(builtinNames[key], { exact: true }) });
    await row.getByRole("switch").press("Space");
    const dialog = page.getByRole("dialog", {
      name: english ? "Preview own resource monitoring" : "自身监控配置预览",
      exact: true,
    });
    await dialog.getByRole("button", { name: /^(确认|Confirm)/ }).click();
    await expect(dialog).not.toBeVisible();
  }
  // This is an isolated mock browser context: seed observation states only.
  // Real filtering/pagination and authorization are covered by PostgreSQL tests.
  await page.evaluate(() => {
    const db = JSON.parse(localStorage.getItem("argus-mock:db-v14")!);
    for (const state of Object.values(db.monitoring) as Array<{
      collections?: Array<Record<string, unknown>>;
    }>) {
      for (const item of state.collections ?? []) {
        if (item.integration === "host_metrics")
          Object.assign(item, {
            desired_enabled: false,
            apply_status: "effective",
            health: "healthy",
            freshness: "fresh",
          });
        if (item.integration === "collector_metrics")
          Object.assign(item, {
            apply_status: "failed",
            health: "failed",
            freshness: "stale",
          });
        if (item.integration === "host_process_metrics")
          Object.assign(item, {
            apply_status: "pending",
            health: "unknown",
            freshness: "no_data",
          });
      }
    }
    localStorage.setItem("argus-mock:db-v14", JSON.stringify(db));
  });
  await page.goto("/monitoring");
  await page.reload();
  await expect(page.locator("tbody tr")).toHaveCount(3);
}

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`monitoring filters ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        await page.setViewportSize({ width, height: 1000 });
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await monitoringFixture(page, english, theme);
        const tx = (zh: string, en: string) => (english ? en : zh);
        const search = page
          .getByRole("searchbox", {
            name: tx("搜索监控名称", "Search monitor name"),
          })
          .or(
            page.getByRole("textbox", {
              name: tx("搜索监控名称", "Search monitor name"),
            }),
          );
        const inputBox = (await search.boundingBox())!,
          tableBox = (await page.getByRole("table").boundingBox())!;
        await page.screenshot({
          path: info.outputPath("monitoring-filters.png"),
          animations: "disabled",
        });
        expect(
          tableBox.y - inputBox.y - inputBox.height,
          "search must not touch the table header",
        ).toBeGreaterThanOrEqual(12);
        const select = async (label: string, option: string) => {
          await selectTrigger(page, label).click();
          await page.getByRole("option", { name: option, exact: true }).click();
        };
        await select(tx("是否启用", "Enablement"), tx("未启用", "Disabled"));
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await expect(page.locator("tbody tr")).toContainText("Host metrics");
        await select(
          tx("是否启用", "Enablement"),
          tx("全部启用状态", "All enablement states"),
        );
        await select(tx("类型", "Type"), tx("主机基础指标", "Host metrics"));
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await select(tx("类型", "Type"), tx("全部类型", "All types"));
        await select(
          tx("配置状态", "Configuration state"),
          tx("应用失败", "Failed"),
        );
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await expect(page.locator("tbody tr")).toContainText(
          "Collector metrics",
        );
        await select(
          tx("配置状态", "Configuration state"),
          tx("全部配置状态", "All configuration states"),
        );
        await select(
          tx("采集健康", "Collection health"),
          tx("正常", "Healthy"),
        );
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await select(
          tx("采集健康", "Collection health"),
          tx("全部采集健康", "All collection health"),
        );
        await select(tx("数据状态", "Data freshness"), tx("数据过期", "Stale"));
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await search.fill("Host metrics");
        await expect(page.locator("tbody tr")).toHaveCount(0);
        await expect(
          page.getByText(tx("没有匹配的监控", "No matching monitors"), {
            exact: true,
          }),
        ).toBeVisible();
        await search.fill("");
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await select(
          tx("数据状态", "Data freshness"),
          tx("全部数据状态", "All data states"),
        );
        await expect(page.locator("tbody tr")).toHaveCount(3);
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        expect(errors).toEqual([]);
        await page.screenshot({
          path: info.outputPath("monitoring-filters-interaction.png"),
          animations: "disabled",
        });
      });
    }

test("monitoring builtin dashboard retains actual host resource type", async ({
  page,
}) => {
  await monitoringFixture(page, false, "light");
  await page
    .getByRole("row")
    .filter({ has: page.getByText("Host metrics", { exact: true }) })
    .getByRole("button", { name: "默认仪表盘", exact: true })
    .click();
  await expect
    .poll(() => new URL(page.url()).searchParams.get("resource_type"))
    .toBe("host");
});
