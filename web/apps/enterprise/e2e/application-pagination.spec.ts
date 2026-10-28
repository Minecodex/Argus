import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import type { MockDb } from "../../../packages/api-client/src/mock/store";
import { selectTrigger } from "./helpers/select";

async function seed(page: Page) {
  await page.evaluate(() => {
    const key = "argus-mock:db-v14";
    const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
    const user = db.users.find((u) => u.username === "root")!;
    const enterprise = db.enterpriseUsers.find(
      (m) => m.userId === user.id,
    )!.enterpriseId;
    const data: NonNullable<MockDb["monitoring"]>[string] = ((db.monitoring ??=
      {})[enterprise] = {
      applications: [],
      services: [],
      bindings: [],
    });
    for (let i = 0; i < 112; i++) {
      const id = `application-${String(i).padStart(3, "0")}`;
      const environment = i % 2 ? "testing" : "production";
      data.applications.push({
        id,
        application_key: id,
        name: `Orders ${String(i).padStart(3, "0")}`,
        environment,
        description: "",
        owner_name: "",
        status: "active",
        version: 1,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      });
      (db.dataAuthorizationGrants ??= []).push({
        subject_type: "user",
        subject_id: user.id,
        resource_type: "application",
        resource_id: id,
        active: true,
      });
    }
    db.auditEvents = Array.from({ length: 61 }, (_, index) => ({
      ...db.auditEvents[0]!,
      id: `pagination-audit-${index}`,
      enterpriseId: enterprise,
      actorUserId: user.id,
      actorType: "enterprise_user",
      actorName: "root",
      action: "host.create",
      origin: "admin_ui",
      result: "success",
      summary: "Pagination audit",
      createdAt: new Date().toISOString(),
    }));
    localStorage.setItem(key, JSON.stringify(db));
  });
  await page.goto("/applications");
}

