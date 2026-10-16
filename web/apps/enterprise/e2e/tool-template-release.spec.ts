import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { browserApi } from "./helpers/browser-api";
import { openToolResults } from "./helpers/tool-results";

test("formal Chatbox loads its production v2 host template with an owned empty tenant", async ({
  page,
}, info) => {
  const manifest = process.env.ARGUS_TEMPLATE_RELEASE_MANIFEST;
  test.skip(!manifest, "Requires the owned release loading fixture");
  const fixture = JSON.parse(readFileSync(manifest!, "utf8")) as {
    Username: string;
    Password: string;
    Conversation: string;
  };
  await page.addInitScript(() => {
    if (window !== window.top) return;
    localStorage.setItem("argus.locale", "zh-CN");
    localStorage.setItem("argus.theme", "light");
  });
  await page.goto("/login");
  await page
    .getByRole("textbox", { name: "用户名", exact: true })
    .fill(fixture.Username);
  await page.getByLabel("密码").fill(fixture.Password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).not.toHaveURL(/\/login/);
  await page.goto("/?c=" + fixture.Conversation);
  const summary = page.getByRole("button", { name: "置顶摘要", exact: true });
  await summary.click();
  const popup = page.getByRole("dialog", { name: "置顶摘要", exact: true });
  await expect(popup).toBeVisible();
  await expect(popup.getByText("会话 MCP 连接 · 0")).toBeVisible();
  await expect(popup.getByText("会话文件 · 0")).toBeVisible();
  const workspaceFiles = await browserApi(
    page,
    `/api/v1/conversations/${fixture.Conversation}/workspace/files`,
  );
  // This owned tenant has no provisioned Workspace; absence is an empty state.
  expect(workspaceFiles.status()).toBe(404);
  await expect(popup.getByRole("alert")).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("formal-pinned-summary.png"),
    animations: "disabled",
  });
  await page.keyboard.press("Escape");
  await expect(popup).not.toBeVisible();
  await expect(summary).toBeFocused();
  await openToolResults(page);
  const frame = page.locator(".argus-tool-presentation iframe");
  await expect(frame).toBeVisible({ timeout: 15000 });
  await expect
    .poll(() =>
      frame
        .contentFrame()
        .locator("html")
        .evaluate((el) => el.scrollHeight - el.clientHeight),
    )
    .toBeLessThanOrEqual(1);
  expect(
    await frame
      .contentFrame()
      .locator("body")
      .evaluate((el) => getComputedStyle(el, "::-webkit-scrollbar").width),
  ).toBe("9px");
  await expect(
    frame
      .contentFrame()
      .getByRole("heading", { name: "主机列表", exact: true }),
  ).toBeVisible();
  await expect(
    frame.contentFrame().getByText("当前范围没有数据", { exact: true }),
  ).toBeVisible();
  await expect(
    frame.contentFrame().locator(".argus-template-pagination"),
  ).toContainText("0");
  const ledger = await (
    await browserApi(
      page,
      `/api/v1/conversations/${fixture.Conversation}/ledger?limit=20`,
    )
  ).json();
  const call = ledger.items.find(
    (event: { event_type: string }) => event.event_type === "tool_presentation",
  ).payload.tool_call_id;
  const stored = await (
    await browserApi(
      page,
      `/api/v1/conversations/${fixture.Conversation}/tool-presentations/${call}`,
    )
  ).json();
  expect(stored.runtime).toBe("argus-template/v2");
  const data = await (
    await browserApi(
      page,
      `/api/v1/conversations/${fixture.Conversation}/tool-presentations/${call}/read`,
      { method: "POST", data: { operation: "result.detail", parameters: {} } },
    )
  ).json();
  expect(data.status).toBe("ready");
  expect(data.data.items).toEqual([]);
  const resultCard = page.getByTestId("tool-result");
  const runtimeURL = await frame.getAttribute("src");
  await resultCard.getByRole("tab", { name: "查询参数", exact: true }).click();
  await expect(
    resultCard.locator(".argus-chat-result-data:visible pre"),
  ).toHaveText("{}");
  await resultCard.getByRole("tab", { name: "返回内容", exact: true }).click();
  const invocation = ledger.items.find(
    (event: { event_type: string }) => event.event_type === "tool_call_result",
  );
  expect(
    JSON.parse(
      await resultCard
        .locator(".argus-chat-result-data:visible pre")
        .innerText(),
    ),
  ).toEqual(invocation.payload.projection.summary);
  await page.screenshot({
    path: info.outputPath("formal-returned-content.png"),
    animations: "disabled",
  });
  await resultCard.getByRole("tab", { name: "视图", exact: true }).click();
  await expect(frame).toBeVisible();
  await expect(frame).toHaveAttribute("src", runtimeURL!);
  await page.screenshot({
    path: info.outputPath("formal-empty-host-template.png"),
  });
  await page.reload();
  await openToolResults(page);
  await expect(
    frame.contentFrame().getByText("当前范围没有数据", { exact: true }),
  ).toBeVisible();
});
