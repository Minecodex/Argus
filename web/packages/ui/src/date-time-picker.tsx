import { Calendar } from "@heroui/react/calendar";
import { DateField } from "@heroui/react/date-field";
import { DatePicker as HeroDatePicker } from "@heroui/react/date-picker";
import {
  parseDate,
  parseDateTime,
  type DateValue,
} from "@internationalized/date";
import { forwardRef, useState, type InputHTMLAttributes } from "react";
import { cx } from "./lib";
import { mergeAriaIds, useFieldContext } from "./form";
import { useUiText } from "./locale";
export type DateTimePickerType = "date" | "datetime-local";
export function parsePickerValue(
  value: string | undefined,
  type: DateTimePickerType,
): DateValue | null {
  try {
    return value
      ? type === "date"
        ? parseDate(value.slice(0, 10))
        : parseDateTime(value)
      : null;
  } catch {
    return null;
  }
}
/** CalendarDateTime keeps the local wall-clock contract; no implicit UTC conversion. */
export const DateTimePicker = forwardRef<
  HTMLDivElement,
  Omit<
    InputHTMLAttributes<HTMLInputElement>,
    "onChange" | "type" | "value" | "onBlur"
  > & {
    value?: string;
    type?: DateTimePickerType;
    onChange?: (value: string) => void;
    onBlur?: () => void;
  }
>(
  (
    {
      className,
      value,
      type = "datetime-local",
      onChange,
      onBlur,
      min,
      max,
      id,
      name,
      required,
      disabled,
      ...props
    },
    ref,
  ) => {
    const field = useFieldContext(),
      text = useUiText();
    const [open, setOpen] = useState(false);
    return (
      <HeroDatePicker
        ref={ref}
        id={id ?? field?.controlId}
        className={cx("argus-date-time-picker", className)}
        value={parsePickerValue(value, type)}
        isOpen={open}
        onOpenChange={setOpen}
        granularity={type === "date" ? "day" : "minute"}
        hourCycle={24}
        hideTimeZone
        minValue={
          parsePickerValue(typeof min === "string" ? min : undefined, type) ??
          undefined
        }
        maxValue={
          parsePickerValue(typeof max === "string" ? max : undefined, type) ??
          undefined
        }
        name={name}
        isRequired={required ?? field?.required}
        isDisabled={disabled}
        isInvalid={field?.invalid}
        aria-label={props["aria-label"]}
        aria-labelledby={mergeAriaIds(props["aria-labelledby"], field?.labelId)}
        aria-describedby={mergeAriaIds(
          props["aria-describedby"],
          field?.descriptionId,
        )}
        onBlur={onBlur}
        onChange={(next) =>
          onChange?.(next?.toString().slice(0, type === "date" ? 10 : 16) ?? "")
        }
      >
        <DateField.Group
          className="argus-date-time-picker__control"
          onClick={() => !disabled && setOpen(true)}
        >
          <DateField.Input>
            {(segment) => <DateField.Segment segment={segment} />}
          </DateField.Input>
          <DateField.Suffix>
            <HeroDatePicker.Trigger
              aria-label={text("打开日历", "Open calendar")}
            >
              <HeroDatePicker.TriggerIndicator />
            </HeroDatePicker.Trigger>
          </DateField.Suffix>
        </DateField.Group>
        <HeroDatePicker.Popover className="argus-date-time-picker__calendar">
          <Calendar aria-label={text("选择日期", "Choose date")}>
            <Calendar.Header>
              <Calendar.YearPickerTrigger>
                <Calendar.YearPickerTriggerHeading />
                <Calendar.YearPickerTriggerIndicator />
              </Calendar.YearPickerTrigger>
              <Calendar.NavButton slot="previous" />
              <Calendar.NavButton slot="next" />
            </Calendar.Header>
            <Calendar.Grid>
              <Calendar.GridHeader>
                {(day) => <Calendar.HeaderCell>{day}</Calendar.HeaderCell>}
              </Calendar.GridHeader>
              <Calendar.GridBody>
                {(date) => <Calendar.Cell date={date} />}
              </Calendar.GridBody>
            </Calendar.Grid>
          </Calendar>
        </HeroDatePicker.Popover>
      </HeroDatePicker>
    );
  },
);
DateTimePicker.displayName = "DateTimePicker";
