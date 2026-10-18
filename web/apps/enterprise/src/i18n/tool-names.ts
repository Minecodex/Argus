import {
  nativeToolNamesEn,
  nativeToolNamesZh,
} from "@argus/api-client/tool-labels";

export const toolNamesZh = {
  toolNames: {
    identifier: "工具标识",
    labels: {
      ...nativeToolNamesZh,
      "tool/search": "工具搜索",
      "tool/describe": "工具详情",
      "tool/invoke": "工具调用",
      tool: "工具调用",
      connect: "远程命令",
      "workflow/remote_plan/preview": "远程修复计划",
      read: "文件读取",
      bash: "命令执行",
      edit: "文件编辑",
      write: "文件写入",
    },
  },
};

export const toolNamesEn = {
  toolNames: {
    identifier: "Tool identifier",
    labels: {
      ...nativeToolNamesEn,
      "tool/search": "Tool search",
      "tool/describe": "Tool details",
      "tool/invoke": "Tool invocation",
      tool: "Tool invocation",
      connect: "Remote command",
      "workflow/remote_plan/preview": "Remote repair plan",
      read: "Read file",
      bash: "Run command",
      edit: "Edit file",
      write: "Write file",
    },
  },
};
