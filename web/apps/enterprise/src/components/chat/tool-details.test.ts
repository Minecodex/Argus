import { expect, it } from "vitest";
import { toolDetailsText } from "./tool-details";
it("retains actual business parameters but hides private credentials from trace details", () => {
  const details = toolDetailsText({
    resource_ids: ["host"],
    credential_id: "reference",
    query: "cpu",
    nested: {
      password: "must-not-render",
      api_key: "secret",
      access_token: "token",
    },
  });
  expect(details).toContain("reference");
  expect(details).toContain("cpu");
  expect(details).not.toContain("must-not-render");
  expect(details).not.toContain('"secret"');
  expect(details).not.toContain('"token"');
  expect(details).toContain("[redacted]");
});
