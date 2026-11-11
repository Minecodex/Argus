export const kubernetesStreamZh = {
  kubernetesStream: {
    exec: "打开终端",
    forward: "端口转发",
    execTitle: "Pod 终端 · {{name}}",
    forwardTitle: "端口转发 · {{name}}",
    execHint:
      "固定当前 Pod、容器代次和所选 Shell；可能访问容器内业务数据。撤权、对象变化或会话过期后终止，关闭窗口即结束。",
    forwardHint:
      "仅连接选定 Pod 或 Service 的 TCP 端口。Service 固定一个实际 Pod，不会自动换端点。",
    container: "容器",
    shell: "Shell",
    requirementsTitle: "普通终端支持条件",
    requirements:
      "所选容器必须安装对应 Shell，集群连接账号必须拥有该命名空间的 pods/exec 权限。无 Shell 的精简镜像（如 Collector）不支持普通终端；请选用包含 Shell 的业务容器。",
    shellHint:
      "目标容器需要包含所选 Shell；不含 Shell 的精简镜像无法打开普通终端。",
    reconnect: "重新连接",
    errors: {
      forbidden:
        "集群连接账号没有此命名空间的终端权限（pods/exec），请授权后重试。",
      forwardForbidden:
        "集群连接账号没有此命名空间的端口转发权限（pods/portforward），请授权后重试。",
      backend:
        "无法连接容器，请检查运行状态、所选 Shell 是否存在，以及集群连接。",
      changed: "登录权限或目标身份已变化，请重新连接。",
      expired: "会话已到期，请重新连接。",
      idle: "长时间没有输入，会话已结束；可以重新连接。",
    },
    port: "目标 TCP 端口",
    reason: "操作原因",
    invalid: "请填写操作原因，端口必须为 1～65535。",
    open: "创建会话",
    close: "关闭会话",
    expires: "最晚结束时间",
    bytes: "传入 / 传出字节",
    cliHint:
      "在本机终端运行下面的命令，再从标准输入粘贴一次性票据。凭据不放入命令参数；只监听回环地址，最多同时 8 个 TCP 连接。",
    ticket: "一次性票据（剩余 {{seconds}} 秒）",
    ticketValue: "一次性连接票据",
    ticketExpired: "票据已过期；尚未连接时可以重新签发。",
    renew: "重新签发票据",
    stopped: "会话已停止。请检查权限、目标状态或重新创建。",
    status: {
      pending: "等待连接",
      connecting: "连接中",
      active: "运行中",
      closing: "正在关闭",
      closed: "已关闭",
      invalidated: "已失效",
      connection_lost: "连接已中断",
    },
  },
};
export const kubernetesStreamEn = {
  kubernetesStream: {
    exec: "Open terminal",
    forward: "Port forward",
    execTitle: "Pod terminal · {{name}}",
    forwardTitle: "Port forward · {{name}}",
    execHint:
      "Pins the Pod, container generation and shell. The terminal can access business data inside the container. Revocation, object changes, expiry or closing this window ends the session.",
    forwardHint:
      "Connects only to the selected TCP port. A Service pins one actual Pod and never switches endpoints automatically.",
    container: "Container",
    shell: "Shell",
    requirementsTitle: "Terminal requirements",
    requirements:
      "The container must include the selected shell and the cluster connection account needs pods/exec access in its namespace. Shell-free images, such as Collector, cannot provide an ordinary terminal; select a business container that includes a shell.",
    shellHint:
      "The container must include the selected shell. Images without a shell cannot open an ordinary terminal.",
    reconnect: "Reconnect",
    errors: {
      forbidden:
        "The cluster connection account lacks terminal access (pods/exec) in this namespace. Grant access and try again.",
      forwardForbidden:
        "The cluster connection account lacks port-forward access (pods/portforward) in this namespace. Grant access and try again.",
      backend:
        "Cannot connect to the container. Check its running state, the selected shell, and the cluster connection.",
      changed: "Access or target identity changed. Connect again.",
      expired: "This session expired. Connect again.",
      idle: "The session ended after inactivity. You can reconnect.",
    },
    port: "Target TCP port",
    reason: "Reason",
    invalid: "Enter a reason; the port must be 1–65535.",
    open: "Create session",
    close: "Close session",
    expires: "Session deadline",
    bytes: "Bytes in / out",
    cliHint:
      "Run this command locally, then paste the one-use ticket through standard input. Credentials stay out of arguments. The listener uses loopback only, with at most 8 concurrent TCP connections.",
    ticket: "One-use ticket ({{seconds}} seconds left)",
    ticketValue: "One-use connection ticket",
    ticketExpired: "Ticket expired. Replace it before connecting.",
    renew: "Replace ticket",
    stopped:
      "Session stopped. Check permissions and target state, or create a new session.",
    status: {
      pending: "Pending",
      connecting: "Connecting",
      active: "Active",
      closing: "Closing",
      closed: "Closed",
      invalidated: "Invalidated",
      connection_lost: "Connection lost",
    },
  },
};
