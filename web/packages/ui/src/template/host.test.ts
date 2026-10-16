// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { createTemplateHost } from "./host";
import {
  TEMPLATE_PROTOCOL,
  TEMPLATE_PROTOCOL_V2,
  isTemplateMessage,
  type TemplateVersion,
} from "./protocol";
class Port {
  onmessage: ((event: MessageEvent) => void) | null = null;
  postMessage = vi.fn();
  start = vi.fn();
  close = vi.fn();
}
const channels: { port1: Port; port2: Port }[] = [];
const hosts: ReturnType<typeof createTemplateHost>[] = [];
afterEach(() => {
  for (const host of hosts) host.destroy();
  hosts.length = 0;
  vi.unstubAllGlobals();
  document.body.replaceChildren();
  channels.length = 0;
  vi.useRealTimers();
});
function mounted(version: TemplateVersion = TEMPLATE_PROTOCOL) {
  vi.useFakeTimers();
  vi.stubGlobal(
    "MessageChannel",
    class {
      port1 = new Port();
      port2 = new Port();
      constructor() {
        channels.push(this);
      }
    },
  );
  const frame = document.createElement("iframe");
  document.body.append(frame);
  const onError = vi.fn(),
    onReady = vi.fn(),
    onResize = vi.fn(),
    onResource = vi.fn();
  const onRead = vi.fn(async () => ({
    status: "ready" as const,
    data: { name: "saved result" },
  }));
  const host = createTemplateHost(frame, {
    origin: "https://templates.argus.test",
    presentation: {
      runtime: version,
      read_operations: ["result.detail"],
      template_source: "<p>Details</p>",
      template_hash: "a".repeat(64),
      detail_data: {},
      resource_refs: [{ type: "host", id: "h1" }],
    },
    context: { locale: "zh-CN", color_scheme: "dark", tokens: {} },
    onError,
    onReady,
    onResize,
    onResource,
    onRead,
  });
  hosts.push(host);
  const posted = vi
    .spyOn(frame.contentWindow!, "postMessage")
    .mockImplementation(() => {});
  frame.dispatchEvent(new Event("load"));
  const hello = posted.mock.calls[0]![0] as { nonce: string };
  const port = channels.at(-1)!.port1;
  const receive = (
    type: string,
    payload: Record<string, unknown> = {},
    sequence = 1,
    nonce = hello.nonce,
    requestId = crypto.randomUUID(),
  ) =>
    port.onmessage?.({
      data: {
        version,
        nonce,
        sequence,
        request_id: requestId,
        type,
        payload,
      },
    } as MessageEvent);
  return {
    host,
    frame,
    receive,
    onError,
    onReady,
    onResize,
    onResource,
    onRead,
    port,
  };
}
describe("Template capability boundary", () => {
  it("does not handshake with queued about:blank documents before Runtime navigation", () => {
    const active = Array.from({ length: 4 }, () => mounted());
    const frame = document.createElement("iframe");
    document.body.append(frame);
    const x = createTemplateHost(frame, {
      origin: "https://templates.argus.test",
      presentation: {
        template_source: "<p/>",
        template_hash: "b".repeat(64),
        detail_data: {},
      },
      context: { locale: "zh-CN", color_scheme: "light", tokens: {} },
      onReady: vi.fn(),
      onError: vi.fn(),
      onResize: vi.fn(),
    });
    hosts.push(x);
    const posted = vi
      .spyOn(frame.contentWindow!, "postMessage")
      .mockImplementation(() => {});
    frame.dispatchEvent(new Event("load"));
    expect(posted).not.toHaveBeenCalled();
    active[0]!.receive("ready");
    expect(frame.src).toContain("templates.argus.test");
    const navigated = vi
      .spyOn(frame.contentWindow!, "postMessage")
      .mockImplementation(() => {});
    frame.dispatchEvent(new Event("load"));
    expect(navigated).toHaveBeenCalledOnce();
  });
  it("correlates v2 reads and notifies data readiness without a business renderer", async () => {
    const x = mounted(TEMPLATE_PROTOCOL_V2),
      id = crypto.randomUUID();
    x.receive("ready");
    x.receive(
      "data_request",
      { operation: "result.detail", parameters: {} },
      2,
      undefined,
      id,
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(x.onRead).toHaveBeenCalledWith({
      operation: "result.detail",
      parameters: {},
    });
    expect(x.port.postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        version: TEMPLATE_PROTOCOL_V2,
        type: "data_response",
        request_id: id,
        payload: {
          result: { status: "ready", data: { name: "saved result" } },
        },
      }),
    );
    x.host.notifyData();
    expect(x.port.postMessage).toHaveBeenLastCalledWith(
      expect.objectContaining({ type: "data_available" }),
    );
    x.host.destroy();
  });
  it("rejects undeclared operations and replayed request identities", () => {
    const x = mounted(TEMPLATE_PROTOCOL_V2),
      id = crypto.randomUUID();
    x.receive(
      "data_request",
      { operation: "result.detail", parameters: {} },
      1,
      undefined,
      id,
    );
    x.receive(
      "data_request",
      { operation: "result.detail", parameters: {} },
      2,
      undefined,
      id,
    );
    expect(x.onRead).toHaveBeenCalledOnce();
    expect(x.onError).toHaveBeenCalledOnce();
    const y = mounted(TEMPLATE_PROTOCOL_V2);
    y.receive("data_request", {
      operation: "host.delete.commit",
      parameters: {},
    });
    expect(y.onRead).not.toHaveBeenCalled();
    expect(y.onError).toHaveBeenCalledOnce();
  });
  it.each([
    "confirm_action",
    "cancel_action",
    "action.invoke",
    "query.invoke",
    "fetch",
  ])("rejects %s without invoking host actions", (type) => {
    const x = mounted();
    x.receive(type);
    expect(x.onError).toHaveBeenCalledOnce();
    expect(x.onResource).not.toHaveBeenCalled();
    expect(x.port.close).toHaveBeenCalled();
  });
  it("preserves full document height and rejects repeated sequences", () => {
    const x = mounted();
    x.receive("ready");
    x.receive("resize", { height: 9000 }, 2);
    expect(x.onReady).toHaveBeenCalledOnce();
    expect(x.onResize).toHaveBeenCalledWith(9000);
    x.receive("resize", { height: 50 }, 2);
    expect(x.onError).toHaveBeenCalledOnce();
  });
  it.each([0, -1, Infinity, NaN, Number.MAX_VALUE, "9000"])(
    "rejects invalid resize %s without changing document geometry",
    (height) => {
      const x = mounted();
      x.receive("resize", { height });
      expect(x.onResize).not.toHaveBeenCalled();
      expect(x.onError).toHaveBeenCalledOnce();
    },
  );
  it("rejects unknown resources", () => {
    const x = mounted();
    x.receive("open_resource", { type: "host", id: "h2" });
    expect(x.onResource).not.toHaveBeenCalled();
    expect(x.onError).toHaveBeenCalledOnce();
  });
  it("rejects a wrong nonce", () => {
    const x = mounted();
    x.receive("ready", {}, 1, "x".repeat(32));
    expect(x.onReady).not.toHaveBeenCalled();
    expect(x.onError).toHaveBeenCalledOnce();
  });
  it("revokes the channel and ignores late messages after destruction", () => {
    const x = mounted();
    x.host.destroy();
    x.receive("ready");
    expect(x.onReady).not.toHaveBeenCalled();
    expect(x.frame.getAttribute("sandbox")).toBe("allow-scripts");
  });
  it("rejects malformed and oversized envelopes", () => {
    expect(isTemplateMessage({})).toBe(false);
    expect(
      isTemplateMessage({
        version: TEMPLATE_PROTOCOL,
        nonce: "a".repeat(32),
        sequence: 1,
        request_id: crypto.randomUUID(),
        type: "ready",
        payload: { text: "x".repeat(3 * 1024 * 1024) },
      }),
    ).toBe(false);
  });
});
