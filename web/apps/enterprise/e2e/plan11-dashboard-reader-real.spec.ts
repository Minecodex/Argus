import { expect, test } from "@playwright/test";
import { writeFile } from "node:fs/promises";

test.skip(
  process.env.ARGUS_PLAN11_DASHBOARD_READER_REAL !== "1",
  "requires owned MFA-authenticated read-only dashboard fixture",
);
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });

for (const locale of ["zh-CN", "en-US"]) {
  for (const theme of ["light", "dark"]) {
    test(`actual read-only default dashboard ${locale} ${theme}`, async ({
      page,
      context,
    }, info) => {
      test.setTimeout(90_000);
      const english = locale === "en-US";
      const tx = (zh: string, en: string) => (english ? en : zh);
      const id = process.env.ARGUS_PLAN11_READER_DEFAULT_ID!;
      const revoked = process.env.ARGUS_PLAN11_READER_REVOKED === "true";
      await context.addCookies(
        JSON.parse(process.env.ARGUS_PLAN11_READER_COOKIES!),
      );
      await page.addInitScript(
        ({ locale, theme }) => {
          if (window !== window.top) return;
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        },
        { locale, theme },
      );
      await page.goto("/dashboards");
      await expect(page).not.toHaveURL(/\/login/);
      const session = await page.evaluate(async () => {
        const response = await fetch("/api/v1/enterprise/auth/session", {
          credentials: "include",
        });
        if (response.status !== 200)
          return {
            status: response.status,
            user: "",
            permissions: [] as string[],
          };
        const value = await response.json();
        return {
          status: response.status,
          user: value.user.id as string,
          permissions: value.permissions as string[],
        };
      });
      expect(session.status).toBe(200);
      expect(session.user).toBe(process.env.ARGUS_PLAN11_READER_ID);
      expect(session.permissions).toContain("telemetry.dashboard.read");
      expect(session.permissions).not.toContain("telemetry.dashboard.manage");
      expect(session.permissions).not.toContain("model.usage.read");
      expect(session.permissions).not.toContain("*");
      const link = page.locator(`a[href="/dashboards/${id}"]`);
      if (revoked) {
        await expect(
          page.getByText(tx("还没有仪表盘", "No dashboards yet"), {
            exact: true,
          }),
        ).toBeVisible();
        await expect(link).toHaveCount(0);
      } else {
        await expect(link).toBeVisible();
      }
      await expect(
        page.locator(
          `a[href="/dashboards/${process.env.ARGUS_PLAN11_READER_PRIVATE_ID}"]`,
        ),
      ).toHaveCount(0);
      await expect(
        page.getByRole("link", { name: /模型治理|Model governance/i }),
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
      await page.screenshot({
        path: info.outputPath("reader-directory.png"),
        fullPage: true,
      });
      if (!revoked) {
        await link.click();
        await expect(
          page.getByRole("heading", {
            name: process.env.ARGUS_PLAN11_READER_DEFAULT_NAME!,
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          page.getByRole("button", {
            name: tx("创建个人副本", "Create personal copy"),
            exact: true,
          }),
        ).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath("reader-definition.png"),
          fullPage: true,
        });
      }
      await writeFile(
        info.outputPath("receipt.json"),
        JSON.stringify(
          {
            passed: true,
            locale,
            theme,
            revoked,
            read_only: true,
            default_dashboard: id,
            actual_session_verified: true,
            scope:
              "actual_deployed_read_only_portal_and_post_revocation_refreshed_context; native_Worker_adapter_parity_separate_PostgreSQL_evidence",
          },
          null,
          2,
        ),
      );
    });
  }
}
