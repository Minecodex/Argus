// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { trustedHello, csp } from "./runtime";
import { TEMPLATE_PROTOCOL } from "@argus/ui/template";
describe("Template bootstrap", () => {
  it("requires the exact parent origin, source, nonce and transferred port", () => {
    const parent = {} as Window;
    const data = {
      version: TEMPLATE_PROTOCOL,
      type: "hello",
      nonce: "a".repeat(32),
      sequence: 1,
      request_id: "b".repeat(32),
      payload: {},
    };
    const event = {
      source: parent,
      origin: "https://argus.test",
      ports: [{}],
      data,
    } as unknown as MessageEvent;
    expect(trustedHello(event, parent, "https://argus.test")).toBe(true);
    expect(
      trustedHello(
        { ...event, origin: "https://other.test" } as MessageEvent,
        parent,
        "https://argus.test",
      ),
    ).toBe(false);
    expect(
      trustedHello(
        { ...event, source: window } as MessageEvent,
        parent,
        "https://argus.test",
      ),
    ).toBe(false);
    expect(
      trustedHello(
        { ...event, ports: [] } as MessageEvent,
        parent,
        "https://argus.test",
      ),
    ).toBe(false);
  });
  it("disables network, frames, forms, workers and dynamic code", () => {
    const policy = csp("n".repeat(32));
    for (const rule of [
      "connect-src 'none'",
      "frame-src 'none'",
      "form-action 'none'",
      "worker-src 'none'",
    ])
      expect(policy).toContain(rule);
    expect(policy).not.toContain("unsafe-eval");
    expect(policy).toContain("script-src 'nonce-");
  });
});
