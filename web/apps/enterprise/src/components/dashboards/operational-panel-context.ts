import { createContext } from "react";
export const OperationalPanelContext = createContext<{
  time?: import("./url-time-range").DashboardTimeSearch;
  namespaces?: string[];
  cluster?: string;
  focus?: (cluster: string, panel: string, uid: string) => void;
}>({});
