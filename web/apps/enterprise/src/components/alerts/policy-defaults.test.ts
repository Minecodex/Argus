import { describe, expect, it } from "vitest";
import type { AlertConfiguration } from "@argus/api-client";
import { defaultPolicyChannelIds } from "./policy-defaults";

function channel(
  id: string,
  enabled = true,
  isDefault = false,
): AlertConfiguration {
  return {
    id,
    kind: "channel",
    name: id,
    spec: {
      enabled,
      provider: "feishu",
      credential_id: "credential",
      default_for_new_policies: isDefault,
    },
    version: 1,
    generation: 1,
    health: "normal",
    created_at: "",
    updated_at: "",
  };
}
describe("new policy notification defaults", () => {
  it("uses arbitrary tenant IDs and excludes disabled and unmarked channels", () => {
    expect(
      defaultPolicyChannelIds([
        channel("tenant-primary", true, true),
        channel("tenant-disabled", false, true),
        channel("tenant-secondary", true, true),
        channel("c3"),
      ]),
    ).toEqual(["tenant-primary", "tenant-secondary"]);
  });
  it("does not guess a target when no defaults exist or mutate configuration", () => {
    const rows = [channel("c3"), channel("other")];
    const before = structuredClone(rows);
    expect(defaultPolicyChannelIds(rows)).toEqual([]);
    expect(defaultPolicyChannelIds([])).toEqual([]);
    expect(rows).toEqual(before);
  });
});
