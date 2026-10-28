import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const path = process.env.ARGUS_AUDIT_BROWSER_FIXTURE;
const fixture = path
  ? (JSON.parse(readFileSync(path, "utf8")) as {
      api: string;
      token: string;
      csrf: string;
      hostId: string;
      failedRef: string;
      commands: string[];
    })
  : undefined;
test.skip(
  !fixture,
  "Requires the owned PostgreSQL and real HTTP audit acceptance fixture",
);
for (const variant of [
  { english: false, theme: "light" },
  { english: false, theme: "dark" },
  { english: true, theme: "light" },
  { english: true, theme: "dark" },
]) {
  test(`real audit, resource progress and single-use result ${variant.english ? "en" : "zh"} ${variant.theme}`, async ({
    page,
    context,
  }, info) => {
    const { english, theme } = variant;
    page.on("requestfailed", (request) =>
      console.log(
        "request failed",
        new URL(request.url()).pathname,
        request.failure()?.errorText,
      ),
    );
    page.on("response", (response) => {
      if (response.status() >= 400)
        console.log(
          "HTTP",
          response.status(),
          new URL(response.url()).pathname,
        );
    });
    await context.addCookies([
      {
        name: "argus_enterprise_session",
        value: fixture!.token,
        domain: "127.0.0.1",
        path: "/",
        httpOnly: true,
        sameSite: "Strict",
      },
      {
        name: "argus_enterprise_csrf",
        value: fixture!.csrf,
        domain: "127.0.0.1",
        path: "/",
        sameSite: "Strict",
      },
    ]);
    await page.addInitScript(
      ({ english, theme }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", theme);
      },
      { english, theme },
    );
    await page.goto("/settings/audit");
    await expect(
      page.getByRole("heading", {
        name: english ? "Enterprise Audit" : "企业审计",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /任务记录|Task History/ }),
    ).toHaveCount(0);
    await expect(page.getByRole("table")).toContainText(
      english ? "Disconnect Telemetry tunnel" : "断开遥测隧道",
    );
    await page
      .getByRole("button", {
        name: english ? "Operation failed" : "操作执行失败",
        exact: true,
      })
      .click();
    const detail = page.getByRole("dialog");
    await expect(detail.locator(".argus-operation-panel")).toContainText(
      english ? "Failed" : "执行失败",
    );
    await expect(detail.locator(".argus-timeline__item")).toHaveCount(3);
    await detail.screenshot({
      path: info.outputPath("audit-final-result.png"),
      animations: "disabled",
    });
    await page.keyboard.press("Escape");
    await page.goto(`/hosts/${fixture!.hostId}?tab=activity`);
    const panels = page.locator(".argus-operation-panel");
    await expect(panels.first()).toBeVisible();
    // The resource list uses the same real GET endpoint and exposes only metadata.
    const data = await page.evaluate(
      async ({ api, hostId }) =>
        (
          await fetch(`${api}/enterprise/executions?resource_id=${hostId}`, {
            credentials: "include",
          })
        ).json(),
      { api: fixture!.api, hostId: fixture!.hostId },
    );
    const executionId = data.items.find(
      (item: { one_time_result_state: string }) =>
        item.one_time_result_state === "available",
    ).execution_id;
    const claim = page
      .getByRole("button", {
        name: english ? "Retrieve installation command" : "领取安装命令",
        exact: true,
      })
      .first();
    await claim.click();
    await expect(
      page.getByText("echo audit-acceptance-claim-canary", { exact: true }),
    ).toBeVisible();
    const repeated = await page.evaluate(
      async ({ api, csrf, id }) =>
        (
          await fetch(`${api}/enterprise/executions/${id}/one-time-result`, {
            method: "POST",
            credentials: "include",
            headers: {
              "X-CSRF-Token": csrf,
              "Idempotency-Key": crypto.randomUUID(),
            },
          })
        ).status,
      { api: fixture!.api, csrf: fixture!.csrf, id: executionId },
    );
    expect(repeated).toBe(409);
    await expect(
      page.getByText(
        english
          ? "Complete installation on the target machine"
          : "在目标机器完成安装",
        { exact: true },
      ),
    ).toBeVisible();
    expect(
      await page.evaluate(() =>
        JSON.stringify({ localStorage, sessionStorage, url: location.href }),
      ),
    ).not.toContain("audit-acceptance-claim-canary");
    await page.screenshot({
      path: info.outputPath("resource-command-result.png"),
      animations: "disabled",
    });
    const accessibility = await new AxeBuilder({ page }).analyze();
    expect(
      accessibility.violations.filter(
        (v) => v.impact === "serious" || v.impact === "critical",
      ),
    ).toEqual([]);
    await page
      .getByRole("button", {
        name: english ? "Close and clear command" : "关闭并清除命令",
        exact: true,
      })
      .click();
    await expect(
      page.getByText("echo audit-acceptance-claim-canary", { exact: true }),
    ).toHaveCount(0);
    await page.reload();
    await expect(
      page.getByText("echo audit-acceptance-claim-canary", { exact: true }),
    ).toHaveCount(0);
    await expect(
      page
        .getByText(english ? "Command retrieved" : "命令已领取", {
          exact: true,
        })
        .first(),
    ).toBeVisible();
  });
}

