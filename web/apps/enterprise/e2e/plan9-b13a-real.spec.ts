import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";
import { createMfaLogin } from "./helpers/mfa-login";
import { selectTrigger } from "./helpers/select";
import { browserApi } from "./helpers/browser-api";

const enabled = process.env.ARGUS_PLAN9_B13A_REAL === "1";
const root = process.env.ARGUS_PLAN9_B13A_OUT ?? "";
const map = enabled
  ? JSON.parse(readFileSync(root + "/resource-map.json", "utf8"))
  : {};
test.skip(!enabled, "Requires the owned B13a deployment and normal login");
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

async function board(page: Page, id: string, search = "") {
  const executing = page.waitForResponse(
    (response) =>
      response.url().includes(`/dashboards/${id}/execute`) &&
      response.request().method() === "POST" &&
      !response.request().postDataJSON()?.candidates_only,
  );
  await page.goto(`/dashboards/${id}${search}`);
  const response = await executing;
  expect(response.status()).toBe(200);
  const value = await response.json();
  await expect(page.locator(".argus-dashboard-panel-content")).toHaveCount(
    value.panels.length,
  );
  return value;
}

for (const [width, english, theme] of [
  [1280, false, "light"],
  [1280, true, "dark"],
  [1440, false, "dark"],
  [1440, true, "light"],
  [1920, false, "light"],
  [1920, true, "dark"],
] as const) {
  test(`B13a real dashboards ${width} ${english ? "en" : "zh"} ${theme}`, async ({
    page,
  }, info) => {
    test.setTimeout(150_000);
    await page.setViewportSize({ width, height: 1000 });
    await page.addInitScript(
      ({ english, theme }) => {
        if (window.self !== window.top) return;
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", theme);
      },
      { english, theme },
    );
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await plan7Login(page, "/login", "kakj", process.env.ARGUS_PLAN9_PASSWORD!);
    const host = await board(
      page,
      map.dashboards["host.overview"].id,
      `?resource=${map.hosts.orders.host_id}&resource_type=host&time_seconds=900`,
    );
    expect(host.panels).toHaveLength(6);
    expect(
      host.panels.every((p: { status: string }) => p.status === "success"),
    ).toBe(true);
    const personal = await board(
      page,
      map.personal_dashboard.id,
      `?resource=${map.targets.redis.target_id}&resource_type=monitoring_target&time_seconds=900`,
    );
    expect(personal.panels).toHaveLength(11);
    expect(
      personal.panels.every((p: { status: string }) => p.status === "success"),
    ).toBe(true);
    await expect(
      page.getByText("B13a Redis 运行时长 / Redis uptime", { exact: true }),
    ).toBeVisible();
    const archived = map.archive_probe.input;
    const historical = await board(
      page,
      map.dashboards.memcached.id,
      `?resource=${map.targets.memcached.target_id}&resource_type=monitoring_target&time_from=${encodeURIComponent(archived.from)}&time_to=${encodeURIComponent(archived.to)}`,
    );
    expect(historical.panels).toHaveLength(9);
    expect(
      historical.panels.every(
        (p: { status: string }) => p.status === "success",
      ),
    ).toBe(true);
    const combined = await board(page, map.batch_dashboard.id);
    expect(combined.panels).toHaveLength(6);
    const redis = combined.panels.find(
      (p: { id: string }) => p.id === "redis-targets",
    );
    expect(Number(redis.targets[0].data[0].value[1])).toBe(1);
    await expect.poll(() => page.locator("canvas").count()).toBeGreaterThan(0);
    const dimensions = await page.locator("canvas").evaluateAll((nodes) =>
      nodes.map((node) => {
        const r = node.getBoundingClientRect();
        return { width: r.width, height: r.height };
      }),
    );
    expect(dimensions.every((r) => r.width > 10 && r.height > 10)).toBe(true);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    await page.screenshot({
      path: info.outputPath("b13a-dashboard.png"),
      fullPage: true,
      animations: "disabled",
    });
    await page
      .getByRole("button", { name: english ? "Refresh" : "刷新", exact: true })
      .focus();
    await expect(
      page.getByRole("button", {
        name: english ? "Refresh" : "刷新",
        exact: true,
      }),
    ).toBeFocused();
    const manual = page.waitForResponse(
      (response) =>
        response
          .url()
          .includes(`/dashboards/${map.batch_dashboard.id}/execute`) &&
        response.request().method() === "POST" &&
        !response.request().postDataJSON()?.candidates_only,
    );
    await page.keyboard.press("Enter");
    expect((await manual).status()).toBe(200);
    expect(errors).toEqual([]);
  });
}

