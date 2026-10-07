import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { dashboardsZh, dashboardsEn } from "../src/i18n/dashboards";
import {
  dashboardLinksZh,
  dashboardLinksEn,
} from "../src/i18n/dashboard-links";
import { createMfaLogin } from "./helpers/mfa-login";
import { setChoice } from "./helpers/choice";
import { dashboardAPI } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
for (const english of [false, true])
  for (const theme of ["light", "dark"])
    test.describe(`PlanV2 resource links ${english ? "en" : "zh"} ${theme}`, () => {
      test.skip(
        process.env.ARGUS_PLANV2_E2E !== "1",
        "Requires temporary PlanV2 Kubernetes environment",
      );
      test.use({
        viewport: { width: 1440, height: 900 },
        actionTimeout: 20000,
        navigationTimeout: 30000,
      });
      test.setTimeout(180000);
      test("real binding, lazy reverse lookup, entry provenance and scope", async ({
        page,
      }, info) => {
        const d = (english ? dashboardsEn : dashboardsZh).dashboards,
          l = (english ? dashboardLinksEn : dashboardLinksZh).dashboardLinks;
        const tx = (zh: string, en: string) => (english ? en : zh);
        const resourceID =
          process.env[
            theme === "light"
              ? "ARGUS_PLANV2_HOST_ID"
              : "ARGUS_PLANV2_CLUSTER_ID"
          ]!;
        const route = theme === "light" ? "hosts" : "kubernetes";
        const boardName = `Resource links ${english ? "en" : "zh"} ${theme}`;
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
          process.env.ARGUS_PLANV2_USERNAME!,
          process.env.ARGUS_PLANV2_PASSWORD!,
        );
        await page.goto("/dashboards");
        await page.getByRole("button", { name: d.create, exact: true }).click();
        await page
          .getByRole("textbox", { name: d.name, exact: true })
          .fill(boardName);
        await page.getByRole("button", { name: d.done, exact: true }).click();
        await page
          .getByRole("button", { name: d.preview, exact: true })
          .click();
        await page
          .getByRole("dialog")
          .getByRole("button", { name: d.publish, exact: true })
          .click();
        await expect(page).toHaveURL(/\/dashboards\//);
        const dashboardURL = page.url(),
          boardID = new URL(dashboardURL).pathname.split("/").pop()!;
        await page.goto(`/${route}/${resourceID}`);
        await page
          .getByRole("tab", { name: l.resourceTab, exact: true })
          .click();
        const region = page.getByRole("region", {
          name: d.resourceLinks,
          exact: true,
        });
        await region
          .getByRole("button", { name: d.manageBindings, exact: true })
          .click();
        const manage = page.getByRole("dialog", {
          name: d.manageBindings,
          exact: true,
        });
        await manage
          .getByRole("button", {
            name: tx(`关联 ${boardName}`, `Link ${boardName}`),
            exact: true,
          })
          .click();
        const review = page.getByRole("dialog", {
          name: d.bindingPreviewTitle,
          exact: true,
        });
        await review
          .getByRole("button", { name: tx("确认执行", "Confirm"), exact: true })
          .click();
        await expect(review).not.toBeVisible();
        await manage
          .getByRole("button", { name: d.close, exact: true })
          .click();
        const executed = page.waitForResponse(
          (r) =>
            r.url().endsWith(`/dashboards/${boardID}/execute`) &&
            r.status() === 200,
        );
        await region
          .locator(".argus-resource-card")
          .filter({ hasText: boardName })
          .getByRole("button", { name: l.openDashboard, exact: true })
          .click();
        const initial = await executed;
        expect(initial.request().postDataJSON().resource_ids).toEqual([
          resourceID,
        ]);
        await expect(
          page.locator(".argus-dashboard-entry-source a"),
        ).toBeVisible();
        const origin = await page
          .locator(".argus-dashboard-entry-source")
          .textContent();
        const path = `/dashboards/${boardID}/bindings`;
        let reverseReads = 0;
        page.on("request", (r) => {
          if (new URL(r.url()).pathname.endsWith(path)) reverseReads++;
        });
        await page.waitForTimeout(300);
        expect(reverseReads).toBe(0);
        await page.getByRole("button", { name: l.more, exact: true }).click();
        const reverse = page.waitForResponse(
          (r) => new URL(r.url()).pathname.endsWith(path) && r.status() === 200,
        );
        await page
          .getByRole("menuitem", { name: l.linkedResources, exact: true })
          .click();
        const bindings = await (await reverse).json();
        expect(bindings).toHaveLength(1);
        expect(bindings[0].resource_id).toBe(resourceID);
        const drawer = page.getByRole("dialog", {
          name: l.linkedResources,
          exact: true,
        });
        await expect(
          drawer.locator(".argus-dashboard-linked-resource"),
        ).toHaveCount(1);
        await page.screenshot({
          path: info.outputPath("reverse-real.png"),
          animations: "disabled",
        });
        const axe = await new AxeBuilder({ page }).analyze();
        expect(
          axe.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact),
          ),
        ).toEqual([]);
        await drawer.press("Escape");
        await expect(
          page.getByRole("button", { name: l.more, exact: true }),
        ).toBeFocused();
        await page
          .getByRole("button", { name: d.resources, exact: true })
          .click();
        const scope = page.getByRole("dialog", {
          name: d.chooseResources,
          exact: true,
        });
        await setChoice(
          scope.getByRole("checkbox", { name: d.all, exact: true }),
        );
        const allExecution = page.waitForResponse(
          (r) =>
            r.url().endsWith(`/dashboards/${boardID}/execute`) &&
            r.status() === 200,
        );
        await scope.getByRole("button", { name: d.apply, exact: true }).click();
        const all = await allExecution;
        expect(all.request().postDataJSON().resource_ids).toEqual([]);
        expect((await all.json()).resources.length).toBeGreaterThan(1);
        await expect(page.locator(".argus-dashboard-entry-source")).toHaveText(
          origin!,
        );
        await page.screenshot({
          path: info.outputPath("scope-real.png"),
          animations: "disabled",
        });
        await page.locator(".argus-dashboard-entry-source a").click();
        await expect(
          page.getByRole("tab", { name: l.resourceTab, exact: true }),
        ).toHaveAttribute("aria-selected", "true");
        await region
          .getByRole("button", { name: d.manageBindings, exact: true })
          .click();
        await manage
          .getByRole("button", {
            name: tx(`解除关联 ${boardName}`, `Unlink ${boardName}`),
            exact: true,
          })
          .click();
        await review
          .getByRole("button", { name: tx("确认执行", "Confirm"), exact: true })
          .click();
        await expect(review).not.toBeVisible();
        await manage
          .getByRole("button", { name: d.close, exact: true })
          .click();
        await expect(
          region.getByRole("link", { name: boardName, exact: true }),
        ).toHaveCount(0);
        await page.goto(dashboardURL);
        await expect(
          page.getByRole("heading", { name: boardName, exact: true }),
        ).toBeVisible();
        const failLookup = new RegExp(`/dashboards/${boardID}/bindings$`);
        await page.route(failLookup, (r) =>
          r.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({
              code: "UNAVAILABLE",
              message: "Injected UI failure",
            }),
          }),
        );
        await page.getByRole("button", { name: l.more, exact: true }).click();
        await page
          .getByRole("menuitem", { name: l.linkedResources, exact: true })
          .click();
        await expect(drawer.getByRole("alert")).toBeVisible();
        await expect(drawer.getByText(l.noLinks, { exact: true })).toHaveCount(
          0,
        );
        await page.unroute(failLookup);
        await drawer
          .getByRole("button", { name: l.retry, exact: true })
          .click();
        await expect(
          drawer.getByText(l.noLinks, { exact: true }),
        ).toBeVisible();
        const detail = await (
          await dashboardAPI(page, `/dashboards/${boardID}`)
        ).json();
        expect(detail.revision.revision_number).toBe(1);
      });
    });
