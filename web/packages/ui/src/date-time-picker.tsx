import { forwardRef, lazy, Suspense, type ComponentProps } from "react";
import type { DateTimePicker as Content } from "./date-time-picker-content";
import { Spinner } from "./display";
export type { DateTimePickerType } from "./date-time-picker-content";
export { parsePickerValue } from "./date-time-picker-value";
const Picker = lazy(() =>
  import("./date-time-picker-content").then((module) => ({
    default: module.DateTimePicker,
  })),
);
/** Load the bundled calendar only on screens that contain a date field. */
export const DateTimePicker = forwardRef<
  HTMLDivElement,
  ComponentProps<typeof Content>
>((props, ref) => (
  <Suspense fallback={<Spinner />}>
    <Picker {...props} ref={ref} />
  </Suspense>
));
DateTimePicker.displayName = "DateTimePicker";
