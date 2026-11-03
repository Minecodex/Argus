import { test, expect } from "@playwright/test";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { openToolResults } from "./helpers/tool-results";

test.skip(
  process.env.ARGUS_PLAN11_MODEL_REAL !== "1",
  "requires an owned deployed Plan11 model conversation",
);
test.use({ trace: "off", viewport: { width: 1440, height: 1000 } });

test("real model observation results and report remain readable in Chatbox", async ({
  page,
  context,
}, info) => {
  test.setTimeout(120_000);
  const conversation = process.env.ARGUS_PLAN11_MODEL_CONVERSATION!;
  const host = process.env.ARGUS_PLAN11_HOST_ID!;
  const hostName = process.env.ARGUS_PLAN11_HOST_NAME!;
  await context.addCookies(JSON.parse(process.env.ARGUS_PLAN11_MODEL_COOKIES!));
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`/?c=${conversation}`);
  await expect(page).not.toHaveURL(/\/login/);
  await openToolResults(page);
  const frames = page.locator(".argus-tool-presentation__frame");
  await expect.poll(() => frames.count()).toBeGreaterThanOrEqual(3);
  const views: string[] = [];
  for (let index = 0; index < (await frames.count()); index++) {
    const body = frames.nth(index).contentFrame().locator("body");
    await expect(body).toBeVisible();
    views.push(await body.innerText());
    await frames
      .nth(index)
      .screenshot({ path: info.outputPath(`view-${index}.png`) });
  }
  const text = views.join("\n");
  expect(text).toMatch(/观测能力|Observation capabilities/);
  expect(text).toMatch(/指标.*目录|Metrics.*catalog/);
  expect(text).toContain(hostName);
  const links = page.locator(".argus-chat-files a[download]");
  await expect.poll(() => links.count()).toBeGreaterThan(0);
  const downloads: Array<{
    name: string;
    bytes: number;
    sha256: string;
    delivery: boolean;
  }> = [];
  let queryChecked = false;
  for (let index = 0; index < (await links.count()); index++) {
    const link = links.nth(index);
    const href = await link.getAttribute("href");
    if (!href) continue;
    const pending = page.waitForEvent("download");
    await link.click();
    const download = await pending;
    const name = download.suggestedFilename().replace(/[^\w.-]/g, "_");
    const path = info.outputPath(name);
    await download.saveAs(path);
    const raw = await readFile(path);
    expect(raw.length).toBeGreaterThan(0);
    const delivery = href.includes("/deliveries/");
    if (delivery)
      expect(raw.toString("utf8")).toMatch(new RegExp(`${host}|${hostName}`));
    if (name.endsWith(".json")) {
      const data = JSON.parse(raw.toString("utf8"));
      if (data.query_context) {
        expect(data.query_context.resources).toEqual([
          {
            type: "host",
            id: host,
          },
        ]);
        expect(data.status).toBe("success");
        queryChecked = true;
      }
    }
    downloads.push({
      name,
      bytes: raw.length,
      sha256: createHash("sha256").update(raw).digest("hex"),
      delivery,
    });
  }
  expect(queryChecked).toBe(true);
  expect(downloads.some((file) => file.delivery)).toBe(true);
  expect(errors).toEqual([]);
  await page.screenshot({ path: info.outputPath("chatbox.png") });
  await writeFile(
    info.outputPath("receipt.json"),
    JSON.stringify(
      {
        passed: true,
        conversation,
        views,
        downloads,
        scope:
          "actual_chatbox_views_and_downloads_not_model_conclusion_quality",
      },
      null,
      2,
    ),
  );
});
