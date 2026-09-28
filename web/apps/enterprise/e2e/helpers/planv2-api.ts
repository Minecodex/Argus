import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";

// Uses the same session, CSRF and host confirmation endpoints as the product.
export async function dashboardAPI(
  page: Page,
  path: string,
  method = "GET",
  data?: unknown,
  expected = 200,
  key = randomUUID(),
) {
  // Fetch from Chromium so the isolated ingress host mapping and browser
  // cookies are identical to the UI. Node's APIRequestContext ignores those
  // host-resolver rules.
  const result = await page.evaluate(
    async ({ path, method, data, key }) => {
      const session = await fetch("/api/v1/enterprise/auth/session");
      if (!session.ok)
        return { sessionStatus: session.status, status: 0, body: null };
      const state = await session.json();
      const response = await fetch(`/api/v1${path}`, {
        method,
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": state.csrf_token,
          "Idempotency-Key": key,
        },
        body: data === undefined ? undefined : JSON.stringify(data),
      });
      return {
        sessionStatus: session.status,
        status: response.status,
        body: response.status === 204 ? null : await response.json(),
      };
    },
    { path, method, data, key },
  );
  expect(result.sessionStatus).toBe(200);
  expect(result.status, `${method} ${path}: ${result.body?.code ?? ""}`).toBe(
    expected,
  );
  return {
    status: () => result.status,
    json: async () => result.body,
  };
}

export async function objectGrant(
  admin: Page,
  subject: string,
  type: string,
  ids: string[],
  remove: boolean,
) {
  const path = `/enterprise/data-authorizations/user/${subject}`;
  const current = await (
    await dashboardAPI(admin, `${path}?resource_type=${type}`)
  ).json();
  await dashboardAPI(
    admin,
    path,
    "POST",
    {
      resource_type: type,
      resource_ids: ids,
      remove,
      expected_version: current.authorization_version,
    },
    204,
  );
}

export async function confirmDashboardAction(
  page: Page,
  ref: string,
  key = randomUUID(),
) {
  const response = await (
    await dashboardAPI(
      page,
      `/enterprise/pending-actions/${ref}/confirm`,
      "POST",
      undefined,
      200,
      key,
    )
  ).json();
  if (response.execution) {
    await expect
      .poll(
        async () =>
          (
            await (
              await dashboardAPI(
                page,
                `/enterprise/executions/${response.execution.execution_id}`,
              )
            ).json()
          ).status,
        { timeout: 65000 },
      )
      .toBe("succeeded");
  } else expect(response.pending_action.status).toBe("succeeded");
  return response;
}
