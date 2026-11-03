import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { createHash } from "node:crypto";
import { openSealedTool } from "./helpers/sealed-dashboard";

const directory = process.env.ARGUS_PLAN11_CAPABILITY_EVIDENCE;
test.skip(
  !directory,
  "requires current owned PostgreSQL capability response evidence",
);

for (const english of [false, true])
  for (const dark of [false, true]) {
    for (const status of ["not_permitted", "unavailable"]) {
      test(`capability partial ${status} ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width: 1440, height: 900 });
        const path = resolve(directory!, `capability-${status}.json`);
        const raw = readFileSync(path),
          data = JSON.parse(raw.toString("utf8"));
        expect(data.complete).toBe(false);
        expect(data.items).toHaveLength(1);
        expect(data.items[0].sections.dashboards.status).toBe(status);
        expect(data.items[0].sections.dashboards.items).toEqual([]);
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "observe.capabilities.get",
          data,
        );
        await expect(content.locator("body")).toContainText(
          english ? "Partial; continue querying" : "结果不完整，请继续查询",
        );
        await expect(content.locator("tbody>tr")).toHaveCount(1);
        await content
          .getByRole("button", {
            name: english ? "Expand" : "展开",
            exact: true,
          })
          .click();
        const section = (name: string) =>
          content
            .locator("details")
            .filter({
              has: content.locator("summary").filter({ hasText: name }),
            });
        const dashboards = section(
          english ? "Suitable dashboards" : "适用仪表盘",
        );
        await dashboards.locator("summary").click();
        await expect(dashboards).toContainText(status);
        await expect(dashboards).toContainText("false");
        await expect(dashboards.locator("a,table")).toHaveCount(0);
        const sources = section(english ? "Sources" : "采集来源");
        await sources.locator("summary").click();
        await expect(sources).toContainText("available");
        await expect(sources).toContainText("true");
        await expect(content.locator("body")).not.toContainText("closed pool");
        await expect(content.locator(".argus-template-spinner")).toHaveCount(0);
        await page.screenshot({
          path: info.outputPath("partial-capabilities.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify(
            {
              passed: true,
              layer: "L4",
              controlled_snapshot_of_actual_PG_response: true,
              actual_response_file: path,
              actual_response_sha256: createHash("sha256")
                .update(raw)
                .digest("hex"),
              locale: english ? "en-US" : "zh-CN",
              theme: dark ? "dark" : "light",
              dashboard_section_status: status,
              healthy_source_section_retained: true,
              actual_deployed_enterprise_role_tested: false,
            },
            null,
            2,
          ),
        );
      });
    }
  }
