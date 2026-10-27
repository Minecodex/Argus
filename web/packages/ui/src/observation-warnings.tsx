import { Alert } from "./code";
import { useUiText } from "./locale";

export function ObservationWarnings({
  warnings,
}: {
  warnings?: readonly string[];
}) {
  const text = useUiText();
  if (!warnings?.length) return null;
  const descriptions = [...new Set(warnings)].map((warning) =>
    warning.startsWith("NAMESPACE_VERIFIED_OBSERVATIONS_ONLY:")
      ? text(
          "Namespace 指标仅包含已验证 UID 归属的观测；未验证归属的数据仅对相应集群的授权用户可见。",
          "Namespace metrics include only observations with verified UID attribution. Unverified observations are visible only to users authorized for the Cluster.",
        )
      : warning,
  );
  return (
    <Alert
      title={text("查询说明", "Query information")}
      description={descriptions.join("; ")}
      tone="warning"
    />
  );
}
