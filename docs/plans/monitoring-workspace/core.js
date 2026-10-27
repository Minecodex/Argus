(() => {
  const D = window.ArgusDemo;
  D.storageKey = "argus.monitoring-workspace.demo.v3";
  try {
    D.state = JSON.parse(localStorage.getItem(D.storageKey));
  } catch {
    /* private browsing */
  }
  if (!D.state || D.state.version !== 3) D.state = D.seed();
  D.ui = {
    page: "monitoring",
    category: "all",
    type: "all",
    search: "",
    app: "order",
    appTab: "overview",
    host: "host-a",
    hostTab: "overview",
    k8sTab: "resources",
    k8sKind: "Deployment",
    namespace: "all",
    logApp: "all",
    logLevel: "all",
    logQuery: "",
    traceApp: "all",
    refSearch: "",
    resourceKind: "all",
    dashboard: "application",
    filters: {
      app: "order",
      env: "production",
      kind: "all",
      service: "all",
      cluster: "all",
      namespace: "all",
      workload: "all",
      source: "all",
      time: "1h",
      dependencies: true,
      ids: null,
    },
  };
  D.t = (zh, en) => (D.state.locale === "en" ? (en ?? zh) : zh);
  D.e = (v) =>
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
  D.n = (o) => (o ? D.t(o.name, o.en || o.name) : "—");
  D.save = () => {
    try {
      localStorage.setItem(D.storageKey, JSON.stringify(D.state));
    } catch {
      D.toast(
        D.t(
          "本地存储不可用，当前会话内有效",
          "Local storage unavailable; changes remain in this session",
        ),
      );
    }
  };
  D.canManage = () => D.state.role === "admin";
  D.authorized = (r) =>
    D.canManage() || (r.kind === "application" && r.apps.includes("order"));
  D.visibleResources = () => D.state.resources.filter(D.authorized);
  D.visibleMonitors = () =>
    D.state.monitors.filter(
      (m) =>
        D.canManage() ||
        (m.apps.includes("order") && ["traces", "logs"].includes(m.signal)),
    );
  D.appName = (id) => D.n(D.state.apps.find((a) => a.id === id));
  D.serviceName = (id) =>
    D.state.services.find((s) => s.id === id)?.name || id || "—";
  D.resource = (id) => D.state.resources.find((r) => r.id === id);
  D.collectorName = (id) =>
    D.state.collectors.find((c) => c.id === id)?.name || "—";
  D.id = (prefix) =>
    `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
  D.envName = (id) =>
    ({
      production: D.t("生产", "Production"),
      staging: D.t("测试", "Staging"),
      all: D.t("全部环境", "All environments"),
    })[id] ||
    id ||
    D.t("未关联环境", "No environment association");
  D.kindName = (id) =>
    ({
      host: D.t("主机", "Host"),
      kubernetes: "Kubernetes",
      application: D.t("应用服务", "Application service"),
      database: D.t("数据库", "Database"),
      cache: D.t("缓存", "Cache"),
      queue: D.t("消息与协调", "Messaging"),
      web: D.t("Web 与代理", "Web & proxy"),
      runtime: D.t("运行时", "Runtime"),
      storage: D.t("对象存储", "Object storage"),
      search: D.t("搜索服务", "Search"),
      probe: D.t("服务探测", "Probe"),
    })[id] || id;
  const paths = {
    grid: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
    activity: "M2 12h4l3-8 6 16 3-8h4",
    box: "m12 3 9 5-9 5-9-5 9-5 M3 8v9l9 5 9-5V8 M12 13v9",
    host: "M3 4h18v6H3z M3 14h18v6H3z M6 7h.01 M6 17h.01",
    app: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 17h7 M17.5 14v7",
    logs: "M5 3h14v18H5z M8 7h8 M8 11h8 M8 15h5",
    trace: "M4 6h6v4H4z M14 14h6v4h-6z M7 10v6h7",
    book: "M3 4h7l2 2 2-2h7v16h-7l-2 2-2-2H3z M12 6v16",
    plus: "M12 5v14 M5 12h14",
    search: "M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
    arrow: "M4 12h16 M14 6l6 6-6 6",
    moon: "M20 15a9 9 0 0 1-11-11 9 9 0 1 0 11 11",
    sun: "M12 3v2 M12 19v2 M3 12h2 M19 12h2 M5.6 5.6l1.4 1.4 M17 17l1.4 1.4 M5.6 18.4 1.4-1.4 M17 7l1.4-1.4 M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
    close: "M6 6l12 12 M6 18 12-12",
    check: "M5 12l4 4L19 6",
    chevron: "m9 5 7 7-7 7",
    refresh: "M20 7v5h-5 M4 17v-5h5 M6 5a8 8 0 0 1 14 7 M18 19A8 8 0 0 1 4 12",
    external: "M14 3h7v7 M21 3 10 14 M10 3H4v17h17v-6",
    shield: "m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z",
    play: "m7 4 14 8-14 8z",
    key: "M14 14l-9 7-2-2 2-3 3 1 3-4 M21 7a5 5 0 1 1-10 0 5 5 0 0 1 10 0",
  };
  D.icon = (name) =>
    `<svg class="argus-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="${paths[name] || paths.box}"/></svg>`;
  D.btn = (action, label, opts = {}) =>
    `<button type="button" class="argus-btn ${opts.primary ? "argus-btn--primary" : ""} ${opts.quiet ? "argus-btn--quiet" : ""} ${opts.danger ? "argus-btn--danger" : ""}" data-action="${action}" ${opts.id ? `data-id="${D.e(opts.id)}"` : ""} ${opts.extra || ""} ${opts.disabled ? "disabled" : ""}>${opts.icon ? D.icon(opts.icon) : ""}${D.e(label)}</button>`;
  D.tag = (text, tone = "") =>
    `<span class="argus-tag ${tone ? "argus-tag--" + tone : ""}">${D.e(text)}</span>`;
  D.status = (status) => {
    const values = {
      healthy: [D.t("正常", "Healthy"), "success"],
      error: [D.t("认证失败", "Auth failed"), "danger"],
      paused: [D.t("已停采", "Paused"), ""],
      waiting: [D.t("等待数据", "Waiting for data"), "warning"],
      offline: [D.t("离线", "Offline"), "danger"],
      pending: [D.t("待应用", "Pending"), "warning"],
    };
    const [label, tone] = values[status] || [status, ""];
    return `<span class="argus-tag argus-tag--${tone}"><span class="argus-dot"></span>${label}</span>`;
  };
  D.toggle = (action, id, enabled, label, disabled = false) =>
    `<button type="button" class="argus-switch" role="switch" aria-checked="${Boolean(enabled)}" aria-label="${D.e(label)}" data-action="${action}" data-id="${D.e(id)}" ${disabled ? "disabled" : ""}></button>`;
  D.menu = (label, items) =>
    `<details class="argus-menu"><summary aria-label="${D.e(label)}">···</summary><div class="argus-menu-items">${items.map((i) => `<button type="button" data-action="${i[0]}" data-id="${D.e(i[2] || "")}" ${i[3] ? "disabled" : ""}>${D.e(i[1])}</button>`).join("")}</div></details>`;
  D.table = (headers, rows, foot = "") =>
    `<div class="argus-panel"><div class="argus-table-wrap"><table class="argus-table"><thead><tr>${headers.map((h) => `<th scope="col">${h || `<span class="argus-sr-only">${D.t("操作", "Actions")}</span>`}</th>`).join("")}</tr></thead><tbody>${rows.length ? rows.map((r) => `<tr>${r.map((c) => `<td>${c}</td>`).join("")}</tr>`).join("") : `<tr><td colspan="${headers.length}">${D.empty(D.t("没有匹配项", "No matches"), D.t("调整筛选条件或新建采集。", "Adjust filters or create a collection."))}</td></tr>`}</tbody></table></div>${foot ? `<div class="argus-table-foot">${foot}</div>` : ""}</div>`;
  D.empty = (title, detail) =>
    `<div class="argus-empty">${D.icon("search")}<strong>${title}</strong><span>${detail || ""}</span></div>`;
  D.note = (text, tone = "") =>
    `<div class="argus-note ${tone ? "argus-note--" + tone : ""}">${text}</div>`;
  D.title = (
    title,
    description,
    actions = "",
    eyebrow = "OBSERVABILITY WORKSPACE",
  ) =>
    `<div class="argus-heading"><div><div class="argus-eyebrow">${eyebrow}</div><h1 tabindex="-1">${title}</h1><p class="argus-subtitle">${description}</p></div><div class="argus-row">${actions}</div></div>`;
  D.stats = (items) =>
    `<div class="argus-stats">${items.map(([label, value, detail]) => `<div class="argus-stat"><span class="argus-muted">${label}</span><strong>${value}</strong><small>${detail}</small></div>`).join("")}</div>`;
  D.tabs = (items, value, action) =>
    `<div class="argus-tabs" role="tablist">${items.map(([id, zh, en]) => `<button class="argus-tab" role="tab" aria-selected="${value === id}" data-action="${action}" data-id="${id}">${D.t(zh, en)}</button>`).join("")}</div>`;
  D.options = (items, value) =>
    items
      .map(
        ([id, label]) =>
          `<option value="${D.e(id)}" ${String(value) === String(id) ? "selected" : ""}>${D.e(label)}</option>`,
      )
      .join("");
  D.select = (name, label, items, value, extra = "") =>
    `<label class="argus-field"><span>${label}</span><select name="${name}" ${extra}>${D.options(items, value)}</select></label>`;
  D.input = (name, label, value = "", extra = "", help = "") =>
    `<label class="argus-field"><span>${label}</span><input name="${name}" value="${D.e(value)}" ${extra}/>${help ? `<small>${help}</small>` : ""}</label>`;
  D.panel = (title, body, actions = "") =>
    `<section class="argus-panel"><div class="argus-panel-head"><h2>${title}</h2><div class="argus-row">${actions}</div></div><div class="argus-panel-body">${body}</div></section>`;
  D.kv = (entries) =>
    `<dl class="argus-kv">${entries.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join("")}</dl>`;
  D.appTags = (ids) =>
    ids?.length
      ? ids.map((id) => D.tag(D.appName(id))).join(" ")
      : `<span class="argus-muted">${D.t("未关联应用", "No application")}</span>`;
  D.toast = (text) => {
    const el = document.getElementById("toast");
    el.textContent = text;
    el.dataset.open = "true";
    clearTimeout(D.toastTimer);
    D.toastTimer = setTimeout(() => delete el.dataset.open, 4200);
  };
  D.audit = (title, en, detail) => {
    D.state.audit.unshift({
      time: new Date().toLocaleTimeString("en-GB"),
      title,
      en,
      detail,
    });
    D.save();
  };
  D.dialog = (title, body, footer = "", drawer = false) => {
    const el = document.getElementById(drawer ? "drawer" : "modal");
    el.innerHTML = `<div class="argus-dialog-head"><div><h2 id="${drawer ? "drawer" : "modal"}-title">${title}</h2><span class="argus-small argus-muted">${D.t("交互原型 · 示例数据", "Interactive prototype · sample data")}</span></div>${D.btn("close", D.t("关闭", "Close"), { quiet: true, icon: "close", extra: `data-dialog="${el.id}"` })}</div><div class="argus-dialog-body">${body}</div>${footer ? `<div class="argus-dialog-foot">${footer}</div>` : ""}`;
    if (!el.open) el.showModal();
    return el;
  };
  D.close = () => {
    document.querySelectorAll("dialog[open]").forEach((el) => el.close());
    D.pending = null;
  };
  D.confirm = (title, body, commit) => {
    if (!D.canManage())
      return D.toast(
        D.t("当前为应用只读角色", "Application viewer cannot change resources"),
      );
    D.pending = commit;
    D.dialog(
      title,
      body,
      D.btn("close", D.t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("confirm", D.t("确认模拟执行", "Confirm simulation"), {
          primary: true,
        }),
    );
  };
  D.navItems = [
    ["dashboards", "仪表盘", "Dashboards", "grid"],
    ["applications", "应用", "Applications", "app"],
    ["monitoring", "监控", "Monitoring", "activity"],
    ["resources", "资源目录", "Resources", "box"],
    ["hosts", "主机", "Hosts", "host"],
    ["kubernetes", "Kubernetes", "Kubernetes", "box"],
    ["logs", "日志探索", "Logs", "logs"],
    ["traces", "Trace 探索", "Traces", "trace"],
  ];
  D.navigate = (page, extra = {}) => {
    Object.assign(D.ui, extra, { page });
    location.hash = page;
    D.render();
    window.scrollTo(0, 0);
  };
  D.render = () => {
    const objectFilters = document.querySelector("[data-filter-details]");
    if (objectFilters) D.ui.k8sFiltersOpen = objectFilters.open;
    document.documentElement.dataset.theme = D.state.theme;
    document.documentElement.lang = D.state.locale === "en" ? "en" : "zh-CN";
    const item = D.navItems.find((i) => i[0] === D.ui.page);
    const title = item
      ? D.t(item[1], item[2])
      : D.ui.page === "references"
        ? D.t("仪表盘参考表", "Dashboard references")
        : D.t("功能与边界", "Scope & architecture");
    const content = D.pages?.[D.ui.page]?.() || "";
    document.getElementById("app").innerHTML =
      `<div class="argus-shell"><aside class="argus-sidebar"><div class="argus-brand"><div class="argus-brand-mark">◉</div><div><strong>Argus</strong><small>enterprise</small></div></div><div class="argus-nav-title">${D.t("可观测与资源", "OBSERVABILITY")}</div><nav class="argus-nav" aria-label="${D.t("主导航", "Main navigation")}">${D.navItems.map(([id, zh, en, icon]) => `<button data-action="navigate" data-id="${id}" ${D.ui.page === id ? 'aria-current="page"' : ""}>${D.icon(icon)}${D.t(zh, en)}${id === "monitoring" ? `<span class="argus-count">${D.visibleMonitors().length}</span>` : ""}</button>`).join("")}</nav><div class="argus-nav-title">${D.t("设计参考", "DESIGN REFERENCE")}</div><nav class="argus-nav"><button data-action="navigate" data-id="references" ${D.ui.page === "references" ? 'aria-current="page"' : ""}>${D.icon("book")}${D.t("Grafana 参考表", "Grafana references")}</button><button data-action="navigate" data-id="blueprint" ${D.ui.page === "blueprint" ? 'aria-current="page"' : ""}>${D.icon("shield")}${D.t("功能与边界", "Scope & architecture")}</button></nav><div class="argus-side-bottom"><div class="argus-row"><div class="argus-avatar">K</div><div><strong>kakj</strong><div class="argus-small argus-muted">${D.canManage() ? D.t("企业运维管理员", "Operations admin") : D.t("订单应用只读", "Order application viewer")}</div></div></div></div></aside><div class="argus-workspace"><header class="argus-topbar"><div class="argus-breadcrumb">${D.t("企业工作台", "Enterprise")} <span class="argus-muted"> / </span> <strong>${title}</strong></div>${D.tag(D.t("设计预览", "Design preview"), "accent")}${D.btn("role", D.canManage() ? D.t("切换只读视角", "Switch to viewer") : D.t("切换管理员", "Switch to admin"), { quiet: true, icon: "shield" })}${D.btn("locale", D.state.locale === "zh" ? "EN" : "中文", { quiet: true })}${D.btn("theme", D.t("主题", "Theme"), { quiet: true, icon: D.state.theme === "light" ? "moon" : "sun" })}</header><main id="main" class="argus-main">${content}</main><footer class="argus-demo-bar"><span class="argus-dot"></span><strong>${D.t("交互原型", "Interactive prototype")}</strong><span>${D.t("所有数据与操作均为模拟，仅保存在当前浏览器。", "All data and operations are simulated and stored only in this browser.")}</span><span class="argus-grow"></span>${D.btn("audit", D.t("操作记录", "Activity"), { quiet: true })}${D.btn("reset", D.t("重置示例", "Reset demo"), { quiet: true })}</footer></div></div>`;
  };
})();
