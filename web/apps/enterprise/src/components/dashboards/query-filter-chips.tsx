import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, X } from "lucide-react";
import { Button, Dialog } from "@argus/ui";
import type { DashboardSchemas, DashboardVariable } from "@argus/api-client";
import { FilterFields } from "./filter-fields";
type Filter = DashboardSchemas["DashboardFilter"];

/** Common filters stay discoverable; full bindings remain in the query editor. */
export function QueryFilterChips({
  filters,
  variables,
  locals,
  onChange,
}: {
  filters: Filter[];
  variables: DashboardVariable[];
  locals: string[];
  onChange: (filters: Filter[]) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false),
    [pending, setPending] = useState<Filter[]>([]);
  const edit = () => {
    setPending(structuredClone(filters));
    setOpen(true);
  };
  return (
    <div className="argus-panel-editor__filter-chips">
      {filters.map((filter, index) => (
        <span className="argus-panel-editor__filter-chip" key={index}>
          <Button variant="ghost" onPress={edit}>
            {filter.field} {filter.operator}{" "}
            {filter.variable
              ? `$${filter.variable}`
              : filter.local_parameter
                ? `$${filter.local_parameter}`
                : (filter.values?.join(", ") ?? filter.value)}
          </Button>
          <Button
            variant="ghost"
            isIconOnly
            aria-label={t("dashboards.editor.removeFilter", {
              field: filter.field,
            })}
            onPress={() => onChange(filters.filter((_, n) => n !== index))}
          >
            <X aria-hidden />
          </Button>
        </span>
      ))}
      <Button variant="ghost" onPress={edit}>
        <Plus aria-hidden />
        {t("dashboards.addFilter")}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t("dashboards.editor.queryFilters")}
        size="lg"
        footer={
          <>
            <Button onPress={() => setOpen(false)}>
              {t("dashboards.cancel")}
            </Button>
            <Button
              variant="primary"
              isDisabled={pending.some((f) => !f.field.trim())}
              onPress={() => {
                onChange(pending);
                setOpen(false);
              }}
            >
              {t("dashboards.apply")}
            </Button>
          </>
        }
      >
        <FilterFields
          filters={pending}
          onChange={setPending}
          variables={variables}
          locals={locals}
        />
      </Dialog>
    </div>
  );
}
