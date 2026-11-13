import { describe, it, expect } from "vitest";
import {
  resourcePatch,
  rebaseResource,
  ResourceRebaseConflict,
} from "./resource-patch";
describe("resource forms preserve concurrent state", () => {
  it("submits only edited fields, preserving unknown native configuration", () => {
    const base = {
      metadata: { name: "api", labels: { owner: "a" } },
      spec: {
        replicas: 2,
        strategy: { type: "RollingUpdate" },
        template: { spec: { securityContext: { runAsNonRoot: true } } },
      },
    };
    expect(
      resourcePatch(base, { ...base, spec: { ...base.spec, replicas: 3 } }),
    ).toEqual({ spec: { replicas: 3 } });
    expect(
      resourcePatch({ data: { a: "1", b: "2" } }, { data: { b: "2" } }),
    ).toEqual({ data: { a: null } });
  });
  it("rebases untouched container fields and additions without replaying a write", () => {
    const old = [{ name: "api", image: "one", env: ["a"] }],
      draft = [{ name: "api", image: "two", env: ["a"] }],
      fresh = [
        { name: "api", image: "one", env: ["b"] },
        { name: "sidecar", image: "side" },
      ];
    expect(rebaseResource(old, draft, fresh)).toEqual([
      { name: "api", image: "two", env: ["b"] },
      { name: "sidecar", image: "side" },
    ]);
  });
  it("requires a decision before restoring an edited container removed concurrently", () => {
    expect(() =>
      rebaseResource(
        [{ name: "api", image: "one" }],
        [{ name: "api", image: "two" }],
        [],
      ),
    ).toThrow(ResourceRebaseConflict);
    expect(
      rebaseResource(
        [{ name: "api", image: "one" }],
        [{ name: "api", image: "two" }],
        [],
        true,
      ),
    ).toEqual([{ name: "api", image: "two" }]);
  });
  it("does not restore an untouched item removed concurrently", () => {
    expect(
      rebaseResource(
        [
          { name: "a", image: "one" },
          { name: "b", image: "one" },
        ],
        [
          { name: "a", image: "one" },
          { name: "b", image: "two" },
        ],
        [{ name: "b", image: "one" }],
      ),
    ).toEqual([{ name: "b", image: "two" }]);
  });
  it("preserves input and reports a conflicting scalar", () => {
    const draft = { spec: { replicas: 3 } };
    expect(() =>
      rebaseResource({ spec: { replicas: 2 } }, draft, {
        spec: { replicas: 4 },
      }),
    ).toThrow("$.spec.replicas");
    expect(draft.spec.replicas).toBe(3);
    expect(
      rebaseResource(
        { spec: { replicas: 2 } },
        draft,
        { spec: { replicas: 4 } },
        true,
      ),
    ).toEqual(draft);
  });
});
