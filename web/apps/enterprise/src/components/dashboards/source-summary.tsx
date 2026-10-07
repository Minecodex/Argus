import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { DashboardSchemas } from "@argus/api-client";
import { Badge, Button, Dialog, KeyValueGrid } from "@argus/ui";

type Source = DashboardSchemas["DashboardResolvedSource"];
export function installationLabel(source: Source, t: (key: string) => string) {
  return t(
    source.current_installation === undefined
      ? "dashboards.unknownInstallation"
      : source.current_installation
        ? "dashboards.currentInstallation"
        : "dashboards.historicalInstallation",
  );
}
export function sourceValueLabels(
  sources: Source[],
  t: (key: string) => string,
): Record<string, string> {
  return Object.fromEntries(
    sources.map((source) => [
      source.id,
      `${installationLabel(source, t)} · ${source.id}`,
    ]),
  );
}
export function SourceSummary({
  title,
  sources,
  resourceNames = {},
}: {
  title: string;
  sources: Source[];
  resourceNames?: Record<string, string>;
}) {
  const [open, setOpen] = useState(false);
  const { t, i18n } = useTranslation();
  const groups = new Map<string, Source[]>();
  for (const source of sources)
    groups.set(source.id, [...(groups.get(source.id) ?? []), source]);
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        aria-label={`${title} ${t("dashboards.resolvedSources")}`}
        onPress={() => setOpen(true)}
      >
        {t("dashboards.resolvedSources")} · {groups.size}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`${title} · ${t("dashboards.resolvedSources")}`}
        size="lg"
      >
        <p>{t("dashboards.sourceSnapshotHint")}</p>
        {!groups.size && <p>{t("dashboards.noResolvedSources")}</p>}
        <div className="argus-dashboard-form-stack">
          {[...groups.entries()].map(([id, versions]) => {
            const source = versions[0]!;
            const registered = versions
              .map((v) => v.registered_at)
              .filter((v): v is string => Boolean(v))
              .sort()[0];
            const date = registered ? new Date(registered) : undefined;
            return (
              <section key={id} aria-label={id}>
                <Badge>{installationLabel(source, t)}</Badge>
                <KeyValueGrid
                  columns={1}
                  items={[
                    { label: t("dashboards.sourceIdentity"), value: id },
                    { label: t("dashboards.source"), value: source.type },
                    {
                      label: t("dashboards.resources"),
                      value:
                        resourceNames[source.resource_id] ?? source.resource_id,
                    },
                    {
                      label: t("dashboards.sourceGeneration"),
                      value: source.generation,
                    },
                    {
                      label: t("dashboards.sourceRevisions"),
                      value: [...new Set(versions.map((v) => v.revision))]
                        .sort((a, b) => a - b)
                        .join(", "),
                    },
                    {
                      label: t("dashboards.sourceRegistered"),
                      value:
                        date && Number.isFinite(date.getTime())
                          ? new Intl.DateTimeFormat(i18n.language, {
                              dateStyle: "medium",
                              timeStyle: "medium",
                            }).format(date)
                          : "—",
                    },
                  ]}
                />
              </section>
            );
          })}
        </div>
      </Dialog>
    </>
  );
}
