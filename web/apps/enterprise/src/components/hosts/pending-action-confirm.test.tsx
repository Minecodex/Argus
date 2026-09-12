// @vitest-environment jsdom
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "i18next";
import { ApiError, type PendingActionPublic } from "@argus/api-client";
import { LocaleProvider } from "@argus/ui";

import "../../i18n";
import { PendingActionConfirm } from "./pending-action-confirm";

const api = vi.hoisted(() => ({
  approvals: { confirm: vi.fn(), cancel: vi.fn() },
  auth: { stepUp: vi.fn() },
}));
vi.mock("@argus/api-client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@argus/api-client")>()),
  useApi: () => api,
}));

const action: PendingActionPublic = {
  schema_version: "argus.pending_action/v1",
  action_ref: "remove-123",
  action_type: "host.removal.forget",
  title: "Remove from Argus only",
  summary: "Revoke server identities without local cleanup",
  risk: "critical",
  preview: { name: "123", mode: "forget" },
  diff: [],
  status: "awaiting_confirmation",
  available_actions: ["confirm", "cancel"],
  expires_at: new Date(Date.now() + 600_000).toISOString(),
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};
function error(code: string) {
  return new ApiError(
    {
      code,
      message_key: `errors.codes.${code}`,
      request_id: "request-123",
      retryable: false,
    },
    403,
  );
}
beforeEach(async () => {
  vi.resetAllMocks();
  window.localStorage.setItem("argus.locale", "zh-CN");
  await i18n.changeLanguage("zh-CN");
  api.approvals.confirm.mockRejectedValueOnce(error("STEP_UP_REQUIRED"));
  api.approvals.confirm.mockResolvedValue({
    pending_action: { ...action, status: "awaiting_approval" },
  });
  api.auth.stepUp.mockResolvedValue({});
});
afterEach(cleanup);

async function requestConfirmation() {
  const onDone = vi.fn();
  render(
    <LocaleProvider>
      <PendingActionConfirm action={action} onDone={onDone} />
    </LocaleProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: "确认执行" }));
  const dialog = await screen.findByRole("dialog", {
    name: "验证身份后继续操作",
  });
  return { dialog, onDone };
}

describe("PendingAction critical confirmation", () => {
  it("verifies identity then confirms the same action and preserves approval", async () => {
    const { dialog, onDone } = await requestConfirmation();
    expect(api.approvals.confirm).toHaveBeenCalledTimes(1);
    fireEvent.change(within(dialog).getByLabelText(/验证码或恢复码/), {
      target: { value: "123456" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "验证并继续" }));
    await waitFor(() => expect(api.approvals.confirm).toHaveBeenCalledTimes(2));
    expect(api.auth.stepUp).toHaveBeenCalledWith({ code: "123456" });
    expect(api.approvals.confirm.mock.calls).toEqual([
      [action.action_ref],
      [action.action_ref],
    ]);
    expect(api.auth.stepUp.mock.invocationCallOrder[0]).toBeLessThan(
      api.approvals.confirm.mock.invocationCallOrder[1]!,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await screen.findAllByText(/等待审批/);
    expect(onDone).not.toHaveBeenCalled();
  });

  it("does not continue when verification is cancelled", async () => {
    const { dialog, onDone } = await requestConfirmation();
    fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(api.auth.stepUp).not.toHaveBeenCalled();
    expect(api.approvals.confirm).toHaveBeenCalledTimes(1);
    expect(onDone).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "确认执行" })).toBeEnabled();
  });

  it("keeps verification open and does not confirm with an invalid MFA proof", async () => {
    api.auth.stepUp.mockRejectedValue(error("MFA_PROOF_INVALID"));
    const { dialog, onDone } = await requestConfirmation();
    fireEvent.change(within(dialog).getByLabelText(/验证码或恢复码/), {
      target: { value: "000000" },
    });
    fireEvent.click(within(dialog).getByRole("button", { name: "验证并继续" }));
    await within(dialog).findByText("无法完成安全验证");
    expect(api.approvals.confirm).toHaveBeenCalledTimes(1);
    expect(onDone).not.toHaveBeenCalled();
  });
});
