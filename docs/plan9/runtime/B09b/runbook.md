# B09b Kubernetes 组件指标运行包

本轮按用户要求完成 API Server、CoreDNS、Scheduler、Controller Manager 和 etcd 的实际采集与默认仪表盘。沿用 B09a 已登记的独立 Kind 集群、Collector 和资源身份，避免重新安装导致当前页面入口失效；B09a 工作负载和历史数据保留。B09b 的新增 Namespace、授权、组件端点准备和回滚基线单独记录，默认单批运行规则在本轮作为同一集群的连续验收执行。

所有平台配置走正式 API 的预览、确认和运行收敛。数据来自 Kubernetes 组件原生 `/metrics`，经过已安装的 Collector、mTLS Ingest、Kafka、Writer、ClickHouse 和正式仪表盘查询；不写入伪造遥测样本。

## 执行边界

- 控制面监听及服务端证书准备只允许修改 B09a 所有权记录中 Docker ID 和 Kind 标签均匹配的独立测试节点。保存原始清单及 SHA256，拒绝覆盖外部变化；不修改 Docker Desktop 的正式控制面。
- Scheduler/Controller Manager 保持 ServiceAccount 认证授权，使用受集群 CA 签发的服务端证书直接 HTTPS 抓取。etcd 使用专用 HTTP metrics 端口，经受权 API Pod Proxy 读取，不使用 etcd 客户端或成员私钥。
- 固定组件默认不开启匿名权限、不跳过 TLS 验证。CoreDNS/etcd 的 Pod Proxy 许可限定在 kube-system，Collector 不获取 Secret、exec 或 RBAC 写权限。
- 指标开关、数据新鲜度和默认仪表盘继续作为日常界面；组件维护细节保留在次级入口。
- 本轮只验收组件指标。当前无已登记的节点主机，Host/Node Claim 去重不作为组件成功取数的前置条件，也不宣称已实测。

## 初始核对

```powershell
python docs/plan9/runtime/B09b/run.py login
python docs/plan9/runtime/B09b/run.py inspect
python docs/plan9/runtime/B09b/run.py prepare
python docs/plan9/runtime/B09b/run.py permissions
python docs/plan9/runtime/B09b/run.py activity
python docs/plan9/runtime/B09b/run.py enable
python docs/plan9/runtime/B09b/run.py verify
python docs/plan9/runtime/B09b/run.py exercise
python docs/plan9/runtime/B09b/run.py availability
python docs/plan9/runtime/B09b/browser.py
```

登录密码通过隐藏输入读取，仅保存正常登录会话。运行记录位于 `artifacts/plan9/p9-b09b-20261109-01/`，私有材料只保存在其中的 `private/`。凭据不进入文档、普通日志或源码。

验收要求：五项均有持续更新的真实目标样本，五套默认仪表盘逐面板取数；覆盖开关独立性、权限/TLS 失败与恢复、真实告警触发和恢复、桌面中英明暗页面。未观察到的数据保持空或不可用，不能由抓取诊断冒充目标指标。

`activity` 在独立 `b09b-components` 中每十秒执行少量真实 DNS/API 请求，每三轮改变一个测试 Deployment 的 1/2 副本，产生 Scheduler/Controller/etcd 的实际活动。绝对截止时间记录在 `activity-deadline.json`，八小时后恢复一副本并保持空闲；容器重启不会重置该预算。

`exercise` 验证错误证书名称、停止与重新开启。配置变化按既有来源围栏关闭旧告警，记录为 `source_changed`，不把它称为条件自然恢复。`availability` 在所有权确认后的独立节点临时拒绝**当前网关 Pod 到 Scheduler 10259** 的流量，Collector 配置版本保持不变，要求同一条件告警从实际故障恢复；120 秒自动回滚和 finally 清理均使用精确匹配的本轮规则。该故障不修改正式 Argus 或客户控制面的网络策略。

端点恢复使用 `run.py restore`，只在原始/本轮应用清单 Hash 匹配时恢复。完整退役前先通过正常平台操作停用本轮告警、五项采集和新增仪表盘绑定，再按记录的 UID 删除 B09b Namespace、Role/RoleBinding；B09a 基础设施与平台共享资源按各自清单处理。人工查看前保留实际组件及低负载来源。
