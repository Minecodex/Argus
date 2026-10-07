import { RadioGroup } from "@heroui/react/radio-group";
import { Radio } from "@heroui/react/radio";
export function SegmentedControl({
  label,
  value,
  options,
  onChange,
  className,
  disabled,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  className?: string;
  disabled?: boolean;
}) {
  return (
    <RadioGroup
      aria-label={label}
      value={value}
      onChange={onChange}
      orientation="horizontal"
      isDisabled={disabled}
      className={className ? `argus-segments ${className}` : "argus-segments"}
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
