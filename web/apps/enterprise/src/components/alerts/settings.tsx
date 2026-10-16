import {useState} from "react";
import {Plus,Send,Mail,Search} from "lucide-react";
import type {AlertConfiguration,AlertKind,AlertMutation,AlertSchemas} from "@argus/api-client";
import {ActionMenu,Button,Input,SegmentedControl} from "@argus/ui";
import {useAlertText,AlertStatus,AlertCallout,AlertTable} from "./common";
export function AlertSettings({
  tab,
  configurations,
  names,
  canNoise,
  canNotify,
  onEdit,
  onPrepare,
  onTest,
  query = "",
  onQuery,
}: {
  tab: string;
  configurations: AlertConfiguration[];
  names: Map<string, string>;
  canNoise: boolean;
  canNotify: boolean;
  onEdit: (kind: AlertKind, row?: AlertConfiguration) => void;
  onPrepare: (m: AlertMutation) => Promise<void>;
  onTest: (
    op: "notification.test" | "notification.resend",
    id: string,
    version: number,
  ) => Promise<void>;
  query?: string;
  onQuery: (query: string) => void;
}) {
  const { t, locale } = useAlertText();
  const [noise, setNoise] = useState("silences"),
    [channelTab, setChannelTab] = useState("contactPoints");
  const rows = (kind: AlertKind) =>
    configurations.filter(
      (c) =>
        c.kind === kind &&
        (!query ||
          JSON.stringify(c).toLowerCase().includes(query.toLowerCase())),
    );
  const channels = rows("channel");
  const search = (
    <div className="argus-alert-search">
      <Search size={16} className="argus-alert-icon" />
      <Input
        size="lg"
        density="compact"
        aria-label={t("alerts.search")}
        placeholder={t("alerts.search")}
        value={query}
        onChange={(e) => onQuery(e.target.value)}
      />
    </div>
  );
  const date = (value: string) =>
    new Intl.DateTimeFormat(locale.startsWith("zh") ? "zh-CN" : "en-GB", {
      timeZone: "Asia/Shanghai",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(new Date(value));
  const change = (c: AlertConfiguration, op: AlertMutation["operation"]) =>
    void onPrepare({
      kind: c.kind,
      operation: op,
      id: c.id,
      expected_version: c.version,
    });
  const tabs = (
    values: string[],
    value: string,
    onValueChange: (value: string) => void,
  ) => (
    <div className="argus-alert-subtabs">
      <SegmentedControl
        density="compact"
        label={t(`alerts.${tab}`)}
        value={value}
        onChange={onValueChange}
        options={values.map((value) => ({
          value,
          label: t(`alerts.${value}`),
        }))}
      />
    </div>
  );
  if (tab === "noise") {
    const kind = noise === "silences" ? "silence" : "inhibition";
    return (
      <>
        {tabs(["silences", "inhibitions"], noise, setNoise)}
        <AlertCallout>{t("alerts.noiseHint")}</AlertCallout>
        <div className="argus-alert-toolbar">
          {search}
          <Button
            density="compact"
            size="lg"
            variant="primary"
            isDisabled={!canNoise}
            onPress={() => onEdit(kind)}
          >
            <Plus size={16} />
            {t(
              kind === "silence"
                ? "alerts.createSilence"
                : "alerts.createInhibition",
            )}
          </Button>
        </div>
        <AlertTable
          data={rows(kind)}
          columns={
            kind === "silence"
              ? [
                  {
                    key: "name",
                    header: t("alerts.name"),
                    render: (c) => (
                      <>
                        <b>{c.name}</b>
                        <div className="argus-alert-secondary">
                          {(c.spec as AlertSchemas["AlertSilenceSpec"]).reason}
                        </div>
                      </>
                    ),
                  },
                  {
                    key: "match",
                    header: t("alerts.scope"),
                    render: (c) => {
                      const s = c.spec as AlertSchemas["AlertSilenceSpec"];
                      return (
                        <div>
                          <span>
                            {(s.resource_ids ?? [])
                              .map((id) => names.get(id) ?? id)
                              .join(", ") || t("alerts.allHosts")}
                          </span>
                          <div className="argus-alert-secondary">
                            {s.metric ||
                              (s.policy_ids ?? [])
                                .map(
                                  (id) =>
                                    configurations.find((c) => c.id === id)
                                      ?.name ?? id,
                                )
                                .join(", ")}
                          </div>
                        </div>
                      );
                    },
                  },
                  {
                    key: "status",
                    header: t("alerts.status"),
                    render: (c) => (
                      <AlertStatus
                        tone="info"
                        value={
                          (c.spec as AlertSchemas["AlertSilenceSpec"]).enabled
                            ? "enabled"
                            : "disabled"
                        }
                      />
                    ),
                  },
                  {
                    key: "until",
                    header: t("alerts.until"),
                    render: (c) =>
                      date(
                        (c.spec as AlertSchemas["AlertSilenceSpec"]).ends_at,
                      ),
                  },
                  {
                    key: "hits",
                    header: t("alerts.matches"),
                    render: (c) => (
                      <span className="argus-alert-count">
                        {c.matched_instances ?? 0}
                      </span>
                    ),
                  },
                  {
                    key: "action",
                    header: t("alerts.action"),
                    render: (c) => (<ActionMenu items={[{label: t("alerts.edit"),onSelect: () => onEdit(kind, c),disabled: (!canNoise)},{label: t("alerts.revoke"),onSelect: () => change(c, "disable"),disabled: (!canNoise)}]} />),
                  },
                ]
              : [
                  {
                    key: "name",
                    header: t("alerts.name"),
                    render: (c) => (
                      <>
                        <b>{c.name}</b>
                        <div className="argus-alert-secondary">
                          {
                            (c.spec as AlertSchemas["AlertInhibitionSpec"])
                              .reason
                          }
                        </div>
                      </>
                    ),
                  },
                  {
                    key: "source",
                    header: t("alerts.source"),
                    render: (c) => (
                      <code>
                        {(c.spec as AlertSchemas["AlertInhibitionSpec"]).source
                          .map((m) => `${m.key}${m.operator}${m.value}`)
                          .join(", ")}
                      </code>
                    ),
                  },
                  {
                    key: "target",
                    header: t("alerts.target"),
                    render: (c) => (
                      <code>
                        {(c.spec as AlertSchemas["AlertInhibitionSpec"]).target
                          .map((m) => `${m.key}${m.operator}${m.value}`)
                          .join(", ")}
                      </code>
                    ),
                  },
                  {
                    key: "equal",
                    header: t("alerts.equal"),
                    render: (c) => (
                      <code>
                        {(
                          c.spec as AlertSchemas["AlertInhibitionSpec"]
                        ).equal.join(", ")}
                      </code>
                    ),
                  },
                  {
                    key: "status",
                    header: t("alerts.status"),
                    render: (c) => (
                      <AlertStatus
                        value={
                          (c.spec as AlertSchemas["AlertInhibitionSpec"])
                            .enabled
                            ? "enabled"
                            : "disabled"
                        }
                      />
                    ),
                  },
                  {
                    key: "action",
                    header: t("alerts.action"),
                    render: (c) => (<ActionMenu items={[{label: t("alerts.edit"),onSelect: () => onEdit(kind, c),disabled: (!canNoise)},{label: t(
                            (c.spec as AlertSchemas["AlertInhibitionSpec"])
                              .enabled
                              ? "alerts.pause"
                              : "alerts.resume",
                          ),onSelect: () =>
                            change(
                              c,
                              (c.spec as AlertSchemas["AlertInhibitionSpec"])
                                .enabled
                                ? "disable"
                                : "enable",
                            ),disabled: (!canNoise)}]} />),
                  },
                ]
          }
        />
      </>
    );
  }
  if (channelTab === "contactPoints")
    return (
      <>
        {tabs(
          ["contactPoints", "routes", "templates"],
          channelTab,
          setChannelTab,
        )}
        <div className="argus-alert-toolbar">
          {search}
          <Button
            density="compact"
            variant="primary"
            size="lg"
            isDisabled={!canNotify}
            onPress={() => onEdit("channel")}
          >
            <Plus size={16} />
            {t("alerts.createChannel")}
          </Button>
        </div>
        <div className="argus-alert-grid">
          {channels.map((c) => {
            const s = c.spec as AlertSchemas["AlertChannelSpec"];
            return (
              <section key={c.id} className="argus-alert-panel">
                <div className="argus-alert-panel-head">
                  <div className="argus-alert-channel-head">
                    <span className="argus-alert-channel-icon">
                      {s.provider === "smtp" ? (
                        <Mail size={20} />
                      ) : (
                        t(`alerts.${s.provider}`).slice(0, 1)
                      )}
                    </span>
                    <div>
                      <h3>{c.name}</h3>
                      <p className="argus-alert-secondary">
                        {t(`alerts.${s.provider}`)}
                      </p>
                    </div>
                  </div>
                  <AlertStatus value={s.enabled ? "enabled" : "disabled"} />
                </div>
                <div className="argus-alert-code-block">
                  {s.display_address ?? "—"}
                </div>
                <div className="argus-alert-channel-meta">
                  <span>
                    {t("alerts.credential")} · {s.credential_id}
                  </span>
                  <span>
                    {
                      rows("route").filter((r) =>
                        (
                          r.spec as AlertSchemas["AlertRouteSpec"]
                        ).channel_ids.includes(c.id),
                      ).length
                    }{" "}
                    {t("alerts.routeCount")}
                  </span>
                </div>
                <div className="argus-alert-actions argus-alert-actions--end">
                  <Button
                    density="compact"
                    size="lg"
                    isDisabled={!canNotify}
                    onPress={() => onEdit("channel", c)}
                  >
                    {t("alerts.configure")}
                  </Button>
                  <Button
                    density="compact"
                    size="lg"
                    isDisabled={!canNotify || !s.enabled}
                    onPress={() =>
                      void onTest("notification.test", c.id, c.version)
                    }
                  >
                    <Send size={16} />
                    {t("alerts.test")}
                  </Button>
                </div>
              </section>
            );
          })}
        </div>
        {!channels.length && (
          <AlertCallout>{t("alerts.emptyChannel")}</AlertCallout>
        )}
      </>
    );
  if (channelTab === "templates") {
    const row = rows("template")[0];
    const content =
      (row?.spec as AlertSchemas["AlertTemplateSpec"] | undefined)?.content ??
      "";
    const sample = {
      severity: t("alerts.critical"),
      policy: rows("policy")[0]?.name ?? "Host CPU",
      resource: "web-prod-01",
      metric: "host_cpu_usage_percent",
      value: "94.7%",
      threshold: "> 85%",
      duration: t("alerts.minuteValue", { n: 12 }),
      detail_url: location.origin + "/alerts",
    };
    return (
      <>
        {tabs(
          ["contactPoints", "routes", "templates"],
          channelTab,
          setChannelTab,
        )}
        <section className="argus-alert-panel">
          <div className="argus-alert-panel-head">
            <h3>{t("alerts.templates")}</h3>
            <Button
              density="compact"
              isDisabled={!canNotify}
              onPress={() => onEdit("template", row)}
            >
              {t("alerts.edit")}
            </Button>
          </div>
          <div className="argus-alert-grid">
            <div>
              <h3>{t("alerts.template")}</h3>
              <pre className="argus-alert-code-block">{content}</pre>
            </div>
            <div>
              <h3>{t("alerts.previewContent")}</h3>
              <pre className="argus-alert-code-block">
                {content.replace(
                  /\{\{\.?([a-z_]+)\}\}/g,
                  (raw, key: string) =>
                    sample[key as keyof typeof sample] ?? raw,
                )}
              </pre>
            </div>
          </div>
        </section>
      </>
    );
  }
  const grouping = rows("grouping")[0];
  const group = grouping?.spec as AlertSchemas["AlertGroupingSpec"] | undefined;
  return (
    <>
      {tabs(
        ["contactPoints", "routes", "templates"],
        channelTab,
        setChannelTab,
      )}
      <AlertCallout>{t("alerts.routeHint")}</AlertCallout>
      <div className="argus-alert-toolbar">
        <span />
        <Button
          density="compact"
          variant="primary"
          size="lg"
          isDisabled={!canNotify}
          onPress={() => onEdit("route")}
        >
          <Plus size={16} />
          {t("alerts.createRoute")}
        </Button>
      </div>
      <AlertTable
        data={rows("route")}
        columns={[
          {
            key: "name",
            header: t("alerts.name"),
            render: (c) => <b>{c.name}</b>,
          },
          {
            key: "match",
            header: t("alerts.match"),
            render: (c) => (
              <code>
                {(c.spec as AlertSchemas["AlertRouteSpec"]).matchers
                  .map((m) => `${m.key}${m.operator}${m.value}`)
                  .join(", ")}
              </code>
            ),
          },
          {
            key: "destinations",
            header: t("alerts.destinations"),
            render: (c) => (
              <div className="argus-alert-labels">
                {(c.spec as AlertSchemas["AlertRouteSpec"]).channel_ids.map(
                  (id) => (
                    <span key={id} className="argus-alert-tag">
                      {channels.find((c) => c.id === id)?.name ?? id}
                    </span>
                  ),
                )}
              </div>
            ),
          },
          {
            key: "action",
            header: t("alerts.action"),
            render: (c) => (<ActionMenu items={[{label: t("alerts.edit"),onSelect: () => onEdit("route", c),disabled: (!canNotify)}]} />),
          },
        ]}
      />
      <section className="argus-alert-panel argus-alert-drawer-section">
        <div className="argus-alert-panel-head">
          <h3>{t("alerts.grouping")}</h3>
          <Button
            density="compact"
            size="sm"
            isDisabled={!canNotify}
            onPress={() => onEdit("grouping", grouping)}
          >
            {t("alerts.configure")}
          </Button>
        </div>
        <div className="argus-alert-grid">
          <div>
            {t("alerts.groupBy")}
            <div className="argus-alert-secondary">
              <code>
                {group?.group_by.join(", ") ?? "policy_id, resource_id"}
              </code>
            </div>
          </div>
          <div>
            {t("alerts.groupWait")}: {group?.wait_seconds ?? 30}
            <div className="argus-alert-secondary">
              {t("alerts.groupInterval")}: {group?.interval_seconds ?? 300} ·{" "}
              {t("alerts.repeatInterval")}: {group?.repeat_seconds ?? 3600}
            </div>
          </div>
        </div>
      </section>
    </>
  );
}
