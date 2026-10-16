import { useState } from "react";
import { ChartNoAxesCombined, Settings2, X } from "lucide-react";
import { DashboardGrid } from "./dashboard-grid";
import type { DashboardLayoutItem } from "./dashboard-layout";
import { DashboardPanelHeader } from "./dashboard-panel-header";
import { Badge } from "./badge";
import { Button } from "./button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "./primitives";
import { useUiText } from "./locale";
import { DataTable } from "./table";

const object = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const format = (value: unknown) =>
  value == null
    ? "—"
    : typeof value === "object"
      ? JSON.stringify(value, null, 2)
      : String(value);
const dimension = (value: unknown, fallback: number, max: number) =>
  typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, Math.min(max, Math.round(value)))
    : fallback;

/** Read-only definitions share the administration grid and panel chrome, never synthetic samples. */
export function DashboardDefinitionView({
  definition,
  metadata = {},
}: {
  definition: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}) {
  const text = useUiText();
  const [selected, setSelected] = useState<string>();
  const panels: (Record<string, unknown> &
    DashboardLayoutItem & { title: string })[] = (
    Array.isArray(definition.panels) ? definition.panels : []
  ).map((raw, index) => {
    const panel = object(raw),
      layout = object(panel.layout);
    const w = Math.max(1, dimension(layout.w, 6, 12));
    return {
      ...panel,
      id: String(panel.id ?? index),
      title: String(
        panel.title ??
          panel.name ??
          panel.id ??
          text("未命名面板", "Untitled panel"),
      ),
      layout: {
        x: dimension(layout.x, (index % 2) * 6, 12 - w),
        y: dimension(layout.y, Math.floor(index / 2) * 6, 1000),
        w,
        h: Math.max(2, dimension(layout.h, 6, 1000)),
        min_w: 1,
        min_h: 2,
      },
    };
  });
  const panel = panels.find((item) => item.id === selected);
  const variables = (
    Array.isArray(definition.variables) ? definition.variables : []
  ).map(object);
  return (
    <section className="argus-definition-view">
      <div className="argus-definition-toolbar">
        <Badge tone="neutral">{text("只读定义", "Read-only definition")}</Badge>
        <span>
          {text("版本", "Revision")}{" "}
          {format(metadata.version ?? metadata.revision_number)}
        </span>
        <span>
          {panels.length} {text("个面板", "panels")}
        </span>
        <span>{text("修改请告诉 AI", "Ask AI to make changes")}</span>
      </div>
      <Tabs defaultValue="layout">
        <TabsList>
          <TabsTrigger value="layout">
            {text("布局预览", "Layout preview")}
          </TabsTrigger>
          <TabsTrigger value="configuration">
            {text("仪表盘配置", "Dashboard configuration")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="layout">
          {!panels.length ? (
            <p>{text("尚未添加面板", "No panels yet")}</p>
          ) : (
            <div
              className={`argus-definition-workspace ${panel ? "has-inspector" : ""}`}
            >
              <DashboardGrid
                items={panels}
                rowHeight={Math.max(
                  24,
                  dimension(object(definition.layout).row_height, 40, 80),
                )}
              >
                {(item) => (
                  <>
                    <DashboardPanelHeader
                      title={item.title}
                      subtitle={[
                        object(item.source_binding).source_type,
                        item.signal,
                        item.type,
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    >
                      <Button
                        variant="ghost"
                        size="sm"
                        isIconOnly
                        aria-label={`${text("查看配置", "View configuration")} · ${item.title}`}
                        onPress={() => setSelected(item.id)}
                      >
                        <Settings2 size={15} />
                      </Button>
                    </DashboardPanelHeader>
                    <div className="argus-definition-empty">
                      <ChartNoAxesCombined aria-hidden size={28} />
                      <strong>{String(item.type ?? "")}</strong>
                      <span>
                        {text(
                          "仅展示布局，尚无查询样本",
                          "Layout only — no query samples",
                        )}
                      </span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onPress={() => setSelected(item.id)}
                      >
                        {text("查看配置", "View configuration")}
                      </Button>
                    </div>
                  </>
                )}
              </DashboardGrid>
              {panel && (
                <aside
                  className="argus-definition-inspector"
                  aria-label={text("面板配置", "Panel configuration")}
                >
                  <header>
                    <strong>{panel.title}</strong>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="ghost"
                      aria-label={text(
                        "关闭面板配置",
                        "Close panel configuration",
                      )}
                      onPress={() => setSelected(undefined)}
                    >
                      <X size={16} />
                    </Button>
                  </header>
                  <Tabs key={panel.id} defaultValue="queries">
                    <TabsList>
                      <TabsTrigger value="queries">
                        {text("查询", "Queries")}
                      </TabsTrigger>
                      <TabsTrigger value="display">
                        {text("展示", "Display")}
                      </TabsTrigger>
                      <TabsTrigger value="scope">
                        {text("范围", "Scope")}
                      </TabsTrigger>
                    </TabsList>
                    <TabsContent value="queries">
                      {(!Array.isArray(panel.targets) ||
                        !panel.targets.length) && (
                        <p>{text("尚未配置查询", "No query configured")}</p>
                      )}
                      {(Array.isArray(panel.targets) ? panel.targets : []).map(
                        (raw, index) => {
                          const target = object(raw);
                          return (
                            <section
                              className="argus-definition-query"
                              key={String(target.id ?? index)}
                            >
                              <strong>
                                {String(target.id ?? index + 1)} ·{" "}
                                {String(target.language ?? "")}
                              </strong>
                              <pre>
                                {format(
                                  target.expression ??
                                    target.query ??
                                    object(object(target.source_definition).dsl)
                                      .expression ??
                                    target.source_definition ??
                                    target,
                                )}
                              </pre>
                              <DefinitionFields
                                entries={Object.entries(target).filter(
                                  ([key]) =>
                                    ![
                                      "id",
                                      "language",
                                      "source_definition",
                                      "query",
                                      "expression",
                                    ].includes(key),
                                )}
                              />
                            </section>
                          );
                        },
                      )}
                    </TabsContent>
                    <TabsContent value="display">
                      <DefinitionFields
                        entries={Object.entries(panel).filter(([key]) =>
                          [
                            "type",
                            "unit",
                            "decimals",
                            "legend",
                            "thresholds",
                            "display",
                            "layout",
                          ].includes(key),
                        )}
                      />
                    </TabsContent>
                    <TabsContent value="scope">
                      <DefinitionFields
                        entries={Object.entries(panel).filter(([key]) =>
                          [
                            "source_binding",
                            "local_filters",
                            "time_override",
                            "transformations",
                            "drilldowns",
                          ].includes(key),
                        )}
                      />
                    </TabsContent>
                  </Tabs>
                </aside>
              )}
            </div>
          )}
        </TabsContent>
        <TabsContent value="configuration">
          <DefinitionFields
            entries={Object.entries(definition).filter(
              ([key]) => key !== "panels" && key !== "variables",
            )}
          />
          {variables.length > 0 && (
            <DataTable
              data={variables}
              getRowKey={(row) => String(row.name)}
              columns={[
                {
                  key: "name",
                  header: text("变量", "Variable"),
                  render: (row) => String(row.label ?? row.name),
                },
                {
                  key: "default",
                  header: text("默认条件", "Default selection"),
                  render: (row) => format(row.default),
                },
                {
                  key: "definition",
                  header: text("定义", "Definition"),
                  render: (row) => <pre>{format(row)}</pre>,
                },
              ]}
            />
          )}
        </TabsContent>
      </Tabs>
      <details className="argus-definition-source">
        <summary>
          {text("完整配置定义", "Full configuration definition")}
        </summary>
        <pre>{format(definition)}</pre>
      </details>
    </section>
  );
}
function DefinitionFields({ entries }: { entries: [string, unknown][] }) {
  const text = useUiText();
  const labels: Record<string, string> = {
    type: text("图型", "Chart type"),
    unit: text("单位", "Unit"),
    decimals: text("小数位", "Decimals"),
    legend: text("图例", "Legend"),
    thresholds: text("阈值", "Thresholds"),
    display: text("显示选项", "Display options"),
    layout: text("布局", "Layout"),
    source_binding: text("采集来源", "Data source"),
    local_filters: text("面板筛选", "Panel filters"),
    time_override: text("面板时间", "Panel time range"),
    transformations: text("数据转换", "Transformations"),
    drilldowns: text("下钻", "Drilldowns"),
    default_time_range: text("默认时间范围", "Default time range"),
    default_refresh_seconds: text(
      "刷新间隔（秒）",
      "Refresh interval (seconds)",
    ),
  };
  return (
    <dl className="argus-definition-fields">
      {entries.map(([key, value]) => (
        <div key={key}>
          <dt>{labels[key] ?? key}</dt>
          <dd>
            <pre>{format(value)}</pre>
          </dd>
        </div>
      ))}
    </dl>
  );
}
