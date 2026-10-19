# 旧项目能力观察与迁移矩阵

**观察日期**：2026-10-17；**新项目状态**：均未迁移，proposed 目标 owner 不代表已有实现。
只读参考 [旧仓入口](<E:/Workspace/aboard-nexus/README.md>)；先读其根规则与文档入口，再有界核对架构、Contracts、目录、Agent、插件和相关测试。
未读取真实数据库/密钥，未运行测试、业务服务、Provider 或插件，未写入旧仓，也未整体复制代码。

## 基线固定与局限

旧 HEAD：`7b01954035d458711498117aa3b08026aede2633`。
观察时有 **117 条 tracked dirty**，另有 untracked；涉及 Host、Contracts、文档、前端和测试。
HEAD 不能独自固定当前行为。本记录是有界、非原子的可见工作树观察，不是完整可重放快照。
特别是 Workflow template 有 dirty/untracked 实现，不能从设计文字推断其最终能力。
将来正式迁移前需另获授权固定一致源码快照及行为夹具；本轮不代用户提交旧仓。

证据级别：**源码**=读过必要片段；**文档**=仅设计/功能说明；**测试源码**=读过测试定义，不代表本轮运行通过。
本表的当前状态不外推为完整产品已验收。

## 能力 → owner → 语义 → 兼容 → 证据

