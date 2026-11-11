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
    options: "日志选项",
    apply: "应用并重新读取",
    retry: "重新读取",
    loading: "正在读取日志",
    noContainers: "当前 Pod 没有可读取的容器。",
    scrollHint:
      "打开即显示日志。向上滚动查看历史，回到底部后继续跟随最新内容。",
    close: "关闭日志会话",
    follow: "跟随新增日志",
    refresh: "读取新增",
    output: "日志内容",
    empty: "该容器当前没有可读取的标准输出日志。",
    windowHint: "0 表示不限制时间，按最近行数读取。",
    windowEmpty:
      "最近 {{seconds}} 秒内没有日志，可在日志选项中将时间窗改为 0。",
    complete: "有界读取已完成",
    truncated: "日志超过单次读取上限，已停止跟随。请缩短初始时间窗后重新打开。",
    retention:
      "页面仅保留最近部分日志，关闭后清空。需要长期检索时，请配置日志采集规则。",
    podUid: "Pod UID",
    restarts: "重启代次",
    invalid: "请选择容器；末尾行数为 1～1000，时间窗为 0～86400 秒。",
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
    options: "Log options",
    apply: "Apply and reload",
    retry: "Retry",
    loading: "Reading logs",
    noContainers: "This Pod has no readable containers.",
    scrollHint:
      "Logs open automatically. Scroll up to read history; return to the bottom to follow new entries.",
    close: "Close log session",
    follow: "Follow new logs",
    refresh: "Read new entries",
    output: "Log content",
    empty: "This container has no retained standard-output logs.",
    windowHint:
      "Use 0 to read the latest retained lines without a time filter.",
    windowEmpty:
      "No logs in the last {{seconds}} seconds. Set the window to 0 in Log options to read retained history.",
    complete: "Bounded read complete",
    truncated:
      "Logs exceeded the per-read limit. Following stopped. Shorten the initial window and open a new session.",
    retention:
      "This page retains recent logs and clears them when closed. Configure log collection rules for long-term search.",
    podUid: "Pod UID",
    restarts: "Restart generation",
    invalid:
      "Choose a container; tail lines must be 1–1000 and the window 0–86400 seconds.",
  },
};
