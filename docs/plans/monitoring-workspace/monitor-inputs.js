(() => {
  const D = window.ArgusDemo,
    t = (a, b) => D.t(a, b),
    e = D.e;
  D.credentialFieldNames = ["authUsername", "authPassword", "authToken"];
  D.credentialModes = (c) => {
    if (
      ["push", "file", "k8slog", "system", "icmp", "dns", "socket"].includes(
        c.mode,
      ) ||
      c.category === "logs"
    )
      return ["none"];
    if (c.mode === "windows") return ["local"];
    if (c.mode === "probe") return ["none", "password", "token"];
    if (["database", "broker", "jmx"].includes(c.mode))
      return ["password", "none"];
    return ["password", "token", "none"];
  };
  D.authLabel = (mode) =>
    ({
      password: t("用户名 / 密码", "Username / password"),
      token: t("访问 Token", "Access token"),
      none: t("无需认证", "No authentication"),
      local: t("使用采集器运行身份", "Collector process identity"),
    })[mode] || mode;
  D.syncWizardOwnership = () => {
    const d = D.wizard.data;
    if (D.wizardBusiness()) {
      d.apps = d.app ? [d.app] : [];
      d.services = d.service ? [d.service] : [];
      d.env =
        D.state.services.find((s) => s.id === d.service && s.app === d.app)
          ?.env || null;
    } else {
      d.app = "";
      d.service = "";
      if (D.catalogById[D.wizard.type].category === "logs") {
        d.apps = [];
        d.services = [];
      }
      const envs = D.environmentIds(d);
      d.env = envs.length === 1 ? envs[0] : null;
    }
  };
  D.ownershipFields = () => {
    const d = D.wizard.data,
      c = D.catalogById[D.wizard.type],
      business = D.wizardBusiness();
    const apps = business ? (d.app ? [d.app] : []) : d.apps;
    let html = `<h3 class="argus-section-label">${business ? t("业务归属 · 必填", "Business ownership · required") : t("关联应用 · 可选", "Applications · optional")}</h3>`;
    if (!business && c.category === "logs")
      return (
        html +
        `<div class="argus-full">${D.note(t("此日志归属主机或集群组件，无需关联业务应用。", "These logs belong to a host or cluster component; no business application is required."))}</div>`
      );
    html += D.pickerField(
      "application-picker",
      business
        ? t("应用 · 必填", "Application · required")
        : t("关联应用", "Applications"),
      D.selectionSummary(apps, "apps"),
      "select-wizard-apps",
    );
    html += D.pickerField(
      "service-picker",
      business
        ? t("服务 · 必填", "Service · required")
        : t("限定服务（可选）", "Services (optional)"),
      D.selectionSummary(
        business ? (d.service ? [d.service] : []) : d.services,
        "services",
      ),
      "select-wizard-services",
      {
        disabled: !apps.length,
        help: business
          ? t(
              "从所选应用的全部服务中选择，环境自动带入。",
              "Choose a service from the application; its environment is inherited.",
            )
          : t(
              "不限定时关联整个应用；可按具体服务缩小范围。",
              "Leave empty for the whole application, or narrow to specific services.",
            ),
      },
    );
    html += `<div class="argus-full argus-derived-environment"><span class="argus-muted">${t("环境归属", "Environment association")}</span><strong>${e(D.environmentLabel(d))}</strong><span class="argus-small argus-muted">${t("自动跟随应用 / 服务；无需重复填写。", "Inherited from applications / services; no duplicate configuration.")}</span></div>`;
    return html;
  };
  D.authenticationFields = (c, d) => {
    const modes = D.credentialModes(c);
    if (modes.length === 1 && modes[0] === "none") return "";
    const hasSaved = Boolean(
      d.managedCredentialId &&
      D.state.monitorCredentials?.some(
        (x) => x.id === d.managedCredentialId && x.mode === d.authMode,
      ),
    );
    let html = `<h3 class="argus-section-label">${t("连接认证", "Connection authentication")}</h3>`;
    html += D.select(
      "authMode",
      t("认证方式", "Authentication"),
      modes.map((id) => [id, D.authLabel(id)]),
      d.authMode,
    );
    html += D.select(
      "tls",
      t("TLS 校验", "TLS validation"),
      [
        ["verify", t("启用并校验证书", "Enable & verify certificate")],
        ["local", t("本机受控连接", "Controlled local connection")],
      ],
      d.tls,
    );
    if (d.authMode === "password")
      html +=
        D.input(
          "authUsername",
          t("用户名", "Username"),
          "",
          'autocomplete="off" placeholder="monitor_user"',
        ) +
        D.input(
          "authPassword",
          t("密码", "Password"),
          "",
          'type="password" autocomplete="new-password" placeholder="' +
            t(
              hasSaved ? "已配置，留空保持不变" : "填写监控账号密码",
              hasSaved
                ? "Configured; leave empty to keep"
                : "Enter monitoring password",
            ) +
            '"',
        );
    if (d.authMode === "token")
      html += `<div class="argus-full">${D.input("authToken", t("访问 Token", "Access token"), "", 'type="password" autocomplete="new-password" placeholder="' + t(hasSaved ? "已配置，留空保持不变" : "填写访问 Token", hasSaved ? "Configured; leave empty to keep" : "Enter access token") + '"')}</div>`;
    if (["password", "token"].includes(d.authMode))
      html += `<div class="argus-full argus-small argus-muted">${hasSaved ? t("已保存监控专用认证信息。留空沿用；填写完整信息可替换。", "Dedicated authentication is configured. Leave blank to retain it, or enter a complete replacement.") : t("在这里填写即可，保存时自动托管为此监控的专用凭证。", "Enter credentials here; saving automatically creates a credential owned by this monitor.")} ${t("原型仅模拟托管状态，不持久化输入的认证信息。", "This prototype stores only simulated credential metadata, not the entered authentication values.")}</div>`;
    return html;
  };
  D.restoreCredentialInputs = () =>
    D.credentialFieldNames.forEach((name) => {
      const el = document.querySelector(`#monitor-form [name="${name}"]`);
      if (el) el.value = D.wizardSecrets?.[name] || "";
    });
  D.credentialError = () => {
    const d = D.wizard.data,
      s = D.wizardSecrets || {},
      existing = D.state.monitorCredentials?.find(
        (x) => x.id === d.managedCredentialId && x.mode === d.authMode,
      );
    if (!["password", "token"].includes(d.authMode)) return "";
    if (existing && !D.credentialFieldNames.some((name) => s[name])) return "";
    if (d.authMode === "token" && !s.authToken)
      return t("请填写访问 Token。", "Enter an access token.");
    if (
      d.authMode === "password" &&
      (!s.authPassword || (!s.authUsername && D.wizard.type !== "redis"))
    )
      return t(
        "请填写完整的监控用户名与密码。",
        "Enter the monitoring username and password.",
      );
    return "";
  };
  D.commitCredential = (monitorId, data) => {
    D.state.monitorCredentials ||= [];
    const previous = D.state.monitorCredentials.find(
      (x) => x.ownerId === monitorId,
    );
    if (!["password", "token"].includes(data.authMode)) {
      D.state.monitorCredentials = D.state.monitorCredentials.filter(
        (x) => x.ownerId !== monitorId,
      );
      return null;
    }
    const changed = D.credentialFieldNames.some(
      (name) => D.wizardSecrets?.[name],
    );
    const credential = {
      id: previous?.id || D.id("monitor-credential"),
      ownerType: "collection",
      ownerId: monitorId,
      purpose: "monitoring",
      mode: data.authMode,
      version: (previous?.version || 0) + (changed || !previous ? 1 : 0),
      configured: true,
      simulated: true,
    };
    if (previous) Object.assign(previous, credential);
    else D.state.monitorCredentials.push(credential);
    return credential.id;
  };
  document.getElementById("modal").addEventListener("close", () => {
    if (document.getElementById("modal").open) return;
    D.wizardSecrets = null;
    document
      .querySelectorAll(
        '#modal input[type="password"], #modal [name="authUsername"]',
      )
      .forEach((el) => (el.value = ""));
  });
})();
