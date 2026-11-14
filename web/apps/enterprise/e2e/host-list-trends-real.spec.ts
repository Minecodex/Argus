import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { openActionMenu } from "./helpers/action-menu";

const enabled = process.env.ARGUS_HOST_LIST_TRENDS_REAL === "1";
const mapping = enabled
  ? JSON.parse(readFileSync(process.env.ARGUS_PLAN9_B11A_MAP!, "utf8"))
  : {};
const hostId = mapping.hosts?.direct?.host_id ?? "";
test.skip(
  !enabled,
  "Requires the real B11a host and deployed host list trends",
);
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});

const metricOf = (query: string) =>
  query.includes("system_cpu_")
    ? "cpu"
    : query.includes("system_memory_")
      ? "memory"
      : "storage";
const cell = (page: Page, kind: string) =>
  page.getByTestId(`host-metric-${kind}-${hostId}`);
async function login(page: Page, locale = "zh-CN", theme = "light") {
  page.setDefaultTimeout(20000);
  await page.setViewportSize({ width: 1440, height: 1080 });
  await page.addInitScript(
    ({ locale, theme }) => {
      localStorage.setItem("argus.locale", locale);
      localStorage.setItem("argus.theme", theme);
    },
    { locale, theme },
  );
  await page.goto("/login");
  await page.locator('input[autocomplete="username"]').fill("kakj");
  await page
    .locator('input[autocomplete="current-password"]')
    .fill(process.env.ARGUS_PLAN9_PASSWORD!);
  await page.locator('form button[type="submit"]').click();
  await expect(page).not.toHaveURL(/\/login/);
}

for (const locale of ["zh-CN", "en-US"])
  for (const theme of ["light", "dark"]) {
    test(`real host list progressive trends ${locale} ${theme}`, async ({
      page,
    }, info) => {
      test.setTimeout(120000);
      const english = locale === "en-US",
        errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await login(page, locale, theme);
      let release!: () => void,
        held = false;
      const gate = new Promise<void>((resolve) => {
        release = resolve;
      });
      const requests: {
        query: string;
        scope: {
          source_type: string;
          resources: { type: string; id: string }[];
        };
      }[] = [];
      page.on("request", (request) => {
        if (
          request.url().endsWith("/metrics/query_range") &&
          request.postDataJSON()?.query?.includes("argus_host_component")
        )
          requests.push(request.postDataJSON());
      });
      await page.route(
        "**/api/v1/enterprise/metrics/query_range",
        async (route) => {
          const query = route.request().postDataJSON().query;
          if (metricOf(query) === "cpu" && !held) {
            held = true;
            await gate;
          }
          await route.continue();
        },
      );
      try {
        await page.goto("/hosts");
        const row = page.locator("tbody tr").filter({
          has: page.getByRole("link", {
            name: "B11a-Linux-Direct",
            exact: true,
          }),
        });
        await expect(row).toBeVisible();
        for (const name of [
          english ? "Last heartbeat" : "最近心跳",
          english ? "Control path" : "控制路径",
        ]) {
          await expect(
            page.getByRole("columnheader", { name, exact: true }),
          ).toHaveCount(0);
        }
        await expect(cell(page, "memory").locator("svg")).toHaveCount(1, {
          timeout: 45000,
        });
        await expect(cell(page, "storage").locator("svg")).toHaveCount(1, {
          timeout: 45000,
        });
        await expect(cell(page, "cpu")).toHaveAttribute("aria-busy", "true");
        const menu = await openActionMenu(row);
        await expect(menu).toBeVisible();
        await page.keyboard.press("Escape");
        expect(
          requests.map((request) => metricOf(request.query)).sort(),
        ).toEqual(["cpu", "memory", "storage"]);
        for (const request of requests) {
          expect(request.scope.source_type).toBe("hostmetrics");
          expect(request.scope.resources).toEqual([
            { type: "host", id: hostId },
          ]);
        }
        release();
        await expect(cell(page, "cpu").locator("svg")).toHaveCount(1, {
          timeout: 45000,
        });
        await cell(page, "memory").locator(".argus-mini-trend").hover();
        const tooltip = page.getByRole("tooltip");
        await expect(tooltip).toContainText(english ? "Usage" : "使用量");
        await expect(tooltip).toContainText(english ? "Capacity" : "容量");
        await expect(tooltip).toContainText(
          english ? "Sample time" : "采样时间",
        );
        await cell(page, "memory").locator(".argus-mini-trend").focus();
        await page.keyboard.press("End");
        await expect(tooltip).toContainText(english ? "Usage" : "使用量");
        await expect(tooltip).not.toHaveAttribute("data-entering", "");
        await page.screenshot({
          path: info.outputPath("memory-tooltip.png"),
          fullPage: true,
        });
        await page
          .getByRole("heading", {
            name: english ? "Hosts" : "主机",
            exact: true,
          })
          .click();
        await page.mouse.move(5, 5);
        await expect(page.locator(".argus-mini-trend-tooltip")).toHaveCount(0);
        for (const width of [1280, 1440, 1920]) {
          await page.setViewportSize({ width, height: 1080 });
          expect(
            await page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth + 1,
            ),
          ).toBe(true);
          await page.screenshot({
            path: info.outputPath(`hosts-${width}.png`),
            fullPage: true,
          });
        }
        await page
          .getByRole("link", { name: "B11a-Linux-Direct", exact: true })
          .click();
        await page
          .getByRole("tab", {
            name: english ? "Overview" : "概览",
            exact: true,
          })
          .click();
        await expect(
          page.getByText(english ? "Control path" : "控制路径", {
            exact: true,
          }),
        ).toBeVisible();
        await expect(
          page.getByText(english ? "Last seen" : "最近心跳", { exact: true }),
        ).toBeVisible();
        await page.screenshot({
          path: info.outputPath("host-connection-detail.png"),
          fullPage: true,
        });
        expect(errors).toEqual([]);
      } finally {
        release();
      }
    });
  }

