# 全门户重设计验收记录

日期：2026-10-07。源码、契约、真实 React 页面与 Docker Desktop Kubernetes 验收分别记录，不能用截图代替真实接口。

## 最新完成度复核

当前资源入口交付见[正式记录](./resource-links-20261007.md)：r7，相关桌面 43 项跨轮次通过、另 12 个公共控件组合；所选真实浏览器 5/5，文件/协议/6 类故障、临时及正式安装各 20/20 通过，Namespace 0、原 19 项恢复。正式两门户 8 组 HTTPS/主题/语言/初始化检查通过；并非完整 PlanV2 或实际模型评测重跑。以下为此前轮次。

当前查看条件、资源选择和过滤项编辑的实现与验收见[最新记录](./dashboard-controls-20261007.md)：新布局 12/12，所选 31 项跨轮次通过，公共控件/初始化 24/24，真实 M2 7/7、临时安装 20/20，测试 Namespace 0、原 19 项工作负载恢复。正式 r6 已交付，健康 20/20、8 组 HTTPS 与初始化复核通过，构建和包体积通过。此次空仪表盘配置/参数闭环不替代真实遥测或完整 PlanV2 验收。以下为此前轮次。

最新弹窗/选值/场景库修正见[本轮记录](./dialogs-presets-20261007.md)。正式版本 `portal-20261007-r5` 已交付；12/12 新布局、所选旧回归 115 个桌面场景跨轮次通过、M2 真实 7/7、安装与正式复核各 20/20，测试清理和 19 项工作负载就绪已核验。8 组正式 HTTPS 双语明暗检查通过。该轮未重跑完整三信号/模型套件，不能扩大成全部 PlanV2 验收。此前轮次结果保留如下。

用户截图新增的样式和 Demo 差异以[用户界面复核](./user-ui-review-20261007.md)为最新记录。152 个桌面场景均有分轮通过覆盖，查询筛选闭环另有 1 项通过；最新所选真实场景共 9 项跨轮次通过，s 轮次运行时、故障及安装成功、退出码 0。测试 Namespace 0、19 项工作负载恢复，正式前后端已升级 `portal-20261007-r4`；正式安装 20/20、两门户 HTTPS/初始化/双语明暗复核通过。下方 r2 等是此前轮次，保留原有范围。

此前发现的真实概览、业务 Tabs、编辑初始反馈、APM 重复扫描及本地部署缺口现已补齐并验证。最新实现、结果及可定位证据统一见[补齐记录](./implementation-20261007.md)。问题发现时的源码事实仍保留在[再次复核](./completion-review-20261007.md)。

最新桌面回归 140 通过/1 范围外跳过。完整真实轮次 `ui-20261007n` 为 44 通过、12 失败、4 未运行；`ui-20261007o` 对对应 16 项复验 16/16 通过，并完成运行时、6 类在途故障、安装 20/20 和退出码 0，构成全部 60 个场景的通过覆盖，不能称为单轮无失败。临时 Namespace 剩余 0、19 项正式工作负载恢复就绪。正式前后端已部署 `portal-20261007-r2`，正式安装 20/20、两门户 Chromium HTTPS 200、控件 32px、已初始化状态均核验。

## 首次复核与后续收尾记录

2026-10-07 对照源码与测试范围重新核查，完整计划尚未关闭。下表记录的是已经执行的检查结果；成功次数不能替代未实现的交互要求或未覆盖的状态。局部筛选候选、指标元数据联动、图型推荐和完整验收矩阵的具体证据与关闭条件见[复核清单](./review-20261007.md)。

后续收尾完成了当时识别的候选、元数据、图型推荐、草稿竞态和部分状态覆盖修正，结果如下。历史结果保留原轮次事实；本计划整体状态以上方最新复核为准。

## 收尾结果

