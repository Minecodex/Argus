import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { dashboardsEn, dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI, objectGrant } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
const editorLogin = createMfaLogin("enterprise", "EDITOR");
const board = process.env.ARGUS_PLANV2_DEPTH_ID!;
const cluster = process.env.ARGUS_PLANV2_CLUSTER_ID!;
const host = process.env.ARGUS_PLANV2_HOST_ID!;
const oldSource = process.env.ARGUS_PLANV2_OLD_SOURCE_ID!;
const newSource = process.env.ARGUS_PLANV2_NEW_SOURCE_ID!;
test.use({
  actionTimeout: 15000,
  navigationTimeout: 30000,
  viewport: { width: 1440, height: 1100 },
});
test.beforeEach(() =>
  test.skip(process.env.ARGUS_PLANV2_E2E !== "1", "Requires deployed PlanV2"),
);

for (const [locale, theme] of [
  ["zh-CN", "light"],
  ["en-US", "dark"],
]) {
  const d =
    locale === "zh-CN" ? dashboardsZh.dashboards : dashboardsEn.dashboards;
  const tx = (zh: string, en: string) => (locale === "zh-CN" ? zh : en);
  test(`real trace, source history and log context ${locale} ${theme}`, async ({
    page,
  }, info) => {
    test.setTimeout(180000);
    await page.addInitScript(
      ({ locale, theme }) => {
        localStorage.setItem("argus.locale", locale!);
        localStorage.setItem("argus.theme", theme!);
      },
      { locale, theme },
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    await page.goto(`/dashboards/${board}?resource=${cluster}`);
    const panel = page.locator('[data-panel-id="linked"]');
    await panel
      .getByRole("button", {
        name: tx("打开详情", "Open details"),
        exact: true,
      })
      .first()
      .click();
    const dialog = page.getByRole("dialog", {
      name: `Cross-resource trace · ${d.details}`,
      exact: true,
    });
    const drill = async (name: string) => {
      const response = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/dashboards/${board}/drilldown`) &&
          r.status() === 200,
      );
      await dialog.getByRole("button", { name, exact: true }).click();
      return (await response).json();
    };
    const detail = await drill(d.drillKinds.trace_details);
    expect(detail.result.data.queryTraceGraph.spans).toHaveLength(2);
    await dialog
      .getByRole("button", { name: "p2-frontend / GET /checkout", exact: true })
      .click();
    const span = dialog.getByRole("region", {
      name: tx("Span 详情", "Span details"),
    });
    await expect(span).toContainText("second-batch");
    await expect(span).toContainText("p2-checkpoint");
    await expect(span).toContainText("async-followup");
    await span
      .getByRole("button", {
        name: tx("关联查询", "Related queries"),
        exact: true,
      })
      .click();
    const full = await drill(d.fullTrace);
    expect(full.result.data.queryTraceGraph.spans).toHaveLength(4);
    for (const source of [oldSource, newSource]) {
      await expect(
        dialog.locator(`tr[data-source-id="${source}"]`),
      ).toHaveCount(1);
    }
    await dialog
      .locator(`tr[data-source-id="${oldSource}"]`)
      .getByRole("button", { name: "p2-backend / POST /orders", exact: true })
      .click();
    await expect(span).toContainText(oldSource);
    await span
      .getByRole("button", {
        name: tx("关联查询", "Related queries"),
        exact: true,
      })
      .click();
    await dialog.screenshot({
      path: info.outputPath("source-separated-waterfall.png"),
    });
    const logs = await drill(d.drillKinds.span_logs);
    expect(JSON.stringify(logs.result.data)).toContain("backend old anchor");
    expect(JSON.stringify(logs.result.data)).not.toContain("backend new");
    await expect(dialog.getByRole("article")).toHaveCount(3);
    const anchor = dialog
      .getByRole("article")
      .filter({ hasText: "backend old anchor" });
    await anchor.locator("summary").click();
    await expect(anchor).toContainText(oldSource);
    await anchor
      .getByRole("button", {
        name: tx("关联查询", "Related queries"),
        exact: true,
      })
      .click();
    const context = await drill(d.drillKinds.log_context);
    expect(JSON.stringify(context.result.data)).not.toContain("backend new");
    await expect(dialog.getByRole("article")).toHaveCount(3);
    await dialog
      .getByLabel(tx("搜索当前结果", "Search current results"))
      .fill("old anchor");
    await expect(dialog.getByRole("article")).toHaveCount(1);
    await dialog
      .getByLabel(tx("搜索当前结果", "Search current results"))
      .fill("");
    const fieldPicker = dialog
      .locator("summary")
      .filter({ hasText: tx("显示字段", "Visible fields") });
    await fieldPicker.focus();
    await page.keyboard.press("Enter");
    await expect(
      dialog.getByRole("checkbox", { name: "source_id", exact: true }),
    ).toBeVisible();
    await dialog
      .getByRole("checkbox", { name: "source_id", exact: true })
      .check();
    await dialog.screenshot({
      path: info.outputPath("log-context-fields.png"),
    });
    await accessible(page);
    await page.keyboard.press("Escape");

    await page.goto(`/dashboards/${board}?resource=${host}`);
    await panel
      .getByRole("button", {
        name: `Cross-resource trace ${d.resolvedSources}`,
        exact: true,
      })
      .click();
    const sources = page.getByRole("dialog", {
      name: `Cross-resource trace · ${d.resolvedSources}`,
      exact: true,
    });
    await expect(
      sources.getByRole("region", { name: oldSource }),
    ).toContainText(d.historicalInstallation);
    await expect(
      sources.getByRole("region", { name: newSource }),
    ).toContainText(d.currentInstallation);
    await sources.screenshot({
      path: info.outputPath("installation-history.png"),
    });
    await page.keyboard.press("Escape");
    await panel
      .getByRole("button", {
        name: "Cross-resource trace Collection source",
        exact: true,
      })
      .click();
    const chooser = page.getByRole("dialog", {
      name: "Cross-resource trace Collection source",
      exact: true,
    });
    await chooser
      .getByRole("checkbox", { name: tx("全部", "All"), exact: true })
      .uncheck();
    await chooser.locator(`input[value="${oldSource}"]`).check();
    const changed = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${board}/execute`) && r.status() === 200,
    );
    await chooser
      .getByRole("button", { name: tx("应用", "Apply"), exact: true })
      .click();
    const result = await (await changed).json();
    expect(result.local_values.linked.origin.values).toEqual([oldSource]);
    expect(JSON.stringify(result.panels[0].targets[0].data)).not.toContain(
      newSource,
    );
    await accessible(page);
  });

  test(`native vendors keep same names separate and map only referenced variables ${locale}`, async ({
    page,
  }) => {
    test.setTimeout(120000);
    await page.addInitScript(
      ({ locale, theme }) => {
        localStorage.setItem("argus.locale", locale!);
        localStorage.setItem("argus.theme", theme!);
      },
      { locale, theme },
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    const id = process.env.ARGUS_PLANV2_NATIVE_MAPPING_ID!;
    const executed = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${id}/execute`) && r.status() === 200,
    );
    await page.goto(`/dashboards/${id}?resource=${cluster}`);
    const first = await (await executed).json();
    const ids = [];
    for (const vendor of ["skywalking", "jaeger"]) {
      await expect(page.locator(`[data-panel-id="${vendor}"]`)).toContainText(
        "p2-shared-service",
      );
      const panel = first.panels.find((p: { id: string }) => p.id === vendor);
      expect(
        panel.sources.every((s: { type: string }) => s.type === vendor),
      ).toBe(true);
      expect(JSON.stringify(panel.targets)).toContain("received_entry_spans");
      ids.push(...panel.sources.map((s: { id: string }) => s.id));
    }
    expect(new Set(ids).size).toBe(ids.length);
    const independent = await page
      .locator('[data-panel-id="independent"]')
      .innerText();
    await page.getByRole("button", { name: "Pool", exact: true }).click();
    const chooser = page.getByRole("dialog", { name: "Pool", exact: true });
    for (const checkbox of await chooser.getByRole("checkbox").all())
      await checkbox.uncheck();
    await chooser.getByRole("radio", { name: "green", exact: true }).check();
    const changed = page.waitForResponse(
      (r) =>
        r.url().endsWith(`/dashboards/${id}/execute`) && r.status() === 200,
    );
    await chooser
      .getByRole("button", { name: tx("应用", "Apply"), exact: true })
      .click();
    const response = await changed;
    expect(response.request().postDataJSON().panel_ids.sort()).toEqual([
      "jaeger",
      "skywalking",
    ]);
    const next = await response.json();
    expect(
      next.panels.every((p: { status: string }) => p.status === "no_data"),
    ).toBe(true);
    await expect(page.locator('[data-panel-id="independent"]')).toHaveText(
      independent,
      { useInnerText: true },
    );
  });
}

test("explicit expansion uses current grants and rejects inherited revoked resources", async ({
  page,
  browser,
}) => {
  test.setTimeout(240000);
  await login(
    page,
    "/login",
    process.env.ARGUS_PLANV2_USERNAME!,
    process.env.ARGUS_PLANV2_PASSWORD!,
  );
  await page.goto("/dashboards");
  const subject = process.env.ARGUS_PLANV2_EDITOR_ID!;
  await objectGrant(page, subject, "dashboard", [board], false);
  await objectGrant(page, subject, "kubernetes_cluster", [cluster], false);
  await objectGrant(page, subject, "host", [host], true);
  const peerContext = await browser.newContext({
    baseURL: process.env.ARGUS_E2E_ENTERPRISE_ORIGIN,
    ignoreHTTPSErrors: true,
  });
  try {
    const peer = await peerContext.newPage();
    const session = async () => {
      await peer.context().clearCookies();
      await editorLogin(
        peer,
        "/login",
        process.env.ARGUS_PLANV2_EDITOR_USERNAME!,
        process.env.ARGUS_PLANV2_EDITOR_PASSWORD!,
      );
      await peer.goto(`/dashboards/${board}?resource=${cluster}`);
      await expect(peer.locator('[data-panel-id="linked"]')).toBeVisible();
    };
    await session();
    const spec = (
      await (await dashboardAPI(peer, `/dashboards/${board}`)).json()
    ).revision.spec;
    const drilldowns = spec.panels[0].drilldowns as Array<{
      id: string;
      kind: string;
      origin_query_ref: string;
      detail_query_ref: string;
    }>;
    const details = drilldowns.find(
      (d) => d.kind === "trace_details" && d.origin_query_ref === "main",
    )!;
    const full = drilldowns.find(
      (d) =>
        d.kind === "full_trace" &&
        d.origin_query_ref === details.detail_query_ref,
    )!;
    const log = drilldowns.find(
      (d) =>
        d.kind === "span_logs" && d.origin_query_ref === full.detail_query_ref,
    )!;
    const expand = async () => {
      const execution = await (
        await dashboardAPI(peer, `/dashboards/${board}/execute`, "POST", {
          resource_ids: [cluster],
        })
      ).json();
      const source = execution.panels[0].sources[0].id;
      const values = {
        trace_id: "aabbccddeeff00112233445566778899",
        source_id: source,
        resource_id: cluster,
      };
      const detail = await (
        await dashboardAPI(peer, `/dashboards/${board}/drilldown`, "POST", {
          context_token: execution.context_token,
          panel_id: "linked",
          drilldown_id: details.id,
          values,
          expand_authorized_resources: false,
        })
      ).json();
      return (
        await dashboardAPI(peer, `/dashboards/${board}/drilldown`, "POST", {
          context_token: detail.context_token,
          panel_id: "linked",
          drilldown_id: full.id,
          values,
          expand_authorized_resources: true,
        })
      ).json();
    };
    expect((await expand()).result.data.queryTraceGraph.spans).toHaveLength(2);
    await objectGrant(page, subject, "host", [host], false);
    await session();
    const expanded = await expand();
    expect(expanded.result.data.queryTraceGraph.spans).toHaveLength(4);
    await objectGrant(page, subject, "host", [host], true);
    await session();
    await dashboardAPI(
      peer,
      `/dashboards/${board}/drilldown`,
      "POST",
      {
        context_token: expanded.context_token,
        panel_id: "linked",
        drilldown_id: log.id,
        expand_authorized_resources: false,
        values: {
          trace_id: "aabbccddeeff00112233445566778899",
          span_id: "2222222222222222",
          resource_id: host,
          source_id: oldSource,
        },
      },
      403,
    );
    expect((await expand()).result.data.queryTraceGraph.spans).toHaveLength(2);
  } finally {
    await peerContext.close();
  }
});

async function accessible(page: Page) {
  const result = await new AxeBuilder({ page }).analyze();
  expect(
    result.violations.filter(
      (v) => v.impact === "critical" || v.impact === "serious",
    ),
  ).toEqual([]);
}
