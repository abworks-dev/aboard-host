# 001 Implementation Plan — 独立 Go Host 与目录工作区基础闭环

**设计状态：proposed/designed，2026-10-18；未经运行证明的技术提案仍未验证。**
**授权入口**：当前功能授权与历史批准仅见[授权记录](<authorization.md>)；沿用[同一规格](<spec.md>)及其6个P1故事、FR-001–028、SC-001–009，未改需求正文。
本文件§8及工具表、“本轮”未安装/未运行说明均保留计划交付时的历史事实，不再定义当前阶段上限；执行依赖与完成门见[任务清单](<tasks.md>)。行为设计与技术验证门仍有效，运行结果须另记新证据。

## 1. 目标、上下文与权威

实现目标不是health endpoint，而是：独立进程认证 → 全新A/B真实目录/SQLite → 非执行文本预览确认 → 正式保存/更新 → 关闭 → 完整结束Host → 新进程重开 → 逐字段读回；同时证明跨进程排他、坏A不无故阻断B、越权与零写inspect拒绝。
总定位、唯一M0–M7路线及非目标仍在[总体范围](<../../docs/scope.md>)；不复制新路线。旧能力证据仍在[迁移基线](<../../docs/migration-baseline.md>)，本轮不重访旧工作树/真实数据。

| 技术上下文 | 本轮状态/提议 |
| --- | --- |
| 工作区 | 实际根 E:\Repository\Github\aboard-host，独立Git master无提交/remote；现有产品/工具文件均先读再改 |
| Spec Kit | CLI 1.2.1固定官方commit；本轮使用已安装AB 0.1.0 speckit-plan；不是重初始化core plan |
| Go | 本机1.27.0；目标固定1.27.2，未安装；module path待真实remote/项目所有权确定，本轮不建go.mod |
| 支持提案 | Windows 11 x64 + Go windows/amd64，本地直连NTFS普通非重解析目录；通过验证后才称supported |
| 架构 | Go模块化单体+端口适配器；标准库HTTP/JSON/context/crypto；无前端、旧Host或微服务依赖 |
| 持久化 | 真实SQLite；首选候选modernc v1.60.1，版本化SQL migration；每区独立库，安装库只管安装事实 |
| 事务提案 | DELETE + synchronous=EXTRA + foreign_keys=ON，逐连接回读；每区单连接/串行提交者，默认不启WAL |
| 公共协议 | loopback HTTP命令/查询；本片不实现持久SSE/Run；明确unavailable而非空成功 |
| 新设计附件 | [研究](<research.md>)、[数据模型/摘要](<data-model.md>)、[公共协议](<contracts/api.md>)；均是本feature必要附件，非第二规划体系 |

所有平台/锁/驱动/DPAPI/持久性选择是待验证技术设计，不是accepted ADR。当前可重用的是规格/治理/工具流程，而非已存在Go实现。

## 2. 最小组件、唯一 owner 与依赖顺序

默认后续实现仍由**主Agent**负责每个组件和集成/Git；不提前创建team或并行写相同契约。两名研究子Agent只读官方资料，不是业务组件owner。若将来并行，先固定共享契约归主Agent，再明确独立文件边界。
下表是计划工作顺序，不是tasks.md或已执行任务；路径仅是未来拟议组件，不本轮建目录。

| 顺序/单位 | 拟议组件/职责 | 依赖/集成点 | accountable owner |
| --- | --- | --- | --- |
| D0 技术门与契约冻结 | 固定Go/driver/嵌入engine/Windows profile；字节夹具、错误/状态与认证/锁证明 | research G1–G6；失败先修设计，不弱化规格 | 主Agent |
| D1 安装安全与独立服务 | cmd/aboard-host；安装库/DPAPI/ACL/host.lock；操作员pipe、会话、标准库HTTP | D0；身份授权与协议唯一owner，不接旧Host | 主Agent |
| D2 目录身份与协调 | workspace admission、Windows file handles、安装登记/create journal/relink、只读inspect | D1授权、D0锁/路径；只能显式create写新目录 | 主Agent |
| D3 每区业务事实 | workspace core + sqlite adapter + versioned migrations；文本/候选/receipt | D2排他、D0字节；安装登记不复制业务表 | 主Agent |
| D4 保存闭环 | preview/confirm、区串行事务、未知结果隔离/调查、正式读取 | D3及当前授权；receipt优先于旧base判断 | 主Agent |
| D5 双区生命周期 | 独立handle/openEpoch、排空、close/shutdown/reopen、局部故障与依赖门 | D2–D4；共享安全事实不确定不能绕过 | 主Agent |
| D6 对外真实验证 | 外部客户端+第二进程harness、故障注入/重启、静态契约一致性 | D1–D5全部；正式保存/新进程重开归001，不延期M2 | 主Agent |

