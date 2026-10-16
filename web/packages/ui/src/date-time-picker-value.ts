import {
  parseDate,
  parseDateTime,
  type DateValue,
} from "@internationalized/date";
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
