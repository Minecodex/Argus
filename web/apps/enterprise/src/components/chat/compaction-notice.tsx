import { useTranslation } from "react-i18next";
import type { CompactionView } from "./event-reducer";

export function CompactionNotice({ value }: { value: CompactionView }) {
  const { t } = useTranslation();
  if (value.status === "idle") return null;
  let key: string = value.status;
  if (value.status === "running")
    key =
      value.tokens_before === undefined
        ? "runningUnknown"
        : value.estimated
          ? "runningEstimated"
          : "running";
  if (value.status === "completed")
    key =
      value.tokens_before === undefined || value.tokens_after === undefined
        ? "completedUnknown"
        : value.estimated
          ? "completedEstimated"
          : "completed";
  return (
    <div className="argus-chat-composer__note" role="status">
      {t(`chat.compaction.${key}`, {
        before: value.tokens_before,
        after: value.tokens_after,
      })}
    </div>
  );
}
