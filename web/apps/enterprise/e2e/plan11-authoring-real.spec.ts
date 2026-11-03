import { expect, test } from "@playwright/test";
import { writeFileSync } from "node:fs";
import { browserApi } from "./helpers/browser-api";

const enabled = process.env.ARGUS_PLAN11_AUTHORING_REAL === "1";
const cases = enabled
  ? (JSON.parse(process.env.ARGUS_PLAN11_AUTHORING_CASES!) as {
      name: string;
      conversation_id: string;
      draft_id: string;
      action_ref: string;
      decision: "cancel" | "confirm";
    }[])
  : [];
test.skip(!enabled, "requires owned real-model authoring previews");
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });

for (const row of cases) {
  test(`Draft publication and Chatbox ${row.decision}`, async ({ page, context }, info) => {
    test.setTimeout(180_000);
    await context.addCookies(JSON.parse(process.env.ARGUS_PLAN11_AUTHORING_COOKIES!));
    let decisions = 0;
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => {
      if (request.method() === "POST" &&
          request.url().includes(`/pending-actions/${row.action_ref}/`) &&
          /\/(confirm|cancel)$/.test(new URL(request.url()).pathname)) decisions++;
    });
    const actionPath = `/api/v1/enterprise/pending-actions/${row.action_ref}`;
    await page.goto(`/?c=${row.conversation_id}`);
    await expect(page).not.toHaveURL(/\/login/);
    const views: { locale: string; theme: string; pending_after_reload: boolean }[] = [];
    for (const locale of ["zh-CN", "en-US"]) {
      for (const theme of ["light", "dark"]) {
        await page.evaluate(({ locale, theme }) => {
          localStorage.setItem("argus.locale", locale);
          localStorage.setItem("argus.theme", theme);
        }, { locale, theme });
        await page.reload();
        const card = page.getByTestId("pending-action-card");
        await expect(card).toHaveCount(1);
        await expect(card).toContainText(row.name);
        await expect(card.getByRole("button", {
          name: locale === "zh-CN" ? "确认执行" : "Confirm", exact: true,
        })).toBeEnabled();
        await expect(card.getByRole("button", {
          name: locale === "zh-CN" ? "取消" : "Cancel", exact: true,
        })).toBeEnabled();
        const response = await browserApi(page, actionPath);
        expect(response.status()).toBe(200);
        const action = await response.json();
        expect(action.action_type).toBe("telemetry.dashboard.publish");
        expect(action.status).toBe("awaiting_confirmation");
        expect(action.action_ref).toBe(row.action_ref);
        expect(decisions).toBe(0);
        views.push({ locale, theme, pending_after_reload: true });
        await page.screenshot({ path: info.outputPath(`preview-${locale}-${theme}.png`) });
        writeFileSync(info.outputPath("preview-views.json"), JSON.stringify({
          conversation_id: row.conversation_id, action_ref: row.action_ref,
          views, decision_requests: decisions,
        }, null, 2));
      }
    }
    // The final view is en-US/dark. One explicit click is the only authority.
    const card = page.getByTestId("pending-action-card");
    const response = page.waitForResponse((value) =>
      value.request().method() === "POST" &&
      new URL(value.url()).pathname.endsWith(`/pending-actions/${row.action_ref}/${row.decision}`),
    );
    await card.getByRole("button", {
      name: row.decision === "confirm" ? "Confirm" : "Cancel", exact: true,
    }).click();
    expect((await response).status()).toBe(200);
    expect(decisions).toBe(1);
    const targetStatus = row.decision === "confirm" ? "succeeded" : "cancelled";
    await expect.poll(async () => {
      const result = await browserApi(page, actionPath);
      return (await result.json()).status as string;
    }, { timeout: 60_000 }).toBe(targetStatus);
    await page.reload();
    await expect(page.getByTestId("pending-action-card")).toBeVisible();
    await expect(page.getByTestId("pending-action-card").getByRole("button", { name: "Confirm", exact: true })).toHaveCount(0);
    // A new idempotency key cannot consume the same action again. This request
    // stays in the same authenticated browser and preserves CSRF enforcement.
    const replay = await browserApi(page, `${actionPath}/confirm`, {
      method: "POST", headers: { "Idempotency-Key": `p11-replay-${crypto.randomUUID()}` },
    });
    expect(replay.status()).toBe(409);
    const replayCode = (await replay.json()).code as string;
    // The Action service maps a terminal status to ErrInvalidated. Resource
    // actions can also use their explicit state-conflict code. Both must deny
    // consumption; an arbitrary 409 or a successful replay is not sufficient.
    expect(["PENDING_ACTION_INVALIDATED", "ACTION_STATE_CONFLICT"]).toContain(replayCode);
    expect(errors).toEqual([]);
    await page.screenshot({ path: info.outputPath("decision.png") });
    writeFileSync(info.outputPath("receipt.json"), JSON.stringify({
      passed: true, conversation_id: row.conversation_id, draft_id: row.draft_id,
      action_ref: row.action_ref, decision: row.decision, views,
      actual_click_requests: 1, final_status: targetStatus,
      replay_status: replay.status(), replay_code: replayCode,
      no_refresh_confirmation: true,
    }, null, 2));
  });
}