for (const width of [1280, 1440, 1920])
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`application filters and shared pagination ${width} ${english ? "en" : "zh"} ${theme}`, async ({
        page,
      }, info) => {
        test.setTimeout(90_000);
        const tx = (zh: string, en: string) => (english ? en : zh);
        const errors: string[] = [];
        page.on("pageerror", (e) => errors.push(e.message));
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
        await seed(page);
        const pager = page.getByRole("navigation", {
          name: tx("分页", "Pagination"),
          exact: true,
        });
        const next = pager.getByRole("button", {
          name: tx("下一页", "Next page"),
          exact: true,
        });
        await expect(page.locator("tbody tr")).toHaveCount(50);
        await expect(page.locator(".argus-topbar")).not.toContainText(
          tx("系统正常", "All systems normal"),
        );
        await next.click();
        await expect(pager.locator('[aria-current="page"]')).toHaveText("2");
        await next.click();
        await expect(page.locator("tbody tr")).toHaveCount(12);
        await expect(next).toBeDisabled();
        await pager
          .getByRole("button", { name: tx("第 1 页", "Page 1"), exact: true })
          .click();
        await expect(
          page.getByRole("link", { name: "Orders 000", exact: true }),
        ).toBeVisible();
        await next.click();
        await selectTrigger(page, tx("环境", "Environment")).click();
        await page
          .getByRole("option", { name: tx("生产", "Production"), exact: true })
          .click();
        await expect(pager.locator('[aria-current="page"]')).toHaveText("1");
        await expect(page.locator("tbody tr")).toHaveCount(50);
        expect(
          await page.locator("tbody tr td:nth-child(2)").allTextContents(),
        ).toEqual(Array(50).fill(tx("生产", "Production")));
        await next.click();
        await expect(page.locator("tbody tr")).toHaveCount(6);
        await page.locator(".argus-page__header").scrollIntoViewIfNeeded();
        await page.screenshot({
          path: info.outputPath("filtered-applications.png"),
        });
        const search = page.getByRole("searchbox", {
          name: tx("搜索应用名称或标识", "Search application name or key"),
        });
        await search.fill(" Orders 110 ");
        await expect(page.locator("tbody tr")).toHaveCount(1);
        await expect(pager.locator('[aria-current="page"]')).toHaveText("1");
        await selectTrigger(page, tx("环境", "Environment")).click();
        await page
          .getByRole("option", { name: tx("测试", "Testing"), exact: true })
          .click();
        await expect(page.locator("tbody tr")).toHaveCount(0);
        await expect(
          page.getByText(tx("没有匹配的应用", "No matching applications"), {
            exact: true,
          }),
        ).toBeVisible();
        await expect(next).toBeDisabled();
        await selectTrigger(page, tx("环境", "Environment")).click();
        await page
          .getByRole("option", {
            name: tx("全部环境", "All environments"),
            exact: true,
          })
          .click();
        await search.fill("");
        await expect(page.locator("tbody tr")).toHaveCount(50);
        const geometry = await pager.evaluate((nav) => {
          const controls = nav
            .querySelector(".argus-pagination__pages")!
            .getBoundingClientRect();
          const rect = nav.getBoundingClientRect();
          return {
            right: Math.abs(controls.right - rect.right),
            bottom: rect.bottom - controls.bottom,
            heights: [...nav.querySelectorAll("button")].map(
              (b) => b.getBoundingClientRect().height,
            ),
          };
        });
        expect(geometry.right).toBeCloseTo(16, 0);
        expect(geometry.bottom).toBeCloseTo(16, 0);
        expect(Math.max(...geometry.heights)).toBeLessThanOrEqual(30);
        await pager
          .getByRole("button", { name: tx("第 2 页", "Page 2"), exact: true })
          .focus();
        await page.keyboard.press("Enter");
        await expect(pager.locator('[aria-current="page"]')).toHaveText("2");
        expect(
          (await new AxeBuilder({ page }).include(".argus-page").analyze())
            .violations,
        ).toEqual([]);
        await page.screenshot({ path: info.outputPath("applications.png") });
        for (const route of [
          "/monitoring",
          "/alerts",
          "/alerts?tab=policies",
          "/settings/audit",
        ]) {
          await page.goto(route);
          await expect(page.locator(".argus-pagination").first()).toBeVisible();
          await expect(page.locator(".argus-topbar")).not.toContainText(
            tx("系统正常", "All systems normal"),
          );
          expect(
            await page
              .locator(".argus-pagination")
              .first()
              .evaluate((nav) => getComputedStyle(nav).justifyContent),
          ).toBe("flex-end");
          const spacing = await page
            .locator(".argus-pagination")
            .first()
            .evaluate((nav) => {
              const rect = nav.getBoundingClientRect();
              const controls = nav
                .querySelector(".argus-pagination__pages")!
                .getBoundingClientRect();
              return {
                right: rect.right - controls.right,
                bottom: rect.bottom - controls.bottom,
                overflow:
                  rect.right - nav.parentElement!.getBoundingClientRect().right,
              };
            });
          expect(spacing.right).toBeCloseTo(16, 0);
          expect(spacing.bottom).toBeCloseTo(16, 0);
          expect(spacing.overflow).toBeLessThanOrEqual(1);
          if (route === "/alerts?tab=policies") {
            await expect(page.locator(".argus-alert-table")).toBeVisible();
            await page.screenshot({
              path: info.outputPath("alert-policies.png"),
            });
          }
        }
        await expect(page.locator("tbody tr")).toHaveCount(50);
        await next.click();
        await expect(page.locator("tbody tr")).toHaveCount(11);
        await expect(next).toBeDisabled();
        await pager
          .getByRole("button", { name: tx("第 1 页", "Page 1"), exact: true })
          .click();
        await expect(page.locator("tbody tr")).toHaveCount(50);
        expect(errors).toEqual([]);
      });
    }
