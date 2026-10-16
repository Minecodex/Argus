import { test, expect, type Page, type FrameLocator } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { captureRegion } from "./helpers/plan6-capture";
import { openToolResults } from "./helpers/tool-results";
import fs from "node:fs/promises";
import path from "node:path";
import { createHash } from "node:crypto";

const enabled = process.env.ARGUS_PLAN6_VISUAL === "1";
const repository = path.resolve("../../..");
const runId = process.env.ARGUS_PLAN6_VISUAL_RUN_ID ?? `plan6-${Date.now()}`;
const output = path.resolve(
  repository,
  process.env.ARGUS_PLAN6_VISUAL_OUTPUT ??
    `artifacts/plan6-continuation-20261009-v/visual/${runId}`,
);
const baseline = path.resolve(
  repository,
  process.env.ARGUS_PLAN6_VISUAL_BASELINE ??
    "artifacts/plan6-continuation-20261009-v/baseline",
);
const tabs = [
  "active",
  "history",
  "deliveries",
  "policies",
  "noise",
  "channels",
] as const;
const sizes = [
  { width: 1280, height: 800 },
  { width: 1440, height: 900 },
  { width: 1920, height: 1080 },
];
const titles = {
  zh: [
    "当前告警",
    "告警历史",
    "通知记录",
    "告警策略",
    "屏蔽与抑制",
    "通知配置",
  ],
  en: [
    "Active alerts",
    "Alert history",
    "Deliveries",
    "Alert policies",
    "Silences",
    "Notifications",
  ],
};

async function demoRoute(page: Page) {
  const files = new Map([
    ["/docs/plan6/demo.html", "text/html"],
    ["/docs/plan6/demo.js", "application/javascript"],
    ["/docs/plan6/demo.css", "text/css"],
    ["/web/packages/design-tokens/src/tokens.css", "text/css"],
  ]);
  await page.route("**/*", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    const contentType = files.get(pathname);
    if (contentType)
      return route.fulfill({
        contentType,
        body: await fs.readFile(path.join(baseline, pathname)),
      });
    return route.continue();
  });
}

