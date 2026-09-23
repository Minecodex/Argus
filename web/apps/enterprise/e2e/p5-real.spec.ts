import { expect, test } from "@playwright/test";
import { enterpriseOrigin } from "./origins";
import { createMfaLogin } from "./helpers/mfa-login";

const login = createMfaLogin("enterprise");
test.describe("P5 real tools and workspace", () => {
  test.skip(
    process.env.ARGUS_P5_E2E !== "1",
    "P5 Kubernetes environment is not active",
  );
  test.describe.configure({ mode: "serial", timeout: 180_000 });

  test("distinguishes estimated usage from reported model totals", async ({
    page,
  }) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    await page.goto(`${enterpriseOrigin}/settings/ai`);
    const response = page.waitForResponse(
      (value) =>
        value.url().includes("/enterprise/model-usage") &&
        value.request().method() === "GET",
    );
    await page
      .getByRole("tab", { name: /治理仪表盘|Governance dashboard/ })
      .click();
    const usage = await response;
    expect(usage.status()).toBe(200);
    const values = await usage.json();
    expect(
      values.some(
        (value: { cached_input_tokens: number }) =>
          value.cached_input_tokens > 0,
      ),
    ).toBe(true);
    expect(
      values.every(
        (value: { cached_input_tokens: number; input_tokens: number }) =>
          value.cached_input_tokens <= value.input_tokens,
      ),
    ).toBe(true);
    expect(
      values.some(
        (value: { usage_complete: boolean; estimated_input_tokens: number }) =>
          !value.usage_complete && value.estimated_input_tokens > 0,
      ),
    ).toBe(true);
    await expect(
      page.getByText(/^(用量记录不完整|Incomplete usage)$/),
    ).toBeVisible();
    await expect(
      page.getByText(/^(实际 Token|Reported tokens)$/),
    ).toBeVisible();
    await expect(
      page.getByText(/^(缓存输入 Token|Cached input tokens)$/),
    ).toBeVisible();
  });

  test("shows immutable tool details and current MCP connection configuration", async ({
    page,
  }) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    await page.goto(
      `${enterpriseOrigin}/?c=${process.env.ARGUS_P5_NATIVE_CONVERSATION}`,
    );
    const frame = page.locator(".argus-tool-presentation__frame").first();
    await expect(frame).toBeVisible();
    const detail = page.frameLocator(".argus-tool-presentation__frame").first();
    // The independent P5 fixture owns no hosts; the fixed template must
    // render its real empty state after the handshake, not an empty iframe.
    await expect(detail.locator("#title")).toHaveText("Resource details");
    await expect(detail.locator("#result")).toHaveText(
      "No resources to display",
    );
    expect(await frame.getAttribute("sandbox")).toBe("allow-scripts");
    await page.goto(`${enterpriseOrigin}/settings/mcp`);
    await expect(
      page.getByRole("heading", { name: /MCP/ }).first(),
    ).toBeVisible();
    await expect(
      page.getByText("P5 agent-sandbox-sse", { exact: true }),
    ).toBeVisible();
  });

  test("downloads the original published bytes and uploads real attachments", async ({
    page,
  }) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    await page.goto(
      `${enterpriseOrigin}/?c=${process.env.ARGUS_P5_WORKSPACE_CONVERSATION}`,
    );
    const download = page
      .getByRole("link", { name: "report.txt", exact: true })
      .first();
    await expect(download).toBeVisible();
    const body = await page.evaluate(
      async (href) => {
        const response = await fetch(href, { credentials: "include" });
        return response.text();
      },
      (await download.getAttribute("href"))!,
    );
    expect(body).toBe("6\n");
    await page.locator('input[type="file"]').setInputFiles({
      name: "browser-data.csv",
      mimeType: "text/csv",
      buffer: Buffer.from("value\n10\n20\n"),
    });
    await expect(
      page.locator(".argus-chat-chip").filter({ hasText: "browser-data.csv" }),
    ).toBeVisible({ timeout: 120_000 });
    const input = page.getByRole("textbox", { name: /发送|Send/ });
    await input.fill("Keep this uploaded data available for my next analysis.");
    const accepted = page.waitForRequest(
      (request) =>
        request.method() === "POST" && request.url().endsWith("/messages"),
    );
    await page.getByRole("button", { name: /发送|Send/ }).click();
    expect((await accepted).postDataJSON().file_ids).toHaveLength(1);
    await expect(input).toHaveValue("", { timeout: 120_000 });
  });

  test("keeps the draft when the complete selected tool set exceeds model capacity", async ({
    page,
  }) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    await page.goto(
      `${enterpriseOrigin}/?c=${process.env.ARGUS_P5_CAPACITY_CONVERSATION}`,
    );
    const input = page.getByRole("textbox", { name: /发送|Send/ });
    await input.fill("Do not discard this draft.");
    let accepted = false;
    page.on("request", (request) => {
      if (request.method() === "POST" && request.url().endsWith("/messages"))
        accepted = true;
    });
    await page.getByRole("button", { name: /发送|Send/ }).click();
    await expect(
      page
        .getByRole("alert")
        .filter({ hasText: /容量不足|insufficient capacity/ }),
    ).toBeVisible();
    await expect(input).toHaveValue("Do not discard this draft.");
    expect(accepted).toBe(false);
  });

  test("explicitly deletes a Workspace containing only model-created files", async ({
    page,
  }) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    await page.goto(
      `${enterpriseOrigin}/?c=${process.env.ARGUS_P5_MODEL_ONLY_CONVERSATION}`,
    );
    const files = page.locator(".argus-conversation-workspace details").last();
    await files.locator("summary").click();
    await expect(files.locator("summary")).toContainText("0");
    const remove = files.getByRole("button", {
      name: /删除 Workspace|Delete Workspace/,
    });
    await expect(remove).toBeEnabled();
    await remove.click();
    const dialog = page.getByRole("dialog", {
      name: /删除 Workspace|Delete Workspace/,
    });
    const deleted = page.waitForResponse(
      (response) =>
        response.request().method() === "DELETE" &&
        response.url().endsWith("/workspace"),
    );
    await dialog
      .getByRole("button", { name: /删除 Workspace|Delete Workspace/ })
      .click();
    expect((await deleted).status()).toBe(202);
    await expect(dialog).not.toBeVisible();
    await expect(remove).toBeDisabled();
    await page.reload();
    await files.locator("summary").click();
    await expect(remove).toBeDisabled();
  });

  test("compaction failure publishes a terminal event and closes the browser stream", async ({
    page,
  }, info) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    const result = await page.evaluate(async (conversation) => {
      const response = await fetch(
        `/api/v1/conversations/${conversation}/events`,
        {
          credentials: "include",
          headers: { accept: "text/event-stream" },
          signal: AbortSignal.timeout(10000),
        },
      );
      if (!response.ok) throw new Error(`SSE status ${response.status}`);
      const text = await response.text();
      const events = text.split("\n\n").flatMap((frame) => {
        const data = frame
          .split("\n")
          .find((line) => line.startsWith("data: "));
        return data ? [JSON.parse(data.slice(6))] : [];
      });
      const terminal = events.filter((event) => event.terminal);
      return {
        terminalCount: terminal.length,
        event: terminal[0]?.data.event_type,
        reason: terminal[0]?.data.payload.stop_reason,
        code: terminal[0]?.data.payload.error_code,
      };
    }, process.env.ARGUS_P5_COMPACTION_FAILURE_CONVERSATION!);
    expect(result).toEqual({
      terminalCount: 1,
      event: "run_failed",
      reason: "context_compaction_failed",
      code: "CONTEXT_COMPACTION_FAILED",
    });
    await info.attach("compaction-failure-terminal", {
      body: JSON.stringify(result),
      contentType: "application/json",
    });
  });

  test("slow browser SSE consumption preserves complete events and resumes by cursor", async ({
    page,
  }, info) => {
    await login(
      page,
      `${enterpriseOrigin}/login`,
      process.env.ARGUS_P5_USERNAME ?? "",
      process.env.ARGUS_P5_PASSWORD ?? "",
    );
    const result = await page.evaluate(async (conversation) => {
      async function consume(cursor?: number) {
        const controller = new AbortController();
        const headers: Record<string, string> = { accept: "text/event-stream" };
        if (cursor !== undefined) headers["last-event-id"] = String(cursor);
        const response = await fetch(
          `/api/v1/conversations/${conversation}/events`,
          { credentials: "include", headers, signal: controller.signal },
        );
        if (!response.ok || !response.body)
          throw new Error(`SSE status ${response.status}`);
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        let maximumBufferedBytes = 0;
        const sequences: number[] = [];
        try {
          for (;;) {
            const { value, done } = await reader.read();
            buffer += decoder.decode(value, { stream: !done });
            maximumBufferedBytes = Math.max(
              maximumBufferedBytes,
              new TextEncoder().encode(buffer).length,
            );
            if (maximumBufferedBytes > 4 * 1024 * 1024)
              throw new Error("SSE browser buffer budget exceeded");
            const frames = buffer.split("\n\n");
            buffer = frames.pop() ?? "";
            for (const frame of frames) {
              const data = frame
                .split("\n")
                .find((line) => line.startsWith("data: "));
              if (!data) continue;
              const event = JSON.parse(data.slice(6)) as {
                sequence: number;
                terminal: boolean;
              };
              sequences.push(event.sequence);
              if (event.terminal) return { sequences, maximumBufferedBytes };
            }
            if (done) throw new Error("SSE ended without its terminal event");
            // Deliberately hold the response stream between browser reads.
            await new Promise((resolve) => setTimeout(resolve, 100));
          }
        } finally {
          controller.abort();
          await reader.cancel().catch(() => undefined);
        }
      }
      const initial = await consume();
      const cursor =
        initial.sequences[Math.floor(initial.sequences.length / 2)];
      return { initial, cursor, resumed: await consume(cursor) };
    }, process.env.ARGUS_P5_NATIVE_CONVERSATION!);
    expect(result.initial.sequences.length).toBeGreaterThan(3);
    expect(result.resumed.sequences).toEqual(
      result.initial.sequences.filter((sequence) => sequence > result.cursor),
    );
    expect(new Set(result.initial.sequences).size).toBe(
      result.initial.sequences.length,
    );
    await info.attach("slow-sse-capacity", {
      body: JSON.stringify(result),
      contentType: "application/json",
    });
  });
});
