import { expect, type BrowserContext, type Page } from "@playwright/test";
import { createMfaLogin } from "./mfa-login";

const freshLogin = createMfaLogin("enterprise");
let authenticated:
  | {
      username: string;
      cookies: Awaited<ReturnType<BrowserContext["cookies"]>>;
    }
  | undefined;

/** Each test still has its own context and locale/theme storage. Reuse only the
 * real MFA-authenticated session, and recheck its server identity in every
 * context, instead of waiting a new TOTP period for every layout assertion. */
export async function plan7Login(
  page: Page,
  url: string,
  username: string,
  password: string,
) {
  if (!authenticated && process.env.ARGUS_PLAN7_SESSION_COOKIES) {
    await page
      .context()
      .addCookies(JSON.parse(process.env.ARGUS_PLAN7_SESSION_COOKIES));
    authenticated = { username, cookies: await page.context().cookies() };
  }
  if (authenticated?.username === username) {
    await page.context().addCookies(authenticated.cookies);
    const response = page.waitForResponse(
      (r) =>
        r.url().includes("/enterprise/auth/session") &&
        r.request().method() === "GET",
    );
    await page.goto("/");
    const session = await response;
    if (session.status() === 200) {
      expect((await session.json()).user.username).toBe(username);
      await expect(page.locator(".argus-chat-composer")).toBeVisible();
      authenticated.cookies = await page.context().cookies();
      return;
    }
    expect(session.status()).toBe(401);
    authenticated = undefined;
  }
  await freshLogin(page, url, username, password);
  authenticated = { username, cookies: await page.context().cookies() };
}
