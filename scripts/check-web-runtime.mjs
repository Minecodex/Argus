import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { chromium } from "@playwright/test";

// Production bundle smoke only. API responses intentionally represent a signed-out
// browser; this does not replace authenticated backend or telemetry E2E coverage.
const browser = await chromium.launch({ headless: true });
try {
  for (const app of ["enterprise", "platform"]) {
    const root = path.resolve("web/apps", app, "dist");
    const manifest = JSON.parse(
      await readFile(path.join(root, ".vite/manifest.json"), "utf8"),
    );
    const server = createServer(async (req, res) => {
      const pathname = new URL(req.url, "http://localhost").pathname;
      if (pathname === "/argus-runtime.json") {
        res.setHeader("Content-Type", "application/json");
        res.end(
          JSON.stringify({
            templateOrigin: "http://template.example.test",
            platformLoginUrl: "/login",
          }),
        );
        return;
      }
      if (pathname.startsWith("/api/")) {
        if (pathname === "/api/v1/setup/status") {
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(
            JSON.stringify({ state: "initialized", platform_name: "Argus" }),
          );
          return;
        }
        res.writeHead(401, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            code: "AUTHENTICATION_REQUIRED",
            message_key: "errors.unauthorized",
            request_id: "bundle-smoke",
            retryable: false,
          }),
        );
        return;
      }
      const relative = pathname.startsWith("/assets/")
        ? pathname.slice(1)
        : "index.html";
      const target = path.resolve(root, relative);
      if (!target.startsWith(root + path.sep)) {
        res.writeHead(403).end();
        return;
      }
      try {
        const data = await readFile(target);
        res.writeHead(200, {
          "Content-Type": target.endsWith(".js")
            ? "text/javascript"
            : target.endsWith(".css")
              ? "text/css"
              : "text/html",
        });
        res.end(data);
      } catch {
        res.writeHead(404).end();
      }
    });
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const context = await browser.newContext();
    try {
      await context.addInitScript(() =>
        localStorage.setItem("argus.locale", "zh-CN"),
      );
      const page = await context.newPage(),
        errors = [];
      page.on("pageerror", (error) => errors.push(error.message));
      await page.goto(`http://127.0.0.1:${server.address().port}/login`);
      await page
        .getByRole("textbox", { name: "用户名", exact: true })
        .waitFor();
      const chunks = [
        ...new Set(
          Object.values(manifest)
            .filter((entry) => entry.file?.endsWith(".js"))
            .map((entry) => "/" + entry.file),
        ),
      ];
      await page.evaluate(async (files) => {
        for (const file of files) await import(file);
      }, chunks);
      assert.deepEqual(errors, [], `${app} production runtime errors`);
      console.log(
        `${app}: signed-out page and ${chunks.length} production chunks loaded`,
      );
    } finally {
      await context.close();
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  }
} finally {
  await browser.close();
}