端口只为真实消费者：Identity/Authorization、InstallationStore、DirectoryAccess、WorkspaceStore、OperationClock/IDs与HTTP适配；不造通用bus/plugin/executor框架。业务核心不导入HTTP、Windows或SQLite具体实现；适配器不自行扩大权限。
D2或D3单独完成只可称partial；001完成要求D6对全部US/SC给出证据。内部可分子片，后续tasks须仍将最终闭环明确归001并连好依赖。

## 3. 关键协议设计

### 3.1 可信本地连接与安装权威

安装进程独占安装host.lock；安装库持稳定installationId/tenantId/principalId、授权/登记/安装操作，DPAPI用户作用域材料在严格SID ACL文件。首建、打开、不可读/坏数据明确分支；不在未知状态换位置、换身份、重置秘密。
回环HTTP仍每请求认证，精确Host校验，拒Origin浏览器访问；会话经本机操作员pipe显式授予，pipe校验OS SID/服务器身份，不把PID当用户。客户端不直读安装数据库、secret文件或任意服务器FS；pipe是Host受控能力，不是第二业务owner。
首次引导、pipe防抢占/远端拒绝/impersonation还原、过期撤销、ACL/DPAPI失败详见R5/G5。具OS操作员权限者能显式管理授权，不意味着受限HTTP客户端有该能力；同用户恶意原生代码/管理员不是已防住的威胁。
共享授权改变或安装库提交结果未知时，依赖该事实的准入/提交一律停；已接收操作也须在业务事务开始前重校验当前授权版本。不能为了B可用而沿用不可信授权cache。

### 3.2 创建、登记与目录打开

- formatVersion=2提案表达真实业务目录；schema=1/API=v1各自版本。旧身份v1只识别并拒绝，不扩表假兼容。
- create必须获准root且全新空目录；安装意图先持久化，再CREATE_NEW锁对象并取得排他，初始化新库genesis、写描述，最后登记/初始grant确认。任何既有/部分文件不覆盖；跨文件/双库没有天然原子事务。
- 每阶段operationId/输入摘要/phase可调查。中断残留、登记结果未知均拒open；显式reconcile仅查证并完成原登记，不创建丢失库/默认业务/新ID。用户另授权前不提供真实数据修复命令。
- register只承认已有、格式/身份一致、当前授权合法实例；同UUID/物理root重复不复制状态；同UUID不同root拒副本。不同安装不能借目录继承grant/密钥。
- open只OPEN_EXISTING：先校验允许位置及句柄真实身份，取得固定writer.lock share=0且非继承句柄，重核root/文件身份、manifest、SQLite/identity/正式摘要；完全通过后发布可写handle/openEpoch。缺文件、未来版本、损坏均不补建。
- SQLite可能在RW open时恢复其自身hot journal；这是**明确获授权open的存储恢复**，不是inspect或补种。须先区分可识别本片日志恢复与未知/不兼容数据；恢复完成仍校验正式内容/receipt，不能自动重放业务。无法证明则拒绝并维持unknown/needsReview。
- 同卷停机rename后显式relink；关闭/已排空/无未知前提+原root file ID一致，保持UUID/正文。复制或跨卷变身份不能伪称同一rename；不交付通用搬家/克隆/备份工具。

### 3.3 无写inspect

不把驱动接到被检源；先以只读OPEN_EXISTING、share=0持有已有writer.lock，缺锁/争用则拒且不创建；再以只读源句柄禁止写/删共享并持续验证祖先/root/文件身份，无日志/活写者且冻结成立后，复制到私有scratch，以固定file URI mode=ro查询副本。scratch是本计划提议的敏感临时检查缓存，不是普通日志/脱敏输出或第二主库；严格ACL、不含于备份、检查后销毁，失败报告未清理。本轮没有创建产品scratch；后续合成验证仅在本仓`.data/logs/`，不写被检目录。
源manifest、DB、锁与祖先替换的共同稳定性必须在G3/G4验证；没有排他证据或见journal/WAL就返回unavailable/recoveryRequired，不取巧删除日志、不切RW、不用immutable假装活库静止。
输出只证明检查当时的快照，不证明之后open安全，不登记/授权/迁移/激活或Provider调用。inspect前后目标递归清单/字节及写操作观测必须不变；系统访问时间/外部进程背景活动与inspect归因分开记录。

### 3.4 预览、正式提交与未知结果