| 能力 | 当前观察状态 | 原 owner → 目标 owner（提议） | 必保权限/数据/运行语义 | 兼容策略与验收证据 |
| --- | --- | --- | --- | --- |
| Contracts、身份、消息 | 源码已有；未迁移 | packages/contracts → 单一跨语言契约来源及生成绑定 | workspaceId 与 public stableWorkspaceKey 分开；tenant/principal/installation 不由路径推导；DTO 不是授权 | 保留版本和严格验证，固定字节/行为夹具；[身份类型:486–567](<E:/Workspace/aboard-nexus/packages/contracts/src/index.ts#L486-L567>)；[预算 ABI 非 JSON grant:48–55](<E:/Workspace/aboard-nexus/packages/contracts/src/native-run-budget.ts#L48-L55>) |
| 目录身份 v1 / C0 | Windows-only Host-private 身份/SQLite lease 基础已有源码；未迁移 | WorkspaceDirectoryStore/identity/paths/lease → 目录身份/生命周期 owner | SQL 单行身份权威，manifest 是 seal；trusted root 不等于权限或沙箱 | 不自动导入/修复/补种；不得加业务表仍称 v1 兼容；[identity:5–95](<E:/Workspace/aboard-nexus/apps/host/src/workspace-directory-identity.ts#L5-L95>)、[store:11–50](<E:/Workspace/aboard-nexus/apps/host/src/workspace-directory-store.ts#L11-L50>)、[lease:9–55](<E:/Workspace/aboard-nexus/apps/host/src/workspace-directory-lease.ts#L9-L55>) |
| 目录登记、业务分库、关闭排空、Apply | 目录文档 1b-a 为实施中；未证实产品接线，未迁移 | 原 Host 各 mutation/persistence → 安装目录登记 + 对应工作区领域 repository | create 不等于 register/open/授权；close lease 不等于 Run 已停稳；文件候选不自动生效 | [目录文档:17–25、74–85](<E:/Workspace/aboard-nexus/docs/aegis/specs/2026-10-17-workspace-directory-format.md#L17-L85>)；当次对 apps/host/src/*.ts 检索目录登记/开关 API 未命中，不能据此宣称已实现 |
| Native controller、预算、Provider/工具许可 | 受限运行路径已有源码；整体 partial，未迁移 | HostRunService/NativeRunController/permit/Broker → 安装级 RunCoordinator + 当前授权 Broker | 累计截止/预算，save-before-dispatch，无失败退款/自动重发；物理持有未收敛不还槽 | 精确保留安装总额；仅受信 DeepseekCompatibleRuntime 适用，External/Stub 不升级保证；[controller:10–39、275–276](<E:/Workspace/aboard-nexus/apps/host/src/native-run-controller.ts#L10-L39>)、[Provider:43–75](<E:/Workspace/aboard-nexus/apps/host/src/native-provider-attempt.ts#L43-L75>)、[tool:64–123](<E:/Workspace/aboard-nexus/apps/host/src/native-tool-attempt.ts#L64-L123>) |
| Session、历史、恢复、审批 | 有受限恢复源码；审批本轮主要文档，未迁移 | HostRunService/NativeRecovery/StateStore → Session/Run 领域 owner | workspace/agent/principal/tenant/session 匹配；新 linked Run，不改原 interrupted；旧审批不是活授权；有工具证据拒绝 text-only replay | 不迁移活许可或未知副作用；旧无预算历史不伪造回填；[recovery:20–101](<E:/Workspace/aboard-nexus/apps/host/src/native-agent-recovery.ts#L20-L101>)；[审批说明:71–76](<E:/Workspace/aboard-nexus/docs/features/agent-runtime.md#L71-L76>)，审批细节待专项源码核对 |
| 插件准入/激活/声明式 UI | 受信生命周期源码已有，整体 partial；未迁移 | PluginRuntime + plugin-api + ui-contributions → 插件内核/SDK/声明式 UI 各单 owner | 安装≠激活≠授权；generation/package/workspace 绑定；staging/disposer；合作取消超时不强杀 JS | 精确 opaque 版本不隐式 failover；不宣称第三方 OS 沙箱；[runtime:352–388](<E:/Workspace/aboard-nexus/apps/host/src/plugin-runtime.ts#L352-L388>)、[scope:16–54、123–155](<E:/Workspace/aboard-nexus/packages/plugin-api/src/scope.ts#L16-L155>)；任意 React 禁入可信图 |
| SQLite/安装存储 | 默认生产聚合 SQLite 源码；不是逐目录业务分库；未迁移 | SqliteStateStore/StateStore + 安装侧载体 → InstallationStore / WorkspaceRepository 分责 | schema1 snapshot 与 native_run_events；revision CAS、digest；生产不回退旧 JSON；secret/session/config/journal 不混入目录授权 | 不双写，不自动导入；[production:5–15](<E:/Workspace/aboard-nexus/apps/host/src/production-state-store.ts#L5-L15>)、[sqlite:20–124、172–199](<E:/Workspace/aboard-nexus/apps/host/src/sqlite-state-store.ts#L20-L199>)、[runtime paths:115–159](<E:/Workspace/aboard-nexus/apps/host/src/runtime-paths.ts#L115-L159>) |
| Build/Studio/页面 | Build 保存/版本/审阅源码已核对；Studio 更大范围仅文档，未迁移 | workspace-build + 页面/Studio owner → Build/资源 owner；Studio 为普通客户端 | 全安装聚合容量，workspace 限定与 expectedRevision CAS，审阅绑定不可变 digest；成果不是聊天缓存 | 保留正式事实和全局限额；[容量与命令守卫:58–70、292–390](<E:/Workspace/aboard-nexus/apps/host/src/workspace-build.ts#L58-L390>)；[Studio 文档](<E:/Workspace/aboard-nexus/docs/features/workspace-pages-and-studio.md>)；声明式 renderer 另验 |
| Knowledge/Workflow/Exchange | partial；本轮主要文档，dirty template 源码未完整审计；未迁移 | 各 Host 领域 owner → 保持资源/执行/adapter 单一归属 | KB/reference 不等于默认生产 RAG；Workflow Host-only opt-in/startReady:false 不等于 HTTP/UI 可运行 | 保留能力清单，专项逐域核对，不从 DTO/reference 测试推导可用；[架构:34–48、98–104](<E:/Workspace/aboard-nexus/docs/architecture.md#L34-L104>)、[功能入口:41–45](<E:/Workspace/aboard-nexus/docs/features/README.md#L41-L45>) |
| 本地身份/凭据/账户/审计 | 工厂差异与路径源码核对；完整权限面未审计；未迁移 | 原安装认证/FileSecretStore/Host → 安装身份/授权/凭据 owner | Web factory 与 standalone 默认不同；凭据不进 DTO/默认备份、不随目录走 | 不复制真实凭据；[Web/standalone 工厂:3581–3621](<E:/Workspace/aboard-nexus/apps/host/src/index.ts#L3581-L3621>)；OS vault、可信连接机制留技术验证 |

未列能力并非默认删除或完成替换。后续每一迁移功能必须更新本表，给出新旧差异、保留/退役策略与新鲜验收证据。
本轮没有批准或执行真实迁移；目标 owner 是责任方向，不预建这些模块。

## 已核实原容量：不得按工作区乘额

| 范围 | 精确原值 | 源码含义与证据 |
| --- | --- | --- |
| 单 Host controller | **合计最多 8** | 模块单例 retained Map，reserve 后才 callback；SEALED 且物理 tasks 不再 retained 才释放；[controller:10–39](<E:/Workspace/aboard-nexus/apps/host/src/native-run-controller.ts#L10-L39>)、[释放:275–276](<E:/Workspace/aboard-nexus/apps/host/src/native-run-controller.ts#L275-L276>) |
| 每 Run 截止/请求/工具 | **600000ms / 8 Provider 请求 / 16 工具尝试** | [native-run-budget:5](<E:/Workspace/aboard-nexus/packages/contracts/src/native-run-budget.ts#L5>)；固定累计预算不是并发数；保存许可后才授权派发，失败不退款或自动重试 |
| Build 安装级总量 | **32 项目 / 64 产物版本 / 64 审阅** | [BUILD_LIMITS:2](<E:/Workspace/aboard-nexus/packages/contracts/src/workspace-build-values.ts#L2>)；[workspace-build:58–70](<E:/Workspace/aboard-nexus/apps/host/src/workspace-build.ts#L58-L70>) 验整个 WorkbenchState 数组，不是每项目/目录各一套 |

M3 开放 Build 前必须跨两个以上工作区验证安装总量32/64/64及满额拒绝，不按目录乘额。
M4/M5 执行/Run 迁移验收必须跨两个以上工作区测总额、未确认保存无派发、逻辑终态不提前还槽。
M1 不实现 Run/Build，也不能声称这些容量已在新仓运行证明。

## 已读测试定义及其证明边界（未运行）

| 测试入口 | 读到的场景 | 不证明 |
| --- | --- | --- |
| [native-run-controller.test:31–118](<E:/Workspace/aboard-nexus/apps/host/tests/native-run-controller.test.ts#L31-L118>) | CLOSED 不派发、hung save、双时钟、跨二区 8 槽/第 9 拒绝、settle 后还槽 | OS/远端断电、真实 Provider |
| [native-provider-attempt.test:33–81](<E:/Workspace/aboard-nexus/apps/host/tests/native-provider-attempt.test.ts#L33-L81>)；[native-tool-attempt.test:105–136](<E:/Workspace/aboard-nexus/apps/host/tests/native-tool-attempt.test.ts#L105-L136>) | Provider 第9不触碰保存/Broker/secret/DNS/wire；工具16/17限制与终槽竞态 | 网络副作用端到端兼容 |
| [workspace-build.test:364–443](<E:/Workspace/aboard-nexus/apps/host/tests/workspace-build.test.ts#L364-L443>) | 64版本满拒增不剪历史，JSON/SQLite 重开 | 本轮未直接读到32项目/64审阅满额专测，不伪称已证明 |
| [目录生命周期:33–97](<E:/Workspace/aboard-nexus/apps/host/tests/workspace-directory-lifecycle.test.ts#L33-L97>)；[lease:57–119](<E:/Workspace/aboard-nexus/apps/host/tests/workspace-directory-lease.test.ts#L57-L119>) | 真 SQLite、seal、move；第二 Node 进程 busy/释放/kill/独立 EXCLUSIVE | 新 Go 驱动、逐区业务分库、网络盘/跨平台/硬断电/离线副本全局排他 |
| [sqlite-state-store.test:32–67](<E:/Workspace/aboard-nexus/apps/host/tests/sqlite-state-store.test.ts#L32-L67>) | missing read无写、restart、queued capture、独立连接 stale revision 拒绝 | 新工作区原子性、所有错误恢复 |
| [plugin-runtime.test:125–150](<E:/Workspace/aboard-nexus/apps/host/tests/plugin-runtime.test.ts#L125-L150>)、[staging:403–425](<E:/Workspace/aboard-nexus/apps/host/tests/plugin-runtime.test.ts#L403-L425>) | workspace/generation/broker 注册，staging失败保留旧代 | 第三方恶意代码沙箱 |

## 有界源字节指纹

以下 SHA-256 指向观察时的文件字节，不是跨语言规范化摘要。仅用于发现旧基线漂移；不是认证或执行授权。
其他片段须重新读取；没有保存全仓镜像或真实数据。诊断完整清单保留在 Git 忽略目录。

| 文件 | SHA-256 |
| --- | --- |
| [预算常量](<E:/Workspace/aboard-nexus/packages/contracts/src/native-run-budget.ts>) | `be4ca1ad31c74de4a45eb230328f61482276b0f01fb2ee17b289727e6749748c` |
| [Build 常量](<E:/Workspace/aboard-nexus/packages/contracts/src/workspace-build-values.ts>) | `3bc3a68f544845db7751df463fe4ad26b4f3fed55c5b5125050440493495bead` |
| [controller](<E:/Workspace/aboard-nexus/apps/host/src/native-run-controller.ts>) | `df3b917f79f0c238bbac0e680249e3cdde8dc2e6c54ab46b1f6be0b5e5efb0a5` |
| [Build owner](<E:/Workspace/aboard-nexus/apps/host/src/workspace-build.ts>) | `ab5b20069a181045bd6c45c18909b74321a926912a04994ae440ab3fa40bb7d8` |
| [身份编码](<E:/Workspace/aboard-nexus/apps/host/src/workspace-directory-identity.ts>) | `e7659c15ee41176f671a7a6eff6e6fe09c45b99b1d287b1da654b8f8c1923909` |
| [聚合 SQLite owner](<E:/Workspace/aboard-nexus/apps/host/src/sqlite-state-store.ts>) | `2d16490d34ddc4f84e334fad79154f23fc57cf62a38682e8c56af4c341c893b2` |

跨语言摘要、序列化、字段排序、Unicode/换行、数值、opaque 版本比较的权威夹具尚未建立，归后续 contract research。
