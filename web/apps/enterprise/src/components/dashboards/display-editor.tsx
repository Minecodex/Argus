import { Checkbox } from "@argus/ui";
import { useTranslation } from "react-i18next";
import type { DashboardPanel, DashboardSchemas } from "@argus/api-client";
import { Button, Field, Input, Select } from "@argus/ui";

type Display = DashboardSchemas["DashboardDisplayOptions"];
const reductions = ["stat", "gauge", "bar_gauge", "bar", "pie", "table"];
const bounds = [
  "timeseries",
  "gauge",
  "bar_gauge",
  "bar",
  "scatter",
  "heatmap",
];
const thresholdCharts = [
  "stat",
  "gauge",
  "bar_gauge",
  "bar",
  "timeseries",
  "scatter",
  "state_timeline",
];
const numeric = (signal: string, type: string) =>
  signal === "metrics" ||
  (signal === "logs" && !["logs", "table"].includes(type));

// A chart change preserves only settings that the new renderer understands.
// It never changes the query mode or query definition.
export function compatibleDisplay(
  panel: DashboardPanel,
  type: string,
): Partial<DashboardPanel> {
  const display = { ...panel.display };
  if (!reductions.includes(type)) delete display.reducer;
  if (!bounds.includes(type)) {
    delete display.min;
    delete display.max;
  }
  if (type !== "timeseries") {
    delete display.draw_style;
    delete display.stack;
    delete display.smooth;
  }
  return {
    display:
      numeric(panel.signal, type) && Object.keys(display).length
        ? display
        : undefined,
    thresholds:
      numeric(panel.signal, type) && thresholdCharts.includes(type)
        ? panel.thresholds
        : [],
  };
}

export function DisplayEditor({
  panel,
  onChange,
}: {
  panel: DashboardPanel;
  onChange: (patch: Partial<DashboardPanel>) => void;
}) {
  const { t } = useTranslation();
  if (!numeric(panel.signal, panel.type)) return null;
  const display = panel.display ?? {};
  const patch = (value: Partial<Display>) =>
    onChange({ display: { ...display, ...value } });
  return (
    <details open>
      <summary>{t("dashboards.display.title")}</summary>
      <div className="argus-dashboard-form-stack">
        <p className="argus-dashboard-muted">{t("dashboards.display.hint")}</p>
        <div className="argus-dashboard-form-grid">
          {reductions.includes(panel.type) && (
            <Field
              label={t("dashboards.display.reducer")}
              requirement="optional"
            >
              <Select
                value={display.reducer ?? "last"}
                options={["last", "min", "max", "mean", "sum"].map((value) => ({
                  value,
                  label: t(`dashboards.display.reducers.${value}`),
                }))}
                onValueChange={(reducer) =>
                  patch({ reducer: reducer as Display["reducer"] })
                }
              />
            </Field>
          )}
          {bounds.includes(panel.type) &&
            (["min", "max"] as const).map((bound) => (
              <Field
                key={bound}
                label={t(`dashboards.display.${bound}`)}
                requirement="optional"
              >
                <Input
                  type="number"
                  step="any"
                  value={display[bound] ?? ""}
                  placeholder={t("dashboards.display.auto")}
                  onChange={(e) =>
                    patch({
                      [bound]:
                        e.target.value === ""
                          ? undefined
                          : Number(e.target.value),
                    })
                  }
                />
              </Field>
            ))}
          {panel.type === "timeseries" && (
            <Field label={t("dashboards.display.style")} requirement="optional">
              <Select
                value={display.draw_style ?? "line"}
                options={["line", "area", "bar"].map((value) => ({
                  value,
                  label: t(`dashboards.display.styles.${value}`),
                }))}
                onValueChange={(style) =>
                  patch({
                    draw_style: style as Display["draw_style"],
                    smooth: style === "bar" ? false : display.smooth,
                  })
                }
              />
            </Field>
          )}
        </div>
        {reductions.includes(panel.type) && (
          <p className="argus-dashboard-muted">
            {t("dashboards.display.reductionHint")}
          </p>
        )}
        {panel.type === "timeseries" &&
          (["stack", "smooth"] as const).map((key) => (
            <Checkbox
              key={key}
              className="argus-dashboard-check"
              isSelected={display[key] ?? false}
              isDisabled={key === "smooth" && display.draw_style === "bar"}
              onChange={(selected) => patch({ [key]: selected })}
            >
              {t(`dashboards.display.${key}`)}
            </Checkbox>
          ))}
        {display.min !== undefined &&
          display.max !== undefined &&
          display.min >= display.max && (
            <p role="alert">{t("dashboards.display.invalidBounds")}</p>
          )}
        {thresholdCharts.includes(panel.type) && (
          <section
            aria-label={t("dashboards.display.thresholds")}
            className="argus-dashboard-form-stack"
          >
            <strong>{t("dashboards.display.thresholds")}</strong>
            <p className="argus-dashboard-muted">
              {t("dashboards.display.thresholdHint")}
            </p>
            {panel.thresholds.map((threshold, index) => (
              <div key={index} className="argus-dashboard-form-grid">
                <Field
                  label={`${t("dashboards.display.thresholdValue")} ${index + 1}`}
                  requirement="required"
                >
                  <Input
                    type="number"
                    step="any"
                    value={threshold.value}
                    onChange={(e) =>
                      onChange({
                        thresholds: panel.thresholds.map((v, i) =>
                          i === index
                            ? { ...v, value: Number(e.target.value) }
                            : v,
                        ),
                      })
                    }
                  />
                </Field>
                <Field
                  label={`${t("dashboards.display.tone")} ${index + 1}`}
                  requirement="required"
                >
                  <Select
                    value={threshold.tone}
                    options={["info", "success", "warning", "danger"].map(
                      (value) => ({
                        value,
                        label: t(`dashboards.display.tones.${value}`),
                      }),
                    )}
                    onValueChange={(tone) =>
                      onChange({
                        thresholds: panel.thresholds.map((v, i) =>
                          i === index
                            ? { ...v, tone: tone as typeof v.tone }
                            : v,
                        ),
                      })
                    }
                  />
                </Field>
                <Button
                  variant="ghost"
                  onPress={() =>
                    onChange({
                      thresholds: panel.thresholds.filter(
                        (_, i) => i !== index,
                      ),
                    })
                  }
                >
                  {t("dashboards.delete")} {index + 1}
                </Button>
              </div>
            ))}
            {panel.thresholds.some(
              (v, i) => i > 0 && v.value <= panel.thresholds[i - 1]!.value,
            ) && <p role="alert">{t("dashboards.display.thresholdOrder")}</p>}
            <Button
              isDisabled={panel.thresholds.length >= 16}
              onPress={() =>
                onChange({
                  thresholds: [
                    ...panel.thresholds,
                    {
                      value: (panel.thresholds.at(-1)?.value ?? -1) + 1,
                      tone: "warning",
                    },
                  ],
                })
              }
            >
              {t("dashboards.display.addThreshold")}
            </Button>
          </section>
        )}
      </div>
    </details>
  );
}
