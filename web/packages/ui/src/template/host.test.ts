// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { createTemplateHost } from "./host";
import { TEMPLATE_PROTOCOL, isTemplateMessage } from "./protocol";
class Port {
  onmessage: ((event: MessageEvent) => void) | null = null;
  postMessage = vi.fn();
  start = vi.fn();
  close = vi.fn();
}
const channels: { port1: Port; port2: Port }[] = [];
afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
  channels.length = 0;
  vi.useRealTimers();
});
function mounted() {
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
  const host = createTemplateHost(frame, {
    origin: "https://templates.argus.test",
    presentation: {
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
  });
  const posted = vi
    .spyOn(frame.contentWindow!, "postMessage")
    .mockImplementation(() => {});
  frame.dispatchEvent(new Event("load"));
  const hello = posted.mock.calls[0]![0] as { nonce: string };
  const port = channels[0]!.port1;
  const receive = (
    type: string,
    payload: Record<string, unknown> = {},
    sequence = 1,
    nonce = hello.nonce,
  ) =>
    port.onmessage?.({
      data: {
        version: TEMPLATE_PROTOCOL,
        nonce,
        sequence,
        request_id: crypto.randomUUID(),
        type,
        payload,
      },
    } as MessageEvent);
  return { host, frame, receive, onError, onReady, onResize, onResource, port };
}
describe("Template capability boundary", () => {
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
  it("accepts readiness and bounded resize, rejects repeated sequences", () => {
    const x = mounted();
    x.receive("ready");
    x.receive("resize", { height: 9000 }, 2);
    expect(x.onReady).toHaveBeenCalledOnce();
    expect(x.onResize).toHaveBeenCalledWith(2000);
    x.receive("resize", { height: 50 }, 2);
    expect(x.onError).toHaveBeenCalledOnce();
  });
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
