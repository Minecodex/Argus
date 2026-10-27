import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

test("alert template generation is identical for LF and CRLF checkouts", async (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "argus-alert-generation-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const dir of [
    "api/openapi/components",
    "api/schemas/alerts",
    "web/packages/api-client/src/generated",
  ]) {
    fs.mkdirSync(path.join(root, dir), { recursive: true });
  }
  fs.writeFileSync(
    path.join(root, "api/openapi/components/alerts.yaml"),
    JSON.stringify({ components: { schemas: { AlertPolicySpec: { type: "object" } } } }),
  );
  const template = '<section aria-label="告警">\n  <p>Alert result</p>\n</section>\n';
  const generator = fileURLToPath(new URL("./generate-alert-schema.mjs", import.meta.url));
  const outputs = [];
  for (const input of [template, template.replaceAll("\n", "\r\n")]) {
    fs.writeFileSync(path.join(root, "api/schemas/alerts/result.html"), input);
    execFileSync(process.execPath, [generator], { cwd: root, stdio: "pipe" });
    outputs.push(fs.readFileSync(path.join(root, "web/packages/api-client/src/generated/alert-result-template.ts"), "utf8"));
  }
  assert.equal(outputs[1], outputs[0]);
  const generated = await import(`data:text/javascript;base64,${Buffer.from(outputs[0]).toString("base64")}`);
  assert.equal(generated.alertResultTemplate, template);
});
