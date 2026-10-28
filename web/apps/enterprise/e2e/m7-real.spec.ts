import {selectTrigger} from "./helpers/select";
import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { createMfaLogin } from "./helpers/mfa-login";

const enabled = process.env.ARGUS_M7_E2E === "1";
const enterpriseUsername = process.env.ARGUS_M7_ENTERPRISE_USERNAME ?? "";
const enterprisePassword = process.env.ARGUS_M7_ENTERPRISE_PASSWORD ?? "";
const clusterId = process.env.ARGUS_M7_CLUSTER_ID ?? "";
const hostId = process.env.ARGUS_M7_HOST_ID ?? "";
const loginWithMfa = createMfaLogin("enterprise");

const variants = [
  { locale: "zh-CN", theme: "light" },
  { locale: "zh-CN", theme: "dark" },
  { locale: "en-US", theme: "light" },
  { locale: "en-US", theme: "dark" },
] as const;

for (const variant of variants) test.describe(`M7 real telemetry flow ${variant.locale} ${variant.theme}`, () => {
  test.skip(!enabled, "M7 Kubernetes environment is not active");
  test.describe.configure({ mode: "serial", timeout: 120_000 });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(({ locale, theme }) => {
      window.localStorage.setItem("argus.locale", locale);
      window.localStorage.setItem("argus.theme", theme);
    }, variant);
    await login(page);
  });

  test("renders authorized Kubernetes Metrics, Logs, and Traces", async ({
    page,
  }) => {
    expect(clusterId).not.toBe("");
    await page.goto(`/kubernetes/${clusterId}`);

    for(const name of ["指标","日志","链路"])await expect(page.getByRole("tab",{name,exact:true})).toHaveCount(0);
    await page.getByRole("button",{name:/^观测查询$|^Observability queries$/}).click();
    await executeObservationSignals(page,variant.locale==="en-US");
    await expectNoSeriousAccessibilityViolations(page);
  });

  test("renders the converged Linux Host Collector and its three signals", async ({
    page,
  }) => {
    expect(hostId).not.toBe("");
    await page.goto(`/hosts/${hostId}`);
    await page.getByRole("tab", { name: /组件|Components/ }).click();
    await expect(page.getByText(/监控中|Monitoring/).first()).toBeVisible();
    await expect(
      page.getByText(/R(\d+) \/ (?:目标|target) R\1/),
    ).toBeVisible();

    for(const name of ["指标","日志","链路"])await expect(page.getByRole("tab",{name,exact:true})).toHaveCount(0);
    await page.getByRole("button",{name:/^观测查询$|^Observability queries$/}).click();
    await executeObservationSignals(page,variant.locale==="en-US");
    await expectNoSeriousAccessibilityViolations(page);
  });

  test("shows retention, usage, and the locked support matrix", async ({
    page,
  }) => {
    await page.goto("/settings/org");
    await page.getByRole("tab", { name: /遥测|Telemetry/ }).click();

    await expect(page.getByText(/Evaluation 保留期|Evaluation retention/)).toBeVisible();
    await expect(page.getByRole("heading", { name: /近月用量|Monthly usage/ })).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "linux_arm64", exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByRole("cell", { name: "windows_amd64", exact: true }).first(),
    ).toBeVisible();
    await expect(page.getByText(/待实体验证|Validation pending/).first()).toBeVisible();
    await expectNoSeriousAccessibilityViolations(page);
  });
});

async function login(page: Page) {
  await loginWithMfa(page, "/login", enterpriseUsername, enterprisePassword);
}

async function expectNoSeriousAccessibilityViolations(page: Page) {
  const result = await new AxeBuilder({ page }).analyze();
  expect(
    result.violations.filter(
      (violation) =>
        violation.impact === "serious" || violation.impact === "critical",
    ),
  ).toEqual([]);
}

async function executeObservationSignals(page:Page,english:boolean){
 const text=(zh:string,en:string)=>english?en:zh,card=page.locator(".argus-observe-query").first();
 for(const signal of [["指标","Metrics"],["日志","Logs"],["链路","Traces"]]){
  await selectTrigger(card,text("查询类型","Query type")).click();await page.getByRole("option",{name:signal[english?1:0],exact:true}).click();
  await expect(selectTrigger(card,text("采集来源","Collection source"))).toBeEnabled();
  await selectTrigger(card,text("采集来源","Collection source")).click();await page.getByRole("option").filter({hasNotText:/选择采集来源|Select a collection source/}).first().click();
  if(signal[0]==="指标"){const input=card.getByRole("combobox",{name:english?/^Metric\*?$/:/^指标\*?$/});await input.click();await input.press("ArrowDown");await page.getByRole("option").first().click();}
  await card.getByRole("button",{name:text("执行查询","Run query"),exact:true}).click();await expect(card.getByRole("status").filter({hasText:text("查询完成","Complete")})).toBeVisible();
  if(signal[0]==="指标")await expect(card.getByRole("img")).toBeVisible();
  else if(signal[0]==="日志")await expect(card.locator(".argus-observation-log-body").first()).toBeVisible();
  else await expect(card.getByRole("columnheader",{name:text("调用链 ID","Trace ID"),exact:true})).toBeVisible();
 }
}
