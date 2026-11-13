import { readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { expect, test, type Page } from "@playwright/test";
import {
  kubernetesResourcesZh,
  kubernetesResourcesEn,
} from "../src/i18n/kubernetes-resources";
const root = process.env.ARGUS_KWR_ARTIFACTS ?? "";
const mapping = root
  ? JSON.parse(readFileSync(root + "/lab-resources.json", "utf8"))
  : {};
test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test.skip(
  process.env.ARGUS_KWR_REAL !== "1",
  "Requires owned real Kubernetes resources",
);
test.describe.configure({ mode: "serial" });
async function login(
  page: Page,
  english: boolean,
  theme: string,
  reader = false,
) {
  page.setDefaultTimeout(20000);
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  const credential = reader
    ? JSON.parse(readFileSync(root + "/private/reader-login.json", "utf8"))
    : { username: "kakj", password: process.env.ARGUS_KWR_PASSWORD! };
  await page.goto("/login");
  await page
    .locator('input[autocomplete="username"]')
    .fill(credential.username);
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(credential.password);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}
for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`typed details, PDB relationships and event pagination ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(180000);
      const r = (english ? kubernetesResourcesEn : kubernetesResourcesZh)
        .kubernetesResources;
      await login(page, english, theme);
      const open = async (kind: string, name: string, namespaced = true) => {
        await page.goto(
          `/kubernetes/${mapping.cluster}?tab=resources&kind=${encodeURIComponent(kind)}${namespaced ? `&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads` : ""}`,
        );
        await page.getByRole("button", { name, exact: true }).first().click();
        return page.getByRole("dialog");
      };
      let drawer = await open("hpa", "kwr-hpa");
      await expect(
        drawer.getByRole("heading", {
          name: r.autoscalingMetrics,
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        drawer.getByRole("cell", { name: "80%", exact: true }),
      ).toBeVisible();
      await expect(
        drawer.getByRole("tab", { name: r.metrics, exact: true }),
      ).toHaveCount(0);
      await page.screenshot({
        path: info.outputPath("hpa-metrics.png"),
        fullPage: true,
      });
      drawer = await open("limitrange", "kwr-limits");
      await expect(
        drawer.getByRole("heading", {
          name: r.resourceConstraints,
          exact: true,
        }),
      ).toBeVisible();
      await expect(drawer.getByRole("cell", { name: /cpu: 5m/ })).toBeVisible();
      await page.screenshot({
        path: info.outputPath("resource-constraints.png"),
        fullPage: true,
      });
      drawer = await open("pdb", "kwr-budget");
      await drawer.getByRole("tab", { name: r.relations, exact: true }).click();
      await expect(
        drawer.getByRole("button", { name: /^kwr-api-/ }).first(),
      ).toBeVisible({ timeout: 30000 });
      await drawer
        .getByRole("button", {
          name: english ? "Next page" : "下一页",
          exact: true,
        })
        .click();
      await expect(
        drawer.getByRole("button", { name: "kwr-api", exact: true }),
      ).toBeVisible({ timeout: 30000 });
      await page.screenshot({
        path: info.outputPath("pdb-workload-relationships.png"),
        fullPage: true,
      });
      drawer = await open("crd", mapping.crd, false);
      await expect(
        drawer.getByRole("heading", { name: r.apiVersions, exact: true }),
      ).toBeVisible();
      await drawer
        .getByRole("tab", { name: r.configuration, exact: true })
        .click();
      await expect(
        drawer.getByRole("heading", { name: r.crd.schema, exact: true }),
      ).toBeVisible();
      await drawer.locator("summary").filter({ hasText: "v1" }).click();
      await expect(drawer).toContainText("message");
      await drawer.getByRole("tab", { name: r.relations, exact: true }).click();
      await expect(
        drawer.getByRole("button", { name: "kwr-widget", exact: true }),
      ).toBeVisible({ timeout: 30000 });
      await page.goto(
        `/kubernetes/${mapping.cluster}?tab=resources&kind=endpointslice&namespace_id=${mapping.namespace}&namespace_name=kwr-workloads`,
      );
      await page
        .getByRole("button", { name: /^kwr-api-/ })
        .first()
        .click();
      drawer = page.getByRole("dialog");
      await expect(
        drawer.getByRole("heading", { name: r.endpointReadiness, exact: true }),
      ).toBeVisible();
      await page.screenshot({
        path: info.outputPath("endpoint-readiness.png"),
        fullPage: true,
      });
      drawer = await open("pod", "kwr-event-source");
      let pending = page.waitForResponse(
        (result) =>
          result.url().includes("/inventory/related?") &&
          result.status() === 200,
      );
      await drawer.getByRole("tab", { name: r.events, exact: true }).click();
      let data = await (await pending).json();
      const seen = new Set<string>();
      let pages = 0;
      do {
        if (data.items.length)
          await expect(
            drawer.getByRole("button", {
              name: data.items[0].name,
              exact: true,
            }),
          ).toBeVisible({ timeout: 30000 });
        else
          await expect(
            drawer.getByText(
              data.next_cursor ? r.continueRelations : r.noRelated,
              { exact: true },
            ),
          ).toBeVisible();
        for (const value of await drawer
          .getByRole("button", { name: /^kwr-review-event-/ })
          .allTextContents())
          seen.add(value);
        pages++;
        if (!data.next_cursor) break;
        pending = page.waitForResponse(
          (result) =>
            result.url().includes("/inventory/related?") &&
            result.status() === 200,
        );
        await drawer
          .getByRole("button", {
            name: english ? "Next page" : "下一页",
            exact: true,
          })
          .click();
        data = await (await pending).json();
      } while (pages < 10);
      expect(seen.size).toBe(65);
      expect(pages).toBeGreaterThan(2);
      await page.screenshot({
        path: info.outputPath("paged-real-events.png"),
        fullPage: true,
      });
    });
    test(`authorized namespace set survives full dashboard link ${english ? "en" : "zh"} ${theme}`, async ({
      page,
      context,
    }, info) => {
      test.setTimeout(120000);
      const r = (english ? kubernetesResourcesEn : kubernetesResourcesZh)
        .kubernetesResources;
      await login(page, english, theme, true);
      await page.goto(`/kubernetes/${mapping.cluster}?time_seconds=7200`);
      await expect(page.locator(".argus-resource-summary")).toBeVisible({
        timeout: 45000,
      });
      await expect(page.locator(".argus-node-allocation")).not.toContainText(
        english ? "Allocatable" : "可分配",
      );
      const results: any[] = [];
      context.on("response", async (response) => {
        if (
          response.url().includes("/dashboards/") &&
          response.url().endsWith("/execute") &&
          response.status() === 200
        )
          results.push(await response.json());
      });
      const popupPromise = page.waitForEvent("popup");
      await page.getByRole("button", { name: r.expand, exact: true }).click();
      const popup = await popupPromise;
      await expect(popup.locator(".argus-resource-summary")).toBeVisible({
        timeout: 45000,
      });
      const url = new URL(popup.url());
      expect(
        url.searchParams.get("namespace_scope")?.split(",").sort(),
      ).toEqual([mapping.namespace, mapping.second_namespace].sort());
      expect(url.searchParams.get("time_seconds")).toBe("7200");
      await expect.poll(() => results.length).toBeGreaterThan(0);
      expect(
        results
          .at(-1)
          .resources.map((ref: { id: string }) => ref.id)
          .sort(),
      ).toEqual([mapping.namespace, mapping.second_namespace].sort());
      await expect(popup.locator(".argus-node-allocation")).not.toContainText(
        english ? "Allocatable" : "可分配",
      );
      await page.screenshot({
        path: info.outputPath("restricted-overview.png"),
        fullPage: true,
      });
      await popup.screenshot({
        path: info.outputPath("restricted-standalone.png"),
        fullPage: true,
      });
      if (english && theme === "dark") {
        try {
          execFileSync("python", [root + "/reader_grants.py", "remove"], {
            encoding: "utf8",
          });
          const denied = popup.waitForResponse(
            (response) =>
              response.url().endsWith("/execute") && response.status() >= 400,
          );
          await popup
            .getByRole("button", { name: "Refresh", exact: true })
            .click();
          expect((await denied).status()).toBe(403);
          await expect(
            popup.locator(".argus-resource-summary"),
          ).not.toBeVisible({ timeout: 45000 });
          await expect(
            popup.locator(".argus-resource-graph__node"),
          ).toHaveCount(0);
          await popup.screenshot({
            path: info.outputPath("revoked-scope.png"),
            fullPage: true,
          });
        } finally {
          execFileSync("python", [root + "/reader_grants.py", "restore"], {
            encoding: "utf8",
          });
        }
      }
    });
  }
