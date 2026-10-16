import { Maximize2, Pencil, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { DashboardPanel, DashboardSchemas } from "@argus/api-client";
import {
  Badge,
  Button,
  ObservationPanel,
  DashboardPanelHeader,
} from "@argus/ui";
export function PanelTile({
  panel,
  execution,
  editable,
  onEdit,
  onRemove,
  onExpand,
  onSelect,
}: {
  panel: DashboardPanel;
  execution?: DashboardSchemas["DashboardPanelExecution"];
  editable?: boolean;
  onEdit?: () => void;
  onRemove?: () => void;
  onExpand?: () => void;
  onSelect?: (row: Record<string, unknown>, target: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DashboardPanelHeader
        title={panel.title}
        subtitle={`${panel.source_binding.source_type} · ${panel.signal}`}
        editing={editable}
      >
        {execution && execution.status !== "success" && (
          <Badge>
            {t(`dashboards.statuses.${execution.status}`, {
              defaultValue: execution.status,
            })}
          </Badge>
        )}
        {onExpand && (
          <Button
            isIconOnly
            variant="ghost"
            aria-label={`${t("dashboards.expand")} ${panel.title}`}
            onPress={onExpand}
          >
            <Maximize2 size={15} />
          </Button>
        )}
        {editable && (
          <>
            <Button
              isIconOnly
              variant="ghost"
              aria-label={`${t("dashboards.editPanel")} ${panel.title}`}
              onPress={onEdit}
            >
              <Pencil size={15} />
            </Button>
            <Button
              isIconOnly
              variant="ghost"
              aria-label={`${t("dashboards.removePanel")} ${panel.title}`}
              onPress={onRemove}
            >
              <Trash2 size={15} />
            </Button>
          </>
        )}
      </DashboardPanelHeader>
      {panel.type.startsWith("apm_") && (
        <p className="argus-dashboard-basis">{t("dashboards.sampleBasis")}</p>
      )}
      <div className="argus-dashboard-panel-content">
        <ObservationPanel
          title={panel.title}
          type={panel.type}
          signal={panel.signal}
          targets={execution?.targets ?? []}
          unit={panel.unit}
          decimals={panel.decimals}
          legend={panel.legend}
          display={panel.display}
          thresholds={panel.thresholds}
          onSelect={onSelect}
        />
      </div>
    </>
  );
}
