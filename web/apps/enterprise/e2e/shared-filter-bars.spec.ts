import { expect, test, type Locator } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

async function styleSignature(bar: Locator) {
  return bar.evaluate((node) => {
    const style = getComputedStyle(node);
    const search = node.querySelector<HTMLElement>(".argus-search-input")!;
    const searchStyle = getComputedStyle(search);
    return {
      background: style.backgroundColor,
      borderColor: style.borderColor,
      borderWidth: style.borderWidth,
      radius: style.borderRadius,
      padding: style.padding,
      gap: style.gap,
      searchHeight: search.getBoundingClientRect().height,
      searchBackground: searchStyle.backgroundColor,
      searchRadius: searchStyle.borderRadius,
      searchFont: getComputedStyle(search.querySelector("input")!).fontSize,
    };
  });
}

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`shared filters ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        const tx = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.setViewportSize({ width, height: 1000 });
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
        await expect(page).not.toHaveURL(/login/);
        await page.goto("/alerts?tab=history");
        const bar = page.locator(".argus-filter-bar").first();
        const search = bar.getByRole("searchbox");
        const from = bar.getByLabel(tx("开始日期", "From date"), {
          exact: true,
        });
        const to = bar.getByLabel(tx("结束日期", "To date"), { exact: true });
        const exportButton = bar.getByRole("button", {
          name: tx("导出记录", "Export records"),
          exact: true,
        });
        await expect(from).toBeVisible();
        await expect(to).toBeVisible();
        const bounds = await Promise.all([
          bar.locator(".argus-search-input").boundingBox(),
          ...(await bar
            .locator(".argus-select__trigger")
            .all()
            .then((items) => items.map((item) => item.boundingBox()))),
          from.boundingBox(),
          to.boundingBox(),
          exportButton.boundingBox(),
        ]);
        const controls = bounds.map((box) => box!);
        expect(controls).toHaveLength(6);
        expect(
          Math.max(...controls.map((b) => b.y)) -
            Math.min(...controls.map((b) => b.y)),
        ).toBeLessThanOrEqual(1);
        expect(
          Math.max(...controls.map((b) => b.height)) -
            Math.min(...controls.map((b) => b.height)),
        ).toBeLessThanOrEqual(1);
        for (let i = 1; i < controls.length; i++)
          expect(controls[i]!.x).toBeGreaterThanOrEqual(
            controls[i - 1]!.x + controls[i - 1]!.width,
          );
        const container = (await bar.boundingBox())!;
        expect(controls.at(-1)!.x + controls.at(-1)!.width).toBeLessThanOrEqual(
          container.x + container.width,
        );
        const expectedStyle = await styleSignature(bar);
        expect(expectedStyle.background).not.toBe("rgba(0, 0, 0, 0)");
        expect(expectedStyle.borderWidth).toBe("1px");
        await from.fill("2026-01-01");
        await to.fill("2026-12-31");
        await search.fill("disk");
        await expect
          .poll(() => new URL(page.url()).searchParams.get("from"))
          .toBe("2026-01-01T00:00:00+08:00");
        await expect
          .poll(() => new URL(page.url()).searchParams.get("to"))
          .toBe("2026-12-31T23:59:59+08:00");
        await expect
          .poll(() => new URL(page.url()).searchParams.get("query"))
          .toBe("disk");
        await search.fill("");
        await from.fill("");
        await to.fill("");
        await expect
          .poll(() => new URL(page.url()).searchParams.has("from"))
          .toBe(false);
        await expect(page.locator(".argus-alert-table").first()).toBeVisible();
        await page
          .getByRole("heading", {
            name: tx("告警中心", "Alert center"),
            exact: true,
          })
          .click();
        await page.screenshot({ path: info.outputPath("alert-history.png") });
        expect(
          (
            await new AxeBuilder({ page })
              .include(".argus-filter-bar")
              .analyze()
          ).violations,
        ).toEqual([]);

        for (const tab of [
          tx("屏蔽与抑制", "Silences"),
          tx("通知配置", "Notifications"),
        ]) {
          await page.getByRole("tab", { name: tab, exact: true }).click();
          await expect(bar.getByRole("searchbox")).toBeVisible();
          expect(await styleSignature(bar)).toEqual(expectedStyle);
        }
        for (const route of ["/applications", "/dashboards", "/monitoring"]) {
          await page.goto(route);
          await expect(bar.getByRole("searchbox")).toBeVisible();
          expect(await styleSignature(bar)).toEqual(expectedStyle);
          const pageBounds = await bar.boundingBox();
          expect(pageBounds!.x + pageBounds!.width).toBeLessThanOrEqual(width);
          await page.screenshot({
            path: info.outputPath(`${route.slice(1)}.png`),
          });
        }
        expect(errors).toEqual([]);
      });
    }
