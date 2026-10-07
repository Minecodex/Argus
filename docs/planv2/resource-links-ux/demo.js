/* Standalone discussion prototype: example data only, no business API calls. */
(() => {
  "use strict";
  const KEY = "argus.planv2.resource-links-ux.v1";
  const app = document.getElementById("app"),
    menu = document.getElementById("menu");
  const drawer = document.getElementById("drawer"),
    modal = document.getElementById("modal");
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
  const saved = (() => {
    try {
      return JSON.parse(localStorage.getItem(KEY)) ?? {};
    } catch {
      return {};
    }
  })();
  const state = {
    lang: saved.lang === "en" ? "en" : "zh",
    theme: saved.theme === "dark" ? "dark" : "light",
    placement: "more",
    canManage: true,
    page: "dashboard",
    board: "service",
    resource: "host-a",
    tab: "dashboards",
    origin: null,
    scope: null,
    time: 3600,
    linkStatus: "ready",
    bindings: {
      service: ["host-a", "cluster-a"],
      host: ["host-a"],
      cluster: ["cluster-a"],
    },
  };
  const T = (zh, en) => (state.lang === "en" ? en : zh);
  const resources = [
    {
      id: "host-a",
      type: "host",
      name: "prod-web-01",
      detail: "10.42.7.18 · Ubuntu 24.04",
    },
    {
      id: "host-b",
      type: "host",
      name: "prod-web-02",
      detail: "10.42.7.19 · Ubuntu 24.04",
    },
    {
      id: "cluster-a",
      type: "cluster",
      name: "production-east",
      detail: "Kubernetes · 8 nodes",
    },
  ];
  const boards = [
    {
      id: "service",
      zh: "应用与主机概览",
      en: "Application & host overview",
      signals: "Metrics · Logs · Trace",
    },
    {
      id: "host",
      zh: "主机诊断",
      en: "Host diagnostics",
      signals: "Metrics · Logs",
    },
    {
      id: "cluster",
      zh: "集群概览",
      en: "Cluster overview",
      signals: "Metrics · Logs · Trace",
    },
  ];
  const resource = (id) => resources.find((r) => r.id === id);
  const boardName = (id) => {
    const b = boards.find((b) => b.id === id);
    return T(b.zh, b.en);
  };
  const kind = (r) => (r.type === "host" ? T("主机", "Host") : "Kubernetes");
  const paths = {
    grid: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>',
    host: '<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M11 6.5h6M11 17.5h6"/>',
    cluster:
      '<path d="m12 3 9 5v8l-9 5-9-5V8Z"/><path d="m3 8 9 5 9-5M12 13v8M7.5 5.5l9 5"/>',
    link: '<path d="m10 13 4-4M8 15l-1 1a4 4 0 0 1-6-6l4-4a4 4 0 0 1 6 0m2 3 1-1a4 4 0 1 1 6 6l-4 4a4 4 0 0 1-6 0"/>',
    more: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
    close: '<path d="m5 5 14 14M19 5 5 19"/>',
    chevron: '<path d="m6 9 6 6 6-6"/>',
    refresh:
      '<path d="M20 7v5h-5M4 17v-5h5"/><path d="M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1"/>',
    clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    back: '<path d="m10 5-7 7 7 7M3 12h18"/>',
    edit: '<path d="m15 4 5 5-10 10-6 1 1-6Z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M5 5l1 1M18 18l1 1M5 19l1-1M18 6l1-1"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7v1"/>',
    plus: '<path d="M12 4v16M4 12h16"/>',
  };
  const icon = (name) =>
    `<svg class="argus-icon" viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.grid}</svg>`;
  const btn = (action, label, symbol = "", style = "", attrs = "") =>
    `<button type="button" class="argus-btn ${style}" data-action="${action}" ${attrs}>${symbol ? icon(symbol) : ""}${label}</button>`;
  const badge = (text) => `<span class="argus-badge">${esc(text)}</span>`;
  const linked = () => state.bindings[state.board];
  const count = () =>
    state.linkStatus === "loading"
      ? "…"
      : state.linkStatus === "error"
        ? "—"
        : linked().length;
  const mainFocus = () => document.querySelector("h1")?.focus();
  let toastTimer,
    restoreAction = "",
    management = null,
    scopeDraft = null;
  const toast = (message) => {
    const e = document.getElementById("toast");
    e.textContent = message;
    e.classList.add("is-visible");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => e.classList.remove("is-visible"), 2200);
  };
  const closeAll = () => {
    menu.hidePopover();
    drawer.close();
    modal.close();
  };
  function openBoard(id, origin = null) {
    closeAll();
    state.page = "dashboard";
    state.board = id;
    state.origin = origin;
    state.scope = origin ? [origin] : null;
    state.time = 3600;
    render();
    mainFocus();
  }
  function openResource(id) {
    closeAll();
    state.page = "resource";
    state.resource = id;
    state.tab = "dashboards";
    render();
    mainFocus();
  }
  function openDialog(surface, title, description, body, footer = "") {
    restoreAction = document.activeElement?.dataset.action ?? "";
    if (
      ["linked", "links-ready"].includes(restoreAction) &&
      state.placement === "more"
    )
      restoreAction = "more";
    const id = surface === drawer ? "drawer-title" : "modal-title";
    surface.innerHTML = `<div class="argus-dialog-heading"><div><h2 id="${id}">${esc(title)}</h2>${description ? `<p>${esc(description)}</p>` : ""}</div>${btn(surface === drawer ? "close-drawer" : "close-modal", "", "close", "argus-btn--icon", `aria-label="${T("关闭", "Close")}"`)}</div><div class="argus-dialog-body">${body}</div>${footer ? `<div class="argus-dialog-footer">${footer}</div>` : ""}`;
    surface.showModal();
  }
  [drawer, modal].forEach((surface) =>
    surface.addEventListener("close", () => {
      const trigger =
        restoreAction &&
        document.querySelector(`[data-action="${restoreAction}"]`);
      (trigger || document.querySelector('[data-action="more"]'))?.focus();
    }),
  );
  function showMenu() {
    menu.innerHTML = `${state.placement === "more" ? `<button type="button" role="menuitem" data-action="linked">${icon("link")}${T("关联资源", "Linked resources")}${badge(count())}</button>` : ""}<button type="button" role="menuitem" data-action="board-info">${icon("info")}${T("仪表盘信息", "Dashboard information")}</button>`;
    const anchor = document.querySelector('[data-action="more"]'),
      rect = anchor.getBoundingClientRect();
    menu.showPopover();
    const tokens = getComputedStyle(document.documentElement),
      gap = parseFloat(tokens.getPropertyValue("--action-gap")),
      pad = parseFloat(tokens.getPropertyValue("--page-padding")),
      width = menu.offsetWidth;
    menu.style.setProperty("--argus-popup-top", `${rect.bottom + gap}px`);
    menu.style.setProperty(
      "--argus-popup-left",
      `${Math.max(pad, Math.min(rect.right - width, innerWidth - width - pad))}px`,
    );
    menu.querySelector("button")?.focus();
  }
  menu.addEventListener("keydown", (e) => {
    const items = [...menu.querySelectorAll("button")],
      at = items.indexOf(document.activeElement);
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
      e.preventDefault();
      items[
        e.key === "Home"
          ? 0
          : e.key === "End"
            ? items.length - 1
            : (at + (e.key === "ArrowDown" ? 1 : -1) + items.length) %
              items.length
      ].focus();
    }
  });
  menu.addEventListener("toggle", (e) => {
    if (e.newState === "closed" && !drawer.open && !modal.open)
      document.querySelector('[data-action="more"]')?.focus();
  });
  function showLinks() {
    menu.hidePopover();
    const title = T("关联资源", "Linked resources");
    let body;
    if (state.linkStatus === "loading")
      body = `<p role="status">${T("正在读取关联资源…", "Loading linked resources…")}</p>${btn("links-ready", T("完成加载", "Finish loading"))}`;
    else if (state.linkStatus === "error")
      body = `<p class="argus-alert" role="alert">${T("关联资源暂时无法加载，请重试。", "Linked resources could not be loaded. Retry to continue.")}</p>${btn("links-ready", T("重试", "Retry"))}`;
    else
      body = linked().length
        ? linked()
            .map((id) => {
              const r = resource(id);
              return `<div class="argus-binding-row"><span class="argus-resource-icon">${icon(r.type)}</span><div class="argus-row-text"><strong>${esc(r.name)}</strong><small>${kind(r)} · ${T("生产环境", "Production")}</small></div>${btn("open-resource", T("打开资源", "Open resource"), "", "argus-btn--small", `data-id="${id}"`)}</div>`;
            })
            .join("")
        : `<div class="argus-empty">${icon("link")}<strong>${T("暂无关联资源", "No linked resources")}</strong><span>${T("仍可从仪表盘目录打开。", "This dashboard remains available in the directory.")}</span></div>`;
    body += `<p class="argus-note">${T("这里展示哪些资源页面可以快捷打开此仪表盘。绑定在主机或 Kubernetes 页面管理；查询范围由顶部筛选和当前权限决定。", "These resource pages provide shortcuts to this dashboard. Manage bindings on the host or Kubernetes page. Filters and current permissions determine the query scope.")}</p>`;
    openDialog(
      drawer,
      title,
      T("查看快捷入口及对应资源", "View shortcuts and their resources"),
      body,
      btn("list-hosts", T("浏览主机", "Browse hosts")) +
        btn("list-clusters", T("浏览 Kubernetes", "Browse Kubernetes")),
    );
  }
  function chart() {
    const hosts = resources.filter(
      (r) => r.type === "host" && (!state.scope || state.scope.includes(r.id)),
    );
    if (!hosts.length)
      return `<div class="argus-empty">${icon("host")}<strong>${T("此图适用于主机", "This panel applies to hosts")}</strong><span>${T("当前筛选为集群，其他适用统计图仍可查看。", "A cluster is selected. Other applicable panels remain available.")}</span></div>`;
    return `<svg class="argus-chart" viewBox="0 0 600 180" role="img" aria-label="${T("主机 CPU 趋势示例", "Example host CPU trend")}"><path class="argus-grid-line" d="M0 30h600M0 75h600M0 120h600M0 165h600"/><path class="argus-chart-fill" d="M0 125 35 115 70 123 105 92 140 98 175 80 210 87 245 65 280 79 315 64 350 76 385 52 420 67 455 51 490 73 525 62 560 82 600 58V180H0Z"/><path class="argus-chart-line" d="M0 125 35 115 70 123 105 92 140 98 175 80 210 87 245 65 280 79 315 64 350 76 385 52 420 67 455 51 490 73 525 62 560 82 600 58"/>${hosts.length > 1 ? '<path class="argus-chart-secondary" d="M0 145 35 131 70 144 105 118 140 134 175 112 210 122 245 105 280 117 315 95 350 106 385 84 420 98 455 87 490 109 525 93 560 113 600 91"/>' : ""}</svg><div class="argus-legend">${hosts.map((r, i) => `<span class="argus-row"><span class="argus-dot ${i ? "argus-dot--second" : ""}"></span>${esc(r.name)}</span>`).join("")}</div>`;
  }
  function dashboard() {
    const chosen = state.scope ? state.scope.map(resource) : resources;
    const scopeLabel = !state.scope
      ? T("全部", "All")
      : chosen.length === 1
        ? chosen[0].name
        : T(`${chosen.length} 项`, `${chosen.length} selected`);
    const source = state.origin ? resource(state.origin) : null;
    const hours = state.time / 3600;
    return `<div class="argus-page-heading"><div><h1 tabindex="-1">${esc(boardName(state.board))}</h1><div class="argus-page-description">${badge(T("已发布", "Published"))}<span>R1</span>${source ? `<span>· ${T("来自", "Opened from")}${kind(source)}</span><button class="argus-link" type="button" data-action="open-resource" data-id="${source.id}">${esc(source.name)}${icon("back")}</button>` : `<span>· ${T("基础设施", "Infrastructure")}</span>`}</div></div><div class="argus-row">${btn("catalog", T("返回目录", "Back to directory"))}${state.placement === "header" ? btn("linked", `${T("关联资源", "Linked resources")} ${count()}`, "link") : ""}${btn("more", "", "more", "argus-btn--icon", `aria-label="${T("更多", "More")}" aria-haspopup="menu"`)}</div></div>
      <div class="argus-controls" role="group" aria-label="${T("查询条件", "Query conditions")}">${btn("time", T(`最近 ${hours} 小时`, `Last ${hours} hour${hours > 1 ? "s" : ""}`), "clock", "argus-btn--quiet")}${btn("scope", `${T("资源范围", "Resource scope")}: ${esc(scopeLabel)} ${icon("chevron")}`, "", "argus-btn--quiet")}${btn("refresh", T("刷新", "Refresh"), "refresh")}<span class="argus-badge">${T("自动刷新关闭", "Auto refresh off")}</span></div>
      <div class="argus-chart-grid">
        <section class="argus-panel argus-panel--stat"><div class="argus-panel-header"><strong>${T("当前资源", "Selected resources")}</strong>${badge(T("示例", "Example"))}</div><div class="argus-panel-body"><div class="argus-stat">${chosen.length} <small>${T("个", "resources")}</small></div><p class="argus-muted argus-small">${T("仅使用当前授权与筛选范围", "Uses current permissions and filters")}</p></div></section>
        <section class="argus-panel argus-panel--stat"><div class="argus-panel-header"><strong>${T("请求速率", "Request rate")}</strong>${badge(T("示例", "Example"))}</div><div class="argus-panel-body"><div class="argus-stat">${state.scope ? "128" : "386"} <small>req/s</small></div><p class="argus-muted argus-small">${T("应用指标 · 当前时间区间", "Application metric · current interval")}</p></div></section>
        <section class="argus-panel argus-panel--stat"><div class="argus-panel-header"><strong>${T("P95 响应时间", "P95 response time")}</strong>${badge(T("示例", "Example"))}</div><div class="argus-panel-body"><div class="argus-stat">${state.scope ? "42" : "58"} <small>ms</small></div><p class="argus-muted argus-small">${T("应用分桶指标", "Application histogram metric")}</p></div></section>
        <section class="argus-panel"><div class="argus-panel-header"><strong>${T("主机 CPU 使用率", "Host CPU utilization")}</strong>${badge(T("示例", "Example"))}</div><div class="argus-panel-body">${chart()}</div><div class="argus-panel-footer">${T("只显示适用主机，来源保持独立", "Applicable hosts only; sources remain separate")}</div></section>
        <section class="argus-panel"><div class="argus-panel-header"><strong>${T("服务请求趋势", "Service request trend")}</strong>${badge(T("示例", "Example"))}</div><div class="argus-panel-body"><svg class="argus-chart" viewBox="0 0 600 180" role="img" aria-label="${T("服务请求趋势示例", "Example request trend")}"><path class="argus-grid-line" d="M0 30h600M0 75h600M0 120h600M0 165h600"/><path class="argus-chart-secondary" d="M0 150 30 148 60 130 90 138 120 125 150 118 180 128 210 96 240 102 270 87 300 94 330 68 360 80 390 60 420 72 450 40 480 51 510 41 540 54 570 38 600 44"/></svg><div class="argus-legend">${T("当前范围 · checkout-api", "Current scope · checkout-api")}</div></div><div class="argus-panel-footer">${T("绑定入口不改变统计图查询定义", "Entry bindings do not change panel queries")}</div></section>
      </div>`;
  }
  const card = (b, origin = "") =>
    `<article class="argus-card"><div class="argus-row">${icon("grid")}<h2>${esc(boardName(b.id))}</h2></div><p>${b.signals}</p><div class="argus-card-footer">${badge(T("已发布 · R1", "Published · R1"))}${btn("open-board", T("打开仪表盘", "Open dashboard"), "", "argus-btn--quiet", `data-id="${b.id}" data-origin="${origin}"`)}</div></article>`;
  function resourcePage() {
    const r = resource(state.resource),
      bound = boards.filter((b) => state.bindings[b.id].includes(r.id));
    return `<div class="argus-page-heading"><div><h1 tabindex="-1">${esc(r.name)}</h1><div class="argus-page-description">${badge(kind(r))}<span>${esc(r.detail)}</span></div></div>${btn("dashboard", T("返回仪表盘", "Back to dashboard"), "back")}</div>
      <div class="argus-tabs" role="tablist" aria-label="${T("资源详情", "Resource details")}">${["overview", "dashboards"].map((tab) => `<button type="button" role="tab" data-action="tab" data-id="${tab}" aria-selected="${state.tab === tab}" aria-controls="resource-content" id="tab-${tab}">${tab === "overview" ? T("概览", "Overview") : T("仪表盘", "Dashboards")}</button>`).join("")}</div>
      <section id="resource-content" role="tabpanel" aria-labelledby="tab-${state.tab}">${state.tab === "overview" ? `<article class="argus-card"><h2>${T("资源概览", "Resource overview")}</h2><p>${T("从“仪表盘”页签查看与管理快捷入口。", "View and manage shortcuts in the Dashboards tab.")}</p>${btn("tab", T("查看关联仪表盘", "View linked dashboards"), "grid", "", 'data-id="dashboards"')}</article>` : `<div class="argus-section-heading"><h2>${T("关联仪表盘", "Linked dashboards")} ${badge(bound.length)}</h2>${state.canManage ? btn("manage", T("管理关联", "Manage links"), "edit") : badge(T("只读", "Read only"))}</div><p class="argus-muted argus-small">${T("打开时预选当前资源；你仍可切换到其他已授权资源。", "Opening preselects this resource. You can still switch to other authorized resources.")}</p><div class="argus-cards">${bound.map((b) => card(b, r.id)).join("")}</div>${!bound.length ? `<div class="argus-empty">${icon("grid")}<strong>${T("尚未关联仪表盘", "No dashboards linked yet")}</strong><span>${T("关联后可从这里快速进入。", "Linked dashboards will appear here as shortcuts.")}</span></div>` : ""}`}</section>`;
  }
  function render() {
    document.documentElement.dataset.theme = state.theme;
    document.documentElement.lang = state.lang === "zh" ? "zh-CN" : "en";
    localStorage.setItem(
      KEY,
      JSON.stringify({ lang: state.lang, theme: state.theme }),
    );
    const current = state.page === "resource" ? resource(state.resource) : null;
    const active =
      state.page === "catalog" || state.page === "dashboard"
        ? "catalog"
        : current?.type === "host"
          ? "list-hosts"
          : "list-clusters";
    const body =
      state.page === "dashboard"
        ? dashboard()
        : state.page === "resource"
          ? resourcePage()
          : state.page === "catalog"
            ? `<div class="argus-page-heading"><h1 tabindex="-1">${T("仪表盘", "Dashboards")}</h1>${badge(T("示例目录", "Example directory"))}</div><div class="argus-cards">${boards.map((b) => card(b)).join("")}</div>`
            : `<div class="argus-page-heading"><h1 tabindex="-1">${state.page === "hosts" ? T("主机", "Hosts") : "Kubernetes"}</h1></div><div class="argus-cards">${resources
                .filter((r) => (state.page === "hosts") === (r.type === "host"))
                .map(
                  (r) =>
                    `<article class="argus-card"><div class="argus-row">${icon(r.type)}<h2>${esc(r.name)}</h2></div><p>${esc(r.detail)}</p>${btn("open-resource", T("打开资源", "Open resource"), "", "", `data-id="${r.id}"`)}</article>`,
                )
                .join("")}</div>`;
      ["catalog", T("仪表盘", "Dashboards"), "grid", 3],
      ["list-hosts", T("主机", "Hosts"), "host", 2],
      ["list-clusters", "Kubernetes", "cluster", 1],
    ]
      .map(
        ([action, label, symbol, total]) =>
          `<button type="button" class="${active === action ? "is-active" : ""}" data-action="${action}">${icon(symbol)}${label}<span class="argus-count">${total}</span></button>`,
      )
      .join(
        "",
      )}</nav></div><div class="argus-note">${T("关联负责快捷导航。权限、当前资源筛选与关联状态各自独立。", "Links provide shortcuts. Permissions, current filters and bindings remain separate.")}</div></aside><div class="argus-workspace"><header class="argus-topbar"><div class="argus-breadcrumb">${icon(current?.type ?? "grid")}<span>${current ? kind(current) : T("仪表盘", "Dashboards")}</span><span> / </span><span>${current ? esc(current.name) : T("基础设施", "Infrastructure")}</span></div><div class="argus-row">${btn("theme", "", "sun", "argus-btn--icon", `aria-label="${T("切换主题", "Toggle theme")}"`)}${btn("language", state.lang === "zh" ? "EN" : "中文", "", "argus-btn--small", `aria-label="${T("切换语言", "Toggle language")}"`)}<span class="argus-avatar">A</span><span class="argus-small">${T("预览用户", "Preview user")}</span></div></header><main class="argus-main">${body}</main></div></div><footer class="argus-demo-bar"><div class="argus-row"><strong>${T("交互提案 · 示例数据", "Interaction proposal · example data")}</strong><span class="argus-small argus-muted">${T("交互原型", "Interactive prototype")}</span></div><div class="argus-row" role="group" aria-label="${T("原型场景", "Prototype scenarios")}"><span class="argus-small argus-muted">${T("入口位置", "Placement")}</span>${btn("placement", T("更多菜单（推荐）", "More menu (recommended)"), "", "argus-btn--small", `data-id="more" aria-pressed="${state.placement === "more"}"`)}${btn("placement", T("标题旁按钮", "Header button"), "", "argus-btn--small", `data-id="header" aria-pressed="${state.placement === "header"}"`)}${btn("role", state.canManage ? T("资源管理员", "Resource manager") : T("只读用户", "Read-only user"), "", "argus-btn--small")}${btn("empty", T("无关联", "No links"), "", "argus-btn--small")}${btn("error", T("加载失败", "Load failure"), "", "argus-btn--small")}${btn("reset", T("重置", "Reset"), "refresh", "argus-btn--small")}</div></footer>`;
  }
  function manage() {
    if (!state.canManage) return;
    const r = resource(state.resource);
    management = {
      resource: r.id,
      before: boards
        .filter((b) => state.bindings[b.id].includes(r.id))
        .map((b) => b.id),
      after: null,
    };
    management.after = [...management.before];
    const body = `<p class="argus-note">${T("在资源页逐项设置快捷入口，操作后先预览再确认。关联不会修改查询或授予数据权限。", "Manage individual shortcuts on the resource page, then preview and confirm. Links do not edit queries or grant access.")}</p>${boards
      .map((b) => {
        const exists = management.before.includes(b.id);
        return `<div class="argus-binding-row"><span class="argus-resource-icon">${icon("grid")}</span><div class="argus-row-text"><strong>${esc(boardName(b.id))}</strong><small>${b.signals} · ${exists ? T("已关联", "Linked") : T("未关联", "Not linked")}</small></div>${btn("select-binding", exists ? T("解除关联", "Unlink") : T("关联", "Link"), "", "argus-btn--small", `data-id="${b.id}" aria-label="${exists ? T("解除关联", "Unlink") : T("关联", "Link")} ${esc(boardName(b.id))}"`)}</div>`;
      })
      .join("")}`;
    openDialog(
      modal,
      T("管理仪表盘关联", "Manage dashboard links"),
      `${kind(r)} · ${r.name}`,
      body,
      btn("close-modal", T("关闭", "Close")),
    );
  }
  function previewBindings() {
    if (!management || !state.canManage) return;
    const added = management.after.filter(
        (id) => !management.before.includes(id),
      ),
      removed = management.before.filter(
        (id) => !management.after.includes(id),
      );
    if (added.length + removed.length !== 1) return;
    modal.close();
    const body =
      [
        ...added.map(
          (id) => `<p>${badge(T("新增", "Add"))} ${esc(boardName(id))}</p>`,
        ),
        ...removed.map(
          (id) => `<p>${badge(T("解除", "Remove"))} ${esc(boardName(id))}</p>`,
        ),
      ].join("") +
      `<p class="argus-note">${T("确认后更新该资源的快捷入口。已发布仪表盘、查询范围及权限保持原有规则。", "Confirmation updates this resource's shortcuts. Published dashboards, query scope and permissions retain their rules.")}</p>`;
    openDialog(
      modal,
      T("关联变更预览", "Review link changes"),
      resource(management.resource).name,
      body,
      btn("close-modal", T("取消", "Cancel")) +
        btn(
          "confirm-bindings",
          T("确认变更", "Confirm changes"),
          "",
          "argus-btn--primary",
        ),
    );
  }
  document.addEventListener("change", (e) => {
    if (e.target.dataset.scope && scopeDraft) {
      if (e.target.dataset.scope === "all") {
        scopeDraft.all = e.target.checked;
        scopeDraft.values = [];
      } else {
        scopeDraft.all = false;
        scopeDraft.values = [...modal.querySelectorAll("[data-scope]:checked")]
          .map((el) => el.dataset.scope)
          .filter((id) => id !== "all");
      }
      modal.querySelector('[data-scope="all"]').checked = scopeDraft.all;
      if (scopeDraft.all)
        modal
          .querySelectorAll('[data-scope]:not([data-scope="all"])')
          .forEach((el) => {
            el.checked = false;
          });
      modal.querySelector('[data-action="apply-scope"]').disabled =
        !scopeDraft.all && !scopeDraft.values.length;
    }
  });
  document.addEventListener("click", (e) => {
    const target = e.target.closest("[data-action]");
    if (!target) return;
    const { action, id, origin } = target.dataset;
    if (action === "more") return showMenu();
    if (action === "linked") return showLinks();
    if (action === "open-resource") return openResource(id);
    if (action === "open-board") return openBoard(id, origin || null);
    if (action === "dashboard") {
      closeAll();
      state.page = "dashboard";
      render();
      return mainFocus();
    }
    if (
      action === "catalog" ||
      action === "list-hosts" ||
      action === "list-clusters"
    ) {
      closeAll();
      state.page =
        action === "catalog"
          ? "catalog"
          : action === "list-hosts"
            ? "hosts"
            : "clusters";
      render();
      return mainFocus();
    }
    if (action === "close-drawer") return drawer.close();
    if (action === "close-modal") return modal.close();
    if (action === "tab") {
      state.tab = id;
      return render();
    }
    if (action === "manage") return manage();
    if (action === "select-binding" && management && state.canManage) {
      management.after = management.before.includes(id)
        ? management.before.filter((board) => board !== id)
        : [...management.before, id];
      return previewBindings();
    }
    if (action === "confirm-bindings" && management && state.canManage) {
      for (const b of boards)
        state.bindings[b.id] = [
          ...state.bindings[b.id].filter((id) => id !== management.resource),
          ...(management.after.includes(b.id) ? [management.resource] : []),
        ];
      modal.close();
      render();
      return toast(
        T("快捷入口已更新（本地示例）", "Shortcuts updated (local example)"),
      );
    }
    if (action === "scope") {
      scopeDraft = { all: state.scope === null, values: state.scope ?? [] };
      const body = `<p class="argus-note">${T("这是本次查询的资源范围。此处包含全部已授权资源，其中也有尚未绑定此仪表盘的主机。", "This is the current query scope. It includes all authorized resources, including a host not linked to this dashboard.")}</p><label class="argus-choice"><input type="checkbox" data-scope="all" ${scopeDraft.all ? "checked" : ""}/><strong>${T("全部授权资源", "All authorized resources")}</strong></label>${resources.map((r) => `<label class="argus-choice"><input type="checkbox" data-scope="${r.id}" ${scopeDraft.values.includes(r.id) ? "checked" : ""}/><span><strong>${esc(r.name)}</strong><small>${kind(r)}</small></span></label>`).join("")}`;
      return openDialog(
        modal,
        T("资源范围", "Resource scope"),
        "",
        body,
        btn("close-modal", T("取消", "Cancel")) +
          btn("apply-scope", T("应用", "Apply"), "", "argus-btn--primary"),
      );
    }
    if (
      action === "apply-scope" &&
      scopeDraft &&
      (scopeDraft.all || scopeDraft.values.length)
    ) {
      state.scope = scopeDraft.all ? null : scopeDraft.values;
      modal.close();
      return render();
    }
    if (action === "time")
      return openDialog(
        modal,
        T("时间范围", "Time range"),
        "",
        [3600, 21600, 86400]
          .map((seconds) =>
            btn(
              "apply-time",
              T(`最近 ${seconds / 3600} 小时`, `Last ${seconds / 3600} hours`),
              "clock",
              "",
              `data-id="${seconds}"`,
            ),
          )
          .join(""),
      );
    if (action === "apply-time") {
      state.time = Number(id);
      modal.close();
      return render();
    }
    if (action === "refresh")
      return toast(T("示例视图已刷新", "Example view refreshed"));
    if (action === "links-ready") {
      drawer.close();
      state.linkStatus = "ready";
      render();
      return showLinks();
    }
    if (action === "board-info") {
      menu.hidePopover();
      return openDialog(
        modal,
        T("仪表盘信息", "Dashboard information"),
        "",
        `<strong>${esc(boardName(state.board))}</strong><span class="argus-muted">R1 · ${T("示例已发布版本", "Example published revision")}</span><p class="argus-note">${T("时间和资源筛选控制当前查看条件；快捷入口只描述从哪里打开。", "Time and resource filters control this view. Shortcuts describe where it can be opened.")}</p>`,
      );
    }
    if (action === "placement") {
      state.placement = id;
      return render();
    }
    if (action === "theme") {
      state.theme = state.theme === "light" ? "dark" : "light";
      return render();
    }
    if (action === "language") {
      closeAll();
      state.lang = state.lang === "zh" ? "en" : "zh";
      return render();
    }
    if (action === "role") {
      closeAll();
      state.canManage = !state.canManage;
      render();
      return toast(
        T(
          state.canManage ? "资源管理入口已显示" : "只保留查看与导航",
          state.canManage
            ? "Resource management actions shown"
            : "Viewing and navigation only",
        ),
      );
    }
    if (action === "empty") {
      closeAll();
      state.bindings[state.board] = [];
      state.linkStatus = "ready";
      state.page = "dashboard";
      return render();
    }
    if (action === "error") {
      closeAll();
      state.linkStatus = "error";
      state.page = "dashboard";
      return render();
    }
    if (action === "reset") {
      closeAll();
      state.bindings = {
        service: ["host-a", "cluster-a"],
        host: ["host-a"],
        cluster: ["cluster-a"],
      };
      state.canManage = true;
      state.linkStatus = "ready";
      return openBoard("service");
    }
  });
  render();
})();

