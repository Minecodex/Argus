import { expect, test, type Locator, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { platformOrigin } from "./origins";

async function login(page: Page, platform = false) {
  await page.goto(platform ? `${platformOrigin}/login` : "/login");
  await page
    .locator('input[autocomplete="username"]')
    .fill(platform ? "admin" : "root");
  await page.locator('input[autocomplete="current-password"]').fill("123456");
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

async function footerStyle(link: Locator) {
  return link.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      height: element.getBoundingClientRect().height,
      padding: style.padding,
      gap: style.gap,
      fontSize: style.fontSize,
      color: style.color,
    };
  });
}

async function expectBottomLink(link: Locator, page: Page) {
  await expect(link).toBeVisible();
  const box = await link.boundingBox();
  const viewport = page.viewportSize()!;
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height);
  expect(viewport.height - box!.y - box!.height).toBeLessThanOrEqual(16);
  expect(box!.x).toBeLessThan(32);
}

for (const locale of ["zh-CN", "en-US"] as const) {
  for (const theme of ["light", "dark"] as const) {
    test.describe(`${locale} ${theme}`, () => {
      const enter = locale === "zh-CN" ? "进入管理后台" : "Open admin console";
      const back = locale === "zh-CN" ? "返回智能会话" : "Back to chat";
      const collapse = locale === "zh-CN" ? "收起导航" : "Collapse navigation";
      const expand = locale === "zh-CN" ? "展开导航" : "Expand navigation";

      test.beforeEach(async ({ page }) => {
        await page.addInitScript(
          ({ locale, theme }) => {
            localStorage.setItem("argus.locale", locale);
            localStorage.setItem("argus.theme", theme);
          },
          { locale, theme },
        );
      });

      for (const viewport of [
        { width: 1280, height: 720 },
        { width: 1920, height: 1080 },
      ]) {
        test(`sidebar round trip and top toggle ${viewport.width}`, async ({
          page,
        }, testInfo) => {
          await page.setViewportSize(viewport);
          await login(page);
          const footer = page.locator(".argus-sidebar__footer");
          const enterLink = footer.getByRole("link", {
            name: enter,
            exact: true,
          });
          await expectBottomLink(enterLink, page);
          const chatStyle = await footerStyle(enterLink);
          await enterLink.click();
          await expect(page).toHaveURL(/\/hosts$/);
          await page.mouse.move(viewport.width / 2, 200);

          const sidebar = page.locator(".argus-sidebar");
          const head = sidebar.locator(".argus-sidebar__head");
          const backLink = footer.getByRole("link", {
            name: back,
            exact: true,
          });
          const collapseButton = head.getByRole("button", {
            name: collapse,
            exact: true,
          });
          const expandButton = head.getByRole("button", {
            name: expand,
            exact: true,
          });
          await expectBottomLink(backLink, page);
          await expect.poll(() => footerStyle(backLink)).toEqual(chatStyle);
          await expect(
            page.locator(".argus-topbar").getByRole("link", { name: back }),
          ).toHaveCount(0);
          await expect(footer.getByRole("button")).toHaveCount(0);
          await expect(collapseButton).toHaveAttribute("aria-expanded", "true");
          expect((await collapseButton.boundingBox())!.y).toBeLessThan(24);
          await expect(page.locator("html")).toHaveAttribute("lang", locale);
          await expect(page.locator("html")).toHaveAttribute(
            "data-theme",
            theme,
          );
          const expandedWidth = (await sidebar.boundingBox())!.width;
          await page.screenshot({ path: testInfo.outputPath("expanded.png") });

          await collapseButton.click();
          await expect(sidebar).toHaveClass(/is-collapsed/);
          await expect(expandButton).toHaveAttribute("aria-expanded", "false");
          expect((await sidebar.boundingBox())!.width).toBeLessThan(
            expandedWidth,
          );
          const compactHead = await head.boundingBox();
          const compactToggle = await expandButton.boundingBox();
          expect(compactToggle!.x).toBeGreaterThanOrEqual(compactHead!.x);
          expect(compactToggle!.x + compactToggle!.width).toBeLessThanOrEqual(
            compactHead!.x + compactHead!.width,
          );
          await expect(backLink.locator("span")).toBeHidden();
          await expectBottomLink(backLink, page);
          await backLink.hover();
          await expect(page.getByRole("tooltip")).toContainText(back);
          await page.mouse.move(viewport.width / 2, 200);
          await page.screenshot({ path: testInfo.outputPath("collapsed.png") });
          const accessibility = await new AxeBuilder({ page })
            .include(".argus-sidebar")
            .analyze();
          expect(
            accessibility.violations.filter((v) =>
              ["serious", "critical"].includes(v.impact ?? ""),
            ),
          ).toEqual([]);

          await expandButton.focus();
          await page.keyboard.press("Enter");
          await expect(sidebar).not.toHaveClass(/is-collapsed/);
          await page.keyboard.press("Control+b");
          await expect(sidebar).toHaveClass(/is-collapsed/);
          await backLink.focus();
          await page.keyboard.press("Enter");
          await expect(page).toHaveURL(/\/$/);
          await expectBottomLink(enterLink, page);
          await enterLink.click();
          await expect(sidebar).toHaveClass(/is-collapsed/);
          await expandButton.click();
          await expect(collapseButton).toBeVisible();
          await sidebar.locator(".argus-sidebar__nav").evaluate((nav) => {
            nav.scrollTop = nav.scrollHeight;
          });
          await expectBottomLink(backLink, page);
          await backLink.click();
          await expect(page).toHaveURL(/\/$/);
        });
      }

      test("shared platform toggle still expands from the sidebar top", async ({
        page,
      }) => {
        await login(page, true);
        const head = page.locator(".argus-sidebar__head");
        await head.getByRole("button", { name: collapse, exact: true }).click();
        const expandButton = head.getByRole("button", {
          name: expand,
          exact: true,
        });
        await expect(expandButton).toHaveAttribute("aria-expanded", "false");
        await expandButton.focus();
        await page.keyboard.press("Enter");
        await expect(
          head.getByRole("button", { name: collapse, exact: true }),
        ).toHaveAttribute("aria-expanded", "true");
      });
    });
  }
}
