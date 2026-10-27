(() => {
  const D = window.ArgusDemo,
    t = (a, b) => D.t(a, b),
    e = D.e;
  D.pages.dashboards = D.dashboardView;
  D.pages.references = () =>
    D.title(
      t("Grafana 仪表盘参考表", "Grafana dashboard references"),
      t(
        "按监控对象选择参考，核对原始指标来源，再转换成 Argus 模板。",
        "Choose references by target, verify metric dependencies, then adapt them into Argus templates.",
      ),
      D.tag(t("2026-10-18 已核对页面", "Pages checked 2026-10-18")),
    ) +
    D.note(
      t(
        "下表是设计参考，不是已导入或已经适配完成的仪表盘。公式、指标名、标签、单位和 Recording Rules 需逐项验证；日志继续使用 Argus KQL，Trace 继续使用现有查询引擎。",
        "These are design references, not imported or validated Argus dashboards. Verify queries, metric names, labels, units and recording rules. Logs retain Argus KQL; traces retain the existing query engine.",
      ),
    ) +
    `<div class="argus-toolbar">${D.input("ref-search", t("搜索对象 / 仪表盘", "Search target / dashboard"), D.ui.refSearch, 'placeholder="MySQL / Kubernetes / logs"')}</div>` +
    D.table(
      [
        t("监控对象", "Target"),
        t("参考仪表盘 / ID", "Reference / ID"),
        t("原始采集依赖", "Original dependency"),
        t("Argus 适配重点", "Argus adaptation"),
      ],
      D.references
        .filter((r) =>
          `${r.target} ${r.title} ${r.id}`
            .toLowerCase()
            .includes(D.ui.refSearch.toLowerCase()),
        )
        .map((r) => [
          e(r.target),
          `<a class="argus-ref-title" href="${e(r.url)}" target="_blank" rel="noopener noreferrer">${e(r.title)} ↗</a><small>${e(r.id)}</small>`,
          e(r.dependency),
          e(t(r.note, r.noteEn)),
        ]),
    );
  const goDashboard = (resource = null, app = null, template = "mixed") => {
    D.ui.component = null;
    D.ui.filters = {
      app: app || "all",
      env: resource
        ? D.environmentIds(resource).length === 1
          ? D.environmentIds(resource)[0]
          : "all"
        : "production",
      kind: "all",
      service: "all",
      cluster: "all",
      namespace: "all",
      workload: "all",
      source: "all",
      time: "1h",
      dependencies: true,
      ids: resource ? [resource.id] : null,
    };
    D.ui.dashboard = template;
    D.close();
    D.navigate("dashboards");
  };
  const simpleData = () => {
    const form = document.getElementById("simple-form");
    if (!form?.reportValidity()) return null;
    return { form, data: Object.fromEntries(new FormData(form)) };
  };
  const showMonitor = (id) => {
    const m = D.state.monitors.find((x) => x.id === id);
    if (!m) return;
    const resource = D.resource(m.target);
    D.dialog(
      D.n(m),
      D.kv([
        [t("状态", "Status"), D.status(m.enabled ? m.status : "paused")],
        [t("目标资源", "Target"), e(resource?.name || m.target)],
        [
          t("执行 Collector", "Executor Collector"),
          e(D.collectorName(m.collector)),
        ],
        [t("类型", "Integration"), e(D.catalogById[m.type]?.name)],
        [t("信号", "Signal"), e(m.signal)],
        [t("归属应用", "Applications"), D.appTags(m.apps)],
        [t("环境归属", "Environment association"), e(D.environmentLabel(m))],
        [t("配置版本", "Revision"), `R${m.revision}`],
        [
          t("采集范围", "Scope"),
          e(m.path || m.workload || resource?.endpoint || m.service || "—"),
        ],
      ]) +
        `<hr class="argus-divider">${D.note(m.status === "error" ? t("示例诊断：目标可达，但只读监控账号缺少必要权限。", "Sample diagnosis: the target is reachable, but the monitoring credential lacks permissions.") : m.status === "waiting" ? t("配置已保存，尚未收到数据。", "Configuration saved; no data received yet.") : t("示例状态仅用于讨论配置与交互。", "Sample status is for design review."))}<div class="argus-row">${D.btn("monitor-edit", t("编辑配置", "Edit configuration"), { id, primary: true, disabled: !D.canManage() })}${D.btn("monitor-dashboard", t("打开仪表盘", "Open dashboard"), { id })}${D.catalogById[m.type]?.mode === "push" ? D.btn("monitor-key", t("接入配置", "Ingest setup"), { id, disabled: !D.canManage() }) : ""}</div>`,
      "",
      true,
    );
  };
  const showResource = (id) => {
    const r = D.resource(id);
    if (!r || !D.authorized(r))
      return D.toast(t("未获得该资源授权", "Resource access not granted"));
    D.dialog(
      r.name,
      D.kv([
        [t("资源类型", "Type"), D.kindName(r.kind) + " · " + e(r.type)],
        [t("目标地址", "Endpoint"), e(r.endpoint || r.workload || "—")],
        [t("环境归属", "Environment association"), e(D.environmentLabel(r))],
        [t("关联应用", "Applications"), D.appTags(r.apps)],
        [t("执行 Collector", "Executor"), e(D.collectorName(r.collector))],
        [t("状态", "Status"), D.status(r.status)],
      ]) +
        `<hr class="argus-divider">${D.note(t("监控目标与执行 Collector 分别记录；更换采集位置不会改变目标资源 ID。", "The target and Collector are recorded separately. Moving collection does not change target identity."))}<div class="argus-row">${D.btn("resource-dashboard", t("打开仪表盘", "Open dashboard"), { id, primary: true })}${D.catalogById[r.type] ? D.btn("resource-monitor", t("新增采集", "Add collection"), { id, disabled: !D.canManage() }) : ""}${D.btn("resource-link", t("关联应用", "Link applications"), { id, disabled: !D.canManage() })}</div>`,
      "",
      true,
    );
  };
  const showTrace = (id, expanded = false) => {
    const r = D.traces.find((x) => x.id === id);
    if (!r || (!D.canManage() && r.app !== "order"))
      return D.toast(t("链路不在授权范围内", "Trace outside authorized scope"));
    const names = [
      r.operation,
      "middleware.auth",
      "redis GET orders",
      "mysql SELECT orders",
      "payment client request",
    ];
    if (expanded && D.canManage()) names.push("billing-api · POST /payments");
    D.dialog(
      t("链路详情", "Trace details"),
      `<div class="argus-row argus-between argus-mb"><h2>${e(r.operation)}</h2>${D.tag(`${r.duration} ms`, r.error ? "danger" : "success")}</div>${D.kv(
        [
          ["Trace ID", e(r.id)],
          [
            t("应用 / 服务", "Application / service"),
            `${D.appName(r.app)} / ${e(r.service)}`,
          ],
          [t("环境", "Environment"), D.envName(r.env)],
        ],
      )}<hr class="argus-divider">${names.map((n, i) => `<div class="argus-span"><span class="argus-mono">${"↳ ".repeat(Math.min(i, 2))}${e(n)}</span><div class="argus-span-bar" style="width:${100 - i * 11}%;margin-left:${i * 2}%"></div></div>`).join("")}<p class="argus-small argus-muted argus-mt">${t("示例链路片段 · 保留来源身份与已接收样本口径", "Sample trace excerpt · source identity and received-sample basis retained")}</p><div class="argus-row argus-mt">${D.btn("trace-logs", t("查看关联日志", "View correlated logs"), { id, primary: true })}${D.btn("trace-expand", t("展开跨应用链路", "Expand cross-application trace"), { id })}</div>${expanded && !D.canManage() ? D.note(t("当前无其他应用授权，仅展示已授权部分。", "Other applications are not authorized; only permitted spans are shown."), "warning") : ""}`,
      "",
      true,
    );
  };
  const linkDialog = (id, isApp) => {
    D.close();
    if (!isApp) {
      const resource = D.resource(id);
      D.openPicker({
        kind: "apps",
        title: t("关联应用", "Link applications"),
        multiple: true,
        selected: resource.apps,
        apply: (ids) =>
          D.confirm(
            t("更新应用关联", "Update application links"),
            D.note(D.appTags(ids)),
            () => {
              resource.apps = ids;
              resource.services = (resource.services || []).filter((sid) =>
                D.state.services.some(
                  (s) => s.id === sid && ids.includes(s.app),
                ),
              );
              D.state.monitors
                .filter(
                  (m) =>
                    m.target === id && !["logs", "traces"].includes(m.signal),
                )
                .forEach((m) => {
                  m.apps = ids.slice();
                  m.services = resource.services.slice();
                });
              D.audit(
                "应用关联已更新",
                "Application links updated",
                resource.name,
              );
              D.render();
            },
          ),
      });
      return;
    }
    const records = isApp
      ? D.state.resources.filter((r) => r.kind !== "application")
      : D.state.apps;
    const current = isApp
      ? D.state.resources.filter((r) => r.apps.includes(id)).map((r) => r.id)
      : D.resource(id).apps;
    D.dialog(
      isApp
        ? t("管理应用依赖", "Manage application dependencies")
        : t("关联应用", "Link applications"),
      `<div class="argus-select-list" id="link-list" data-id="${id}" data-is-app="${isApp}">${records.map((r) => `<label><input type="checkbox" name="link-id" value="${r.id}" ${current.includes(r.id) ? "checked" : ""}><div><strong>${e(isApp ? r.name : D.n(r))}</strong>${isApp ? `<div class="argus-small argus-muted">${D.kindName(r.kind)} · ${D.environmentLabel(r)}</div>` : ""}</div></label>`).join("")}</div>${D.note(t("关联不会授予资源访问权；同一资源可以关联多个应用。", "Links do not grant access. A resource can be linked to multiple applications."))}`,
      D.btn("close", t("取消", "Cancel")) +
        `<span class="argus-grow"></span>` +
        D.btn("links-preview", t("预览关联", "Preview links"), {
          primary: true,
        }),
    );
  };
  const workloadCommit = (w, patch, title) => {
    w.previous = { replicas: w.replicas, image: w.image };
    Object.assign(w, patch);
    w.ready = w.replicas;
    w.revision++;
    D.audit(title, title, `${w.namespace}/${w.name} · R${w.revision}`);
    D.render();
    D.toast(t("模拟变更已完成", "Simulated change completed"));
  };
  const actions = {
    navigate: (id) => D.navigate(id),
    locale: () => {
      D.state.locale = D.state.locale === "zh" ? "en" : "zh";
      D.save();
      D.render();
    },
    theme: () => {
      D.state.theme = D.state.theme === "light" ? "dark" : "light";
      D.save();
      D.render();
    },
    role: () => {
      D.close();
      D.wizard = null;
      D.state.role = D.canManage() ? "viewer" : "admin";
      D.ui.app = "order";
      D.save();
      D.render();
    },
    close: (_id, el) => {
      const target = el.dataset.dialog;
      target ? document.getElementById(target).close() : D.close();
    },
    confirm: () => {
      const commit = D.pending;
      D.pending = null;
      D.close();
      commit?.();
    },
    reset: () => {
      D.state = D.seed();
      D.save();
      D.ui.app = "order";
      D.ui.category = "all";
      D.ui.type = "all";
      D.ui.search = "";
      D.navigate("monitoring");
      D.toast(t("示例已重置", "Demo reset"));
    },
    audit: () => D.dialog(t("操作记录", "Activity"), D.auditView(), "", true),
    category: (id) => {
      D.ui.category = id;
      D.ui.type = "all";
      D.render();
    },
    "monitor-type": (id) => {
      D.ui.type = id;
      D.render();
    },
    catalog: () => D.startWizard(),
    "new-monitor": () =>
      D.startWizard(
        D.ui.type !== "all"
          ? D.ui.type
          : D.catalog.find((c) => c.category === D.ui.category)?.id || "mysql",
      ),
    "new-log": () => D.startWizard("filelog"),
    "new-trace": () => D.startWizard("otlp"),
    "wizard-type": (id) => {
      const old = D.wizard;
      D.startWizard(id, { app: old.data.app, service: old.data.service });
    },
    "wizard-next": () => {
      if (D.wizard.step === 2 && !D.captureWizard(true)) return;
      if (D.wizard.step === 3 && !D.wizard.tested) return;
      D.wizard.step++;
      D.renderWizard();
    },
    "wizard-back": () => {
      if (D.wizard.step === 2) D.captureWizard();
      D.wizard.step--;
      if (D.wizard.step < 3) D.wizard.tested = false;
      D.renderWizard();
    },
    "wizard-test": () => D.testWizard(),
    "select-wizard-apps": () => D.openWizardSelection("apps"),
    "select-wizard-services": () => D.openWizardSelection("services"),
    "select-dashboard-app": () => D.openScopeSelection("apps"),
    "select-dashboard-service": () => D.openScopeSelection("services"),
    "select-explorer-app": (page) => D.openExplorerApplication(page),
    "select-k8s-application": () => {
      const input = document.querySelector('#simple-form input[name="app"]');
      D.openPicker({
        kind: "apps",
        title: t("关联应用", "Link application"),
        multiple: false,
        selected: input.value ? [input.value] : [],
        apply: (ids) => {
          input.value = ids[0] || "";
          document.querySelector(
            '#simple-form [name="k8s-application-picker"] span',
          ).textContent = ids.length
            ? D.appName(ids[0])
            : t("不关联应用", "No application");
        },
      });
    },
    "wizard-create": () => D.commitWizard(),
    "monitor-detail": showMonitor,
    "monitor-edit": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      D.close();
      D.startWizard(
        m.type,
        {
          ...m,
          app: m.apps[0] || "",
          targetId: m.target,
          endpoint: D.resource(m.target)?.endpoint || "",
          skipType: true,
        },
        id,
      );
    },
    "monitor-toggle": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      D.confirm(
        t("变更采集状态", "Change collection state"),
        D.kv([
          [t("采集", "Collection"), e(D.n(m))],
          [
            t("变更", "Change"),
            m.enabled
              ? t("开启 → 停采；保留历史", "Enabled → paused; retain history")
              : t("停采 → 开启；等待数据", "Paused → enabled; wait for data"),
          ],
          [t("配置版本", "Revision"), `R${m.revision} → R${m.revision + 1}`],
        ]),
        () => {
          m.enabled = !m.enabled;
          m.status = m.enabled ? "waiting" : "paused";
          m.revision++;
          D.audit("采集启停变更", "Collection state changed", D.n(m));
          D.render();
        },
      );
    },
    "monitor-delete": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      D.confirm(
        t("删除采集配置", "Delete collection"),
        D.note(e(D.n(m))) +
          D.note(
            t(
              "只移除示例采集配置，资源对象与历史示例数据保留。",
              "Remove the sample collection; target and sample history remain.",
            ),
          ),
        () => {
          D.state.monitors = D.state.monitors.filter((x) => x.id !== id);
          D.state.monitorCredentials = D.state.monitorCredentials.filter(
            (x) => x.ownerId !== id,
          );
          D.audit("采集配置已删除", "Collection deleted", m.name);
          D.render();
        },
      );
    },
    "monitor-test": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      D.dialog(
        t("采集诊断（模拟）", "Collection diagnostics (demo)"),
        D.note(
          m.status === "error"
            ? t(
                "目标可达；监控账号权限不足。请修改凭据后重新校验。",
                "Target reachable; credential lacks required permissions. Edit the credential and validate again.",
              )
            : t(
                "配置与路由检查通过（模拟）。此结果不代表真实数据采集完成。",
                "Simulated configuration and route checks passed. This does not verify real collection.",
              ),
          m.status === "error" ? "warning" : "success",
        ),
      );
    },
    "monitor-dashboard": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      goDashboard(
        D.resource(m.target),
        null,
        m.signal === "traces" ? "application" : "mixed",
      );
    },
    "monitor-key": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      D.close();
      D.showKeys(m.apps[0], id);
    },
    "app-list": () => {
      D.ui.app = null;
      D.render();
    },
    "app-open": (id) => {
      D.ui.app = id;
      D.ui.appTab = "overview";
      D.navigate("applications");
    },
    "app-tab": (id) => {
      D.ui.appTab = id;
      D.render();
    },
    "new-app": () => D.showNewApp(),
    "app-create": () => {
      const x = simpleData();
      if (!x) return;
      const { data } = x;
      if (
        D.state.apps.some(
          (a) => a.name.toLowerCase() === data.name.trim().toLowerCase(),
        )
      )
        return D.toast(t("应用名称已存在", "Application name already exists"));
      const id = D.id("app");
      D.state.apps.push({
        id,
        ...data,
        en: data.name,
        ownerEn: data.owner,
        descriptionEn: data.description,
      });
      D.audit("应用已创建", "Application created", data.name);
      D.close();
      D.ui.app = id;
      D.ui.appTab = "services";
      D.render();
    },
    "new-service": (id) => D.showNewService(id),
    "service-create": () => {
      const x = simpleData();
      if (!x) return;
      const app = x.form.dataset.app;
      if (
        D.state.services.some(
          (s) =>
            s.app === app && s.name === x.data.name && s.env === x.data.env,
        )
      )
        return D.toast(
          t(
            "该环境中已存在同名服务",
            "Service already exists in this environment",
          ),
        );
      D.state.services.push({
        id: D.id("service"),
        app,
        ...x.data,
        instances: 0,
      });
      D.audit("服务已登记", "Service registered", x.data.name);
      D.close();
      D.render();
    },
    "service-access": (id) => {
      const s = D.state.services.find((x) => x.id === id);
      D.startWizard("otlp", {
        app: s.app,
        service: s.id,
        env: s.env,
        skipType: true,
      });
    },
    "app-trace": (id) => D.startWizard("otlp", { app: id, skipType: true }),
    "app-log": (id) => D.startWizard("filelog", { app: id, skipType: true }),
    "app-keys": (id) => D.showKeys(id),
    "app-dashboard": (id) => goDashboard(null, id, "application"),
    "link-resources": (id) => linkDialog(id, true),
    "resource-link": (id) => linkDialog(id, false),
    "links-preview": () => {
      const list = document.getElementById("link-list"),
        id = list.dataset.id,
        isApp = list.dataset.isApp === "true",
        values = [...list.querySelectorAll("input:checked")].map(
          (x) => x.value,
        );
      D.confirm(
        t("更新应用关联", "Update application links"),
        D.note(`${t("选中", "Selected")}: ${values.length}`),
        () => {
          if (isApp)
            D.state.resources.forEach((r) => {
              r.apps = r.apps.filter((a) => a !== id);
              if (values.includes(r.id)) r.apps.push(id);
            });
          else D.resource(id).apps = values;
          D.state.monitors.forEach((m) => {
            if (!["logs", "traces"].includes(m.signal))
              m.apps = D.resource(m.target)?.apps.slice() || [];
          });
          D.audit("应用关联已更新", "Application links updated", id);
          D.render();
        },
      );
    },
    "resource-detail": showResource,
    "resource-monitor": (id) => {
      const r = D.resource(id);
      D.close();
      D.startWizard(r.type, {
        targetId: r.id,
        endpoint: r.endpoint || "",
        apps: r.apps.slice(),
        services: (r.services || []).slice(),
        collector: r.collector,
        skipType: true,
      });
    },
    "resource-kind": (id) => {
      D.ui.resourceKind = id;
      D.render();
    },
    "resource-dashboard": (id) =>
      goDashboard(
        D.resource(id),
        null,
        D.resource(id)?.kind === "host" ? "host" : "mixed",
      ),
    "host-tab": (id) => {
      D.ui.hostTab = id;
      D.render();
    },
    "host-toggle": (id) => {
      const c = D.state.hostComponents[D.ui.host];
      D.confirm(
        t("更新主机采集", "Update host collection"),
        D.note(`${e(D.ui.host)} · ${id} · ${c[id] ? "ON → OFF" : "OFF → ON"}`),
        () => {
          c[id] = !c[id];
          D.audit(
            "主机采集已更新",
            "Host collection updated",
            `${D.ui.host} / ${id}`,
          );
          D.render();
        },
      );
    },
    "host-log": (id) =>
      D.startWizard("filelog", {
        collector: D.resource(id).collector,
        skipType: true,
      }),
    "k8s-tab": (id) => {
      D.ui.k8sTab = id;
      D.render();
    },
    "k8s-kind": (id) => {
      D.ui.k8sKind = id;
      D.render();
    },
    "component-toggle": (id) => {
      const c = D.state.components.find((x) => x.id === id);
      if (!c.available) return;
      D.confirm(
        t("更新组件指标采集", "Update component collection"),
        D.kv([
          [t("组件", "Component"), D.n(c)],
          [t("变更", "Change"), c.enabled ? "ON → OFF" : "OFF → ON"],
          [
            t("仪表盘", "Dashboard"),
            t("保留历史与默认模板", "Retain history and default template"),
          ],
        ]),
        () => {
          c.enabled = !c.enabled;
          D.audit("组件监控已更新", "Component monitoring updated", c.name);
          D.render();
        },
      );
    },
    "component-dashboard": (id) => {
      goDashboard(D.resource("cluster-a"), null, "kubernetes");
      D.ui.component = id;
      D.render();
    },
    "cluster-dashboard": () =>
      goDashboard(D.resource("cluster-a"), null, "kubernetes"),
    "k8s-log": () => D.startWizard("k8slog", { skipType: true }),
    "workload-log": (id) => {
      const w = D.state.workloads.find((x) => x.id === id);
      D.startWizard("k8slog", {
        namespace: w.namespace,
        kind: w.kind,
        workload: w.name,
        app: w.app,
        service: w.service,
        logClass: w.app ? "business" : "system",
        skipType: true,
      });
    },
    "k8s-scale": D.showScale,
    "scale-preview": () => {
      const x = simpleData();
      if (!x) return;
      const w = D.state.workloads.find((y) => y.id === x.form.dataset.id),
        replicas = Number(x.data.replicas);
      D.confirm(
        t("副本变更预览", "Scale preview"),
        D.kv([
          [t("工作负载", "Workload"), e(w.name)],
          [t("副本", "Replicas"), `${w.replicas} → ${replicas}`],
        ]),
        () => workloadCommit(w, { replicas }, "Scale workload"),
      );
    },
    "k8s-restart": (id) => {
      const w = D.state.workloads.find((x) => x.id === id);
      D.confirm(
        t("滚动重启预览", "Rollout restart preview"),
        D.note(
          `${e(w.namespace)}/${e(w.name)} · ${w.replicas} ${t("个副本", "replicas")}`,
        ),
        () => workloadCommit(w, {}, "Rollout restart"),
      );
    },
    "k8s-yaml": D.showYaml,
    "yaml-preview": () => {
      const x = simpleData();
      if (!x) return;
      const w = D.state.workloads.find((y) => y.id === x.form.dataset.id),
        replicas =
          w.kind === "DaemonSet"
            ? ["", String(w.replicas)]
            : x.data.yaml.match(/^\s{2}replicas:\s*(\d+)\s*$/m),
        image = x.data.yaml.match(/^\s+image:\s*(\S+)\s*$/m);
      if (!replicas || !image || Number(replicas[1]) > 20)
        return D.toast(
          t(
            "请输入有效的 replicas（0–20）和 image",
            "Enter valid replicas (0–20) and image",
          ),
        );
      const expected = D.workloadYaml({
        ...w,
        replicas: Number(replicas[1]),
        image: image[1],
      });
      if (x.data.yaml.trim() !== expected.trim())
        return D.toast(
          t(
            "原型仅允许修改 replicas 和 image，其他字段须保持原值",
            "Only replicas and image are editable in this prototype",
          ),
        );
      D.confirm(
        t("YAML 变更预览", "YAML change preview"),
        D.kv([
          ["replicas", `${w.replicas} → ${replicas[1]}`],
          ["image", `${e(w.image)} → ${e(image[1])}`],
        ]),
        () =>
          workloadCommit(
            w,
            { replicas: Number(replicas[1]), image: image[1] },
            "Apply workload YAML",
          ),
      );
    },
    "k8s-rollback": (id) => {
      const w = D.state.workloads.find((x) => x.id === id);
      if (!w.previous) return;
      D.confirm(
        t("回滚预览", "Rollback preview"),
        D.kv([
          ["image", `${e(w.image)} → ${e(w.previous.image)}`],
          ["replicas", `${w.replicas} → ${w.previous.replicas}`],
        ]),
        () => workloadCommit(w, { ...w.previous }, "Rollback workload"),
      );
    },
    "k8s-delete": (id) => {
      const w = D.state.workloads.find((x) => x.id === id);
      D.confirm(
        t("删除工作负载", "Delete workload"),
        D.note(
          `${e(w.namespace)}/${e(w.name)} · ${t("对应采集规则将失去目标，历史保留。", "Collection rules lose their target; history remains.")}`,
        ),
        () => {
          D.state.workloads = D.state.workloads.filter((x) => x.id !== id);
          D.state.monitors
            .filter(
              (m) =>
                m.type === "k8slog" &&
                m.namespace === w.namespace &&
                m.workload === w.name,
            )
            .forEach((m) => (m.status = "waiting"));
          D.audit("工作负载已删除", "Workload deleted", w.name);
          D.render();
        },
      );
    },
    "k8s-create": D.showK8sCreate,
    "k8s-create-preview": () => {
      const x = simpleData();
      if (!x) return;
      const kind = x.form.dataset.kind,
        d = x.data;
      const duplicate =
        kind === "Namespace"
          ? D.state.namespaces.includes(d.name)
          : [...D.state.workloads, ...D.state.objects].some(
              (o) =>
                o.kind === kind &&
                o.name === d.name &&
                o.namespace === d.namespace,
            );
      if (duplicate)
        return D.toast(
          t("同一范围内已存在该名称", "Name already exists in this scope"),
        );
      D.confirm(
        t("创建资源预览", "Create resource preview"),
        D.kv(Object.entries(d).map(([k, v]) => [e(k), e(v)])),
        () => {
          if (kind === "Namespace") D.state.namespaces.push(d.name);
          else if (["Deployment", "StatefulSet", "DaemonSet"].includes(kind))
            D.state.workloads.push({
              id: D.id("workload"),
              kind,
              ...d,
              replicas: Number(d.replicas),
              ready: Number(d.replicas),
              revision: 1,
              status: "healthy",
              service: "",
            });
          else D.state.objects.push({ id: D.id("object"), kind, ...d });
          D.audit(
            "Kubernetes 资源已创建",
            "Kubernetes resource created",
            `${kind}/${d.name}`,
          );
          D.render();
        },
      );
    },
    "workload-detail": (id) => {
      const w = D.state.workloads.find((x) => x.id === id);
      D.dialog(
        w.name,
        D.kv([
          ["Kind", e(w.kind)],
          ["Namespace", e(w.namespace)],
          [t("就绪副本", "Ready replicas"), `${w.ready}/${w.replicas}`],
          [
            t("应用", "Application"),
            w.app ? D.appName(w.app) : t("基础设施", "Infrastructure"),
          ],
          ["Image", e(w.image)],
        ]) +
          `<hr class="argus-divider"><div class="argus-row">${D.btn("k8s-yaml", t("编辑 YAML", "Edit YAML"), { id })}${D.btn("workload-log", t("配置日志", "Configure logs"), { id })}</div>`,
        "",
        true,
      );
    },
    "pod-detail": (id) => {
      const [wid, index] = id.split(":"),
        w = D.state.workloads.find((x) => x.id === wid);
      D.dialog(
        `${w.name}-${index}`,
        D.kv([
          ["Namespace", e(w.namespace)],
          ["Owner", e(w.name)],
          ["Node", `worker-sh-0${(Number(index) % 3) + 1}`],
          ["Status", "Running"],
        ]) +
          D.note(
            t(
              "示例事件：Scheduled → Pulled → Created → Started",
              "Sample events: Scheduled → Pulled → Created → Started",
            ),
          ),
        "",
        true,
      );
    },
    "pod-logs": (id) => {
      const w = D.state.workloads.find((x) => x.id === id.split(":")[0]);
      D.dialog(
        t("容器日志（模拟）", "Container logs (demo)"),
        `<pre class="argus-console">${e(`14:32:08 INFO ${w.name} started\n14:32:09 INFO health check OK\n14:32:11 INFO request completed`)}</pre>`,
        "",
        true,
      );
    },
    "pod-terminal": (id) => {
      D.dialog(
        t("容器终端（模拟）", "Container terminal (demo)"),
        D.note(
          t(
            "此终端只演示会话交互，命令不会发送到真实集群。可输入 help、pwd、ls、exit。",
            "This terminal simulates a session. No commands reach a cluster. Try help, pwd, ls, exit.",
          ),
        ) +
          `<pre class="argus-console" id="terminal-output">$ connected to ${e(id)} (demo)</pre><form id="terminal-form" class="argus-row argus-mt"><label class="argus-field argus-grow"><span>${t("命令", "Command")}</span><input name="command" autocomplete="off" placeholder="help"></label><button class="argus-btn argus-btn--primary" type="submit">${t("运行（模拟）", "Run (demo)")}</button></form>`,
        "",
        true,
      );
    },
    "object-detail": (id) => {
      const o = D.state.objects.find((x) => x.id === id);
      D.dialog(
        o.name,
        D.kv([
          ["Kind", e(o.kind)],
          ["Namespace", e(o.namespace)],
          [
            t("内容", "Content"),
            `<pre class="argus-console">${e(o.value)}</pre>`,
          ],
        ]),
        "",
        true,
      );
    },
    "object-edit": (id) => {
      const o = D.state.objects.find((x) => x.id === id);
      D.close();
      D.dialog(
        t("编辑资源", "Edit resource"),
        `<form id="simple-form" data-id="${id}">${D.input("value", o.kind === "PVC" ? t("新容量（Gi）", "New capacity (Gi)") : t("配置内容", "Configuration"), o.value, "required")}</form>`,
        D.btn("object-preview", t("预览变更", "Preview changes"), {
          primary: true,
        }),
      );
    },
    "object-preview": () => {
      const x = simpleData();
      if (!x) return;
      const o = D.state.objects.find((y) => y.id === x.form.dataset.id);
      if (
        o.kind === "PVC" &&
        (!/^\d+Gi$/.test(x.data.value) ||
          parseInt(x.data.value) <= parseInt(o.value))
      )
        return D.toast(
          t(
            "容量必须以 Gi 表示且大于当前容量",
            "Capacity must be in Gi and greater than current size",
          ),
        );
      D.confirm(
        t("资源变更预览", "Resource change preview"),
        D.kv([
          [t("当前", "Current"), e(o.value)],
          [t("修改为", "New"), e(x.data.value)],
        ]),
        () => {
          o.value = x.data.value;
          D.audit("资源配置已更新", "Resource updated", o.name);
          D.render();
        },
      );
    },
    "object-delete": (id) => {
      const o = D.state.objects.find((x) => x.id === id);
      D.confirm(
        t("删除资源", "Delete resource"),
        D.note(`${e(o.kind)} / ${e(o.name)}`),
        () => {
          D.state.objects = D.state.objects.filter((x) => x.id !== id);
          D.audit("资源已删除", "Resource deleted", o.name);
          D.render();
        },
      );
    },
    "trace-detail": (id) => showTrace(id),
    "trace-expand": (id) => showTrace(id, true),
    "trace-logs": (id) => {
      D.close();
      D.ui.logQuery = id;
      D.ui.logApp = "all";
      D.ui.logLevel = "all";
      D.navigate("logs");
    },
    "key-copy": async (id) => {
      const k = D.state.keys.find((x) => x.id === id);
      try {
        await navigator.clipboard.writeText(k.key);
        D.toast(t("示例 Key 已复制", "Demo key copied"));
      } catch {
        D.toast(
          t(
            "浏览器未开放剪贴板，可手动复制示例文本",
            "Clipboard unavailable; copy the sample text manually",
          ),
        );
      }
    },
    "key-rotate": (id) => {
      const k = D.state.keys.find((x) => x.id === id);
      D.confirm(
        t("轮换接入 Key", "Rotate ingest key"),
        D.note(`${e(k.service)} / ${D.envName(k.env)}`),
        () => {
          k.key = `DEMO_ONLY_ROTATED_${D.id("key").toUpperCase()}`;
          D.audit("Key 已轮换", "Key rotated", k.service);
          D.showKeys(k.app);
          D.render();
        },
      );
    },
    "key-revoke": (id) => {
      const k = D.state.keys.find((x) => x.id === id);
      D.confirm(
        t("撤销接入 Key", "Revoke ingest key"),
        D.note(
          t(
            "撤销后该示例 Key 不能继续模拟上报。",
            "The revoked demo key cannot simulate further ingest.",
          ),
        ),
        () => {
          k.enabled = false;
          D.audit("Key 已撤销", "Key revoked", k.service);
          D.showKeys(k.app);
          D.render();
        },
      );
    },
    "simulate-ingest": (id) => {
      const m = D.state.monitors.find((x) => x.id === id);
      if (!D.state.keys.some((k) => k.monitor === id && k.enabled)) return;
      m.status = "healthy";
      D.audit("收到模拟 SDK 上报", "Simulated SDK data received", m.name);
      D.toast(
        t(
          "已收到模拟上报，采集状态已更新",
          "Simulated ingest received; collection state updated",
        ),
      );
      D.render();
    },
    "select-scope": D.openScopePicker,
    "scope-apply": () => {
      D.ui.filters.ids = document.getElementById("scope-all").checked
        ? null
        : [...document.querySelectorAll('input[name="scope-id"]:checked')].map(
            (x) => x.value,
          );
      D.close();
      D.render();
    },
    "scope-explain": () =>
      D.dialog(
        t("仪表盘筛选逻辑", "Dashboard scope logic"),
        D.kv([
          [
            t("第一层", "Layer 1"),
            t(
              "企业与当前用户数据授权",
              "Enterprise and current data authorization",
            ),
          ],
          [
            t("第二层", "Layer 2"),
            t(
              "时间、环境、应用与明确选择的关联依赖",
              "Time, environment, application and explicit dependencies",
            ),
          ],
          [
            t("第三层", "Layer 3"),
            t(
              "资源类型、目标 ID、服务、K8s 对象、采集来源",
              "Resource type, target IDs, service, Kubernetes objects, source",
            ),
          ],
          [
            t("第四层", "Layer 4"),
            t(
              "面板支持的信号、来源、资源类型与局部条件",
              "Panel signal/source/resource compatibility and local conditions",
            ),
          ],
        ]) +
          D.note(
            t(
              "条件求交，空集合保持为空。执行 Collector 不作为被监控对象身份；应用关联不授予权限。",
              "Scopes intersect; empty stays empty. The Collector is not the monitored identity. Application links do not grant access.",
            ),
          ),
        "",
        true,
      ),
    "panel-scope": (id) => {
      const p = D.dashboardPanels().find((x) => x.id === id),
        rs = D.scopeResources().filter((r) => p.kinds.includes(r.kind));
      D.dialog(
        t(p.zh, p.en),
        D.kv([
          [
            t("适用资源", "Compatible resources"),
            p.kinds.map(D.kindName).join(", "),
          ],
          [
            t("本次对象", "Current targets"),
            rs.map((r) => e(r.name)).join(", ") || t("空集合", "Empty set"),
          ],
          [t("统计口径", "Basis"), p.caption],
        ]),
      );
    },
    "dashboard-reset": () => goDashboard(null, null, D.ui.dashboard),
    "dashboard-template": (id) => {
      D.ui.component = null;
      D.ui.dashboard = id;
      D.ui.filters = {
        app: id === "application" ? "order" : "all",
        env: "production",
        kind:
          id === "host" ? "host" : id === "kubernetes" ? "kubernetes" : "all",
        service: "all",
        cluster: "all",
        namespace: "all",
        workload: "all",
        source: "all",
        time: "1h",
        dependencies: true,
        ids: null,
      };
      D.render();
    },
  };
  document.addEventListener("click", async (event) => {
    const el = event.target.closest("[data-action]");
    if (!el || el.disabled) return;
    document
      .querySelectorAll(".argus-menu[open]")
      .forEach((menu) => (menu.open = false));
    const action = actions[el.dataset.action];
    if (action) await action(el.dataset.id, el);
  });
  document.addEventListener(
    "toggle",
    (event) => {
      if (
        event.target.matches?.("[data-filter-details]") &&
        event.target.isConnected
      ) {
        D.ui.k8sFiltersOpen = event.target.open;
      }
      if (event.target.matches?.(".argus-menu") && event.target.open) {
        const box = event.target.getBoundingClientRect(),
          items = event.target.querySelector(".argus-menu-items");
        items.style.left = `${Math.min(box.right - 180, window.innerWidth - 196)}px`;
        items.style.top = `${Math.min(box.bottom, window.innerHeight - items.offsetHeight - 48)}px`;
      }
    },
    true,
  );
  const renderFocus = (name) => {
    D.render();
    document.querySelector(`[name="${name}"]`)?.focus();
  };
  document.addEventListener("change", (event) => {
    const el = event.target,
      name = el.name,
      value = el.value;
    if (el.dataset.dashboardFilter !== undefined) {
      D.ui.filters[name] = el.type === "checkbox" ? el.checked : value;
      renderFocus(name);
      return;
    }
    if (name === "wizard-category") {
      D.wizard.category = value;
      D.renderWizard();
      return;
    }
    if (name === "wizard-scenario") {
      D.wizard.data.scenario = value;
      D.wizard.tested = false;
      D.renderWizard();
      return;
    }
    if (el.closest("#monitor-form")) {
      D.captureWizard();
      D.wizard.tested = false;
      if (["namespace", "kind", "logClass", "authMode"].includes(name)) {
        if (name === "namespace" || name === "kind")
          D.wizard.data.workload = "";
        if (name === "authMode") D.wizardSecrets = {};
        D.renderWizard();
      }
      return;
    }
    if (el.id === "scope-all")
      document
        .querySelectorAll('input[name="scope-id"]')
        .forEach((x) => (x.checked = el.checked));
    if (name === "scope-id")
      document.getElementById("scope-all").checked = false;
    const mappings = {
      "host-select": "host",
      namespace: "namespace",
      "log-app": "logApp",
      "log-level": "logLevel",
      "trace-app": "traceApp",
    };
    if (mappings[name]) {
      D.ui[mappings[name]] = value;
      renderFocus(name);
    }
  });
  document.addEventListener("input", (event) => {
    const name = event.target.name,
      map = {
        "monitor-search": "search",
        "log-query": "logQuery",
        "ref-search": "refSearch",
      };
    if (!map[name]) return;
    const pos = event.target.selectionStart;
    D.ui[map[name]] = event.target.value;
    D.render();
    const el = document.querySelector(`[name="${name}"]`);
    el?.focus();
    el?.setSelectionRange(pos, pos);
  });
  document.addEventListener("submit", (event) => {
    event.preventDefault();
    if (event.target.id !== "terminal-form") return;
    const input = event.target.elements.command,
      cmd = input.value.trim(),
      output = document.getElementById("terminal-output");
    const answers = {
      help: "help · pwd · ls · exit",
      pwd: "/app",
      ls: "app  config  logs",
      exit: "Demo session closed.",
    };
    output.textContent += `\n$ ${cmd}\n${answers[cmd] || t("模拟终端仅支持 help、pwd、ls、exit。", "Demo terminal supports help, pwd, ls, exit only.")}`;
    input.value = "";
    if (cmd === "exit") input.disabled = true;
  });
  document.addEventListener("keydown", (event) => {
    const tab = event.target.closest('[role="tab"]');
    if (!tab || !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key))
      return;
    event.preventDefault();
    const siblings = [...tab.parentElement.querySelectorAll('[role="tab"]')];
    let index = siblings.indexOf(tab);
    index =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? siblings.length - 1
          : (index + (event.key === "ArrowRight" ? 1 : -1) + siblings.length) %
            siblings.length;
    const action = siblings[index].dataset.action,
      id = siblings[index].dataset.id;
    siblings[index].click();
    document
      .querySelector(`[data-action="${action}"][data-id="${id}"]`)
      ?.focus();
  });
  window.addEventListener("hashchange", () => {
    const page = location.hash.slice(1);
    if (D.pages[page] && D.ui.page !== page) {
      D.ui.page = page;
      D.render();
    }
  });
  const start = location.hash.slice(1);
  if (D.pages[start]) D.ui.page = start;
  D.render();
})();
