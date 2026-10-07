import { expect, test } from "@playwright/test";
import { dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";
import { selectTrigger } from "./helpers/select";

const login = createMfaLogin("enterprise"),
  d = dashboardsZh.dashboards;
test("real candidate absence resets All while search and request failures preserve the selected value", async ({
  page,
}, info) => {
  test.skip(
    process.env.ARGUS_PLANV2_E2E !== "1",
    "Requires PlanV2 Kubernetes environment",
  );
  test.setTimeout(180000);
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "dark");
  });
  await login(
    page,
    "/login",
    process.env.ARGUS_PLANV2_USERNAME!,
    process.env.ARGUS_PLANV2_PASSWORD!,
  );
  const board = process.env.ARGUS_PLANV2_DASHBOARD_ID!;
  const initial = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/dashboards/${board}/execute`) && r.status() === 200,
  );
  await page.goto(
    `/dashboards/${board}?resource=${process.env.ARGUS_PLANV2_CLUSTER_ID!}`,
  );
  expect((await (await initial).json()).variables.environment).toEqual({
    all: false,
    values: ["planv2-e2e"],
  });
  await page.getByRole("button", { name: "Environment", exact: true }).click();
  const choices = page.getByRole("dialog", {
    name: "Environment",
    exact: true,
  });
  const searched = page.waitForResponse(
    (r) =>
      r.url().endsWith("/dashboards/catalog/query") &&
      r.request().postDataJSON()?.search === "no-matching-environment",
  );
  await choices
    .getByRole("textbox", { name: "搜索候选值", exact: true })
    .fill("no-matching-environment");
  expect((await (await searched).json()).values).toEqual([]);
  await expect(
    choices.getByRole("radio", { name: "planv2-e2e", exact: true }),
  ).toBeChecked();
  await expect(
    choices.getByRole("checkbox", { name: "全部", exact: true }),
  ).not.toBeChecked();
  // Transport failure in this browser only; all data and subsequent execution
  // still use the real deployed Query service.
  await page.route("**/dashboards/catalog/query", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        code: "TELEMETRY_DEPENDENCY_UNAVAILABLE",
        message_key: "errors.telemetry.dependency_unavailable",
        request_id: "00000000-0000-4000-8000-000000000001",
        retryable: true,
      }),
    }),
  );
  await choices
    .getByRole("textbox", { name: "搜索候选值", exact: true })
    .fill("temporary-failure");
  await expect(choices.getByRole("alert")).toContainText("当前选值保持不变");
  await expect(
    choices.getByRole("radio", { name: "planv2-e2e", exact: true }),
  ).toBeChecked();
  await expect(
    choices.getByRole("checkbox", { name: "全部", exact: true }),
  ).not.toBeChecked();
  await choices.screenshot({
    path: info.outputPath("candidate-failure-retains-value.png"),
  });
  await page.unroute("**/dashboards/catalog/query");
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: d.time, exact: true }).click();
  const time = page.getByRole("dialog", { name: d.time, exact: true });
  await selectTrigger(time, d.timeKind).click();
  await page.getByRole("option", { name: d.absoluteTime, exact: true }).click();
  // Keep the existing valid one-hour interval, but move both endpoints before retention.
  for (const label of [d.timeFrom, d.timeTo]) {
    const year = time.getByRole("spinbutton", {
      name: `年, ${label}`,
      exact: true,
    });
    await year.focus();
    await year.pressSequentially("2000");
  }
  await time.getByRole("heading", { name: d.time, exact: true }).click();
  const absent = page.waitForResponse(
    (r) =>
      r.url().endsWith(`/dashboards/${board}/execute`) && r.status() === 200,
  );
  await time.getByRole("button", { name: d.apply, exact: true }).click();
  const empty = await (await absent).json();
  expect(empty.variables.environment).toEqual({ all: true, values: [] });
  expect(empty.variable_candidates.environment.complete).toBe(true);
  expect(empty.variable_candidates.environment.reset).toBe(true);
  expect(empty.resources.map((r: { id: string }) => r.id)).toEqual([
    process.env.ARGUS_PLANV2_CLUSTER_ID!,
  ]);
  await page.getByRole("button", { name: "Environment", exact: true }).click();
  await expect(
    choices.getByRole("checkbox", { name: "全部", exact: true }),
  ).toBeChecked();
  await expect(
    choices.getByRole("radio", { name: "planv2-e2e", exact: true }),
  ).toHaveCount(0);
  await choices.screenshot({ path: info.outputPath("proven-absence-all.png") });
  const published = await (
    await dashboardAPI(page, `/dashboards/${board}`)
  ).json();
  expect(published.revision.spec.variables[0].default).toEqual({
    all: false,
    values: ["planv2-e2e"],
  });
});
