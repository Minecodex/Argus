import { useState } from "react";
import { CalendarClock, ChevronDown } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { DashboardSchemas } from "@argus/api-client";
import {
  Button,
  DateTimePicker,
  FilterPopover,
  Field,
  Input,
  Select,
} from "@argus/ui";
type Range = DashboardSchemas["DashboardTimeRange"];
const local = (iso?: string) => {
  if (!iso) return "";
  const date = new Date(iso);
  if (!Number.isFinite(date.getTime())) return "";
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
};
export function TimeRangePicker({
  value,
  onChange,
  label,
}: {
  value: Range;
  onChange: (value: Range) => void;
  label: string;
}) {
  const { t } = useTranslation(),
    [open, setOpen] = useState(false),
    [draft, setDraft] = useState(value);
  const valid =
    draft.kind === "relative"
      ? Number(draft.seconds) > 0
      : Boolean(
          draft.from &&
          draft.to &&
          Date.parse(draft.to) > Date.parse(draft.from),
        );
  const presets = new Map([
    [3600, "oneHour"],
    [21600, "sixHours"],
    [86400, "day"],
    [604800, "week"],
  ]);
  const display =
    value.kind === "absolute"
      ? `${local(value.from)} — ${local(value.to)}`
      : presets.has(value.seconds ?? 0)
        ? t(`dashboards.${presets.get(value.seconds!)}`)
        : t("dashboards.relativeSeconds", { count: value.seconds });
  return (
    <FilterPopover
      size="lg"
      title={label}
      open={open}
      onOpenChange={setOpen}
      trigger={
        <Button
          aria-label={label}
          title={
            value.kind === "absolute"
              ? `${local(value.from)} — ${local(value.to)}`
              : display
          }
          onPress={() => {
            setDraft(value);
            setOpen(true);
          }}
        >
          <CalendarClock aria-hidden />
          {display}
          <ChevronDown aria-hidden />
        </Button>
      }
      footer={
        <>
          <Button onPress={() => setOpen(false)}>{t("common.cancel")}</Button>
          <Button
            variant="primary"
            isDisabled={!valid}
            onPress={() => {
              onChange(draft);
              setOpen(false);
            }}
          >
            {t("dashboards.apply")}
          </Button>
        </>
      }
    >
      <div className="argus-dashboard-form-stack">
        <Select
          ariaLabel={t("dashboards.timeKind")}
          value={draft.kind}
          options={[
            { value: "relative", label: t("dashboards.relativeTime") },
            { value: "absolute", label: t("dashboards.absoluteTime") },
          ]}
          onValueChange={(kind) =>
            setDraft(
              kind === "relative"
                ? { kind, seconds: 3600 }
                : {
                    kind: "absolute",
                    from: new Date(Date.now() - 3600000).toISOString(),
                    to: new Date().toISOString(),
                  },
            )
          }
        />
        {draft.kind === "relative" ? (
          <>
            <Select
              ariaLabel={t("dashboards.timePresets")}
              value={String(draft.seconds ?? 3600)}
              options={[...presets]
                .map(([seconds, key]) => ({
                  value: String(seconds),
                  label: t(`dashboards.${key}`),
                }))
                .concat(
                  presets.has(draft.seconds ?? 0)
                    ? []
                    : [
                        {
                          value: String(draft.seconds),
                          label: t("dashboards.relativeSeconds", {
                            count: draft.seconds,
                          }),
                        },
                      ],
                )}
              onValueChange={(seconds) =>
                setDraft({ kind: "relative", seconds: Number(seconds) })
              }
            />
            <Field label={t("dashboards.timeSeconds")} requirement="required">
              <Input
                type="number"
                min={1}
                value={draft.seconds ?? 3600}
                onChange={(e) =>
                  setDraft({
                    kind: "relative",
                    seconds: Number(e.target.value),
                  })
                }
              />
            </Field>
          </>
        ) : (
          <>
            <Field label={t("dashboards.timeFrom")} requirement="required">
              <DateTimePicker
                value={local(draft.from)}
                onChange={(value) =>
                  setDraft({
                    ...draft,
                    from: value ? new Date(value).toISOString() : undefined,
                  })
                }
              />
            </Field>
            <Field label={t("dashboards.timeTo")} requirement="required">
              <DateTimePicker
                value={local(draft.to)}
                onChange={(value) =>
                  setDraft({
                    ...draft,
                    to: value ? new Date(value).toISOString() : undefined,
                  })
                }
              />
            </Field>
            <p className="argus-dashboard-muted">
              {t("dashboards.localTimeHint")}
            </p>
          </>
        )}
      </div>
    </FilterPopover>
  );
}
