import { expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { openToolResults } from "./tool-results";

const fixtures = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
);
export const dashboardBinding = fixtures.bindings.find(
  (row: { Tool: string }) => row.Tool === "telemetry.dashboard.query",
);

/** Controlled presentation of the current immutable production template source. */
export async function openSealedDashboard(
  page: Page,
  english: boolean,
  dark: boolean,
  data: Record<string, unknown>,
) {
  return openSealedTool(page, english, dark, "telemetry.dashboard.query", data);
}

export async function openSealedTool(
  page: Page,
  english: boolean,
  dark: boolean,
  tool: string,
  data: Record<string, unknown>,
) {
  const binding = fixtures.bindings.find(
    (row: { Tool: string }) => row.Tool === tool,
  );
  if (!binding || binding.Mode !== "template")
    throw new Error("Reviewed template binding required");
  await page.addInitScript(
    ({ english, dark }) => {
      if (window !== window.top) return;
      localStorage.setItem("argus.locale", english ? "en-US" : "zh-CN");
      localStorage.setItem("argus.theme", dark ? "dark" : "light");
    },
    { english, dark },
  );
  await page.goto("/login");
  await page.getByLabel(english ? "Username" : "用户名").fill("root");
  await page.getByLabel(english ? "Password" : "密码").fill("123456");
  await page
    .getByRole("button", {
      name: english ? "Sign in" : "登录",
      exact: true,
    })
    .click();
  const composer = page.locator(".argus-chat-composer");
  await composer.getByRole("textbox").fill("Sealed business result acceptance");
  await composer
    .getByRole("button", {
      name: english ? "Send" : "发送",
      exact: true,
    })
    .click();
  await expect(page).toHaveURL(/c=/);
  await expect(composer.getByRole("textbox")).toHaveValue("");
  const conversation = new URL(page.url()).searchParams.get("c")!;
  await page.evaluate(
    ({ conversation, binding, source, data }) => {
      const key = "argus-mock:db-v14",
        db = JSON.parse(localStorage.getItem(key)!);
      const call = crypto.randomUUID();
      db.messages = db.messages.filter(
        (row: { conversationId: string }) =>
          row.conversationId !== conversation,
      );
      db.planv5.presentations[call] = {
        conversationId: conversation,
        value: {
          tool_call_id: call,
          runtime: "argus-template/v2",
          status: "ready",
          template_source: source,
          template_hash: binding.Hash,
          read_operations: binding.Reads,
          detail_data: data,
        },
      };
      db.messages.push({
        id: call,
        conversationId: conversation,
        role: "assistant",
        content: "",
        createdAt: new Date().toISOString(),
        presentations: [call],
      });
      localStorage.setItem(key, JSON.stringify(db));
    },
    {
      conversation,
      binding,
      source: fixtures.assets[binding.Hash],
      data,
    },
  );
  await page.reload();
  await openToolResults(page);
  const frame = page.locator(".argus-tool-presentation iframe");
  await expect(frame).toBeVisible();
  return { frame, content: frame.contentFrame() };
}
