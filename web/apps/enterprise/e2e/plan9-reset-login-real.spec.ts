import { expect, test } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";

test.use({ trace: "off", video: "off" });

test.describe("Plan9 fresh requested accounts", () => {
  test.skip(
    process.env.ARGUS_PLAN9_RESET_LOGIN_E2E !== "1",
    "requires the explicitly requested fresh local accounts",
  );
  for (const audience of ["platform", "enterprise"] as const) {
    test(`${audience} normal portal login`, async ({ page }, info) => {
      const origin =
        audience === "platform"
          ? "https://platform.argus.dev"
          : "https://argus.dev";
      const username = audience === "platform" ? "admin" : "kakj";
      await page.addInitScript(() => {
        localStorage.setItem("argus.locale", "zh-CN");
        localStorage.setItem("argus.theme", "light");
      });
      await createMfaLogin(audience)(
        page,
        `${origin}/login`,
        username,
        process.env.ARGUS_PLAN9_RESET_PASSWORD ?? "",
      );
      const response = await page.request.get(
        `${origin}/api/v1/${audience}/auth/session`,
      );
      expect(response.status()).toBe(200);
      const session = await response.json();
      expect(session.user.username).toBe(username);
      if (audience === "enterprise") {
        expect(session.user.enterprise_id).toBe(
          process.env.ARGUS_PLAN9_ENTERPRISE_ID,
        );
      } else {
        expect(session.user.enterprise_id ?? null).toBeNull();
      }
      await page.screenshot({
        path: info.outputPath(`${audience}-after-normal-login.png`),
        fullPage: true,
      });
    });
  }
});
