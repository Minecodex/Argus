(() => {
  const D = window.ArgusDemo,
    t = (a, b) => D.t(a, b),
    e = D.e;
  D.pages = {};
  D.monitorRows = (monitors) =>
    monitors
      .filter(
        (m) =>
          D.canManage() ||
          (m.apps.includes("order") && ["traces", "logs"].includes(m.signal)),
      )
      .map((m) => [
        `<button class="argus-link" data-action="monitor-detail" data-id="${m.id}">${e(D.n(m))}</button><small>${e(D.resource(m.target)?.name || m.target)}</small>`,
        `${e(D.catalogById[m.type]?.name || m.type)}<small>${m.signal} · ${e(m.interval)}</small>`,
        `${D.appTags(m.apps)}<small>${e(m.service || D.environmentLabel(m))}</small>`,
        `<span class="argus-small">${e(D.collectorName(m.collector))}</span><small>R${m.revision} · ${t("已应用", "Applied")}</small>`,
        `${D.status(m.enabled ? m.status : "paused")}<small>${m.status === "error" ? t("凭据权限不足", "Insufficient credential permissions") : m.status === "waiting" ? t("尚无上报样本", "No received samples") : m.enabled ? t("最近样本：12 秒前 · 示例", "Last sample: 12s ago · demo") : t("历史仍可查询", "History remains queryable")}</small>`,
        D.toggle(
          "monitor-toggle",
          m.id,
          m.enabled,
          `${D.n(m)} ${t("启用", "enabled")}`,
          !D.canManage(),
        ),
        D.menu(`${D.n(m)} ${t("操作", "actions")}`, [
          ["monitor-detail", t("查看配置", "View configuration"), m.id],
          [
            "monitor-edit",
            t("编辑采集", "Edit collection"),
            m.id,
            !D.canManage(),
          ],
          ["monitor-dashboard", t("查看仪表盘", "View dashboard"), m.id],
          [
            "monitor-test",
            t("测试采集（模拟）", "Test collection (demo)"),
            m.id,
            !D.canManage(),
          ],
          [
            "monitor-delete",
            t("删除采集", "Delete collection"),
            m.id,
            !D.canManage(),
          ],
        ]),
      ]);
  D.monitorTable = (list) =>
    D.table(
      [
        t("采集名称 / 监控对象", "Collection / target"),
        t("类型 / 信号", "Type / signal"),
        t("关联应用", "Applications"),
        t("执行 Collector", "Executor Collector"),
        t("实际状态", "Observed status"),
        t("启用", "Enabled"),
        "",
      ],
      D.monitorRows(list),
      t(
        `${list.length} 条配置 · 开关修改单条采集，配置生效状态单独显示。`,
        `${list.length} configurations · desired state and effective status are separate.`,
      ),
    );
  D.appOverview = (app, services) => {
    const traces = D.traces.filter((x) => x.app === app.id),
      dependencies = D.state.resources.filter(
        (r) => r.kind !== "application" && r.apps.includes(app.id),
      );
    const durations = traces.map((x) => x.duration).sort((a, b) => a - b),
      p95 = durations.length
        ? durations[Math.ceil(durations.length * 0.95) - 1]
        : null;
    const health = traces.length
      ? `<div class="argus-row argus-between argus-mb"><strong>${t("请求样本时延", "Received request latency")}</strong>${D.tag(`p95 · ${p95} ms`, "warning")}</div>${D.chart(23, "ms", durations)}<p class="argus-small argus-muted">${t("示例 Trace 样本，不推算全量请求。", "Sample trace data; no total-request extrapolation.")}</p>`
      : D.empty(
          t("尚无业务遥测", "No telemetry yet"),
          t(
            "登记服务并配置 Trace / 日志接入。",
            "Register a service and configure trace/log ingest.",
          ),
        );
    const facts = D.kv([
      [
        t("登记服务", "Registered services"),
        services.map((s) => e(s.name)).join(" · ") || "—",
      ],
      [
        t("依赖资源", "Dependencies"),
        dependencies
          .filter(D.authorized)
          .map((r) => D.tag(r.name))
          .join(" ") || "—",
      ],
      [t("负责人", "Owner"), e(t(app.owner, app.ownerEn))],
    ]);
    return (
      D.stats([
        [
          t("服务", "Services"),
          services.length,
          t("按环境区分", "Separated by environment"),
        ],
        [
          t("运行实例", "Instances"),
          services.reduce((n, s) => n + s.instances, 0),
          t("来自示例遥测", "Sample telemetry"),
        ],
        [
          t("Trace 样本", "Trace samples"),
          traces.length,
          t("当前示例集合", "Current sample dataset"),
        ],
        [
          t("可见关联资源", "Visible dependencies"),
          dependencies.filter(D.authorized).length,
          t(
            "共享资源保留独立权限",
            "Shared resources retain separate permissions",
          ),
        ],
      ]) +
      `<div class="argus-grid2">${D.panel(t("应用健康", "Application health"), health, D.btn("app-dashboard", t("打开仪表盘", "Open dashboard"), { id: app.id, quiet: true, icon: "arrow" }))}${D.panel(t("服务与依赖", "Services & dependencies"), facts + `<hr class="argus-divider">${D.note(t("应用权限控制业务遥测；关联资源需要独立授权。", "Application permissions cover business telemetry; dependencies require separate access."))}`)}</div>`
    );
  };
  D.pages.monitoring = () => {
    let list = D.visibleMonitors().filter(
      (m) =>
        (D.ui.category === "all" ||
          D.catalogById[m.type]?.category === D.ui.category) &&
        (D.ui.type === "all" || m.type === D.ui.type) &&
        `${m.name} ${m.en} ${D.resource(m.target)?.name}`
          .toLowerCase()
          .includes(D.ui.search.toLowerCase()),
    );
    const types = D.catalog.filter(
      (c) => D.ui.category === "all" || c.category === D.ui.category,
    );
    return (
      D.title(
        t("监控", "Monitoring"),
        t(
          "为每个服务建立采集实例，统一查看配置、归属与采集健康。",
          "Create collections for your services and track ownership and collection health.",
        ),
        D.btn("new-monitor", t("新建监控", "New monitor"), {
          primary: true,
          icon: "plus",
          disabled: !D.canManage(),
        }),
      ) +
      D.stats([
        [
          t("采集实例", "Collections"),
          D.visibleMonitors().length,
          t("覆盖业务与基础设施", "Across services and infrastructure"),
        ],
        [
          t("正常采集", "Healthy"),
          D.visibleMonitors().filter((m) => m.enabled && m.status === "healthy")
            .length,
          t("示例观测结果", "Sample observations"),
        ],
        [
          t("需要处理", "Needs attention"),
          D.visibleMonitors().filter((m) => m.enabled && m.status === "error")
            .length,
          t(
            "认证失败 · 可进入详情排查",
            "Authentication failure · open to inspect",
          ),
        ],
        [
          t("已停采", "Paused"),
          D.visibleMonitors().filter((m) => !m.enabled).length,
          t("保留配置与历史数据", "Configuration and history retained"),
        ],
      ]) +
      D.tabs(D.categories, D.ui.category, "category") +
      (D.ui.category !== "all"
        ? `<div class="argus-chips">${[["all", t("全部类型", "All types")], ...types.map((c) => [c.id, c.name])].map(([id, name]) => `<button class="argus-chip" data-action="monitor-type" data-id="${id}" aria-pressed="${D.ui.type === id}">${e(name)}</button>`).join("")}</div>`
        : "") +
      `<div class="argus-toolbar">${D.input("monitor-search", t("搜索采集或对象", "Search collections or targets"), D.ui.search, 'placeholder="MySQL / order-api"')}<span class="argus-grow"></span>${D.tag(t("两批集成 · 同一新建流程", "Both waves · one creation flow"))}${D.btn("catalog", t("查看支持目录", "Browse catalog"), { icon: "book" })}</div>` +
      D.monitorTable(list)
    );
  };
  D.pages.applications = () => {
    const app = D.state.apps.find((a) => a.id === D.ui.app);
    if (!app)
      return (
        D.title(
          t("应用", "Applications"),
          t(
            "以应用组织服务、Trace、业务日志和依赖。",
            "Organize services, traces, business logs and dependencies.",
          ),
          D.btn("new-app", t("新建应用", "New application"), {
            primary: true,
            icon: "plus",
            disabled: !D.canManage(),
          }),
        ) +
        D.table(
          [
            t("应用", "Application"),
            t("负责人", "Owner"),
            t("服务数", "Services"),
            t("数据接入", "Ingest"),
            "",
          ],
          D.state.apps
            .filter((a) => D.canManage() || a.id === "order")
            .map((a) => [
              D.btn("app-open", D.n(a), { id: a.id, quiet: true }),
              e(t(a.owner, a.ownerEn)),
              D.state.services.filter((s) => s.app === a.id).length,
              D.state.monitors.filter(
                (m) =>
                  m.apps.includes(a.id) &&
                  ["logs", "traces"].includes(m.signal),
              ).length,
              D.btn("app-open", t("打开", "Open"), {
                id: a.id,
                icon: "arrow",
                quiet: true,
              }),
            ]),
        )
      );
    const services = D.state.services.filter((s) => s.app === app.id),
      monitors = D.state.monitors.filter((m) => m.apps.includes(app.id));
    const tabs = [
      ["overview", "概览", "Overview"],
      ["services", "服务与实例", "Services & instances"],
      ["access", "数据接入", "Ingest"],
      ["dependencies", "关联资源", "Dependencies"],
      ["logs", "日志", "Logs"],
      ["traces", "Trace", "Traces"],
    ];
    let body = "";
    if (D.ui.appTab === "overview") body = D.appOverview(app, services);
    if (D.ui.appTab === "services")
      body = D.table(
        [
          t("服务", "Service"),
          t("环境", "Environment"),
          t("技术栈", "Runtime"),
          t("实例", "Instances"),
          t("数据接入", "Ingest"),
          "",
        ],
        services.map((s) => [
          e(s.name),
          D.envName(s.env),
          e(s.runtime),
          s.instances,
          D.tag("Trace · Logs"),
          D.btn("service-access", t("接入配置", "Configure ingest"), {
            id: s.id,
            disabled: !D.canManage(),
          }),
        ]),
      );
    if (D.ui.appTab === "access")
      body =
        `<div class="argus-toolbar">${D.btn("app-trace", t("新建 Trace 接入", "New trace ingest"), { id: app.id, primary: true, icon: "plus", disabled: !D.canManage() })}${D.btn("app-log", t("新建业务日志采集", "New business log collection"), { id: app.id, disabled: !D.canManage() })}${D.btn("app-keys", t("管理接入 Key", "Manage ingest keys"), { id: app.id, icon: "key", disabled: !D.canManage() })}</div>` +
        D.note(
          t(
            "业务 Trace 与日志必须关联应用和服务。接入 Key 绑定服务与环境；文件／容器日志由受控采集规则绑定。",
            "Business traces and logs require an application and service. Keys are scoped to service and environment; file/container logs use collection bindings.",
          ),
        ) +
        D.monitorTable(
          monitors.filter((m) => ["logs", "traces"].includes(m.signal)),
        );
    if (D.ui.appTab === "dependencies")
      body =
        D.note(
          t(
            "资源可以被多个应用共享。此处的关联不改变资源权限，也不代表资源指标仅由该应用产生。",
            "Resources may be shared. Links do not grant access or attribute all resource usage to this application.",
          ),
        ) +
        D.resourceTable(
          D.state.resources.filter(
            (r) => r.kind !== "application" && r.apps.includes(app.id),
          ),
        ) +
        D.btn("link-resources", t("管理关联资源", "Manage dependencies"), {
          id: app.id,
          disabled: !D.canManage(),
        });
    if (D.ui.appTab === "logs")
      body = D.logTable(D.logs.filter((l) => l.app === app.id));
    if (D.ui.appTab === "traces")
      body = D.traceTable(D.traces.filter((r) => r.app === app.id));
    return (
      `<div class="argus-row argus-mb">${D.btn("app-list", t("所有应用", "All applications"), { quiet: true })}<span class="argus-muted">/</span>${e(D.n(app))}</div>` +
      D.title(
        D.n(app),
        `${e(t(app.description, app.descriptionEn))} · ${e(t(app.owner, app.ownerEn))}`,
        D.btn("new-service", t("登记服务", "Register service"), {
          id: app.id,
          primary: true,
          icon: "plus",
          disabled: !D.canManage(),
        }),
        "APPLICATION",
      ) +
      D.tabs(tabs, D.ui.appTab, "app-tab") +
      body
    );
  };
  D.resourceTable = (list) =>
    D.table(
      [
        t("资源", "Resource"),
        t("类型", "Type"),
        t("环境", "Environment"),
        t("关联应用", "Applications"),
        t("状态", "Status"),
        "",
      ],
      list.filter(D.authorized).map((r) => [
        `<button class="argus-link" data-action="resource-detail" data-id="${r.id}">${e(r.name)}</button><small>${e(r.endpoint || r.workload || "")}</small>`,
        `${D.kindName(r.kind)}<small>${e(r.type)}</small>`,
        D.environmentLabel(r),
        D.appTags(r.apps),
        D.status(r.status),
        D.menu(`${r.name} ${t("操作", "actions")}`, [
          ["resource-detail", t("查看详情", "View details"), r.id],
          ["resource-dashboard", t("查看仪表盘", "View dashboard"), r.id],
          [
            "resource-link",
            t("关联应用", "Link applications"),
            r.id,
            !D.canManage(),
          ],
        ]),
      ]),
    );
  D.pages.resources = () =>
    D.title(
      t("资源目录", "Resource directory"),
      t(
        "监控对象拥有稳定身份。执行采集的主机与被监控对象分别记录。",
        "Monitored targets have stable identities, separate from collection executors.",
      ),
    ) +
    D.tabs(
      [
        ["all", "全部", "All"],
        ["application", "应用服务", "Applications"],
        ["database", "数据库", "Databases"],
        ["cache", "缓存", "Caches"],
        ["queue", "消息队列", "Messaging"],
        ["host", "主机", "Hosts"],
        ["kubernetes", "集群", "Clusters"],
      ],
      D.ui.resourceKind,
      "resource-kind",
    ) +
    D.resourceTable(
      D.state.resources.filter(
        (r) => D.ui.resourceKind === "all" || r.kind === D.ui.resourceKind,
      ),
    );
  D.pages.hosts = () => {
    if (!D.canManage())
      return (
        D.title(
          t("主机", "Hosts"),
          t("基础设施资源", "Infrastructure resources"),
        ) +
        D.empty(
          t("未获得主机访问授权", "Host access not granted"),
          t(
            "应用只读权限不包含基础设施资源。",
            "Application access does not include infrastructure.",
          ),
        )
      );
    const r = D.resource(D.ui.host),
      comp = D.state.hostComponents[r.id];
    const opts = D.state.resources
      .filter((x) => x.kind === "host")
      .map((x) => [x.id, x.name]);
    let body = "";
    if (D.ui.hostTab === "overview")
      body =
        D.stats([
          [
            t("CPU 使用率", "CPU utilization"),
            "24.8%",
            t("示例 · 8 核", "Demo · 8 cores"),
          ],
          [
            t("内存使用", "Memory used"),
            "12.6 / 32 GB",
            t("示例资产与遥测", "Sample asset and telemetry"),
          ],
          [
            t("文件系统", "Filesystem"),
            "42%",
            t("示例 · /data", "Demo · /data"),
          ],
          ["Collector", D.t("在线", "Online"), "R4 / R4"],
        ]) +
        `<div class="argus-grid2">${D.panel(
          t("资产信息", "Host inventory"),
          D.kv([
            [t("主机", "Host"), e(r.name)],
            [t("地址", "Address"), e(r.endpoint)],
            [
              t("操作系统", "OS"),
              r.id === "host-win" ? "Windows Server" : "Ubuntu Linux",
            ],
            [
              t("采集范围", "Collection scope"),
              t("本机系统与授权文件", "Local system and authorized files"),
            ],
            [
              t("最近观测", "Last observed"),
              t("示例 · 12 秒前", "Demo · 12 seconds ago"),
            ],
          ]),
        )}${D.panel(t("CPU 与内存", "CPU & memory"), D.chart(16))}</div>`;
    if (D.ui.hostTab === "collection")
      body =
        D.table(
          [
            t("自身采集项", "Local capability"),
            t("范围", "Scope"),
            t("状态", "Status"),
            t("开启", "Enabled"),
          ],
          [
            [
              "metrics",
              t("主机基础指标", "Host metrics"),
              t("CPU、内存、磁盘、网络", "CPU, memory, disk, network"),
            ],
            [
              "system",
              t("系统日志", "System logs"),
              r.id === "host-win" ? "Windows Event Log" : "Journald",
            ],
            [
              "self",
              t("Collector 自身指标", "Collector metrics"),
              t("接收、队列、发送", "Receive, queue, export"),
            ],
            [
              "process",
              t("进程与系统服务", "Processes & services"),
              t("受控进程集合", "Selected processes"),
            ],
          ].map(([id, name, scope]) => [
            name,
            scope,
            D.status(comp[id] ? "healthy" : "paused"),
            D.toggle("host-toggle", id, comp[id], name),
          ]),
        ) +
        D.panel(
          t("文件日志规则", "File log rules"),
          D.monitorTable(
            D.state.monitors.filter(
              (m) =>
                ["filelog", "journald", "eventlog"].includes(m.type) &&
                m.collector === r.collector,
            ),
          ),
          D.btn("host-log", t("新增日志采集", "Add log collection"), {
            id: r.id,
            icon: "plus",
            primary: true,
          }),
        );
    if (D.ui.hostTab === "logs")
      body = D.logTable(D.logs.filter((l) => l.resource === r.id));
    if (D.ui.hostTab === "assignments")
      body =
        D.note(
          t(
            "这些实例由本机 Collector 执行，目标可以位于其他机器。其遥测归属目标资源。",
            "This Collector executes these collections, including remote targets. Telemetry belongs to each target.",
          ),
        ) +
        D.monitorTable(
          D.state.monitors.filter((m) => m.collector === r.collector),
        );
    return (
      D.title(
        r.name,
        t(
          "主机资源、自身采集与 Collector 运行状态。",
          "Host resources, local collections and Collector state.",
        ),
        D.btn("resource-dashboard", t("主机仪表盘", "Host dashboard"), {
          id: r.id,
          icon: "grid",
        }),
        "HOST",
      ) +
      `<div class="argus-toolbar">${D.select("host-select", t("主机", "Host"), opts, r.id)}</div>` +
      D.tabs(
        [
          ["overview", "概览", "Overview"],
          ["collection", "自身监控", "Local monitoring"],
          ["logs", "系统日志", "System logs"],
          ["assignments", "执行的采集任务", "Assigned collections"],
        ],
        D.ui.hostTab,
        "host-tab",
      ) +
      body
    );
  };
  D.pages.kubernetes = () => {
    if (!D.canManage())
      return (
        D.title(
          "Kubernetes",
          t("集群资源管理", "Cluster resource management"),
        ) +
        D.empty(
          t("未获得集群访问授权", "Cluster access not granted"),
          t(
            "应用日志仍可在应用页按授权查看。",
            "Authorized business logs remain available in the application page.",
          ),
        )
      );
    let body = "";
    if (D.ui.k8sTab === "monitoring")
      body =
        D.note(
          t(
            "按组件直接启停。系统生成采集配置与对应仪表盘；不可访问的端点显示原因。",
            "Enable components directly. Collection configuration and dashboards are provisioned together; unavailable endpoints show a reason.",
          ),
        ) +
        D.table(
          [
            t("组件", "Component"),
            t("监控内容", "Coverage"),
            t("采集状态", "Collection status"),
            t("启用", "Enabled"),
            t("仪表盘", "Dashboard"),
          ],
          D.state.components.map((c) => [
            D.n(c),
            e(t(c.detail, c.detailEn)),
            c.available
              ? D.status(c.enabled ? "healthy" : "paused")
              : D.tag(t("端点不可访问", "Endpoint unavailable"), "warning"),
            D.toggle("component-toggle", c.id, c.enabled, D.n(c), !c.available),
            D.btn("component-dashboard", t("查看指标", "View metrics"), {
              id: c.id,
              quiet: true,
            }),
          ]),
        );
    if (D.ui.k8sTab === "logs")
      body =
        `<div class="argus-toolbar">${D.btn("k8s-log", t("新建日志采集", "New log collection"), { primary: true, icon: "plus" })}</div>` +
        D.note(
          t(
            "业务日志必须选择应用与服务；组件日志归属集群。按工作负载持续跟随 Pod，不固定当前 Pod 名称。",
            "Business logs require an application and service; component logs belong to the cluster. Workload rules follow new pods.",
          ),
        ) +
        D.monitorTable(D.state.monitors.filter((m) => m.type === "k8slog"));
    if (D.ui.k8sTab === "dashboards")
      body = D.panel(
        t("集群默认仪表盘", "Cluster dashboards"),
        D.table(
          [t("名称", "Name"), t("覆盖范围", "Coverage"), ""],
          [
            [
              "Kubernetes / Compute Resources",
              t(
                "集群、Namespace、工作负载、Pod",
                "Cluster, namespace, workload, pod",
              ),
              D.btn("cluster-dashboard", t("打开", "Open")),
            ],
            [
              "Kubernetes / Control Plane",
              t(
                "API Server、Scheduler、Controller Manager",
                "API Server, Scheduler, Controller Manager",
              ),
              D.btn("component-dashboard", t("打开", "Open"), {
                id: "apiserver",
              }),
            ],
            [
              "Kubernetes / Kubelet",
              t("节点与容器", "Nodes & containers"),
              D.btn("component-dashboard", t("打开", "Open"), {
                id: "kubelet",
              }),
            ],
          ],
        ),
      );
    if (D.ui.k8sTab === "resources") {
      const kinds = [
        ["overview", "集群概览", "Overview"],
        ["Deployment", "Deployments", "Deployments"],
        ["StatefulSet", "StatefulSets", "StatefulSets"],
        ["DaemonSet", "DaemonSets", "DaemonSets"],
        ["Pod", "Pods", "Pods"],
        ["Service", "Services", "Services"],
        ["Ingress", "Ingresses", "Ingresses"],
        ["ConfigMap", "ConfigMaps", "ConfigMaps"],
        ["Secret", "Secrets", "Secrets"],
        ["PVC", "存储卷 PVC", "Volumes / PVC"],
        ["Namespace", "Namespaces", "Namespaces"],
        ["Node", "Nodes", "Nodes"],
        ["events", "事件与变更", "Events & changes"],
      ];
      body = `<div class="argus-subnav-layout"><nav class="argus-subnav" aria-label="${t("Kubernetes 资源类型", "Kubernetes resource kinds")}">${kinds.map(([id, zh, en]) => `<button data-action="k8s-kind" data-id="${id}" ${D.ui.k8sKind === id ? 'aria-current="page"' : ""}>${t(zh, en)}</button>`).join("")}</nav><section>${D.k8sResourceView()}</section></div>`;
    }
    return (
      D.title(
        "production-shanghai",
        t(
          "在线管理集群资源，观察组件健康，配置业务日志归属。",
          "Manage cluster resources, inspect component health and configure log ownership.",
        ),
        D.tag(
          t("示例集群 · 3 / 3 节点就绪", "Demo cluster · 3 / 3 nodes ready"),
          "success",
        ),
        "KUBERNETES",
      ) +
      D.tabs(
        [
          ["resources", "资源管理", "Resources"],
          ["monitoring", "组件监控", "Component metrics"],
          ["logs", "日志采集", "Log collection"],
          ["dashboards", "仪表盘", "Dashboards"],
        ],
        D.ui.k8sTab,
        "k8s-tab",
      ) +
      body
    );
  };
  D.k8sResourceView = () => {
    const kind = D.ui.k8sKind;
    const selector = D.select(
      "namespace",
      t("命名空间", "Namespace"),
      [
        ["all", t("全部 Namespace", "All namespaces")],
        ...D.state.namespaces.map((n) => [n, n]),
      ],
      D.ui.namespace,
    );
    const tools = `<div class="argus-toolbar">${selector}<span class="argus-grow"></span>${["Deployment", "StatefulSet", "DaemonSet", "Service", "Ingress", "ConfigMap", "PVC", "Namespace"].includes(kind) ? D.btn("k8s-create", t("创建资源", "Create resource"), { id: kind, primary: true, icon: "plus" }) : ""}</div>`;
    if (kind === "overview")
      return (
        D.stats([
          [t("节点", "Nodes"), 3, "Ready"],
          [
            t("工作负载", "Workloads"),
            D.state.workloads.length,
            t("示例", "Demo"),
          ],
          [
            "Pods",
            D.state.workloads.reduce((n, w) => n + w.replicas, 0),
            "Running",
          ],
          ["Namespaces", D.state.namespaces.length, t("示例", "Demo")],
        ]) + D.panel(t("集群资源使用", "Cluster resource use"), D.chart(35))
      );
    if (kind === "events") return D.auditView();
    if (kind === "Namespace")
      return (
        tools +
        D.table(
          ["Namespace", t("工作负载", "Workloads"), t("状态", "Status")],
          D.state.namespaces.map((n) => [
            e(n),
            D.state.workloads.filter((w) => w.namespace === n).length,
            D.status("healthy"),
          ]),
        )
      );
    if (kind === "Node")
      return (
        tools +
        D.table(
          ["Node", t("就绪", "Ready"), "CPU", t("内存", "Memory")],
          ["worker-sh-01", "worker-sh-02", "worker-sh-03"].map((n, i) => [
            n,
            D.status("healthy"),
            `${24 + i * 8}%`,
            `${42 + i * 9}%`,
          ]),
        )
      );
    if (kind === "Pod")
      return (
        tools +
        D.table(
          [
            "Pod",
            "Namespace",
            t("状态", "Status"),
            t("归属应用", "Application"),
            "",
          ],
          D.state.workloads
            .filter(
              (w) => D.ui.namespace === "all" || w.namespace === D.ui.namespace,
            )
            .flatMap((w) =>
              Array.from({ length: w.replicas }, (_, i) => [
                `${e(w.name)}-${i} <small>${e(w.image)}</small>`,
                e(w.namespace),
                D.status("healthy"),
                w.app
                  ? D.appTags([w.app])
                  : D.tag(t("基础设施", "Infrastructure")),
                D.menu(`${w.name}-${i} ${t("操作", "actions")}`, [
                  [
                    "pod-detail",
                    t("详情与事件", "Details & events"),
                    `${w.id}:${i}`,
                  ],
                  [
                    "pod-logs",
                    t("实时日志（模拟）", "Live logs (demo)"),
                    `${w.id}:${i}`,
                  ],
                  [
                    "pod-terminal",
                    t("终端（模拟）", "Terminal (demo)"),
                    `${w.id}:${i}`,
                  ],
                ]),
              ]),
            ),
        )
      );
    if (["Deployment", "StatefulSet", "DaemonSet"].includes(kind))
      return (
        tools +
        D.table(
          [
            t("工作负载", "Workload"),
            "Namespace",
            t("就绪 / 期望", "Ready / desired"),
            t("应用服务", "Application service"),
            t("版本", "Revision"),
            "",
          ],
          D.state.workloads
            .filter(
              (w) =>
                w.kind === kind &&
                (D.ui.namespace === "all" || w.namespace === D.ui.namespace),
            )
            .map((w) => [
              `<button class="argus-link" data-action="workload-detail" data-id="${w.id}">${e(w.name)}</button><small>${e(w.image)}</small>`,
              e(w.namespace),
              `${w.ready} / ${w.replicas}`,
              w.app
                ? `${D.appTags([w.app])}<small>${e(w.service)}</small>`
                : D.tag(t("基础设施", "Infrastructure")),
              `R${w.revision}`,
              D.menu(`${w.name} ${t("操作", "actions")}`, [
                [
                  "k8s-scale",
                  t("调整副本", "Scale"),
                  w.id,
                  kind === "DaemonSet",
                ],
                ["k8s-restart", t("滚动重启", "Rollout restart"), w.id],
                ["k8s-yaml", t("查看 / 编辑 YAML", "View / edit YAML"), w.id],
                [
                  "k8s-rollback",
                  t("回滚上一版本", "Roll back"),
                  w.id,
                  !w.previous,
                ],
                ["workload-log", t("配置日志采集", "Configure logs"), w.id],
                ["k8s-delete", t("删除资源", "Delete resource"), w.id],
              ]),
            ]),
        )
      );
    return (
      tools +
      D.table(
        [t("名称", "Name"), "Namespace", t("配置摘要", "Configuration"), ""],
        D.state.objects
          .filter(
            (o) =>
              o.kind === kind &&
              (D.ui.namespace === "all" || o.namespace === D.ui.namespace),
          )
          .map((o) => [
            e(o.name),
            e(o.namespace),
            `<span class="argus-mono">${e(o.value)}</span>`,
            D.menu(`${o.name} ${t("操作", "actions")}`, [
              ["object-detail", t("查看详情", "Details"), o.id],
              [
                "object-edit",
                kind === "PVC" ? t("扩容", "Expand") : t("编辑配置", "Edit"),
                o.id,
                kind === "Secret",
              ],
              [
                "object-delete",
                t("删除资源", "Delete"),
                o.id,
                kind === "Secret",
              ],
            ]),
          ]),
      )
    );
  };
  D.logTable = (logs) =>
    D.table(
      [
        t("时间 / 级别", "Time / level"),
        t("服务 / 来源", "Service / source"),
        t("日志内容", "Log message"),
        t("关联链路", "Trace"),
      ],
      logs
        .filter((l) => D.canManage() || l.app === "order")
        .map((l) => [
          `${e(l.time)}<small>${D.tag(l.level, l.level === "ERROR" ? "danger" : l.level === "WARN" ? "warning" : "")}</small>`,
          `${e(l.service)}<small>${e(l.source)} · ${D.envName(l.env)}</small>`,
          e(l.body),
          l.trace
            ? D.btn("trace-detail", l.trace.slice(-10), {
                id: l.trace,
                quiet: true,
              })
            : D.tag(t("无 Trace ID", "No trace ID")),
        ]),
    );
  D.pages.logs = () =>
    D.title(
      t("日志探索", "Log explorer"),
      t(
        "按应用、来源与内容检索；基础设施日志保留自己的资源归属。",
        "Explore application and infrastructure logs with explicit ownership.",
      ),
      D.btn("new-log", t("新建日志采集", "New log collection"), {
        primary: true,
        icon: "plus",
        disabled: !D.canManage(),
      }),
    ) +
    `<div class="argus-toolbar">${D.explorerApplicationField("logs")}${D.select(
      "log-level",
      t("级别", "Level"),
      [
        ["all", t("全部", "All")],
        ["ERROR", "ERROR"],
        ["WARN", "WARN"],
        ["INFO", "INFO"],
      ],
      D.ui.logLevel,
    )}${D.input("log-query", t("内容或 Trace ID", "Text or trace ID"), D.ui.logQuery)}</div>` +
    D.logTable(
      D.logs.filter(
        (l) =>
          (D.ui.logApp === "all" ||
            (D.ui.logApp === "infra" && !l.app) ||
            l.app === D.ui.logApp) &&
          (D.ui.logLevel === "all" || l.level === D.ui.logLevel) &&
          `${l.body} ${l.trace}`
            .toLowerCase()
            .includes(D.ui.logQuery.toLowerCase()),
      ),
    );
  D.traceTable = (traces) =>
    D.table(
      [
        t("请求 / Trace", "Request / trace"),
        t("应用服务", "Application service"),
        t("耗时", "Duration"),
        t("状态", "Status"),
        "Spans",
        "",
      ],
      traces
        .filter((r) => D.canManage() || r.app === "order")
        .map((r) => [
          `<button class="argus-link" data-action="trace-detail" data-id="${r.id}">${e(r.operation)}</button><small>${e(r.id)}</small>`,
          `${D.appTags([r.app])}<small>${e(r.service)}</small>`,
          `${r.duration} ms`,
          D.tag(r.error ? "ERROR" : "OK", r.error ? "danger" : "success"),
          r.spans,
          D.btn("trace-detail", t("查看链路", "Inspect trace"), {
            id: r.id,
            quiet: true,
            icon: "arrow",
          }),
        ]),
    );
  D.pages.traces = () =>
    D.title(
      t("Trace 探索", "Trace explorer"),
      t(
        "按应用识别服务与实例，保留跨服务链路及采集来源。",
        "Identify services and instances by application while retaining trace provenance.",
      ),
      D.btn("new-trace", t("新建 Trace 接入", "New trace ingest"), {
        primary: true,
        icon: "plus",
        disabled: !D.canManage(),
      }),
    ) +
    D.note(
      t(
        "此处为已接收样本。跨应用展开按数据授权裁剪；日志关联使用 Trace ID / Span ID。",
        "Received samples only. Cross-application traces respect data access; log correlation uses Trace ID / Span ID.",
      ),
    ) +
    `<div class="argus-toolbar">${D.explorerApplicationField("traces")}</div>` +
    D.traceTable(
      D.traces.filter(
        (r) => D.ui.traceApp === "all" || r.app === D.ui.traceApp,
      ),
    );
  D.auditView = () =>
    `<div class="argus-timeline">${D.state.audit.map((a) => `<article><small>${e(a.time)} · ${t("模拟执行", "Simulation")}</small><strong>${e(t(a.title, a.en))}</strong><p class="argus-small argus-muted">${e(a.detail)}</p></article>`).join("")}</div>`;
  D.pages.blueprint = () =>
    D.title(
      t("功能范围与架构边界", "Scope & architecture"),
      t(
        "本原型覆盖拟议体验，真实后端与集群操作按领域逐步实施。",
        "This prototype covers the proposed experience; production capabilities require domain implementation.",
      ),
    ) +
    D.table(
      [
        t("领域", "Domain"),
        t("交付内容", "Deliverable"),
        t("关键边界", "Boundary"),
      ],
      [
        [
          t("应用与服务", "Applications & services"),
          t(
            "应用、服务、环境、实例、负责人、Key",
            "Application, service, environment, instance, owner, keys",
          ),
          t(
            "业务遥测以应用服务归属；主机与集群仍是独立资源",
            "Business telemetry belongs to services; hosts and clusters remain distinct",
          ),
        ],
        [
          t("监控与采集", "Monitoring & collections"),
          t(
            "两批服务目录、多实例、启停、版本、诊断",
            "Both integration waves, multiple instances, state, revision, diagnostics",
          ),
          t(
            "目标、执行者、采集来源分开；接入能力需逐项验证",
            "Separate target, executor and source; validate each integration",
          ),
        ],
        [
          "Kubernetes",
          t(
            "资源查询、创建、扩缩容、重启、回滚、日志与终端",
            "Browse, create, scale, restart, rollback, logs and terminal",
          ),
          t(
            "按动作授权；变更预览、版本冲突、审计；终端独立会话",
            "Action authorization, preview, version conflicts, audit; separate terminal sessions",
          ),
        ],
        [
          t("日志与 Trace", "Logs & traces"),
          t(
            "业务归属、工作负载映射、SDK 接入与关联",
            "Business ownership, workload bindings, SDK ingest and correlation",
          ),
          t(
            "系统日志无需业务应用；无 Trace ID 不伪造关联",
            "System logs need no business app; do not invent trace correlation",
          ),
        ],
        [
          t("仪表盘与告警", "Dashboards & alerts"),
          t(
            "默认模板、资源筛选、应用依赖、图表适用性",
            "Templates, resource filters, dependencies, panel compatibility",
          ),
          t(
            "授权范围求交；空集合不扩大；共享依赖不当作独占用量",
            "Intersect access and filters; keep empty scope; identify shared dependencies",
          ),
        ],
        [
          t("实施顺序", "Delivery sequence"),
          t(
            "身份与权限 → 采集实例 → 应用/日志/Trace → K8s 管理 → 模板和完整验收",
            "Identity & access → collections → apps/logs/traces → K8s management → templates & acceptance",
          ),
          t(
            "每一阶段贯通存储、API、门户、Agent、审计和真实 E2E",
            "Each stage spans storage, API, portal, Agent, audit and real E2E",
          ),
        ],
      ],
    ) +
    D.panel(
      t("原型操作路径", "Try the prototype"),
      `<div class="argus-row">${D.btn("new-monitor", t("创建一个 MySQL 监控", "Create a MySQL monitor"), { primary: true })}${D.btn("navigate", t("查看应用接入", "Explore application ingest"), { id: "applications" })}${D.btn("navigate", t("管理 Kubernetes", "Manage Kubernetes"), { id: "kubernetes" })}${D.btn("navigate", t("体验仪表盘筛选", "Try dashboard filters"), { id: "dashboards" })}</div>`,
    );
})();
