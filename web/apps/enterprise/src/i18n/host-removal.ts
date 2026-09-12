export const hostRemovalZh = {
  hostRemoval: {
    incomplete: {
      runningTitle: "取消安装并删除记录",
      failedTitle: "删除未完成的安装记录",
      dialogDescription: "处理 {{name}} 的未完成接入记录。",
      warning:
        "此操作会终止本次接入并删除 Argus 中的记录，不通过 SSH 清理目标机。目标机可能保留安装残留，也不会卸载属于其他身份的 Connector。",
      cancelAndDelete: "取消安装并删除",
      deleteRecord: "删除记录",
      previewing: "正在准备删除…",
      previewFailed: "无法生成删除预览",
      confirmDelete: "确认删除",
      backToResource: "返回资源",
    },
  },
} as const;

export const hostRemovalEn = {
  hostRemoval: {
    incomplete: {
      runningTitle: "Cancel installation and delete the record",
      failedTitle: "Delete the unfinished installation record",
      dialogDescription:
        "Manage the unfinished onboarding record for {{name}}.",
      warning:
        "This ends the current onboarding attempt and deletes its Argus record without using SSH to clean the target. Installation files may remain, and a Connector belonging to another identity will not be uninstalled.",
      cancelAndDelete: "Cancel installation and delete",
      deleteRecord: "Delete record",
      previewing: "Preparing deletion…",
      previewFailed: "Could not create the deletion preview",
      confirmDelete: "Confirm deletion",
      backToResource: "Back to resource",
    },
  },
} as const;
