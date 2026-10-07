import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { platformOrigin } from "./origins";
test.use({ actionTimeout: 15000 });
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
async function a11y(page: Page) {
  const result = await new AxeBuilder({ page }).analyze();
  expect(
    result.violations.filter((v) => ["serious", "critical"].includes(v.impact)),
  ).toEqual([]);
}
async function createPanel(page: Page, english = false) {
  const tx = (zh: string, en: string) => (english ? en : zh);
  await page.goto("/dashboards");
  await page
    .getByRole("button", {
      name: tx("新建仪表盘", "Create dashboard"),
      exact: true,
    })
    .click();
  await page
    .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
    .fill("Redesign acceptance");
  await page
    .getByRole("button", { name: tx("完成", "Done"), exact: true })
    .click();
  await page
    .getByRole("button", { name: tx("添加统计图", "Add panel"), exact: true })
    .first()
    .click();
  await page
    .getByRole("button", {
      name: tx("CPU 使用率", "CPU utilization"),
      exact: true,
    })
    .click();
  await expect(page).toHaveURL(/\/panels\//);
  await expect(
    page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
  ).toBeVisible();
}
for (const viewport of [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
])
  test.describe(`redesign matrix ${viewport.width}x${viewport.height}`, () => {
    test.use({ viewport });
    test.setTimeout(180000);
    for (const english of [false, true])
      for (const theme of ["light", "dark"]) {
        test(`redesign editor ${english ? "en" : "zh"} ${theme}: query, style, restore and publish`, async ({
          page,
        }, info) => {
          const tx = (zh: string, en: string) => (english ? en : zh);
          await login(page, english, theme);
          await createPanel(page, english);
          await page
            .getByRole("button", {
              name: tx("运行查询", "Run query"),
              exact: true,
            })
            .click();
          await expect(
            page.getByText(tx("最近运行结果", "Latest query result"), {
              exact: true,
            }),
          ).toBeVisible();
        await page.screenshot({ path: info.outputPath("chart-recommendations.png"), animations: "disabled" });
          await page
            .getByRole("tab", { name: tx("样式", "Style"), exact: true })
            .click();
          await page.getByLabel(tx("小数位数", "Decimal places")).fill("2");
          await expect(
            page.getByText(tx("最近运行结果", "Latest query result"), {
              exact: true,
            }),
          ).toBeVisible();
          await expect(
            page.getByText(tx("草稿已保存", "Draft saved"), { exact: false }),
          ).toBeVisible();
          await page.screenshot({ path: info.outputPath("editor.png") });
          await a11y(page);
          const runBox = page.getByRole("button", {
            name: tx("运行查询", "Run query"),
            exact: true,
          });
          expect(
            await runBox.evaluate((e) => e.getBoundingClientRect().height),
          ).toBe(32);
          await page.reload();
          await page
            .getByRole("tab", { name: tx("样式", "Style"), exact: true })
            .click();
          await expect(
            page.getByLabel(tx("小数位数", "Decimal places")),
          ).toHaveValue("2");
          await page
            .getByRole("button", { name: tx("完成", "Done"), exact: true })
            .click();
          await expect(page).toHaveURL(/\/dashboard-drafts\/[^/]+$/);
          await page
            .getByRole("button", {
              name: tx("预览并发布", "Preview and publish"),
            })
            .click();
          await page
            .getByRole("dialog")
            .getByRole("button", {
              name: tx("确认发布", "Confirm publication"),
              exact: true,
            })
            .click();
          await expect(page).toHaveURL(/\/dashboards\//);
          await expect(
            page.getByRole("button", {
              name: tx("编辑统计图", "Edit panel"),
              exact: true,
            }),
          ).toHaveCount(0);
          await page.goto("/dashboards");
          await expect(
            page.locator(".argus-resource-card").first(),
          ).toBeVisible();
          await page.screenshot({
            path: info.outputPath("dashboard-directory.png"),
            animations: "disabled",
          });
        });
        test(`redesign routes ${english ? "en" : "zh"} ${theme}: shared cards, controls and menu`, async ({
          page,
        }, info) => {
          await login(page, english, theme);
          for (const route of [
            "/hosts",
            "/kubernetes",
            "/dashboards",
            "/tasks",
            "/approvals",
            "/remote-sessions",
            "/hosts/host-web-11",
            "/kubernetes/k8s-prod-east",
            "/settings/org",
            "/settings/ai",
            "/settings/mcp",
            "/settings/secrets",
            "/settings/audit",
            "/account",
          ]) {
            await page.goto(route);
            await expect(
              page.locator(".argus-page__title").first(),
            ).toBeVisible();
            if (["/hosts", "/kubernetes"].includes(route)) {
              await expect(
                page.locator(".argus-resource-card").first(),
              ).toBeVisible();
            }
            await a11y(page);
            expect(
              await page.evaluate(
                () => document.documentElement.scrollWidth > innerWidth,
              ),
            ).toBe(false);
            await page.screenshot({
              path: info.outputPath(
                route.slice(1).replaceAll("/", "-") + ".png",
              ),
            });
          }
          await page.goto("/hosts");
          const control = page
            .locator(".argus-filter-bar .argus-select__trigger")
            .first();
          await control.click();
          await expect(page.getByRole("listbox")).toBeVisible();
          const style = await page
            .getByRole("option")
            .first()
            .evaluate((element) => {
              const s = getComputedStyle(element);
              return {
                height: parseFloat(s.minHeight),
                color: s.color,
              };
            });
          expect(style.height).toBeGreaterThanOrEqual(32);
          await page.keyboard.press("Escape");
          await expect(control).toBeFocused();
        });
      }
    for (const [width, height] of [
      [1280, 800],
      [1440, 900],
      [1920, 1080],
    ])
      test(`redesign desktop ${width}: card grid and equal control heights`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height });
        await login(page);
        await page.goto("/kubernetes");
        await expect(
          page.locator(".argus-resource-card").first(),
        ).toBeVisible();
        const sizes = await page
          .locator(
            ".argus-page__actions .argus-button,.argus-page__content .argus-filter-bar .argus-select__trigger",
          )
          .evaluateAll((elements) =>
            elements.map((e) => Math.round(e.getBoundingClientRect().height)),
          );
        expect(sizes.length).toBeGreaterThan(0);
        expect(new Set(sizes)).toEqual(new Set([32]));
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth > innerWidth,
          ),
        ).toBe(false);
      });
    for (const english of [false, true])
      for (const theme of ["light", "dark"]) {
        test(`redesign shared controls ${english ? "en" : "zh"} ${theme}: sizes, selected and disabled options`, async ({
          page,
        }, info) => {
          await login(page, english, theme);
          await page.goto("/demo");
          const gallery = page.locator(".argus-control-showcase");
          await gallery.scrollIntoViewIfNeeded();
          for (const [size, height] of [
            ["sm", 28],
            ["md", 32],
            ["lg", 36],
          ] as const) {
            const row = gallery.locator(`[data-control-size="${size}"]`);
            const heights = await row
              .locator(".argus-button,.argus-input,.argus-select__trigger")
              .evaluateAll((elements) =>
                elements.map((e) =>
                  Math.round(e.getBoundingClientRect().height),
                ),
              );
            expect(heights).toHaveLength(4);
            expect(new Set(heights)).toEqual(new Set([height]));
          }
          const select = gallery.locator(
            '[data-control-size="md"] .argus-select__trigger',
          );
          await select.click();
          await expect(
            page.getByRole("option", {
              name: english ? "Retired environment" : "已停用的环境",
              exact: true,
            }),
          ).toHaveAttribute("aria-disabled", "true");
          await page.keyboard.press("Escape");
          await expect(select).toBeFocused();
          await a11y(page);
          await gallery.screenshot({
            path: info.outputPath("shared-controls.png"),
            animations: "disabled",
          });
        });
      }
    for (const english of [false, true])
      for (const theme of ["light", "dark"]) {
        test(`redesign platform routes ${english ? "en" : "zh"} ${theme}: complete desktop route matrix`, async ({
          page,
        }, info) => {
          await page.addInitScript(
            ({ english, theme }) => {
              localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
              localStorage.setItem("argus.theme", theme);
            },
            { english, theme },
          );
          await page.goto(`${platformOrigin}/login?initialized=true&reset=1`);
          await expect(
            page.getByLabel(english ? "Username" : "用户名"),
          ).toBeVisible();
          await a11y(page);
          await page.screenshot({
            path: info.outputPath("platform-login.png"),
          });
          await page.getByLabel(english ? "Username" : "用户名").fill("admin");
          await page.getByLabel(english ? "Password" : "密码").fill("123456");
          await page
            .getByRole("button", {
              name: english ? "Sign in" : "登录",
              exact: true,
            })
            .click();
          await expect(page).not.toHaveURL(/login/);
          for (const route of [
            "/",
            "/enterprises",
            "/admins",
            "/sandbox",
            "/audit",
            "/pki",
            "/account",
          ]) {
            await page.goto(`${platformOrigin}${route}`);
            await expect(
              page.locator(".argus-page__title").first(),
            ).toBeVisible();
            if (route === "/enterprises")
              await expect(
                page.locator(".argus-resource-card").first(),
              ).toBeVisible();
            await a11y(page);
            expect(
              await page.evaluate(
                () => document.documentElement.scrollWidth > innerWidth,
              ),
            ).toBe(false);
            await page.screenshot({
              path: info.outputPath(
                route === "/" ? "overview.png" : `${route.slice(1)}.png`,
              ),
              animations: "disabled",
            });
          }
        });
      }
    test("redesign platform: entity cards and desktop sidebar collapse", async ({
      page,
    }) => {
      await page.addInitScript(() =>
        localStorage.setItem("argus.locale", "zh-CN"),
      );
      await page.goto(`${platformOrigin}/login?initialized=true&reset=1`);
      await page.getByLabel("用户名").fill("admin");
      await page.getByLabel("密码").fill("123456");
      await page.getByRole("button", { name: "登录", exact: true }).click();
      await expect(page).not.toHaveURL(/login/);
      await page.goto(`${platformOrigin}/enterprises`);
      await expect(page.locator(".argus-resource-card").first()).toBeVisible();
      await a11y(page);
      await page.getByRole("button", { name: "收起导航", exact: true }).click();
      expect(
        await page
          .locator(".argus-sidebar")
          .evaluate((e) => e.getBoundingClientRect().width),
      ).toBe(64);
      await a11y(page);
      await page.getByRole("button", { name: "展开导航", exact: true }).click();
      expect(
        await page
          .locator(".argus-sidebar")
          .evaluate((e) => e.getBoundingClientRect().width),
      ).toBe(224);
    });
    test("redesign desktop zoom keeps the editor actions usable", async ({
      page,
    }, info) => {
      await login(page);
      await createPanel(page);
      await page
        .getByLabel("统计图标题")
        .fill("VeryLongServiceIdentity".repeat(10));
      await page.getByRole("button", { name: /^收起导航/ }).click();
      // 720 x 450 CSS pixels represents a 1440 x 900 desktop at 200% zoom.
      await page.setViewportSize({ width: 720, height: 450 });
      const run = page.getByRole("button", { name: "运行查询", exact: true });
      await run.click();
      await expect(
        page.getByText("最近运行结果", { exact: true }),
      ).toBeVisible();
      await a11y(page);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
      ).toBe(false);
      await page.screenshot({
        path: info.outputPath("desktop-zoom.png"),
        animations: "disabled",
      });
      await page.getByRole("button", { name: "完成", exact: true }).click();
      await expect(page).toHaveURL(/\/dashboard-drafts\/[^/]+$/);
    });
    for (const english of [false, true])
      for (const theme of ["light", "dark"]) {
        test(`redesign initialization ${english ? "en" : "zh"} ${theme}: token and preflight states`, async ({
          page,
        }, info) => {
          await page.addInitScript(
            ({ english, theme }) => {
              localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
              localStorage.setItem("argus.theme", theme);
            },
            { english, theme },
          );
          await page.goto(
            `${platformOrigin}/login?initialized=false&reset=1#argus_setup_token=setup-token-e2e`,
          );
          await expect(
            page.locator('input[name="admin.username"]'),
          ).toBeVisible();
          await a11y(page);
          await page.screenshot({
            path: info.outputPath("initialization.png"),
            animations: "disabled",
          });
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth > innerWidth,
            ),
          ).toBe(false);
        });
      }
  });
