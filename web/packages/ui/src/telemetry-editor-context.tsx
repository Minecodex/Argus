import { createContext, useContext, type ReactNode } from "react";

export type TelemetryCompletionRequest = {
  kind: "metrics" | "fields" | "values";
  metric?: string;
  field?: string;
  search?: string;
  filters?: {
    field: string;
    operator?: "=" | "!=" | ">" | ">=" | "<" | "<=" | "=~" | "!~" | ":";
    values: string[];
  }[];
};
export type TelemetryCompletionPage = {
  metrics: {
    name: string;
    type: string;
    unit: string;
    description?: string;
    labels: string[];
  }[];
  fields: { name: string; type: string }[];
  values: string[];
  has_more: boolean;
};
export type TelemetryEditorEnvironment = {
  key: string;
  load?: (
    input: TelemetryCompletionRequest,
    signal: AbortSignal,
  ) => Promise<TelemetryCompletionPage>;
  onRun?: () => void;
};
const Context = createContext<TelemetryEditorEnvironment>({ key: "offline" });
export const useTelemetryEditorEnvironment = () => useContext(Context);
export function TelemetryEditorProvider({
  value,
  children,
}: {
  value: TelemetryEditorEnvironment;
  children: ReactNode;
}) {
  return <Context.Provider value={value}>{children}</Context.Provider>;
}
