import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createMfaLogin } from "./helpers/mfa-login";
import { platformOrigin } from "./origins";

const enabled = process.env.ARGUS_PLANV2_E2E === "1";
test.use({ actionTimeout: 15000 });
const enterpriseCases = [
  ["/hosts", "/enterprise/hosts"],
  ["/kubernetes", "/enterprise/kubernetes-clusters"],
  ["/dashboards", "/dashboards"],
  ["/tasks", "/executions"],
  ["/approvals", "/pending-actions"],
  ["/remote-sessions", "/enterprise/remote-access-sessions"],
  ["/settings/org", "/enterprise/users"],
  ["/settings/ai", "/enterprise/ai-models"],
  ["/settings/mcp", "/enterprise/mcp-connections"],
  ["/settings/secrets", "/enterprise/secrets"],
  ["/settings/audit", "/enterprise/audit-events"],
] as const;
const platformCases = [
  ["/enterprises", "/platform/enterprises"],
  ["/admins", "/platform/enterprise-admins"],
  ["/sandbox", "/platform/sandbox/backends"],
  ["/audit", "/platform/audit-events"],
  ["/pki", "/platform/pki"],
] as const;

function emptyCollections(body: unknown): unknown {
  if (Array.isArray(body)) return [];
  if (!body || typeof body !== "object") return body;
  return Object.fromEntries(
    Object.entries(body).map(([key, value]) => [
      key,
      ["has_more", "hasMore"].includes(key)
        ? false
        : ["next_cursor", "nextCursor"].includes(key)
          ? null
          : ["total", "count"].includes(key) && typeof value === "number"
            ? 0
            : emptyCollections(value),
    ]),
  );
}

async function verifyPage(page: Page) {
  await expect(page.locator(".argus-page__title").first()).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
  ).toBe(false);
  const audit = await new AxeBuilder({ page }).analyze();
  expect(
    audit.violations.filter((v) => ["serious", "critical"].includes(v.impact)),
  ).toEqual([]);
}

for (const audience of ["enterprise", "platform"] as const)
  for (const english of [false, true])
    for (const theme of ["light", "dark"]) {
      test(`PlanV2 portal states real ${audience} ${english ? "en" : "zh"} ${theme}: loading, empty, failure, denied and recovery`, async ({
        page,
      }, info) => {
        test.skip(!enabled, "Requires the temporary Kubernetes environment");
        test.setTimeout(540000);
        await page.addInitScript(
          ({ english, theme }) => {
            localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
            localStorage.setItem("argus.theme", theme);
          },
          { english, theme },
        );
        await createMfaLogin(audience)(
          page,
          audience === "enterprise" ? "/login" : `${platformOrigin}/login`,
          process.env[
            audience === "enterprise"
              ? "ARGUS_PLANV2_USERNAME"
              : "ARGUS_PLANV2_PLATFORM_USERNAME"
          ]!,
          process.env[
            audience === "enterprise"
              ? "ARGUS_PLANV2_PASSWORD"
              : "ARGUS_PLANV2_PLATFORM_PASSWORD"
          ]!,
        );
        const cases =
          audience === "enterprise" ? enterpriseCases : platformCases;
        for (const [path, suffix] of cases) {
          const url =
            audience === "enterprise" ? path : `${platformOrigin}${path}`;
          const response = page.waitForResponse(
            (r) =>
              r.request().method() === "GET" &&
              new URL(r.url()).pathname.startsWith("/api/v1/") &&
              new URL(r.url()).pathname.endsWith(suffix),
          );
          await page.goto(url);
          const original = await response;
          expect(original.status(), `${path} original API`).toBe(200);
          const body: unknown = await original.json();
          await verifyPage(page);
          const pattern = new RegExp(
            `/api/v1[^?]*${suffix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(\\?|$)`,
          );
          for (const phase of [
            "loading",
            "empty",
            "failure",
            "denied",
          ] as const) {
            let release: () => void = () => {};
            const gate = new Promise<void>((resolve) => {
              release = resolve;
            });
            let requests = 0;
            const handler = async (route: import("@playwright/test").Route) => {
              if (route.request().method() !== "GET") {
                await route.continue();
                return;
              }
              requests++;
              if (phase === "loading") {
                await gate;
                await route.continue().catch(() => {});
              } else
                await route.fulfill({
                  status:
                    phase === "empty" ? 200 : phase === "denied" ? 403 : 503,
                  contentType: "application/json",
                  json:
                    phase === "empty"
                      ? emptyCollections(body)
                      : {
                          code:
                            phase === "denied"
                              ? "PERMISSION_DENIED"
                              : "TELEMETRY_DEPENDENCY_UNAVAILABLE",
                          message_key: "errors.unavailable",
                          request_id: "00000000-0000-4000-8000-000000000007",
                          retryable: phase !== "denied",
                        },
                });
            };
            await page.route(pattern, handler);
            try {
              await page.goto(url);
              await expect.poll(() => requests).toBeGreaterThan(0);
              if (phase === "loading")
                await expect(
                  page.locator(".argus-spinner").first(),
                ).toBeVisible();
              else if (phase !== "empty")
                await expect(page.getByRole("alert").first()).toBeVisible();
              else
                await expect(
                  page.locator(".argus-spinner").first(),
                ).not.toBeVisible();
              await verifyPage(page);
              await page.screenshot({
                path: info.outputPath(
                  `${path.replaceAll("/", "-")}-${phase}.png`,
                ),
              });
            } finally {
              release();
              await page.unroute(pattern, handler);
            }
          }
          await page.goto(url);
          await verifyPage(page);
        }
      });
    }
