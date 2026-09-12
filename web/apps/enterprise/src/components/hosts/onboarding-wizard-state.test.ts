import { describe, expect, it } from "vitest";

import {
  onboardingWizardReducer,
  type OnboardingWizardState,
} from "./onboarding-wizard-state";

describe("onboardingWizardReducer", () => {
  it("follows command and operation result routes", () => {
    let command: OnboardingWizardState<"command"> = {
      phase: "select_mode",
      mode: "command",
    };
    command = onboardingWizardReducer(command, {
      type: "next",
      terminal: "confirm_command",
    });
    command = onboardingWizardReducer(command, {
      type: "next",
      terminal: "confirm_command",
    });
    command = onboardingWizardReducer(command, { type: "commit_command" });
    expect(command.phase).toBe("command_result");

    let direct: OnboardingWizardState<"direct"> = {
      phase: "select_mode",
      mode: "direct",
    };
    direct = onboardingWizardReducer(direct, {
      type: "next",
      terminal: "verify",
    });
    direct = onboardingWizardReducer(direct, {
      type: "next",
      terminal: "verify",
    });
    direct = onboardingWizardReducer(direct, { type: "commit_operation" });
    direct = onboardingWizardReducer(direct, { type: "operation_complete" });
    expect(direct.phase).toBe("completed");
  });

  it("ignores illegal transitions", () => {
    const state: OnboardingWizardState<"command"> = {
      phase: "select_mode",
      mode: "command",
    };
    expect(onboardingWizardReducer(state, { type: "commit_command" })).toBe(
      state,
    );
    expect(onboardingWizardReducer(state, { type: "operation_complete" })).toBe(
      state,
    );
  });
});
