import { describe, expect, it, vi } from "vitest";
import { HttpTransport } from "./http";
describe("authorized file previews", () => {
  it("uses the authenticated original download route and preserves the bytes", async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        new Response("## report", {
          headers: { "content-type": "text/markdown" },
        }),
      );
    const http = new HttpTransport({ base_url: "https://argus.test", fetch });
    expect(
      await (await http.preview("conversations/c/files/f/content")).text(),
    ).toBe("## report");
    expect(fetch.mock.calls[0]?.[1]?.credentials).toBe("include");
  });
  it("rejects oversized chunked responses and cancels their reader", async () => {
    const cancel = vi.fn();
    const body = new ReadableStream({
      start(controller) {
        controller.enqueue(new Uint8Array(6));
      },
      cancel,
    });
    const http = new HttpTransport({
      base_url: "https://argus.test",
      fetch: vi.fn().mockResolvedValue(new Response(body)),
    });
    await expect(http.preview("file", undefined, 5)).rejects.toMatchObject({
      code: "WORKSPACE_PREVIEW_TOO_LARGE",
    });
    expect(cancel).toHaveBeenCalled();
  });
  it("retains authorization failures and invalidates a revoked session", async () => {
    const invalidate = vi.fn();
    const http = new HttpTransport({
      base_url: "https://argus.test",
      on_authentication_invalidated: invalidate,
      fetch: vi
        .fn()
        .mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "SESSION_REVOKED",
              message_key: "errors.sessionRevoked",
              request_id: "request",
              retryable: false,
            }),
            { status: 401 },
          ),
        ),
    });
    await expect(http.preview("file")).rejects.toMatchObject({
      code: "SESSION_REVOKED",
      status: 401,
    });
    expect(invalidate).toHaveBeenCalled();
  });
});
