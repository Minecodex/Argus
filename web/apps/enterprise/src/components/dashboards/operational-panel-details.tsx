import { useTranslation } from "react-i18next";
import { formatErrorCode, type DashboardSchemas } from "@argus/api-client";
import { InfoPopover } from "@argus/ui";
const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
export function OperationalPanelDetails({
  title,
  execution,
}: {
  title: string;
  execution?: DashboardSchemas["DashboardPanelExecution"];
}) {
  const { t } = useTranslation(),
    target = execution?.targets[0],
    data = record(target?.data),
    errors = record(data.errors),
    entries = Object.entries(errors);
  if (!target || (target.status === "success" && data.complete !== false))
    return null;
  const forbidden = entries.some(
    ([, code]) =>
      String(code).includes("FORBIDDEN") || String(code).includes("DENIED"),
  );
  return (
    <InfoPopover
      title={t("kubernetesResources.readStatus")}
      label={t("kubernetesResources.readStatus") + " · " + title}
      attention
    >
      <p>
        {t(
          forbidden
            ? "kubernetesResources.connectionReadPermission"
            : "kubernetesResources.partial",
        )}
      </p>
      <dl>
        {entries.map(([kind, code]) => (
          <div key={kind}>
            <dt>{kind}</dt>
            <dd>
              {formatErrorCode(
                String(code),
                t("kubernetesResources.notAvailable"),
              )}
            </dd>
          </div>
        ))}
      </dl>
      {target.code && (
        <p>
          {formatErrorCode(target.code, t("kubernetesResources.notAvailable"))}
        </p>
      )}
    </InfoPopover>
  );
}
