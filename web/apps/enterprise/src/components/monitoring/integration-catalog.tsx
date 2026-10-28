import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { MonitoringIntegration } from "@argus/api-client";
import {
  Badge,
  Button,
  DataTable,
  Dialog,
  FilterBar,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";

const categories = [
  "all",
  "database",
  "cache",
  "messaging",
  "web",
  "probe",
  "traces",
  "logs",
  "runtime",
  "storage",
] as const;
function category(item: MonitoringIntegration) {
  if (item.family !== "ingestion") return item.family;
  return ["otlp", "skywalking", "jaeger"].includes(item.key)
    ? "traces"
    : "logs";
}

export function IntegrationCatalog({
  items,
  canCreate,
  onCreate,
}: {
  items: MonitoringIntegration[];
  canCreate: boolean;
  onCreate: (entry: MonitoringIntegration) => void;
}) {
  const { t } = useTranslation(),
    [tab, setTab] = useState("all"),
    [search, setSearch] = useState("");
  const visible = items.filter(
    (item) =>
      (tab === "all" || category(item) === tab) &&
      `${item.name} ${item.key}`.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <div className="argus-monitoring-catalog">
      <FilterBar
        search={{
          value: search,
          onChange: setSearch,
          placeholder: t("monitoring.searchIntegrations"),
        }}
      />
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          {categories.map((key) => (
            <TabsTrigger key={key} value={key}>
              {t(`monitoring.categories.${key}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {categories.map((key) => (
          <TabsContent key={key} value={key}>
            <DataTable
              data={visible}
              getRowKey={(item) => item.key}
              columns={[
                { key: "name", header: t("monitoring.integration") },
                {
                  key: "signals",
                  header: t("monitoring.signals"),
                  render: (item) =>
                    item.signals
                      .map((signal) => t(`monitoring.signalsLabel.${signal}`))
                      .join(" / "),
                },
                {
                  key: "validation",
                  header: t("monitoring.availability"),
                  render: (item) => (
                    <Badge>
                      {t(
                        item.selection !== "candidate"
                          ? "monitoring.deferred"
                          : item.validation === "passed"
                            ? "monitoring.available"
                            : item.validation === "blocked"
                              ? "monitoring.validationBlocked"
                              : item.adapter_ready
                                ? "monitoring.validationPending"
                                : "monitoring.notOpen",
                      )}
                    </Badge>
                  ),
                },
                {
                  key: "reason",
                  header: t("monitoring.supportScope"),
                  render: (item) =>
                    item.key === "tls"
                      ? t("monitoring.certificateProbeScope")
                      : item.reason
                        ? t(`monitoring.reasons.${item.reason}`)
                        : t("monitoring.variantVerificationHint"),
                },
                {
                  key: "action",
                  header: t("monitoring.actions"),
                  render: (item) => (
                    <Button
                      size="sm"
                      isDisabled={
                        !canCreate ||
                        !item.adapter_ready ||
                        item.selection !== "candidate"
                      }
                      onPress={() => onCreate(item)}
                    >
                      {t("monitoring.createCollection")}
                    </Button>
                  ),
                },
              ]}
            />
          </TabsContent>
        ))}
      </Tabs>
    </div>
  );
}

export function IntegrationPicker({
  items,
  onSelect,
  onClose,
}: {
  items: MonitoringIntegration[];
  onSelect: (entry: MonitoringIntegration) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog
      open
      size="cover"
      title={t("monitoring.chooseIntegration")}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <IntegrationCatalog items={items} canCreate onCreate={onSelect} />
    </Dialog>
  );
}
