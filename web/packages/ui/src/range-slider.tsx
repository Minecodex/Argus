import { Slider } from "@heroui/react/slider";

/** One playback position contract for terminal and RDP recordings. */
export function RangeSlider({
  value,
  min = 0,
  max,
  step = 1,
  onChange,
  label,
  disabled,
  valueText,
}: {
  value: number;
  min?: number;
  max: number;
  step?: number;
  onChange: (value: number) => void;
  label: string;
  disabled?: boolean;
  valueText?: string;
}) {
  return (
    <Slider
      aria-label={label}
      value={value}
      minValue={min}
      maxValue={max}
      step={step}
      isDisabled={disabled}
      onChange={(next) => onChange(typeof next === "number" ? next : next[0]!)}
      className="argus-range-slider"
    >
      <Slider.Track className="argus-range-slider__track">
        <Slider.Fill />
        <Slider.Thumb aria-label={label} aria-valuetext={valueText} />
      </Slider.Track>
    </Slider>
  );
}
