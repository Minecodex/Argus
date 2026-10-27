export const kubernetesLogSessionZh = {
  kubernetesLogSession: {
    title: "Pod 日志 · {{pod}}",
    action: "查看日志",
    hint: "会话固定当前 Pod、容器和重启代次，有效期 15 分钟。日志按运维权限读取并屏蔽凭据；目标变化或撤权后停止。",
    container: "容器",
    tail: "初始末尾行数",
    window: "初始时间窗（秒）",
    previous: "读取上次终止容器日志",
    open: "打开日志会话",
    close: "关闭日志会话",
    follow: "跟随新增日志",
    refresh: "读取新增",
    output: "日志内容",
    empty: "当前范围尚无日志。",
    complete: "有界读取已完成",
    truncated: "日志超过单次读取上限，已停止跟随。请缩短初始时间窗后重新打开。",
    retention:
      "页面仅保留最近部分日志，关闭后清空。需要长期检索时，请配置日志采集规则。",
    podUid: "Pod UID",
    restarts: "重启代次",
    invalid: "请选择容器；末尾行数为 1～1000，时间窗为 1～86400 秒。",
  },
};
export const kubernetesLogSessionEn = {
  kubernetesLogSession: {
    title: "Pod logs · {{pod}}",
    action: "View logs",
    hint: "The 15-minute session pins the current Pod, container and restart generation. Operational-log access and credential redaction apply. Target changes or revocation stop reading.",
    container: "Container",
    tail: "Initial tail lines",
    window: "Initial window (seconds)",
    previous: "Read the previously terminated container",
    open: "Open log session",
    close: "Close log session",
    follow: "Follow new logs",
    refresh: "Read new entries",
    output: "Log content",
    empty: "No logs in the current scope.",
    complete: "Bounded read complete",
    truncated:
      "Logs exceeded the per-read limit. Following stopped. Shorten the initial window and open a new session.",
    retention:
      "This page retains recent logs and clears them when closed. Configure log collection rules for long-term search.",
    podUid: "Pod UID",
    restarts: "Restart generation",
    invalid:
      "Choose a container; tail lines must be 1–1000 and the window 1–86400 seconds.",
  },
};
