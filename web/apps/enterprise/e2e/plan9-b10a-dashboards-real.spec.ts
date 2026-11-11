import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

test.use({ trace: "off", video: "off", reducedMotion: "reduce" });
test("B10a deployed dashboards and current alert semantics", async ({
  page,
  context,
}, info) => {
  test.skip(
    process.env.ARGUS_PLAN9_B10A_E2E !== "1",
    "Requires the owned B10a runtime and a normal login",
  );
  test.setTimeout(180_000);
  const mapping = JSON.parse(
    readFileSync(process.env.ARGUS_PLAN9_B10A_RESOURCE_MAP!, "utf8"),
  );
  const session = JSON.parse(
    readFileSync(process.env.ARGUS_PLAN9_SESSION_PATH!, "utf8"),
  );
  await context.addCookies(
    session.cookies.map((cookie: object) => ({
      ...cookie,
      httpOnly: true,
      sameSite: "Lax",
    })),
  );
  await page.setViewportSize({ width: 1920, height: 1100 });
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const cluster = mapping.cluster.id;
  for (const [key, board] of Object.entries(mapping.dashboards) as [
    string,
    { id: string },
  ][]) {
    const executing = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname.endsWith(
          "/dashboards/" + board.id + "/execute",
        ) && response.request().method() === "POST",
    );
    await page.goto(
      "/dashboards/" +
        board.id +
        "?resource=" +
        cluster +
        "&resource_type=kubernetes_cluster",
    );
    const response = await executing;
    expect(response.status()).toBe(200);
    const value = await response.json();
    expect(value.panels.length).toBeGreaterThan(0);
    for (const panel of value.panels) {
      expect(panel.status).toBe("success");
      expect(
        panel.targets.some(
          (target: {
            data?: { values?: [number, string][]; value?: [number, string] }[];
          }) =>
            target.data?.some((series) =>
              [
                ...(series.values ?? []),
                ...(series.value ? [series.value] : []),
              ].some((point) => Number.isFinite(Number(point[1]))),
            ),
        ),
      ).toBe(true);
    }
    await expect(page.locator(".argus-dashboard-panel-content")).toHaveCount(
      value.panels.length,
    );
    await expect.poll(() => page.locator("canvas").count()).toBeGreaterThan(0);
    const boxes = await page.locator("canvas").evaluateAll((nodes) =>
      nodes.map((node) => ({
        width: node.getBoundingClientRect().width,
        height: node.getBoundingClientRect().height,
      })),
    );
    expect(boxes.every((box) => box.width > 300 && box.height > 150)).toBe(
      true,
    );
    await expect(page.getByText("暂无数据", { exact: true })).toHaveCount(0);
    await expect(page.getByText(/QUERY_BUDGET_EXCEEDED/)).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath(key + ".png"),
      fullPage: true,
    });
  }
  const response = await page.request.get("/api/v1/alerts/instances?limit=100");
  expect(response.status()).toBe(200);
  const current = await response.json();
  expect(
    current.items.every(
      (row: { lifecycle: string }) => row.lifecycle !== "normal",
    ),
  ).toBe(true);
  const summary = await page.request.get("/api/v1/alerts/summary");
  expect((await summary.json()).current_total).toBe(current.total);
  await page.goto("/alerts");
  await expect(page.locator("tbody tr")).toHaveCount(current.total);
  await page.screenshot({
    path: info.outputPath("current-alerts.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
});
