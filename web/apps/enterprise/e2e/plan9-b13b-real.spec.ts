import { readFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { plan7Login } from "./helpers/plan7-login";
import { createMfaLogin } from "./helpers/mfa-login";
import { browserApi } from "./helpers/browser-api";

const enabled = process.env.ARGUS_PLAN9_B13B_REAL === "1";
const root = process.env.ARGUS_PLAN9_B13B_OUT ?? "";
const read = (name: string) =>
  enabled ? JSON.parse(readFileSync(`${root}/${name}.json`, "utf8")) : {};
const map = read("resource-map");
const core = read("core-acceptance");
const retry = read("grouping-retry-acceptance");
test.skip(
  !enabled,
  "Requires the real B13b faults, restored scopes and owned TLS inbox",
);
test.use({
  ignoreHTTPSErrors: false,
  trace: "off",
  video: "off",
  reducedMotion: "reduce",
});

async function normalLogin(
  page: Page,
  width: number,
  english: boolean,
  theme: string,
) {
  await page.setViewportSize({ width, height: 1050 });
  await page.addInitScript(
    ({ english, theme }) => {
      if (window.self !== window.top) return;
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
  await plan7Login(page, "/login", "kakj", process.env.ARGUS_PLAN9_PASSWORD!);
}

for (const [width, english, theme] of [
  [1280, false, "light"],
  [1280, true, "dark"],
  [1440, false, "dark"],
  [1440, true, "light"],
  [1920, false, "light"],
  [1920, true, "dark"],
] as const) {
  test(`B13b real alert history and delivery ${width} ${english ? "en" : "zh"} ${theme}`, async ({
    page,
  }, info) => {
    test.setTimeout(120_000);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await normalLogin(page, width, english, theme);
    await page.goto(
      `/alerts?tab=active&policy_id=${map.alert_policies.no_data.id}`,
    );
    await expect(
      page.getByRole("heading", {
        name: english ? "Alert center" : "告警中心",
        exact: true,
      }),
    ).toBeVisible();
    await expect(page.getByRole("tab")).toHaveCount(6);
    await expect(page.locator("tbody tr")).toHaveCount(1);
    await expect(page.locator("tbody tr").first()).toContainText(
      "B13b-Reports-Host",
    );
    const noData = await browserApi(
      page,
      `/api/v1/alerts/instances?policy_id=${map.alert_policies.no_data.id}`,
    );
    expect(noData.status()).toBe(200);
    const current = (await noData.json()).items;
    expect(current).toHaveLength(1);
    expect(current[0].health).toBe("no_data");
    expect(current[0].state.value ?? null).toBeNull();
    const event = core.history.find(
      (x: { policy_id: string }) =>
        x.policy_id === map.alert_policies.independent.id,
    );
    await page.goto(
      `/alerts?tab=history&policy_id=${map.alert_policies.independent.id}&instance=${event.id}`,
    );
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(
      dialog
        .getByText("B13b independent service and CPU", { exact: true })
        .first(),
    ).toBeVisible();
    const detail = await browserApi(
      page,
      `/api/v1/alerts/instances/${event.id}`,
    );
    const value = await detail.json();
    expect(value.instance.lifecycle).toBe("resolved");
    expect(
      value.events.some(
        (x: { event_type: string }) => x.event_type === "first",
      ),
    ).toBe(true);
    expect(
      value.events.some(
        (x: { event_type: string }) => x.event_type === "recovery",
      ),
    ).toBe(true);
    const trend = dialog.getByRole("img", {
      name: english ? "Metric trend" : "指标趋势",
      exact: true,
    });
    await expect(trend).toBeVisible({ timeout: 15_000 });
    expect(await trend.locator("path, polyline").count()).toBeGreaterThan(0);
    await page.screenshot({
      path: info.outputPath("b13b-history.png"),
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    const retried = retry.delivery_details.find(
      (x: { attempts: { result_code: string }[] }) =>
        x.attempts.some((a) => String(a.result_code) === "451"),
    );
    expect(retried).toBeTruthy();
    await page.goto(
      `/alerts?tab=deliveries&channel_id=${map.notification.channel_id}&delivery=${retried.delivery.id}`,
    );
    await expect(page.getByRole("dialog")).toBeVisible();
    await expect(page.getByRole("dialog")).toContainText("451");
    await expect(page.getByRole("dialog")).toContainText("250");
    await page.screenshot({
      path: info.outputPath("b13b-delivery-attempts.png"),
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await page.goto(
      `/alerts?tab=policies&policy=${map.alert_policies.independent.id}`,
    );
    await expect(
      page
        .getByRole("dialog")
        .getByText("B13b independent service and CPU", { exact: true })
        .first(),
    ).toBeVisible();
    await page.keyboard.press("Escape");
    await page.goto(`/alerts?tab=noise`);
    await expect(
      page
        .getByText("B13b Reports maintenance window", { exact: true })
        .first(),
    ).toBeVisible();
    await page.goto(`/alerts?tab=channels`);
    await expect(
      page.getByText("B13b 本地 TLS 收件器", { exact: true }).first(),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    expect(errors).toEqual([]);
  });
}

test("B13b real member never receives Reports episode snapshots", async ({
  page,
}, info) => {
  test.setTimeout(90_000);
  const credentials = JSON.parse(
    readFileSync(`${root}/private/reader-credentials.json`, "utf8"),
  );
  await createMfaLogin("enterprise")(
    page,
    "/login",
    credentials.username,
    credentials.password,
  );
  const result = await browserApi(page, "/api/v1/alerts/history?limit=200");
  expect(result.status()).toBe(200);
  const rows = (await result.json()).items;
  const owned = rows.filter((x: { policy_id: string }) =>
    Object.values(map.alert_policies).some(
      (p: unknown) => (p as { id: string }).id === x.policy_id,
    ),
  );
  expect(owned.length).toBeGreaterThan(0);
  for (const row of owned) {
    expect(row.resource_ids).toEqual([map.hosts.orders.host_id]);
    expect(JSON.stringify(row.state)).not.toContain(map.hosts.reports.host_id);
  }
  const forbidden = core.history.find((x: { resource_ids: string[] }) =>
    x.resource_ids.includes(map.hosts.reports.host_id),
  );
  const denial = await browserApi(
    page,
    `/api/v1/alerts/instances/${forbidden.id}`,
  );
  expect([403, 404]).toContain(denial.status());
  await page.goto(
    `/alerts?tab=history&resource_id=${map.hosts.orders.host_id}`,
  );
  await expect(
    page.getByRole("heading", { name: "告警中心", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "创建告警策略", exact: true }),
  ).toBeDisabled();
  await page.screenshot({
    path: info.outputPath("b13b-readonly-history.png"),
    animations: "disabled",
  });
});
