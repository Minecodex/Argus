(() => {
  const D = window.ArgusDemo,
    t = (a, b) => D.t(a, b),
    e = D.e;
  D.startWizard = (type = "mysql", preset = {}, editId = null) => {
    if (!D.canManage())
      return D.toast(
        t("当前角色没有配置权限", "This role cannot configure collections"),
      );
    const c = D.catalogById[type] || D.catalog[0];
    const defaults = {
      name: "",
      env: null,
      collector:
        c.mode === "k8slog"
          ? "col-k8s-agent"
          : c.mode === "push"
            ? "col-k8s"
            : c.mode === "windows"
              ? "col-win"
              : "col-a",
      endpoint: "",
      targetId: null,
      app: "",
      service: "",
      apps: [],
      services: [],
      scope: "",
      interval: "30s",
      authMode: D.credentialModes(c)[0],
      managedCredentialId: null,
      tls: "verify",
      logClass:
        ["system", "windows"].includes(c.mode) && c.category === "logs"
          ? "system"
          : "business",
      path: "/data/app/logs/*.log",
      parser: "json",
      charset: "UTF-8",
      start: "end",
      multiline: "",
      namespace: "commerce",
      kind: "Deployment",
      workload: "order-api",
      container: "api",
      selector: "",
      oracleService: "ORCL",
      sqlInstance: "MSSQLSERVER",
      protocol: "http/protobuf",
      expected: "200",
      method: "GET",
      timeout: "5s",
      domain: "api.example.test",
      expectedAnswer: "10.20.1.10",
      scenario: "ok",
    };
    D.wizard = {
      type: c.id,
      step: preset.skipType ? 2 : 1,
      category: c.category,
      data: { ...defaults, ...preset },
      editId,
      tested: false,
    };
    D.wizardSecrets = {};
    D.syncWizardOwnership();
    D.renderWizard();
  };
  D.wizardBusiness = () => {
    const c = D.catalogById[D.wizard.type],
      d = D.wizard.data;
    return (
      c.mode === "push" || (c.category === "logs" && d.logClass === "business")
    );
  };
  D.renderWizard = () => {
    D.syncWizardOwnership();
    const w = D.wizard,
      c = D.catalogById[w.type],
      d = w.data;
    let body = "",
      footer = "";
    if (w.step === 1) {
      body = `<div class="argus-toolbar">${D.select(
        "wizard-category",
        t("分类", "Category"),
        D.categories
          .filter((x) => x[0] !== "all")
          .map(([id, zh, en]) => [id, t(zh, en)]),
        w.category,
      )}</div><div class="argus-catalog">${D.catalog
        .filter((x) => x.category === w.category)
        .map(
          (x) =>
            `<button type="button" data-action="wizard-type" data-id="${x.id}" aria-pressed="${x.id === w.type}"><div class="argus-row argus-between"><strong>${e(x.name)}</strong>${D.tag(t(`第 ${x.phase} 批`, `Wave ${x.phase}`))}</div><small>${e(t(x.zh, x.en))}</small></button>`,
        )
        .join("")}</div>`;
    }
    if (w.step === 2)
      body = `<form id="monitor-form" class="argus-form">${D.wizardFields(c, d)}</form>`;
    if (w.step === 3)
      body =
        D.note(
          c.mode === "push"
            ? t(
                "校验接入配置后创建写入凭据，收到 SDK 数据后才会显示“正常”。",
                "Validate the ingest configuration before issuing a write credential. Health becomes normal only after SDK data arrives.",
              )
            : t(
                "从所选 Collector 检测目标、权限与采集范围。这里执行模拟校验。",
                "Check the target, permissions and scope from the selected Collector. This prototype simulates validation.",
              ),
        ) +
        D.kv([
          [t("采集类型", "Integration"), e(c.name)],
          [t("监控目标", "Target"), e(D.wizardTargetLabel())],
          [t("执行位置", "Executor"), e(D.collectorName(d.collector))],
          [
            t("归属", "Ownership"),
            D.wizardBusiness()
              ? `${D.appName(d.app)} / ${e(d.service)} / ${D.envName(d.env)}`
              : d.apps.length
                ? D.appTags(d.apps)
                : t("基础设施资源", "Infrastructure resource"),
          ],
        ]) +
        `<hr class="argus-divider"><div class="argus-toolbar">${D.select(
          "wizard-scenario",
          t("原型测试场景", "Prototype test scenario"),
          [
            ["ok", t("正常", "Success")],
            ["network", t("目标不可达", "Target unreachable")],
            ["auth", t("凭据权限不足", "Insufficient credentials")],
          ],
          d.scenario,
        )}${D.btn("wizard-test", c.mode === "push" ? t("校验接入配置（模拟）", "Validate ingest (demo)") : t("测试采集（模拟）", "Test collection (demo)"), { primary: true, icon: "play" })}</div><div id="wizard-test-result">${w.tested ? D.note(t("模拟校验通过。配置版本、采集范围和连接认证已检查。", "Simulated validation passed. Revision, scope and connection authentication checked."), "success") : D.note(t("等待执行校验", "Waiting for validation"))}</div>`;
    if (w.step === 4) {
      const ref = D.references?.find((r) => r.types.includes(c.id));
      body =
        D.note(
          t(
            "确认后保存到本地示例状态。正式实现沿用配置预览、一次确认、下发与生效状态。",
            "Confirmation updates local demo state. Production uses configuration preview, confirmation, rollout and effective status.",
          ),
        ) +
        D.kv([
          [t("名称", "Name"), e(d.name)],
          [
            t("类型 / 信号", "Type / signal"),
            e(c.name) + " · " + D.wizardSignal(),
          ],
          [t("监控目标", "Target"), e(D.wizardTargetLabel())],
          [
            t("执行 Collector", "Executor Collector"),
            e(D.collectorName(d.collector)),
          ],
          [
            t("应用关联", "Applications"),
            D.wizardBusiness() ? D.appTags([d.app]) : D.appTags(d.apps),
          ],
          [
            t("环境归属（自动）", "Environment association (inherited)"),
            e(D.environmentLabel(d)),
          ],
          [
            t("连接认证", "Connection authentication"),
            e(D.authLabel(d.authMode)),
          ],
          [
            t("配置变化", "Change"),
            w.editId
              ? t(
                  "更新现有实例，递增配置版本",
                  "Update collection and increment revision",
                )
              : t("新增一个独立采集实例", "Create an independent collection"),
          ],
          [
            t("默认仪表盘", "Default dashboard"),
            ref
              ? `<a href="${e(ref.url)}" target="_blank" rel="noopener noreferrer">${e(ref.title)} ↗</a>`
              : t(
                  "按信号与来源选择对应视图",
                  "Select the appropriate signal/source view",
                ),
          ],
        ]) +
        `<hr class="argus-divider">${D.note(c.mode === "push" ? t("接入 Key 仅用于示例；创建后展示配置说明，初始状态为等待数据。", "The key is a demo credential. Setup instructions follow creation; initial state is waiting for data.") : t("保存配置不代表已收到真实数据，示例将显示“等待数据”。", "Saving configuration does not prove data collection. The demo will show “Waiting for data”."))}`;
    }
    footer =
      D.btn(
        w.step === 1 ? "close" : "wizard-back",
        w.step === 1 ? t("取消", "Cancel") : t("上一步", "Back"),
      ) +
      `<span class="argus-grow"></span><span class="argus-small argus-muted">${e(c.name)}</span>` +
      D.btn(
        w.step === 4 ? "wizard-create" : "wizard-next",
        w.step === 4
          ? w.editId
            ? t("确认保存（模拟）", "Save changes (demo)")
            : t("创建监控（模拟）", "Create monitor (demo)")
          : t("下一步", "Continue"),
        { primary: true, disabled: w.step === 3 && !w.tested },
      );
    const modal = D.dialog(
      w.editId
        ? t("编辑采集实例", "Edit collection")
        : t("新建监控", "New monitor"),
      body,
      footer,
    );
    modal.querySelector(".argus-dialog-head").insertAdjacentHTML(
      "afterend",
      `<div class="argus-steps">${[
        ["选择类型", "Choose type"],
        ["配置采集", "Configure"],
        ["校验", "Validate"],
        ["预览确认", "Review"],
      ]
        .map(
          ([zh, en], i) =>
            `<span class="argus-step" ${w.step === i + 1 ? 'aria-current="step"' : ""}><b>${i + 1}</b>${t(zh, en)}</span>`,
        )
        .join("")}</div>`,
    );
    if (w.step === 2) D.restoreCredentialInputs();
  };
  D.wizardFields = (c, d) => {
    const req = "required",
      f = (name, zh, en, value, extra = "", help = "") =>
        D.input(name, t(zh, en), value, extra, help);
    const sel = (name, zh, en, items, value, extra = "") =>
      D.select(name, t(zh, en), items, value, extra);
    const collectors = D.state.collectors.filter((x) =>
      c.mode === "windows"
        ? x.platform === "windows"
        : c.mode === "k8slog"
          ? x.role === "DaemonSet"
          : c.mode === "system"
            ? x.platform === "linux"
            : c.mode === "file"
              ? x.platform !== "kubernetes"
              : x.role !== "DaemonSet",
    );
    const colOpts = collectors.map((x) => [
      x.id,
      `${x.name} · ${x.platform}${x.state === "offline" ? t(" · 离线", " · offline") : ""}`,
    ]);
    let html = f(
      "name",
      "采集名称",
      "Collection name",
      d.name,
      req + ' placeholder="' + e(c.name + " monitoring") + '"',
    );
    if (d.targetId)
      html +=
        '<div class="argus-field"><span>' +
        t("目标资源", "Target resource") +
        "</span><strong>" +
        e(D.resource(d.targetId)?.name || d.targetId) +
        "</strong><small>" +
        t("由资源入口自动带入。", "Inherited from the resource entry point.") +
        "</small></div>";
    if (
      c.category === "logs" &&
      c.mode !== "push" &&
      !["system", "windows"].includes(c.mode)
    )
      html +=
        sel(
          "logClass",
          "日志归属类型",
          "Log ownership",
          [
            [
              "business",
              t(
                "业务日志 · 必须关联应用",
                "Business logs · application required",
              ),
            ],
            ["system", t("系统 / 组件日志", "System / component logs")],
          ],
          d.logClass,
        ) + `<div></div>`;
    html += `<h3 class="argus-section-label">${t("目标与采集位置", "Target and collection location")}</h3>`;
    if (
      !["push", "file", "k8slog", "system", "windows", "icmp"].includes(c.mode)
    ) {
      html += f(
        "endpoint",
        c.mode === "http" || c.mode === "probe" ? "服务 URL" : "连接地址",
        c.mode === "http" || c.mode === "probe" ? "Service URL" : "Endpoint",
        d.endpoint,
        req +
          (d.targetId ? " readonly" : "") +
          ` placeholder="${c.mode === "http" || c.mode === "probe" ? "https://service.example.test/metrics" : c.mode === "jmx" ? "service:jmx:rmi:///jndi/rmi://host:9999/jmxrmi" : `db.example.test:${c.port}`}"`,
      );
    }
    html += sel(
      "collector",
      c.mode === "push" ? "接收入口所在 Collector" : "执行 Collector",
      c.mode === "push" ? "Ingest Collector" : "Executor Collector",
      colOpts,
      d.collector,
      req,
    );
    if (c.mode !== "push" && c.category !== "logs")
      html += sel(
        "interval",
        "采集间隔",
        "Collection interval",
        [
          ["15s", "15s"],
          ["30s", "30s"],
          ["60s", "60s"],
        ],
        d.interval,
      );
    if (c.id === "oracle")
      html += f(
        "oracleService",
        "Oracle Service Name",
        "Oracle service name",
        d.oracleService,
        req,
      );
    if (c.id === "sqlserver")
      html += f(
        "sqlInstance",
        "SQL Server 实例名",
        "SQL Server instance name",
        d.sqlInstance,
        req,
      );
    if (c.mode === "database")
      html += f(
        "scope",
        "数据库范围",
        "Database scope",
        d.scope,
        'placeholder="' +
          t("全部授权数据库或指定名称", "All permitted databases or a name") +
          '"',
      );
    if (c.mode === "broker")
      html += f(
        "scope",
        "Topic / 消费组范围",
        "Topic / consumer group scope",
        d.scope,
        'placeholder="orders.* / order-worker"',
      );
    if (c.id === "rabbitmq")
      html += f(
        "scope",
        "Virtual host / Queue",
        "Virtual host / queue",
        d.scope,
        'placeholder="/orders"',
      );
    if (c.mode === "jmx")
      html += `<div class="argus-full">${D.note(t("该方案需要已批准的 JMX 采集组件及 Java 运行环境。原型仅展示配置契约。", "This integration requires an approved JMX helper and Java runtime. The prototype demonstrates configuration only."))}</div>`;
    if (c.mode === "windows" && c.id === "iis")
      html += f(
        "scope",
        "站点 / 应用池",
        "Site / application pool",
        d.scope,
        'placeholder="Default Web Site"',
      );
    if (c.mode === "probe")
      html +=
        sel(
          "method",
          "请求方法",
          "Method",
          [
            ["GET", "GET"],
            ["HEAD", "HEAD"],
          ],
          d.method,
        ) +
        f(
          "expected",
          "预期状态码",
          "Expected status code",
          d.expected,
          'required pattern="[1-5][0-9]{2}"',
        ) +
        f("timeout", "超时", "Timeout", d.timeout, "required");
    if (c.mode === "dns")
      html +=
        f("domain", "查询域名", "Domain", d.domain, req) +
        f(
          "expectedAnswer",
          "预期解析结果",
          "Expected answer",
          d.expectedAnswer,
          req,
        );
    if (c.mode === "icmp")
      html +=
        f(
          "endpoint",
          "目标 IP / 主机名",
          "Target IP / hostname",
          d.endpoint,
          req,
        ) +
        `<div class="argus-full">${D.note(t("需要 Collector 具备 ICMP 探测权限。", "Collector requires ICMP probing permissions."))}</div>`;
    if (c.mode === "file")
      html +=
        f(
          "path",
          "日志路径",
          "Log paths",
          d.path,
          req,
          "/data/order/logs/*.log",
        ) +
        sel(
          "parser",
          "解析方式",
          "Parser",
          [
            ["json", "JSON"],
            ["text", t("纯文本", "Plain text")],
            ["nginx", "Nginx access log"],
          ],
          d.parser,
        ) +
        sel(
          "charset",
          "字符编码",
          "Encoding",
          [
            ["UTF-8", "UTF-8"],
            ["GB18030", "GB18030"],
          ],
          d.charset,
        ) +
        sel(
          "start",
          "首次读取位置",
          "Initial position",
          [
            ["end", t("从文件末尾", "End of file")],
            ["beginning", t("从文件开头", "Beginning of file")],
          ],
          d.start,
        ) +
        f(
          "multiline",
          "多行起始表达式（可选）",
          "Multiline start pattern (optional)",
          d.multiline,
          'placeholder="^\\d{4}-\\d{2}-\\d{2}"',
        );
    if (c.mode === "k8slog") {
      const workloads = D.state.workloads.filter(
        (x) => x.namespace === d.namespace && x.kind === d.kind,
      );
      html +=
        sel(
          "namespace",
          "命名空间",
          "Namespace",
          D.state.namespaces.map((n) => [n, n]),
          d.namespace,
        ) +
        sel(
          "kind",
          "选择目标",
          "Target kind",
          [
            ["Deployment", "Deployment"],
            ["StatefulSet", "StatefulSet"],
            ["DaemonSet", "DaemonSet"],
            ["Pod", "Pod"],
            ["Selector", t("标签选择器", "Label selector")],
          ],
          d.kind,
        );
      if (d.kind === "Selector")
        html += f(
          "selector",
          "标签选择器",
          "Label selector",
          d.selector,
          'required placeholder="app=order-api"',
        );
      else if (d.kind === "Pod")
        html += f(
          "workload",
          "Pod 名称 / UID",
          "Pod name / UID",
          d.workload,
          req,
        );
      else
        html += sel(
          "workload",
          "工作负载",
          "Workload",
          [
            ["", t("请选择", "Select")],
            ...workloads.map((x) => [x.name, x.name]),
          ],
          d.workload,
          req,
        );
      html +=
        f(
          "container",
          "容器范围",
          "Container scope",
          d.container,
          "required",
          t(
            "指定容器名或 *；工作负载规则持续跟随新 Pod。",
            "Container name or *; workload rules follow replacement pods.",
          ),
        ) +
        sel(
          "parser",
          "日志解析",
          "Log parser",
          [
            ["json", "JSON"],
            ["text", t("纯文本 / CRI", "Plain text / CRI")],
          ],
          d.parser,
        );
    }
    if (c.mode === "system")
      html += f(
        "scope",
        "Systemd unit 过滤",
        "Systemd unit filter",
        d.scope,
        'placeholder="sshd.service"',
      );
    if (c.id === "eventlog")
      html += sel(
        "scope",
        "事件通道",
        "Event channel",
        [
          ["System", "System"],
          ["Application", "Application"],
          ["Security", "Security"],
        ],
        d.scope || "System",
      );
    if (c.mode === "push")
      html +=
        sel(
          "protocol",
          "传输协议",
          "Protocol",
          c.id === "otlp" || c.id === "sdklog"
            ? [
                ["http/protobuf", "OTLP / HTTP"],
                ["grpc", "OTLP / gRPC"],
              ]
            : [
                [
                  c.id,
                  c.id === "skywalking" ? "SkyWalking gRPC" : "Jaeger gRPC",
                ],
              ],
          d.protocol,
        ) +
        `<div class="argus-full">${D.note(t("创建后生成示例推送地址与写入 Key。应用服务与环境共同确定数据归属。", "Creation issues a sample endpoint and write key. Application, service and environment determine data ownership."))}</div>`;
    html += D.authenticationFields(c, d);
    html += D.ownershipFields();
    return html;
  };
  D.captureWizard = (validate = false) => {
    const form = document.getElementById("monitor-form");
    if (!form) return true;
    const fields = new FormData(form),
      d = D.wizard.data;
    D.wizardSecrets ||= {};
    for (const [key, value] of fields) {
      if (D.credentialFieldNames.includes(key))
        D.wizardSecrets[key] = String(value);
      else if (
        ![
          "env",
          "apps",
          "app",
          "service",
          "services",
          "existing",
          "secret",
        ].includes(key)
      )
        d[key] = String(value).trim();
    }
    D.syncWizardOwnership();
    if (!validate) return true;
    if (!form.reportValidity()) return false;
    if (D.wizardBusiness() && (!d.app || !d.service)) {
      D.toast(t("请选择应用及服务。", "Select an application and service."));
      form
        .querySelector(
          '[name="' + (!d.app ? "application-picker" : "service-picker") + '"]',
        )
        .focus();
      return false;
    }
    const error = D.credentialError();
    if (error) {
      D.toast(error);
      return false;
    }
    return true;
  };
  D.wizardSignal = () =>
    D.catalogById[D.wizard.type].category === "trace"
      ? "traces"
      : D.catalogById[D.wizard.type].category === "logs"
        ? "logs"
        : "metrics";
  D.wizardTargetLabel = () => {
    const c = D.catalogById[D.wizard.type],
      d = D.wizard.data;
    if (c.mode === "push") return `${D.appName(d.app)} / ${d.service}`;
    if (c.mode === "file") return `${D.collectorName(d.collector)} · ${d.path}`;
    if (c.mode === "k8slog")
      return `${d.namespace} / ${d.kind} / ${d.kind === "Selector" ? d.selector : d.workload}`;
    if (["system", "windows"].includes(c.mode) && c.category === "logs")
      return D.collectorName(d.collector);
    return (
      d.endpoint ||
      D.resource(d.targetId)?.endpoint ||
      D.collectorName(d.collector)
    );
  };
  D.testWizard = () => {
    const w = D.wizard,
      d = w.data,
      c = D.catalogById[w.type];
    w.tested = false;
    const col = D.state.collectors.find((x) => x.id === d.collector);
    let error =
      d.scenario === "network"
        ? t(
            "所选 Collector 无法连接目标。",
            "Selected Collector cannot reach the target.",
          )
        : d.scenario === "auth"
          ? t(
              "认证权限不足，请返回修改账号、密码或 Token。",
              "Insufficient permissions; update the username, password or token.",
            )
          : !col || col.state === "offline"
            ? t(
                "Collector 离线，无法应用配置。",
                "Collector is offline; configuration cannot be applied.",
              )
            : "";
    if (c.mode === "windows" && col?.platform !== "windows")
      error = t(
        "该采集项需要 Windows Collector。",
        "This integration requires a Windows Collector.",
      );
    error ||= D.credentialError();
    if (
      D.wizardBusiness() &&
      !D.state.services.some(
        (s) => s.id === d.service && s.app === d.app && s.env === d.env,
      )
    )
      error = t(
        "应用、服务与环境不匹配。",
        "Application, service and environment do not match.",
      );
    const overlap = D.state.monitors.some(
      (m) =>
        m.id !== w.editId &&
        m.enabled &&
        m.type === w.type &&
        m.collector === d.collector &&
        ((c.mode === "file" && m.path === d.path) ||
          (c.mode === "k8slog" &&
            m.namespace === d.namespace &&
            m.workload === d.workload)),
    );
    if (overlap)
      error = t(
        "与现有日志采集范围重叠，请调整路径或目标。",
        "Log scope overlaps an existing collection. Change the path or target.",
      );
    w.tested = !error;
    D.renderWizard();
    if (error)
      document.getElementById("wizard-test-result").innerHTML = D.note(
        e(error),
        "warning",
      );
  };
  D.commitWizard = () => {
    const w = D.wizard;
    if (!w || w.step !== 4 || !w.tested || !D.canManage()) return;
    const c = D.catalogById[w.type],
      d = w.data,
      signal = D.wizardSignal();
    let target = d.targetId;
    if (D.wizardBusiness()) {
      target = D.state.resources.find(
        (r) => r.kind === "application" && r.service === d.service,
      )?.id;
      if (!target) {
        target = D.id("svc");
        D.state.resources.push({
          id: target,
          name: d.service,
          type: "application",
          kind: "application",
          env: d.env,
          apps: [d.app],
          service: d.service,
          collector: d.collector,
          status: "waiting",
          ...(c.mode === "k8slog"
            ? {
                cluster: "cluster-a",
                namespace: d.namespace,
                workload: d.workload,
              }
            : {}),
        });
      }
    } else if (c.category === "logs")
      target =
        c.mode === "k8slog"
          ? "cluster-a"
          : D.state.collectors.find((x) => x.id === d.collector).host;
    else if (!target) {
      target = D.id("resource");
      D.state.resources.push({
        id: target,
        name: d.name,
        type: c.id,
        kind: c.category,
        endpoint: d.endpoint,
        env: d.env,
        apps: d.apps.slice(),
        services: d.services.slice(),
        collector: d.collector,
        status: "waiting",
      });
    }
    const existing =
        w.editId && D.state.monitors.find((m) => m.id === w.editId),
      id = existing?.id || D.id("monitor");
    const managedCredentialId = D.commitCredential(id, d);
    const monitor = {
      ...d,
      id,
      name: d.name,
      en: d.name,
      type: c.id,
      target,
      collector: d.collector,
      apps: D.wizardBusiness() ? [d.app] : d.apps,
      services: D.wizardBusiness() ? [d.service] : d.services,
      managedCredentialId,
      signal,
      interval:
        c.mode === "push" ? "push" : signal === "logs" ? "stream" : d.interval,
      enabled: true,
      status: "waiting",
      revision: (existing?.revision || 0) + 1,
    };
    if (target && !D.wizardBusiness() && c.category !== "logs") {
      const resource = D.resource(target);
      resource.apps = d.apps.slice();
      resource.services = d.services.slice();
      D.state.monitors
        .filter((m) => m.target === target)
        .forEach((m) => {
          m.apps = d.apps.slice();
          m.services = d.services.slice();
        });
    }
    if (existing) Object.assign(existing, monitor);
    else D.state.monitors.push(monitor);
    if (c.mode === "push")
      D.state.keys.push({
        id: D.id("key"),
        monitor: id,
        app: d.app,
        service: d.service,
        env: d.env,
        protocol: c.id,
        enabled: true,
        key: `DEMO_ONLY_${id.toUpperCase()}_NOT_VALID`,
      });
    D.audit(
      t("采集配置已保存", "Collection saved"),
      "Collection saved",
      `${d.name} · R${monitor.revision} · ${D.collectorName(d.collector)}`,
    );
    D.close();
    D.ui.category = c.category;
    D.ui.type = "all";
    D.ui.search = "";
    D.navigate("monitoring");
    if (c.mode === "push") D.showKeys(d.app, id);
    else
      D.toast(
        t(
          "已保存示例配置，等待数据。",
          "Demo configuration saved; waiting for data.",
        ),
      );
    D.wizard = null;
  };
  D.showKeys = (app, monitorId) => {
    const keys = D.state.keys.filter(
      (k) => k.app === app && (!monitorId || k.monitor === monitorId),
    );
    D.dialog(
      t("应用服务接入", "Application service ingest"),
      D.note(
        t(
          "以下地址与 Key 仅用于原型展示，无法向真实平台发送数据。",
          "Endpoints and keys below are nonfunctional prototype examples.",
        ),
      ) +
        keys
          .map(
            (k) =>
              `<section class="argus-panel"><div class="argus-panel-head"><h3>${e(D.serviceName(k.service))} · ${D.envName(k.env)}</h3>${D.status(k.enabled ? "healthy" : "paused")}</div><div class="argus-panel-body">${D.kv(
                [
                  ["Endpoint", "https://telemetry.demo.invalid:4318"],
                  [t("应用", "Application"), D.appName(k.app)],
                  [t("接入协议", "Protocol"), e(k.protocol)],
                ],
              )}<p class="argus-key argus-mono argus-mt">${e(k.enabled ? k.key : "REVOKED")}</p><div class="argus-row argus-mt">${D.btn("key-copy", t("复制示例 Key", "Copy demo key"), { id: k.id, disabled: !k.enabled })}${D.btn("key-rotate", t("轮换", "Rotate"), { id: k.id, disabled: !k.enabled || !D.canManage() })}${D.btn("key-revoke", t("撤销", "Revoke"), { id: k.id, disabled: !k.enabled || !D.canManage() })}${k.monitor ? D.btn("simulate-ingest", t("模拟 SDK 上报", "Simulate SDK ingest"), { id: k.monitor, primary: true, disabled: !k.enabled }) : ""}</div><pre class="argus-console argus-mt">${e(k.protocol === "otlp" || k.protocol === "sdklog" ? `OTEL_SERVICE_NAME=${D.serviceName(k.service)}\nOTEL_EXPORTER_OTLP_ENDPOINT=https://telemetry.demo.invalid:4318\nOTEL_EXPORTER_OTLP_HEADERS=api-key=${k.enabled ? k.key : "REVOKED"}\nOTEL_RESOURCE_ATTRIBUTES=deployment.environment.name=${k.env}` : t("原生协议需要对应 SDK 配置与接入认证适配；此处展示凭据生命周期。", "Native protocols require SDK-specific setup and authentication adapters; this demo illustrates credential lifecycle."))}</pre></div></section>`,
          )
          .join("") +
        (keys.length
          ? ""
          : D.empty(
              t("暂无接入 Key", "No ingest keys"),
              t(
                "新建 Trace 或 SDK 日志接入后生成。",
                "Create a trace or SDK log integration to issue a key.",
              ),
            )),
      "",
      true,
    );
  };
  D.showNewApp = () => {
    D.dialog(
      t("新建应用", "New application"),
      `<form id="simple-form" class="argus-form">${D.input("name", t("应用名称", "Application name"), "", "required")}${D.input("owner", t("负责人 / 团队", "Owner / team"), "", "required")}${D.input("description", t("用途说明", "Description"), "")}<div class="argus-full">${D.note(t("应用用于组织服务与业务遥测。创建后登记服务，再配置 Trace 与日志。", "Applications organize services and business telemetry. Register services before configuring traces and logs."))}</div></form>`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("app-create", t("创建应用", "Create application"), {
          primary: true,
        }),
    );
  };
  D.showNewService = (app) =>
    D.dialog(
      t("登记服务", "Register service"),
      `<form id="simple-form" class="argus-form" data-app="${app}">${D.input("name", t("服务名称", "Service name"), "", 'required pattern="[A-Za-z0-9._-]+" placeholder="order-api"')}${D.select(
        "env",
        t("环境", "Environment"),
        [
          ["production", D.envName("production")],
          ["staging", D.envName("staging")],
        ],
        "production",
      )}${D.select(
        "runtime",
        t("技术栈", "Runtime"),
        ["Go", "Java", ".NET", "Node.js", "Python", "Other"].map((x) => [x, x]),
        "Go",
      )}</form>`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("service-create", t("登记服务", "Register service"), {
          primary: true,
        }),
    );
  D.showScale = (id) => {
    const w = D.state.workloads.find((x) => x.id === id);
    D.dialog(
      t("调整副本", "Scale workload"),
      D.note(`${e(w.namespace)} / ${e(w.name)} · R${w.revision}`) +
        `<form id="simple-form" data-id="${id}">${D.input("replicas", t("期望副本数", "Desired replicas"), w.replicas, 'required type="number" min="0" max="20"')}</form>`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("scale-preview", t("预览变更", "Preview change"), {
          primary: true,
        }),
    );
  };
  D.workloadYaml = (w) =>
    `apiVersion: apps/v1\nkind: ${w.kind}\nmetadata:\n  name: ${w.name}\n  namespace: ${w.namespace}\nspec:\n${w.kind === "DaemonSet" ? "" : `  replicas: ${w.replicas}\n`}  selector:\n    matchLabels:\n      app: ${w.name}\n  template:\n    metadata:\n      labels:\n        app: ${w.name}\n    spec:\n      containers:\n        - name: app\n          image: ${w.image}\n`;
  D.showYaml = (id) => {
    const w = D.state.workloads.find((x) => x.id === id);
    D.dialog(
      t("工作负载 YAML", "Workload YAML"),
      D.note(
        t(
          "原型支持修改 replicas 与 image 并预览差异；其他字段在此演示中保持原值。",
          "The prototype supports changes to replicas and image with a diff preview. Other fields are retained.",
        ),
      ) +
        `<form id="simple-form" data-id="${id}"><label class="argus-field"><span>YAML · R${w.revision}</span><textarea name="yaml" class="argus-code" spellcheck="false">${e(D.workloadYaml(w))}</textarea></label></form>`,
      D.btn("close", t("关闭", "Close")) +
        `<span class="argus-grow"></span>` +
        D.btn("yaml-preview", t("预览变更", "Preview change"), {
          primary: true,
        }),
    );
  };
  D.showK8sCreate = (kind) => {
    let fields = D.input(
      "name",
      t("名称", "Name"),
      "",
      'required pattern="[a-z0-9]([a-z0-9-]*[a-z0-9])?"',
    );
    if (kind !== "Namespace")
      fields += D.select(
        "namespace",
        "Namespace",
        D.state.namespaces.map((n) => [n, n]),
        D.ui.namespace === "all" ? "commerce" : D.ui.namespace,
      );
    if (["Deployment", "StatefulSet", "DaemonSet"].includes(kind))
      fields +=
        D.input(
          "image",
          t("镜像", "Image"),
          "registry.example.test/app:1.0",
          "required",
        ) +
        (kind === "DaemonSet"
          ? `<input type="hidden" name="replicas" value="3"><div class="argus-note">${t("DaemonSet 随符合条件的节点调度；示例为 3 个节点。", "DaemonSet follows eligible nodes; this fixture has 3 nodes.")}</div>`
          : D.input(
              "replicas",
              t("副本", "Replicas"),
              1,
              'type="number" min="0" max="20" required',
            )) +
        D.pickerField(
          "k8s-application-picker",
          t("关联应用（可选）", "Application (optional)"),
          t("不关联应用", "No application"),
          "select-k8s-application",
        ) +
        '<input type="hidden" name="app" value="">';
    else if (kind !== "Namespace")
      fields += D.input(
        "value",
        kind === "PVC"
          ? t("申请容量", "Storage request")
          : t("配置内容", "Configuration"),
        kind === "PVC" ? "20Gi" : "",
        "required",
      );
    D.dialog(
      `${t("创建", "Create")} ${kind}`,
      `<form id="simple-form" data-kind="${kind}" class="argus-form">${fields}</form>`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("k8s-create-preview", t("预览创建", "Preview creation"), {
          primary: true,
        }),
    );
  };
})();
