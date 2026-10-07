import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Badge,
  Button,
  ChartTypeIcon,
  Dialog,
  EmptyState,
  ScenarioCard,
  SearchInput,
  SegmentedControl,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@argus/ui";
import type { DashboardPanel, DashboardSpec } from "@argus/api-client";
import { metricCharts, newPanel } from "./model";
import {
  createChartPanel,
  createPresetPanel,
  panelPresets,
} from "./panel-presets";
import "../../styles/dashboard-panel-presets.css";
export function PanelPresetPicker({
  spec,
  onChoose,
  onClose,
}: {
  spec: DashboardSpec;
  onChoose: (panel: DashboardPanel) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [tab, setTab] = useState("scenarios"),
    [signal, setSignal] = useState("all"),
    [search, setSearch] = useState("");
  const matches = (text: string) =>
    text.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase());
  const presets = panelPresets.filter(
    (p) =>
      (signal === "all" || signal === p.signal) &&
      matches(
        `${t(`dashboardPresets.titles.${p.id}`)} ${t(`dashboardPresets.descriptions.${p.id}`)} ${p.metric ?? ""}`,
      ),
  );
  const charts = metricCharts.filter((type) =>
    matches(t(`dashboards.charts.${type}`)),
  );
  return (
    <Dialog
      open
      title={t("dashboards.addPanel")}
      description={t("dashboardPresets.intro")}
      onOpenChange={(open) => !open && onClose()}
      size="lg"
      className="argus-panel-preset-dialog"
      footer={
        <div className="argus-panel-preset-footer">
          <p className="argus-dashboard-muted">
            {t("dashboardPresets.notEnabled")}
          </p>
          <Button
            onPress={() =>
              onChoose(newPanel(spec, t("dashboards.editor.presets.custom")))
            }
          >
            {t("dashboards.editor.presets.custom")}
          </Button>
        </div>
      }
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="scenarios">
            {t("dashboardPresets.scenarios")}
          </TabsTrigger>
          <TabsTrigger value="charts">
            {t("dashboardPresets.charts")}
          </TabsTrigger>
        </TabsList>
        <div className="argus-panel-preset-toolbar">
          {tab === "scenarios" && (
            <SegmentedControl
              label={t("dashboards.signal")}
              value={signal}
              onChange={setSignal}
              options={[
                { value: "all", label: t("dashboardPresets.all") },
                ...(["metrics", "logs", "traces"] as const).map((value) => ({
                  value,
                  label: t(`dashboards.editor.signals.${value}`),
                })),
              ]}
            />
          )}
          <SearchInput
            aria-label={t("dashboardPresets.search")}
            placeholder={t("dashboardPresets.search")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </div>
        <TabsContent value="scenarios">
          <div className="argus-panel-preset-grid">
            {presets.map((preset) => (
              <ScenarioCard
                key={preset.id}
                className="argus-panel-preset-card"
                selected={false}
                ariaLabel={t(`dashboardPresets.titles.${preset.id}`)}
                title={t(`dashboardPresets.titles.${preset.id}`)}
                description={t(`dashboardPresets.descriptions.${preset.id}`)}
                diagram={<ChartTypeIcon type={preset.type} />}
                footer={
                  <span className="argus-panel-preset-card__meta">
                    <Badge>
                      {t(`dashboards.editor.sources.${preset.source}`)}
                    </Badge>
                    <span>
                      {t(`dashboardPresets.requires.${preset.requires}`)}
                    </span>
                  </span>
                }
                onSelect={() =>
                  onChoose(
                    createPresetPanel(
                      spec,
                      preset,
                      t(`dashboardPresets.titles.${preset.id}`),
                    ),
                  )
                }
              />
            ))}
          </div>
          {!presets.length && (
            <EmptyState title={t("dashboardPresets.noMatch")} description="" />
          )}
        </TabsContent>
        <TabsContent value="charts">
          <p className="argus-dashboard-muted">
            {t("dashboardPresets.chartHint")}
          </p>
          <div className="argus-panel-preset-grid">
            {charts.map((type) => (
              <ScenarioCard
                key={type}
                className="argus-panel-preset-card"
                selected={false}
                ariaLabel={t(`dashboards.charts.${type}`)}
                title={t(`dashboards.charts.${type}`)}
                description={t(`dashboardPresets.shapes.${type}`)}
                diagram={<ChartTypeIcon type={type} />}
                onSelect={() =>
                  onChoose(
                    createChartPanel(
                      spec,
                      type,
                      t(`dashboards.charts.${type}`),
                    ),
                  )
                }
              />
            ))}
          </div>
        </TabsContent>
      </Tabs>
    </Dialog>
  );
}
