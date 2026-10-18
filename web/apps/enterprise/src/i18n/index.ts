import { hostInventoryZh, hostInventoryEn } from "./host-inventory";
import { resourceListsZh, resourceListsEn } from "./resource-lists";
import { alertsZh, alertsEn } from "./alerts";
import { dashboardsZh, dashboardsEn } from "./dashboards";
import { dashboardPresetsZh, dashboardPresetsEn } from "./dashboard-presets";
import { dashboardControlsZh, dashboardControlsEn } from "./dashboard-controls";
import { dashboardLinksZh, dashboardLinksEn } from "./dashboard-links";
import { planv5Zh, planv5En } from "./planv5";
import { plan7Zh, plan7En } from "./plan7";
import { chatInteractionsZh, chatInteractionsEn } from "./chat-interactions";
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { aiSettingsEn, aiSettingsZh } from "./ai-settings";
import { accountEn, accountZh } from "./account";
import { chatEn, chatZh } from "./chat";
import { toolNamesEn, toolNamesZh } from "./tool-names";
import { commonEn, commonZh } from "./common";
import { connectionChecksEn, connectionChecksZh } from "./connection-checks";
import { hostsEn, hostsZh } from "./hosts";
import { resourceNamesEn, resourceNamesZh } from "./resource-names";
import { hostRemovalEn, hostRemovalZh } from "./host-removal";
import { kubernetesEn, kubernetesZh } from "./kubernetes";
import { loginEn, loginZh } from "./login";
import { pendingActionsEn, pendingActionsZh } from "./pending-actions";
import { remoteAccessEn, remoteAccessZh } from "./remote-access";
import { remoteSessionsEn, remoteSessionsZh } from "./remote-sessions";
import { governanceEn, governanceZh } from "./governance";
import { settingsEn, settingsZh } from "./settings";
import { shellEn, shellZh } from "./shell";
import { telemetryEn, telemetryZh } from "./telemetry";
import { collectionUiEn, collectionUiZh } from "./collection-ui";
import { errorsEn, errorsZh } from "./errors";

/**
 * i18n 模块化注册模式：
 * - 每个业务模块一个 `i18n/<module>.ts`，导出 `<module>Zh` / `<module>En`
 *   两个资源对象，顶层命名空间与模块同名（如 `login.*`）。
 * - 新模块在这里的 `modulesZh` / `modulesEn` 清单中各加一行即可生效，
 *   资源对象在这里浅合并后注入 i18next。
 * - 通用/跨模块文案放 `common.ts`，外壳与导航文案放 `shell.ts`。
 */
const modulesZh = [
  chatInteractionsZh,
  plan7Zh,
  hostInventoryZh,
  resourceListsZh,
  alertsZh,
  dashboardsZh,
  dashboardPresetsZh,
  dashboardControlsZh,
  dashboardLinksZh,
  planv5Zh,
  connectionChecksZh,
  commonZh,
  accountZh,
  shellZh,
  loginZh,
  chatZh,
  toolNamesZh,
  hostsZh,
  resourceNamesZh,
  hostRemovalZh,
  kubernetesZh,
  aiSettingsZh,
  remoteAccessZh,
  remoteSessionsZh,
  telemetryZh,
  collectionUiZh,
  settingsZh,
  governanceZh,
  pendingActionsZh,
  errorsZh,
];
const modulesEn = [
  chatInteractionsEn,
  plan7En,
  hostInventoryEn,
  resourceListsEn,
  alertsEn,
  dashboardsEn,
  dashboardPresetsEn,
  dashboardControlsEn,
  dashboardLinksEn,
  planv5En,
  connectionChecksEn,
  commonEn,
  accountEn,
  shellEn,
  loginEn,
  chatEn,
  toolNamesEn,
  hostsEn,
  resourceNamesEn,
  hostRemovalEn,
  kubernetesEn,
  aiSettingsEn,
  remoteAccessEn,
  remoteSessionsEn,
  telemetryEn,
  collectionUiEn,
  settingsEn,
  governanceEn,
  pendingActionsEn,
  errorsEn,
];

const storedLocale = window.localStorage.getItem("argus.locale");

void i18n.use(initReactI18next).init({
  lng: storedLocale === "en-US" ? "en-US" : "zh-CN",
  fallbackLng: "zh-CN",
  supportedLngs: ["zh-CN", "en-US"],
  interpolation: { escapeValue: false },
  resources: {
    "zh-CN": { translation: Object.assign({}, ...modulesZh) },
    "en-US": { translation: Object.assign({}, ...modulesEn) },
  },
});

export default i18n;
