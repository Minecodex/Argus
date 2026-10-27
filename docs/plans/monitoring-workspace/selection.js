(() => {
  const D = window.ArgusDemo,
    t = (zh, en) => D.t(zh, en),
    e = D.e;
  D.appEnvironments = (id) => {
    const app = D.state.apps.find((a) => a.id === id);
    return [
      ...new Set(
        [
          ...(app?.environments || []),
          ...D.state.services.filter((s) => s.app === id).map((s) => s.env),
        ].filter(Boolean),
      ),
    ];
  };
  D.environmentIds = (entity, appFilter = "all") => {
    if (!entity) return [];
    const apps = entity.apps?.length
      ? entity.apps
      : entity.app
        ? [entity.app]
        : [];
    const services = entity.service ? [entity.service] : entity.services || [];
    const selectedApps = apps.filter(
      (id) => appFilter === "all" || id === appFilter,
    );
    if (selectedApps.length)
      return [
        ...new Set(
          selectedApps
            .flatMap((id) => {
              const linked = D.state.services.filter(
                (s) => s.app === id && services.includes(s.id),
              );
              return linked.length
                ? linked.map((s) => s.env)
                : D.appEnvironments(id);
            })
            .filter(Boolean),
        ),
      ];
    if (["host", "kubernetes"].includes(entity.kind))
      return entity.env ? [entity.env] : [];
    if (entity.target && entity.target !== entity.id) {
      const target = D.resource(entity.target);
      if (target && ["host", "kubernetes"].includes(target.kind))
        return D.environmentIds(target, appFilter);
    }
    return [];
  };
  D.environmentLabel = (entity) => {
    const envs = D.environmentIds(entity);
    return envs.length
      ? envs.map(D.envName).join(" / ")
      : t("未关联环境", "No environment association");
  };
  D.selectionSummary = (ids, kind) => {
    if (!ids?.length) return t("点击选择", "Select from list");
    const labels = ids.map((id) =>
      kind === "apps"
        ? D.appName(id)
        : `${D.serviceName(id)} · ${D.envName(D.state.services.find((s) => s.id === id)?.env)}`,
    );
    return (
      labels.slice(0, 2).join("、") +
      (labels.length > 2
        ? t(` 等 ${labels.length} 项`, ` + ${labels.length - 2} more`)
        : "")
    );
  };
  D.pickerField = (name, label, summary, action, opts = {}) =>
    `<div class="argus-field"><span id="label-${name}">${e(label)}</span><button type="button" name="${name}" class="argus-btn argus-picker-trigger" data-action="${action}" ${opts.id ? `data-id="${e(opts.id)}"` : ""} aria-labelledby="label-${name}" aria-haspopup="dialog" ${opts.disabled ? "disabled" : ""}><span>${e(summary)}</span>${D.icon("search")}</button>${opts.help ? `<small>${e(opts.help)}</small>` : ""}</div>`;
  D.openPicker = (options) => {
    const dialog = document.getElementById("picker");
    D.picker = {
      ...options,
      selected: new Set(options.selected || []),
      query: "",
      page: 0,
      pageSize: 8,
      opener: document.activeElement,
    };
    dialog.innerHTML = `<div class="argus-dialog-head"><div><h2 id="picker-title">${e(options.title)}</h2><p class="argus-small argus-muted">${options.multiple ? t("可选择多项，跨页保留选择。", "Select multiple items; selection persists across pages.") : t("选择一项后确认。", "Select one item and confirm.")}</p></div>${D.btn("picker-cancel", t("关闭", "Close"), { quiet: true, icon: "close" })}</div><div class="argus-dialog-body"><div class="argus-toolbar">${D.input("picker-query", t("搜索名称、负责人或环境", "Search name, owner or environment"), "", 'autocomplete="off"')}${options.allowAll ? D.btn("picker-all", options.allLabel || t("全部已授权项", "All authorized items")) : ""}<span class="argus-grow"></span><span id="picker-selection-count" class="argus-small argus-muted" role="status"></span></div><div id="picker-rows"></div></div><div class="argus-dialog-foot">${D.btn("picker-cancel", t("取消", "Cancel"))}${!options.required ? D.btn("picker-clear", t("清除选择", "Clear selection"), { quiet: true }) : ""}<span class="argus-grow"></span>${D.btn("picker-apply", t("确认选择", "Apply selection"), { primary: true })}</div>`;
    D.renderPicker();
    dialog.showModal();
    dialog.querySelector('[name="picker-query"]').focus();
  };
  D.pickerRecords = () => {
    const p = D.picker;
    const allowedApps = D.state.apps.filter(
      (a) => D.canManage() || a.id === "order",
    );
    if (p.kind === "apps")
      return [
        ...(p.extraRows || []),
        ...allowedApps.map((a) => ({
          id: a.id,
          name: D.n(a),
          detail: t(a.owner, a.ownerEn),
          env: D.appEnvironments(a.id).map(D.envName).join(" / ") || "—",
          count: D.state.services.filter((s) => s.app === a.id).length,
        })),
      ];
    return D.state.services
      .filter(
        (s) =>
          allowedApps.some((a) => a.id === s.app) &&
          (!p.appIds?.length || p.appIds.includes(s.app)) &&
          (!p.env || p.env === "all" || s.env === p.env),
      )
      .map((s) => ({
        id: s.id,
        name: s.name,
        detail: D.appName(s.app),
        env: D.envName(s.env),
        count: s.runtime,
      }));
  };
  D.renderPicker = () => {
    const p = D.picker,
      query = p.query.toLocaleLowerCase();
    const records = D.pickerRecords().filter((r) =>
      `${r.name} ${r.detail} ${r.env} ${r.id}`
        .toLocaleLowerCase()
        .includes(query),
    );
    const pages = Math.max(1, Math.ceil(records.length / p.pageSize));
    p.page = Math.min(p.page, pages - 1);
    const rows = records.slice(p.page * p.pageSize, (p.page + 1) * p.pageSize);
    document.getElementById("picker-rows").innerHTML =
      D.table(
        [
          t("名称", "Name"),
          p.kind === "apps"
            ? t("负责人", "Owner")
            : t("所属应用", "Application"),
          t("环境", "Environment"),
          p.kind === "apps" ? t("服务数", "Services") : t("技术栈", "Runtime"),
          t("选择", "Select"),
        ],
        rows.map((r) => [
          e(r.name),
          e(r.detail),
          e(r.env),
          e(r.count),
          D.btn(
            "picker-toggle",
            p.selected.has(r.id)
              ? t("已选择", "Selected")
              : t("选择", "Select"),
            {
              id: r.id,
              primary: p.selected.has(r.id),
              extra: `aria-pressed="${p.selected.has(r.id)}"`,
            },
          ),
        ]),
      ) +
      `<div class="argus-row argus-between"><span class="argus-small argus-muted">${t(`共 ${records.length} 项 · 第 ${p.page + 1} / ${pages} 页`, `${records.length} items · page ${p.page + 1} / ${pages}`)}</span><div class="argus-row">${D.btn("picker-prev", t("上一页", "Previous"), { disabled: p.page === 0 })}${D.btn("picker-next", t("下一页", "Next"), { disabled: p.page + 1 >= pages })}</div></div>`;
    document.getElementById("picker-selection-count").textContent =
      p.selected.has("all")
        ? p.allLabel || t("全部已授权项", "All authorized items")
        : t(`已选择 ${p.selected.size} 项`, `${p.selected.size} selected`);
    document.querySelector('#picker [data-action="picker-apply"]').disabled =
      Boolean(p.required && !p.selected.size);
  };
  D.pickerAction = (action, id) => {
    const p = D.picker;
    if (!p) return;
    if (action === "picker-toggle") {
      const wasSelected = p.selected.has(id);
      if (!p.multiple) p.selected.clear();
      p.selected.delete("all");
      if (wasSelected) p.selected.delete(id);
      else p.selected.add(id);
    }
    if (action === "picker-clear") p.selected.clear();
    if (action === "picker-all") p.selected = new Set(["all"]);
    if (action === "picker-prev") p.page = Math.max(0, p.page - 1);
    if (action === "picker-next") p.page++;
    if (action === "picker-cancel" || action === "picker-apply") {
      const values = [...p.selected],
        apply = p.apply,
        opener = p.opener;
      document.getElementById("picker").close();
      D.picker = null;
      if (action === "picker-apply") apply(values);
      else opener?.focus();
      return;
    }
    D.renderPicker();
  };
  D.openWizardSelection = (kind) => {
    D.captureWizard();
    const d = D.wizard.data,
      business = D.wizardBusiness();
    const selectedApps = business ? (d.app ? [d.app] : []) : d.apps;
    D.openPicker({
      kind,
      title:
        kind === "apps"
          ? t("选择应用", "Select applications")
          : t("选择服务", "Select services"),
      multiple: !business,
      required: business,
      appIds: kind === "services" ? selectedApps : undefined,
      selected:
        kind === "apps"
          ? selectedApps
          : business
            ? d.service
              ? [d.service]
              : []
            : d.services,
      apply: (ids) => {
        if (kind === "apps") {
          d.apps = ids;
          d.app = business ? ids[0] || "" : "";
          d.services = (d.services || []).filter((id) =>
            D.state.services.some((s) => s.id === id && ids.includes(s.app)),
          );
          if (
            !D.state.services.some(
              (s) => s.id === d.service && ids.includes(s.app),
            )
          )
            d.service = "";
        } else {
          d.services = ids;
          d.service = business ? ids[0] || "" : "";
        }
        D.syncWizardOwnership();
        D.wizard.tested = false;
        D.renderWizard();
        document
          .querySelector(
            `#monitor-form [name="${kind === "apps" ? "application-picker" : "service-picker"}"]`,
          )
          ?.focus();
      },
    });
  };
  D.openScopeSelection = (kind) => {
    const f = D.ui.filters;
    D.openPicker({
      kind,
      title:
        kind === "apps"
          ? t("筛选应用", "Filter applications")
          : t("筛选服务", "Filter services"),
      multiple: false,
      required: true,
      allowAll: true,
      allLabel:
        kind === "apps"
          ? t("全部已授权应用", "All authorized applications")
          : t("全部匹配服务", "All matching services"),
      appIds: kind === "services" && f.app !== "all" ? [f.app] : undefined,
      env: kind === "services" ? f.env : undefined,
      selected: [kind === "apps" ? f.app : f.service],
      apply: (ids) => {
        f[kind === "apps" ? "app" : "service"] = ids[0];
        D.render();
      },
    });
  };
  D.openExplorerApplication = (page) => {
    const key = page === "logs" ? "logApp" : "traceApp";
    D.openPicker({
      kind: "apps",
      title: t("筛选应用", "Filter applications"),
      multiple: false,
      required: true,
      allowAll: true,
      allLabel: t("全部已授权项", "All authorized items"),
      selected: [D.ui[key]],
      extraRows:
        page === "logs"
          ? [
              {
                id: "infra",
                name: t("基础设施日志", "Infrastructure logs"),
                detail: t("主机 / 集群 / 组件", "Host / cluster / component"),
                env: "—",
                count: "—",
              },
            ]
          : [],
      apply: (ids) => {
        D.ui[key] = ids[0];
        D.render();
      },
    });
  };
  D.explorerApplicationField = (page) => {
    const value = D.ui[page === "logs" ? "logApp" : "traceApp"];
    const label =
      value === "all"
        ? t("全部已授权项", "All authorized items")
        : value === "infra"
          ? t("基础设施日志", "Infrastructure logs")
          : D.appName(value);
    return D.pickerField(
      page + "-application",
      t("应用范围", "Application scope"),
      label,
      "select-explorer-app",
      { id: page },
    );
  };
  document.addEventListener("click", (event) => {
    const el = event.target.closest('[data-action^="picker-"]');
    if (el && !el.disabled) D.pickerAction(el.dataset.action, el.dataset.id);
  });
  document.addEventListener("input", (event) => {
    if (event.target.name === "picker-query" && D.picker) {
      D.picker.query = event.target.value;
      D.picker.page = 0;
      D.renderPicker();
    }
  });
  document.getElementById("picker").addEventListener("cancel", () => {
    D.picker = null;
  });
})();
