import { RadioGroup } from "@heroui/react/radio-group";
import { Radio } from "@heroui/react/radio";
export function RadioOptions({
  label,
  value,
  options,
  onChange,
  disabled,
}: {
  label: string;
  value: string;
  options: { value: string; label: string; disabled?: boolean }[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <RadioGroup
      aria-label={label}
      value={value}
      onChange={onChange}
      isDisabled={disabled}
      className="argus-choice-list"
    >
      {options.map((option) => (
        <Radio
          value={option.value}
          key={option.value}
          isDisabled={option.disabled}
          className="argus-choice-list__item"
        >
          <Radio.Content>
            <Radio.Control>
              <Radio.Indicator />
            </Radio.Control>
            {option.label}
          </Radio.Content>
        </Radio>
      ))}
    </RadioGroup>
  );
}
