import type { AIModel, ModelAvailability } from "@argus/api-client";
import { Select } from "@argus/ui";
import { useTranslation } from "react-i18next";

export function ConversationModelSelect({
  models,
  availability,
  value,
  disabled,
  onChange,
}: {
  models: AIModel[];
  availability: ModelAvailability[];
  value: string;
  disabled?: boolean;
  onChange(value: string): void;
}) {
  const { t } = useTranslation();
  return (
    <Select
      className="argus-chat-composer__model"
      ariaLabel={t("chat.model.select")}
      value={value}
      onValueChange={onChange}
      disabled={disabled}
      size="sm"
      appearance="pill"
      sizing="selection"
      placement="top end"
      options={models.map((model) => {
        const state = availability.find((entry) => entry.modelId === model.id);
        return {
          value: model.id,
          disabled: !state?.available,
          label: `${model.name}${state?.available ? "" : ` · ${t(`chat.model.reason.${state?.reason ?? "unavailable"}`)}`}`,
        };
      })}
    />
  );
}
