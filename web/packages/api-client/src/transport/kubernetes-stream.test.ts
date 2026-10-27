// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { KubernetesTerminalConnection } from "./kubernetes-stream";
import type { KubernetesStreamTicket } from "../kubernetes-workbench";
class Socket extends EventTarget {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 1;
  sent: string[] = [];
  constructor(readonly url: string) {
    super();
    Socket.instances.push(this);
  }
  send(value: string) {
    this.sent.push(value);
  }
  close() {}
  open() {
    this.dispatchEvent(new Event("open"));
  }
  frame(value: object) {
    this.dispatchEvent(
      new MessageEvent("message", {
        data: JSON.stringify({
          protocol: "argus.kubernetes_stream/v1",
          ...value,
        }),
      }),
    );
  }
}
const ticket = (): KubernetesStreamTicket => ({
  ticket: "a".repeat(43),
  ticket_expires_at: new Date(Date.now() + 30000).toISOString(),
  protocol_version: "argus.kubernetes_stream/v1",
  websocket_url:
    "wss://enterprise.example.test/v1/kubernetes/sessions/00000000-0000-0000-0000-000000000001",
  session: {
    id: "00000000-0000-0000-0000-000000000001",
    cluster_id: "cluster",
    namespace_id: "ns",
    namespace: "orders",
    namespace_uid: "ns-uid",
    pod: "api",
    pod_uid: "pod-uid",
    container: "main",
    restart_count: 0,
    mode: "exec",
    shell: "sh",
    status: "pending",
    version: 1,
    expires_at: new Date(Date.now() + 300000).toISOString(),
    bytes_in: 0,
    bytes_out: 0,
  },
});
beforeEach(() => {
  Socket.instances = [];
  vi.stubGlobal("WebSocket", Socket);
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => vi.unstubAllGlobals());
it("keeps the one-use ticket out of the URL/storage and decodes split UTF-8 bytes", () => {
  const chunks: string[] = [],
    states: string[] = [];
  const c = new KubernetesTerminalConnection({
    output: (v) => chunks.push(v),
    state: (s) => states.push(s),
  });
  const value = ticket();
  c.connect(value);
  const ws = Socket.instances[0]!;
  ws.open();
  expect(value.ticket).toBe("");
  expect(ws.url).not.toContain("a".repeat(43));
  ws.frame({
    type: "server_ready",
    sequence: 1,
    mode: "exec",
    nonce: "n".repeat(32),
  });
  ws.frame({
    type: "output",
    sequence: 2,
    data: btoa(String.fromCharCode(0xe4)),
  });
  ws.frame({
    type: "output",
    sequence: 3,
    data: btoa(String.fromCharCode(0xb8, 0xad)),
  });
  expect(chunks.join("")).toBe("中");
  c.input("pwd\n");
  c.resize(1, 1);
  const input = JSON.parse(ws.sent[1]!);
  expect(input.nonce).toBe("n".repeat(32));
  expect(input.sequence).toBe(2);
  expect(JSON.parse(ws.sent[2]!).cols).toBe(20);
  expect(
    JSON.stringify({ local: localStorage, session: sessionStorage }),
  ).not.toContain("a".repeat(43));
  c.close();
  expect(states.at(-1)).toBe("disconnected");
});
it("rejects replayed server sequences before exposing their output", () => {
  const output = vi.fn(),
    state = vi.fn();
  const c = new KubernetesTerminalConnection({ output, state });
  c.connect(ticket());
  const ws = Socket.instances[0]!;
  ws.open();
  ws.frame({
    type: "server_ready",
    sequence: 1,
    mode: "exec",
    nonce: "n".repeat(32),
  });
  ws.frame({ type: "output", sequence: 1, data: btoa("must-not-be-rendered") });
  expect(output).not.toHaveBeenCalled();
  expect(state).toHaveBeenLastCalledWith(
    "disconnected",
    "KUBERNETES_STREAM_PROTOCOL_ERROR",
  );
});
it("rejects tokens in URL queries and non-TLS remote endpoints", () => {
  for (const url of [
    "ws://remote.example.test/v1/kubernetes/sessions/id",
    ticket().websocket_url + "?ticket=private",
  ]) {
    const c = new KubernetesTerminalConnection({
      output: () => undefined,
      state: () => undefined,
    });
    expect(() => c.connect({ ...ticket(), websocket_url: url })).toThrow();
    expect(Socket.instances).toHaveLength(0);
  }
});
