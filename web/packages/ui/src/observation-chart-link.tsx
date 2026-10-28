import { createContext, useContext, type ReactNode } from "react";
const Context = createContext<string | undefined>(undefined);
export const useObservationChartLink = () => useContext(Context);
export function ObservationChartLink({
  group,
  children,
}: {
  group?: string;
  children: ReactNode;
}) {
  return <Context.Provider value={group}>{children}</Context.Provider>;
}