TextRecord、不可变TextVersion、Candidate、SaveOperation与Receipt同区库，全部显式workspace/tenant，文本字节在库中，无对象协议依赖。preview持久化候选但不修改正式Head；create/更新均绑定区Head baseRevision，更新另绑原contentDigest。
确认先重验主体/target并调查同operationId既有关联；同输入只返回原结果，不先被候选applied/expiry/旧base/旧epoch挡住，不再次写入。只有全新operation才重验当前epoch、候选digest/previewVersion/expiry/基线，确证admission后由该区提交者短事务写版本/Head/receipt。区已关闭或无可信读视图时须获准重开调查，不自动open。receipt与正式效果同事务，存储Commit已确认后才可报告业务成功；HTTP回执交付丢失不逆转已确认提交。内容/基线变化须新预览。
断连只移除等待者：业务生命周期不直接使用HTTP request context。内部取消/timeout触发收尾而非“已经回滚”；存储调用尚未结束时不释放lock/连接/epoch。
Commit/busy/IO/取消错误须按驱动实际行为分类；错误不等于回滚。同operationId查已存在receipt、重新授权，不启动第二提交；不同inputDigest拒key重用。未知时封住A写入，按operationId调查并等待原事务物理结束，重获排他后的新可信视图才允许确定已/未提交。不能用旧snapshot未见receipt宣布安全重试。
本片receipt不裁剪；不支持备份历史替换/自动续跑。保存key明确为(workspaceId, operationId)，安装key为(installationId, operationId)，跨区无全局查重；调查必须保留原作用域，不能换区/换key重试unknown。只保证同一持久历史、同区同key的至多一次正式效果，不宣传通用exactly-once。正式提交与视图/回执交付失败分别表达；无投影写入，不覆盖未确认候选。

### 3.5 状态、关闭、重启和隔离

业务ready/unavailable、目录lifecycle、accessMode、integrity、persistenceHealth独立；操作logical/physical/persistence独立。inspect不是readOnly open。Run本片不存在，不给假Run状态或伪取消成功。
close先原子拒A新准入，等已accepted保存实际结束/调查，关闭连接后释放OS锁；超过等待窗口仅报告draining/unknown。A旧epoch的命令/迟到回调拒绝，B不复用A context/队列/DB。正常Host停机逐区同规则；强杀不伪造“排空成功”，重启重新验证锁与持久历史。
每区单连接/独立工作队列，候选错误/坏库/未知保存限A；安装DB只做短协调事务，不持全局临界区等待A IO。队列提案每区32、安装合计64个未结束HTTP操作（含排队/执行/调查），准入前429，不绕过未知收尾释放名额；每区独立推进，A不能占满B的执行循环。冻结等待与资源上限在G1/G6观测后调优，不把这些数误当Run/Build预算。
共享Host crash/OOM/磁盘满/机器断电可影响全部区，单机/单进程不是OS隔离。将来高风险worker需输出限制/超时/排空/安装总容量，当前不实现每区Host或插件执行。

## 4. 逐项需求与验证追踪

下表为计划覆盖检查，不是测试通过记录。验证V组详见下一节；每个FR均保留，不以范围审查伪装Run已实测。

| FR | 设计锚点 | US / SC / 验证归属 |
| --- | --- | --- |
| FR-001 | §2 D1，§3.1 独立标准库HTTP | US1 / SC-001 / V1 |
| FR-002 | §3.1 稳定安装身份、pipe引导、逐请求session | US1 / SC-001/004 / V1/V7 |
| FR-003 | §3.1；协议§1复合目标/当前授权 | US1/5 / SC-004 / V1/V5 |
| FR-004 | §3.4–3.5 Host拥有已accepted操作 | US1/3 / SC-009 / V4/V6 |
| FR-005 | §3.2–3.3 分离五种操作 | US2/6 / SC-006/008 / V2/V7 |
| FR-006 | §3.2 create意图/genesis/finalize | US2 / SC-001/006 / V2/V4 |
| FR-007 | §3.3 源零写、私有敏感检查缓存 | US6 / SC-006 / V7 |
| FR-008 | §3.2 版本/身份/摘要/锁后验证 | US6 / SC-006 / V2/V7 |
| FR-009 | §3.2 定位/授权分离，同卷relink/副本拒绝 | US2/4 / SC-008 / V2/V3 |
| FR-010 | 模型§1 目录2/schema1/APIv1 | US2/6 / SC-006 / V7/V8 |
| FR-011 | 模型§3–4 最小非执行文本 | US3 / SC-001/002 / V2/V8 |
| FR-012 | §3.4；模型§5 确认绑定完整预览 | US3 / SC-007/008 / V4/V8 |
| FR-013 | §3.4 版本/基线/并发冲突 | US3 / SC-002/008 / V2/V4 |
| FR-014 | D6新进程完整重开而非内存 | US3 / SC-001/002 / V2 |
| FR-015 | §3.4 receipt/unknown/fence | US3 / SC-007 / V4/V6 |
| FR-016 | 模型首段文本入库，无对象引用 | US3 / SC-001 / V8；对象协议M2，未伪装完成 |
| FR-017 | §3.4 候选不自动生效，协议501 | US3 / SC-007 / V2/V8 |
| FR-018 | §3.2 Windows句柄排他 | US4 / SC-003 / V3 |
| FR-019 | §3.4–3.5 重获排他/持久验证 | US4 / SC-003/009 / V3/V6 |
| FR-020 | §3.5 close准入/排空/epoch | US5 / SC-005/009 / V5/V6 |
| FR-021 | §3.5 每区独立库/队列/handle | US5 / SC-002/005 / V2/V5 |
| FR-022 | §3.1/3.5 局部坏与共享依赖门 | US5 / SC-005 / V5 |
| FR-023 | 模型§6 独立状态轴 | US5/6 / SC-009 / V6/V8 |
| FR-024 | §3.5 本片不实现Run；不复制安装预算 | US5 / SC-009 / V8范围审查；Run实测M4/M5 |
| FR-025 | §3.5 共享基础设施局限 | US5 / SC-005 / V5/V8；不声称完全故障独立 |
| FR-026 | 协议§4 错误/关联/脱敏 | US1–6 / SC-004/006/007 / V1/V4/V7/V8 |
| FR-027 | §5 合成根/证据/拒空交付 | US1–6 / SC-001–009 / V1–V8 |
| FR-028 | §1、协议unavailable、无业务实验 | US1–6 / SC-006 / V8范围审查 |

