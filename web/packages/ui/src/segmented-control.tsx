import { RadioGroup } from "@heroui/react/radio-group";
import { Radio } from "@heroui/react/radio";
export function SegmentedControl({
  label,
  value,
  options,
  onChange,
  className,
  disabled,
  density = "default",
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  className?: string;
  disabled?: boolean;
  density?: "default" | "compact";
}) {
  return (
    <RadioGroup
      aria-label={label}
      value={value}
      onChange={onChange}
      orientation="horizontal"
      isDisabled={disabled}
      className={[
        "argus-segments",
        density === "compact" && "argus-segments--compact",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    >
      {options.map((option) => (
        <Radio
          value={option.value}
          key={option.value}
          className="argus-segments__item"
        >
          <Radio.Content className="argus-segments__control">
            {option.label}
          </Radio.Content>
        </Radio>
      ))}
    </RadioGroup>
  );
}