test("real metric failure leaves the host list usable and manual refresh recovers", async ({
  page,
}) => {
  test.setTimeout(120000);
  await login(page);
  let failed = false;
  await page.route(
    "**/api/v1/enterprise/metrics/query_range",
    async (route) => {
      if (
        metricOf(route.request().postDataJSON().query) === "memory" &&
        !failed
      ) {
        failed = true;
        await route.abort("failed");
      } else await route.continue();
    },
  );
  await page.goto("/hosts");
  await expect(cell(page, "memory")).toHaveAttribute(
    "data-metric-state",
    "query_failed",
  );
  await expect(cell(page, "cpu").locator("svg")).toHaveCount(1);
  await expect(cell(page, "storage").locator("svg")).toHaveCount(1);
  await expect(
    page.getByRole("link", { name: "B11a-Linux-Direct", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(cell(page, "memory").locator("svg")).toHaveCount(1, {
    timeout: 45000,
  });
});

test("real filtering cancels a pending metric without restoring the removed row", async ({
  page,
}) => {
  test.setTimeout(120000);
  await login(page);
  let release!: () => void,
    held = false;
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(
    "**/api/v1/enterprise/metrics/query_range",
    async (route) => {
      if (metricOf(route.request().postDataJSON().query) === "cpu" && !held) {
        held = true;
        await gate;
      }
      try {
        await route.continue();
      } catch {
        /* The actual browser request was cancelled by the changed scope. */
      }
    },
  );
  try {
    await page.goto("/hosts");
    await expect(cell(page, "memory").locator("svg")).toHaveCount(1, {
      timeout: 45000,
    });
    const search = page.getByPlaceholder("搜索名称 / IP…");
    await search.fill("no-matching-b11a-host");
    await expect(
      page.getByRole("link", { name: "B11a-Linux-Direct", exact: true }),
    ).toHaveCount(0);
    release();
    await expect(cell(page, "cpu")).toHaveCount(0);
    await search.clear();
    await expect(cell(page, "cpu").locator("svg")).toHaveCount(1, {
      timeout: 45000,
    });
  } finally {
    release();
  }
});