async function geometry(page: Page | FrameLocator, rootSelector: string) {
  return page.locator(rootSelector).evaluate((root) => {
    const styles = [
      "fontFamily",
      "fontSize",
      "fontWeight",
      "lineHeight",
      "color",
      "backgroundColor",
      "borderColor",
      "borderRadius",
      "boxShadow",
      "paddingTop",
      "paddingBottom",
      "paddingLeft",
      "paddingRight",
      "gap",
      "height",
    ] as const;
    const selectors = [
      ".argus-alert-hero",
      ".argus-alert-description",
      ".argus-alert-stats",
      ".argus-alert-stat",
      ":is(.argus-alert-tabs, [role=tablist]):not(.argus-alert-subtabs):not(.argus-alert-detail-tabs [role=tablist])",
      ".argus-alert-subtabs, .argus-alert-detail-tabs [role=tablist]",
      ".argus-alert-toolbar",
      ":is(.argus-alert-table, .argus-table--dense)",
      ":is(.argus-alert-table-wrap, .argus-table-wrap)",
      "thead tr",
      "thead th",
      "tbody tr",
      "tbody td",
      ".argus-alert-table-footer",
      ".argus-alert-event-cell button",
      ".argus-alert-tag",
      ".argus-alert-labels",
      ".argus-alert-actions button, .argus-alert-toolbar > button, tbody td:last-child button, .argus-alert-rule-head button, .argus-alert-form-divider > button, .argus-preview-card__footer button, .argus-dialog__footer button, .argus-alert-modal-head button, .argus-overlay-close",
      ".argus-alert-channel-head",
      ".argus-alert-code-block",
      ".argus-alert-detail-value",
      ".argus-alert-drawer-section h3",
      ".argus-alert-form-divider h3",
      ".argus-alert-timeline li",
      "p.argus-alert-footer-note",
      "input:not([type=checkbox]):not([type=radio]):not([type=hidden]), textarea, select, [role=combobox], .argus-select__trigger",
      ":is(.argus-alert-field-label, .argus-field__label)",
      "input[type=checkbox]",
      ":is(.argus-alert-secondary, .argus-alert-result__meta, .argus-preview-card__hint, .argus-dialog__description)",
      "code",
      ".argus-alert-preview-row",
      ":is(.argus-alert-result__header, .argus-alert-panel-head, .argus-preview-card__header)",
      ":is(.argus-alert-result__note, .argus-alert-callout)",
      ":is(.argus-alert-result__row, .argus-alert-result-row)",
      ":is(.argus-alert-result__name, .argus-alert-result-row button)",
      ":is(.argus-alert-result__badge, .argus-alert-result-row .argus-alert-badge)",
      ":is(.argus-alert-pending .argus-alert-panel-head .argus-alert-badge, .argus-preview-card__header .argus-status-badge)",
    ];
    return Object.fromEntries(
      selectors.map((selector) => [
        selector,
        (selector === ".argus-alert-labels"
          ? [
              ...new Set(
                [...root.querySelectorAll<HTMLElement>(".argus-alert-tag")].map(
                  (node) => node.parentElement!,
                ),
              ),
            ]
          : [
              ...(root.matches(selector) ? [root] : []),
              ...root.querySelectorAll<HTMLElement>(selector),
            ]
        )
          .filter(
            (element) =>
              element.getBoundingClientRect().width > 0 &&
              element.getBoundingClientRect().height > 0 &&
              getComputedStyle(element).clipPath !== "inset(50%)" &&
              getComputedStyle(element).clip !== "rect(0px, 0px, 0px, 0px)" &&
              !element.closest('[aria-hidden="true"]') &&
              (!element.closest("details:not([open])") ||
                !!element.closest("summary")),
          )
          .map((element) => {
            const rect = element.getBoundingClientRect(),
              computed = getComputedStyle(element);
            let background = computed.backgroundColor;
            let ancestor = element.parentElement;
            while (
              (background === "rgba(0, 0, 0, 0)" ||
                background === "transparent") &&
              ancestor
            ) {
              background = getComputedStyle(ancestor).backgroundColor;
              ancestor = ancestor.parentElement;
            }
            // Segmented controls have a group inside their business border.
            // Compare the group spacing, rather than an unused wrapper gap.
            const gapStyle = getComputedStyle(
              element.matches(".argus-alert-subtabs")
                ? (element.querySelector(".argus-segments") ?? element)
                : element,
            );
            const flowChildren = [...element.childNodes].filter((child) =>
              child.nodeType === Node.TEXT_NODE
                ? !!child.textContent?.trim()
                : child instanceof HTMLElement || child instanceof SVGElement
                  ? !["absolute", "fixed"].includes(
                      getComputedStyle(child).position,
                    ) && child.getBoundingClientRect().width > 0
                  : false,
            ).length;
            const before = getComputedStyle(element, "::before");
            const hasFlowBefore =
              before.content !== "none" &&
              before.content !== "normal" &&
              before.display !== "none" &&
              !["absolute", "fixed"].includes(before.position);
            return {
              x: rect.x,
              y: rect.y,
              width: rect.width,
              height: rect.height,
              styles: Object.fromEntries(
                styles.map((name) => [
                  name,
                  name === "backgroundColor"
                    ? background
                    : name === "borderColor"
                      ? ["Top", "Right", "Bottom", "Left"]
                          .filter(
                            (side) =>
                              parseFloat(
                                computed.getPropertyValue(
                                  `border-${side.toLowerCase()}-width`,
                                ),
                              ) > 0,
                          )
                          .map(
                            (side) =>
                              `${side}:${computed.getPropertyValue(`border-${side.toLowerCase()}-color`)}`,
                          )
                          .join(";")
                      : name === "gap" &&
                          (element.tagName === "SELECT" ||
                            element.classList.contains(
                              "argus-select__trigger",
                            ) ||
                            !/^(inline-)?(flex|grid)$/.test(gapStyle.display) ||
                            (!element.matches(".argus-alert-subtabs") &&
                              flowChildren + Number(hasFlowBefore) < 2))
                        ? "0px"
                        : name === "gap"
                          ? gapStyle.gap
                          : computed[name],
                ]),
              ),
            };
          }),
      ]),
    );
  });
}

async function ready(page: Page, root: string) {
  const region = page.locator(root).first();
  await expect(region).toBeVisible();
  await expect(region.locator(".argus-skeleton")).toHaveCount(0);
  await page.evaluate(() => document.fonts.ready);
  let previous = "";
  await expect
    .poll(async () => {
      const box = JSON.stringify(await region.boundingBox());
      const stable = box === previous;
      previous = box;
      return stable;
    })
    .toBe(true);
}

async function measuredGeometry(page: Page, root: string) {
  const locator = page.locator(root).first();
  if (await locator.evaluate((node) => node.tagName === "IFRAME")) {
    const frame = page.frameLocator(root);
    await frame
      .locator("html")
      .evaluate((node) => node.ownerDocument.fonts.ready);
    const measurements = await geometry(frame, ".argus-alert-result");
    const box = (await locator.boundingBox())!;
    for (const nodes of Object.values(measurements))
      for (const node of nodes) {
        node.x += box.x;
        node.y += box.y;
      }
    return measurements;
  }
  return geometry(page, root);
}

async function businessBox(page: Page, rootSelector: string) {
  return page.locator(rootSelector).evaluate((root) => {
    const hero = root.querySelector(".argus-alert-hero")!;
    const start = hero.getBoundingClientRect();
    const bounds = [...root.children]
      .filter((node) => !node.querySelector('[data-action="coverage"]'))
      .map((node) => node.getBoundingClientRect());
    return {
      x: start.x,
      y: start.y,
      width: Math.max(...bounds.map((r) => r.right)) - start.x,
      height: Math.max(...bounds.map((r) => r.bottom)) - start.y,
    };
  });
}

