import type { Page } from "@playwright/test";

/** Use the browser's host mapping, TLS session and cookies for business facts. */
export async function browserApi(
  page: Page,
  path: string,
  options: {
    method?: "GET" | "POST";
    data?: unknown;
    headers?: Record<string, string>;
  } = {},
) {
  if (!path.startsWith("/api/v1/"))
    throw new Error("Business assertions require a same-origin API path");
  const result = await page.evaluate(
    async ({ path, options }) => {
      const method = options.method ?? "GET";
      const headers = new Headers(options.headers);
      if (method !== "GET") {
        headers.set("Content-Type", "application/json");
        const csrf = document.cookie
          .split(";")
          .map((part) => part.trim())
          .find((part) => part.startsWith("argus_enterprise_csrf="));
        if (!csrf)
          throw new Error("Browser assertion has no enterprise CSRF token");
        headers.set(
          "X-CSRF-Token",
          decodeURIComponent(csrf.slice(csrf.indexOf("=") + 1)),
        );
      }
      const response = await fetch(path, {
        method,
        headers,
        credentials: "same-origin",
        body: method === "GET" ? undefined : JSON.stringify(options.data),
      });
      const body = await response.text();
      return { status: response.status, ok: response.ok, body };
    },
    { path, options },
  );
  return {
    ok: () => result.ok,
    status: () => result.status,
    json: async () => JSON.parse(result.body),
  };
}