test("real browser submits an operation through the queue and follows its audit outcome", async ({
  page,
  context,
}) => {
  await context.addCookies([
    {
      name: "argus_enterprise_session",
      value: fixture!.token,
      domain: "127.0.0.1",
      path: "/",
      httpOnly: true,
      sameSite: "Strict",
    },
    {
      name: "argus_enterprise_csrf",
      value: fixture!.csrf,
      domain: "127.0.0.1",
      path: "/",
      sameSite: "Strict",
    },
  ]);
  await page.addInitScript(() => localStorage.setItem("argus.locale", "zh-CN"));
  await page.goto(`/hosts/${fixture!.hostId}?tab=activity`);
  await expect(page.locator(".argus-operation-panel").first()).toBeVisible();
  const submitted = await page.evaluate(async ({ api, csrf, hostId }) => {
    const call = async (path: string, body?: object) => {
      const response = await fetch(`${api}${path}`, {
        credentials: "include",
        ...(body !== undefined
          ? { method: "POST", body: JSON.stringify(body) }
          : {}),
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
          "Idempotency-Key": crypto.randomUUID(),
        },
      });
      if (!response.ok) throw new Error(`Operation HTTP ${response.status}`);
      return response.json();
    };
    const host = await call(`/enterprise/hosts/${hostId}`);
    const preview = await call(
      `/enterprise/hosts/${hostId}/actions/preview-update`,
      {
        name: "audit-browser-queue-target",
        expected_version: host.resource_version,
      },
    );
    const response = await fetch(
      `${api}/enterprise/pending-actions/${preview.action_ref}/confirm`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          "X-CSRF-Token": csrf,
          "Idempotency-Key": crypto.randomUUID(),
        },
      },
    );
    if (!response.ok) throw new Error(`Confirm HTTP ${response.status}`);
    return { ...(await response.json()), actionRef: preview.action_ref };
  }, fixture!);
  expect(submitted.execution.status).toBe("pending");
  await page.goto(
    `/settings/audit?operation_ref=${encodeURIComponent(submitted.actionRef)}`,
  );
  await expect(
    page.getByText("当前仅显示这次操作的关联审计事件。", { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("table")).toContainText("执行成功", {
    timeout: 20000,
  });
  await expect(page.getByRole("table")).toContainText(
    "audit-browser-queue-target",
  );
  await page.getByRole("button", { name: "操作执行成功", exact: true }).click();
  await expect(
    page.getByRole("dialog").locator(".argus-timeline__item"),
  ).toHaveCount(3);
});
