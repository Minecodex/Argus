import { expect, test } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { openSealedTool } from "./helpers/sealed-dashboard";

const directory = process.env.ARGUS_PLAN11_DISCOVERY_EVIDENCE;
test.skip(
  !directory,
  "requires current owned PostgreSQL filtered directory responses",
);

for (const english of [false, true])
  for (const dark of [false, true]) {
    for (const stage of ["first", "second", "empty"]) {
      test(`filtered discovery ${stage} ${english ? "en" : "zh"} ${dark ? "dark" : "light"}`, async ({
        page,
      }, info) => {
        await page.setViewportSize({ width: 1440, height: 900 });
        const file = resolve(directory!, `discovery-${stage}.json`);
        const raw = readFileSync(file),
          source = JSON.parse(raw.toString("utf8"));
        expect(source.actual_postgres).toBe(true);
        const { content } = await openSealedTool(
          page,
          english,
          dark,
          "observe.resources.list",
          {
            ...source.result,
            request_context: source.input,
          },
        );
        const scope = content.locator("details").filter({
          has: content
            .locator("summary")
            .filter({ hasText: english ? "Query scope" : "本次查询条件" }),
        });
        await scope.locator("summary").click();
        for (const filter of ["monitoring_target", "database", "same-name"])
          await expect(scope).toContainText(filter);
        await expect(scope).toContainText(
          stage === "empty" ? "redis" : "mysql",
        );
        if (source.input.cursor) {
          await expect(scope).not.toContainText(source.input.cursor);
          await expect(scope).toContainText(
            english ? "Continuation page" : "续查页",
          );
        }
        const rows = content.locator("tbody>tr");
        if (stage === "empty") {
          expect(source.result.items).toHaveLength(0);
          await expect(rows).toHaveCount(0);
          await expect(content.locator("body")).toContainText(
            english ? "No data in this range" : "当前范围没有数据",
          );
          await expect(
            content.locator(".argus-template-pagination"),
          ).toContainText("0");
        } else {
          expect(source.result.items).toHaveLength(1);
          const item = source.result.items[0];
          expect(item.family).toBe("database");
          expect(item.integrations).toEqual(["mysql"]);
          await expect(rows).toHaveCount(1);
          await expect(rows.first()).toContainText("same-name");
          await expect(rows.first()).toContainText("mysql");
          await content
            .getByRole("button", {
              name: english ? "Expand" : "展开",
              exact: true,
            })
            .click();
          await expect(content.locator("tbody")).toContainText(item.ref.id);
        }
        if (stage === "first") {
          expect(source.result.has_more).toBe(true);
          expect(source.result.complete).toBe(false);
          await expect(content.locator("body")).toContainText(
            english
              ? "this page is not the full inventory"
              : "本页不是全部数据",
          );
        } else {
          expect(source.result.has_more).toBe(false);
          await expect(content.locator("body")).not.toContainText(
            english
              ? "this page is not the full inventory"
              : "本页不是全部数据",
          );
        }
        await page.screenshot({
          path: info.outputPath("filtered-directory.png"),
        });
        writeFileSync(
          info.outputPath("receipt.json"),
          JSON.stringify(
            {
              passed: true,
              layer: "L4",
              snapshot_of_actual_PG_response: true,
              source_file: file,
              source_sha256: createHash("sha256").update(raw).digest("hex"),
              stage,
              locale: english ? "en-US" : "zh-CN",
              theme: dark ? "dark" : "light",
              applied_conditions_visible: true,
              completeness_and_continuation_correct: true,
              actual_deployed_model_or_receiver_claimed: false,
            },
            null,
            2,
          ),
        );
      });
    }
  }
