import { readFileSync, writeFileSync } from "node:fs";
import { expect, test, type Page } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { plan7Login } from "./helpers/plan7-login";
import { selectRowAction } from "./helpers/action-menu";

const enabled = process.env.ARGUS_PLAN9_B13C_REAL === "1";
const stage = process.env.ARGUS_PLAN9_B13C_STAGE;
const root = process.env.ARGUS_PLAN9_B13C_OUT ?? "";
const read = (name: string) =>
  JSON.parse(readFileSync(`${root}/${name}.json`, "utf8"));
test.skip(
  !enabled,
  "Requires the real B13c target, dashboards and normal login",
);
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});

async function preferences(
  page: Page,
  width = 1440,
  english = false,
  theme = "light",
) {
  await page.setViewportSize({ width, height: 1040 });
  await page.addInitScript(
    ({ english, theme }) => {
      if (window.self !== window.top) return;
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", theme);
    },
    { english, theme },
  );
}

async function admin(page: Page) {
  await plan7Login(
    page,
    "/login",
    process.env.ARGUS_PLAN9_USERNAME ?? "kakj",
    process.env.ARGUS_PLAN9_PASSWORD!,
  );
}

async function targets(page: Page, english = false) {
  await page.goto("/monitoring");
  await page
    .getByRole("tab", {
      name: english ? "Monitoring targets" : "监控目标",
      exact: true,
    })
    .click();
}

test("B13c portal archive preview rejects the live target dependencies", async ({
  page,
}, info) => {
  test.skip(
    stage !== "blocked",
    "Run while the target still has a collection and bindings",
  );
  const row = read("resource-map").targets.memcached;
  await preferences(page);
  await admin(page);
  await targets(page);
  const target = page
    .getByRole("row")
    .filter({ has: page.getByRole("button", { name: row.name, exact: true }) });
  const preview = page.waitForResponse(
    (r) =>
      r.url().endsWith("/monitoring/commands/preview") &&
      r.request().method() === "POST",
  );
  await selectRowAction(target, "归档目标");
  const result = await preview;
  expect(result.status()).toBe(409);
  const failure = await result.json();
  expect(failure.code).toBe("MONITORING_DEPENDENCIES_EXIST");
  await expect(page.getByRole("alert")).toContainText("此对象仍有关联");
  await expect(target).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("live-dependencies-block-archive.png"),
    animations: "disabled",
  });
  writeFileSync(
    `${root}/portal-archive-dependencies.json`,
    JSON.stringify(
      { passed: true, code: failure.code, target_id: row.target_id },
      null,
      2,
    ),
  );
});

test("B13c portal archives the target through preview and confirmed execution", async ({
  page,
}, info) => {
  test.skip(
    stage !== "archive",
    "Run after the batch-owned collection and bindings are archived",
  );
  const map = read("resource-map");
  expect(map.archive_probe.dependencies_retired).toBe(true);
  await preferences(page, 1440, true, "dark");
  await admin(page);
  await targets(page, true);
  const target = page
    .getByRole("row")
    .filter({
      has: page.getByRole("button", {
        name: map.targets.memcached.name,
        exact: true,
      }),
    });
  const previewing = page.waitForResponse(
    (r) =>
      r.url().endsWith("/monitoring/commands/preview") &&
      r.request().method() === "POST",
  );
  await selectRowAction(target, "Archive target");
  const preview = await previewing;
  expect(preview.status()).toBe(201);
  const action = await preview.json();
  expect(action.action_type).toBe("monitoring.target.archive");
  expect(action.preview.id).toBe(map.targets.memcached.target_id);
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(map.targets.memcached.name);
  await expect(dialog).toContainText(
    "Previously collected data remains available",
  );
  await page.screenshot({
    path: info.outputPath("target-archive-preview.png"),
    animations: "disabled",
  });
  const committing = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/pending-actions/${action.action_ref}/confirm`) &&
      r.request().method() === "POST",
  );
  await dialog
    .getByRole("button", { name: "Confirm changes", exact: true })
    .click();
  const committed = await committing;
  expect(committed.ok()).toBe(true);
  const value = await committed.json();
  expect(value.execution.execution_id).toBeTruthy();
  await expect(dialog).toHaveCount(0, { timeout: 40_000 });
  await expect(target).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: map.targets.redis.name, exact: true }),
  ).toBeVisible();
  writeFileSync(
    `${root}/portal-target-archive.json`,
    JSON.stringify(
      {
        passed: true,
        action_ref: action.action_ref,
        execution_id: value.execution.execution_id,
        absent_from_active_directory: true,
      },
      null,
      2,
    ),
  );
});

for (const [width, english, theme] of [
  [1280, false, "light"],
  [1440, true, "dark"],
  [1920, false, "dark"],
] as const) {
  test(`B13c retained control and archived native history ${width} ${english ? "en" : "zh"} ${theme}`, async ({
    page,
  }, info) => {
    test.skip(stage !== "review", "Requires the completed lifecycle run");
    test.setTimeout(90_000);
    const map = read("resource-map");
    await preferences(page, width, english, theme);
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await admin(page);
    await targets(page, english);
    await expect(
      page.getByRole("button", { name: map.targets.redis.name, exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", {
        name: map.targets.memcached.name,
        exact: true,
      }),
    ).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath("retained-target.png"),
      animations: "disabled",
    });
    const range = map.archive_probe.input;
    const board = map.dashboards.memcached.id;
    const execution = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${board}/execute`) &&
        r.request().method() === "POST" &&
        !r.request().postDataJSON()?.candidates_only,
    );
    await page.goto(
      `/dashboards/${board}?${new URLSearchParams({
        resource: map.targets.memcached.target_id,
        resource_type: "monitoring_target",
        time_from: range.from,
        time_to: range.to,
      })}`,
    );
    const history = await execution;
    expect(history.status()).toBe(200);
    const value = await history.json();
    expect(value.panels).toHaveLength(9);
    expect(
      value.panels.every(
        (item: { status: string }) => item.status === "success",
      ),
    ).toBe(true);
    await expect(
      page.locator(".argus-observation-chart canvas").first(),
    ).toBeVisible();
    await page.screenshot({
      path: info.outputPath("archived-native-history.png"),
      animations: "disabled",
    });
    await page.goto(
      `/dashboards/${map.personal_dashboard.id}?resource=${map.targets.redis.target_id}&resource_type=monitoring_target&time_seconds=900`,
    );
    await expect(
      page.getByText("B13c Redis 运行时长 · 轮换后 / Uptime after rotation", {
        exact: true,
      }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
    ).toBe(true);
    expect(errors).toEqual([]);
  });
}

test("B13c retained member has read access to the Redis target and no archive action", async ({
  page,
}) => {
  test.skip(
    stage !== "review",
    "Requires the retained scoped user after revocation",
  );
  const map = read("resource-map");
  const creds = read("private/reader-credentials");
  await preferences(page, 1280, true, "dark");
  await createMfaLogin("enterprise")(
    page,
    "/login",
    creds.username,
    creds.password,
  );
  await targets(page, true);
  await expect(
    page.getByRole("button", { name: map.targets.redis.name, exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: `${map.targets.redis.name} · More actions`,
    }),
  ).toHaveCount(0);
});
