import { test, expect } from "@playwright/test";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { openToolResults } from "./helpers/tool-results";

test.skip(
  process.env.ARGUS_PLAN11_INTEGRATION_REAL !== "1",
  "requires actual owned receiver data and native tool run",
);
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });
test("actual integration discovery, source, query and sealed download in Chatbox", async ({
  page,
  context,
}, info) => {
  test.setTimeout(120_000);
  const conversation = process.env.ARGUS_PLAN11_INTEGRATION_CONVERSATION!;
  const target = process.env.ARGUS_PLAN11_INTEGRATION_ID!;
  const metric = process.env.ARGUS_PLAN11_INTEGRATION_METRIC!;
  await context.addCookies(
    JSON.parse(process.env.ARGUS_PLAN11_INTEGRATION_COOKIES!),
  );
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(`/?c=${conversation}`);
  await expect(page).not.toHaveURL(/\/login/);
  await openToolResults(page);
  const frames = page.locator(".argus-tool-presentation__frame");
  const views: string[] = [];
  for (let i = 0; i < (await frames.count()); i++) {
    const body = frames.nth(i).contentFrame().locator("body");
    await expect(body).toBeVisible();
    views.push(await body.innerText());
    await frames.nth(i).screenshot({ path: info.outputPath(`view-${i}.png`) });
  }
  const text = `${await page.locator("body").innerText()}\n${views.join("\n")}`;
  expect(text).toContain(metric);
  const links = page.locator(".argus-chat-files a[download]");
  await expect.poll(() => links.count()).toBeGreaterThan(0);
  let checked = false;
  for (let i = 0; i < (await links.count()); i++) {
    const pending = page.waitForEvent("download");
    await links.nth(i).click();
    const file = await pending;
    const path = info.outputPath(
      file.suggestedFilename().replace(/[^\w.-]/g, "_"),
    );
    await file.saveAs(path);
    const raw = await readFile(path);
    if (
      createHash("sha256").update(raw).digest("hex") !==
      process.env.ARGUS_PLAN11_INTEGRATION_HASH
    )
      continue;
    const value = JSON.parse(raw.toString("utf8"));
    expect(value.status).toBe("success");
    expect(value.query_context.resources).toEqual([
      { type: "monitoring_target", id: target },
    ]);
    expect(value.argus_meta.loaded_samples).toBeGreaterThan(0);
    expect(value.data.result.length).toBeGreaterThan(0);
    for (const row of value.data.result) {
      expect(row.metric.argus_resource_id).toBe(target);
      expect(row.metric.__name__).toBe(metric);
    }
    checked = true;
  }
  expect(checked).toBe(true);
  expect(errors).toEqual([]);
  await page.screenshot({ path: info.outputPath("chatbox.png") });
  await writeFile(
    info.outputPath("receipt.json"),
    JSON.stringify(
      {
        passed: true,
        conversation,
        target,
        metric,
        views,
        actual_model_evaluated: true,
        query_hash: process.env.ARGUS_PLAN11_INTEGRATION_HASH,
      },
      null,
      2,
    ),
  );
});
