import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { openActionMenu } from "./helpers/action-menu";

const real = process.env.ARGUS_KWR_REAL === "1";
test.use({ reducedMotion: "reduce", trace: "off", video: "off" });

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`resource menus and drawer ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(120000);
        page.setDefaultTimeout(20000);
        const text = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1080 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
            Object.defineProperty(navigator, "clipboard", {
              value: {
                writeText: async (value: string) => {
                  (window as any).__copiedResourceLink = value;
                },
              },
              configurable: true,
            });
          },
          { english, theme },
        );
        await page.goto("/login");
        await page
          .locator('input[autocomplete="username"]')
          .fill(real ? "kakj" : "root");
        await page
          .locator('input[autocomplete="current-password"]')
          .fill(real ? process.env.ARGUS_KWR_PASSWORD! : "123456");
        await page.locator('form button[type="submit"]').click();
        await expect(page).not.toHaveURL(/\/login/);
        const cluster = real ? process.env.ARGUS_KWR_CLUSTER! : "k8s-prod-east";
        await page.goto(`/kubernetes/${cluster}?tab=resources&kind=pod`);
        const firstName = page.locator(".argus-k8s-resource-name-link").first();
        await expect(firstName).toBeVisible({ timeout: 45000 });
        const row = page
          .locator("tbody > tr")
          .filter({ has: page.locator(".argus-k8s-resource-name-link") })
          .first();
        await expect(row.locator("td").last().getByRole("button")).toHaveCount(
          1,
        );
        const status = page.locator(".argus-k8s-status-select");
        const filterWidth = (await status.boundingBox())!.width;
        expect(filterWidth).toBeGreaterThanOrEqual(120);
        expect(filterWidth).toBeLessThanOrEqual(240);
        const tableBounds = (await page
          .locator(".argus-k8s-inventory-wrap")
          .boundingBox())!;
        const actionBounds = (await row
          .getByRole("button", { name: /更多操作|More actions/ })
          .boundingBox())!;
        expect(actionBounds.x + actionBounds.width).toBeLessThanOrEqual(
          tableBounds.x + tableBounds.width,
        );
        const headerLineHeight = await row
          .page()
          .locator(".argus-k8s-inventory-table th")
          .evaluateAll((headers) =>
            headers.every(
              (header) => header.getBoundingClientRect().height < 48,
            ),
          );
        expect(headerLineHeight).toBe(true);
        await expect(
          page.getByRole("button", {
            name: text("查看日志", "View logs"),
            exact: true,
          }),
        ).toHaveCount(0);
        let menu = await openActionMenu(row);
        await expect(
          menu.getByRole("menuitem", {
            name: text("查看日志", "View logs"),
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          menu.getByRole("menuitem", {
            name: text("打开终端", "Open terminal"),
            exact: true,
          }),
        ).toBeVisible();
        await expect(page.locator(".argus-dropdown__description")).toHaveCount(
          1,
        );
        await expect(menu).not.toContainText(/待检查|Pending check/);
        await page.screenshot({
          path: info.outputPath("pod-menu.png"),
          animations: "disabled",
        });
        await menu
          .getByRole("menuitem", {
            name: text("复制资源链接", "Copy resource link"),
            exact: true,
          })
          .click();
        await expect
          .poll(() => page.evaluate(() => (window as any).__copiedResourceLink))
          .toContain("resource_uid=");
        const link = await page.evaluate(
          () => (window as any).__copiedResourceLink as string,
        );
        expect(new URL(link).searchParams.get("tab")).toBe("resources");
        expect(new URL(link).searchParams.get("kind")).toBe("pod");
        await page.goto(link);
        const drawer = page.getByRole("dialog");
        await expect(drawer).toBeVisible();
        await expect(drawer.locator(".argus-k8s-detail-actions")).toHaveCount(
          0,
        );
        await expect(
          drawer.getByRole("button", {
            name: text("复制资源链接", "Copy resource link"),
            exact: true,
          }),
        ).toHaveCount(0);
        const initial = (await drawer.boundingBox())!;
        await drawer
          .getByRole("tab", {
            name: text("技术信息", "Technical details"),
            exact: true,
          })
          .click();
        await drawer
          .getByRole("button", {
            name: text("放大详情窗口", "Expand detail window"),
            exact: true,
          })
          .click();
        await expect
          .poll(async () => (await drawer.boundingBox())!.width)
          .toBeGreaterThan(initial.width + 100);
        await expect(
          drawer.getByRole("tab", {
            name: text("技术信息", "Technical details"),
            exact: true,
          }),
        ).toHaveAttribute("aria-selected", "true");
        await expect(page).toHaveURL(/full=true/);
        const wide = (await drawer.boundingBox())!;
        expect(wide.width).toBeLessThanOrEqual(width - 32);
        await page.screenshot({
          path: info.outputPath("expanded-detail.png"),
          animations: "disabled",
        });
        const axe = await new AxeBuilder({ page }).analyze();
        expect(
          axe.violations.filter(
            (v) => v.impact === "critical" || v.impact === "serious",
          ),
        ).toEqual([]);
        await drawer
          .getByRole("button", {
            name: text("恢复抽屉宽度", "Restore drawer width"),
            exact: true,
          })
          .click();
        await expect
          .poll(async () => (await drawer.boundingBox())!.width)
          .toBeCloseTo(initial.width, 0);
        await drawer
          .getByRole("button", {
            name: text("放大详情窗口", "Expand detail window"),
            exact: true,
          })
          .click();
        await expect
          .poll(async () => (await drawer.boundingBox())!.width)
          .toBeCloseTo(wide.width, 0);
        await page.mouse.click(16, 300);
        await expect(drawer).toHaveCount(0);
        await expect(page).not.toHaveURL(/resource_uid=/);
        await expect(firstName).toBeVisible();
        await page.screenshot({
          path: info.outputPath("resource-list.png"),
          animations: "disabled",
        });

        // The same row renderer must also cover controllers, network, config,
        // storage and cluster-scoped/read-only resources.
        if (!real && width === 1440 && !english && theme === "light") {
          for (const kind of [
            "deployment",
            "service",
            "configmap",
            "pvc",
            "node",
            "crd",
          ]) {
            await page.goto(
              `/kubernetes/${cluster}?tab=resources&kind=${kind}`,
            );
            const name = page.locator(".argus-k8s-resource-name-link").first();
            await expect(name).toBeVisible();
            const resourceRow = page
              .locator("tbody > tr")
              .filter({ has: page.locator(".argus-k8s-resource-name-link") })
              .first();
            await expect(
              resourceRow.locator("td").last().getByRole("button"),
            ).toHaveCount(1);
            menu = await openActionMenu(resourceRow);
            await expect(
              menu.getByRole("menuitem", { name: "复制资源链接", exact: true }),
            ).toBeVisible();
            await page.keyboard.press("Escape");
          }
        }
        expect(errors).toEqual([]);
      });
    }

test("shared compact alert drawer expands and dismisses outside", async ({
  page,
}, info) => {
  test.skip(real, "Uses seeded alert fixtures");
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/alerts");
  await page
    .getByRole("button", { name: "CPU 使用率持续过高", exact: true })
    .first()
    .click();
  const drawer = page.getByRole("dialog");
  await expect(drawer).toBeVisible();
  const initial = (await drawer.boundingBox())!;
  await drawer
    .getByRole("button", { name: "放大详情窗口", exact: true })
    .click();
  await expect
    .poll(async () => (await drawer.boundingBox())!.width)
    .toBeGreaterThan(initial.width + 100);
  const expand = (await drawer
    .getByRole("button", { name: "恢复抽屉宽度", exact: true })
    .boundingBox())!;
  const close = (await drawer
    .getByRole("button", { name: "关闭", exact: true })
    .boundingBox())!;
  expect(Math.abs(expand.y - close.y)).toBeLessThan(1);
  expect(close.x).toBeGreaterThan(expand.x + expand.width);
  await page.screenshot({
    path: info.outputPath("shared-alert-drawer.png"),
    animations: "disabled",
  });
  await page.mouse.click(16, 300);
  await expect(drawer).toHaveCount(0);
});
