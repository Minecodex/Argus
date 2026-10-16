// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, expect, it } from "vitest";
import * as renderers from "./renderers";
import { fixtures, presentTool } from "./test-support";
const source = readFileSync(
  resolve(process.cwd(), "../../../api/schemas/alerts/result.html"),
  "utf8",
);
afterEach(() => document.body.replaceChildren());
it("keeps event lifecycle distinct from pending notification status and formats the observed value", () => {
  const template = document.createElement("template");
  template.innerHTML = source;
  const script = template.content.querySelector("script")!.textContent!;
  template.content.querySelector("script")!.remove();
  document.body.replaceChildren(template.content);
  new Function("ArgusTemplate", script)({
    ...renderers,
    onData: (callback: (data: unknown, context: unknown) => void) =>
      callback(
        {
          tool: "alert.event.query",
          page: {
            resource_ids: ["host"],
            items: [
              {
                lifecycle: "firing",
                resource_ids: ["host"],
                state: {
                  name: "CPU 演示告警",
                  value: 4.47790828867494,
                  unit: "%",
                  notification_state: "pending",
                },
              },
            ],
          },
          resources: [{ id: "host", name: "argus-test-host" }],
        },
        { locale: "zh-CN" },
      ),
  });
  expect(
    document.querySelector(".argus-alert-result__badge")?.textContent,
  ).toBe("活跃告警");
  expect(document.body.textContent).toContain("4.48%");
  expect(document.body.textContent).toContain("通知：待发送");
  expect(document.body.textContent).toContain("argus-test-host");
});

it("renders the historical policy and actual resource without replacing it with current configuration", () => {
  const data = fixtures.bindings.find(
    (row) => row.Tool === "alert.event.get",
  )!.Data;
  presentTool("alert.event.get", {
    ...data,
    current_configuration: { name: "Changed current policy", version: 4 },
  });
  expect(document.body.textContent).toContain("Host resource pressure / 3");
  expect(document.body.textContent).toContain("db-prod-01");
  expect(document.body.textContent).toContain("91 % / 85 %");
  expect(document.body.textContent).toContain("maintenance");
  expect(document.body.textContent).toContain("触发时策略快照");
  expect(document.body.textContent).not.toContain("Changed current policy");
});

it("keeps failed notification content and its delivery error readable", () => {
  presentTool("alert.notification.query", undefined, "en-US");
  expect(document.body.textContent).toContain("CPU pressure on db-prod-01");
  expect(document.body.textContent).toContain("CPU 91% > threshold 85%");
  expect(document.body.textContent).toContain("Failed");
  expect(document.body.textContent).toContain("SMTP connection refused");
});
