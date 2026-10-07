import { Select as HeroSelect } from "@heroui/react/select";
import { ListBox } from "@heroui/react/list-box";
import { Label } from "@heroui/react/label";
import { Description } from "@heroui/react/description";
import { type AriaAttributes, type ReactNode } from "react";
import { mergeAriaIds, useFieldContext } from "./form";
import { cx } from "./lib";
import type { ControlSize } from "./button";
import { useUiText } from "./locale";

export type SelectOption = {
  value: string;
  label: ReactNode;
  textValue?: string;
  description?: string;
  disabled?: boolean;
};
export type SelectProps = {
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  ariaLabel?: string;
  className?: string;
  disabled?: boolean;
  id?: string;
  name?: string;
  required?: boolean;
  size?: ControlSize;
} & Pick<
  AriaAttributes,
  "aria-describedby" | "aria-invalid" | "aria-labelledby" | "aria-required"
>;

export function Select({
  value,
  onValueChange,
  options,
  placeholder,
  ariaLabel,
  className,
  disabled,
  id,
  name,
  required,
  size = "md",
  ...aria
}: SelectProps) {
  const field = useFieldContext();
  const text = useUiText();
  const encode = (value: string) => `argus-option:${value}`;
  return (
    <HeroSelect
      aria-label={ariaLabel}
      aria-labelledby={mergeAriaIds(aria["aria-labelledby"], field?.labelId)}
      aria-describedby={mergeAriaIds(
        aria["aria-describedby"],
        field?.descriptionId,
      )}
      validationBehavior="aria"
      isInvalid={Boolean(aria["aria-invalid"] ?? field?.invalid)}
      isRequired={required ?? field?.required}
      isDisabled={disabled || !options.length}
      name={name}
      value={
        value === "" && !options.some((o) => o.value === "")
          ? null
          : encode(value)
      }
      disabledKeys={options
        .filter((o) => o.disabled)
        .map((o) => encode(o.value))}
      className={cx("argus-select", `argus-control--${size}`, className)}
      placeholder={
        placeholder ??
        (options.length
          ? text("请选择", "Select an option")
          : text("没有可用选项", "No options available"))
      }
      onChange={(next) => {
        const decoded = String(next ?? "").slice("argus-option:".length);
        if (options.some((o) => o.value === decoded && !o.disabled))
          onValueChange(decoded);
      }}
    >
      <HeroSelect.Trigger
        id={id ?? field?.controlId}
        className="argus-select__trigger"
        aria-label={ariaLabel}
        aria-labelledby={field?.labelId}
        aria-describedby={mergeAriaIds(
          aria["aria-describedby"],
          field?.descriptionId,
        )}
        aria-invalid={aria["aria-invalid"] ?? (field?.invalid || undefined)}
        aria-required={
          aria["aria-required"] ?? ((required ?? field?.required) || undefined)
        }
      >
        <HeroSelect.Value className="argus-select__value" />
        <HeroSelect.Indicator />
      </HeroSelect.Trigger>
      <HeroSelect.Popover
        className="argus-select__content"
        placement="bottom start"
      >
        <ListBox className="argus-select__viewport">
          {options.map((option) => (
            <ListBox.Item
              key={encode(option.value)}
              id={encode(option.value)}
              textValue={
                option.textValue ??
                (typeof option.label === "string" ? option.label : option.value)
              }
              className="argus-select__item"
            >
              <div className="argus-select__label">
                <Label>{option.label}</Label>
                {option.description && (
                  <Description>{option.description}</Description>
                )}
              </div>
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </HeroSelect.Popover>
    </HeroSelect>
  );
}
