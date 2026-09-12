import { formatErrorCode } from "@argus/api-client";
import type { TFunction } from "i18next";

const callbackFailureKeys: Record<string, string> = {
  HOST_ONBOARDING_CALLBACK_CONFIG_INVALID: "configInvalid",
  HOST_ONBOARDING_CALLBACK_DNS_FAILED: "dnsFailed",
  HOST_ONBOARDING_CALLBACK_CONNECT_FAILED: "connectFailed",
  HOST_ONBOARDING_CALLBACK_TLS_FAILED: "tlsFailed",
  HOST_ONBOARDING_CALLBACK_HTTP_FAILED: "httpFailed",
  HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID: "responseInvalid",
  HOST_ONBOARDING_CALLBACK_TIMEOUT: "timeout",
};

export function onboardingFailureMessageKey(code?: string) {
  if (code && callbackFailureKeys[code]) {
    return `hosts.onboardingProgress.callbackFailure.${callbackFailureKeys[code]}`;
  }
  return code?.includes("ARTIFACT_")
    ? "hosts.onboardingProgress.artifactFailure"
    : "hosts.onboardingProgress.failure";
}

export function connectionTestFailureMessage(
  code: string | undefined,
  t: TFunction,
) {
  const message = formatErrorCode(
    code,
    code && callbackFailureKeys[code]
      ? t(onboardingFailureMessageKey(code))
      : t("hosts.wizard.testFailed"),
  );
  return code
    ? `${message} ${t("hosts.onboardingProgress.errorCode", { code })}`
    : message;
}
