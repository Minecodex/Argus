(() => {
  const D = window.ArgusDemo,
    t = (a, b) => D.t(a, b),
    e = D.e;
  D.chart = (seed = 1, unit = "", series = null) => {
    const values =
      series ||
      Array.from(
        { length: 24 },
        (_, i) =>
          24 + ((i * 7 + seed * 3) % 31) + Math.sin(i * 0.8 + seed) * 12,
      );
    const max = unit === "%" ? 100 : Math.max(...values, 1) * 1.25,
      points = values.map(
        (v, i) =>
          `${40 + (i * 520) / Math.max(1, values.length - 1)},${135 - (v / max) * 105}`,
      );
    return `<svg class="argus-chart" viewBox="0 0 590 164" role="img" aria-label="${t("示例时间序列", "Sample time series")}">${[30, 65, 100, 135].map((y, i) => `<path class="argus-chart-grid" d="M40 ${y}H565"/><text x="2" y="${y + 4}">${Math.round((max * (3 - i)) / 3)}</text>`).join("")}<path class="argus-chart-area" d="M40 135 L${points.join(" L")} L560 135 Z"/><path class="argus-chart-line" d="M${points.join(" L")}"/><text x="40" y="159">−${e(D.ui.filters.time)}</text><text x="510" y="159">${t("现在", "Now")} ${e(unit)}</text></svg>`;
  };
  D.scopeResources = (ignoreIds = false) => {
    const f = D.ui.filters;
    return D.visibleResources().filter((r) => {
      if (f.env !== "all" && !D.environmentIds(r, f.app).includes(f.env))
        return false;
      if (
        f.app !== "all" &&
        (!r.apps.includes(f.app) ||
          (r.kind !== "application" && !f.dependencies))
      )
        return false;
      if (f.kind !== "all" && r.kind !== f.kind) return false;
      if (
        f.service !== "all" &&
        r.service !== f.service &&
        !(f.dependencies && r.services?.includes(f.service))
      )
        return false;
      if (f.cluster !== "all" && r.cluster !== f.cluster && r.id !== f.cluster)
        return false;
      if (
        f.namespace !== "all" &&
        r.namespace !== f.namespace &&
        !(
          r.kind === "kubernetes" &&
          D.state.workloads.some(
            (w) =>
              w.namespace === f.namespace &&
              (f.workload === "all" || w.name === f.workload),
          )
        )
      )
        return false;
      if (
        f.workload !== "all" &&
        r.workload !== f.workload &&
        !(
          r.kind === "kubernetes" &&
          D.state.workloads.some(
            (w) =>
              w.name === f.workload &&
              (f.namespace === "all" || w.namespace === f.namespace),
          )
        )
      )
        return false;
      if (!ignoreIds && f.ids !== null && !f.ids.includes(r.id)) return false;
      if (
        f.source !== "all" &&
        !D.state.monitors.some((m) => m.target === r.id && m.type === f.source)
      )
        return false;
      return true;
    });
  };
  const baseValues = {
    "svc-order": 240,
    "svc-worker": 60,
    "svc-billing": 110,
    "svc-staging": 12,
    "db-order": 34,
    "db-staging": 8,
    "redis-order": 98,
    "kafka-order": 83,
    "nginx-edge": 320,
    "host-a": 24,
    "host-b": 37,
    "host-win": 18,
    "cluster-a": 44,
  };
  D.dashboardPanels = () => {
    if (D.ui.component) {
      const specs = {
        apiserver: [
          ["API 请求速率", "API request rate", "req/s"],
          ["API 请求时延 p95", "API request latency p95", "ms"],
        ],
        scheduler: [
          ["调度队列", "Scheduling queue", "pods"],
          ["调度时延", "Scheduling latency", "ms"],
        ],
        controller: [
          ["工作队列深度", "Work queue depth", "items"],
          ["控制器处理速率", "Controller processing rate", "items/s"],
        ],
        coredns: [
          ["DNS 请求速率", "DNS request rate", "req/s"],
          ["DNS 请求时延", "DNS latency", "ms"],
        ],
        etcd: [
          ["etcd 提交时延", "etcd commit latency", "ms"],
          ["etcd 存储容量", "etcd storage", "MiB"],
        ],
        collector: [
          ["接收样本速率", "Received sample rate", "samples/s"],
          ["导出队列", "Export queue", "items"],
        ],
        kubelet: [
          ["节点 CPU 使用率", "Node CPU utilization", "%"],
          ["容器内存使用", "Container memory usage", "GiB"],
        ],
        "cluster-state": [
          ["工作负载就绪副本", "Workload ready replicas", "replicas"],
          ["节点就绪数", "Ready nodes", "nodes"],
        ],
      };
      return (specs[D.ui.component] || specs.kubelet).map(
        ([zh, en, unit], i) => ({
          id: `component-${i}`,
          zh,
          en,
          unit,
          kinds: ["kubernetes"],
          scale: i ? 0.7 : 1,
          caption: t(
            "组件示例指标 · 需按实际指标契约适配",
            "Sample component metric · adapt to the actual metric contract",
          ),
        }),
      );
    }
    const all = [
      {
        id: "latency",
        zh: "服务请求样本时延",
        en: "Service request sample latency",
        kinds: ["application"],
        unit: "ms",
        scale: 0.55,
        caption: t("Trace 入口样本 · p95", "Received entry spans · p95"),
      },
      {
        id: "traffic",
        zh: "请求样本速率",
        en: "Received request sample rate",
        kinds: ["application"],
        unit: "samples/s",
        scale: 1,
        caption: t(
          "已接收样本，不推算全量",
          "Received samples; no total-volume extrapolation",
        ),
      },
      {
        id: "database",
        zh: "数据库活动连接",
        en: "Active database connections",
        kinds: ["database"],
        unit: "connections",
        scale: 1,
        caption: t(
          "共享实例状态 · 非应用独占用量",
          "Shared instance state · not exclusive application usage",
        ),
      },
      {
        id: "cache",
        zh: "缓存命中率",
        en: "Cache hit ratio",
        kinds: ["cache"],
        unit: "%",
        scale: 1,
        caption: t("目标资源指标", "Monitored target metrics"),
      },
      {
        id: "host",
        zh: "主机 CPU 使用率",
        en: "Host CPU utilization",
        kinds: ["host"],
        unit: "%",
        scale: 1,
        caption: t(
          "被监控主机 · 不按执行 Collector 扩大",
          "Monitored hosts · executor does not expand scope",
        ),
      },
      {
        id: "kubernetes",
        zh: "集群 CPU 使用率",
        en: "Cluster CPU utilization",
        kinds: ["kubernetes"],
        unit: "%",
        scale: 1,
        caption: t("集群及组件指标", "Cluster and component metrics"),
      },
      {
        id: "queue",
        zh: "消费组积压",
        en: "Consumer group lag",
        kinds: ["queue"],
        unit: "messages",
        scale: 1,
        caption: t("消息队列实例", "Messaging resource"),
      },
      {
        id: "web",
        zh: "Web 请求速率",
        en: "Web request rate",
        kinds: ["web"],
        unit: "req/s",
        scale: 1,
        caption: t("服务状态接口", "Service status endpoint"),
      },
    ];
    const ids = {
      application: ["latency", "traffic", "database", "cache"],
      infrastructure: ["host", "kubernetes", "database", "queue"],
      database: ["database", "cache"],
      kubernetes: ["kubernetes", "host"],
      host: ["host"],
      mixed: all.map((p) => p.id),
    };
    return all.filter((p) => (ids[D.ui.dashboard] || ids.mixed).includes(p.id));
  };
  D.dashboardView = () => {
    const f = D.ui.filters,
      scope = D.scopeResources(),
      candidates = D.scopeResources(true);
    const appOptions = [
      ["all", t("全部已授权应用", "All authorized applications")],
      ...D.state.apps
        .filter((a) => D.canManage() || a.id === "order")
        .map((a) => [a.id, D.n(a)]),
    ];
    const kindOptions = [
      ["all", t("全部资源类型", "All resource types")],
      ...[
        "application",
        "database",
        "cache",
        "queue",
        "web",
        "runtime",
        "storage",
        "search",
        "probe",
        "host",
        "kubernetes",
      ].map((id) => [id, D.kindName(id)]),
    ];
    const services = D.state.services.filter(
      (s) =>
        (D.canManage() || s.app === "order") &&
        (f.app === "all" || s.app === f.app) &&
        (f.env === "all" || s.env === f.env),
    );
    const filter = (name, label, items, value) => {
      if (name === "app" || name === "service") {
        const labelValue =
          value === "all"
            ? name === "app"
              ? t("全部已授权应用", "All authorized applications")
              : t("全部匹配服务", "All matching services")
            : name === "app"
              ? D.appName(value)
              : D.serviceName(value);
        return D.pickerField(
          "dashboard-" + name,
          label,
          labelValue,
          "select-dashboard-" + name,
        );
      }
      return D.select(
        name,
        label,
        items.some((i) => i[0] === value)
          ? items
          : [
              ...items,
              [
                value,
                t(
                  "原选项不在当前候选：",
                  "Previous selection outside candidates: ",
                ) + value,
              ],
            ],
        value,
        "data-dashboard-filter",
      );
    };
    const filterHtml = `<div class="argus-panel"><div class="argus-filterbar">${filter("app", t("应用", "Application"), appOptions, f.app)}${filter(
      "env",
      t("环境", "Environment"),
      [
        ["all", t("全部环境", "All environments")],
        ["production", D.envName("production")],
        ["staging", D.envName("staging")],
      ],
      f.env,
    )}${filter("kind", t("资源类型", "Resource type"), kindOptions, f.kind)}${filter(
      "time",
      t("时间范围", "Time range"),
      [
        ["15m", t("最近 15 分钟", "Last 15 minutes")],
        ["1h", t("最近 1 小时", "Last hour")],
        ["6h", t("最近 6 小时", "Last 6 hours")],
      ],
      f.time,
    )}${filter("service", t("应用服务", "Service"), [["all", t("全部匹配服务", "All matching services")], ...services.map((s) => [s.id, s.name + " · " + D.envName(s.env)])], f.service)}<div class="argus-field"><span>${t("目标资源", "Target resources")}</span>${D.btn("select-scope", f.ids === null ? t(`全部匹配资源（${candidates.length}）`, `All matching resources (${candidates.length})`) : t(`已选择 ${f.ids.length} 项`, `Selected ${f.ids.length}`), { icon: "box" })}</div>${filter("source", t("采集来源", "Collection source"), [["all", t("全部来源", "All sources")], ...Array.from(new Set(D.state.monitors.map((m) => m.type))).map((id) => [id, D.catalogById[id]?.name || id])], f.source)}<div class="argus-field"><span>${t("关联依赖", "Dependencies")}</span><label class="argus-check"><input type="checkbox" name="dependencies" data-dashboard-filter ${f.dependencies ? "checked" : ""}>${t("包含应用关联资源", "Include application dependencies")}</label></div></div><details class="argus-panel-body" data-filter-details ${D.ui.k8sFiltersOpen ? "open" : ""}><summary>${t("Kubernetes 对象筛选", "Kubernetes object filters")}</summary><div class="argus-grid3 argus-mt">${filter(
      "cluster",
      t("集群", "Cluster"),
      [
        ["all", t("全部", "All")],
        ["cluster-a", "production-shanghai"],
      ],
      f.cluster,
    )}${filter("namespace", "Namespace", [["all", t("全部", "All")], ...D.state.namespaces.map((n) => [n, n])], f.namespace)}${filter("workload", t("工作负载", "Workload"), [["all", t("全部", "All")], ...D.state.workloads.filter((w) => f.namespace === "all" || w.namespace === f.namespace).map((w) => [w.name, w.name])], f.workload)}</div><p class="argus-small argus-muted argus-mt">${t("这些条件仅匹配具有对应集群归属的资源；外部数据库不会因 Collector 位于该集群而被纳入。", "These filters match resource placement. External databases are not included merely because their Collector runs in the cluster.")}</p></details><div class="argus-scope argus-row"><span>${D.icon("shield")}</span><strong id="scope-count">${t(`实际查询 ${scope.length} 个资源`, `Querying ${scope.length} resources`)}</strong><span>${scope.map((r) => e(r.name)).join(" · ") || t("无匹配资源，保持空集合", "No matching resources; scope stays empty")}</span><span class="argus-grow"></span>${D.btn("scope-explain", t("查看筛选逻辑", "Inspect scope"), { quiet: true })}</div></div>`;
    const panels = D.dashboardPanels()
      .map((p) => {
        const matched = scope.filter((r) => p.kinds.includes(r.kind));
        const samples = matched.filter((r) => baseValues[r.id] !== undefined);
        let body;
        if (
          D.ui.component &&
          !D.state.components.find((c) => c.id === D.ui.component)?.available
        )
          body = D.empty(
            t("组件端点不可访问", "Component endpoint unavailable"),
            t(
              "未观测到该组件数据，不展示替代数值。",
              "No component observations; no substitute values are shown.",
            ),
          );
        else if (
          D.ui.component &&
          !["kubelet", "cluster-state"].includes(D.ui.component) &&
          (f.namespace !== "all" || f.workload !== "all")
        )
          body = D.empty(
            t("组件指标不适用当前对象筛选", "Object filter not applicable"),
            t(
              "此组件面板使用集群范围，请明确清除 Namespace / 工作负载筛选。",
              "This component uses cluster scope; explicitly clear namespace/workload filters.",
            ),
          );
        else if (!matched.length)
          body = D.empty(
            t("当前范围无匹配资源", "No matching resources"),
            t(
              `此图适用于：${p.kinds.map(D.kindName).join("、")}`,
              `Applies to: ${p.kinds.map(D.kindName).join(", ")}`,
            ),
          );
        else if (!samples.length)
          body = D.empty(
            t("尚无可用样本", "No samples available"),
            t(
              "配置已存在，等待真实采集结果。",
              "Configuration exists; waiting for collection results.",
            ),
          );
        else {
          let val =
            (samples.reduce((n, r) => n + baseValues[r.id], 0) /
              (["%", "ms"].includes(p.unit) ? samples.length : 1)) *
            p.scale;
          if (p.id === "latency") {
            const values = D.traces
              .filter((x) => samples.some((r) => r.id === x.resource))
              .map((x) => x.duration)
              .sort((a, b) => a - b);
            if (values.length)
              val = values[Math.ceil(values.length * 0.95) - 1];
          }
          if (
            p.kinds.includes("kubernetes") &&
            (f.namespace !== "all" || f.workload !== "all")
          )
            val *=
              D.state.workloads.filter(
                (w) =>
                  (f.namespace === "all" || w.namespace === f.namespace) &&
                  (f.workload === "all" || w.name === f.workload),
              ).length / Math.max(1, D.state.workloads.length);
          const factor = f.time === "15m" ? 0.83 : f.time === "6h" ? 1.16 : 1;
          const value = (
            p.unit === "%" ? Math.min(100, val * factor) : val * factor
          ).toFixed(p.unit === "%" ? 1 : 0);
          const series = Array.from({ length: 24 }, (_, i) =>
            Math.max(
              0,
              Math.min(
                p.unit === "%" ? 100 : Infinity,
                val *
                  factor *
                  (0.86 + (i % 5) * 0.045 + Math.sin(i * 0.5) * 0.06),
              ),
            ),
          );
          body = `<div class="argus-row argus-between"><strong class="argus-chart-value">${value} <span class="argus-small argus-muted">${p.unit}</span></strong>${D.tag(t("示例数据", "Sample data"))}</div>${D.chart(1, p.unit, series)}<div class="argus-small argus-muted">${samples.map((r) => e(r.name)).join(" · ")}<br>${p.caption}</div>`;
        }
        return `<section class="argus-panel" data-panel="${p.id}"><div class="argus-panel-head"><h2>${t(p.zh, p.en)}</h2>${D.btn("panel-scope", t("范围", "Scope"), { id: p.id, quiet: true })}</div><div class="argus-panel-body">${body}</div></section>`;
      })
      .join("");
    return (
      D.title(
        D.ui.component
          ? `${D.n(D.state.components.find((c) => c.id === D.ui.component))} · ${t("组件仪表盘", "Component dashboard")}`
          : t("跨资源仪表盘", "Cross-resource dashboards"),
        t(
          "应用、服务与基础设施共用筛选体系，每张图声明适用资源和来源。",
          "One scope model for applications, services and infrastructure; each panel declares compatible resources and sources.",
        ),
        D.btn("dashboard-reset", t("重置筛选", "Reset filters"), {
          icon: "refresh",
        }),
        "DASHBOARD EXPLORER",
      ) +
      D.tabs(
        [
          ["application", "应用运行总览", "Application overview"],
          ["infrastructure", "基础设施", "Infrastructure"],
          ["database", "数据库", "Databases"],
          ["kubernetes", "Kubernetes", "Kubernetes"],
          ["host", "主机", "Hosts"],
          ["mixed", "全部图表", "All panels"],
        ],
        D.ui.dashboard,
        "dashboard-template",
      ) +
      filterHtml +
      (!scope.length
        ? D.note(
            t(
              "条件交集为空。不会自动扩大到全部资源；请调整筛选。",
              "The intersection is empty. Scope is not widened automatically; adjust the filters.",
            ),
            "warning",
          )
        : "") +
      `<div class="argus-grid2">${panels}</div>`
    );
  };
  D.openScopePicker = () => {
    const rows = D.scopeResources(true),
      ids = D.ui.filters.ids;
    D.dialog(
      t("选择目标资源", "Select target resources"),
      `<div class="argus-select-list"><label><input type="checkbox" id="scope-all" ${ids === null ? "checked" : ""}><strong>${t("全部匹配且已授权资源", "All matching authorized resources")}</strong></label>${rows.map((r) => `<label><input type="checkbox" name="scope-id" value="${r.id}" ${ids === null || ids.includes(r.id) ? "checked" : ""}><div><strong>${e(r.name)}</strong><div class="argus-small argus-muted">${D.kindName(r.kind)} · ${D.environmentLabel(r)}</div></div></label>`).join("")}</div>${D.note(t("取消全部选择代表空范围；不会解释为全部。", "Selecting nothing means an empty scope, never all resources."))}`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("scope-apply", t("应用筛选", "Apply filters"), { primary: true }),
    );
  };
})();