## 5. 验证设计：全部业务行为 unverified

运行前提：按[授权记录](<authorization.md>)满足阶段门，固定Go/driver/Windows profile、全新合成安装/目录、主Agent控制真实进程启动/终止和故障注入。计划交付时未执行下表；当前执行状态以任务和新证据为准。
本仓开发/验证所有输出、临时合成目录、工具状态位于Git忽略的`.data/`，日志/诊断只`.data/logs/`；建议合成根也设在`.data/logs/001-validation/`，不得借用旧项目或真实用户home/库。运行时产品数据根由Host受支持配置决定，不与开发日志规则混同。

| 组 | 拟议过程与反例 | 必须保留的证据 / 对应SC |
| --- | --- | --- |
| V1 独立与认证 | 不启前端/旧Host/Provider/插件；外部客户端完成引导；错token/过期/撤销/Host/Origin/错误SID/跨区主体 | binary/工具版本、启动参数脱敏、身份保持与全部拒绝；SC-001/004 |
| V2 正式闭环 | A/B各>=10条正文，交错创建及各>=1更新；关闭所有区/终止Host/新PID重开，逐字段对照；候选/手改文件不自动生效 | 两库真实文件/版本及逐字段比较、旧进程确实退出、新PID、没有缓存依赖；SC-001/002/008 |
| V3 跨进程排他 | 持有、同时竞争、kill各>=10轮；正常释放、PID残留/伪造、锁删除/替换、继承句柄、别名/路径换位/副本 | 子进程ID/时间线、OS拒绝码、最多一写者及失败者零保存、重新排他/数据保持；SC-003/008 |
| V4 保存边界 | admission前失败/未知、commit前/中/后异常、丢ACK、busy/满盘合成故障、并发同base/同key异载荷、陈旧候选/权限撤销 | 每类>=1；正式旧/新状态、唯一receipt、unknown围栏/调查、无自动重放或提前success；SC-007/008 |
| V5 两区隔离 | A关闭/B新增更新读取；A停机坏库/B同样操作；伪造workspace/resource/candidate/operation；A长IO/队列满；共享权限未知 | B三操作成功与内容归属；局部/共享阻断理由；串区/泄露零例；SC-004/005 |
| V6 排空与迟到 | 断连、close中提交悬置、Context timeout但物理仍活、迟到回调、旧epoch；重启调查未知 | 连接/逻辑/物理/持久时间线；锁未早放、close未假完成、B独立；SC-009/003/007 |
| V7 坏目录无副作用 | 每类>=1：缺库/坏digest/伪造identity/未来format/schema/旧v1/不存在；另hot journal/零字节/reparse/不支持FS/不安全ACL/DPAPI失效 | inspect前后全清单/字节/写操作观测、open拒绝、外部调用0；受限错误不泄漏；SC-006/004 |
| V8 契约/静态与回归 | 固定字节夹具、revision精度/溢出、JSON重复键、未知字段/DSN注入、公开DTO秘密扫描、文件链接/ID覆盖；capability unavailable与范围 | 结构与独立字节expected值、拒绝反例、当前变更指纹；SC-001–009各自相关；非运行替代 |

