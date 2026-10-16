import { Plus } from "lucide-react";
import type { AlertSchemas } from "@argus/api-client";
import { Button, Field, Input, Select } from "@argus/ui";
import { useAlertText } from "./common";
type Matcher = AlertSchemas["AlertMatcher"];
export function MatcherEditor({
  value,
  onChange,
  label,
}: {
  value: Matcher[];
  onChange: (value: Matcher[]) => void;
  label: string;
}) {
  const { t } = useAlertText();
  return (
    <Field label={label} requirement="optional" controlMode="group">
      <div className="argus-alert-rules">
        {value.map((m, i) => (
          <div className="argus-alert-actions" key={i}>
            <Input
              aria-label={t("alerts.labelKey")}
              size="lg"
              value={m.key}
              onChange={(e) =>
                onChange(
                  value.map((x, n) =>
                    n === i ? { ...x, key: e.target.value } : x,
                  ),
                )
              }
            />
            <Select
              density="compact"
              size="lg"
              ariaLabel={t("alerts.operator")}
              value={m.operator}
              options={["=", "!=", "=~", "!~"].map((value) => ({
                value,
                label: value,
              }))}
              onValueChange={(op) =>
                onChange(
                  value.map((x, n) =>
                    n === i ? { ...x, operator: op as Matcher["operator"] } : x,
                  ),
                )
              }
            />
            <Input
              size="lg"
              aria-label={t("alerts.labelValue")}
              value={m.value}
              onChange={(e) =>
                onChange(
                  value.map((x, n) =>
                    n === i ? { ...x, value: e.target.value } : x,
                  ),
                )
              }
            />
            <Button
              density="compact"
              variant="ghost"
              size="sm"
              onPress={() => onChange(value.filter((_, n) => n !== i))}
            >
              {t("alerts.remove")}
            </Button>
          </div>
        ))}
        <Button
          density="compact"
          size="sm"
          onPress={() =>
            onChange([
              ...value,
              { key: "env", operator: "=", value: "production" },
            ])
          }
        >
          <Plus size={14} />
          {t("alerts.addMatcher")}
        </Button>
      </div>
    </Field>
  );
}