| 检查 | 最新结果 | 证据（`artifacts/frontend-redesign/closure`） |
| --- | --- | --- |
| 类型、样式、表单和国际化 | 通过 | `types-final.log`、`lint-final.log`、`tests-final.log` |
| 全工作区单元 | 通过；UI 72、Enterprise 159、Platform 17、Template 2 | `tests-final.log` |
| 相关后端与契约 | 通过 | `backend-final.log`、`e2e-helper.log` |
| 正式构建与包体积 | 通过；本轮 Docker 镜像包含最终代码 | `build-final.log`、`bundle-final.log`、`k8s-retest.log` |
| 桌面组合与业务回归 | 140 通过，1 移动端范围外显式跳过 | `mock-final.log`、`mock-final/`；三种尺寸×中英×明暗 |
| 完整真实浏览器轮次 | 35/44 通过，9 失败；失败不能计为成功 | `k8s-accepted.log`（`ui-20261007l`） |
| 集中真实复验 | 11/11 通过，含全部 9 项失败及另外 2 个中文状态组合；全部 44 场景有通过证据 | `k8s-retest.log`、`k8s-retest/playwright-planv2/`（`ui-20261007m`） |
| 查询、文件与故障 | 实际后端通过；崩溃、取消、损坏、配额、归档均通过 | `k8s-retest/planv2-files.json`、`planv2-inflight-faults.json`、`planv2-tool-protocol.json` |
| 安装与最终退出 | 20/20；退出码 0；报告准确标为 selected_browsers_and_runtime | `k8s-retest/verify/verify.json`、`k8s-retest/result.json` |
| 清理与恢复 | 本轮 Namespace 剩余 0；19 项正式工作负载恢复到原副本并就绪 | `cleanup-proof.json` |

全部场景的覆盖由完整轮次和失败项复验共同构成，不能描述为单轮 44/44。状态检查在真实登录和初始数据之后，由浏览器注入延迟、空集合、503、403；实际授权与运行时故障另有领域/工具回归。实际 AI 模型未配置，未执行模型质量评测；本地转发、NetworkPolicy、共享 Sandbox 的边界保持原声明。当前正式安装未做镜像升级，本次验收结束后恢复此前部署。

## 本次修复与失败记录

- 候选查询仅传 field 的缺口已消除；候选使用完整定义、映射和实际依赖，分页/失败不回退。
- 指标类型不再从前一个指标沿用，计算选项不再强制改写真实类型；十一类图型按样本形状及全部查询判定并给出原因。
- 列表加载错误不再显示为空集合，平台审计包含在共享反馈修复中。PKI 的真实长记录暴露滚动区域缺少键盘焦点，已在共享 DataTable 修复，四个平台主题/语言复验通过 Axe。
- 下钻生成返回旧草稿会覆盖新标题，改为同一保存队列和三方合并，并以延迟操作测试及中英真实工作台验证。
- 旧 Select 名称匹配、隐藏 checkbox/radio 的直接鼠标检查、旧日期 textbox 定位、资源弹层退出时序及拦截 HTML 的脚本问题均已修正；结果断言和真实数据保留。未降低 Axe 或授权断言。
- 长浏览器测试后验收会话过期导致文件配置 401，继续取数前重新完成平台与企业 MFA；没有扩大有效期或绕过认证。
- `closure/k8s.log` 保留缺少默认 nginx IngressClass 的前置失败；`k8s-full.log` 保留第一次完整范围失败；`k8s-accepted.log` 保留第二次 9 项失败。旧失败未覆盖为成功。

## 历史首轮检查结果

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 全工作区 TypeScript 类型检查 | 通过 | `artifacts/frontend-redesign/types-final.log` |
| ESLint、表单语义、样式/导入防漂移 | 通过 | `lint-final.log`、`style-guard.log` |
| pnpm 全工作区单元及 i18n | 通过 | `tests-final.log`；UI 67、Enterprise 153、Platform 17、Template 2 |
| 相关后端与契约 | 通过 | `backend-final.log`；dashboard/httpapi/toolgateway/argusctl/argusdev/contract |
| 正式前端构建与包体积 | 通过 | `build-final.log`、`bundle-check.log`；无 mock 种子标记 |
| 新增/修改代码文件行数 | 244 个检查，超限 0 | `file-size-check.json`；生成约束保持生成器的紧凑格式 |
| 相关浏览器回归 | 95 通过，1 显式跳过 | `accepted-ui.log`；跳过的是不在桌面范围的 mobile navigation |
| 索引子路由后的专项 | 32/32 | `route-final.log`；中英×明暗、三种尺寸、200% 放大、长标题、初始化/平台 |
| 焦点边框重复检查 | 6/6 | `focus-final.log`；修正了情景卡片单行继承及弹层布局 |
| 真实浏览器 | 7/7 | `k8s-verified.log` / `k8s-verified/playwright-planv2` |
| 安装检查 | 20/20 | `k8s-verified/verify/verify.json` |
| 整轮退出与清理恢复 | exit 0；本轮 Namespace 剩余 0；正式副本全部恢复就绪 | `k8s-verified/result.json`、`cleanup-proof.json` |

