# B02a Web / 代理真实运行包

本批已由用户确认通过并退役，当前运行 [B02b](../../batches/B02b-review.md)。以下保留可重复执行的历史运行包。

本包部署两台独立身份的 Debian/systemd 测试主机，每台运行 Nginx、Apache、HAProxy 和两个真实 HTTP 后端。A 用于故障与负载，B 为健康对照。不存在向 Argus 数据库写指标或模拟 OTLP 的路径。

## 拓扑与边界

| 进程 | 业务端口 | 原生采集入口 | 后端 |
| --- | --- | --- | --- |
| Nginx | 8081 | `/nginx_status` | 9000 |
| Apache | 8082 | `/server-status?auto` | 9000 |
| HAProxy | 8083 | 8404 `/stats`，Receiver 读取原生 CSV | 独立后端 9001 |

每个中间件分别创建 Monitoring Target 和采集实例，共六个目标。采集由对应主机上的正式 Collector 执行。`web-a`、`web-b` 是本批 Docker 网络内的名称；它们不是浏览器或 Kubernetes Pod 的 localhost。

只有 SSH 发布到本机 22231/22232。两台主机合计上限 3 CPU / 2560 MiB、每台 512 PID；systemd 容器使用 privileged 与私有 cgroup，不挂宿主 Docker Socket、服务目录或可写 cgroup。观测仍属于共享内核与容器可见范围。

## 准备、启动与接入

从仓库根目录执行；先按正常流程清理上一批。`manage.py up` 检查 B01 的清理记录，已注册的容器不会被自动重建。

```powershell
docker build --tag argus-plan9-web:b02a-20261027-v2 --file docs/plan9/runtime/B02a/Dockerfile docs/plan9/runtime/B02a
python docs/plan9/runtime/B02a/manage.py prepare
python docs/plan9/runtime/B02a/manage.py up
```

基础镜像复用 B01 的**未安装身份的干净构建产物**，不依赖 B01 运行环境。构建前校验基础镜像 ID；三种原生服务使用 Debian 官方包，实际版本、镜像摘要和源码摘要记录在 `images.lock.json` 与本次 artifacts。Dockerfile 统一可执行脚本的 LF 换行。

登录会话和 CA 由正常登录/信任流程取得。下面的路径变量只引用受保护材料，不含账号密码。

```powershell
python docs/plan9/runtime/B02a/integration.py onboard --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py collectors --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py builtins --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py targets --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py dashboards --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py alerts --session $sessionPath --ca $caPath
python docs/plan9/runtime/B02a/integration.py review --session $sessionPath --ca $caPath
```

Host/Collector 接入复用 `runtime/common/hosts.py`，正常 API 会话、CSRF、幂等键和预览确认复用 `runtime/common/api.py`。root 凭据随机生成，仅保存在本次受限 `private/` 目录并通过 stdin 交给主机，不写命令行或报告。

## 真实变化与告警

- Nginx：12 个并发的真实慢请求，活动连接阈值 8、恢复阈值 4。
- Apache：12 个真实慢请求，忙碌 Worker 阈值 8、恢复阈值 4；当前 event MPM 中 Worker 为工作线程。
- HAProxy：停止 9001 的独立后端，真实请求产生 HTTP 503；使用前端 5xx 计数的 30 秒速率，阈值 0.1 req/s、恢复为 0。主 Nginx/Apache 后端保持运行。
- 三条策略均使用 10 秒评估周期、20 秒触发/恢复持续窗口，范围仅对应 A 的单个目标；没有外部通知渠道。
- 独立停采 Nginx-A，检查暂停评估、其他五个目标正常，再恢复采集。没有配置或意外断流仍按 NoData/Error 处理。

阈值在实际负载和故障前配置并冻结。压力最长 120 秒，有独立 systemd 时间/内存/CPU 限制；确认触发后可以提前停止并验证恢复。

```powershell
python docs/plan9/runtime/B02a/manage.py load --host web-a --integration nginx
python docs/plan9/runtime/B02a/manage.py load --host web-a --integration apache
python docs/plan9/runtime/B02a/manage.py fault --host web-a
python docs/plan9/runtime/B02a/manage.py recover --host web-a
```

低频真实 HTTP 请求约每个端口两秒一次，有基于本批准备时间的绝对 24 小时截止时间，重启不会无限续期。客户端仅使用 HTTP，逐次关闭连接；后台流量有 64 MiB 限制。结束增长性流量后服务和采集继续存在，历史异常需按交付记录的时间窗口查看。

Nginx stub_status 不提供错误率/时延分位数，本批不补造。HAProxy 后端状态以原生 CSV 和真实 HTTP 响应核对，告警使用实际可采集的 5xx Counter。

## 自检与交付

`verify` 核验六项采集的新鲜度、数据范围隔离、Nginx 原生计数与采样、三类默认仪表盘。真实门户 E2E 位于 `web/apps/enterprise/e2e/plan9-b02a-real.spec.ts`，覆盖中英/明暗、六目标导航、查询与资源范围、告警时间线和 1280/1920 桌面宽度。

当前入口和真实结果见[本批交付记录](../../batches/B02a-review.md)。自动通过不代替人工验收，B02b 不自动启动。

## 清理顺序

下一次收到用户销毁/切换指令后：封存证据，关闭本批策略和流量，恢复后端；正常 API 归档六项业务采集并停止固定采集，撤销本批资源授权；正式卸载两台主机并验证本地清理，再删除主机记录、禁用 SSH 凭据。最后核验 run 标签与容器 ID 后执行本项目 `compose down --volumes`，保留共享平台、默认模板、公共评估身份和历史记录。不能直接先删容器，让软件卸载失去目标。

原生接入依据：[Nginx Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/nginxreceiver)、[Apache Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/apachereceiver)、[HAProxy Receiver](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/v0.159.0/receiver/haproxyreceiver)。
