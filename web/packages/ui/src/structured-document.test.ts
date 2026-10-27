import { describe, expect, it } from "vitest";
import {
  formatStructuredDocument,
  parseStructuredDocument,
  StructuredDocumentError,
} from "./structured-document";

describe("structured resource documents", () => {
  it("round trips a Kubernetes manifest with JSON value types", () => {
    const source =
      'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: orders\ndata:\n  enabled: "true"\n  mode: "0123"\n';
    const value = parseStructuredDocument(source, "yaml");
    expect(
      parseStructuredDocument(formatStructuredDocument(value, "json"), "json"),
    ).toEqual(value);
    expect(
      parseStructuredDocument(formatStructuredDocument(value, "yaml"), "yaml"),
    ).toEqual(value);
    expect(value.data).toEqual({ enabled: "true", mode: "0123" });
  });
  for (const source of [
    "a: 1\na: 2",
    "a: !custom value",
    "a: &cycle [*cycle]",
    "---\na: 1\n---\nb: 2",
    "a: .inf",
    "a: 9007199254740993",
    "- scalar",
  ]) {
    it(`rejects ambiguous or non-JSON input ${source.slice(0, 20)}`, () =>
      expect(() => parseStructuredDocument(source, "yaml")).toThrow(
        StructuredDocumentError,
      ));
  }
  it("rejects duplicate JSON keys rather than silently replacing a value", () =>
    expect(() => parseStructuredDocument('{"a":1,"a":2}', "json")).toThrow());
  it("does not include private parser input in diagnostics", () => {
    try {
      parseStructuredDocument("a: !private-canary-secret bad", "yaml");
      expect.fail("accepted");
    } catch (error) {
      expect(String(error)).not.toContain("canary");
      expect(JSON.stringify(error)).not.toContain("canary");
    }
  });
  it("bounds document size and alias expansion", () => {
    expect(() =>
      parseStructuredDocument('a: "' + "x".repeat(1 << 20) + '"', "yaml"),
    ).toThrow();
    const lines = ["a: &a [x,x,x,x,x,x,x,x,x,x]"];
    for (let i = 1; i < 6; i++)
      lines.push(
        `${String.fromCharCode(97 + i)}: &${String.fromCharCode(97 + i)} [${Array(
          10,
        )
          .fill("*" + String.fromCharCode(96 + i))
          .join(",")}]`,
      );
    expect(() => parseStructuredDocument(lines.join("\n"), "yaml")).toThrow(
      StructuredDocumentError,
    );
  });
});