实际文件均在 `artifacts/frontend-redesign`。界面查看见[截图](./screenshots.md)，覆盖路由见[清单](./routes.md)。

专项已覆盖主要路由的中英×明暗正常状态；三种桌面尺寸的专门检查集中在 K8s 卡片页，长标题与放大检查集中在统计图编辑页。尚不能据此认定全部路由在全部尺寸、语言、主题及加载/失败/无权限状态均已验收。真实浏览器采用下述选择器，不能计入没有被选中的历史用例。

## 真实范围

运行号 `ui-20261007i`，上下文 `docker-desktop`。构建真实后端/门户/Collector 镜像，在独立临时 Namespace 完成初始化、资源接入、Collector、三信号、查询、工具协议、分片文件、取消/撤权、故障恢复、审计与安装检查。

浏览器选择为 `PlanV2 redesign real|real workbench creates`，覆盖中英图型值/单位/桶、个人草稿、单图执行、样式不查询、冻结下钻、查询变更失效、迟到响应、三信号配置、冲突整理和归档恢复。其它历史 PlanV2 浏览器用例没有被自动计入这一轮。工作区的普通浏览器回归和专项矩阵单独列出。

本机采用实际的 HTTPS/Connector 本地转发通道；安装检查验证 Connector 主机名、安装 CA、TLS 1.3 客户端证书及 HTTP/2 settings 通信，报告为 `connector-forward-tls`。外部 LoadBalancer、NetworkPolicy 强隔离和独立 Sandbox Runtime 不是本次本地验收的结论。未调用实际 AI 模型；Chat/工具协议采用确定性测试服务，真实取数/文件与权限是实际后端。

验证助手在执行安装完成后的空闲阶段更新为当前 mTLS 检查器；二进制校验值保存于 `verification-helper.json`。直接对本轮 Gateway 的只读检查也通过，见 `forward-live-test.log`。这一调整仅影响本地验证入口，正常安装默认仍检查外部 LoadBalancer，Production 不能以本地转发替代。

## 修复与失败记录

- 旧测试使用原生标签/旧 Drawer 定位，现采用实际共享组件与独立编辑路由；草稿下钻测试补齐契约必填布尔值。
- Node 请求不使用 Chromium 的 TLS 主机映射，延迟交付测试改为复用浏览器获取的真实响应；没有替换成虚构遥测数据。
- 工作区根据 pathname 条件渲染造成偶发空白，改为父工作区＋明确的画布索引和统计图子路由。
- 多行情景卡片继承普通按钮的单行/固定高度造成溢出；共享组合组件允许自然增高。弹层重设供应商默认负边距/容器布局，焦点检查考虑实际缩放。
- 同集群安装修复 Strimzi 全局定义复用、CRD 服务端默认名字、PKI trust-source RBAC 名称和暂停恢复顺序。所有复用均检查兼容性并保留外部所有权。
- `k8s-closed` 轮功能与故障场景通过，但最后安装器因无外部 LB 地址失败；该轮没有标记整轮成功。最终 `k8s-verified` 采用明确的本地认证连接检查后整轮通过。

旧失败日志保留在 artifacts，未删除、覆盖为成功或将失败/未跑项目划成通过。临时测试资源按所有权清理，未改动 Judex、正式数据或此前不属于此次任务的工作区变更。
