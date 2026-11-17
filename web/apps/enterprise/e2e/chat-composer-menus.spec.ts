import { expect, test } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";

const real = process.env.ARGUS_COMPOSER_MENUS_REAL === "1";
const baseline = process.env.ARGUS_COMPOSER_MENUS_BASELINE === "1";
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`compact composer menus ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width, height: 1000 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        let mutations = 0;
        page.on("request", (request) => {
          if (
            request.method() !== "GET" &&
            /\/(messages|preflight|mode|access-mode)$/.test(
              new URL(request.url()).pathname,
            )
          )
            mutations++;
        });
        if (real) {
          await plan7Login(
            page,
            "/login",
            "kakj",
            process.env.ARGUS_PLAN9_PASSWORD ?? "",
          );
          const conversation = process.env.ARGUS_COMPOSER_MENUS_CONVERSATION;
          if (conversation)
            await page.goto("/?c=" + encodeURIComponent(conversation));
        } else {
          await page.goto("/login");
          await page.getByLabel(english ? "Username" : "用户名").fill("root");
          await page.getByLabel(english ? "Password" : "密码").fill("123456");
          await page
            .getByRole("button", {
              name: english ? "Sign in" : "登录",
              exact: true,
            })
            .click();
        }
        const composer = page.locator(".argus-chat-composer");
        const input = composer.getByRole("textbox");
        await input.fill("Keep this unsent draft");
        const measurements: Record<string, unknown> = {};
        for (const described of [false, true]) {
          const trigger = described
            ? composer.locator(".argus-chat-access")
            : composer.getByRole("button", {
                name: english ? "Add" : "添加",
                exact: true,
              });
          await trigger.click();
          const menu = page.locator(".argus-composer-actions");
          await expect(menu).toBeVisible();
          await expect(menu).not.toHaveAttribute("data-entering", "true");
          const measurement = await menu.evaluate((el) => {
            const rect = el.getBoundingClientRect();
            const labels = [...el.querySelectorAll('[data-slot="label"]')];
            const descriptions = [
              ...el.querySelectorAll('[data-slot="description"]'),
            ];
            return {
              width: rect.width,
              height: rect.height,
              x: rect.x,
              right: rect.right,
              font: labels.map((x) => parseFloat(getComputedStyle(x).fontSize)),
              descriptions: descriptions.map((x) =>
                parseFloat(getComputedStyle(x).fontSize),
              ),
              icons: [...el.querySelectorAll("svg")].map(
                (x) => x.getBoundingClientRect().width,
              ),
              overflow: el.scrollWidth > el.clientWidth,
            };
          });
          measurements[described ? "permissions" : "add"] = measurement;
          if (!baseline) {
            expect(measurement.width).toBeLessThanOrEqual(
              described ? 322 : 262,
            );
            expect(measurement.width).toBeGreaterThanOrEqual(
              described ? 310 : 250,
            );
            expect(measurement.height).toBeLessThanOrEqual(
              described ? 210 : 220,
            );
            expect(measurement.font.length).toBeGreaterThan(0);
            expect(measurement.font.every((size) => size <= 13)).toBe(true);
            expect(measurement.descriptions.every((size) => size <= 12)).toBe(
              true,
            );
            expect(measurement.icons.every((size) => size <= 16)).toBe(true);
            expect(measurement.overflow).toBe(false);
            expect(measurement.x).toBeGreaterThanOrEqual(0);
            expect(measurement.right).toBeLessThanOrEqual(width);
          }
          await expect(menu.getByRole("menuitem")).toHaveCount(
            described ? 2 : 4,
          );
          if (described)
            await expect(menu).toContainText(
              english ? "enterprise approval" : "企业审批",
            );
          await page.screenshot({
            path: info.outputPath(described ? "permissions.png" : "add.png"),
            animations: "disabled",
          });
          if (described) await page.mouse.click(width - 32, 220);
          else {
            await page.keyboard.press("Escape");
            await expect(trigger).toBeFocused();
          }
          await expect(menu).toHaveCount(0);
          await expect(input).toHaveValue("Keep this unsent draft");
        }
        expect(mutations).toBe(0);
        await info.attach("menu-measurements", {
          body: JSON.stringify(measurements, null, 2),
          contentType: "application/json",
        });
      });
    }
