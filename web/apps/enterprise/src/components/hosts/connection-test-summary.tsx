import type { ConnectionTest } from "@argus/api-client";
import { Alert } from "@argus/ui";
import { useTranslation } from "react-i18next";
import { useEffect, useRef } from "react";
import { connectionTestFailureMessage } from "./onboarding-errors";

export function ConnectionTestSummary({ result }: { result: ConnectionTest }) {
  const { t } = useTranslation();
  const succeeded = result.status === "succeeded";
  const container = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (result.status === "failed")
      container.current?.scrollIntoView({ block: "nearest" });
  }, [result]);
  return (
    <div className="argus-detail-section" ref={container}>
      <Alert
        title={t("hosts.wizard.connectionTestSummary")}
        tone={succeeded ? "success" : "danger"}
        description={
          succeeded
            ? t("hosts.wizard.testPassed")
            : connectionTestFailureMessage(result.error_code, t)
        }
      />
      {result.checks?.length > 0 && (
        <ul>
          {result.checks.map((check, index) => {
            const name = typeof check.name === "string" ? check.name : "check";
            const status = check.status;
            return (
              <li key={`${name}-${index}`}>
                {t(`connectionChecks.names.${name}`, { defaultValue: name })}
                {": "}
                {t(`connectionChecks.${status}`)}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
