import { expect, test } from "@playwright/test";
import type { MockDb } from "../../../packages/api-client/src/mock/store";

for (const english of [false, true]) {
  for (const theme of ["light", "dark"]) {
    test(`Plan11 read-only default dashboard ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      const tx = (zh: string, en: string) => (english ? en : zh);
      await page.addInitScript(
        ({ english, theme }) => {
          if (window !== window.top) return;
          localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
          localStorage.setItem("argus.theme", theme);
        },
        { english, theme },
      );
      await page.setViewportSize({ width: 1440, height: 1000 });
      await page.goto("/login");
      await page.getByLabel(tx("用户名", "Username")).fill("root");
      await page.getByLabel(tx("密码", "Password")).fill("123456");
      await page
        .getByRole("button", { name: tx("登录", "Sign in"), exact: true })
        .click();
      await expect(page).not.toHaveURL(/\/login/);
      const target = crypto.randomUUID();
      await page.evaluate(
        ({ target }) => {
          const key = "argus-mock:db-v14";
          const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
          const member = db.enterpriseUsers.find(
            (row) => row.userId === "u-root",
          )!;
          const enterprise = member.enterpriseId;
          for (const role of db.roles)
            role.permissions = [
              "telemetry.dashboard.read",
              "monitoring.read",
              "workspace.use",
            ];
          (db.monitoring ??= {})[enterprise] = {
            applications: [],
            services: [],
            bindings: [],
            targets: [
              {
                id: target,
                name: "Reader MySQL",
                engine: "mysql",
                kind: "database",
                identity: { endpoint: "db.test:3306" },
                status: "active",
                version: 1,
              },
              {
                id: crypto.randomUUID(),
                name: "Private PostgreSQL",
                engine: "postgresql",
                kind: "database",
                identity: { endpoint: "private.test:5432" },
                status: "active",
                version: 1,
              },
            ],
          };
          db.dataAuthorizationGrants = [
            {
              subject_type: "user",
              subject_id: member.userId,
              resource_type: "monitoring_target",
              resource_id: target,
              active: true,
            },
          ];
          localStorage.setItem(key, JSON.stringify(db));
        },
        { target },
      );
      await page.goto("/dashboards");
      const mysql = page.getByRole("link", { name: "MySQL", exact: true });
      await expect(mysql).toBeVisible();
      await expect(
        page.getByRole("link", { name: "PostgreSQL", exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", {
          name: tx("新建仪表盘", "Create dashboard"),
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("tab", { name: tx("草稿", "Drafts"), exact: true }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("link", { name: /模型治理|Model governance/i }),
      ).toHaveCount(0);
      await page.screenshot({
        path: info.outputPath("reader-directory.png"),
        fullPage: true,
      });
      await mysql.click();
      await expect(page).toHaveURL(/\/dashboards\/[^/]+$/);
      await expect(
        page.getByRole("heading", { name: "MySQL", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", {
          name: tx("创建个人副本", "Create personal copy"),
          exact: true,
        }),
      ).toHaveCount(0);
      await page.evaluate((target) => {
        const key = "argus-mock:db-v14";
        const db = JSON.parse(localStorage.getItem(key)!) as MockDb;
        for (const grant of db.dataAuthorizationGrants ?? [])
          if (grant.resource_id === target) grant.active = false;
        localStorage.setItem(key, JSON.stringify(db));
      }, target);
      await page.goto("/dashboards");
      await expect(
        page.getByText(tx("还没有仪表盘", "No dashboards yet"), {
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        page.getByRole("link", { name: "MySQL", exact: true }),
      ).toHaveCount(0);
      await page.screenshot({
        path: info.outputPath("reader-revoked-directory.png"),
        fullPage: true,
      });
    });
  }
}
