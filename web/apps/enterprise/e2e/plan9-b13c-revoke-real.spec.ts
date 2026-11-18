import { createHash, randomUUID } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { expect, test, type Page, type WebSocket } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_PLAN9_B13C_REAL === "1";
const root = process.env.ARGUS_PLAN9_B13C_OUT ?? "";
const read = (name: string) =>
  JSON.parse(readFileSync(`${root}/${name}.json`, "utf8"));
test.skip(!enabled, "Requires the owned B13c member and native resources");
test.use({
  trace: "off",
  video: "off",
  ignoreHTTPSErrors: false,
  reducedMotion: "reduce",
});

const api = "/api/v1";
const stale = [401, 403, 409];
const nonce = () => `p9-b13c-${randomUUID()}`;

async function csrf(page: Page) {
  const result = await page.request.get(`${api}/enterprise/auth/session`);
  expect(result.ok()).toBe(true);
  return (await result.json()).csrf_token as string;
}

async function post(page: Page, path: string, token: string, data?: unknown) {
  return page.request.post(`${api}${path}`, {
    headers: {
      Origin: new URL(page.url()).origin,
      "X-CSRF-Token": token,
      "Idempotency-Key": nonce(),
    },
    data,
  });
}

test("B13c revokes an open dashboard, native query, CSV export and real terminal from the same member", async ({
  page,
  browser,
}, info) => {
  test.setTimeout(125_000);
  const map = read("resource-map");
  const dataset = read("export-baseline");
  const creds = read("private/reader-credentials");
  const retained = map.hosts.orders.host_id as string;
  const revoked = map.hosts.reports.host_id as string;
  const dashboard = map.dashboards["host.overview"].id as string;
  const dashboardPath = `/dashboards/${dashboard}?resource=${revoked}&resource_type=host&time_seconds=900`;
  const history = `/alerts?tab=history&policy_id=${dataset.policy_id}`;
  const filePath = `${api}/alerts/history/export?policy_id=${dataset.policy_id}`;
  const grantsPath = `/enterprise/data-authorizations/role/${map.reader.role_id}`;

  await page.context().addInitScript(() => {
    if (window.self !== window.top) return;
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  await createMfaLogin("enterprise")(
    page,
    "/login",
    creds.username,
    creds.password,
  );
  const oldToken = await csrf(page);
  const view = await page.context().newPage();
  const oldExecute = view.waitForResponse(
    (r) =>
      r.url().endsWith(`/dashboards/${dashboard}/execute`) &&
      r.request().method() === "POST" &&
      !r.request().postDataJSON()?.candidates_only,
  );
  await view.goto(dashboardPath);
  const execution = await oldExecute;
  expect(execution.ok()).toBe(true);
  expect(
    (await execution.json()).resources.map((row: { id: string }) => row.id),
  ).toEqual([revoked]);
  const frozen = execution.request().postDataJSON();
  await expect(
    view.locator(".argus-observation-chart canvas").first(),
  ).toBeVisible();

  const nativeQuery = {
    query: 'avg by (argus_resource_id) (system_cpu_utilization{state="idle"})',
    scope: { mode: "explicit", resources: [{ type: "host", id: revoked }] },
    time_range: { from: frozen.from, to: frozen.to },
  };
  const queryBaseline = await post(
    page,
    "/enterprise/metrics/query",
    oldToken,
    nativeQuery,
  );
  expect(queryBaseline.status()).toBe(200);
  expect((await queryBaseline.json()).data.result).not.toHaveLength(0);
  const downloads = await page.context().newPage();
  await downloads.goto(history);
  await expect(
    downloads.getByRole("button", { name: "导出记录", exact: true }),
  ).toBeEnabled();
  const downloaded = downloads.waitForEvent("download");
  await downloads
    .getByRole("button", { name: "导出记录", exact: true })
    .click();
  const csv = await downloaded;
  const before = readFileSync((await csv.path())!);
  expect(before.toString()).toContain(revoked);
  expect(before.toString()).toContain(retained);

  let ws: WebSocket | undefined;
  let sessionId = "";
  let output = "";
  let close = false;
  page.on("websocket", (socket) => {
    socket.on("framereceived", ({ payload }) => {
      try {
        const frame = JSON.parse(String(payload));
        if (frame.type === "server_ready") {
          ws = socket;
          sessionId = frame.session_id;
        }
        if (frame.type === "output" && socket === ws) output += frame.data;
      } catch {
        /* Unrelated socket data. */
      }
    });
    socket.on("close", () => {
      if (socket === ws) close = true;
    });
  });
  await page.goto(`/hosts/${revoked}`);
  await page.getByRole("tab", { name: "终端与会话", exact: true }).click();
  await page.getByLabel(/^协议/).click();
  await page.getByRole("option", { name: "OpenSSH", exact: true }).click();
  await page.getByLabel(/^登录账号/).click();
  await page.getByRole("option", { name: "plan9review", exact: true }).click();
  await page
    .getByLabel(/事由/)
    .fill("B13c scoped member revocation verification");
  await page.getByRole("button", { name: "建立会话", exact: true }).click();
  await expect.poll(() => sessionId, { timeout: 35_000 }).not.toBe("");
  const terminal = page.locator(".argus-terminal-dock .argus-terminal__xterm");
  await terminal.click();
  await page.keyboard.type(
    "printf 'B13c-begin\\n'; hostname; id -un; id -u; printf 'B13c-end\\n'",
  );
  await page.keyboard.press("Enter");
  await expect
    .poll(() => output, { timeout: 20_000 })
    .toMatch(
      /B13c-begin\r?\np9-b13c-[^\r\n]+-reports\r?\nplan9review\r?\n[1-9]\d*\r?\nB13c-end/,
    );

  const adminContext = await browser.newContext({
    baseURL: process.env.ARGUS_E2E_ENTERPRISE_ORIGIN,
    ignoreHTTPSErrors: false,
  });
  const admin = await adminContext.newPage();
  try {
    await plan7Login(
      admin,
      "/login",
      process.env.ARGUS_PLAN9_USERNAME ?? "kakj",
      process.env.ARGUS_PLAN9_PASSWORD!,
    );
    const adminToken = await csrf(admin);
    const grants = await (
      await admin.request.get(`${api}${grantsPath}?resource_type=host`)
    ).json();
    expect(
      grants.items.some(
        (x: { resource_id: string; direct: boolean }) =>
          x.resource_id === revoked && x.direct,
      ),
    ).toBe(true);
    const started = Date.now();
    const removal = await post(admin, grantsPath, adminToken, {
      resource_type: "host",
      resource_ids: [revoked],
      remove: true,
      expected_version: grants.authorization_version,
    });
    expect(removal.status()).toBe(204);

    const oldQuery = await post(
      downloads,
      "/enterprise/metrics/query",
      oldToken,
      nativeQuery,
    );
    const oldDashboard = await post(
      downloads,
      `/dashboards/${dashboard}/execute`,
      oldToken,
      frozen,
    );
    const oldDownload = await downloads.request.get(filePath);
    for (const result of [oldQuery, oldDashboard, oldDownload])
      expect(stale).toContain(result.status());
    const oldErrors = await Promise.all(
      [oldQuery, oldDashboard, oldDownload].map(async (result) => ({
        status: result.status(),
        code: (await result.json()).code,
      })),
    );

    await expect
      .poll(
        async () => {
          const details = await admin.request.get(
            `${api}/enterprise/remote-access-sessions/${sessionId}`,
          );
          return details.ok()
            ? (await details.json()).status
            : details.status();
        },
        { timeout: 35_000 },
      )
      .toBe("invalidated");
    await expect.poll(() => close, { timeout: 15_000 }).toBe(true);
    expect([403, 409, 410]).toContain(
      (
        await post(
          admin,
          `/enterprise/remote-access-sessions/${sessionId}/tickets`,
          adminToken,
        )
      ).status(),
    );
    const elapsedMs = Date.now() - started;
    // Refresh the already open view through normal client code. Automatic
    // refresh may have invalidated it by the time the session closure arrives.
    if (!/\/login/.test(view.url())) {
      const refresh = view.getByRole("button", { name: "刷新", exact: true });
      if (await refresh.isVisible()) await refresh.click();
    }
    await expect(view).toHaveURL(/\/login/, { timeout: 40_000 });
    await expect(view.locator(".argus-observation-chart canvas")).toHaveCount(
      0,
    );

    await createMfaLogin("enterprise")(
      downloads,
      "/login",
      creds.username,
      creds.password,
    );
    const newToken = await csrf(downloads);
    expect([403, 404]).toContain(
      (
        await post(
          downloads,
          "/enterprise/metrics/query",
          newToken,
          nativeQuery,
        )
      ).status(),
    );
    expect([403, 404]).toContain(
      (
        await post(
          downloads,
          `/dashboards/${dashboard}/execute`,
          newToken,
          frozen,
        )
      ).status(),
    );
    const narrowed = await downloads.request.get(filePath);
    expect(narrowed.status()).toBe(200);
    const content = (await narrowed.json()).content as string;
    expect(content).toContain(retained);
    expect(content).not.toContain(revoked);
    expect(content).not.toContain(dataset.alert_ids_by_host[revoked]);
    const oldHostRow = await downloads.request.get(
      `${api}/enterprise/hosts/${revoked}`,
    );
    expect([403, 404]).toContain(oldHostRow.status());
    const control = await post(
      downloads,
      "/enterprise/metrics/query",
      newToken,
      {
        ...nativeQuery,
        scope: {
          mode: "explicit",
          resources: [{ type: "host", id: retained }],
        },
      },
    );
    expect(control.status()).toBe(200);
    expect((await control.json()).data.result).not.toHaveLength(0);
    await downloads.goto(history);
    await expect(
      downloads.getByText("B13c-Orders-Host", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      downloads.getByText("B13c-Reports-Host", { exact: true }),
    ).toHaveCount(0);
    await downloads.screenshot({
      path: info.outputPath("scoped-history-after-revocation.png"),
      animations: "disabled",
    });
    writeFileSync(
      `${root}/revocation-acceptance.json`,
      JSON.stringify(
        {
          passed: true,
          session_id: sessionId,
          socket_closed: close,
          elapsed_ms: elapsedMs,
          old_requests: oldErrors,
          real_unprivileged_pty: true,
          reopened_view_excludes_revoked_host: true,
          csv_sha256_before: createHash("sha256").update(before).digest("hex"),
          csv_bytes_before: before.length,
          csv_after_only_retained_host: true,
          native_retained_query_ok: true,
        },
        null,
        2,
      ),
    );
  } finally {
    if (sessionId && !close) {
      const token = await csrf(admin).catch(() => "");
      if (token)
        await post(
          admin,
          `/enterprise/remote-access-sessions/${sessionId}/terminate`,
          token,
          { reason: "B13c test session cleanup" },
        );
    }
    await adminContext.close();
  }
});
