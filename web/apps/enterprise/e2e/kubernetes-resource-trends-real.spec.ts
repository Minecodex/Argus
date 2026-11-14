import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { openActionMenu } from "./helpers/action-menu";

const enabled = process.env.ARGUS_KWR_TRENDS_REAL === "1";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B10C_MAP!, "utf8"))
  : {};
const fixtures = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_KWR_TRENDS_FIXTURES!, "utf8"))
  : {};
test.skip(
  !enabled,
  "Requires the owned B10c deployment and real metric sources",
);
test.use({
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
  ignoreHTTPSErrors: false,
});

async function login(page: Page, english = false, theme = "light") {
  page.setDefaultTimeout(20000);
  await page.setViewportSize({
    width: theme === "light" ? 1280 : 1920,
    height: 1080,
  });
  await page.addInitScript(
    ({ english, theme }) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_PLAN9_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}
function resourcePath(kind: string, namespaced = true) {
  const query = new URLSearchParams({ tab: "resources", kind });
  if (namespaced) {
    query.set("namespace_id", mapping.namespaces["b10c-workloads"].id);
    query.set("namespace_name", "b10c-workloads");
  }
  return `/kubernetes/${mapping.cluster.id}?${query}`;
}
const rowByName = (page: Page, name: string) =>
  page
    .locator(".argus-k8s-inventory-table tbody > tr")
    .filter({ has: page.getByRole("button", { name, exact: true }) });

for (const english of [false, true])
  for (const theme of ["light", "dark"]) {
    test(`real progressive resource trends ${english ? "en" : "zh"} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(180000);
      const text = (zh: string, en: string) => (english ? en : zh);
      const pageErrors: string[] = [];
      page.on("pageerror", (error) => pageErrors.push(error.message));
      await login(page, english, theme);
      let release!: () => void,
        held = false;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const reads: string[] = [];
      let preparations = 0;
      page.on("request", (request) => {
        if (request.url().endsWith("/inventory/metrics/prepare"))
          preparations++;
        if (request.url().endsWith("/inventory/metrics/read"))
          reads.push(request.postDataJSON().metric);
      });
      // Delay forwarding one actual request; no response body or metric is fabricated.
      await page.route("**/inventory/metrics/read", async (route) => {
        if (route.request().postDataJSON().metric === "cpu" && !held) {
          held = true;
          await gate;
        }
        try {
          await route.continue();
        } catch {
          /* A scope change may cancel the actual request. */
        }
      });
      await page.goto(resourcePath("deployment"));
      const row = rowByName(page, "b10c-repo");
      await expect(row).toBeVisible({ timeout: 45000 });
      const menu = await openActionMenu(row);
      await expect(menu).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(
        row.getByTestId(/^metric-memory-/).locator("svg"),
      ).toHaveCount(1, { timeout: 45000 });
      await expect(row.getByTestId(/^metric-cpu-/)).toHaveAttribute(
        "aria-busy",
        "true",
      );
      expect(preparations).toBe(1);
      expect(reads.sort()).toEqual(["cpu", "memory", "storage"]);
      await expect(
        page.getByRole("columnheader", {
          name: text("镜像", "Images"),
          exact: true,
        }),
      ).toHaveCount(0);
      await expect(
        page.getByRole("columnheader", {
          name: text("新版本副本 / 期望", "Updated / Desired"),
          exact: true,
        }),
      ).toHaveCount(0);
      release();
      await expect(row.getByTestId(/^metric-cpu-/).locator("svg")).toHaveCount(
        1,
        { timeout: 45000 },
      );
      await expect(
        row.getByTestId(/^metric-storage-/).locator("svg"),
      ).toHaveCount(1, { timeout: 45000 });
      for (const width of [1280, 1440, 1920]) {
        await page.setViewportSize({ width, height: 1080 });
        await page.screenshot({
          path: info.outputPath(`deployment-${width}.png`),
          animations: "disabled",
        });
      }
      await row.getByRole("button", { name: "b10c-repo", exact: true }).hover();
      const summary = page.getByRole("dialog", {
        name: "b10c-repo",
        exact: true,
      });
      await expect(summary).toContainText("host.docker.internal:5001");
      await expect(
        summary.getByRole("button", { name: /复制|Copy/ }).first(),
      ).toBeEnabled();
      await page.screenshot({
        path: info.outputPath("name-images-summary.png"),
        animations: "disabled",
      });
      await page.mouse.move(16, 120);
      await expect(summary).toHaveCount(0);
      const curve = row
        .getByTestId(/^metric-cpu-/)
        .locator(".argus-mini-trend");
      await curve.focus();
      await page.keyboard.press("End");
      const tooltip = page.locator(".argus-mini-trend-tooltip");
      await expect(tooltip).toContainText(text("使用量", "Usage"));
      await expect(tooltip).toContainText(
        text("上限使用率", "Limit utilization"),
      );
      await expect(tooltip).toContainText(text("上限", "Limit"));
      await page.screenshot({
        path: info.outputPath("cpu-inspection.png"),
        animations: "disabled",
      });
      await page.keyboard.press("Tab");

      for (const [kind, name, scoped] of [
        ["node", mapping.maintenance_node, false],
        ["namespace", "b10c-workloads", false],
        [
          "pod",
          fixtures.pods.find(
            (value: { variant: string }) => value.variant === "unlimited",
          ).name,
          true,
        ],
      ] as const) {
        await page.goto(resourcePath(kind, scoped));
        const resource = rowByName(page, name);
        await expect(
          resource.getByTestId(/^metric-cpu-/).locator("svg"),
        ).toHaveCount(1, { timeout: 45000 });
        await expect(
          resource.getByTestId(/^metric-memory-/).locator("svg"),
        ).toHaveCount(1, { timeout: 45000 });
        if (kind === "pod")
          await expect(
            resource
              .getByTestId(/^metric-cpu-/)
              .locator(".argus-mini-trend__reference"),
          ).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath(`${kind}-trends.png`),
          animations: "disabled",
        });
      }
      const partialName = fixtures.pods.find(
        (value: { variant: string }) => value.variant === "partial",
      ).name;
      const partialRow = rowByName(page, partialName);
      await expect(
        partialRow.getByTestId(/^metric-cpu-/).locator("svg"),
      ).toHaveCount(1, { timeout: 45000 });
      await expect(
        partialRow
          .getByTestId(/^metric-cpu-/)
          .locator(".argus-mini-trend__reference"),
      ).toHaveCount(0);
      const requestedName = fixtures.pods.find(
        (value: { variant: string }) => value.variant === "request-only",
      ).name;
      const requested = rowByName(page, requestedName).getByTestId(
        /^metric-cpu-/,
      );
      await expect(
        requested.locator(".argus-mini-trend__reference"),
      ).toHaveCount(1, { timeout: 45000 });
      await requested.locator(".argus-mini-trend").focus();
      await page.keyboard.press("End");
      await expect(page.locator(".argus-mini-trend-tooltip")).toContainText(
        text("申请利用率", "Request utilization"),
      );
      await expect(page.locator(".argus-mini-trend-tooltip")).toContainText(
        text("不是使用上限", "not a usage limit"),
      );
      for (const kind of ["job", "cronjob", "configmap"]) {
        const count = preparations;
        await page.goto(resourcePath(kind));
        await expect(
          page.getByRole("tab", {
            name:
              kind === "job"
                ? "Job"
                : kind === "cronjob"
                  ? "CronJob"
                  : "ConfigMap",
            exact: true,
          }),
        ).toHaveAttribute("aria-selected", "true");
        await expect(page.locator(".argus-mini-trend")).toHaveCount(0);
        expect(preparations).toBe(count);
      }
      expect(pageErrors).toEqual([]);
    });
  }

test("one actual metric transport failure leaves the list and other metrics usable", async ({
  page,
}, info) => {
  test.setTimeout(90000);
  await login(page);
  await page.route("**/inventory/metrics/read", async (route) => {
    if (route.request().postDataJSON().metric === "storage")
      await route.abort("connectionreset");
    else await route.continue();
  });
  await page.goto(resourcePath("deployment"));
  const row = rowByName(page, "b10c-repo");
  await expect(row.getByTestId(/^metric-cpu-/).locator("svg")).toHaveCount(1, {
    timeout: 45000,
  });
  await expect(row.getByTestId(/^metric-memory-/).locator("svg")).toHaveCount(
    1,
    { timeout: 45000 },
  );
  await expect(row.getByTestId(/^metric-storage-/)).toContainText("读取失败");
  await expect(await openActionMenu(row)).toBeVisible();
  await page.screenshot({
    path: info.outputPath("independent-storage-failure.png"),
    animations: "disabled",
  });
});

test("changing resource scope cancels delayed metric requests", async ({
  page,
}) => {
  test.setTimeout(90000);
  await page.addInitScript(() => {
    const state = window as Window & { __argusMetricAbortCount: number };
    state.__argusMetricAbortCount = 0;
    const original = window.fetch.bind(window);
    window.fetch = (input, init) => {
      const url =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.href
            : input.url;
      const signal =
        init?.signal ?? (input instanceof Request ? input.signal : undefined);
      if (url.endsWith("/inventory/metrics/read") && signal)
        signal.addEventListener(
          "abort",
          () => state.__argusMetricAbortCount++,
          { once: true },
        );
      return original(input, init);
    };
  });
  await login(page);
  let release!: () => void,
    held = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/inventory/metrics/read", async (route) => {
    if (!held && route.request().postDataJSON().metric === "cpu") {
      held = true;
      await gate;
    }
    try {
      await route.continue();
    } catch {
      /* Expected cancellation of the old resource scope. */
    }
  });
  await page.goto(resourcePath("deployment"));
  await expect.poll(() => held).toBe(true);
  await page.getByRole("tab", { name: "集群", exact: true }).click();
  await page.getByRole("tab", { name: "Node", exact: true }).click();
  release();
  const node = rowByName(page, mapping.maintenance_node);
  await expect(node.getByTestId(/^metric-cpu-/).locator("svg")).toHaveCount(1, {
    timeout: 45000,
  });
  await expect(
    page.getByRole("button", { name: "b10c-repo", exact: true }),
  ).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as Window & { __argusMetricAbortCount: number })
            .__argusMetricAbortCount,
      ),
    )
    .toBeGreaterThan(0);
});