test("B13a visible auto refresh and navigation cancellation", async ({
  page,
}) => {
  test.setTimeout(90_000);
  await page.addInitScript(() => {
    if (window.self !== window.top) return;
    localStorage.setItem("argus.locale", "en-US");
  });
  await plan7Login(page, "/login", "kakj", process.env.ARGUS_PLAN9_PASSWORD!);
  await board(page, map.batch_dashboard.id);
  let requests = 0;
  page.on("request", (request) => {
    if (
      request.url().includes(`/dashboards/${map.batch_dashboard.id}/execute`) &&
      request.method() === "POST"
    )
      requests++;
  });
  await selectTrigger(page, "Refresh interval").click();
  await page.getByRole("option", { name: "Every 5 s", exact: true }).click();
  await expect.poll(() => requests, { timeout: 15_000 }).toBeGreaterThan(0);
  await page.goto("/hosts");
  await expect(
    page.getByRole("heading", { name: "Hosts", exact: true }),
  ).toBeVisible();
  const stopped = requests;
  await page.waitForTimeout(6500);
  expect(requests).toBe(stopped);
});

test("B13a normal-login member sees Orders scope and rejects explicit Reports", async ({
  page,
}, info) => {
  test.setTimeout(90_000);
  const credentials = JSON.parse(
    readFileSync(root + "/private/reader-credentials.json", "utf8"),
  );
  await page.addInitScript(() => {
    if (window.self !== window.top) return;
    localStorage.setItem("argus.locale", "zh-CN");
  });
  await createMfaLogin("enterprise")(
    page,
    "/login",
    credentials.username,
    credentials.password,
  );
  const value = await board(page, map.batch_dashboard.id);
  expect(value.resources.map((r: { id: string }) => r.id).sort()).toEqual(
    [
      map.hosts.orders.host_id,
      map.targets.redis.target_id,
      map.applications.orders.id,
    ].sort(),
  );
  const controls = page.getByRole("group", { name: "仪表盘查询条件" });
  await controls.getByRole("button", { name: /^筛选应用:/ }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText("B13a Orders", { exact: true })).toBeVisible();
  await expect(dialog.getByText("B13a Reports", { exact: true })).toHaveCount(
    0,
  );
  await page.keyboard.press("Escape");
  const denial = await browserApi(
    page,
    `/api/v1/dashboards/${map.batch_dashboard.id}/execute`,
    {
      method: "POST",
      data: {
        from: value.from,
        to: value.to,
        resource_mode: "explicit",
        resource_types: ["host"],
        resource_ids: [map.hosts.reports.host_id],
      },
    },
  );
  expect([403, 404]).toContain(denial.status());
  await page.screenshot({
    path: info.outputPath("b13a-member.png"),
    animations: "disabled",
  });
  await controls.getByRole("button", { name: /^选择资源:/ }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "清空选择", exact: true })
    .click();
  const emptyResponse = page.waitForResponse(
    (response) =>
      response
        .url()
        .includes(`/dashboards/${map.batch_dashboard.id}/execute`) &&
      response.request().method() === "POST" &&
      response.request().postDataJSON()?.resource_mode === "explicit" &&
      response.request().postDataJSON()?.resource_ids?.length === 0 &&
      !response.request().postDataJSON()?.candidates_only,
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "确认选择", exact: true })
    .click();
  const empty = await (await emptyResponse).json();
  expect(empty.scope_empty).toBe(true);
  expect(empty.resources).toEqual([]);
  await expect(
    controls.getByRole("button", { name: /^选择资源:.*0/ }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("b13a-explicit-empty.png"),
    animations: "disabled",
  });
});