拟议工程命令在有真实模块后才确定：`go version`、`go env GOOS GOARCH CGO_ENABLED`、`go test ./...`、目标binary构建/外部harness。`go test -race`须先验证Windows/cgo工具链可用，单独记录；race/编译/Mock都不能代替V2–V7。
第二进程锁harness使用同一Windows适配器及合成授权夹具，在进程边界真实竞争/尝试保存，不开生产后门；单安装第二完整Host可先被host.lock挡住，因此不能只测它就宣称writer.lock已证明。锁harness须专门覆盖区锁与真实SQLite，测试模式不得变成对外绕授权API。
不在普通HTTP暴露故障注入；使用测试构建/内部适配器，数据根严格限制合成前缀，证据留PID/输入/故障阶段/观察值。设计中所有happy与negative都需执行，不能删失败反例换绿。
真实kill证明的是进程崩溃恢复，不是硬断电耐久。SQLite EXTRA、OS FlushFileBuffers/设备缓存行为尚无实测；没有设备/故障证据时明确unverified，不宣传确认提交抵抗一切断电。

## 6. Constitution Check（逐项，设计覆盖≠运行通过）

基准[宪章1.0.0](<../../.specify/memory/constitution.md>)。当前无请求豁免、无已知需修改原则的设计；以下运行证明全待后续，不能填“passed”。

| 原则/不变量 | 具体设计证据 | 未解决验证/非适用理由 |
| --- | --- | --- |
| I / INV-01 | §2单体/唯一owner，§3.2登记非业务副本，§3.4区提交者 | V2/V3/V5；不接旧Host |
| I / INV-02 | 模型§2–3 identity/复合归属；§3.1稳定主体 | V1/V2/V5 |
| I / INV-03 | 协议每路径workspace+当前context，无全局current | V1/V5 |
| I / INV-04 | §3.2同卷停机relink、UUID非路径 | V2/V3；跨卷移动工具不在片内 |
| I / INV-05 | root物理ID登记/同UUID副本拒绝；跨安装不继权 | V3/V7 |
| II / INV-06 | §3.3源不接SQLite、不可冻结就拒绝 | G3/G4,V7；scratch敏感缓存另隔离/销毁，不是日志 |
| II / INV-07 | §3.2 OPEN_EXISTING、严格版本/摘要、拒残留 | V2/V7；绝不补种fallback |
| II / INV-08 | Windows share=0句柄生命周期，非PID | G4,V3；真实OS证明待做 |
| III / INV-09 | §3.4正式版本+receipt原子，存储Commit确认后才success | G6,V4；本片无执行消费者 |
| III / INV-10 | §3.4 unknown围栏/原key调查，不自动重放 | G6,V4/V6 |
| III / INV-11 | 模型Candidate baseRevision/baseDigest+重预览 | V4/V8；通用外部候选M3 |
| III / INV-12 | 协议commit与viewDelivery分报；无投影写候选 | V4/V8；完整投影M2 |
| III / INV-13 | 本片正文入DB，不创建对象引用 | 无对象时条件不适用；M2必须独立字节先行证明，不称已完成 |
| III / INV-14 | §3.2安装意图+残留/unknown；研究G2硬件边界 | G2/G6,V4；无跨文件/两库/断电保证 |
| IV / INV-15 | §3.4–3.5 physical/persistence独立、timeout不释锁 | V6；Worker/Provider实测M4/M5 |
| IV / INV-16 | close排空/未知保留锁，停机relink前置 | V3/V6；备份/搬家工具M7 |
| IV / INV-17 | HTTP断连不取消accepted操作 | V4/V6；Run独立取消后期 |
| V / INV-18 | confirm/reconcile/receipt均当前授权；摘要非凭据 | V1/V4/V5 |
| V / INV-19 | 安装队列总额与每区限额；不复制旧8/32等预算 | V5/V8；Native8、600000ms、Provider8、tool16与Build32/64/64保留迁移基线，不在M1假跑 |
| V / INV-20 | DPAPI/SID、token受控交付、脱敏DTO/日志 | G5,V1/V8；默认备份本片无功能 |
| V / INV-21 | §3.5共享故障明示；不称目录/子进程为sandbox | V8文档/权限审查；恶意同用户原生不保证 |
| VI / 数据隔离 | 独立库、复合FK/授权，外区ID/cursor拒绝 | V2/V5；对象/事件/产物后续同边界 |
| VI / 生命周期与故障 | 每区handle/队列；局部unknown vs安装依赖门 | V5/V6 |
| VI / 执行与资源 | 本片无执行器；有界HTTP准入、不复制预算 | V5/V8；高风险worker、输出/取消/排空M4/M5，未宣称完成 |
| VI / 共享基础设施 | 共享进程crash/OOM/满盘/断电可跨区 | V5/V8；无每区Host增强 |
| VI / 跨区协作 | 默认拒绝，目标owner/授权原则仍见总体范围 | 具体业务协作后续独立spec，不造bus |
| VII / 独立与可换客户端 | Go+公共HTTP，外部客户端验证，无旧TS依赖 | V1/V2；React/第二产品客户端M6 |
| VII / Studio真实可用 | 本片capability unavailable、无假页面/执行 | V8；声明式renderer/Build M3及后续，不假称implemented |
| 架构/安全约束 | §1–3及研究G1–G6，禁止框架清单未新增依赖 | SQL migration/单一结构权威待工程落实 |
| 工作流/治理 | 同001；当前授权及历史只引用[授权记录](<authorization.md>) | 按授权与真实技术门推进；无原则豁免或替代规划体系 |