test.describe("Plan6 deterministic Demo comparison", () => {
  test.skip(!enabled, "explicit visual gate");
  // Every variant has its own browser context and output files. A failed
  // capture must not prevent the remaining independent variants from running.
  test.beforeAll(async ({ browser }) => {
    await fs.mkdir(output, { recursive: true });
    const frozen = JSON.parse(
      await fs.readFile(path.join(baseline, "manifest.json"), "utf8"),
    );
    for (const item of frozen) {
      const data = await fs.readFile(path.join(baseline, item.path));
      expect(
        createHash("sha256").update(data).digest("hex").toUpperCase(),
      ).toBe(item.sha256);
    }
    const sources = execFileSync(
      "git",
      ["ls-files", "-z", "--cached", "--others", "--exclude-standard"],
      { cwd: repository },
    )
      .toString()
      .split("\0")
      .filter((name) =>
        /^(web\/|api\/schemas\/alerts\/|scripts\/plan6-visual|pnpm-lock.yaml$|package.json$)/.test(
          name,
        ),
      );
    const hashes = Object.fromEntries(
      await Promise.all(
        sources.map(async (name) => [
          name,
          createHash("sha256")
            .update(await fs.readFile(path.join(repository, name)))
            .digest("hex"),
        ]),
      ),
    );
    await fs.writeFile(
      path.join(output, "run.json"),
      JSON.stringify(
        {
          run_id: runId,
          browser: browser.version(),
          frozen_clock: "2026-10-07T02:32:00Z",
          baseline: frozen,
          source_hashes: hashes,
          fonts: "frozen Argus token font stacks and browser system fonts",
          seed: "alerts-fixture.json",
        },
        null,
        2,
      ),
    );
  });
  test.afterAll(async () => {
    const recorded = JSON.parse(
      await fs.readFile(path.join(output, "run.json"), "utf8"),
    ) as { source_hashes: Record<string, string> };
    const current = execFileSync(
      "git",
      ["ls-files", "-z", "--cached", "--others", "--exclude-standard"],
      { cwd: repository },
    )
      .toString()
      .split("\0")
      .filter((name) =>
        /^(web\/|api\/schemas\/alerts\/|scripts\/plan6-visual|pnpm-lock.yaml$|package.json$)/.test(
          name,
        ),
      );
    expect(current.sort()).toEqual(Object.keys(recorded.source_hashes).sort());
    for (const name of current) {
      const hash = createHash("sha256")
        .update(await fs.readFile(path.join(repository, name)))
        .digest("hex");
      expect(hash, `visual source changed during capture: ${name}`).toBe(
        recorded.source_hashes[name],
      );
    }
    await fs.writeFile(
      path.join(output, "source-stability.json"),
      JSON.stringify(
        { passed: true, files: current.length, run_id: runId },
        null,
        2,
      ),
    );
  });
  for (const locale of ["zh", "en"] as const)
    for (const theme of ["light", "dark"] as const)
      for (const size of sizes) {
        test(`${locale}-${theme}-${size.width} six business tabs`, async ({
          browser,
        }) => {
          test.setTimeout(120_000);
          await fs.mkdir(output, { recursive: true });
          const context = await browser.newContext({
            viewport: size,
            locale: locale === "zh" ? "zh-CN" : "en-US",
            timezoneId: "Asia/Shanghai",
            colorScheme: theme,
          });
          await context.addInitScript(
            ({ locale, theme }) => {
              localStorage.setItem(
                "argus.locale",
                locale === "zh" ? "zh-CN" : "en-US",
              );
              localStorage.setItem("argus.theme", theme);
              localStorage.setItem(
                "argus.plan6.preferences.v1",
                JSON.stringify({ locale, theme }),
              );
            },
            { locale, theme },
          );
          const portal = await context.newPage();
          const demo = await context.newPage();
          await portal.clock.setFixedTime(new Date("2026-10-07T02:32:00Z"));
          await demo.clock.setFixedTime(new Date("2026-10-07T02:32:00Z"));
          await demoRoute(demo);
          await portal.goto("/login");
          const textboxes = portal.getByRole("textbox");
          await textboxes.nth(0).fill("root");
          await portal.locator('input[type="password"]').fill("123456");
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "登录" : "Sign in",
              exact: true,
            })
            .click();
          await expect(portal).not.toHaveURL(/\/login/);
          await portal.goto("/alerts");
          await demo.goto("http://127.0.0.1:4173/docs/plan6/demo.html");
          await expect(portal.getByTestId("alert-center")).toBeVisible();
          await expect(demo.locator(".argus-alert-stats")).toBeVisible();
          await expect(
            portal.locator(".argus-alert-stat-value").nth(0),
          ).toHaveText("5");
          const rows = [];
          for (let i = 0; i < tabs.length; i++) {
            const tab = tabs[i]!;
            await portal.getByRole("tab").nth(i).click();
            await demo.locator(`[data-action="tab"][data-id="${tab}"]`).click();
            await expect(portal.locator(".argus-skeleton")).toHaveCount(0);
            if (["active", "history", "deliveries", "policies"].includes(tab))
              await expect(
                portal.getByRole("tabpanel").locator("tbody tr").first(),
              ).toBeVisible();
            if (tab === "channels")
              await expect(
                portal.locator(".argus-alert-channel-head"),
              ).toHaveCount(4);
            await portal.evaluate(() => document.fonts.ready);
            await demo.evaluate(() => document.fonts.ready);
            await portal.screenshot({
              path: path.join(
                output,
                `${locale}-${theme}-${size.width}-${tab}-portal-full.png`,
              ),
              animations: "disabled",
            });
            await demo.screenshot({
              path: path.join(
                output,
                `${locale}-${theme}-${size.width}-${tab}-demo-full.png`,
              ),
              animations: "disabled",
            });
            const portalBox = await businessBox(
              portal,
              '[data-testid="alert-center"]',
            );
            const demoBox = await businessBox(demo, ".argus-alert-page");
            const portalImageBox = await captureRegion(
              portal,
              '[data-testid="alert-center"]',
              portalBox,
              path.join(
                output,
                `${locale}-${theme}-${size.width}-${tab}-portal-full.png`,
              ),
              repository,
            );
            const demoImageBox = await captureRegion(
              demo,
              ".argus-alert-page",
              demoBox,
              path.join(
                output,
                `${locale}-${theme}-${size.width}-${tab}-demo-full.png`,
              ),
              repository,
            );
            expect(portalBox).not.toBeNull();
            rows.push({
              run_id: runId,
              locale,
              theme,
              viewport: size,
              tab,
              title: titles[locale][i],
              portalBox,
              demoBox,
              portalImageBox,
              demoImageBox,
              portalGeometry: await geometry(
                portal,
                '[data-testid="alert-center"]',
              ),
              demoGeometry: await geometry(demo, ".argus-alert-page"),
            });
            if (tab === "active") {
              const checkboxLayouts: Record<string, unknown> = {};
              for (const [side, page] of [
                ["portal", portal],
                ["demo", demo],
              ] as const) {
                checkboxLayouts[side] = await page
                  .locator(".argus-alert-table input[type=checkbox]")
                  .evaluateAll((nodes) =>
                    nodes.map((node) => {
                      const s = getComputedStyle(node),
                        parent = node.closest("td,th")!,
                        p = getComputedStyle(parent),
                        r = node.getBoundingClientRect(),
                        pr = parent.getBoundingClientRect();
                      return {
                        y: r.y,
                        cellY: pr.y,
                        cellHeight: pr.height,
                        display: s.display,
                        verticalAlign: s.verticalAlign,
                        margin: s.margin,
                        font: s.font,
                        parentFont: p.font,
                        parentVertical: p.verticalAlign,
                        color: s.color,
                      };
                    }),
                  );
              }
              await fs.writeFile(
                path.join(
                  output,
                  `${locale}-${theme}-${size.width}-checkbox-layout.json`,
                ),
                JSON.stringify(checkboxLayouts, null, 2),
              );
            }
          }
          await fs.writeFile(
            path.join(output, `${locale}-${theme}-${size.width}.json`),
            JSON.stringify(rows, null, 2),
          );
          const scenarios: Array<Record<string, unknown>> = [];
          const capture = async (
            name: string,
            portalRoot: string,
            demoRoot: string,
          ) => {
            const key = `${locale}-${theme}-${size.width}-${name}`;
            await expect(portal.locator(portalRoot).first()).toBeVisible();
            await expect(demo.locator(demoRoot)).toBeVisible();
            await ready(portal, portalRoot);
            await ready(demo, demoRoot);
            // Existing conversation shells differ. Compare the business card
            // at the actual portal width; no screenshot pixels are resized.
            let comparisonWidth: number | undefined;
            if (
              name === "conversation-query" ||
              name === "conversation-preview"
            ) {
              comparisonWidth = (await portal
                .locator(portalRoot)
                .first()
                .boundingBox())!.width;
              await demo.locator(demoRoot).evaluate((node, width) => {
                (node as HTMLElement).style.width = `${width}px`;
              }, comparisonWidth);
              await ready(demo, demoRoot);
            }
            await portal.locator(portalRoot).first().scrollIntoViewIfNeeded();
            await demo.locator(demoRoot).scrollIntoViewIfNeeded();
            for (const page of [portal, demo]) {
              await page.evaluate(
                () =>
                  new Promise<void>((done) =>
                    requestAnimationFrame(() =>
                      requestAnimationFrame(() => done()),
                    ),
                  ),
              );
            }
            if (name === "conversation-query") {
              const frameHosts = await portal
                .locator(".argus-tool-presentation")
                .evaluateAll((nodes) =>
                  nodes.map((node) => {
                    const frame = node.querySelector("iframe"),
                      toggle = node.querySelector("[aria-expanded]"),
                      r = frame?.getBoundingClientRect();
                    return {
                      expanded: toggle?.getAttribute("aria-expanded"),
                      text: toggle?.textContent,
                      height: r?.height,
                      frameCount: node.querySelectorAll("iframe").length,
                    };
                  }),
                );
              await fs.writeFile(
                path.join(output, `${key}-capture-hosts.json`),
                JSON.stringify(frameHosts, null, 2),
              );
              await expect(
                portal.locator(".argus-tool-result__header button").last(),
              ).toHaveAttribute("aria-expanded", "true");
              const currentFrame = portal.frameLocator(portalRoot);
              const exposure = await currentFrame
                .locator(".argus-alert-result")
                .evaluate((node) => ({
                  bottom: node.getBoundingClientRect().bottom,
                  viewport: node.ownerDocument.defaultView!.innerHeight,
                }));
              const actualFrame = (await portal
                .locator(portalRoot)
                .boundingBox())!;
              expect(exposure.bottom).toBeLessThanOrEqual(exposure.viewport);
              expect(exposure.bottom).toBeLessThanOrEqual(actualFrame.height);
              await fs.writeFile(
                path.join(output, `${key}-capture-exposure.json`),
                JSON.stringify({ ...exposure, frame: actualFrame }, null, 2),
              );
            }
            await portal.screenshot({
              path: path.join(output, `${key}-portal-full.png`),
              animations: "disabled",
            });
            await demo.screenshot({
              path: path.join(output, `${key}-demo-full.png`),
              animations: "disabled",
            });
            const scenario: Record<string, unknown> = {
              run_id: runId,
              locale,
              theme,
              viewport: size,
              capture_viewport: portal.viewportSize(),
              tab: name,
              comparison_width: comparisonWidth,
              title: name,
              portalBox: await portal
                .locator(portalRoot)
                .evaluateAll((nodes) => {
                  const rectangles = nodes.map((node) =>
                    node.getBoundingClientRect(),
                  );
                  const x = Math.min(...rectangles.map((r) => r.x)),
                    y = Math.min(...rectangles.map((r) => r.y));
                  return {
                    x,
                    y,
                    width: Math.max(...rectangles.map((r) => r.right)) - x,
                    height: Math.max(...rectangles.map((r) => r.bottom)) - y,
                  };
                }),
              demoBox: await demo.locator(demoRoot).boundingBox(),
              portalGeometry: await measuredGeometry(portal, portalRoot),
              demoGeometry: await geometry(demo, demoRoot),
            };
            if (
              await portal
                .locator(portalRoot)
                .first()
                .evaluate((node) => node.tagName === "IFRAME")
            ) {
              const frameBox = scenario.portalBox as {
                x: number;
                y: number;
                width: number;
                height: number;
              };
              const contentBox = await portal
                .frameLocator(portalRoot)
                .locator(".argus-alert-result")
                .evaluate((node) => {
                  const r = node.getBoundingClientRect();
                  return { x: r.x, y: r.y, width: r.width, height: r.height };
                });
              scenario.portalBox = {
                ...contentBox,
                x: frameBox.x + contentBox.x,
                y: frameBox.y + contentBox.y,
              };
            }
            if (name === "organization-permissions") {
              scenario.portalImageBox = await captureRegion(
                portal,
                portalRoot,
                scenario.portalBox as {
                  x: number;
                  y: number;
                  width: number;
                  height: number;
                },
                path.join(output, `${key}-portal-full.png`),
                repository,
              );
              scenario.demoImageBox = await captureRegion(
                demo,
                demoRoot,
                scenario.demoBox as {
                  x: number;
                  y: number;
                  width: number;
                  height: number;
                },
                path.join(output, `${key}-demo-full.png`),
                repository,
              );
            }
            if (
              name === "conversation-preview" ||
              name === "detail" ||
              name === "editor" ||
              name === "create"
            ) {
              const extra: Record<string, unknown> = {
                name:
                  name === "detail"
                    ? "detail-body"
                    : name === "editor" || name === "create"
                      ? "editor-fields"
                      : "preview-fields",
              };
              for (const [side, page, root] of [
                ["portal", portal, portalRoot],
                ["demo", demo, demoRoot],
              ] as const) {
                const selector =
                  name === "detail"
                    ? `${root} ${side === "portal" ? ".argus-drawer__body" : ".argus-alert-drawer-body"} > *`
                    : name === "editor" || name === "create"
                      ? `${root} ${side === "portal" ? ".argus-dialog__body" : ".argus-alert-modal-body"} > *`
                      : `${root} .argus-alert-preview-row`;
                if (name === "conversation-preview") {
                  await expect(page.locator(selector)).toHaveCount(8);
                } else {
                  expect(await page.locator(selector).count()).toBeGreaterThan(
                    0,
                  );
                }
                const box = await page
                  .locator(selector)
                  .evaluateAll((nodes) => {
                    const bounds = nodes
                      .map((node) => node.getBoundingClientRect())
                      .filter((rect) => rect.width > 0 && rect.height > 0);
                    if (!bounds.length)
                      throw new Error(
                        "Full content has no rendered business nodes",
                      );
                    const x = Math.min(...bounds.map((r) => r.x)),
                      y = Math.min(...bounds.map((r) => r.y));
                    return {
                      x,
                      y,
                      width: Math.max(...bounds.map((r) => r.right)) - x,
                      height: Math.max(...bounds.map((r) => r.bottom)) - y,
                    };
                  });
                const file = `${key}-${side}-${extra.name}.png`;
                extra[`${side}File`] = file;
                extra[`${side}ImageBox`] = await captureRegion(
                  page,
                  selector,
                  box,
                  path.join(output, file),
                  repository,
                );
              }
              scenario.extra_views = [extra];
            }
            if (name === "detail" || name === "editor" || name === "create") {
              const layouts: Record<string, unknown> = {};
              for (const [side, page, root] of [
                ["portal", portal, portalRoot],
                ["demo", demo, demoRoot],
              ] as const) {
                layouts[side] = await page.locator(root).evaluate((node) =>
                  [
                    ...node.querySelectorAll(
                      ".argus-alert-drawer-section, .argus-alert-form-divider, .argus-alert-timeline, .argus-alert-detail-tabs, .argus-tabs__content, .argus-alert-grid, .argus-alert-rule, .argus-alert-rule-head, .argus-field, .argus-alert-field",
                    ),
                  ].map((item) => {
                    const r = item.getBoundingClientRect(),
                      s = getComputedStyle(item);
                    return {
                      class: item.className,
                      y: r.y,
                      height: r.height,
                      display: s.display,
                      marginTop: s.marginTop,
                      marginBottom: s.marginBottom,
                      paddingTop: s.paddingTop,
                      paddingBottom: s.paddingBottom,
                    };
                  }),
                );
              }
              await fs.writeFile(
                path.join(output, `${key}-layout.json`),
                JSON.stringify(layouts, null, 2),
              );
            }
            scenarios.push(scenario);
          };
          await portal
            .getByRole("tab", { name: titles[locale][3], exact: true })
            .click();
          await demo.locator('[data-action="tab"][data-id="policies"]').click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "编辑" : "Edit",
              exact: true,
            })
            .first()
            .click();
          await demo
            .locator('[data-action="policy-edit"][data-id="p1"]')
            .click();
          await capture("editor", "[role=dialog]", "#modal[open]");
          await portal
            .getByRole("dialog")
            .getByRole("button", {
              name: locale === "zh" ? "预览变更" : "Preview changes",
              exact: true,
            })
            .click();
          await demo.locator('#modal form button[type="submit"]').click();
          await expect(
            portal.getByRole("dialog", {
              name: locale === "zh" ? "预览变更" : "Preview changes",
              exact: true,
            }),
          ).toBeVisible();
          await expect(portal.getByRole("dialog")).toHaveCount(1);
          await capture("preview", "[role=dialog]", "#modal[open]");
          await portal
            .getByRole("dialog")
            .getByRole("button", {
              name: locale === "zh" ? "取消" : "Cancel",
              exact: true,
            })
            .click();
          await demo
            .locator('#modal footer [data-action="modal-close"]')
            .click();
          await expect(
            portal.getByRole("dialog", {
              name: locale === "zh" ? "编辑" : "Edit",
              exact: true,
            }),
          ).toBeVisible();
          await portal
            .getByRole("dialog", {
              name: locale === "zh" ? "编辑" : "Edit",
              exact: true,
            })
            .getByRole("button", {
              name: locale === "zh" ? "关闭" : "Close",
              exact: true,
            })
            .click();
          await expect(portal.getByRole("dialog")).toHaveCount(0);
          // Existing-policy screenshots do not prove a fresh draft's defaults.
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "创建告警策略" : "Create policy",
              exact: true,
            })
            .first()
            .click();
          await demo.locator('[data-action="policy-create"]').first().click();
          const create = portal.getByRole("dialog", {
            name: locale === "zh" ? "创建告警策略" : "Create policy",
            exact: true,
          });
          await expect(
            create.getByRole("checkbox", {
              name: locale === "zh" ? "生产事件群" : "Production incidents",
              exact: true,
            }),
          ).toBeChecked();
          await expect(
            demo.locator('#modal input[type="checkbox"][value="c3"]'),
          ).toBeChecked();
          await capture("create", "[role=dialog]", "#modal[open]");
          const draftName =
            locale === "zh" ? "默认目标策略" : "Default targets policy";
          await create
            .getByRole("textbox", {
              name: locale === "zh" ? "名称" : "Name",
              exact: true,
            })
            .fill(draftName);
          await demo
            .locator('#modal input[name="policy-name"]')
            .fill(draftName);
          await create
            .getByRole("button", {
              name: locale === "zh" ? "预览变更" : "Preview changes",
              exact: true,
            })
            .click();
          await demo.locator('#modal form button[type="submit"]').click();
          await expect(
            portal
              .getByRole("dialog", {
                name: locale === "zh" ? "预览变更" : "Preview changes",
                exact: true,
              })
              .getByText(
                locale === "zh" ? "生产事件群" : "Production incidents",
                { exact: true },
              ),
          ).toBeVisible();
          await expect(portal.getByRole("dialog")).toHaveCount(1);
          await capture("create-preview", "[role=dialog]", "#modal[open]");
          await portal
            .getByRole("dialog")
            .getByRole("button", {
              name: locale === "zh" ? "取消" : "Cancel",
              exact: true,
            })
            .click();
          await expect(
            create.getByRole("checkbox", {
              name: locale === "zh" ? "生产事件群" : "Production incidents",
              exact: true,
            }),
          ).toBeChecked();
          await create
            .getByRole("button", {
              name: locale === "zh" ? "关闭" : "Close",
              exact: true,
            })
            .click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "放弃更改" : "Discard changes",
              exact: true,
            })
            .click();
          await demo
            .locator('#modal footer [data-action="modal-close"]')
            .click();
          await expect(portal.getByRole("dialog")).toHaveCount(0);
          await portal
            .getByRole("tab", { name: titles[locale][0], exact: true })
            .click();
          await demo.locator('[data-action="tab"][data-id="active"]').click();
          await portal
            .locator("tbody")
            .getByRole("button", {
              name: locale === "zh" ? "详情" : "Details",
              exact: true,
            })
            .first()
            .click();
          await demo
            .locator('[data-action="event-detail"][data-id="e1"]')
            .first()
            .click();
          await expect(
            portal.getByRole("dialog").locator(".argus-skeleton"),
          ).toHaveCount(0);
          await capture("detail", "[role=dialog]", "#drawer[open]");
          const closeForms = async () => {
            await portal
              .getByRole("dialog")
              .getByRole("button", {
                name: locale === "zh" ? "关闭" : "Close",
                exact: true,
              })
              .click();
            await demo
              .locator('#modal [data-action="modal-close"]')
              .first()
              .click();
            await expect(portal.getByRole("dialog")).toHaveCount(0);
          };
          await portal
            .getByRole("dialog")
            .getByRole("button", {
              name: locale === "zh" ? "关闭" : "Close",
              exact: true,
            })
            .click();
          await demo.locator('#drawer [data-action="drawer-close"]').click();
          await portal
            .getByRole("tab", { name: titles[locale][4], exact: true })
            .click();
          await demo.locator('[data-action="tab"][data-id="noise"]').click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "创建屏蔽" : "Create silence",
              exact: true,
            })
            .click();
          await demo.locator('[data-action="silence-create"]').click();
          await capture("silence-editor", "[role=dialog]", "#modal[open]");
          await closeForms();
          await portal
            .getByRole("radiogroup")
            .getByText(locale === "zh" ? "抑制规则" : "Inhibition rules", {
              exact: true,
            })
            .click();
          await demo
            .locator('[data-action="subtab"][data-extra="inhibitions"]')
            .click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "创建抑制规则" : "Create inhibition",
              exact: true,
            })
            .click();
          await demo.locator('[data-action="inhibition-create"]').click();
          await capture("inhibition-editor", "[role=dialog]", "#modal[open]");
          await closeForms();
          await portal
            .getByRole("tab", { name: titles[locale][5], exact: true })
            .click();
          await demo.locator('[data-action="tab"][data-id="channels"]').click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "配置" : "Configure",
              exact: true,
            })
            .first()
            .click();
          await demo.locator('[data-action="channel-edit"]').first().click();
          await capture("channel-editor", "[role=dialog]", "#modal[open]");
          await closeForms();
          await portal
            .getByRole("radiogroup")
            .getByText(locale === "zh" ? "路由与合并" : "Routing & grouping", {
              exact: true,
            })
            .click();
          await demo
            .locator('[data-action="subtab"][data-extra="routes"]')
            .click();
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "编辑" : "Edit",
              exact: true,
            })
            .first()
            .click();
          await demo.locator('[data-action="route-edit"]').first().click();
          await capture("route-editor", "[role=dialog]", "#modal[open]");
          await closeForms();
          await portal.goto("/settings/org");
          await portal
            .getByRole("tab", {
              name: locale === "zh" ? "角色" : "Roles",
              exact: true,
            })
            .click();
          await expect(
            portal.locator(".argus-alert-permissions tbody tr"),
          ).toHaveCount(7);
          await demo
            .locator('[data-action="page-entry"][data-id="organization"]')
            .click();
          await capture(
            "organization-permissions",
            ".argus-alert-permissions .argus-table",
            ".argus-alert-permission-table",
          );
          await portal
            .getByRole("tab", {
              name: locale === "zh" ? "告警审计" : "Alert audit",
              exact: true,
            })
            .click();
          await demo
            .locator('[data-action="subtab"][data-extra="audit"]')
            .click();
          await expect(
            portal.getByRole("tabpanel").locator("tbody tr"),
          ).toHaveCount(3);
          await capture(
            "organization-audit",
            '[role="tabpanel"] .argus-table-wrap',
            ".argus-alert-page .argus-alert-table-wrap",
          );
          // Compare complete inline cards at their original pixel width. A
          // taller desktop capture surface avoids clipping by either shell's
          // chat scroller; the 72 base cases keep the three original viewports.
          await portal.setViewportSize({ width: size.width, height: 1600 });
          await demo.setViewportSize({ width: size.width, height: 1600 });
          await portal.goto("/");
          await demo
            .locator('[data-action="page-entry"][data-id="workspace"]')
            .click();
          const queryPrompt =
            locale === "zh"
              ? "查询当前告警，并说明评估健康、无数据和范围。"
              : "Query current alerts and explain evaluation health, no data and scope.";
          await portal.locator("textarea").last().fill(queryPrompt);
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "发送" : "Send",
              exact: true,
            })
            .click();
          await demo.locator("#ai-input").fill(queryPrompt);
          await demo.locator('#ai-form button[type="submit"]').click();
          await expect(
            portal.getByTestId("chat-message-assistant").last(),
          ).toContainText(locale === "zh" ? "健康" : "health", {
            timeout: 30_000,
          });
          // Tool presentations can appear in the temporary streaming message.
          // Its persisted replacement is a separate component; freeze the
          // completed message before expanding or measuring the result.
          await expect(
            portal.locator(".argus-chat-message__caret"),
          ).toHaveCount(0);
          await expect(
            portal.getByRole("button", {
              name: locale === "zh" ? "停止生成" : "Stop generating",
              exact: true,
            }),
          ).toHaveCount(0);
          await expect(portal.locator("textarea").last()).toBeEnabled();
          await openToolResults(portal);
          await expect(
            portal
              .frameLocator(".argus-tool-presentation__frame")
              .last()
              .getByRole("heading", {
                name: locale === "zh" ? "只读查询结果" : "Read-only result",
              }),
          ).toBeVisible();
          await expect(
            portal.locator(".argus-tool-result__header button").last(),
          ).toHaveAttribute("aria-expanded", "true");
          let frameMeasurement: Record<string, unknown> = {};
          try {
            await expect
              .poll(async () => {
                try {
                  frameMeasurement = await portal
                    .frameLocator(".argus-tool-presentation__frame")
                    .last()
                    .locator("html")
                    .evaluate((node) => {
                      const body = node.ownerDocument.body;
                      const root = node.querySelector<HTMLElement>(
                        "#argus-template-root",
                      );
                      if (!root)
                        return {
                          viewport: 0,
                          documentHeight: 1,
                          loading: true,
                        };
                      return {
                        viewport: node.ownerDocument.defaultView!.innerHeight,
                        documentHeight: node.scrollHeight,
                        bodyHeight: body.scrollHeight,
                        bodyStyle: {
                          padding: getComputedStyle(body).padding,
                          margin: getComputedStyle(body).margin,
                        },
                        rootHeight: root.getBoundingClientRect().height,
                        rootScrollHeight: root.scrollHeight,
                        rootBottom: root.getBoundingClientRect().bottom,
                      };
                    });
                } catch (error) {
                  if (
                    error instanceof Error &&
                    /Frame was detached|Execution context was destroyed/.test(
                      error.message,
                    )
                  )
                    return false;
                  throw error;
                }
                return (
                  Number(frameMeasurement.documentHeight) <=
                  Number(frameMeasurement.viewport)
                );
              })
              .toBe(true);
          } finally {
            await fs.writeFile(
              path.join(
                output,
                `${locale}-${theme}-${size.width}-frame-size.json`,
              ),
              JSON.stringify(frameMeasurement, null, 2),
            );
          }
          await capture(
            "conversation-query",
            ".argus-tool-presentation__frame",
            ".argus-alert-workbench-result",
          );
          const createPrompt =
            locale === "zh"
              ? "创建 CPU 和内存同时超阈值的告警策略，先预览，等我确认。"
              : "Create an alert policy where both CPU and memory exceed their thresholds; preview and wait for confirmation.";
          await portal.locator("textarea").last().fill(createPrompt);
          await portal
            .getByRole("button", {
              name: locale === "zh" ? "发送" : "Send",
              exact: true,
            })
            .click();
          await demo.locator("#ai-input").fill(createPrompt);
          await demo.locator('#ai-form button[type="submit"]').click();
          await expect(portal.getByTestId("pending-action-card")).toBeVisible();
          await expect(
            portal
              .getByTestId("pending-action-card")
              .getByText(
                locale === "zh" ? "生产事件群" : "Production incidents",
                { exact: true },
              ),
          ).toBeVisible();
          await expect(
            portal.locator(".argus-chat-message__caret"),
          ).toHaveCount(0);
          await capture(
            "conversation-preview",
            '[data-testid="pending-action-card"]',
            ".argus-alert-pending",
          );
          await fs.writeFile(
            path.join(
              output,
              `${locale}-${theme}-${size.width}-scenarios.json`,
            ),
            JSON.stringify(scenarios, null, 2),
          );
          await context.close();
        });
      }
});
