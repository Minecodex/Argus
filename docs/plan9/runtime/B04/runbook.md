# B04 Redis / Memcached 真实场景运行包

运行 `p9-b04-20261027-01`，前置批次 B03 已经正常停采、卸载和回收。本批一个正式接入的 Collector 主机、一个 Redis 和一个 Memcached；不声明 Cluster/Sentinel 拓扑支持。

## 环境与预算

- 实际版本 Redis 8.10.0、Memcached 1.6.45；本次验证镜像 ID/摘要在 `images.lock.json`。主机使用无已注册身份的 B01 基础镜像，禁用 B01 业务单元，通过 SSH 正常安装 Connector/Collector。
- 3 个容器、2 个命名卷、1 个专用网络；CPU 上限合计 2.5，内存上限合计 1568 MiB。只有 SSH 发布到 `127.0.0.1:22261`，缓存端口仅在本批网络访问。systemd 主机使用 privileged/private cgroup，无 Docker socket 或无关宿主挂载。
- Redis `maxmemory 8mb`、`allkeys-lru`、禁用持久化；Memcached 内存 8 MiB、64 连接、单工作线程、禁用 UDP。Memcached 服务最大单条目为默认兼容的 1 MiB，实际负载只写 32 KiB。
- Redis default 用户禁用，独立 monitor/app 随机凭据；配置挂载使用 ACL 密码摘要。monitor 仅 INFO、CONFIG GET、PING，实测不能读业务键；app 仅有 `p9:*` 键的所需操作权限。Memcached 当前原生接入不支持 SASL，本批使用专网明文协议，不声称缓存 TLS/SASL 验收。
- 缓存与正式 Argus Redis 完全独立；所有数据由实际 RESP/Memcached 协议产生，没有向 Argus 数据库或遥测入口写测试曲线。

## 可重复操作

需要本机已验证的基础与缓存镜像，`up` 锁定当前 image ID，并要求 B03 的 `cleanup.json` 为 `destroyed`。已有容器按身份核验，重跑不重置注册身份。

```powershell
python docs/plan9/runtime/B04/manage.py prepare
docker build --tag argus-plan9-cache:b04-20261027 --file docs/plan9/runtime/B04/Dockerfile docs/plan9/runtime/B04
python docs/plan9/runtime/B04/manage.py up
python docs/plan9/runtime/B04/manage.py verify

$sessionPath = 'artifacts/plan9/reset-20261026/private/enterprise-session.json'
$caPath = 'artifacts/plan9/reset-20261026/trust-bundle.pem'
python docs/plan9/runtime/B04/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py idle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/manage.py workload
python docs/plan9/runtime/B04/integration.py alerts --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py exercise --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py lifecycle --session $sessionPath --ca $caPath
python docs/plan9/runtime/B04/integration.py verify --session $sessionPath --ca $caPath
```

`idle` 需要 Collector 已有至少两个真实采样点、最近五分钟没有 GET。首次部署先执行它再启动持续负载；已启动负载时应先停止对应 systemd 单元并等待五分钟窗口自然移出，不能清累计计数模拟空闲。平台会话过期通过正常企业登录刷新，不在脚本中保存平台密码。

## 真实业务和告警

`verify` 用真实键验证写入、成功读取、读取不存在的键和 2 秒 TTL 到期。持续负载每 2 秒写入一个小值，再做一次命中和一次真实未命中；键名循环使用 32 个，TTL 12 秒，运行上限 8 小时。到期只停客户端，保留服务和采集。

告警在压力开始前冻结：内存 `> 5 MiB` 持续 20 秒触发，`<= 2 MiB` 持续 20 秒恢复，评估间隔 10 秒。每次只压一种缓存，对方作为健康对照。压力一次最多 1024 个 32 KiB 键，总发送 32 MiB，所有压力键 TTL 120 秒；内存由服务 8 MiB 容量约束，实际触发淘汰。取得告警及原生计数后显式删除本批压力键，等待同一事件恢复；即使驱动中断，TTL 仍会到期。不用 FLUSHALL/flush_all 清其他键。

Redis 独立停采验证配置生效、后台评估暂停、Memcached 继续取得新鲜样本，再恢复。实例停止采集与服务宕机分开表达，不制造假的零值或正常状态。

当前原生缓存模板升级到 v2：Redis 11 面板、Memcached 9 面板，增加命中/未命中速率和五分钟窗口命中率，Redis 另增加淘汰/到期速率。按资源独立计算比率；分母为零时明确没有有效观测，不补成 0%/100%。未新增平行渲染组件，沿用共享图表、单位、主题和标题信息弹层。模板、生成契约与中英文标题同步。

真实端到端测试为 `web/apps/enterprise/e2e/plan9-b04-real.spec.ts`，覆盖两目标默认图跳转、资源隔离、全部面板、历史告警及时间线、中英/明暗、1280/1920 桌面宽度。结果见 [B04 查看记录](../../batches/B04-review.md)。

## 清理

用户明确要求下一批/清理后执行：

```powershell
python docs/plan9/runtime/B04/retire.py --session $sessionPath --ca $caPath
```

先停两个有界客户端、删除本批压力键，随后经正常 API 删除本批策略、归档采集、撤销本批资源授权、卸载并退役主机，最后按精确 run 标签和容器 ID 回收本批容器/卷/网络。企业账号、历史、共享默认模板和共享评估身份保留。

B03-D01 的原生认证失败诊断缺口仍保留在 B03 记录中；本批切换不构成对该问题的修复或人工验收。