后续任何改动驱动、日志模式、inspect冻结协议或认证引导，必须同步本计划/附件对应锚点并将旧受影响证据标历史、重新验证；不能用治理降级掩盖技术失败。

## 7. 风险、恢复与评审门

### 必须技术验证后才定案（不是让用户猜技术事实）

1. G1驱动版本/SQLite/Go/libc与许可证/安全修复矩阵：modernc v1.60.1优先验证；mattn v1.14.52仅比较候选，切换须记录/重验证，不自动fallback。
2. G2 DELETE+EXTRA的Windows VFS flush语义与故障范围；文档不是本机断电证据。
3. G3/G4 inspect持续冻结/源零写、路径/文件身份与锁竞争；证明失败就拒此操作，不读写源“修复”。
4. G5 OS安全引导、pipe/SID/ACL/DPAPI及session撤销；不可用时阻断可信连接，不退化loopback免认证。
5. G6提交错误到rejected/unknown的驱动映射、原事务物理结束、receipt调查和close排空。

### 安全回退与数据边界

本轮只有文档，review时保留未提交变更；不重置工作树或自动提交。后续工程只用新合成目录，失败保留证据、不迁移/删除真实数据。
实现失败可回退binary/设计，但不能用旧binary打开新schema或静默降版本；已确认正式数据不靠删库“恢复绿”。未知保持围栏，显式调查；发现损坏先停止对应范围、只读保留证据，真实数据修复/备份恢复另授权。
本片没有旧库迁移/凭据继承/目录克隆/通用备份升级协议，因此不能宣称替代旧Host完成；旧dirty基线/容量证据的未核实项保持原状态。

### 计划交付截面与当前入口

- 计划交付时已完成必要技术文档研究、001设计与US/FR/SC/宪章静态覆盖；§8保留当时实查，不证明后续运行结果。
- 当时所有G/V未运行，真实业务Host及保存/第二进程/重启验收尚未完成。其后阶段批准不在本文件重复维护，只见[授权记录](<authorization.md>)。
- 执行进度与证据见[任务清单](<tasks.md>)；未验证技术不因任务批准变为事实。

## 8. 本轮静态自查记录

2026-10-18本轮实查（只限静态文档/环境，不是G/V运行验收）：

- 已先独立同步AGENTS、README、scope、初始化记录、宪章阶段说明、spec状态与requirements门状态，保留原规格正文/21不变量；新增本feature的plan/research/data-model/contracts四附件，未生成tasks或业务产物。
- 12份产品/评审文档：53个本仓链接、1个heading/行锚检查成功；42个既有旧仓证据链接沿用历史、未重读/重新认证旧树。占位符、覆盖行与常见秘密模式检查无发现；扫描不是完整安全证明。
- 逐行核对28个FR与21个INV各一行；6个US及9个SC均有设计/验证归属。人工检查单一owner、未知不重试、零写inspect、局部/共享故障与最终重启闭环，未将对象/Run/插件延期功能假称本片实现。
- 独立只读审阅发现并关闭2处P2：既有operation必须先当前授权/同输入调查，不能被候选applied/expiry/旧base/旧epoch误拒；保存key明确每区作用域，非跨区全局去重。修改后定点复核两项均关闭；该审阅未独立复核网页、未运行实现。另由主Agent明确inspect先占已有锁/冻结源与敏感scratch边界，以及close内存协调状态不冒充持久保存receipt。
- Specify `version`与`integration status`均exit 0；CLI 1.2.1，default/installed=dsh，missing/invalid/unchecked均0。状态为WARNING而非M0的OK：4个受管SKILL已被现装AB 0.1.0替换，正文逐一与已启用preset源匹配；其余18个官方文件摘要一致。未修改/升级生成技能、模板或脚本；未强制恢复4项。
- `git diff --check` exit 0；因尚无提交/全为untracked，另对54个共享文件逐一作`git diff --no-index --check NUL <file>`与完整新增差异保存，exit 1只表示存在新增差异，无空白诊断。检查Git忽略覆盖`.data/`工具/缓存/日志、feature指针与DSH非skills本机状态；共享可见文件无上述本机状态、go.mod/Go代码/数据库/exe/tasks。
- 工作目录/Git根仍为E:\Repository\Github\aboard-host，master无提交/remote/暂存；未改全局Git身份/旧项目/真实数据，未启动Host/Provider/插件，未push/发布。诊断/差异/指纹仅本仓`.data/logs/`；官方网页工具自动spill局限见research末节。
- 本会话已实际加载并执行现装`/speckit-plan`指令，无需为这些技能刷新会话；没有把slash技能当终端命令，亦未运行mutating setup/helper/hook。仍待用户计划评审；所有G1–G6与V1–V8是unverified。

