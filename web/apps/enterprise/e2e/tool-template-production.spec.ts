import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const catalog = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
);
for (const tool of [
  "host.list",
  "telemetry.promql.query",
  "telemetry.dashboard.query",
])
  test("production bundled opaque runtime: " + tool, async ({ page }) => {
    const origin = process.env.ARGUS_TEMPLATE_BUILD_ORIGIN;
    test.skip(!origin, "Requires the real compiled Runtime preview");
    const binding = catalog.bindings.find(
      (row: { Tool: string }) => row.Tool === tool,
    );
    if (tool === "telemetry.dashboard.query") {
      for (const panel of binding.Data.panels) {
        panel.display = null;
        panel.thresholds = null;
      }
    }
    await page.goto("/login");
    await page.evaluate(
      async ({ origin, binding, source }) => {
        const iframe = document.createElement("iframe");
        iframe.id = "production-runtime";
        iframe.setAttribute("sandbox", "allow-scripts");
        iframe.style.width = "900px";
        iframe.style.height = "700px";
        const loaded = new Promise<void>((resolve) =>
          iframe.addEventListener("load", () => resolve(), { once: true }),
        );
        iframe.src = `${origin}/?parent_origin=${encodeURIComponent(location.origin)}`;
        document.body.append(iframe);
        await loaded;
        const channel = new MessageChannel(),
          nonce = crypto.randomUUID();
        let sequence = 1;
        channel.port1.onmessage = (event) => {
          const message = event.data;
          if (message.type === "error")
            throw new Error(String(message.payload.code));
          if (message.type !== "data_request") return;
          channel.port1.postMessage({
            version: "argus-template/v2",
            nonce,
            sequence: ++sequence,
            request_id: message.request_id,
            type: "data_response",
            payload: {
              result: {
                status: "ready",
                data:
                  message.payload.operation === "result.page"
                    ? { items: binding.Data.items ?? [], offset: 0 }
                    : binding.Data,
              },
            },
          });
        };
        channel.port1.start();
        iframe.contentWindow!.postMessage(
          {
            version: "argus-template/v2",
            nonce,
            sequence,
            request_id: crypto.randomUUID(),
            type: "hello",
            payload: {
              runtime: "argus-template/v2",
              template_source: source,
              template_hash: binding.Hash,
              read_operations: binding.Reads,
              detail_data: binding.Data,
              context: { locale: "zh-CN", color_scheme: "light", tokens: {} },
            },
          },
          "*",
          [channel.port2],
        );
      },
      { origin: origin!, binding, source: catalog.assets[binding.Hash] },
    );
    const content = page.frameLocator("#production-runtime");
    await expect(
      content.locator("#argus-template-root h3").first(),
    ).toBeVisible({ timeout: 15000 });
    if (tool !== "host.list") {
      await expect
        .poll(() => content.locator("canvas").count())
        .toBeGreaterThan(0);
      for (const plot of await content.locator("canvas").all())
        expect((await plot.boundingBox())!.height).toBeGreaterThan(120);
    }
    await expect(content.getByRole("alert")).toHaveCount(0);
  });
