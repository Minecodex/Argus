import { expect, test, type Page } from "@playwright/test";
import { dashboardsEn, dashboardsZh } from "../src/i18n/dashboards";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
test.use({ viewport: { width: 1440, height: 1100 } });
for (const english of [false, true]) {
  test(`real candidate pagination, cascades, local scope and explicit mapping ${english ? "en dark" : "zh light"}`, async ({
    page,
    context,
  }, info) => {
    test.skip(
      process.env.ARGUS_PLANV2_E2E !== "1",
      "Requires PlanV2 Kubernetes environment",
    );
    test.setTimeout(180000);
    const d = (english ? dashboardsEn : dashboardsZh).dashboards;
    await context.addInitScript(
      ({ english }) => {
        localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
        localStorage.setItem("argus.theme", english ? "dark" : "light");
      },
      { english },
    );
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    const id = process.env.ARGUS_PLANV2_PARAMETERS_ID!;
    const cluster = process.env.ARGUS_PLANV2_CLUSTER_ID!;
    const loaded = execution(page, id);
    await page.goto(`/dashboards/${id}?resource=${cluster}`);
    const initial = await (await loaded).json();
    expect(initial.variable_candidates.member.values).toHaveLength(200);
    expect(initial.variable_candidates.member.complete).toBe(false);
    expect(initial.variable_candidates.member.selected_exists["blue-220"]).toBe(
      true,
    );
    expect(initial.variables.member).toEqual({
      all: false,
      values: ["blue-220"],
    });
    expect(
      JSON.stringify(
        initial.panels.find((p: { id: string }) => p.id === "mapped_logs"),
      ),
    ).toContain("planv2 selection blue");
    const unchanged = await page
      .locator('[data-panel-id="independent"]')
      .innerText();

    await page.getByRole("button", { name: "Member", exact: true }).click();
    const member = page.getByRole("dialog", { name: "Member", exact: true });
    await expect(
      member.getByRole("checkbox", { name: "blue-099", exact: true }),
    ).toBeVisible();
    await expect(
      member.getByRole("checkbox", { name: "blue-220", exact: true }),
    ).toBeChecked();
    await member
      .getByRole("button", {
        name: english ? "More candidates" : "更多候选",
        exact: true,
      })
      .click();
    await expect(
      member.getByRole("checkbox", { name: "blue-199", exact: true }),
    ).toBeVisible();
    await member
      .getByRole("button", {
        name: english ? "More candidates" : "更多候选",
        exact: true,
      })
      .click();
    await expect(
      member.getByRole("checkbox", { name: "blue-229", exact: true }),
    ).toBeVisible();
    await expect(
      member.getByRole("button", {
        name: english ? "More candidates" : "更多候选",
        exact: true,
      }),
    ).toHaveCount(0);
    await member
      .getByRole("checkbox", { name: "blue-220", exact: true })
      .uncheck();
    await member
      .getByRole("checkbox", { name: "blue-229", exact: true })
      .check();
    const selected = execution(page, id);
    await member.getByRole("button", { name: d.apply, exact: true }).click();
    const selectionResponse = await selected;
    expect(selectionResponse.request().postDataJSON().panel_ids).toEqual([
      "dependent",
    ]);
    const selection = await selectionResponse.json();
    expect(selection.variables.member.values).toEqual(["blue-229"]);
    expect(selection.from).toBe(initial.from);
    expect(selection.to).toBe(initial.to);
    expect(
      await page.locator('[data-panel-id="independent"]').innerText(),
    ).toBe(unchanged);

    const catalog = page.waitForResponse(
      (r) =>
        r.url().includes("/dashboards/catalog/query") && r.status() === 200,
    );
    await page
      .getByRole("button", { name: "local Instance", exact: true })
      .click();
    const lookup = await catalog;
    // The request must use the current Panel's effective resource set.
    expect(lookup.request().postDataJSON().resource_ids).toEqual([cluster]);
    const local = page.getByRole("dialog", {
      name: "local Instance",
      exact: true,
    });
    await local.getByRole("radio", { name: "blue-005", exact: true }).check();
    const locally = execution(page, id);
    await local.getByRole("button", { name: d.apply, exact: true }).click();
    const localResponse = await locally;
    expect(localResponse.request().postDataJSON().panel_ids).toEqual(["local"]);
    expect(
      (await localResponse.json()).local_values.local.instance.values,
    ).toEqual(["blue-005"]);

    const text = page.getByRole("textbox", {
      name: "mapped_logs Keyword",
      exact: true,
    });
    await text.fill("keep missing text");
    const empty = execution(page, id);
    await text.press("Enter");
    const noData = await (await empty).json();
    expect(noData.panels[0].status).toBe("no_data");
    expect(noData.local_values.mapped_logs.term.values).toEqual([
      "keep missing text",
    ]);

    await page.getByRole("button", { name: "Pool", exact: true }).click();
    const pool = page.getByRole("dialog", { name: "Pool", exact: true });
    await pool.getByRole("radio", { name: "green", exact: true }).check();
    const changed = execution(page, id);
    await pool.getByRole("button", { name: d.apply, exact: true }).click();
    const changedResponse = await changed;
    expect(changedResponse.request().postDataJSON().panel_ids).toEqual([
      "dependent",
      "local",
      "mapped_logs",
    ]);
    const cascade = await changedResponse.json();
    expect(cascade.variables.member.all).toBe(true);
    expect(cascade.variable_candidates.member.reset).toBe(true);
    expect(cascade.local_values.local.instance.all).toBe(true);
    expect(cascade.local_values.mapped_logs.term.values).toEqual([
      "keep missing text",
    ]);
    expect(
      await page.locator('[data-panel-id="independent"]').innerText(),
    ).toBe(unchanged);
    await text.fill("");
    const mapped = execution(page, id);
    await text.press("Enter");
    const mappedResult = await (await mapped).json();
    expect(JSON.stringify(mappedResult.panels)).toContain(
      "planv2 selection green",
    );
    const published = await (
      await dashboardAPI(page, `/dashboards/${id}`)
    ).json();
    expect(published.revision.spec.variables[0].default.values).toEqual([
      "blue",
    ]);
    expect(published.revision.spec.variables[1].default.values).toEqual([
      "blue-220",
    ]);
    await page.screenshot({
      path: info.outputPath("cascade-local-mapping.png"),
      fullPage: true,
    });

    // Browser-local failure-state injection checks presentation; this is not
    // evidence of an actual Query service outage.
    // Browser fetch retains Chromium's isolated ingress resolution. Node's
    // route.fetch does not use the browser host-resolver mapping.
    const verified = await (
      await dashboardAPI(page, `/dashboards/${id}/execute`, "POST", {
        resource_ids: [cluster],
        variables: cascade.variables,
        local_values: mappedResult.local_values,
        from: cascade.from,
        to: cascade.to,
      })
    ).json();
    const executeURL = `**/dashboards/${id}/execute`;
    await page.route(executeURL, async (route) => {
      const result = structuredClone(verified);
      result.variable_candidates.pool = {
        ...result.variable_candidates.pool,
        status: "unavailable",
        code: "CATALOG_UNAVAILABLE",
        complete: false,
        values: [],
        selected_exists: {},
        reset: false,
      };
      result.partial = true;
      await route.fulfill({ status: 200, json: result });
    });
    await page.getByRole("button", { name: d.refresh, exact: true }).click();
    await expect(
      page.getByText(d.candidateFailed, { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Pool", exact: true }),
    ).toContainText("green");
    await page.unroute(executeURL);
    await page.getByRole("button", { name: d.refresh, exact: true }).click();
    await expect(
      page.getByText(d.candidateFailed, { exact: true }),
    ).toHaveCount(0);

    // Top-level All can contain Hosts and Clusters. Local Catalog must still
    // send only the resolved resources applicable to this Panel.
    const resourcesButton = page.getByRole("button", {
      name: new RegExp(`^${d.resources}:`),
    });
    const resources = page.getByRole("dialog", {
      name: d.chooseResources,
      exact: true,
    });
    await resourcesButton.click();
    const allExecution = execution(page, id);
    await resources.getByRole("button", { name: d.all, exact: true }).click();
    const allScope = await (await allExecution).json();
    expect(
      allScope.resources.some((r: { type: string }) => r.type === "host"),
    ).toBe(true);
    const applicable = allScope.resources
      .filter((r: { type: string }) => r.type === "kubernetes_cluster")
      .map((r: { id: string }) => r.id);
    const scopedCatalog = page.waitForResponse(
      (r) =>
        r.url().includes("/dashboards/catalog/query") && r.status() === 200,
    );
    await page
      .getByRole("button", { name: "local Instance", exact: true })
      .click();
    expect((await scopedCatalog).request().postDataJSON().resource_ids).toEqual(
      applicable,
    );
    await local
      .getByRole("button", { name: english ? "Cancel" : "取消", exact: true })
      .click();

    // Resource switching must not treat an inapplicable local scope as All.
    const hosts = await (await dashboardAPI(page, "/enterprise/hosts")).json();
    const host = hosts.items[0];
    await resourcesButton.click();
    // Keep locator positions stable while changing checked state.
    for (const checkbox of await resources.getByRole("checkbox").all())
      await checkbox.uncheck();
    await resources
      .getByRole("checkbox", { name: host.name, exact: true })
      .check();
    const switched = execution(page, id);
    await resources.getByRole("button", { name: d.apply, exact: true }).click();
    const hostScope = await (await switched).json();
    expect(hostScope.resources.map((r: { id: string }) => r.id)).toEqual([
      host.id,
    ]);
    expect(
      hostScope.panels.every(
        (p: { status: string }) => p.status === "not_applicable",
      ),
    ).toBe(true);
    let queried = false;
    const observe = (request: import("@playwright/test").Request) => {
      if (request.url().includes("/dashboards/catalog/query")) queried = true;
    };
    page.on("request", observe);
    await page
      .getByRole("button", { name: "local Instance", exact: true })
      .click();
    await expect(
      local.getByRole("checkbox", {
        name: english ? "All" : "全部",
        exact: true,
      }),
    ).toBeChecked();
    await expect(local.getByRole("status")).toHaveCount(0);
    expect(queried).toBe(false);
    page.off("request", observe);
  });
}
function execution(page: Page, id: string) {
  return page.waitForResponse(
    (r) => r.url().endsWith(`/dashboards/${id}/execute`) && r.status() === 200,
  );
}
