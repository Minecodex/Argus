import { useTranslation } from "react-i18next";
import {
  builtinCollections,
  type MonitoringIntegration,
  type MonitoringListInput,
} from "@argus/api-client";
import { FilterBar } from "@argus/ui";

export type CollectionFilterValue = Pick<
  MonitoringListInput,
  "integration" | "desired_enabled" | "apply_status" | "health" | "freshness"
>;

export function CollectionFilters({
  value,
  search,
  integrations,
  onChange,
  onSearch,
  onRefresh,
  refreshing,
}: {
  value: CollectionFilterValue;
  search: string;
  integrations: MonitoringIntegration[];
  onChange: (value: CollectionFilterValue) => void;
  onSearch: (search: string) => void;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const { t, i18n } = useTranslation();
  const types = [
    ...builtinCollections.map((entry) => ({
      value: entry.key,
      label: t(`builtinCollections.names.${entry.key}`),
    })),
    ...integrations.map((entry) => ({ value: entry.key, label: entry.name })),
  ].sort((a, b) => a.label.localeCompare(b.label, i18n.language));
  return (
    <FilterBar
      className="argus-monitoring-collection-filters"
      search={{
        value: search,
        onChange: onSearch,
        placeholder: t("monitoring.searchCollections"),
      }}
      filters={[
        {
          key: "integration",
          ariaLabel: t("monitoring.integration"),
          allLabel: t("monitoring.filters.allTypes"),
          value: value.integration ?? "",
          options: types,
          onChange: (integration) =>
            onChange({ ...value, integration: integration || undefined }),
        },
        {
          key: "apply_status",
          ariaLabel: t("monitoring.applicationState"),
          allLabel: t("monitoring.filters.allApplyStates"),
          value: value.apply_status ?? "",
          options: [
            "pending",
            "applying",
            "effective",
            "failed",
            "unknown",
          ].map((state) => ({
            value: state,
            label: t(`monitoring.applyStates.${state}`),
          })),
          onChange: (state) =>
            onChange({
              ...value,
              apply_status: (state ||
                undefined) as CollectionFilterValue["apply_status"],
            }),
        },
        {
          key: "health",
          ariaLabel: t("monitoring.collectionHealth"),
          allLabel: t("monitoring.filters.allHealthStates"),
          value: value.health ?? "",
          options: ["unknown", "healthy", "degraded", "failed", "stopped"].map(
            (state) => ({
              value: state,
              label: t(`monitoring.healthStates.${state}`),
            }),
          ),
          onChange: (state) =>
            onChange({
              ...value,
              health: (state || undefined) as CollectionFilterValue["health"],
            }),
        },
        {
          key: "freshness",
          ariaLabel: t("monitoring.dataState"),
          allLabel: t("monitoring.filters.allDataStates"),
          value: value.freshness ?? "",
          options: ["no_data", "fresh", "stale"].map((state) => ({
            value: state,
            label: t(`monitoring.freshnessStates.${state}`),
          })),
          onChange: (state) =>
            onChange({
              ...value,
              freshness: (state ||
                undefined) as CollectionFilterValue["freshness"],
            }),
        },
        {
          key: "desired_enabled",
          ariaLabel: t("monitoring.filters.enablement"),
          allLabel: t("monitoring.filters.allEnabledStates"),
          value:
            value.desired_enabled === undefined
              ? ""
              : String(value.desired_enabled),
          options: [
            { value: "true", label: t("monitoring.filters.enabled") },
            { value: "false", label: t("monitoring.filters.disabled") },
          ],
          onChange: (state) =>
            onChange({
              ...value,
              desired_enabled: state === "" ? undefined : state === "true",
            }),
        },
      ]}
      onRefresh={onRefresh}
      refreshing={refreshing}
    />
  );
}
