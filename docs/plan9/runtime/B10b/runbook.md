# B10b 可执行运行包

运行号 `p9-b10b-20261112-01`。前件 B10a 已通过正常平台退役流程清理，旧历史保留。本包使用独立 Kind 集群，正式 Argus 的六个 PVC 绑定保留。Kubernetes 调度、CSI、HPA、网络策略由真实 Kubernetes 实现，业务对象走 Argus 预览、确认和异步执行。

## 基础设施与资源

- Kind v0.32.0 / Kubernetes v1.36.1，单节点 3 CPU、3584 MiB；默认 CNI 关闭，Pod CIDR `192.168.0.0/16`。
- Calico v3.33.0、metrics-server v0.8.1、Rawfile LocalPV CSI v0.15.1。公开源清单、图表摘要及镜像 digest 在本轮 artifacts 中固定。
- kubelet 使用本集群 CA 签发的 serving certificate；仅审批身份、用途、DNS/IP 与本节点一致的 CSR。metrics-server 和 Collector 均保持 TLS 验证。
- CSI 数据只在本轮节点的 `/var/lib/argus-plan9/p9-b10b-20261112-01/rawfile`；`b10b-expandable` 使用 WaitForFirstConsumer、ext4、Delete 和在线扩容。
- 业务 Pod 非 root、无 ServiceAccount token；HPA 最多 3 副本。两个客户端每 30 秒请求一次，八小时截止；定时任务到同一时刻由精确 UID 的保护计时器暂停，环境和监控保留。
- 平台连接身份只能修改 `b10b-workloads` 的相关对象；PV、StorageClass 只读；不能写 kube-system。

## 本次执行顺序

以下在仓库根运行。记录的是本轮顺序；前件已删除后不要重复执行 transition 或沿用旧成功证据创建下一轮。私有会话正常登录获取，目录限制 ACL，不能提交私有文件。

```powershell
python docs/plan9/runtime/B10b/transition.py
python docs/plan9/runtime/B10b/run.py cluster
python docs/plan9/runtime/B10b/run.py network
python docs/plan9/runtime/B10b/metrics.py
python docs/plan9/runtime/B10b/storage.py
python docs/plan9/runtime/B10b/run.py bootstrap
python docs/plan9/runtime/B10b/build_ca_fix.py
python docs/plan9/runtime/B10b/publish_ca_fix.py
python docs/plan9/runtime/B10b/run.py foundation
python docs/plan9/runtime/B10b/build_workload.py
python docs/plan9/runtime/B10b/run.py objects
python docs/plan9/runtime/B10b/run.py storage-test
python docs/plan9/runtime/B10b/run.py jobs-test
python docs/plan9/runtime/B10b/run.py monitoring
python docs/plan9/runtime/B10b/run.py network-test
python docs/plan9/runtime/B10b/deadline.py
python docs/plan9/runtime/B10b/delivery_watch.py
python docs/plan9/runtime/B10b/build_observation_fix.py
python docs/plan9/runtime/B10b/publish_observation_fix.py
python docs/plan9/runtime/B10b/build_native_ca_fix.py
python docs/plan9/runtime/B10b/publish_native_ca_fix.py
python docs/plan9/runtime/B10b/build_web.py
python docs/plan9/runtime/B10b/publish_web.py
python docs/plan9/runtime/B10b/verify_node_ca.py
python docs/plan9/runtime/B10b/browser.py
python docs/plan9/runtime/B10b/run.py verify
python docs/plan9/runtime/B10b/finalize.py
```

若 jobs-test 已安装 deadline 保护，不重复执行 deadline.py。本轮实际首次在 jobs-test 完成后补装，当前保护以 `cron-deadline.json` 为准。实际修复后的重复运行和原始失败日志均保留，不把失败尝试算通过。

PVC 的创建和消费者部署分开提交，等待实际 Bound 后确认创建生效，避免把 WaitForFirstConsumer 当作失败。异步操作通过正常操作列表及创建时间关联；不能把 pending action 的公开 action_ref 当作内部 UUID。

## 实际断言

- 写入文件 → 滚动重启 → 内容保留；在线扩容 64 MiB → 128 MiB，挂载 statvfs 容量真实增加，内容保留；缩容预览拒绝；临时 PVC/消费者删除后 PV 实际回收。
- Job 暂停后活动 Pod 退出，恢复后真实完成并写入卷和标准输出；CronJob 暂停、恢复、定时执行和从当前版本手动运行均经正常平台 API。
- 实际 metrics.k8s.io CPU 驱动 HPA 从 1 到 3，再回到 1；未手工修改 HPA 结果。
- 两个客户端先均可访问；真实 Calico 策略允许指定客户端并阻断另一个；删除策略恢复；重建策略再次阻断。
- 三项原生采集健康且样本推进；四个绑定仪表盘有真实数据；业务可用副本降至零的告警触发，恢复到一后解除。
- 浏览器覆盖中英明暗、桌面宽度、七类资源、真实手动 Job、实际 Pod 日志与 Shell、仪表盘 canvas 和当前告警过滤。
- 初始长 Job 创建尚在观察时被再次编辑，旧记录经本次共享修复明确结算为 `KUBERNETES_OPERATION_SUPERSEDED`。未知写入仍保留未知，不重放。

Rawfile 在本环境未向 kubelet 提供该 PVC 的 volume stats。PVC 对象和终端展示实际容量；没有补造卷使用率曲线。HPA/Job/CPU/内存曲线使用本轮观测到的原生指标。

## 后续清理

仅在用户要求下一批时执行：

```powershell
python docs/plan9/runtime/B10b/run.py retire
```

先校验 Namespace UID 和运行所有权，删除定时任务、HPA、消费者及任务，再删除业务 PVC，并等待 CSI PV 回收后才调用共享 Collector/集群/凭据退役和 Kind 删除。正式服务与 PVC 必须与清理前一致；保留指标、告警、操作历史及本轮证据。

最终公开 CA 使用受管配置卷的固定文件，节点不支持服务名覆盖。基础业务镜像从仓库 B01/B09a/B10a Dockerfile 复用构建缓存，缺失时只构建镜像，不启动旧场景。每个基础设施清单/图表先校验固定 SHA256。后端和 Web 发布顺序执行，前后均核对精确 digest 及非本次组件保留。