## 9. 长期数据设计校准与当前实现边界（2026-10-18）

**状态：designed，尚未实现。** 用户另行确认只修订 scope、data-model、plan、research 四份文档，不安装新规范；本次确认不是整份计划批准或 tasks/implement 授权。§8保留此前交付的历史检查事实，不借用其结果声称本次已验证。长期分类/延期项的唯一入口为[总体范围 §3.1](<../../docs/scope.md#31-长期数据策略与单一实现范围designed未实现>)，本节只映射 001 范围、依赖与验证门。

### 9.1 当前只实现一种本地方案

后续获准实现时，仅采用安装库 + 每区 SQLite；M1 最小文本直接入库。完整产品的对象文件归 M2，不在 M1 预建目录、表或引用。modernc v1.60.1 仍是首选待验证候选，mattn 仅比较备选，不交付双驱动或自动 fallback；若候选不满足 G1/G6，先记录失败并修订设计，再重验受影响 profile。

每区单连接/串行提交者、DELETE/EXTRA、逐连接 foreign_keys 回读继续按当前提案验证。长期保持单一正式写入 owner，不把永久单连接作为产品约束。只为已有用例建立 WorkspaceStore 等业务端口；不引入通用 Repository、可插拔数据库框架或空 Worker/事件设施。

| 类别 | 001 落地/验收归属 | 后续边界，不计入本片实现 |
| --- | --- | --- |
| 事实与版本 | D3/D4：文本、候选、不可变版本、Head、同事务回执与审计；V2/V4/V8 | 对象及投影 M2；不能以此延期首片正式保存/重开 |
| schema 演进 | D0/D3：新库有序 SQL、各库 ledger/校验和、genesis 原子性；G6/V7/V8验证版本和 ledger 矛盾拒绝 | 已有库升级/降级器及旧数据迁移另行规格；open/inspect 不自动升级 |
| 并发与资源 | D3/D5：当前 profile、短事务、独立队列/epoch、安装总容量；G1/G2/G6及V3–V6 | WAL、有界读连接、分页/索引/归档优化需消费者或实测证据，不静默切换持久化协议 |
| 生命周期与恢复 | D2/D4/D5：残留/unknown 调查、锁/连接实际排空、完整重启；V2–V7 | 对象 GC、快照/备份恢复与移动/克隆工具归 M2/M7对应功能；不是本片新增工具 |
| 执行与订阅 | 当前明确 unavailable；V8只检查边界声明 | Run、事件重放/缺口/保留期/背压归 M5，不将订阅与数据库事务绑定 |
| 数据库与部署扩展 | 当前不增加实现；保持授权/事实 owner 与驱动适配边界 | PostgreSQL/服务端/多机写者/动态切换及专用索引按总体范围触发条件另行评估 |

### 9.2 必须保留的失败边界与验证影响

- 新库 ledger/schema/user_version 不一致、未知校验和、未完成初始化、旧/未来 schema 均拒绝，不在 open/inspect 修复；补充反例见[数据模型 §7](<data-model.md#7-首片数据实现范围与演进门designed未实现>)。D0 固定夹具与预期，D3实现，沿用 G6/V7/V8 验证，不新建竞争任务清单。
- 数据库事务不等于整个工作区的生命周期锁，也不覆盖对象文件或两个数据库。当前文本闭环无对象依赖；后续对象/备份功能必须独立验证引用、保留、快照和恢复一致性，不能用复制 DB 或检查点成功替代完整证明。
- M1 receipt 不裁剪，同一未替换历史的原 key 调查保证不变。旧快照恢复/分叉不能自动延续该保证，更不能从无 receipt 推导重放安全；这些能力在 M7 的显式协议与授权前不可用。
- 未确定本项目性能 SLA，不把 SQLite/驱动资料当容量证据。本次只补设计和后续门，未运行驱动对比、压力/故障实验、migration 或任何业务服务。所有 G/V 运行门继续 unverified。

本次文档检查只验证四文件范围、链接/锚点、范围一致性与差异，不替代运行验收。沿用现有 Spec Kit/AB 与宪章，不修改需求正文、公共 API、工具配置、生成模板/技能；不新增 feature、tasks、业务代码、go.mod 或依赖，不 stage/commit/push。

## 10. 当前执行证据与停止门（2026-10-18）

前文为设计交付历史，不替代当前[授权](<authorization.md>)或运行记录。最初T004虽正常profile及进程kill/重开通过，但Windows刷写失败负例曾 **V003/G2 failed**，当时正确停止T003/T005。固定libc把原生FlushFileBuffers失败返回-1、win32 VFS将非零当SQLITE_OK，是真实错误传播缺陷，不是普通硬断电限制；原始失败与完整记录保留于[研究T004](<research.md#t004--v003-存储前置实测与g2阻断2026-10-18>)。

**首次冻结历史：T001–T009当时在各自前置/原语范围完成，V001–V007（含V002a/b）passed；不覆盖冻结后的源码。** 经新增授权，只在项目可再生构建副本作单行BOOL失败返回修正，保留固定源/许可证/diff/遗漏修正拒绝，真实SQLite COMMIT逐刷写故障及正常/强杀重开复验后才解除G2停止；未改module cache、未启用备选或放宽DELETE/EXTRA/确认语义。G4三轮真实进程竞争、G3三轮有界PID写守恒+NTFS通知/ACL/清理、G5三轮原生SID/DPAPI/pipe/remote反例及独立审阅已记录于[研究](<research.md>)，精确任务/V状态由[任务](<tasks.md#verification>)维护；这些不是完整SC验收。

T008/V007存储提交与ledger门已完成；T009于2026-10-18组合复验收齐当前前置证据：独立协议/摘要/绑定漂移检查、依赖完整性、全仓vet与全仓真实测试三轮均exit0。唯一工具/连接/路径/安全profile与当前源码指纹见[研究T009记录](<research.md#t009--前置组合签收2026-10-18>)；只解除Phase B/C前置停止，不将六包测试当完整Host验收。G6创建协调/端到端调查/物理排空仍需V008/V010/V014，001正式保存→完整Host退出→新进程重开闭环仍由T027负责且未验收。没有真实数据迁移、Provider/插件运行、旧仓修改、提交/push或发布。硬断电、完整path/failed-attempt trace和恶意同SID代码沙箱不在上述原语通过结论中。

本次恢复执行：profile物理关闭及安装材料源码变更后，受影响V003/V006/V007曾重新unverified、T009重新打开；现已focused及全仓组合三轮实际exit0并以新输入/命令/日志指纹重新冻结T009，保留旧结果，不复用旧源pass。T013/T014中已有安装操作调查/reconcile归业务层，HTTP不协调；当前下一批为重新冻结后T010→T012→T011及后继依赖，而非重新从T001初始化。最新实测见[冻结后复验](<research.md#t009--冻结后变更与重新验收2026-10-18>)。

2026-10-19实现截面：T010–T017在tasks既有局部完成门范围签收；T016的OPEN_EXISTING/当前定位授权/唯一生命周期、完整历史校验、保守DELETE日志私有副本预恢复及cold unknown围栏，已用真实Windows/SQLite子进程和原生资源复验。最终四包三轮、全仓一次及vet全部exit0；[源码、命令、历史失败与限制](<research.md#t016-既有区打开与保守日志恢复2026-10-19implemented联合验收仍unverified>)保留独立证据。T017的当前正式读取/逐次授权/作用域游标亦经真实A/B与受限主体、三轮focused/全仓/vet验证，详见[研究T017](<research.md#t017-当前正式记录读取2026-10-19implementedv011v013联合验收仍unverified>)。随后T018不可变持久预览与T019唯一正式保存用例亦通过各自真实Windows/SQLite局部三轮、全仓/vet；T019原生admission/正式COMMIT六个刷写边界逐项故障验证unknown保留物理资源，并验证admission后真实IO内取消等待者不取消已接收提交，详见[研究当前证据](<research.md>)。T020原保存结果调查及可信物理收尾也已按局部门完成：六个真实native刷写失败、冷accepted历史、恢复清理失败重试与当前授权/取消交付边界经focused三轮、全仓/vet通过，完整同历史正面证据才可判NOT_COMMITTED，缺admission仍unknown且不重放。T021独立队列、32/64有界准入、控制/排空优先与公开close/shutdown也经局部完成门签收：真实A正式COMMIT原生回调悬置及满队列期间B实际保存/读取，等待取消/关闭超时保留原生锁，六刷写故障后的unknown close与显式调查、代际handoff/停止竞态均有证据；最终四包三轮、全仓/vet exit0，详情见[研究当前证据](<research.md>)。下一依赖为T022；T022–T023完整Host/HTTP/受保护pipe与外部harness仍待接通，最终V008–V015和T027完整Host重启闭环仍unverified，不能把局部测试称全量验收。
