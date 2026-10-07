import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";
import { platformOrigin } from "./origins";
import { settingsEn, settingsZh } from "../src/i18n/settings";
import { remoteAccessEn, remoteAccessZh } from "../src/i18n/remote-access";
import { sandboxEn, sandboxZh } from "../../platform/src/i18n/sandbox";

const enabled = process.env.ARGUS_PLANV2_E2E === "1";
test.use({ viewport: { width: 1280, height: 800 }, actionTimeout: 15000 });
type Case = { path: string; suffix: string; tabs: string[] };
function cases(english: boolean, audience: "enterprise" | "platform"): Case[] {
  const org = (english ? settingsEn : settingsZh).settings.org.tabs;
  const remote = (english ? remoteAccessEn : remoteAccessZh).remoteAccess.tabs;
  const secrets = (english ? settingsEn : settingsZh).settings.secrets.tabs;
  const sandbox = english ? sandboxEn : sandboxZh;
  if (audience === "platform")
    return [
      { path: "/", suffix: "/platform/overview", tabs: [] },
      ...(["images", "profiles", "quotas", "sessions", "usage"] as const).map(
        (key) => ({
          path: "/sandbox",
          suffix:
            key === "quotas"
              ? "/platform/enterprises"
              : key === "usage"
                ? "/platform/overview"
                : `/platform/sandbox/${key}`,
          tabs: [sandbox[`sandbox.tabs.${key}`]],
        }),
      ),
    ];
  return [
    ...(["departments", "roles", "access", "telemetry"] as const).map(
      (key) => ({
        path: "/settings/org",
        suffix:
          key === "access"
            ? "/enterprise/service-accounts"
            : key === "telemetry"
              ? "/enterprise/telemetry/usage"
              : `/enterprise/${key}`,
        tabs: [org[key]],
      }),
    ),
    ...(["grants", "rules", "workflows", "profiles"] as const).map((key) => ({
      path: "/settings/org",
      suffix: `/enterprise/${{ grants: "remote-access-grants", rules: "remote-access-rules", workflows: "approval-workflows", profiles: "session-profiles" }[key]}`,
      tabs: [org.remote_access, remote[key]],
    })),
    {
      path: "/settings/secrets",
      suffix: "/enterprise/credentials",
      tabs: [secrets.credentials],
    },
    {
      path: "/settings/secrets",
      suffix: "/enterprise/managed-accounts",
      tabs: [secrets.managedAccounts],
    },
  ];
}
function empty(body: unknown): unknown {
  if (Array.isArray(body)) return [];
  if (!body || typeof body !== "object") return body;
  return Object.fromEntries(
    Object.entries(body).map(([key, value]) => [
      key,
      ["has_more", "hasMore"].includes(key)
        ? false
        : ["next_cursor", "nextCursor"].includes(key)
          ? null
          : ["count", "total"].includes(key)
            ? 0
            : empty(value),
    ]),
  );
}
async function verify(page: Page) {
  await expect(page.locator(".argus-page__title").first()).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth > innerWidth,
    ),
  ).toBe(false);
  const audit = await new AxeBuilder({ page }).analyze();
  expect(
    audit.violations.filter((item) =>
      ["serious", "critical"].includes(item.impact),
    ),
  ).toEqual([]);
}
async function login(
  page: Page,
  english: boolean,
  theme: string,
  audience: "enterprise" | "platform",
) {
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
}
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    for (const audience of ["enterprise", "platform"] as const)
      test(`PlanV2 subpages real ${audience} ${english ? "en" : "zh"} ${theme}: dependencies, loading, empty, failure, denial and retry`, async ({
        page,
      }, info) => {
        test.skip(!enabled, "Requires the temporary Kubernetes environment");
        test.setTimeout(900000);
        await login(page, english, theme, audience);
        for (const item of cases(english, audience)) {
          const url =
            audience === "enterprise"
              ? item.path
              : `${platformOrigin}${item.path}`;
          const open = async () => {
            await page.goto(url);
            for (const name of item.tabs)
              await page.getByRole("tab", { name, exact: true }).click();
          };
          const response = page.waitForResponse(
            (r) =>
              r.request().method() === "GET" &&
              new URL(r.url()).pathname.startsWith("/api/v1/") &&
              new URL(r.url()).pathname.endsWith(item.suffix),
          );
          await open();
          const original = await response;
          expect(original.status(), item.suffix).toBe(200);
          const body: unknown = await original.json();
          const pattern = new RegExp(
            `/api/v1[^?]*${item.suffix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(\\?|$)`,
          );
          for (const phase of [
            "loading",
            "empty",
            "failure",
            "denied",
          ] as const) {
            let release = () => {},
              retry = false,
              requests = 0;
            const gate = new Promise<void>((resolve) => {
              release = resolve;
            });
            const handler = async (route: import("@playwright/test").Route) => {
              if (route.request().method() !== "GET" || retry) {
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
                      ? empty(body)
                      : {
                          code:
                            phase === "denied"
                              ? "PERMISSION_DENIED"
                              : "DEPENDENCY_UNAVAILABLE",
                          message_key: "errors.unavailable",
                          request_id: "00000000-0000-4000-8000-000000000017",
                          retryable: phase !== "denied",
                        },
                });
            };
            await page.route(pattern, handler);
            try {
              await open();
              await expect.poll(() => requests).toBeGreaterThan(0);
              const panel = item.tabs.length
                ? page.locator('[role="tabpanel"]:visible').last()
                : page.locator(".argus-page__content");
              if (phase === "loading")
                await expect(
                  panel.locator(".argus-spinner").first(),
                ).toBeVisible();
              else if (phase !== "empty") {
                await expect(panel.getByRole("alert").first()).toBeVisible();
                await expect(
                  panel.getByText(
                    phase === "denied"
                      ? english
                        ? "Access denied"
                        : "无权限访问"
                      : english
                        ? "Failed to load data"
                        : "数据加载失败",
                    { exact: true },
                  ),
                ).toBeVisible();
                await verify(page);
                await page.screenshot({
                  path: info.outputPath(
                    `${item.suffix.replaceAll("/", "-")}-${phase}.png`,
                  ),
                });
                retry = true;
                await panel
                  .getByRole("button", {
                    name: english ? "Retry" : "重试",
                    exact: true,
                  })
                  .click();
                await expect(
                  panel.locator(".argus-query-feedback"),
                ).toHaveCount(0);
              } else
                await expect(panel.locator(".argus-spinner")).toHaveCount(0);
              await verify(page);
              await page.screenshot({
                path: info.outputPath(
                  `${item.suffix.replaceAll("/", "-")}-${phase}${phase === "failure" || phase === "denied" ? "-recovery" : ""}.png`,
                ),
              });
            } finally {
              release();
              await page.unroute(pattern, handler);
            }
          }
          await open();
          await verify(page);
        }
      });
    test(`PlanV2 draft initial read real ${english ? "en" : "zh"} ${theme}: failed, denied, missing and retry without publishing`, async ({
      page,
    }, info) => {
      test.skip(!enabled, "Requires the temporary Kubernetes environment");
      test.setTimeout(180000);
      await login(page, english, theme, "enterprise");
      const id = process.env.ARGUS_PLANV2_GALLERY_ID!;
      const published = await (
        await dashboardAPI(page, `/dashboards/${id}`)
      ).json();
      const draft = await (
        await dashboardAPI(
          page,
          "/dashboard-drafts",
          "POST",
          {
            dashboard_id: id,
            name: published.dashboard.name,
            description: published.dashboard.description,
            spec: published.revision.spec,
            proposed_bindings: [],
          },
          201,
        )
      ).json();
      const panel = draft.spec.panels.find(
        (item: { signal: string }) => item.signal === "metrics",
      );
      const url = `/dashboard-drafts/${draft.id}/panels/${panel.id}`;
      let publications = 0;
      page.on("request", (r) => {
        if (r.url().endsWith("/preview") || r.url().endsWith("/confirm"))
          publications++;
      });
      for (const status of [503, 403, 404]) {
        let retry = false;
        const pattern = new RegExp(`/api/v1/dashboard-drafts/${draft.id}$`);
        const handler = async (route: import("@playwright/test").Route) => {
          if (route.request().method() !== "GET" || retry)
            await route.continue();
          else
            await route.fulfill({
              status,
              contentType: "application/json",
              json: {
                code:
                  status === 403
                    ? "PERMISSION_DENIED"
                    : status === 404
                      ? "NOT_FOUND"
                      : "DEPENDENCY_UNAVAILABLE",
                message_key: "errors.unavailable",
                request_id: "00000000-0000-4000-8000-000000000018",
                retryable: status === 503,
              },
            });
        };
        await page.route(pattern, handler);
        try {
          await page.goto(url);
          await expect(page.getByRole("alert").first()).toBeVisible();
          await expect(page.locator(".argus-spinner")).toHaveCount(0);
          await expect(
            page.getByRole("textbox", {
              name: english ? "Panel title" : "统计图标题",
            }),
          ).toHaveCount(0);
          await verify(page);
          await page.screenshot({
            path: info.outputPath(`draft-read-${status}.png`),
          });
          retry = true;
          await page
            .getByRole("button", {
              name: english ? "Retry" : "重试",
              exact: true,
            })
            .click();
          await expect(
            page.getByRole("textbox", {
              name: english ? "Panel title" : "统计图标题",
            }),
          ).toHaveValue(panel.title);
        } finally {
          await page.unroute(pattern, handler);
        }
      }
      expect(publications).toBe(0);
    });
    test(`PlanV2 approval policies real ${english ? "en" : "zh"} ${theme}: current configuration, failed read, denial and recovery`, async ({
      page,
    }) => {
      test.skip(!enabled, "Requires the temporary Kubernetes environment");
      test.setTimeout(120000);
      await login(page, english, theme, "enterprise");
      const strings = (english ? settingsEn : settingsZh).settings;
      const open = async () => {
        await page.goto("/settings/org");
        await page
          .getByRole("tab", { name: strings.org.tabs.policies, exact: true })
          .click();
      };
      const response = page.waitForResponse(
        (r) =>
          r.request().method() === "GET" &&
          new URL(r.url()).pathname === "/api/v1/enterprise/approval-policies",
      );
      await open();
      const original = await response;
      expect(original.status()).toBe(200);
      const policies = await original.json();
      expect(Array.isArray(policies)).toBe(true);
      const panel = page.locator('[role="tabpanel"]:visible').last();
      await expect(panel.getByRole("button").first()).toBeEnabled();
      for (const policy of policies)
        await expect(
          panel.getByText(policy.name, { exact: true }),
        ).toBeVisible();
      await verify(page);
      for (const status of [503, 403]) {
        let retry = false;
        const pattern = /\/api\/v1\/enterprise\/approval-policies$/;
        const handler = async (route: import("@playwright/test").Route) => {
          if (route.request().method() !== "GET" || retry)
            await route.continue();
          else
            await route.fulfill({
              status,
              contentType: "application/json",
              json: {
                code:
                  status === 403
                    ? "PERMISSION_DENIED"
                    : "DEPENDENCY_UNAVAILABLE",
                message_key: "errors.unavailable",
                request_id: "00000000-0000-4000-8000-000000000019",
                retryable: status === 503,
              },
            });
        };
        await page.route(pattern, handler);
        try {
          await open();
          await expect(panel.getByRole("alert").first()).toBeVisible();
          await expect(panel.locator(".argus-empty-state")).toHaveCount(0);
          await expect(panel.getByRole("button").first()).toBeDisabled();
          await verify(page);
          retry = true;
          await panel
            .getByRole("button", {
              name: english ? "Retry" : "重试",
              exact: true,
            })
            .click();
          await expect(panel.locator(".argus-query-feedback")).toHaveCount(0);
          await expect(panel.getByRole("button").first()).toBeEnabled();
        } finally {
          await page.unroute(pattern, handler);
        }
      }
    });
  }
