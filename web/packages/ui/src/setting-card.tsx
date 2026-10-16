import { useId } from "react";
import { Switch } from "./form";

/** A described, reversible setting; callers own draft and commit semantics. */
export function SettingCard({
  title,
  description,
  checked,
  onChange,
  disabled = false,
}: {
  title: string;
  description: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  const descriptionId = useId();
  return (
    <div className="argus-setting-card" data-selected={checked || undefined}>
      <div className="argus-setting-card__content">
        <strong>{title}</strong>
        <p id={descriptionId}>{description}</p>
      </div>
      <Switch
        label={title}
        descriptionId={descriptionId}
        checked={checked}
        onChange={onChange}
        disabled={disabled}
      />
    </div>
  );
}
