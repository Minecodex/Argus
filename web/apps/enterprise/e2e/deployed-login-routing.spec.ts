import { expect, test } from "@playwright/test";

test.describe("deployed portal API routing", () => {
  test.skip(process.env.ARGUS_LOGIN_ROUTING_E2E !== "1", "requires a deployed real frontend");

  for (const portal of ["enterprise", "platform"] as const) {
    test(`${portal} login reaches its own API origin`, async ({ page }, testInfo) => {
      const origin = process.env[portal === "enterprise"
        ? "ARGUS_E2E_ENTERPRISE_ORIGIN"
        : "ARGUS_E2E_PLATFORM_ORIGIN"];
      expect(origin, "deployed portal origin must be explicit").toBeTruthy();
      const apiRequests: string[] = [];
      const pageErrors: string[] = [];
      page.on("request", (request) => {
        if (new URL(request.url()).pathname.startsWith("/api/v1/")) apiRequests.push(request.url());
      });
      page.on("pageerror", (error) => pageErrors.push(error.message));
      await page.goto(`${origin}/login`);
      await expect.poll(() => apiRequests.length).toBeGreaterThan(0);
      expect(apiRequests.filter((url) => new URL(url).origin !== origin), "bootstrap API requests must stay on the portal origin").toEqual([]);
      await expect(page.locator('input[name="username"]')).toBeVisible();
      await page.locator('input[name="username"]').fill(`routecheck_${Date.now()}`);
      await page.locator('input[name="password"]').fill("NoSuchAccount2026!");
      const responsePromise = page.waitForResponse((response) =>
        response.url() === `${origin}/api/v1/${portal}/auth/login` &&
        response.request().method() === "POST");
      await page.locator('button[type="submit"]').click();
      const response = await responsePromise;
      expect(response.status(), "a nonexistent test account must be rejected by the real API").toBe(401);
      expect((await response.json()).code).toBeTruthy();
      await expect(page.getByRole("alert")).toBeVisible();
      await expect(page.locator('button[type="submit"]')).toBeEnabled();
      expect(apiRequests.filter((url) => new URL(url).origin !== origin)).toEqual([]);
      expect(pageErrors).toEqual([]);
      await page.screenshot({ path: testInfo.outputPath(`${portal}-login.png`), fullPage: false });
      await testInfo.attach("api-requests", { body: JSON.stringify(apiRequests, null, 2), contentType: "application/json" });
    });
  }
});
