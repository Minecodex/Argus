/* Interactive UX proposal. All data and validation states are simulated. */
(() => {
  "use strict";
  const KEY = "argus.planv2.editor-ux-demo.v1";
  const app = document.getElementById("app");
  const modal = document.getElementById("modal");
  const toastEl = document.getElementById("toast");
  const copy = (v) => structuredClone(v);
  const esc = (v) =>
    String(v ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  const T = (zh, en) => (state.lang === "en" ? en : zh);
  const icons = {
    grid: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    line: '<path d="M3 18h18M4 14l5-6 5 4 6-8"/>',
    stat: '<path d="M7 5h6v14M7 19h12M4 12h3"/>',
    bar: '<path d="M4 20V12M10 20V5M16 20V9M22 20H2"/>',
    pie: '<path d="M12 3v9h9A9 9 0 0 0 12 3Z"/><path d="M9 4a9 9 0 1 0 11 11H9Z"/>',
    gauge:
      '<path d="M4 18a9 9 0 1 1 16 0M12 12l5-5"/><circle cx="12" cy="12" r="1"/>',
    logs: '<path d="M4 5h16M4 10h16M4 15h11M4 20h13"/>',
    trace: '<path d="M4 5h15M7 10h10M10 15h9M10 20h5"/>',
    table:
      '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M10 4v16M3 15h18"/>',
    plus: '<path d="M12 4v16M4 12h16"/>',
    back: '<path d="m13 5-7 7 7 7M6 12h14"/>',
    edit: '<path d="m15 4 5 5-10 10-6 1 1-6Z"/>',
    close: '<path d="m5 5 14 14M19 5 5 19"/>',
    check: '<path d="m4 12 5 5L20 6"/>',
    play: '<path d="m7 4 13 8-13 8Z"/>',
    filter: '<path d="M3 4h18l-7 8v7l-4 2v-9Z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1 1M18 18l1 1M5 19l1-1M18 6l1-1"/>',
    move: '<path d="M12 3v18M3 12h18m-13-9 4-3 4 3m-8 12 4 3 4-3M6 8l-3 4 3 4m12-8 3 4-3 4"/>',
    alert: '<path d="m12 3 10 18H2Z"/><path d="M12 9v5M12 17v1"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7v1"/>',
    copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V4H4v12h4"/>',
    resize: '<path d="m8 20 12-12m-6 12 6-6"/>',
  };
  const icon = (name) =>
    `<svg class="argus-icon" viewBox="0 0 24 24" aria-hidden="true">${icons[name] || icons.line}</svg>`;
  const btn = (action, label, kind = "", extra = "", symbol = "") =>
    `<button type="button" class="argus-btn ${kind}" data-action="${action}" ${extra}>${symbol ? icon(symbol) : ""}${label}</button>`;
  const sourceNames = {
    hostmetrics: ["主机指标", "Host metrics"],
    prometheus: ["Prometheus", "Prometheus"],
    otlp: ["OTLP", "OTLP"],
    skywalking: ["SkyWalking", "SkyWalking"],
    jaeger: ["Jaeger", "Jaeger"],
    filelog: ["文件日志", "File logs"],
  };
  const sourceName = (id) => T(...(sourceNames[id] || [id, id]));
  const signalName = (id) =>
    ({
      metrics: T("指标", "Metrics"),
      logs: T("日志", "Logs"),
      traces: T("链路 / APM", "Traces / APM"),
    })[id];
  const charts = {
    timeseries: ["趋势图", "Time series", "line"],
    stat: ["统计值", "Number", "stat"],
    gauge: ["仪表图", "Gauge", "gauge"],
    bar_gauge: ["条形仪表", "Bar gauge", "bar"],
    bar: ["柱状图", "Bar", "bar"],
    pie: ["饼图", "Pie", "pie"],
    histogram: ["直方图", "Histogram", "bar"],
    heatmap: ["热力图", "Heatmap", "grid"],
    state_timeline: ["状态时间线", "State timeline", "trace"],
    scatter: ["散点图", "Scatter", "line"],
    table: ["表格", "Table", "table"],
    logs: ["日志列表", "Log list", "logs"],
    trace_list: ["链路列表", "Trace list", "trace"],
    apm_services: ["服务概览", "Services", "grid"],
    apm_red: ["RED 概览", "RED overview", "line"],
    apm_topology: ["服务拓扑", "Topology", "grid"],
  };
  const chartName = (id) => T(...(charts[id] || [id, id]));
  const metrics = [
    {
      id: "system_cpu_utilization",
      zh: "CPU 使用率",
      en: "CPU utilization",
      unit: "percent_ratio",
      type: "gauge",
    },
    {
      id: "system_memory_utilization",
      zh: "内存使用率",
      en: "Memory utilization",
      unit: "percent_ratio",
      type: "gauge",
    },
    {
      id: "system_memory_usage",
      zh: "已使用内存",
      en: "Memory used",
      unit: "bytes",
      type: "gauge",
    },
    {
      id: "http_requests_total",
      zh: "HTTP 请求总数",
      en: "HTTP request count",
      unit: "requests",
      type: "counter",
    },
    {
      id: "http_request_duration_seconds_bucket",
      zh: "请求时延直方图桶",
      en: "Request duration buckets",
      unit: "s",
      type: "histogram",
    },
  ];
  const metricInfo = (p) =>
    metrics.find((m) => m.id === p.metric) || metrics[0];
  const metricLabel = (m) => T(m.zh, m.en);
  const presets = [
    {
      id: "cpu",
      signal: "metrics",
      title: ["CPU 使用率", "CPU utilization"],
      description: ["按主机比较 CPU 趋势", "Compare CPU trends across hosts"],
      source: "hostmetrics",
      chart: "timeseries",
      metric: "system_cpu_utilization",
      operation: "avg",
      group: "host_name",
    },
    {
      id: "memory",
      signal: "metrics",
      title: ["内存使用率", "Memory utilization"],
      description: ["查看当前内存占用", "See current memory utilization"],
      source: "hostmetrics",
      chart: "stat",
      metric: "system_memory_utilization",
      operation: "avg",
      group: "",
    },
    {
      id: "error",
      signal: "logs",
      title: ["错误日志趋势", "Error log trend"],
      description: ["按分钟统计 ERROR 日志", "Count ERROR logs by minute"],
      source: "otlp",
      chart: "timeseries",
      operation: "count_over_time",
      filters: [{ field: "severity", value: "ERROR" }],
    },
    {
      id: "logs",
      signal: "logs",
      title: ["最近日志", "Recent logs"],
      description: ["检索并展开日志上下文", "Search and open log context"],
      source: "otlp",
      chart: "logs",
      operation: "records",
    },
    {
      id: "traces",
      signal: "traces",
      title: ["慢链路", "Slow traces"],
      description: ["找到耗时较长的已接收链路", "Find slow received traces"],
      source: "otlp",
      chart: "trace_list",
      operation: "list",
    },
    {
      id: "services",
      signal: "traces",
      title: ["服务概览", "Service overview"],
      description: [
        "查看已接收样本的服务统计",
        "Inspect service statistics for received samples",
      ],
      source: "otlp",
      chart: "apm_services",
      operation: "apm_services",
    },
  ];
  const makePanel = (preset) => ({
    ...copy(preset),
    origin: preset.id,
    id: crypto.randomUUID(),
    title: T(...preset.title),
    customTitle: false,
    mode: "builder",
    expression: "",
    filters: copy(preset.filters || []),
    usesEnv: false,
    legend: true,
    unit: "auto",
    decimals: 1,
    reducer: "last",
    draw: "line",
    smooth: false,
    threshold: "",
    drilldown: true,
    w: 6,
    h: 340,
    status: "success",
    step: "auto",
    bucket: 60,
    limit: 100,
  });
  function initialState() {
    const panels = [presets[0], presets[2], presets[1], presets[4]].map(
      makePanel,
    );
    panels[0].usesEnv = true;
    panels[2].w = 4;
    panels[3].w = 8;
    const board = {
      name: "生产环境概览",
      description: "",
      panels,
      variable: {
        enabled: true,
        label: "环境",
        multiple: false,
        field: "deployment_environment",
      },
      defaultRange: "1h",
    };
    return {
      version: 1,
      lang: "zh",
      theme: "light",
      published: copy(board),
      draft: copy(board),
      revision: 4,
      base: 4,
      view: "draft",
      scope: { range: "1h", resource: "all", env: "production" },
      editor: null,
      dialog: null,
      tab: "chart",
      resultView: "chart",
      advanced: false,
      moreCharts: false,
      addSignal: "all",
      catalogSearch: "",
      filterField: "severity",
      filterValue: "ERROR",
      filterVariable: false,
      conflictChoice: "latest",
    };
  }
  let state;
  try {
    const saved = JSON.parse(localStorage.getItem(KEY));
    state = saved?.version === 1 ? saved : null;
  } catch {}
  if (!state) {
    state = { lang: "zh" };
    state = initialState();
  }
  state.dialog = null;
  let toastTimer,
    saveTimer,
    dragId = null,
    gesture = null;
  function persist() {
    try {
      localStorage.setItem(KEY, JSON.stringify({ ...state, dialog: null }));
    } catch {}
  }
  function toast(message) {
    toastEl.textContent = message;
    toastEl.dataset.visible = "true";
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toastEl.dataset.visible = "false"), 2600);
  }
  function board() {
    return state.view === "published" ? state.published : state.draft;
  }
  function environmentLabel() {
    const value = board().variable.label;
    return value === "环境" ? T("环境", "Environment") : value;
  }
  function normalizeScope() {
    if (!board().variable.multiple && Array.isArray(state.scope.env))
      state.scope.env =
        state.scope.env.length === 1 ? state.scope.env[0] : "all";
  }
  function dirty() {
    return JSON.stringify(state.draft) !== JSON.stringify(state.published);
  }
  function title(p) {
    const preset = presets.find((x) => x.id === p.preset || x.id === p.origin);
    return !p.customTitle && preset ? T(...preset.title) : p.title;
  }
  function startEditor(p, fresh = false) {
    const panel = copy(p);
    panel.origin =
      panel.origin ||
      presets.find(
        (x) =>
          x.metric === panel.metric &&
          x.chart === panel.chart &&
          x.signal === panel.signal,
      )?.id;
    state.editor = {
      panel,
      fresh,
      initial: copy(panel),
      last: copy(panel),
      queryDirty: false,
    };
    state.tab = "chart";
    state.moreCharts = false;
    state.advanced = false;
    state.openDetails = {};
    state.resultView = "chart";
    state.dialog = null;
    render();
    persist();
  }
  const panel = () => state.editor?.panel;
  function field(label, content) {
    return `<label class="argus-field"><span>${label}</span>${content}</label>`;
  }
  function select(id, value, options, attributes = "") {
    return `<select id="${id}" data-field="${id}" ${attributes}>${options.map(([v, label]) => `<option value="${esc(v)}" ${String(value) === String(v) ? "selected" : ""}>${esc(label)}</option>`).join("")}</select>`;
  }
  function input(id, value, type = "text", attrs = "") {
    return `<input class="argus-input" id="${id}" data-field="${id}" type="${type}" value="${esc(value)}" ${attrs}>`;
  }
  function query(p) {
    if (p.mode === "code") return p.expression;
    const filter = p.filters.map((f) => `${f.field}="${f.value}"`);
    if (p.usesEnv) filter.push('deployment_environment="$env"');
    if (p.signal === "metrics") {
      const raw = p.metric + (filter.length ? `{${filter.join(",")}}` : "");
      if (p.operation === "rate") return `rate(${raw}[5m])`;
      if (p.operation === "value") return raw;
      return `${p.operation || "avg"}${p.group ? ` by (${p.group})` : ""} (${raw})`;
    }
    if (p.signal === "logs") {
      const filters = filter.length ? filter.join(" AND ") : "*";
      if (p.operation === "records") return `${filters} | limit ${p.limit}`;
      if (p.operation === "count") return `${filters} | stats count()`;
      return `${filters} | stats count() by bin(timestamp, ${p.bucket}s)`;
    }
    const args = [];
    if (p.service && p.service !== "all")
      args.push(`serviceName:${JSON.stringify(p.service)}`);
    for (const f of p.filters)
      if (["service_name", "status"].includes(f.field))
        args.push(
          `${f.field === "service_name" ? "serviceName" : "status"}:${JSON.stringify(f.value)}`,
        );
    if (p.usesEnv)
      args.push(
        'filters:[{field:"resource_attributes.deployment.environment.name",values:$env}]',
      );
    const declaration = p.usesEnv ? "query($env:[String!])" : "query";
    const root =
      p.operation === "apm_services" ? "queryAPMServices" : "queryTraces";
    args.push(
      p.operation === "apm_services"
        ? `limit:${p.limit}`
        : `pageSize:${p.limit}`,
    );
    const projection =
      p.operation === "apm_services"
        ? "basis rows {serviceName sampleCount errorRate durationMeanMs}"
        : "total traces {traceId rootService rootOperation duration status spanCount}";
    return `${declaration} { ${root}(${args.join(",")}) { ${projection} } }`;
  }
  function validation(p) {
    if (!p.title.trim()) return T("请填写统计图名称。", "Enter a panel title.");
    if (p.status === "invalid")
      return T(
        "示例：查询语法错误。修正后才能加入草稿。",
        "Example: invalid query syntax. Fix it before adding to the draft.",
      );
    if (p.mode === "code") {
      const text = p.expression.trim();
      if (!text) return T("查询语句不能为空。", "The query cannot be empty.");
      if (
        text.includes("INVALID") ||
        [...text].filter((x) => x === "(").length !==
          [...text].filter((x) => x === ")").length ||
        [...text].filter((x) => x === "{").length !==
          [...text].filter((x) => x === "}").length
      )
        return T(
          "示例格式检查未通过，请检查括号或语句。",
          "Example format check failed. Check brackets and query text.",
        );
    }
    return "";
  }
  function convertible(p) {
    const canonical = copy(p);
    canonical.mode = "builder";
    return p.expression.trim() === query(canonical);
  }
  function compatible(p) {
    if (p.signal === "metrics")
      return metricInfo(p).type === "histogram"
        ? ["histogram", "heatmap", "table"]
        : [
            "timeseries",
            "stat",
            "gauge",
            "bar_gauge",
            "bar",
            "pie",
            "state_timeline",
            "scatter",
            "table",
          ];
    if (p.signal === "logs")
      return p.operation === "records"
        ? ["logs", "table"]
        : p.operation === "count"
          ? ["stat", "bar", "table"]
          : ["timeseries", "bar", "table"];
    return p.operation === "apm_services" ? ["apm_services"] : ["trace_list"];
  }
  function panelView(p) {
    const e = state.editor;
    if (e && e.queryDirty)
      return {
        ...copy(e.last),
        chart: p.chart,
        legend: p.legend,
        draw: p.draw,
        unit: p.unit,
        decimals: p.decimals,
        reducer: p.reducer,
        threshold: p.threshold,
      };
    return p;
  }
  function samples(p) {
    const scopeFactor = state.scope.resource === "host-a" ? 0.82 : 1;
    const environments = Array.isArray(state.scope.env)
      ? state.scope.env
      : [state.scope.env];
    const envFactor = p.usesEnv
      ? environments.length > 1
        ? 0.86
        : environments[0] === "staging"
          ? 0.72
          : 1
      : 1;
    const level = p.filters.find((f) => f.field === "severity")?.value;
    const operationFactor =
      ({ max: 1.25, min: 0.7, sum: 1.7 }[p.operation] || 1) *
      (p.signal === "logs"
        ? level === "ERROR"
          ? 0.55
          : level === "WARN"
            ? 0.8
            : 1
        : 1);
    return Array.from(
      { length: 32 },
      (_, i) =>
        (p.signal === "logs"
          ? Math.max(2, 18 + 13 * Math.sin(i * 0.43) + ((i * 7) % 13))
          : p.metric === "system_memory_usage"
            ? 8.7 + 1.1 * Math.sin(i * 0.2)
            : p.metric === "system_memory_utilization"
              ? 63 + 8 * Math.sin(i * 0.23)
              : p.metric === "http_requests_total"
                ? 150 + 50 * Math.sin(i * 0.24)
                : 35 + 12 * Math.sin(i * 0.3) + 7 * Math.cos(i * 0.61)) *
        scopeFactor *
        envFactor *
        operationFactor,
    );
  }
  function values(p) {
    const v = samples(p);
    return p.reducer === "mean"
      ? v.reduce((a, b) => a + b, 0) / v.length
      : p.reducer === "max"
        ? Math.max(...v)
        : p.reducer === "min"
          ? Math.min(...v)
          : v.at(-1);
  }
  function suffix(p) {
    return p.unit !== "auto"
      ? { percent: "%", bytes: " GiB", ms: " ms", s: " s", requests: "" }[
          p.unit
        ] || ""
      : p.signal === "metrics"
        ? {
            percent_ratio: "%",
            bytes: " GiB",
            s: " s",
            requests: p.operation === "rate" ? " /s" : "",
          }[metricInfo(p).unit] || ""
        : "";
  }
  function formatted(p) {
    return values(p).toFixed(p.decimals) + suffix(p);
  }
  function seriesNames(p) {
    if (!p.group) return [T("当前查询", "Current query")];
    if (p.group === "service_name") return ["checkout-api", "payment-api"];
    const host = p.filters.find((f) => f.field === "host_name")?.value;
    return host
      ? [host]
      : state.scope.resource === "host-a"
        ? ["host-a"]
        : ["host-a", "host-b"];
  }
  function plot(p, compact = false) {
    const series = seriesNames(p);
    if (state.scope.resource === "cluster-b" && p.source === "hostmetrics")
      return `<div class="argus-empty">${icon("info")}<span>${T("此图适用于主机，当前选择的是集群。", "This panel applies to hosts; a cluster is selected.")}</span></div>`;
    if (p.status === "empty")
      return `<div class="argus-empty">${icon("line")}<strong>${T("当前范围没有数据", "No data in this range")}</strong><small>${T("配置可保留，样本数据尚未验证。", "Configuration can be kept; sample data remains unverified.")}</small></div>`;
    if (p.status === "unavailable")
      return `<div class="argus-empty">${icon("alert")}<strong>${T("样本查询暂时不可用", "Sample query temporarily unavailable")}</strong><small>${T("保留上次配置；发布预览会显示警告。", "Keep the configuration; publication preview will show a warning.")}</small></div>`;
    if (p.status === "invalid")
      return `<div class="argus-empty">${icon("alert")}<strong>${T("查询需要修正", "Query needs correction")}</strong></div>`;
    if (p.chart === "stat")
      return `<div class="argus-stats-list">${series.map((name, i) => `<div class="argus-stat"><strong>${(i ? values(p) * 0.73 + 7 : values(p)).toFixed(p.decimals)}${suffix(p)}</strong><span>${esc(name)} · ${T(p.reducer === "last" ? "最近值" : "时段计算", p.reducer === "last" ? "Last value" : "Range calculation")}</span></div>`).join("")}</div>`;
    if (p.chart === "logs")
      return `<div>${Array.from({ length: compact ? 5 : 6 }, (_, i) => `<div class="argus-log-row"><time>14:${56 - i}:0${i}</time><span class="argus-pill ${i % 3 === 0 ? "argus-pill--warn" : "argus-pill--info"}">${i % 3 === 0 ? "ERROR" : "INFO"}</span><button data-action="detail" data-kind="logs" ${p.drilldown ? "" : "disabled"}>${["checkout request completed · 200 · 42 ms", "payment upstream timeout · trace_id=8ef01…", "inventory reservation succeeded · 18 ms"][i % 3]}</button></div>`).join("")}</div>`;
    if (p.chart === "trace_list" || p.chart === "apm_services")
      return `<table class="argus-table"><thead><tr><th>${p.chart === "trace_list" ? "Trace ID" : T("服务", "Service")}</th><th>${T("耗时", "Duration")}</th><th>${T("状态", "Status")}</th><th>${p.chart === "trace_list" ? "Spans" : T("样本数", "Samples")}</th></tr></thead><tbody>${[
        "checkout-api",
        "payment-api",
        "inventory-api",
        "gateway",
      ]
        .map((s, i) => ({ s, i }))
        .filter(
          ({ s, i }) =>
            (!p.service || p.service === "all" || s === p.service) &&
            (!p.filters.some(
              (f) => f.field === "status" && f.value === "ERROR",
            ) ||
              i === 0),
        )
        .map(
          ({ s, i }) =>
            `<tr ${p.drilldown ? 'data-action="detail" data-kind="traces" tabindex="0"' : ""}><td>${p.chart === "trace_list" ? `<span class="argus-code">${["8ef014ac…", "da305b3e…", "9216c04b…", "640feeab…"][i]}</span>` : s}</td><td>${[1240, 680, 230, 85][i]} ms</td><td><span class="argus-pill ${i === 0 ? "argus-pill--warn" : "argus-pill--good"}">${i === 0 ? "ERROR" : "OK"}</span></td><td>${p.chart === "trace_list" ? [18, 11, 7, 5][i] : [2840, 2315, 1920, 3820][i]}</td></tr>`,
        )
        .join("")}</tbody></table>`;
    if (p.chart === "table")
      return `<table class="argus-table"><thead><tr><th>${T("分组 / 结果", "Group / result")}</th><th>${T("值", "Value")}</th><th>${T("来源", "Source")}</th></tr></thead><tbody>${series.map((h, i) => `<tr><td>${esc(h)}</td><td>${(i ? values(p) * 0.73 + 7 : values(p)).toFixed(p.decimals)}${suffix(p)}</td><td>${sourceName(p.source)}</td></tr>`).join("")}</tbody></table>`;
    const list = samples(p),
      max =
        p.signal === "logs" || p.chart === "histogram"
          ? 50
          : p.metric === "system_memory_usage"
            ? 16
            : p.metric === "http_requests_total"
              ? 250
              : 100;
    const x = (i) => 48 + (i * 652) / 31,
      y = (v) => 178 - (v / max) * 150,
      points = list
        .map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`)
        .join(" ");
    const grid = Array.from({ length: 5 }, (_, i) => {
      const value = (i * max) / 4;
      return `<line class="argus-plot-grid" x1="48" y1="${y(value)}" x2="710" y2="${y(value)}"/><text x="36" y="${y(value) + 4}" text-anchor="end">${Math.round(value)}${p.chart === "histogram" ? "" : suffix(p)}</text>`;
    }).join("");
    const ticks =
      state.scope.range === "15m"
        ? ["14:45", "14:50", "14:55", "15:00"]
        : state.scope.range === "6h"
          ? ["09:00", "11:00", "13:00", "15:00"]
          : ["14:00", "14:20", "14:40", "15:00"];
    let marks;
    if (p.chart === "gauge") {
      const v = Math.min(100, values(p));
      return `<svg class="argus-plot" viewBox="0 0 740 210" role="img" aria-label="${esc(title(p))}"><path d="M240 155a130 130 0 0 1 260 0" fill="none" stroke="var(--bg-muted)" stroke-width="20"/><path d="M240 155a130 130 0 0 1 260 0" fill="none" stroke="var(--info)" stroke-width="20" stroke-dasharray="${v * 4.08} 408"/><text x="370" y="148" text-anchor="middle" style="font-size:var(--font-size-34);fill:var(--text-primary)">${formatted(p)}</text><text x="240" y="185">0</text><text x="500" y="185">100</text></svg>`;
    }
    if (p.chart === "pie")
      return `<svg class="argus-plot" viewBox="0 0 740 210" role="img" aria-label="${esc(title(p))}"><circle cx="325" cy="105" r="72" fill="none" stroke="var(--brand-end)" stroke-width="30"/><circle cx="325" cy="105" r="72" fill="none" stroke="var(--info)" stroke-width="30" stroke-dasharray="280 452" transform="rotate(-90 325 105)"/><text x="440" y="90">host-a · 62%</text><text x="440" y="120">host-b · 38%</text></svg>`;
    if (p.chart === "bar_gauge")
      return `<div class="argus-stack" style="padding-top:var(--space-5)">${["host-a", "host-b", "host-c"].map((h, i) => `<div><div class="argus-between"><small>${h}</small><small>${(values(p) + i * 6).toFixed(1)}${suffix(p)}</small></div><div style="height:var(--space-3);background:var(--bg-muted);border-radius:var(--radius-xs);margin-top:var(--space-1)"><div style="width:${Math.min(100, values(p) + i * 6)}%;height:100%;background:var(--info);border-radius:var(--radius-xs)"></div></div></div>`).join("")}</div>`;
    if (p.chart === "heatmap")
      marks = Array.from({ length: 12 }, (_, i) =>
        Array.from(
          { length: 5 },
          (_, j) =>
            `<rect x="${50 + i * 53}" y="${26 + j * 30}" width="48" height="25" rx="2" fill="var(--info)" opacity="${0.14 + ((i * 3 + j * 7) % 10) / 13}"/>`,
        ).join(""),
      ).join("");
    else if (p.chart === "histogram")
      marks = [9, 24, 42, 28, 16, 7]
        .map(
          (v, i) =>
            `<rect x="${68 + i * 104}" y="${178 - v * 3}" width="76" height="${v * 3}" rx="3" fill="var(--info)"/><text x="${105 + i * 104}" y="201" text-anchor="middle">${["0–50", "50–100", "100–200", "200–500", "500–1k", "1k+"][i]} ms</text>`,
        )
        .join("");
    else if (p.chart === "state_timeline")
      marks = Array.from({ length: 3 }, (_, j) =>
        Array.from(
          { length: 16 },
          (_, i) =>
            `<rect x="${49 + i * 40}" y="${40 + j * 40}" width="39" height="26" rx="2" fill="var(--${(i + j) % 7 === 0 ? "warning" : "success"})" opacity=".7"/>`,
        ).join(""),
      ).join("");
    else if (p.chart === "bar")
      marks = list
        .filter((_, i) => i % 2 === 0)
        .map(
          (v, i) =>
            `<rect x="${50 + i * 41}" y="${y(v)}" width="24" height="${178 - y(v)}" rx="3" fill="var(--info)"/>`,
        )
        .join("");
    else if (p.chart === "scatter")
      marks = list
        .map(
          (v, i) =>
            `<circle cx="${x(i)}" cy="${y(v)}" r="4" fill="var(--info)"/>`,
        )
        .join("");
    else
      marks = `${p.draw === "area" ? `<polygon class="argus-plot-fill" points="48,178 ${points} 700,178"/>` : ""}<polyline class="argus-plot-line" points="${points}"/>${series.length > 1 ? `<polyline class="argus-plot-line argus-plot-line--alt" points="${list.map((v, i) => `${x(i)},${y(v * 0.73 + 7)}`).join(" ")}"/>` : ""}`;
    return `<div class="argus-plot-wrap"><svg class="argus-plot" viewBox="0 0 740 220" role="img" aria-label="${esc(title(p))}">${grid}${marks}${p.chart !== "histogram" ? ticks.map((v, i) => `<text x="${48 + i * 217}" y="202" text-anchor="${i === 0 ? "start" : i === 3 ? "end" : "middle"}">${v}</text>`).join("") : ""}</svg>${p.legend ? `<div class="argus-legend"><span><i class="argus-dot"></i>${esc(series[0])}</span>${series.length > 1 ? `<span><i class="argus-dot argus-dot--alt"></i>${esc(series[1])}</span>` : ""}</div>` : ""}</div>`;
  }
  function header(editor = false) {
    return `<header class="argus-appbar"><div class="argus-flex"><span class="argus-logo-mark">A</span><span class="argus-logo">Argus</span><span class="argus-nav-divider"></span><span class="argus-breadcrumb">${T("仪表盘 / 生产环境概览", "Dashboards / Production overview")}${editor ? ` / ${T("编辑统计图", "Edit panel")}` : ""}</span></div><div class="argus-flex"><span class="argus-pill argus-pill--info">${T("交互原型 · 示例数据", "UX prototype · sample data")}</span>${btn("scenario", T("演示场景", "Scenarios"), "argus-btn--ghost argus-btn--small")}${btn("language", state.lang === "zh" ? "EN" : "中文", "argus-btn--ghost argus-btn--small")}${btn("theme", state.theme === "light" ? T("深色", "Dark") : T("浅色", "Light"), "argus-btn--ghost argus-btn--small", "", "sun")}${btn("reset", T("重置", "Reset"), "argus-btn--ghost argus-btn--small")}</div></header>`;
  }
  function scopeToolbar() {
    return `<div class="argus-filter-strip"><label>${select(
      "scope-range",
      state.scope.range,
      [
        ["15m", T("最近 15 分钟", "Last 15 minutes")],
        ["1h", T("最近 1 小时", "Last hour")],
        ["6h", T("最近 6 小时", "Last 6 hours")],
      ],
      `class="argus-select" aria-label="${T("时间范围", "Time range")}"`,
    )}</label><label>${select(
      "scope-resource",
      state.scope.resource,
      [
        ["all", T("全部有权限资源", "All authorized resources")],
        ["host-a", "host-a"],
        ["cluster-b", "cluster-b"],
      ],
      `class="argus-select" aria-label="${T("资源范围", "Resource scope")}"`,
    )}</label>${
      board().variable.enabled && board().variable.multiple
        ? btn(
            "scope-env-open",
            esc(environmentLabel()) +
              ": " +
              esc(
                (Array.isArray(state.scope.env)
                  ? state.scope.env
                  : [state.scope.env]
                )
                  .map((v) => (v === "all" ? T("全部", "All") : v))
                  .join(", "),
              ),
          )
        : board().variable.enabled
          ? `<label>${select(
              "scope-env",
              state.scope.env,
              [
                ["all", environmentLabel() + ": " + T("全部", "All")],
                ["production", environmentLabel() + ": production"],
                ["staging", environmentLabel() + ": staging"],
              ],
              `class="argus-select" aria-label="${T("环境变量", "Environment variable")}"`,
            )}</label>`
          : ""
    }</div>`;
  }
  function renderBoard() {
    const b = board(),
      editing = state.view === "draft";
    return `${header()}<div class="argus-frame"><aside class="argus-nav"><div class="argus-nav-section">${T("工作台", "WORKSPACE")}</div><button class="argus-nav-item" aria-current="page" data-action="draft">${icon("grid")}${T("仪表盘", "Dashboards")}</button><div class="argus-nav-section" style="margin-top:var(--space-6)">${T("当前仪表盘", "CURRENT DASHBOARD")}</div><button class="argus-nav-item" data-action="published" ${!editing ? 'aria-current="page"' : ""}>${icon("check")}${T("已发布版本", "Published version")} · R${state.revision}</button><button class="argus-nav-item" data-action="draft" ${editing ? 'aria-current="page"' : ""}>${icon("edit")}${T("我的草稿", "My draft")}${dirty() ? '<span class="argus-dirty-dot"></span>' : ""}</button><div class="argus-nav-note">${T("先把图配置好，再统一发布这次修改。", "Configure panels first, then publish the session together.")}</div></aside><main class="argus-main"><div class="argus-between argus-heading"><div><div class="argus-flex" style="margin-bottom:var(--space-2)"><span class="argus-pill ${editing ? "argus-pill--accent" : "argus-pill--good"}">${editing ? T("个人草稿", "Personal draft") : T("已发布", "Published")}</span><small>${editing ? T("基于", "Based on") : ""} R${editing ? state.base : state.revision}</small></div><h1>${esc(b.name)}</h1><p class="argus-muted">${editing ? T("修改自动保存在本机演示草稿中，查看者仍使用已发布版本。", "Changes are saved to this local demo draft; viewers use the published version.") : T("这里展示已发布定义，草稿变更不会影响当前视图。", "This view uses the published definition; draft edits do not affect it.")}</p></div><div class="argus-flex">${editing ? `${btn("settings", T("仪表盘设置", "Settings"), "", "", "grid")}${btn("publish", T("预览并发布", "Preview & publish"), "argus-btn--primary", dirty() ? "" : "disabled", "check")}` : btn("draft", T("继续编辑草稿", "Continue editing"), "argus-btn--primary", "", "edit")}</div></div>${state.base !== state.revision && editing ? `<div class="argus-banner argus-banner--warning">${T("发布版本已变化，需要比较并整理草稿。", "The published version changed. Compare and reconcile your draft.")} ${btn("conflict", T("整理差异", "Resolve differences"), "argus-btn--small")}</div>` : ""}<div class="argus-toolbar">${scopeToolbar()}<div class="argus-flex">${editing ? `${btn("variables", T("管理过滤项", "Manage filters"), "argus-btn--ghost", "", "filter")}${btn("add", T("添加统计图", "Add panel"), "", "", "plus")}` : ""}</div></div><div class="argus-grid">${b.panels.map((p) => renderTile(p, editing)).join("")}${editing ? `<button class="argus-add-tile" data-action="add"><div class="argus-stack" style="justify-items:center">${icon("plus")}<span>${T("从常见统计图开始", "Start with a common panel")}</span><small>${T("数据、查询和推荐图型已预选", "Data, query and suggested chart are preselected")}</small></div></button>` : ""}</div><div class="argus-toolbar"><small>${T("时间与资源约束所有适用图；环境仅影响明确引用它的图。", "Time and resources scope all applicable panels; environment affects only panels that reference it.")}</small><span class="argus-saving">${T("演示草稿已自动保存", "Demo draft autosaved")}</span></div></main></div>`;
  }
  function renderTile(p, editing) {
    return `<section class="argus-panel" data-panel="${p.id}" style="grid-column:span ${p.w};height:${p.h}px"><header class="argus-between argus-panel-header" ${editing ? `draggable="true" data-drag="${p.id}"` : ""}><div><h2 class="argus-panel-title">${esc(title(p))}</h2><div class="argus-panel-source">${sourceName(p.source)} · ${signalName(p.signal)}${p.usesEnv ? ` · ${T("引用 $env", "Uses $env")}` : ""}</div></div><div class="argus-panel-actions">${editing ? `${btn("edit", T("编辑", "Edit"), "argus-btn--ghost argus-btn--small", `data-id="${p.id}"`, "edit")}${btn("clone", "", "argus-btn--ghost argus-btn--small", `data-id="${p.id}" aria-label="${T("复制统计图", "Duplicate panel")}"`, "copy")}` : ""}</div></header><div class="argus-panel-content">${plot(p, true)}</div><footer class="argus-panel-footer"><span>${p.signal === "traces" ? T("已接收样本统计", "Received-sample statistics") : T("示例数据", "Sample data")}</span>${editing ? `<div class="argus-layout-tools"><small>${p.w}/12</small><button class="argus-btn argus-btn--ghost argus-btn--small" data-resize="${p.id}" aria-label="${T("拖动调整大小，方向键调整宽高", "Drag to resize, or use arrow keys")}">${icon("resize")}</button></div>` : `<small>${chartName(p.chart)}</small>`}</footer></section>`;
  }
  function renderEditor() {
    const p = panel(),
      e = state.editor,
      invalid = validation(p),
      vp = panelView(p);
    return `${header(true)}<main class="argus-editor"><div class="argus-between argus-editor-heading"><div class="argus-flex">${btn("back", T("返回仪表盘", "Back to dashboard"), "argus-btn--ghost", "", "back")}<span class="argus-nav-divider"></span><input id="panel-title" data-field="panel-title" class="argus-title-input" value="${esc(title(p))}" aria-label="${T("统计图名称", "Panel title")}"></div><div class="argus-flex"><small class="argus-saving">${T("编辑内容已暂存", "Working edit retained")}</small>${btn("apply", e.fresh ? T("加入草稿", "Add to draft") : T("完成编辑", "Apply to draft"), "argus-btn--primary", invalid ? "disabled" : "", "check")}</div></div><div class="argus-editor-layout"><div><section class="argus-card"><div class="argus-card-heading"><div class="argus-flex"><h3>${T("预览", "Preview")}</h3><span class="argus-pill ${e.queryDirty ? "argus-pill--warn" : "argus-pill--good"}">${e.queryDirty ? T("查询已修改，待运行", "Query changed · run to preview") : T("示例结果", "Sample result")}</span></div><div class="argus-flex">${select(
      "scope-range",
      state.scope.range,
      [
        ["15m", T("最近 15 分钟", "Last 15 min")],
        ["1h", T("最近 1 小时", "Last hour")],
        ["6h", T("最近 6 小时", "Last 6 hours")],
      ],
      `class="argus-select" aria-label="${T("预览时间范围", "Preview time range")}"`,
    )}${btn("result", state.resultView === "chart" ? T("数据表格", "Data table") : T("返回图形", "Back to chart"), "argus-btn--small")}</div></div><div class="argus-preview"><div class="argus-preview-chart">${state.resultView === "table" ? plot({ ...vp, chart: "table" }) : plot(vp)}</div><div class="argus-preview-caption">${p.signal === "traces" ? T("口径：仅统计已接收样本，不推算全量。", "Basis: received samples only; no extrapolation.") : T("示例数据 · 调整样式立即生效，查询修改需运行预览。", "Sample data · style changes apply immediately; run after query changes.")}${e.queryDirty ? ` · ${T("仍显示上次运行的数据。", "Showing the previous query result.")}` : ""}</div></div></section>${renderQuery(p)}${invalid ? `<div class="argus-banner argus-banner--error" style="margin-top:var(--space-4)" role="alert">${invalid}</div>` : ""}${["empty", "unavailable"].includes(vp.status) ? `<div class="argus-banner argus-banner--warning" style="margin-top:var(--space-4)">${T("样本未验证正常，可保留配置；发布时会明确提示。", "Sample data is unverified. Keep the configuration; publication will show a warning.")}</div>` : ""}</div><aside class="argus-card argus-rail"><div class="argus-tabs" role="tablist" aria-label="${T("显示与交互配置", "Visualization settings")}">${["chart", "style", "interaction"].map((id, i) => `<button role="tab" data-action="tab" data-id="${id}" aria-selected="${state.tab === id}" aria-controls="rail-panel" id="tab-${id}" tabindex="${state.tab === id ? 0 : -1}">${[T("图型", "Chart"), T("样式", "Style"), T("交互", "Interaction")][i]}</button>`).join("")}</div><section id="rail-panel" role="tabpanel" aria-labelledby="tab-${state.tab}" class="argus-rail-body">${renderRail(p)}</section></aside></div></main>`;
  }
  function renderQuery(p) {
    const language =
      p.signal === "metrics"
        ? "PromQL"
        : p.signal === "logs"
          ? "KQL"
          : "Trace GraphQL";
    const sources =
      p.signal === "metrics"
        ? ["hostmetrics", "prometheus", "otlp"]
        : p.signal === "logs"
          ? ["otlp", "filelog"]
          : ["otlp", "skywalking", "jaeger"];
    let fields;
    if (p.mode === "code")
      fields =
        field(
          language,
          `<textarea id="expression" data-field="expression" spellcheck="false" aria-label="${language}">${esc(p.expression)}</textarea>`,
        ) +
        (!convertible(p)
          ? `<p class="argus-query-summary">${T("此语句不能无损还原为当前构建器，继续使用语句模式。", "This query cannot be restored losslessly to this builder. Continue in code mode.")}</p>`
          : "");
    else if (p.signal === "metrics")
      fields = `<div class="argus-query-row">${field(
        T("指标", "Metric"),
        select(
          "metric",
          p.metric,
          metrics.map((m) => [
            m.id,
            `${metricLabel(m)} · ${T(...{ gauge: ["瞬时值", "Gauge"], counter: ["累计计数", "Counter"], histogram: ["分布桶", "Buckets"] }[m.type])}`,
          ]),
        ),
      )}${field(
        T("跨资源计算", "Across resources"),
        select(
          "operation",
          p.operation,
          metricInfo(p).type === "histogram"
            ? [["value", T("原始直方图桶", "Raw histogram buckets")]]
            : metricInfo(p).type === "counter"
              ? [
                  ["rate", T("每秒速率", "Rate per second")],
                  ["value", T("原始累计值", "Raw cumulative value")],
                ]
              : [
                  ["avg", T("平均值", "Average")],
                  ["max", T("最大值", "Maximum")],
                  ["min", T("最小值", "Minimum")],
                  ["sum", T("总和", "Sum")],
                  ["value", T("原始值", "Raw value")],
                ],
        ),
      )}${field(
        T("拆分曲线", "Split series"),
        select("group", p.group || "", [
          ["", T("不拆分", "No split")],
          ["host_name", T("按主机", "By host")],
          ["service_name", T("按服务", "By service")],
        ]),
      )}</div><p class="argus-query-summary">${esc(metricInfo(p).id)} · ${T("单位和数据类型来自指标目录。", "Unit and data type come from the metric catalog.")}</p>`;
    else if (p.signal === "logs")
      fields = `<div class="argus-inline-fields">${field(
        T("查看什么", "What to show"),
        select("operation", p.operation, [
          ["records", T("日志内容", "Log records")],
          ["count_over_time", T("数量随时间变化", "Count over time")],
          ["count", T("当前范围的日志数量", "Log count in range")],
        ]),
      )}${
        p.operation === "count_over_time"
          ? field(
              T("统计间隔", "Time bucket"),
              select("bucket", p.bucket, [
                [60, T("自动 · 1 分钟", "Auto · 1 minute")],
                [300, T("5 分钟", "5 minutes")],
              ]),
            )
          : field(
              T("返回条数", "Result limit"),
              select("limit", p.limit, [
                [100, "100"],
                [500, "500"],
                [1000, "1,000"],
              ]),
            )
      }</div>`;
    else
      fields = `<div class="argus-inline-fields">${field(
        T("分析对象", "Analysis view"),
        select("operation", p.operation, [
          ["list", T("链路列表", "Trace list")],
          ["apm_services", T("服务总览", "Services overview")],
        ]),
      )}${field(
        T("服务", "Service"),
        select("service", p.service || "all", [
          ["all", T("全部适用服务", "All applicable services")],
          ["checkout-api", "checkout-api"],
          ["payment-api", "payment-api"],
        ]),
      )}</div><p class="argus-query-summary">${T("服务与实例仅在当前采集来源内匹配，不按同名自动合并。", "Services and instances are resolved within this source; matching names are not automatically merged.")}</p>`;
    return `<section class="argus-card argus-query-card"><div class="argus-card-heading"><div class="argus-flex"><span class="argus-pill">A</span><h3>${T("数据查询", "Data query")}</h3></div><div class="argus-flex"><div class="argus-segments" aria-label="${T("查询编辑方式", "Query editor mode")}"><button data-action="mode" data-id="builder" aria-pressed="${p.mode === "builder"}" ${p.mode === "code" && !convertible(p) ? `disabled title="${T("无法完整无损转换", "Lossless conversion unavailable")}"` : ""}>${T("构建器", "Builder")}</button><button data-action="mode" data-id="code" aria-pressed="${p.mode === "code"}">${language}</button></div>${btn("run", T("运行预览", "Run preview"), "argus-btn--primary argus-btn--small", "", "play")}</div></div><div class="argus-query-body"><div class="argus-source-line"><span>${T("数据来源", "Data source")}</span>${select(
      "source",
      p.source,
      sources.map((s) => [s, sourceName(s)]),
      `class="argus-select" aria-label="${T("数据来源", "Data source")}"`,
    )}<span class="argus-scope">${T("继承顶部时间 / 资源范围", "Uses dashboard time / resource scope")}</span></div>${fields}<div class="argus-filters" ${p.mode === "code" ? "hidden" : ""}>${p.filters.map((f, i) => `<span class="argus-filter-chip">${esc(f.field)} = ${esc(f.value)} <button data-action="remove-filter" data-id="${i}" aria-label="${T("移除筛选条件", "Remove filter")}">×</button></span>`).join("")}${p.usesEnv ? `<span class="argus-filter-chip">${T("环境", "Environment")} = <strong>$env</strong><button data-action="unlink-env" aria-label="${T("取消引用环境变量", "Unlink environment variable")}">×</button></span>` : ""}${btn("filter", T("添加筛选", "Add filter"), "argus-btn--ghost argus-btn--small", "", "plus")}</div><details class="argus-disclosure" ${state.advanced ? "open" : ""} id="query-advanced"><summary>${T("高级查询选项", "Advanced query options")}</summary><div>${
      p.signal === "metrics"
        ? field(
            T("查询分辨率", "Query resolution"),
            select("step", p.step, [
              ["auto", T("自动，按图宽选择", "Auto, based on chart width")],
              ["60", T("固定 60 秒", "Fixed 60 seconds")],
              ["300", T("固定 5 分钟", "Fixed 5 minutes")],
            ]),
          )
        : p.signal === "traces" && p.mode === "builder"
          ? field(
              T("返回样本数量上限", "Sample limit"),
              select("limit", p.limit, [
                [100, "100"],
                [500, "500"],
                [1000, "1,000"],
              ]),
            )
          : ""
    }<div class="argus-callout">${T("内部目标 ID、权限范围和执行预算由系统维护。额外查询与参数映射可在正式高级编辑器中配置。", "The system maintains target IDs, permissions and execution budgets. Additional queries and parameter mappings belong in the full advanced editor.")}</div><div class="argus-readonly">${esc(query(p))}</div></div></details></div><div class="argus-query-footer"><span>${T("仅演示交互与基本格式检查；真实发布需服务端验证。", "Interaction and basic format checks only; real publication requires server validation.")}</span><span>Ctrl / ⌘ + Enter</span></div></section>`;
  }
  function renderRail(p) {
    if (
      state.tab === "style" &&
      (p.signal === "traces" ||
        (p.signal === "logs" && p.operation === "records"))
    ) {
      return `<h3>${T("标准明细样式", "Standard detail styling")}</h3><div class="argus-auto"><span>${T("列格式", "Column formats")}</span><strong>${T("按字段类型", "From field types")}</strong></div><p class="argus-help">${T("日志与链路列表按标准字段显示时间、级别或状态、耗时与内容，无需设置数值图的单位和图例。", "Log and trace lists use standard time, severity/status, duration and content fields. Numeric chart units and legends do not apply.")}</p>`;
    }
    if (state.tab === "chart") {
      const compatibleTypes = compatible(p),
        common =
          p.signal === "metrics"
            ? metricInfo(p).type === "histogram"
              ? ["histogram", "heatmap", "table"]
              : ["timeseries", "stat", "gauge", "bar"]
            : compatibleTypes;
      const shown =
        state.moreCharts && p.signal === "metrics"
          ? Object.keys(charts).filter(
              (x) =>
                ![
                  "logs",
                  "trace_list",
                  "apm_services",
                  "apm_red",
                  "apm_topology",
                ].includes(x),
            )
          : common;
      return `<div><h3>${T("推荐图型", "Suggested charts")}</h3><p class="argus-help" style="margin-top:var(--space-1)">${T("根据当前查询的数据形状推荐。", "Suggested for the shape of this query.")}</p></div><div class="argus-chart-picker">${shown.map((c) => `<button class="argus-chart-option" data-action="chart" data-id="${c}" aria-pressed="${p.chart === c}" ${!compatibleTypes.includes(c) ? `disabled title="${T("需要其他数据形状，例如完整直方图桶。", "Requires a different data shape, such as histogram buckets.")}"` : ""}>${icon(charts[c][2])}<span>${chartName(c)}</span></button>`).join("")}</div>${p.signal === "metrics" ? btn("more-charts", state.moreCharts ? T("收起其他图型", "Show fewer charts") : T("全部 11 类图型", "All 11 chart types"), "argus-btn--ghost argus-btn--small") : ""}<div class="argus-auto"><span>${T("单位", "Unit")}</span><strong>${p.signal === "metrics" ? (metricInfo(p).unit === "percent_ratio" ? T("自动 · 百分比", "Auto · percentage") : metricInfo(p).unit) : T("自动", "Auto")}</strong></div><p class="argus-help">${T("切换显示图型不会改写查询。需要不同数据时，回到数据查询调整。", "Changing the visualization does not rewrite the query. Adjust data queries when a different result is needed.")}</p>`;
    }
    if (state.tab === "style")
      return `${field(
        T("单位", "Unit"),
        select("unit", p.unit, [
          ["auto", T("自动识别", "Automatic")],
          ["percent", T("百分比", "Percentage")],
          ["bytes", T("字节", "Bytes")],
          ["ms", T("毫秒", "Milliseconds")],
          ["s", T("秒", "Seconds")],
        ]),
      )}${
        ["stat", "gauge", "bar", "bar_gauge", "pie", "table"].includes(p.chart)
          ? field(
              T("显示哪个值", "Displayed value"),
              select("reducer", p.reducer, [
                ["last", T("最近值", "Last value")],
                ["mean", T("时段平均", "Range average")],
                ["max", T("时段最大值", "Range maximum")],
                ["min", T("时段最小值", "Range minimum")],
              ]),
            )
          : ""
      }${
        p.chart === "timeseries"
          ? field(
              T("绘制方式", "Draw style"),
              select("draw", p.draw, [
                ["line", T("折线", "Line")],
                ["area", T("面积", "Area")],
              ]),
            )
          : ""
      }<label class="argus-check"><input id="legend" data-field="legend" type="checkbox" ${p.legend ? "checked" : ""}>${T("显示图例", "Show legend")}</label><details id="display-more" class="argus-disclosure" ${state.openDetails?.["display-more"] ? "open" : ""}><summary>${T("更多显示设置", "More display settings")}</summary><div>${field(T("小数位数", "Decimals"), input("decimals", p.decimals, "number", 'min="0" max="5"'))}${field(T("参考阈值", "Reference threshold"), input("threshold", p.threshold, "number", `placeholder="${T("不设置", "Not set")}"`))}<p class="argus-help">${T("轴范围、颜色映射等能力保留在高级显示设置中，默认自动。", "Axis bounds and color mappings remain advanced options with automatic defaults.")}</p></div></details><p class="argus-help">${T("这些修改即时作用于已有结果，不额外发起数据查询。", "These changes immediately restyle the current result without another query.")}</p>`;
    return `<h3>${T("点击数据后的操作", "Click behavior")}</h3><label class="argus-check"><input id="drilldown" data-field="drilldown" type="checkbox" ${p.drilldown ? "checked" : ""}>${T("启用标准详情", "Enable standard details")}</label><div class="argus-auto"><span>${T("默认操作", "Default action")}</span><strong>${p.signal === "logs" ? T("日志上下文", "Log context") : p.signal === "traces" ? T("完整链路详情", "Trace details") : T("按资源查看详情", "Resource details")}</strong></div><p class="argus-help">${T("平台按来源和图型生成标准下钻，随统计图统一预览发布。", "The platform generates source-appropriate drilldowns, reviewed and published with the panel.")}</p><details id="interaction-more" class="argus-disclosure" ${state.openDetails?.["interaction-more"] ? "open" : ""}><summary>${T("编辑下钻定义", "Edit drilldown definition")}</summary><div><p class="argus-help">${T("正式实现会打开独立高级编辑器，复用数据查询构建器；不在当前表单嵌套另一整套表单。", "The product opens a separate advanced editor reusing the query builder, without nesting another full form here.")}</p><div class="argus-readonly">${p.signal === "traces" ? "Trace → Span → correlated logs" : p.signal === "logs" ? "Log → ±20 context rows → linked Trace" : "Series → resource scope → same time window"}</div></div></details>`;
  }
  function dialogFrame(name, sub, body, footer) {
    return `<header class="argus-between argus-dialog-header"><div><h2 id="modal-title">${esc(name)}</h2>${sub ? `<p class="argus-muted">${sub}</p>` : ""}</div>${btn("close-dialog", "", "argus-btn--ghost argus-btn--icon", `aria-label="${T("关闭", "Close")}"`, "close")}</header><div class="argus-dialog-body">${body}</div><footer class="argus-dialog-footer">${footer}</footer>`;
  }
  function renderDialog() {
    if (!state.dialog) return "";
    const kind = state.dialog.kind;
    if (kind === "add") {
      const choices = presets.filter(
        (p) =>
          (state.addSignal === "all" || p.signal === state.addSignal) &&
          (T(...p.title) + T(...p.description))
            .toLowerCase()
            .includes(state.catalogSearch.toLowerCase()),
      );
      return dialogFrame(
        T("添加统计图", "Add a panel"),
        T(
          "先选一个常见场景，进入后再调整。",
          "Start with a common use case, then customize it.",
        ),
        `<div class="argus-between"><div class="argus-segments">${["all", "metrics", "logs", "traces"].map((v) => `<button data-action="add-signal" data-id="${v}" aria-pressed="${state.addSignal === v}">${v === "all" ? T("全部", "All") : signalName(v)}</button>`).join("")}</div>${input("catalog-search", state.catalogSearch, "search", `placeholder="${T("搜索常见统计图", "Find common panels")}" aria-label="${T("搜索常见统计图", "Find common panels")}"`)}</div><div class="argus-template-grid">${choices.map((p) => `<button class="argus-template" data-action="preset" data-id="${p.id}">${icon(charts[p.chart][2])}<strong>${T(...p.title)}</strong><p>${T(...p.description)}</p><span class="argus-pill">${sourceName(p.source)} · ${chartName(p.chart)}</span></button>`).join("")}</div>`,
        btn(
          "blank",
          T("打开通用指标编辑器", "Open the metric editor"),
          "argus-btn--ghost",
        ),
      );
    }
    if (kind === "scope-env") {
      const values = Array.isArray(state.scope.env)
        ? state.scope.env
        : [state.scope.env];
      return dialogFrame(
        environmentLabel(),
        T(
          "只刷新明确引用 $env 的统计图。",
          "Refresh only panels that explicitly reference $env.",
        ),
        `<div class="argus-stack">${["all", "production", "staging"].map((v) => `<label class="argus-check"><input id="env-option-${v}" type="checkbox" data-field="scope-env-item" data-value="${v}" ${values.includes(v) ? "checked" : ""}>${v === "all" ? T("全部", "All") : v}</label>`).join("")}</div>`,
        btn("close-dialog", T("完成", "Done"), "argus-btn--primary"),
      );
    }
    if (kind === "filter") {
      const p = panel(),
        fields =
          p.signal === "logs"
            ? [
                ["severity", T("日志级别", "Severity")],
                ["service_name", T("服务", "Service")],
                ["deployment_environment", T("环境", "Environment")],
              ]
            : p.signal === "traces"
              ? [
                  ["service_name", T("服务", "Service")],
                  ["status", T("状态", "Status")],
                  ["deployment_environment", T("环境", "Environment")],
                ]
              : [
                  ["host_name", T("主机", "Host")],
                  ["deployment_environment", T("环境", "Environment")],
                ];
      const literals =
        state.filterField === "severity"
          ? [
              ["ERROR", "ERROR"],
              ["WARN", "WARN"],
              ["INFO", "INFO"],
            ]
          : state.filterField === "deployment_environment"
            ? [
                ["production", "production"],
                ["staging", "staging"],
              ]
            : state.filterField === "status"
              ? [
                  ["ERROR", "ERROR"],
                  ["OK", "OK"],
                ]
              : state.filterField === "host_name"
                ? [
                    ["host-a", "host-a"],
                    ["host-b", "host-b"],
                  ]
                : [
                    ["checkout-api", "checkout-api"],
                    ["payment-api", "payment-api"],
                  ];
      return dialogFrame(
        T("添加筛选", "Add a filter"),
        T(
          "只约束当前统计图。顶部时间和资源范围始终生效。",
          "Applies to this panel only. Dashboard time and resource scope still apply.",
        ),
        `<div class="argus-stack">${field(T("字段", "Field"), select("filter-field", state.filterField, fields))}${field(T("匹配值", "Value"), select("filter-value", state.filterValue, literals))}${state.draft.variable.enabled ? `<label class="argus-check"><input id="filter-variable" data-field="filter-variable" type="checkbox" ${state.filterVariable ? "checked" : ""}>${T("引用仪表盘变量 $env", "Use dashboard variable $env")}</label><p class="argus-help">${T("引用后仅此图随变量改变。字段绑定明确保存，不按同名自动推断。", "Only this panel responds to the variable. The field binding is explicit.")}</p>` : ""}</div>`,
        `${btn("close-dialog", T("取消", "Cancel"))}${btn("add-filter", T("添加", "Add"), "argus-btn--primary")}`,
      );
    }
    if (kind === "variables")
      return dialogFrame(
        T("仪表盘过滤项", "Dashboard filters"),
        T(
          "时间和资源是固定系统过滤；自定义变量只影响引用者。",
          "Time and resources are system filters. Custom variables affect references only.",
        ),
        `<div class="argus-stack"><div class="argus-auto"><span>${T("时间范围 / 资源范围", "Time / resources")}</span><strong>${T("系统提供", "Built in")}</strong></div><div class="argus-change"><div><strong>${T("环境", "Environment")} <span class="argus-pill">$env</span></strong><p class="argus-muted">${T("候选字段：deployment_environment · 主机指标", "Candidate field: deployment_environment · host metrics")}</p></div>${btn("edit-variable", T("配置", "Configure"), "argus-btn--small")}</div><p class="argus-help">${T("无需手写变量查询。选择候选字段和使用它的统计图即可。", "No variable query text is needed. Choose the candidate field and consuming panels.")}</p></div>`,
        btn("close-dialog", T("完成", "Done"), "argus-btn--primary"),
      );
    if (kind === "variable")
      return dialogFrame(
        T("配置环境过滤项", "Configure environment filter"),
        T(
          "变量查询使用构建器；引用范围由作者明确选择。",
          "Variables use a builder; the author chooses their consumers.",
        ),
        `<div class="argus-stack">${field(T("显示名称", "Display name"), input("variable-label", state.draft.variable.label))}<div class="argus-auto"><span>${T("候选来源", "Candidate source")}</span><strong>${T("主机指标", "Host metrics")}</strong></div>${field(T("候选字段", "Candidate field"), select("variable-field", state.draft.variable.field, [["deployment_environment", T("环境 · deployment_environment", "Environment · deployment_environment")]]))}<label class="argus-check"><input id="variable-multiple" data-field="variable-multiple" type="checkbox" ${state.draft.variable.multiple ? "checked" : ""}>${T("允许多选", "Allow multiple values")}</label><div><h3>${T("使用此变量的统计图", "Panels using this variable")}</h3><div class="argus-stack" style="margin-top:var(--space-3)">${state.draft.panels.map((p) => `<label class="argus-check"><input id="consumer-${p.id}" type="checkbox" data-consumer="${p.id}" ${p.usesEnv ? "checked" : ""}>${esc(title(p))}<small>${sourceName(p.source)}</small></label>`).join("")}</div></div><div class="argus-callout">${T("示例候选：production、staging。只有确认完整候选中选值已消失，才回退“全部”；失败或分页不足不回退。", "Example candidates: production, staging. Reset to All only after verified absence, never on errors or incomplete pages.")}</div></div>`,
        btn(
          "close-dialog",
          T("完成并保存草稿", "Done · save draft"),
          "argus-btn--primary",
        ),
      );
    if (kind === "settings")
      return dialogFrame(
        T("仪表盘设置", "Dashboard settings"),
        T(
          "这里管理整板信息，统计图的查询留在各图编辑器中。",
          "Board settings stay here; panel queries stay in the panel editor.",
        ),
        `<div class="argus-stack">${field(T("名称", "Name"), input("board-name", state.draft.name))}${field(
          T("默认时间范围", "Default time range"),
          select("default-range", state.draft.defaultRange, [
            ["15m", T("最近 15 分钟", "Last 15 min")],
            ["1h", T("最近 1 小时", "Last hour")],
            ["6h", T("最近 6 小时", "Last 6 hours")],
          ]),
        )}<details id="settings-more" class="argus-disclosure" ${state.openDetails?.["settings-more"] ? "open" : ""}><summary>${T("说明与目录", "Description and folder")}</summary><div>${field(T("说明", "Description"), input("board-description", state.draft.description))}<div class="argus-auto"><span>${T("目录", "Folder")}</span><strong>${T("运维 / 生产环境", "Operations / Production")}</strong></div></div></details><p class="argus-help">${T("修改保存为个人草稿，不触发发布确认。", "Changes are saved to the personal draft without a publication confirmation.")}</p></div>`,
        btn("close-dialog", T("完成", "Done"), "argus-btn--primary"),
      );
    if (kind === "publish") {
      const errors = [
        ...(!state.draft.name.trim()
          ? [T("仪表盘名称不能为空。", "Dashboard title is required.")]
          : []),
        ...state.draft.panels.map(validation).filter(Boolean),
      ];
      const changes = changedPanels(),
        warnings = state.draft.panels.filter((p) =>
          ["empty", "unavailable"].includes(p.status),
        );
      return dialogFrame(
        T("预览并发布", "Preview & publish"),
        `R${state.base} → R${state.revision + 1} · ${T("本次编辑的修改一次确认", "Confirm the editing session once")}`,
        `${errors.length ? `<div class="argus-banner argus-banner--error" role="alert">${errors.map(esc).join(" ")}</div>` : ""}${state.base !== state.revision ? `<div class="argus-banner argus-banner--error">${T("基线版本已变化，禁止覆盖。请先整理差异。", "The baseline changed. Publication is blocked until differences are resolved.")}</div>` : ""}<div class="argus-change-list">${changes.map((p) => `<div class="argus-change"><div><strong>${esc(title(p))}</strong><p class="argus-muted">${sourceName(p.source)} · ${chartName(p.chart)}</p></div><span class="argus-pill argus-pill--info">${state.published.panels.some((x) => x.id === p.id) ? T("修改", "Changed") : T("新增", "Added")}</span></div>`).join("")}${JSON.stringify(state.draft.variable) !== JSON.stringify(state.published.variable) ? `<div class="argus-change">${T("过滤项配置", "Filter configuration")}<span class="argus-pill">${T("修改", "Changed")}</span></div>` : ""}${state.draft.name !== state.published.name ? `<div class="argus-change">${T("仪表盘名称", "Dashboard name")}<strong>${esc(state.draft.name)}</strong></div>` : ""}</div><div class="argus-callout">${T("原型配置检查通过。真实发布仍需服务端校验语法、结果类型、权限、预算和版本基线。", "Prototype checks passed. Real publication requires server-side syntax, result-type, permission, budget and version validation.")}</div>${warnings.length ? `<div class="argus-banner argus-banner--warning" style="margin-top:var(--space-3)">${warnings.length} ${T("张图样本未验证正常。允许保留配置，不代表数据已验证。", "panels have unverified samples. Keeping configuration does not validate data.")}</div>` : ""}<p class="argus-query-summary">${T("确认后普通查看使用新的已发布版本；草稿保存不会反复要求确认。", "After confirmation, normal viewing uses the new published revision. Draft saves require no repeated confirmation.")}</p>`,
        `${btn("simulate-conflict", T("模拟他人先发布", "Simulate another editor publishing"), "argus-btn--ghost argus-btn--small")}${state.base !== state.revision ? btn("conflict", T("整理差异", "Resolve differences"), "argus-btn--primary") : `${btn("close-dialog", T("继续编辑", "Keep editing"))}${btn("confirm-publish", T("确认发布（原型）", "Confirm publication (prototype)"), "argus-btn--primary", errors.length ? "disabled" : "")}`}`,
      );
    }
    if (kind === "conflict")
      return dialogFrame(
        T("整理版本差异", "Resolve version differences"),
        `R${state.base} → R${state.revision} · ${T("不会自动覆盖他人的发布", "No silent overwrite")}`,
        `<div class="argus-conflict"><div><h3>${T("最新发布内容", "Latest published content")}</h3><p>${esc(state.published.name)}</p><p class="argus-query-summary">${T("示例：另一位编辑者修改了仪表盘名称。", "Example: another editor changed the board title.")}</p></div><div><h3>${T("我的草稿", "My draft")}</h3><p>${esc(state.draft.name)}</p><p class="argus-query-summary">${changedPanels().length} ${T("张图的修改待整理", "panel edits to reconcile")}</p></div></div><div class="argus-stack" style="margin-top:var(--space-4)">${field(
          T("名称采用哪一版", "Which title to keep"),
          select("conflict-choice", state.conflictChoice, [
            ["latest", T("采用最新发布的名称", "Use latest published title")],
            ["mine", T("保留我的名称", "Keep my title")],
          ]),
        )}<p class="argus-help">${T("确认此项整理后保留你的统计图修改，并重新建立发布基线；仍需再次预览发布。", "Resolve this item, keep your panel edits, and establish a new baseline. Preview publication again.")}</p></div>`,
        `${btn("close-dialog", T("稍后整理", "Later"))}${btn("resolve-conflict", T("确认整理", "Confirm reconciliation"), "argus-btn--primary")}`,
      );
    if (kind === "detail")
      return dialogFrame(
        state.dialog.data === "logs"
          ? T("日志上下文", "Log context")
          : T("链路详情", "Trace details"),
        T(
          "标准下钻示例 · 继承当前时间与资源范围",
          "Standard drilldown example · current time and resources",
        ),
        state.dialog.data === "logs"
          ? `<div class="argus-readonly">14:56:02 INFO  request accepted\n14:56:02 ERROR payment upstream timeout trace_id=8ef014ac\n14:56:03 INFO  retry scheduled</div>`
          : `<div class="argus-auto"><span>Trace 8ef014ac</span><strong>1,240 ms · 18 spans</strong></div><div class="argus-stack" style="margin-top:var(--space-4)">${["gateway · 1,240 ms", "checkout-api · 1,080 ms", "payment-api · 920 ms", "postgres · 85 ms"].map((x, i) => `<div style="margin-left:${i * 5}%;width:${95 - i * 17}%;background:var(--${i === 2 ? "warning-soft" : "info-soft"});color:var(--text-secondary);padding:var(--space-2);border-radius:var(--radius-xs)">${x}</div>`).join("")}</div>`,
        btn("close-dialog", T("关闭", "Close")),
      );
    if (kind === "scenario")
      return dialogFrame(
        T("演示场景", "Prototype scenarios"),
        T(
          "仅用于检查原型状态，不是正式产品中的配置项。",
          "Prototype controls only; these are not product configuration fields.",
        ),
        `<div class="argus-stack">${state.editor ? `<h3>${T("当前图的样本状态", "Current panel sample state")}</h3>${["success", "empty", "unavailable", "invalid"].map((v) => btn("sample-state", { success: T("正常示例数据", "Normal sample data"), empty: T("无数据", "No data"), unavailable: T("暂时不可用", "Temporarily unavailable"), invalid: T("硬配置错误", "Invalid configuration") }[v], "", `data-id="${v}"`)).join("")}${panel().signal === "metrics" ? btn("complex-code", T("尝试不可无损还原的 PromQL", "Try a PromQL query beyond the builder")) : ""}` : `${btn("simulate-conflict", T("模拟另一位编辑者发布", "Simulate concurrent publication"))}<p class="argus-help">${T("进入统计图编辑器后可体验无数据、失败和语句模式。", "Open a panel editor to try no-data, failure and code-mode scenarios.")}</p>`}</div>`,
        btn("close-dialog", T("关闭", "Close")),
      );
    if (kind === "discard")
      return dialogFrame(
        T("返回仪表盘", "Return to dashboard"),
        T(
          "当前图有未加入整板草稿的修改。",
          "This panel has changes not yet applied to the board draft.",
        ),
        `<p>${T("可以先完成编辑；或放弃这张图的临时修改。其他图的草稿不会被清除。", "Apply the panel edit, or discard its working changes. Other panel drafts are kept.")}</p>`,
        `${btn("discard-panel", T("放弃本图修改", "Discard panel changes"))}${btn("close-dialog", T("继续编辑", "Keep editing"), "argus-btn--primary")}`,
      );
    if (kind === "reset")
      return dialogFrame(
        T("重置交互原型", "Reset the prototype"),
        "",
        `<p>${T("恢复示例仪表盘和本机演示草稿。不会操作正式产品的数据。", "Reset the sample dashboard and this demo's local draft. Product data is unaffected.")}</p>`,
        `${btn("close-dialog", T("取消", "Cancel"))}${btn("confirm-reset", T("重置原型", "Reset prototype"), "argus-btn--primary")}`,
      );
    return "";
  }
  function changedPanels() {
    return state.draft.panels.filter(
      (p) =>
        JSON.stringify(p) !==
        JSON.stringify(state.published.panels.find((x) => x.id === p.id)),
    );
  }
  function render() {
    const active = document.activeElement,
      id = active?.id,
      selection = active?.selectionStart;
    state.openDetails ||= {};
    document
      .querySelectorAll("details[id]")
      .forEach((d) => (state.openDetails[d.id] = d.open));
    state.advanced = Boolean(state.openDetails["query-advanced"]);
    document.documentElement.dataset.theme = state.theme;
    document.documentElement.lang = state.lang === "en" ? "en" : "zh-CN";
    app.innerHTML = state.editor ? renderEditor() : renderBoard();
    if (state.dialog) {
      modal.innerHTML = renderDialog();
      if (!modal.open) modal.showModal();
    } else if (modal.open) modal.close();
    if (id) {
      const next = document.getElementById(id);
      if (next && (!modal.open || modal.contains(next))) {
        next.focus({ preventScroll: true });
        if (
          typeof selection === "number" &&
          next.setSelectionRange &&
          !["number", "checkbox"].includes(next.type)
        )
          try {
            next.setSelectionRange(selection, selection);
          } catch {}
      }
    }
  }
  function openDialog(kind, data) {
    state.dialog = { kind, data };
    render();
  }
  function applyPanel() {
    const e = state.editor;
    if (validation(e.panel)) return;
    const p = copy(e.panel);
    const index = state.draft.panels.findIndex((x) => x.id === p.id);
    if (index >= 0) state.draft.panels[index] = p;
    else state.draft.panels.push(p);
    state.editor = null;
    state.view = "draft";
    render();
    persist();
    toast(
      T(
        "已加入个人草稿，尚未发布。",
        "Applied to your draft; not published yet.",
      ),
    );
  }
  function simulateConflict() {
    state.published.name = T(
      "生产环境概览 · 服务视角",
      "Production overview · Service view",
    );
    state.revision++;
    state.dialog = { kind: "conflict" };
    render();
    persist();
  }
  function onClick(event) {
    const b = event.target.closest("[data-action]");
    if (!b || b.disabled) return;
    const { action, id, kind } = b.dataset,
      p = panel();
    switch (action) {
      case "language":
        state.lang = state.lang === "zh" ? "en" : "zh";
        break;
      case "theme":
        state.theme = state.theme === "light" ? "dark" : "light";
        break;
      case "reset":
        openDialog("reset");
        return;
      case "confirm-reset":
        state.lang = "zh";
        state = initialState();
        persist();
        break;
      case "published":
        state.view = "published";
        state.editor = null;
        normalizeScope();
        break;
      case "draft":
        state.view = "draft";
        state.editor = null;
        normalizeScope();
        break;
      case "add":
        state.addSignal = "all";
        state.catalogSearch = "";
        openDialog("add");
        return;
      case "add-signal":
        state.addSignal = id;
        break;
      case "preset": {
        const preset = presets.find((x) => x.id === id);
        const created = makePanel(preset);
        created.origin = id;
        startEditor(created, true);
        return;
      }
      case "blank": {
        const created = makePanel(presets[0]);
        created.title = T("新统计图", "New panel");
        created.customTitle = true;
        startEditor(created, true);
        return;
      }
      case "edit":
        startEditor(state.draft.panels.find((x) => x.id === id));
        return;
      case "clone": {
        const c = copy(state.draft.panels.find((x) => x.id === id));
        c.id = crypto.randomUUID();
        c.title = title(c) + T(" · 副本", " · Copy");
        c.customTitle = true;
        state.draft.panels.push(c);
        toast(T("已复制到草稿。", "Copied into the draft."));
        break;
      }
      case "back":
        if (
          JSON.stringify(state.editor.panel) !==
          JSON.stringify(state.editor.initial)
        ) {
          openDialog("discard");
          return;
        }
        state.editor = null;
        break;
      case "discard-panel":
        state.editor = null;
        state.dialog = null;
        break;
      case "apply":
        applyPanel();
        return;
      case "tab":
        state.tab = id;
        break;
      case "more-charts":
        state.moreCharts = !state.moreCharts;
        break;
      case "chart":
        if (compatible(p).includes(id)) p.chart = id;
        break;
      case "mode": {
        const before = query(p);
        if (id === "code" && p.mode === "builder") {
          p.expression = query(p);
          p.mode = "code";
        } else if (id === "builder" && convertible(p)) {
          p.mode = "builder";
        }
        state.editor.queryDirty =
          state.editor.queryDirty || before !== query(p);
        break;
      }
      case "run":
        if (validation(p)) {
          toast(validation(p));
          return;
        }
        state.editor.last = copy(p);
        state.editor.queryDirty = false;
        toast(T("已更新示例预览。", "Sample preview updated."));
        break;
      case "result":
        state.resultView = state.resultView === "chart" ? "table" : "chart";
        break;
      case "filter":
        state.filterField =
          p.signal === "logs"
            ? "severity"
            : p.signal === "traces"
              ? "service_name"
              : "host_name";
        state.filterValue =
          p.signal === "logs"
            ? "ERROR"
            : p.signal === "traces"
              ? "checkout-api"
              : "host-a";
        state.filterVariable = false;
        openDialog("filter");
        return;
      case "add-filter":
        if (state.filterVariable) {
          p.usesEnv = true;
        } else
          p.filters.push({
            field: state.filterField,
            value: state.filterValue,
          });
        state.editor.queryDirty = true;
        state.dialog = null;
        break;
      case "remove-filter":
        p.filters.splice(Number(id), 1);
        state.editor.queryDirty = true;
        break;
      case "unlink-env":
        p.usesEnv = false;
        state.editor.queryDirty = true;
        break;
      case "scope-env-open":
        openDialog("scope-env");
        return;
      case "settings":
        openDialog("settings");
        return;
      case "variables":
        openDialog("variables");
        return;
      case "edit-variable":
        openDialog("variable");
        return;
      case "publish":
        if (!dirty()) return;
        openDialog("publish");
        return;
      case "confirm-publish":
        if (
          state.base !== state.revision ||
          !state.draft.name.trim() ||
          state.draft.panels.some((x) => validation(x))
        )
          return;
        state.published = copy(state.draft);
        state.revision++;
        state.base = state.revision;
        state.dialog = null;
        state.view = "published";
        toast(
          T(
            "原型已发布，普通查看已切换到新版本。",
            "Prototype published; viewing now uses the new revision.",
          ),
        );
        break;
      case "simulate-conflict":
        simulateConflict();
        return;
      case "conflict":
        openDialog("conflict");
        return;
      case "resolve-conflict":
        if (state.conflictChoice === "latest")
          state.draft.name = state.published.name;
        state.base = state.revision;
        state.dialog = null;
        state.view = "draft";
        toast(
          T(
            "已整理基线，请重新预览发布。",
            "Baseline reconciled. Preview publication again.",
          ),
        );
        break;
      case "scenario":
        openDialog("scenario");
        return;
      case "sample-state":
        p.status = id;
        state.editor.last = copy(p);
        state.editor.queryDirty = false;
        state.dialog = null;
        break;
      case "complex-code":
        p.mode = "code";
        p.expression = "avg_over_time(system_cpu_utilization[10m])";
        state.editor.queryDirty = true;
        state.dialog = null;
        break;
      case "detail":
        openDialog("detail", kind);
        return;
      case "close-dialog":
        state.dialog = null;
        break;
      default:
        return;
    }
    render();
    persist();
  }
  function onField(event) {
    const el = event.target,
      id = el.dataset.field,
      p = panel();
    if (el.dataset.consumer) {
      const consumer = state.draft.panels.find(
        (x) => x.id === el.dataset.consumer,
      );
      consumer.usesEnv = el.checked;
      persist();
      return;
    }
    if (!id) return;
    const value = el.type === "checkbox" ? el.checked : el.value;
    if (id === "scope-env-item") {
      let selected = Array.isArray(state.scope.env)
        ? [...state.scope.env]
        : [state.scope.env];
      if (el.dataset.value === "all") selected = ["all"];
      else {
        selected = selected.filter(
          (v) => v !== "all" && v !== el.dataset.value,
        );
        if (el.checked) selected.push(el.dataset.value);
        if (!selected.length) selected = ["all"];
      }
      state.scope.env = selected;
      render();
      persist();
      return;
    }
    if (id.startsWith("scope-")) {
      state.scope[id.slice(6)] = value;
      if (state.editor) {
        state.editor.last = copy(p);
        state.editor.queryDirty = false;
      }
    } else if (id === "catalog-search") state.catalogSearch = value;
    else if (id === "board-name") state.draft.name = value;
    else if (id === "board-description") state.draft.description = value;
    else if (id === "default-range") state.draft.defaultRange = value;
    else if (id === "variable-label") state.draft.variable.label = value;
    else if (id === "variable-field") state.draft.variable.field = value;
    else if (id === "variable-multiple") {
      state.draft.variable.multiple = value;
      normalizeScope();
    } else if (id === "conflict-choice") state.conflictChoice = value;
    else if (id.startsWith("filter-")) {
      if (id === "filter-field") {
        state.filterField = value;
        state.filterValue =
          value === "severity"
            ? "ERROR"
            : value === "deployment_environment"
              ? "production"
              : value === "status"
                ? "ERROR"
                : value === "host_name"
                  ? "host-a"
                  : "checkout-api";
        if (value !== "deployment_environment") state.filterVariable = false;
      } else if (id === "filter-value") state.filterValue = value;
      else {
        state.filterVariable = value;
        if (value) state.filterField = "deployment_environment";
      }
    } else if (p) {
      if (id === "panel-title") {
        p.title = value;
        p.customTitle = true;
      } else {
        p[id] = ["bucket", "limit", "decimals"].includes(id)
          ? Number(value)
          : value;
      }
      if (
        [
          "metric",
          "source",
          "operation",
          "group",
          "expression",
          "bucket",
          "limit",
          "service",
          "step",
        ].includes(id)
      )
        state.editor.queryDirty = true;
      if (id === "expression" && p.mode === "code")
        p.usesEnv = String(value).includes("$env");
      if (id === "metric") {
        p.operation =
          metricInfo(p).type === "histogram"
            ? "value"
            : metricInfo(p).type === "counter"
              ? "rate"
              : "avg";
        if (!compatible(p).includes(p.chart)) p.chart = compatible(p)[0];
      }
      if (id === "operation" && !compatible(p).includes(p.chart))
        p.chart = compatible(p)[0];
    }
    render();
    clearTimeout(saveTimer);
    saveTimer = setTimeout(persist, 250);
  }
  app.addEventListener("click", onClick);
  modal.addEventListener("click", onClick);
  app.addEventListener("input", onField);
  modal.addEventListener("input", onField);
  modal.addEventListener("cancel", () => {
    state.dialog = null;
    persist();
  });
  modal.addEventListener("click", (e) => {
    if (e.target === modal) {
      state.dialog = null;
      modal.close();
      persist();
    }
  });
  document.addEventListener("keydown", (e) => {
    if (
      (e.ctrlKey || e.metaKey) &&
      e.key === "Enter" &&
      state.editor &&
      !state.dialog
    ) {
      e.preventDefault();
      if (!validation(panel())) {
        state.editor.last = copy(panel());
        state.editor.queryDirty = false;
        render();
        persist();
      }
    }
    if (
      e.target.matches('[role="tab"]') &&
      ["ArrowLeft", "ArrowRight"].includes(e.key)
    ) {
      e.preventDefault();
      const ids = ["chart", "style", "interaction"];
      state.tab =
        ids[(ids.indexOf(state.tab) + (e.key === "ArrowRight" ? 1 : 2)) % 3];
      render();
      document.getElementById("tab-" + state.tab).focus();
    }
    if (
      e.target.dataset.resize &&
      ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(e.key)
    ) {
      e.preventDefault();
      const p = state.draft.panels.find(
        (x) => x.id === e.target.dataset.resize,
      );
      p.w = Math.max(
        3,
        Math.min(
          12,
          p.w + (e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0),
        ),
      );
      p.h = Math.max(
        260,
        p.h + (e.key === "ArrowDown" ? 20 : e.key === "ArrowUp" ? -20 : 0),
      );
      render();
      persist();
      document.querySelector(`[data-resize="${p.id}"]`).focus();
    }
    if (e.key === "Enter" && e.target.matches('tr[data-action="detail"]'))
      onClick({ target: e.target });
  });
  app.addEventListener("dragstart", (e) => {
    const h = e.target.closest("[data-drag]");
    if (h && !e.target.closest("button")) {
      dragId = h.dataset.drag;
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragId);
    } else e.preventDefault();
  });
  app.addEventListener("dragover", (e) => {
    if (dragId && e.target.closest("[data-panel]")) e.preventDefault();
  });
  app.addEventListener("drop", (e) => {
    const tile = e.target.closest("[data-panel]");
    if (!tile || !dragId) return;
    e.preventDefault();
    const from = state.draft.panels.findIndex((p) => p.id === dragId),
      to = state.draft.panels.findIndex((p) => p.id === tile.dataset.panel);
    if (from >= 0 && to >= 0) {
      state.draft.panels.splice(to, 0, state.draft.panels.splice(from, 1)[0]);
      render();
      persist();
    }
    dragId = null;
  });
  app.addEventListener("dragend", () => (dragId = null));
  app.addEventListener("pointerdown", (e) => {
    const handle = e.target.closest("[data-resize]");
    if (!handle) return;
    e.preventDefault();
    const p = state.draft.panels.find((x) => x.id === handle.dataset.resize),
      grid = document.querySelector(".argus-grid");
    gesture = {
      p,
      x: e.clientX,
      y: e.clientY,
      w: p.w,
      h: p.h,
      unit: grid.clientWidth / 12,
      handle,
    };
    handle.setPointerCapture(e.pointerId);
  });
  app.addEventListener("pointermove", (e) => {
    if (!gesture) return;
    const g = gesture;
    g.p.w = Math.max(
      3,
      Math.min(12, g.w + Math.round((e.clientX - g.x) / g.unit)),
    );
    g.p.h = Math.max(260, g.h + Math.round((e.clientY - g.y) / 20) * 20);
    const tile = document.querySelector(`[data-panel="${g.p.id}"]`);
    tile.style.gridColumn = `span ${g.p.w}`;
    tile.style.height = `${g.p.h}px`;
  });
  app.addEventListener("pointerup", () => {
    if (gesture) {
      gesture = null;
      render();
      persist();
    }
  });
  app.addEventListener("pointercancel", () => {
    if (gesture) {
      gesture.p.w = gesture.w;
      gesture.p.h = gesture.h;
      gesture = null;
      render();
    }
  });
  window.addEventListener("beforeunload", persist);
  render();
  persist();
})();
