import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { dashboardsZh, dashboardsEn } from "../src/i18n/dashboards";
import {
  dashboardLinksZh,
  dashboardLinksEn,
} from "../src/i18n/dashboard-links";
import { setChoice } from "./helpers/choice";

for (const viewport of [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test.describe(`resource links ${viewport.width} ${english ? "en" : "zh"} ${theme}`, () => {
        test.use({ viewport, actionTimeout: 15000 });
        test.setTimeout(90000);
        test("menu, resource tab, single confirmation and independent entry scope", async ({
          page,
        }, info) => {
          const d = (english ? dashboardsEn : dashboardsZh).dashboards,
            l = (english ? dashboardLinksEn : dashboardLinksZh).dashboardLinks;
          const tx = (zh: string, en: string) => (english ? en : zh);
          const path =
            theme === "light"
              ? "/hosts/host-web-11"
              : "/kubernetes/k8s-prod-east";
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
            .getByRole("button", { name: d.create, exact: true })
            .click();
          await page
            .getByRole("textbox", { name: d.name, exact: true })
            .fill("Resource navigation");
          await page.getByRole("button", { name: d.done, exact: true }).click();
          await page
            .getByRole("button", { name: d.preview, exact: true })
            .click();
          await page
            .getByRole("dialog")
            .getByRole("button", { name: d.publish, exact: true })
            .click();
          await expect(page).toHaveURL(/\/dashboards\//);
          const dashboardURL = page.url();
          await expect(
            page.locator(".argus-dashboard-resource-links"),
          ).toHaveCount(0);
          await page.getByRole("button", { name: l.more, exact: true }).click();
          await page
            .getByRole("menuitem", { name: l.linkedResources, exact: true })
            .click();
          const drawer = page.getByRole("dialog", {
            name: l.linkedResources,
            exact: true,
          });
          await expect(drawer).toContainText(l.noLinks);
          await expect(
            drawer.getByRole("button", { name: d.manageBindings, exact: true }),
          ).toHaveCount(0);
          await drawer.press("Escape");
          await expect(
            page.getByRole("button", { name: l.more, exact: true }),
          ).toBeFocused();
          await page.goto(path);
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
              name: tx("关联 Resource navigation", "Link Resource navigation"),
              exact: true,
            })
            .click();
          const review = page.getByRole("dialog", {
            name: d.bindingPreviewTitle,
            exact: true,
          });
          await review
            .getByRole("button", { name: tx("取消", "Cancel"), exact: true })
            .click();
          await manage
            .getByRole("button", { name: d.close, exact: true })
            .click();
          await expect(
            region.getByRole("link", {
              name: "Resource navigation",
              exact: true,
            }),
          ).toHaveCount(0);
          await region
            .getByRole("button", { name: d.manageBindings, exact: true })
            .click();
          await manage
            .getByRole("button", {
              name: tx("关联 Resource navigation", "Link Resource navigation"),
              exact: true,
            })
            .click();
          await review
            .getByRole("button", {
              name: tx("确认执行", "Confirm"),
              exact: true,
            })
            .click();
          await expect(review).not.toBeVisible();
          await manage
            .getByRole("button", { name: d.close, exact: true })
            .click();
          await expect(
            region.getByRole("link", {
              name: "Resource navigation",
              exact: true,
            }),
          ).toBeVisible();
          await page.screenshot({
            path: info.outputPath("resource-tab.png"),
            animations: "disabled",
          });
          await region
            .getByRole("button", { name: l.openDashboard, exact: true })
            .click();
          await expect(page).toHaveURL(/resource=/);
          const origin = page.locator(".argus-dashboard-entry-source");
          await expect(origin.locator("a")).toBeVisible();
          const originText = await origin.textContent();
          const scopeButton = page.getByRole("button", {
            name: d.resources,
            exact: true,
          });
          await expect(scopeButton).not.toContainText(d.all);
          await scopeButton.click();
          const scope = page.getByRole("dialog", {
            name: d.chooseResources,
            exact: true,
          });
          await setChoice(
            scope.getByRole("checkbox", { name: d.all, exact: true }),
          );
          await scope
            .getByRole("button", { name: d.apply, exact: true })
            .click();
          await expect(scopeButton).toContainText(d.all);
          await expect(origin).toHaveText(originText!);
          await page.screenshot({
            path: info.outputPath("entry-context.png"),
            animations: "disabled",
          });
          await page.getByRole("button", { name: l.more, exact: true }).click();
          await page
            .getByRole("menuitem", { name: l.linkedResources, exact: true })
            .click();
          await expect(
            drawer.locator(".argus-dashboard-linked-resource"),
          ).toHaveCount(1);
          const axe = await new AxeBuilder({ page }).analyze();
          expect(
            axe.violations.filter((v) =>
              ["serious", "critical"].includes(v.impact),
            ),
          ).toEqual([]);
          const box = (await drawer.boundingBox())!;
          expect(Math.abs(box.x + box.width - viewport.width)).toBeLessThan(1);
          await page.screenshot({
            path: info.outputPath("linked-resources.png"),
            animations: "disabled",
          });
          await drawer
            .getByRole("link", { name: l.openResource, exact: true })
            .click();
          await expect(
            page.getByRole("tab", { name: l.resourceTab, exact: true }),
          ).toHaveAttribute("aria-selected", "true");
          await region
            .getByRole("button", { name: d.manageBindings, exact: true })
            .click();
          await manage
            .getByRole("button", {
              name: tx(
                "解除关联 Resource navigation",
                "Unlink Resource navigation",
              ),
              exact: true,
            })
            .click();
          await review
            .getByRole("button", {
              name: tx("确认执行", "Confirm"),
              exact: true,
            })
            .click();
          await expect(review).not.toBeVisible();
          await manage
            .getByRole("button", { name: d.close, exact: true })
            .click();
          await expect(
            region.getByRole("link", {
              name: "Resource navigation",
              exact: true,
            }),
          ).toHaveCount(0);
          await page.goto(dashboardURL);
          await expect(
            page.getByRole("heading", {
              name: "Resource navigation",
              exact: true,
            }),
          ).toBeVisible();
        });
      });
    }
