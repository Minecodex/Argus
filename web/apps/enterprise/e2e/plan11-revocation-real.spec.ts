import { test, expect } from "@playwright/test";
import { writeFile } from "node:fs/promises";

test.skip(process.env.ARGUS_PLAN11_REVOCATION_REAL !== "1", "requires owned real SDK revocation fixture");
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`actual revoked resources stay inaccessible in Chatbox ${locale} ${theme}`, async ({ page, context }, info) => {
      test.setTimeout(90_000);
      const conversation = process.env.ARGUS_PLAN11_REVOCATION_CONVERSATION!;
      const revoked = process.env.ARGUS_PLAN11_REVOKED_APPLICATION!;
      const retained = process.env.ARGUS_PLAN11_RETAINED_APPLICATION!;
      const file = process.env.ARGUS_PLAN11_REVOKED_FILE!;
      await context.addCookies(JSON.parse(process.env.ARGUS_PLAN11_REVOCATION_COOKIES!));
      await page.addInitScript(({ locale, theme }) => {
        if (window !== window.top) return;
        localStorage.setItem("argus.locale", locale);
        localStorage.setItem("argus.theme", theme);
      }, { locale, theme });
      const errors: string[] = [];
      page.on("pageerror", error => errors.push(error.message));
      await page.goto(`/?c=${conversation}`);
      await expect(page).not.toHaveURL(/\/login/);
      const area = page.locator(".argus-chat-composer").getByRole("textbox");
      const directoryResponse = (id: string) => page.waitForResponse(response => {
        const url = new URL(response.url());
        return url.pathname.endsWith("/enterprise/telemetry/query/resources") && url.searchParams.get("kind") === "application" && url.searchParams.get("search") === id;
      });
      const deniedResponse = directoryResponse(revoked);
      await area.fill(`@${revoked}`);
      const denied = await deniedResponse;
      expect(denied.status()).toBe(200);
      const deniedPage = await denied.json();
      expect(deniedPage.items.some((row: { id: string }) => row.id === revoked)).toBe(false);
      await expect(page.getByRole("listbox").getByRole("option")).toHaveCount(0);
      const retainedResponse = directoryResponse(retained);
      await area.fill(`@${retained}`);
      const allowed = await retainedResponse;
      expect(allowed.status()).toBe(200);
      const allowedPage = await allowed.json();
      const item = allowedPage.items.find((row: { id: string }) => row.id === retained);
      expect(item).toBeTruthy();
      await expect(page.getByRole("listbox").getByRole("option").filter({ hasText: item.name })).toBeVisible();
      // Use Chromium's existing host mapping, TLS context and session cookies.
      // Node APIRequestContext does not inherit browser host-resolver-rules.
      const deniedFileStatus = await page.evaluate(async (path) => {
        const response = await fetch(path, { credentials: "include" });
        return response.status;
      }, `/api/v1/conversations/${conversation}/workspace/files/${file}/content`);
      expect(deniedFileStatus).toBe(403);
      await area.fill("");
      expect(errors).toEqual([]);
      await page.screenshot({ path: info.outputPath("revocation-chatbox.png") });
      await writeFile(info.outputPath("receipt.json"), JSON.stringify({ passed: true, locale, theme, revoked, retained, revoked_file: file, directory_denied: true, other_application_visible: true, source_file_http_status: deniedFileStatus, scope: "actual_api_post_revocation_chatbox_directory_and_old_file_denial; in-memory_transition_checked_by_component_regressions" }, null, 2));
    });
  }
