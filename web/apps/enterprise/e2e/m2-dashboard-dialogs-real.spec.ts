import { test, expect } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import AxeBuilder from "@axe-core/playwright";
import { dashboardsZh, dashboardsEn } from "../src/i18n/dashboards";
import {
  dashboardControlsZh,
  dashboardControlsEn,
} from "../src/i18n/dashboard-controls";
import { selectTrigger } from "./helpers/select";
const login = createMfaLogin("enterprise"),
  enabled = process.env.ARGUS_M2_E2E === "1";
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test.describe(`M2 real dashboard dialogs ${english ? "en" : "zh"} ${theme}`, () => {
      test.skip(!enabled, "M2 temporary Kubernetes environment is required");
      test.use({
        viewport: { width: 1440, height: 900 },
        actionTimeout: 15000,
      });
      test.setTimeout(90000);
      test("compact form, aligned selection, close and complete panel library", async ({
        page,
      }, info) => {
        const tx = (zh: string, en: string) => (english ? en : zh);
        const d = (english ? dashboardsEn : dashboardsZh).dashboards;
        const c = (english ? dashboardControlsEn : dashboardControlsZh)
          .dashboardControls;
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        await login(
          page,
          "/login",
          process.env.ARGUS_M2_ENTERPRISE_USERNAME!,
          process.env.ARGUS_M2_ENTERPRISE_PASSWORD!,
        );
        await page.goto("/dashboards");
        await page
          .getByRole("button", {
            name: tx("新建仪表盘", "Create dashboard"),
            exact: true,
          })
          .click();
        const dialog = page.getByRole("dialog", {
          name: tx("新建仪表盘", "Create dashboard"),
          exact: true,
        });
        await expect(dialog).toBeVisible();
        await page.evaluate(async () => {
          await Promise.all(
            document
              .getAnimations()
              .filter((a) =>
                Number.isFinite(a.effect?.getComputedTiming().endTime),
              )
              .map((a) => a.finished.catch(() => undefined)),
          );
        });
        const rect = (await dialog.boundingBox())!,
          close = (await dialog
            .getByRole("button", { name: tx("关闭", "Close"), exact: true })
            .boundingBox())!;
        expect(rect.height).toBeLessThan(520);
        expect(Math.abs(rect.x + rect.width / 2 - 720)).toBeLessThan(1);
        expect(Math.abs(close.y - rect.y - 16)).toBeLessThan(2);
        expect(
          Math.abs(rect.x + rect.width - close.x - close.width - 16),
        ).toBeLessThan(2);
        const select = dialog.locator(".argus-select__trigger");
        expect(
          await select.evaluate((el) => {
            const r = el.getBoundingClientRect(),
              v = el
                .querySelector(".argus-select__value")!
                .getBoundingClientRect();
            return Math.abs(r.y + r.height / 2 - v.y - v.height / 2);
          }),
        ).toBeLessThan(1);
        await page.screenshot({
          path: info.outputPath("create-real.png"),
          animations: "disabled",
        });
        await dialog
          .getByRole("textbox", { name: tx("名称", "Name"), exact: true })
          .fill(`UI scenario ${english ? "en" : "zh"} ${theme}`);
        const created = page.waitForResponse(
          (r) =>
            r.request().method() === "POST" &&
            new URL(r.url()).pathname.endsWith("/dashboard-drafts") &&
            r.status() === 201,
          { timeout: 30000 },
        );
        await dialog
          .getByRole("button", { name: tx("完成", "Done"), exact: true })
          .click();
        await created;
        await expect(page).toHaveURL(/\/dashboard-drafts\//);
        await page
          .getByRole("button", {
            name: tx("添加统计图", "Add panel"),
            exact: true,
          })
          .first()
          .click();
        const library = page.getByRole("dialog", {
          name: tx("添加统计图", "Add panel"),
          exact: true,
        });
        await expect(library.locator(".argus-panel-preset-card")).toHaveCount(
          21,
        );
        await page.screenshot({
          path: info.outputPath("scenarios-real.png"),
          animations: "disabled",
        });
        await library
          .getByRole("tab", {
            name: tx("全部图型", "All chart types"),
            exact: true,
          })
          .click();
        await expect(library.locator(".argus-panel-preset-card")).toHaveCount(
          11,
        );
        await page.evaluate(async () => {
          await Promise.all(
            document
              .getAnimations()
              .filter((a) =>
                Number.isFinite(a.effect?.getComputedTiming().endTime),
              )
              .map((a) => a.finished.catch(() => undefined)),
          );
        });
        const result = await new AxeBuilder({ page }).analyze();
        expect(
          result.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact),
          ),
        ).toEqual([]);
        await library
          .getByRole("button", { name: tx("关闭", "Close"), exact: true })
          .click();
        await expect(library).not.toBeVisible();
        await expect(page).toHaveURL(/\/dashboard-drafts\/[^/]+$/);
        await page
          .getByRole("button", { name: c.settings, exact: true })
          .click();
        const settings = page.getByRole("dialog", {
          name: c.settings,
          exact: true,
        });
        await settings
          .getByRole("button", { name: d.defaultTime, exact: true })
          .click();
        const timeDefaults = page.getByRole("dialog", {
          name: d.defaultTime,
          exact: true,
        });
        await selectTrigger(timeDefaults, d.timePresets).click();
        await page
          .getByRole("option", { name: d.sixHours, exact: true })
          .click();
        await timeDefaults
          .getByRole("button", { name: d.apply, exact: true })
          .click();
        await settings.getByRole("spinbutton").fill("10");
        await settings
          .getByRole("button", { name: d.done, exact: true })
          .click();
        await page
          .getByRole("button", { name: c.filterManager, exact: true })
          .click();
        const variables = page.getByRole("dialog", {
          name: c.filterManager,
          exact: true,
        });
        await expect(variables.getByRole("spinbutton")).toHaveCount(0);
        await variables
          .getByRole("button", { name: d.addVariable, exact: true })
          .click();
        await variables
          .getByRole("textbox", { name: d.field, exact: true })
          .fill("resource_attributes.deployment.environment.name");
        const candidates = page.waitForResponse(
          (r) =>
            r.request().method() === "POST" &&
            new URL(r.url()).pathname.endsWith("/dashboards/catalog/query") &&
            r.status() === 200,
        );
        await variables
          .getByRole("button", { name: c.previewCandidates, exact: true })
          .click();
        const lookup = await candidates;
        const requested = lookup.request().postDataJSON();
        expect(Date.parse(requested.to) - Date.parse(requested.from)).toBe(
          21600000,
        );
        await page.screenshot({
          path: info.outputPath("variables-real.png"),
          animations: "disabled",
        });
        const savedVariable = page.waitForResponse(
          (r) =>
            r.request().method() === "PATCH" &&
            /\/dashboard-drafts\/[^/]+$/.test(new URL(r.url()).pathname) &&
            r.status() === 200,
        );
        await variables
          .getByRole("button", { name: d.done, exact: true })
          .click();
        await savedVariable;
        await page.reload();
        await page
          .getByRole("button", { name: c.filterManager, exact: true })
          .click();
        await expect(
          variables.getByRole("textbox", { name: d.field, exact: true }),
        ).toHaveValue("resource_attributes.deployment.environment.name");
        await variables
          .getByRole("button", { name: c.removeVariable, exact: true })
          .click();
        await variables
          .getByRole("button", { name: d.done, exact: true })
          .click();
        await page
          .getByRole("button", { name: d.preview, exact: true })
          .click();
        const executed = page.waitForResponse(
          (r) =>
            r.request().method() === "POST" &&
            new URL(r.url()).pathname.endsWith("/execute") &&
            r.status() === 200,
        );
        await page
          .getByRole("dialog")
          .getByRole("button", { name: d.publish, exact: true })
          .click();
        const executionResponse = await executed;
        const execution = await executionResponse.json();
        expect(Date.parse(execution.to) - Date.parse(execution.from)).toBe(
          21600000,
        );
        const controls = page.getByRole("group", {
          name: c.viewConditions,
          exact: true,
        });
        const range = controls.getByRole("button", {
          name: d.time,
          exact: true,
        });
        const resources = controls.getByRole("button", {
          name: d.resources,
          exact: true,
        });
        const rangeBox = (await range.boundingBox())!,
          resourcesBox = (await resources.boundingBox())!;
        expect(
          resourcesBox.x - rangeBox.x - rangeBox.width,
        ).toBeGreaterThanOrEqual(7);
        expect(
          resourcesBox.x - rangeBox.x - rangeBox.width,
        ).toBeLessThanOrEqual(9);
        await selectTrigger(controls, c.refreshInterval).click();
        await page
          .getByRole("option", { name: c.refreshOff, exact: true })
          .click();
        await resources.click();
        const resourcePopup = page.getByRole("dialog", {
          name: d.chooseResources,
          exact: true,
        });
        await expect(resourcePopup).toContainText(c.resourceHint);
        await resourcePopup.press("Escape");
        await expect(resources).toBeFocused();
        await range.click();
        const picker = page.getByRole("dialog", { name: d.time, exact: true });
        await selectTrigger(picker, d.timePresets).click();
        await page.getByRole("option", { name: d.day, exact: true }).click();
        const changedExecution = page.waitForResponse(
          (r) =>
            r.request().method() === "POST" &&
            new URL(r.url()).pathname.endsWith("/execute") &&
            r.status() === 200,
        );
        await picker
          .getByRole("button", { name: d.apply, exact: true })
          .click();
        const changed = await (await changedExecution).json();
        expect(Date.parse(changed.to) - Date.parse(changed.from)).toBe(
          86400000,
        );
        const detail = await page.evaluate(
          async (url) => (await fetch(url, { credentials: "include" })).json(),
          executionResponse.url().replace(/\/execute$/, ""),
        );
        expect(detail.revision.spec.default_time_range.seconds).toBe(21600);
        expect(detail.revision.spec.default_refresh_seconds).toBe(10);
        await page.screenshot({
          path: info.outputPath("controls-real.png"),
          animations: "disabled",
        });
      });
    });
