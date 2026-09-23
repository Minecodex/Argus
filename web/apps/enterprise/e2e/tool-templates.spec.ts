import { readFileSync, writeFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import { templateOrigin } from "./origins";

const fixtures: {
  name: string;
  source: string;
  data: Record<string, unknown>;
  text?: string;
  localizedText?: { "zh-CN": string; "en-US": string };
  rows?: number;
  selector: string;
}[] = [
  {
    name: "Empty Host",
    source: "toolgateway/templates/resource",
    data: { items: [] },
    selector: "#result",
  },
  {
    name: "Host",
    source: "toolgateway/templates/resource",
    data: {
      items: [{ name: "host-proof", address: "10.0.0.1", platform: "linux" }],
    },
    text: "host-proof",
    selector: "table tbody tr",
  },
  {
    name: "Kubernetes",
    source: "toolgateway/templates/resource",
    data: {
      items: [
        {
          name: "pod-proof",
          namespace: "production",
          summary: { phase: "Running", ready: true },
        },
      ],
    },
    text: "Running",
    selector: "table tbody tr",
  },
  {
    name: "Connector",
    source: "toolgateway/templates/resource",
    data: {
      items: [{ name: "connector-proof", role: "bastion", status: "online" }],
    },
    text: "connector-proof",
    selector: "table tbody tr",
  },
  {
    name: "Metric",
    source: "telemetry/templates/metric",
    data: {
      data: {
        resultType: "matrix",
        result: [
          {
            metric: { service: "api" },
            values: [
              [1, "2"],
              [2, "5"],
            ],
          },
        ],
      },
    },
    selector: "svg polyline",
  },
  {
    name: "Log",
    source: "telemetry/templates/log",
    data: {
      result_type: "log_entries",
      data: [
        {
          timestamp: "2026-09-13",
          severity_text: "ERROR",
          service_name: "checkout",
          body: "log-proof",
        },
      ],
    },
    text: "log-proof",
    selector: "table tbody tr",
  },
  {
    name: "Trace",
    source: "telemetry/templates/trace",
    data: {
      data: {
        customAlias: {
          traces: [
            {
              traceId: "trace-proof",
              rootService: "api",
              rootOperation: "/checkout",
              spans: [{ spanId: "span-proof", serviceName: "payments" }],
            },
          ],
        },
      },
    },
    text: "span-proof",
    selector: "table tbody tr",
  },
  {
    name: "Pod logs",
    source: "toolgateway/templates/logs",
    data: { content: "pod-log-proof", truncated: true },
    text: "pod-log-proof",
    selector: "pre",
  },
  {
    name: "Preview details",
    source: "toolgateway/templates/preview",
    data: { preview: { change: "preview-proof", replicas: 3 } },
    text: "preview-proof",
    selector: "dl",
  },
];

const partialText = {
  "zh-CN": "结果不完整，请发送新消息继续查询。",
  "en-US": "Partial results. Send another message to query further.",
};
for (const fixture of fixtures.filter((item) =>
  ["Empty Host", "Host", "Metric", "Log", "Trace"].includes(item.name),
)) {
  fixtures.push({
    ...fixture,
    name: `Partial ${fixture.name}`,
    data: { ...fixture.data, partial: true },
    localizedText: partialText,
  });
}
for (const [name, source, selector, data] of [
  ["Empty Metric", "metric", "#chart", { data: { result: [] } }],
  ["Empty Log", "log", "#logs", { data: [] }],
  ["Empty Trace", "trace", "#traces", { data: {} }],
] as const) {
  fixtures.push({
    name,
    source: `telemetry/templates/${source}`,
    selector,
    data,
    localizedText: {
      "zh-CN": "没有可显示的数据",
      "en-US": "No data to display",
    },
  });
}
fixtures.push(
  {
    name: "Large Host",
    source: "toolgateway/templates/resource",
    selector: "table tbody tr",
    data: {
      items: Array.from({ length: 1001 }, (_, index) => ({
        name: `host-${index}`,
      })),
    },
    rows: 200,
    localizedText: {
      "zh-CN": "仅显示 1001 项中的前 200 项。请发送新消息缩小查询范围。",
      "en-US":
        "Showing the first 200 of 1001 items. Send another message to narrow the query.",
    },
  },
  {
    name: "Large Metric",
    source: "telemetry/templates/metric",
    selector: "svg polyline",
    // Below the 1 MiB presentation limit, above V8's function argument limit.
    data: {
      data: {
        result: [
          {
            values: Array.from({ length: 130000 }, (_, index) => [
              index % 2,
              index % 3,
            ]),
          },
        ],
      },
    },
  },
  {
    name: "Many Metric series",
    source: "telemetry/templates/metric",
    selector: "svg polyline",
    data: {
      data: {
        result: Array.from({ length: 25 }, () => ({
          values: [
            [1, 2],
            [2, 3],
          ],
        })),
      },
    },
    localizedText: {
      "zh-CN": "仅显示 25 项中的前 20 项。请发送新消息缩小查询范围。",
      "en-US":
        "Showing the first 20 of 25 items. Send another message to narrow the query.",
    },
  },
);

for (const fixture of fixtures) {
  for (const locale of ["zh-CN", "en-US"]) {
    for (const theme of ["light", "dark"]) {
      test(`${fixture.name} Tool template renders ${locale}/${theme} protocol data`, async ({
        page,
      }, testInfo) => {
        const source = readFileSync(
          new URL(
            `../../../../internal/${fixture.source}.html`,
            import.meta.url,
          ),
          "utf8",
        );
        await page.goto("/login");
        const renderStarted = await page.evaluate(
          async ({ source, data, locale, theme, origin }) => {
            const started = performance.now();
            const iframe = document.createElement("iframe");
            iframe.id = "tool-template-proof";
            iframe.setAttribute("sandbox", "allow-scripts");
            const loaded = new Promise<void>((resolve) =>
              iframe.addEventListener("load", () => resolve(), { once: true }),
            );
            iframe.src = `${origin}/?parent_origin=${encodeURIComponent(location.origin)}`;
            document.body.append(iframe);
            await loaded;
            const hash = Array.from(
              new Uint8Array(
                await crypto.subtle.digest(
                  "SHA-256",
                  new TextEncoder().encode(source),
                ),
              ),
              (x) => x.toString(16).padStart(2, "0"),
            ).join("");
            const channel = new MessageChannel();
            channel.port1.start();
            iframe.contentWindow!.postMessage(
              {
                version: "argus-template/v1",
                nonce: crypto.randomUUID(),
                sequence: 1,
                request_id: crypto.randomUUID(),
                type: "hello",
                payload: {
                  template_source: source,
                  template_hash: hash,
                  detail_data: data,
                  context: { locale, color_scheme: theme, tokens: {} },
                },
              },
              "*",
              [channel.port2],
            );
            return started;
          },
          { source, data: fixture.data, locale, theme, origin: templateOrigin },
        );
        const frame = page.frameLocator("#tool-template-proof");
        await expect(frame.locator(fixture.selector).first()).toBeVisible();
        const visibleMs = await page.evaluate(
          (started) => performance.now() - started,
          renderStarted,
        );
        const timingPath = testInfo.outputPath("template-visible-timing.json");
        writeFileSync(
          timingPath,
          JSON.stringify({
            fixture: fixture.name,
            locale,
            theme,
            visible_ms: visibleMs,
            budget_ms: 15000,
            detail_bytes: Buffer.byteLength(JSON.stringify(fixture.data)),
          }),
        );
        await testInfo.attach("template-visible-timing", {
          path: timingPath,
          contentType: "application/json",
        });
        // Match the host's initialization deadline, including iframe loading,
        // the handshake, rendering and the browser's visibility observation.
        expect(visibleMs).toBeLessThan(15000);
        if (fixture.name === "Empty Host")
          await expect(frame.locator("#result")).toHaveText(
            locale === "zh-CN" ? "没有可显示的资源" : "No resources to display",
          );
        if (fixture.text)
          await expect(
            frame.getByText(fixture.text, { exact: true }).first(),
          ).toBeVisible();
        if (fixture.localizedText)
          await expect(
            frame
              .getByText(fixture.localizedText[locale as "zh-CN" | "en-US"], {
                exact: true,
              })
              .first(),
          ).toBeVisible();
        if (fixture.rows !== undefined)
          await expect(frame.locator("table tbody tr")).toHaveCount(
            fixture.rows,
          );
        await expect(
          frame.getByRole("button", { name: /确认|取消|confirm|cancel/i }),
        ).toHaveCount(0);
      });
    }
  }
}
