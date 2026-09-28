import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { createMfaLogin } from "./helpers/mfa-login";
import { dashboardAPI } from "./helpers/planv2-api";

const login = createMfaLogin("enterprise");
test.use({ actionTimeout: 15000, navigationTimeout: 30000 });
for (const english of [false, true])
  test(`real Chat selection and creation mode ${english ? "English dark" : "Chinese light"}`, async ({
    page,
  }, info) => {
    test.skip(process.env.ARGUS_PLANV2_E2E !== "1", "Requires deployed PlanV2");
    test.setTimeout(150000);
    const tx = (zh: string, en: string) => (english ? en : zh);
    await page.addInitScript((english) => {
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", english ? "dark" : "light");
    }, english);
    await login(
      page,
      "/login",
      process.env.ARGUS_PLANV2_USERNAME!,
      process.env.ARGUS_PLANV2_PASSWORD!,
    );
    await page.goto("/dashboards");
    const convo = await (
      await dashboardAPI(
        page,
        "/conversations",
        "POST",
        {
          title: "PlanV2 UI context",
          selected_model_id: process.env.ARGUS_PLANV2_MODEL_ID,
        },
        201,
      )
    ).json();
    await page.goto(`/?c=${convo.id}`);
    const composer = page.locator(".argus-chat-composer"),
      text = composer.getByRole("textbox");
    await text.fill("@PlanV2 native source mapping");
    await page
      .getByRole("option", {
        name: "PlanV2 native source mapping",
        exact: true,
      })
      .click();
    await expect(composer).toContainText("PlanV2 native source mapping");
    const send = async (content: string) => {
      await text.fill(content);
      const accepted = page.waitForResponse(
        (r) =>
          r.url().endsWith(`/conversations/${convo.id}/messages`) &&
          r.request().method() === "POST",
      );
      await composer
        .getByRole("button", { name: tx("发送", "Send"), exact: true })
        .click();
      const response = await accepted;
      expect(response.status()).toBe(202);
      await expect(text).toHaveValue("");
      const run = (await response.json()).run.run_id;
      await expect
        .poll(
          async () =>
            (await (await dashboardAPI(page, `/runs/${run}`)).json()).status,
          { timeout: 45000 },
        )
        .toBe("succeeded");
      return response.request().postDataJSON();
    };
    // Replay emits no tool calls. These assertions cover persisted user intent,
    // transport and UI only, never model interpretation or conclusion quality.
    const selected = await send("检查所选仪表盘 argus_e2e_plan_b64:W10");
    expect(selected.dashboard_context.mode).toBe("analyze");
    expect(selected.dashboard_context.dashboard_ids).toEqual([
      process.env.ARGUS_PLANV2_NATIVE_MAPPING_ID,
    ]);
    await page.reload();
    await expect(composer).toContainText("PlanV2 native source mapping");
    await send("继续 argus_e2e_plan_b64:W10");
    await page.screenshot({
      path: info.outputPath("persistent-selected-dashboard.png"),
      fullPage: true,
    });
    await composer
      .getByRole("button", {
        name: tx("退出仪表盘模式", "Exit dashboard mode"),
        exact: true,
      })
      .click();
    expect(
      (await send("普通会话 argus_e2e_plan_b64:W10")).dashboard_context.mode,
    ).toBe("none");
    await page.reload();
    await expect(composer).not.toContainText("PlanV2 native source mapping");
    await composer
      .getByRole("button", {
        name: tx("创建仪表盘", "Create dashboard"),
        exact: true,
      })
      .click();
    await expect(text).toHaveValue(tx("/创建仪表盘 ", "/create-dashboard "));
    expect(
      (await send("/创建仪表盘 argus_e2e_plan_b64:W10")).dashboard_context.mode,
    ).toBe("create");
    await page.reload();
    await expect(composer).toContainText(
      tx("创建 / 编辑仪表盘", "Create / edit dashboards"),
    );
    const history = page.getByRole("region", {
      name: tx("会话消息", "Conversation messages"),
      exact: true,
    });
    await history.focus();
    await expect(history).toBeFocused();
    await page.keyboard.press("ArrowUp");
    const a11y = await new AxeBuilder({ page }).analyze();
    expect(
      a11y.violations.filter(
        (v) => v.impact === "critical" || v.impact === "serious",
      ),
    ).toEqual([]);
  });

test("64 real panels remain keyboard-accessible and refresh within the runtime limit", async ({
  page,
}, info) => {
  test.skip(process.env.ARGUS_PLANV2_E2E !== "1", "Requires deployed PlanV2");
  test.setTimeout(180000);
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "en-US");
    localStorage.setItem("argus.theme", "dark");
  });
  await login(
    page,
    "/login",
    process.env.ARGUS_PLANV2_USERNAME!,
    process.env.ARGUS_PLANV2_PASSWORD!,
  );
  const id = process.env.ARGUS_PLANV2_CAPACITY_ID!;
  const executed = page.waitForResponse(
    (r) => r.url().endsWith(`/dashboards/${id}/execute`) && r.status() === 200,
  );
  await page.goto(
    `/dashboards/${id}?resource=${process.env.ARGUS_PLANV2_CLUSTER_ID}`,
  );
  expect((await (await executed).json()).panels).toHaveLength(64);
  await expect(page.locator("[data-panel-id]")).toHaveCount(64);
  const refresh = page.getByRole("button", { name: "Refresh", exact: true });
  await refresh.focus();
  const again = page.waitForResponse(
    (r) => r.url().endsWith(`/dashboards/${id}/execute`) && r.status() === 200,
  );
  await page.keyboard.press("Enter");
  expect((await (await again).json()).panels).toHaveLength(64);
  const stat = page.getByRole("region", {
    name: "Metric 00 · Statistics",
    exact: true,
  });
  await stat.focus();
  await expect(stat).toBeFocused();
  await page.keyboard.press("ArrowRight");
  await page.screenshot({
    path: info.outputPath("64-panels.png"),
    fullPage: true,
  });
  const a11y = await new AxeBuilder({ page }).analyze();
  expect(
    a11y.violations.filter(
      (v) => v.impact === "critical" || v.impact === "serious",
    ),
  ).toEqual([]);
});
