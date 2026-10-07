import { test, expect, type Locator, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
async function settled(page: Page) {
  await page.evaluate(async () => {
    await Promise.all(
      document
        .getAnimations()
        .filter((a) => Number.isFinite(a.effect?.getComputedTiming().endTime))
        .map((a) => a.finished.catch(() => undefined)),
    );
  });
}
async function closePosition(dialog: Locator) {
  const close = dialog.getByRole("button", { name: /^(关闭|Close)$/ });
  await expect(close).toBeVisible();
  const box = (await dialog.boundingBox())!,
    button = (await close.boundingBox())!;
  expect(
    Math.abs(box.x + box.width - button.x - button.width - 16),
  ).toBeLessThan(2);
  expect(Math.abs(button.y - box.y - 16)).toBeLessThan(2);
}
async function selectedAlignment(page: Page) {
  const invalid = await page
    .locator(".argus-select__trigger:visible")
    .evaluateAll((elements) =>
      elements.flatMap((element) => {
        const value = element.querySelector(".argus-select__value"),
          text = value?.querySelector('[data-slot="label"]') ?? value;
        if (!value || !text) return [];
        const box = element.getBoundingClientRect(),
          rect = text.getBoundingClientRect();
        return Math.abs(box.y + box.height / 2 - rect.y - rect.height / 2) > 1
          ? [element.textContent]
          : [];
      }),
    );
  expect(invalid).toEqual([]);
}
for (const viewport of [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
])
  test.describe(`dialogs and presets ${viewport.width}`, () => {
    test.use({ viewport, actionTimeout: 15000 });
    test.setTimeout(120000);
    for (const english of [false, true])
      for (const theme of ["light", "dark"])
        test(`modal bounds, close, selected values and scenario search ${english ? "en" : "zh"} ${theme}`, async ({
          page,
        }, info) => {
          const tx = (zh: string, en: string) => (english ? en : zh);
          await page.addInitScript(
            ({ english, theme }) => {
              localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
              localStorage.setItem("argus.theme", theme);
            },
            { english, theme },
          );
          await page.goto("/login");
          await page.getByLabel(tx("用户名", "Username")).fill("root");
          await page.getByLabel(tx("密码", "Password")).fill("123456");
          await page
            .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
            .click();
          await expect(page).not.toHaveURL(/login/);
          await page.goto("/dashboards");
          await page
            .getByRole("button", {
              name: tx("新建仪表盘", "Create dashboard"),
              exact: true,
            })
            .click();
          const create = page.getByRole("dialog", {
            name: tx("新建仪表盘", "Create dashboard"),
            exact: true,
          });
          await expect(create).toBeVisible();
          await settled(page);
          await closePosition(create);
          const bounds = (await create.boundingBox())!;
          expect(bounds.height).toBeLessThan(520);
          expect(
            Math.abs(bounds.x + bounds.width / 2 - viewport.width / 2),
          ).toBeLessThan(2);
          await selectedAlignment(page);
          await page.screenshot({
            path: info.outputPath("create-dashboard.png"),
            animations: "disabled",
          });
          await create
            .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
            .fill("Dialog acceptance");
          await create
            .getByRole("button", { name: tx("完成", "Done"), exact: true })
            .click();
          await expect(page).toHaveURL(/\/dashboard-drafts\//);
          await page
            .getByRole("button", {
              name: tx("添加统计图", "Add panel"),
              exact: true,
            })
            .first()
            .click();
          const picker = page.getByRole("dialog", {
            name: tx("添加统计图", "Add panel"),
            exact: true,
          });
          await expect(picker).toBeVisible();
          await settled(page);
          await closePosition(picker);
          await expect(picker.locator(".argus-panel-preset-card")).toHaveCount(
            21,
          );
          const tiles = picker.locator(".argus-panel-preset-card");
          const first = (await tiles.nth(0).boundingBox())!,
            second = (await tiles.nth(1).boundingBox())!,
            third = (await tiles.nth(2).boundingBox())!;
          expect(first.y).toBe(second.y);
          expect(second.y).toBe(third.y);
          expect(second.x).toBeGreaterThan(first.x + first.width);
          await expect(
            picker.getByRole("button", {
              name: tx("自定义统计图", "Custom panel"),
              exact: true,
            }),
          ).toBeInViewport();
          await page.screenshot({
            path: info.outputPath("scenario-library.png"),
            animations: "disabled",
          });
          const search = picker.getByRole("searchbox", {
            name: tx(
              "搜索场景、指标或图型",
              "Find scenarios, metrics or charts",
            ),
          });
          await search.fill("Pod");
          await expect(picker.locator(".argus-panel-preset-card")).toHaveCount(
            2,
          );
          await search.fill("");
          await picker
            .getByRole("tab", {
              name: tx("全部图型", "All chart types"),
              exact: true,
            })
            .click();
          await expect(picker.locator(".argus-panel-preset-card")).toHaveCount(
            11,
          );
          await page.screenshot({
            path: info.outputPath("all-chart-types.png"),
            animations: "disabled",
          });
          await settled(page);
          const axe = await new AxeBuilder({ page }).analyze();
          expect(
            axe.violations.filter((v) =>
              ["serious", "critical"].includes(v.impact),
            ),
          ).toEqual([]);
          await picker
            .getByRole("button", { name: tx("热力图", "Heatmap"), exact: true })
            .click();
          await expect(page).toHaveURL(/\/panels\//);
          await expect(
            page.getByRole("combobox", {
              name: tx("指标", "Metric"),
              exact: true,
            }),
          ).toHaveValue("");
          await selectedAlignment(page);
          await page.goto("/hosts");
          await expect(
            page.locator(".argus-resource-card").first(),
          ).toBeVisible();
          await selectedAlignment(page);
          await page
            .getByRole("button", {
              name: tx("添加普通主机", "Add Host"),
              exact: true,
            })
            .click();
          await settled(page);
          const drawer = page.getByRole("dialog").last();
          await closePosition(drawer);
          const drawerRect = (await drawer.boundingBox())!;
          expect(
            Math.abs(drawerRect.x + drawerRect.width / 2 - viewport.width / 2),
          ).toBeLessThan(2);
        });
  });
