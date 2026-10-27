import { expect, test } from "@playwright/test";
import { readFile, writeFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { plan7Login } from "./helpers/plan7-login";

const enabled = process.env.ARGUS_PLAN8_MODEL_FORM_REAL === "1";
test.use({ trace: "off", video: "off" });
test("real model requests secure monitoring form and verifies actual collection", async ({
  page,
}, info) => {
  test.skip(
    !enabled,
    "requires owned database, enrolled Collector and actual approved provider",
  );
  test.setTimeout(660000);
  const fixture = JSON.parse(
    await readFile(process.env.ARGUS_PLAN8_MODEL_FORM_FIXTURE!, "utf8"),
  ) as {
    conversation_id: string;
    collector_name: string;
    collector_id: string;
    name: string;
    endpoint: string;
    database: string;
    username: string;
    password: string;
    prompt: string;
    already_requested?: boolean;
  };
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.addInitScript(() => {
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  await plan7Login(
    page,
    "/login",
    process.env.ARGUS_PLAN7_USERNAME!,
    process.env.ARGUS_PLAN7_PASSWORD!,
  );
  await page.goto(`/?c=${fixture.conversation_id}`);
  const composer = page.locator(".argus-chat-composer");
  if (!fixture.already_requested) {
    await composer.getByRole("textbox").fill(fixture.prompt);
    await composer.getByRole("button", { name: "发送", exact: true }).click();
  }
  const open = page.getByRole("button", {
    name: "打开监控安全表单",
    exact: true,
  });
  const deadline = Date.now() + 180000;
  while (!(await open.isVisible().catch(() => false))) {
    for (const button of await page
      .getByRole("button", { name: /^查看 · / })
      .all())
      if (await button.isVisible()) await button.click();
    if (Date.now() > deadline)
      throw new Error("actual model did not offer the trusted monitoring form");
    await page.waitForTimeout(500);
  }
  // Asking for the form must not create a target or collection. The future Go driver
  // must verify that invariant plus final model usage and canary evidence.
  await open.click();
  const form = page.getByRole("dialog").last();
  await form.getByRole("textbox", { name: /监控名称/ }).fill(fixture.name);
  await form.getByRole("textbox", { name: /服务地址/ }).fill(fixture.endpoint);
  if (fixture.database) {
    const database = form.getByRole("textbox", { name: /数据库/ });
    if (await database.count()) await database.fill(fixture.database);
  }
  await form.getByRole("textbox", { name: /用户名/ }).fill(fixture.username);
  // Avoid locator.fill(secret): timeout diagnostics can include its argument.
  await form.locator('input[type="password"]').evaluate((element, value) => {
    const input = element as HTMLInputElement;
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }, fixture.password);
  await form
    .getByRole("button", { name: "选择执行收集器", exact: true })
    .click();
  const picker = page.getByRole("dialog", {
    name: "选择执行收集器",
    exact: true,
  });
  const selected = picker
    .getByRole("row")
    .filter({ hasText: fixture.collector_name });
  await expect(selected).toHaveCount(1);
  await selected.getByRole("button", { name: /^选择 / }).click();
  const previewResponse = page.waitForResponse(
    (response) =>
      response.url().endsWith("/monitoring/commands/preview") &&
      response.request().method() === "POST",
  );
  await form.getByRole("button", { name: "预览采集配置", exact: true }).click();
  const preview = await previewResponse;
  expect(preview.status()).toBe(201);
  const publicPreview = await preview.json();
  if (JSON.stringify(publicPreview).includes(fixture.password))
    throw new Error("credential material reached public preview");
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  const confirmResponse = page.waitForResponse(
    (response) =>
      response
        .url()
        .endsWith(`/pending-actions/${publicPreview.action_ref}/confirm`) &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "确认变更", exact: true }).click();
  const confirmed = await confirmResponse;
  expect(confirmed.status()).toBe(200);
  const result = await confirmed.json();
  if (JSON.stringify(result).includes(fixture.password))
    throw new Error("credential material reached confirmation response");
  const persisted = await page.evaluate(
    (password) =>
      [...Object.values(localStorage), ...Object.values(sessionStorage)].some(
        (value) => value.includes(password),
      ),
    fixture.password,
  );
  if (persisted)
    throw new Error("credential material entered browser persistent storage");
  // Confirmation accepts an async Execution. The Go driver must poll the same
  // action to its final ResourceRef, never submit a second confirmation, verify
  // one user_confirmation row, then send a natural follow-up in the same chat.
  // Initial model request and final follow-up have separate real usage receipts.
  await writeFile(
    process.env.ARGUS_PLAN8_MODEL_FORM_RECEIPT ??
      info.outputPath("secure-form-receipt.json"),
    JSON.stringify(
      {
        scope:
          "browser_protected_form_and_one_confirmation_async_execution_and_model_followup_pending",
        conversation_id: fixture.conversation_id,
        action_ref: publicPreview.action_ref,
        confirmation_count: 1,
        unique_receipt: randomUUID(),
      },
      null,
      2,
    ),
    { mode: 0o600 },
  );
  await page.screenshot({
    path: info.outputPath("secure-form-confirmed.png"),
    fullPage: true,
    animations: "disabled",
  });
});
