import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test } from "@playwright/test";
import { templateOrigin } from "./origins";

const catalog = JSON.parse(
  readFileSync(
    resolve(process.cwd(), "../template-runtime/src/fixtures.generated.json"),
    "utf8",
  ),
) as {
  assets: Record<string, string>;
  bindings: {
    Tool: string;
    Entry: string;
    Hash: string;
    Reads: string[];
    Data: Record<string, unknown>;
  }[];
};

// The full Chatbox matrix is in tool-template-v2.spec.ts. This gate keeps the
// runtime's actual immutable asset/handshake/deadline checks independently.
for (const tool of [
  "host.list",
  "telemetry.promql.query",
  "telemetry.kql.query",
  "telemetry.skywalking.trace",
])
  for (const locale of ["zh-CN", "en-US"])
    for (const theme of ["light", "dark"]) {
      test(`${tool} immutable v2 runtime ${locale}/${theme}`, async ({
        page,
      }, info) => {
        const binding = catalog.bindings.find((item) => item.Tool === tool)!;
        const source = catalog.assets[binding.Hash]!;
        await page.goto("/login");
        const started = await page.evaluate(
          async ({ binding, source, locale, theme, origin }) => {
            const start = performance.now();
            const iframe = document.createElement("iframe");
            iframe.id = "tool-template-proof";
            iframe.setAttribute("sandbox", "allow-scripts");
            iframe.style.width = "800px";
            iframe.style.height = "600px";
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
              if (message.type !== "data_request") return;
              if (!binding.Reads.includes(message.payload.operation))
                throw new Error("Undeclared read");
              const data =
                message.payload.operation === "result.page"
                  ? {
                      items: ((binding.Data.items as unknown[]) ?? []).slice(
                        0,
                        25,
                      ),
                      offset: 0,
                    }
                  : binding.Data;
              channel.port1.postMessage({
                version: "argus-template/v2",
                nonce,
                sequence: ++sequence,
                request_id: message.request_id,
                type: "data_response",
                payload: { result: { status: "ready", data } },
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
                  context: { locale, color_scheme: theme, tokens: {} },
                },
              },
              "*",
              [channel.port2],
            );
            return start;
          },
          { binding, source, locale, theme, origin: templateOrigin },
        );
        const frame = page.frameLocator("#tool-template-proof");
        await expect(frame.locator("table tbody tr").first()).toBeVisible();
        await expect(frame.getByRole("alert")).toHaveCount(0);
        await expect(
          frame.getByRole("button", { name: /确认|取消|confirm|cancel/i }),
        ).toHaveCount(0);
        const visibleMs = await page.evaluate(
          (start) => performance.now() - start,
          started,
        );
        expect(visibleMs).toBeLessThan(15000);
        writeFileSync(
          info.outputPath("template-visible-timing.json"),
          JSON.stringify({
            tool,
            locale,
            theme,
            hash: binding.Hash,
            visible_ms: visibleMs,
            budget_ms: 15000,
          }),
        );
      });
    }
