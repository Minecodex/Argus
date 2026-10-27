import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { emptyDashboardSpec } from "../../../packages/api-client/src/dashboard";
import type { MockDb } from "../../../packages/api-client/src/mock/store";
import { selectTrigger } from "./helpers/select";

const folderName = "生产环境 Production dashboards with a long folder name";

async function seedDirectory(page: Page) {
  await page.evaluate(
    ({ spec, folderName }) => {
      const key = "argus-mock:db-v14";
      const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
      const user = db.users.find((user) => user.username === "root")!;
      const enterprise = db.enterpriseUsers.find(
        (member) => member.userId === user.id,
      )!.enterpriseId;
      const updated_at = new Date().toISOString();
      db.dashboards = { items: [], drafts: [], folders: [], revisions: [] };
      db.dashboards.folders.push({
        enterprise,
        value: {
          id: "directory-production",
          name: folderName,
          description: "",
          sort_order: 0,
          status: "active",
          version: 1,
        },
      });
      for (const tab of ["published", "drafts", "archived"]) {
        for (const [index, name] of [
          "CPU 生产",
          "Memory 生产",
          "CPU 未分组",
        ].entries()) {
          const id = `directory-${tab}-${index}`;
          const common = {
            id,
            name: `${name} ${tab}`,
            description: "CPU appears in this description only for Memory",
            folder_id: index < 2 ? "directory-production" : undefined,
            updated_at,
          };
          if (tab === "drafts") {
            db.dashboards.drafts.push({
              enterprise,
              editor: user.id,
              value: {
                ...common,
                base_object_version: 0,
                draft_version: 1,
                status: "editing",
                spec,
                proposed_bindings: [],
              },
            });
          } else {
            db.dashboards.items.push({
              enterprise,
              creator: user.id,
              value: {
                ...common,
                active_revision_id: `${id}-revision`,
                version: 1,
                lifecycle: tab === "archived" ? "archived" : "active",
              },
            });
            (db.dataAuthorizationGrants ??= []).push({
              subject_type: "user",
              subject_id: user.id,
              resource_type: "dashboard",
              resource_id: id,
              active: true,
            });
          }
        }
      }
      localStorage.setItem(key, JSON.stringify(db));
    },
    { spec: emptyDashboardSpec(), folderName },
  );
}

for (const english of [false, true]) {
  for (const theme of ["light", "dark"]) {
    for (const width of [1280, 1440, 1920]) {
      test(`dashboard directory filters and tabs ${english ? "en" : "zh"} ${theme} ${width}`, async ({
        page,
      }, info) => {
        test.setTimeout(60_000);
        const tx = (zh: string, en: string) => (english ? en : zh);
        await page.setViewportSize({ width, height: 900 });
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        const errors: string[] = [];
        page.on("pageerror", (error) => errors.push(error.message));
        await page.goto("/login");
        await page.getByLabel(tx("用户名", "Username")).fill("root");
        await page.getByLabel(tx("密码", "Password")).fill("123456");
        await page
          .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
          .click();
        await expect(page).not.toHaveURL(/login/);
        await seedDirectory(page);
        await page.goto("/dashboards");

        const tabs = page.getByRole("tablist");
        await expect(tabs.getByRole("tab")).toHaveCount(3);
        const published = tabs.getByRole("tab", {
          name: tx("已发布", "Published"),
          exact: true,
        });
        await expect(published).toHaveAttribute("aria-selected", "true");
        const panel = page.getByRole("tabpanel");
        const search = panel.getByRole("searchbox", {
          name: tx("按仪表盘名称搜索…", "Search dashboards by name…"),
        });
        const folder = selectTrigger(panel, tx("分组", "Folder"));
        const chooseFolder = async (name: string) => {
          await folder.click();
          await page.getByRole("option", { name, exact: true }).click();
        };
        const rows = panel.locator("tbody tr");
        await expect(
          panel.getByRole("link", { name: "CPU 生产 published", exact: true }),
        ).toBeVisible();
        const searchBox = await panel
          .locator(".argus-search-input")
          .boundingBox();
        const folderBox = await folder.boundingBox();
        expect(folderBox!.width).toBeLessThanOrEqual(240);
        expect(searchBox!.width).toBeLessThanOrEqual(320);
        expect(Math.abs(folderBox!.y - searchBox!.y)).toBeLessThanOrEqual(1);
        expect(
          Math.abs(folderBox!.height - searchBox!.height),
        ).toBeLessThanOrEqual(1);
        expect(folderBox!.x).toBeGreaterThan(searchBox!.x + searchBox!.width);
        await page.screenshot({
          path: info.outputPath("directory.png"),
          fullPage: true,
        });

        await search.fill(" cPu ");
        await expect(rows).toHaveCount(2);
        await chooseFolder(folderName);
        await expect(rows).toHaveCount(1);
        expect((await folder.boundingBox())!.width).toBeLessThanOrEqual(240);

        for (const [key, label] of [
          ["published", tx("已发布", "Published")],
          ["drafts", tx("我的草稿", "My drafts")],
          ["archived", tx("已归档", "Archived")],
        ]) {
          await tabs.getByRole("tab", { name: label, exact: true }).click();
          await expect(search).toHaveValue(" cPu ");
          await expect(folder).toContainText(folderName);
          await expect(rows).toHaveCount(1);
          await expect(
            panel.getByRole("link", { name: `CPU 生产 ${key}`, exact: true }),
          ).toBeVisible();
          await search.fill("");
          await expect(rows).toHaveCount(2);
          await search.fill("未分组");
          await expect(rows).toHaveCount(0);
          await expect(
            panel.getByRole("heading", {
              name: tx("没有匹配的仪表盘", "No matching dashboards"),
              exact: true,
            }),
          ).toBeVisible();
          await chooseFolder(tx("未分组", "Unfiled"));
          await expect(rows).toHaveCount(1);
          await expect(
            panel.getByRole("link", { name: `CPU 未分组 ${key}`, exact: true }),
          ).toBeVisible();
          await chooseFolder(tx("全部分组", "All folders"));
          await search.fill(" cPu ");
          await expect(rows).toHaveCount(2);
          await chooseFolder(folderName);
        }

        await published.click();
        await published.press("ArrowRight");
        const draftTab = tabs.getByRole("tab", {
          name: tx("我的草稿", "My drafts"),
          exact: true,
        });
        await expect(draftTab).toBeFocused();
        await expect(draftTab).toHaveAttribute("aria-selected", "true");
        await expect(
          panel.getByRole("link", { name: "CPU 生产 drafts", exact: true }),
        ).toBeVisible();
        expect(
          await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
          ),
        ).toBe(true);
        const audit = await new AxeBuilder({ page }).analyze();
        expect(
          audit.violations.filter((v) =>
            ["serious", "critical"].includes(v.impact ?? ""),
          ),
        ).toEqual([]);
        expect(errors).toEqual([]);
      });
    }
  }
}
