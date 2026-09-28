import { readFileSync } from "node:fs";
import { expect, it, vi } from "vitest";
import { createConfiguredApiClient } from "../../factory";
import { HttpTransport } from "../../transport/http";
import { createPlanV5Domains } from "./planv5";

type SpecNode = {
  $ref?: string;
  parameters?: SpecNode[];
  in?: string;
  name?: string;
  required?: boolean;
  [key: string]: unknown;
};

it("PlanV5 adapter URLs, methods and required headers match the authoritative OpenAPI", async () => {
  const contract = JSON.parse(
    readFileSync(
      new URL(
        "../../../../../../api/openapi/generated/argus.bundle.json",
        import.meta.url,
      ),
      "utf8",
    ),
  ) as { paths: Record<string, SpecNode>; [key: string]: unknown };
  const resolve = (node: SpecNode): SpecNode => {
    if (!node.$ref) return node;
    if (!node.$ref.startsWith("#/"))
      throw new Error("Unbundled parameter reference");
    let value: unknown = contract;
    for (const key of node.$ref.slice(2).split("/"))
      value = (value as Record<string, unknown>)[
        key.replace(/~1/g, "/").replace(/~0/g, "~")
      ];
    return resolve(value as SpecNode);
  };
  const check = (url: URL, method: string) => {
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const match = Object.entries(contract.paths).find(([pattern]) =>
      new RegExp(`^${pattern.replace(/\{[^}]+\}/g, "[^/]+")}$`).test(path),
    );
    expect(
      match,
      `${method} ${path} is absent from the contract`,
    ).toBeDefined();
    expect(
      match![1][method.toLowerCase()],
      `${method} ${path} is not declared`,
    ).toBeDefined();
    return [
      ...(match![1].parameters ?? []),
      ...((match![1][method.toLowerCase()] as SpecNode).parameters ?? []),
    ].map(resolve);
  };
  const id = "11111111-1111-4111-8111-111111111111";
  const fetch = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const params = check(new URL(String(input)), init?.method ?? "GET");
      const headers = new Headers(init?.headers);
      for (const parameter of params) {
        if (parameter.in === "header" && parameter.required)
          expect(
            headers.has(parameter.name!),
            `Missing required header ${parameter.name}`,
          ).toBe(true);
      }
      return new Response(JSON.stringify({ id, items: [] }), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  const http = new HttpTransport({
    base_url: "https://api.example.test",
    fetch,
    csrf_token: () => "csrf-test",
  });
  vi.spyOn(http, "upload").mockImplementation(async (path) => {
    check(http.resolve(path), "PUT");
    return { id } as never;
  });
  const api = createPlanV5Domains(http, () => "request-test");
  const input = {
    name: "MCP",
    endpoint: "https://mcp.example.test",
    auth_type: "none" as const,
    member_ids: [id],
    expected_version: 1,
  };
  await api.mcp.list();
  await api.mcp.create(input);
  await api.mcp.update(id, input);
  await api.mcp.test(id);
  await api.mcp.setState(id, "disabled", 1);
  await api.mcp.setMembers(id, [], 1);
  await api.workspace.get(id);
  await api.workspace.listFiles(id);
  await api.workspace.upload(
    id,
    { name: "data.csv", size: 2 } as File,
    () => {},
  );
  check(new URL(api.workspace.downloadUrl(id, id, "file")), "GET");
  check(new URL(api.workspace.downloadUrl(id, id, "delivery")), "GET");
  await api.workspace.remove(id);
  await api.presentations.get(id, id);
  await api.presentations.result(`result_${id}`);
  const complete = await createConfiguredApiClient({
    portal: "enterprise",
    mode: "real",
    base_url: "https://api.example.test",
    fetch,
    csrf_token: () => "csrf-test",
  });
  await complete.conversations.dashboardContext(id);
  await complete.conversations.preflight(id, {
    content: "Check capacity",
    dashboard_context: {
      mode: "analyze",
      dashboard_ids: [id],
      expected_version: 0,
    },
    file_ids: [],
  });
});
