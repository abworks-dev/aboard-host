# 001 Research — 决策依据与未验证边界

**状态：proposed/designed，2026-10-18；不是 accepted ADR。**
范围与设计见[计划](<plan.md>)；当前功能授权与历史批准仅见[授权记录](<authorization.md>)。本文件只保留能改变设计或验收的研究，不复写总体路线。
`documented` 表示官方正文已读取；`unverified` 表示本项目组合尚无运行证据。以下“本轮”与未安装/未运行说明均为研究交付时的历史截面，不是当前权限限制；新增运行结果须另附证据，不改写历史。

## R1. 工具链与 SQLite 驱动

- 已核查本机 `go version`：`go1.27.0 windows/amd64`；目标继续固定 **Go 1.27.2**，不静默改为 latest、不在本轮升级。
- [Go 发布与支持政策](<https://go.dev/doc/devel/release>)记载 1.27.2 于 2026-10-08 发布，含安全修复；每个大版本支持至另有两个更新大版本。支持分支不等于旧补丁已包含修复。
- **proposed 首选 `modernc.org/sqlite v1.60.1`**：其[版本文档](<https://pkg.go.dev/modernc.org/sqlite@v1.60.1>)描述纯 Go 驱动及 Windows 平台，当前正文列嵌入 SQLite 3.53.4。避免首片再引入 C 编译器，但纯 Go 不是正确性或耐久性证据。
- 备选 [mattn/go-sqlite3](<https://github.com/mattn/go-sqlite3>) 使用 cgo/GCC；官方API已核实比较候选发布 [v1.14.52](<https://github.com/mattn/go-sqlite3/releases/tag/v1.14.52>)（2026-09-05），但尚未核实该tag的嵌入engine/构建。若modernc的Windows、事务中断或只读门槛失败，记录原因并重做同套验收，不自动fallback；本轮未下载任何驱动。
- 驱动冻结前必须核实模块 tag、go.mod、校验和、嵌入 SQLite 实际版本、许可证/漏洞、目标 Go/Windows 兼容及构建结果；当前只得到官方文档证据，未下载模块，**依赖 pin 仍是提案**。
- [Go database/sql](<https://pkg.go.dev/database/sql>)：`sql.Open` 通常不建立实际连接；不能以返回 nil 证明数据库有效。连接池上限不等于 OS 排他。驱动不支持取消时，Context 到期不证明查询已结束。

**验证门 G1**：固定 Go/driver/SQLite 组合，在 Windows 目标构建、真实落盘/关闭重开、连接初始化、事务/取消结果、错误分类全部验证；缺失时不能宣布 M1 支持。

## R2. 事务、日志模式与持久性

- [SQLite 事务](<https://sqlite.org/lang_transaction.html>)：同库同时最多一个写事务；`BEGIN IMMEDIATE` 在写者占用时可返回 BUSY。SQLite 的写事务锁不代替 Host 整个打开周期的业务 owner。
- [synchronous 官方说明](<https://sqlite.org/pragma.html#pragma_synchronous>)明确区分 rollback FULL 与 EXTRA：DELETE 日志下 FULL 不必然抵抗断电；EXTRA 额外处理删除 journal 后目录同步。WAL FULL 的文档条件更强，仍依赖正确 VFS、OS 与存储刷写。
- **proposed M1：rollback `journal_mode=DELETE`、`synchronous=EXTRA`、`foreign_keys=ON`，每区一个长期连接，由区内串行提交者使用。** 逐连接设置并回读验证；`busy_timeout=1000ms`，BUSY 返回可解释拒绝而非循环重试提交。安装库采用相同已验证配置。
- 原因：首片不承诺并发只读 open，读写串行足够；避免 WAL/SHM、checkpoint 和活 WAL 检查增加只读与收尾复杂度。不是说 WAL 不可用或 DELETE 自动安全。
- [WAL 官方页](<https://sqlite.org/wal.html>)记载 WAL-reset race 影响 3.7.0–3.51.2，修复于 3.51.3，另有 3.44.6/3.50.7 回补。不得因“应用只有一个 writer”忽略其他连接/checkpoint；即使本片不启 WAL，也必须核实嵌入引擎是已修复版本。
- 不使用 OFF/MEMORY journal，不在 open 路径静默切换旧目录日志模式。目录 v2 声明本片模式；发现 WAL/SHM 或不同持久化协议先拒绝/要求明确恢复评估。
- 文档的 EXTRA/ACID 矩阵**不是本项目 Windows VFS 已通过断电试验**。需要核实 EXTRA 在所选 Windows VFS 中具体 flush/目录行为；若只能给出条件性保证，应如实限定，不能宣称硬断电零丢失。

**G2**：记录真实 `sqlite_version()`、journal/synchronous/foreign_keys 回读与实际 VFS；验证故障注入、进程 kill、确认后重开。真实断电/磁盘缓存保证在无设备证据时仍 unverified。不可用 FULL 替换 EXTRA 后沿用原证据。

## R3. inspect 必须真正不写目标

- [SQLite URI](<https://sqlite.org/uri.html>)：`mode=ro` 不授予恢复写权限；`immutable=1` 关闭锁与变化检测，若文件变化可能读错或报损坏，不能用于活动工作区。
- [SQLite 打开接口](<https://sqlite.org/c3ref/open.html>)：`SQLITE_OPEN_EXCLUSIVE` 对 `sqlite3_open_v2` 不产生预期 OS 独占，不能据此实现写者门禁。
- [只读错误码](<https://sqlite.org/rescode.html#readonly_rollback>)区分 READONLY_ROLLBACK(776)、READONLY_RECOVERY(264)：hot journal 或 WAL 恢复可能需要写入；不得捕获后改 RW“检查”。
- modernc [Driver.Open 文档](<https://pkg.go.dev/modernc.org/sqlite@v1.60.1#Driver.Open>)提醒：普通 filename 上的 query 可被忽略，`path?mode=ro` 不等于受保证的 file URI；`_pragma` 能执行 SQL。所有 DSN 由适配器按固定允许表构造，不接收用户 DSN/SQL/pragma，不接受 ATTACH 或多语句输入。
- **proposed**：inspect不把SQLite驱动直接接到源目录。先只读`OPEN_EXISTING`打开已有writer.lock、share=0且非继承，持有至检查结束，阻止本协议写者进入；缺锁/争用则拒绝，不创建/改写锁内容。再取得禁止写入和删除共享的manifest/主库只读句柄并维持祖先/root稳定性，确认无journal/WAL/SHM及身份变化，才复制主库到Host私有、严格ACL的按操作scratch，以固定`file:` URI `mode=ro` + query_only检查副本。只读inspect锁句柄没有业务写权限，不能转为可写open。
- 源句柄或稳定性证据拿不到时返回 `INSPECTION_UNAVAILABLE`；发现残留日志返回 `RECOVERY_REQUIRED`，不删除日志、不创建锁文件、不尝试恢复。inspect 从不授予 open 或 write；活库 inspect 无成功承诺，这不削弱损坏样本零写入要求。
- scratch 是可丢弃敏感缓存而非日志/第二主库；不输出原文，不纳入备份；关闭副本后清除该操作 scratch，失败仅报告清理未完成。未来产品 scratch 与开发诊断分离；本仓验收只用 `.data/logs/` 下合成根。原目标前后递归清单/字节必须相同，抓取任何意外 sidecar 为失败。

**G3**：缺库/零字节/坏摘要/旧 v1/未来格式与 schema/伪造描述/不存在目录、hot journal、活写者、DSN 特殊字符、重解析竞态；实际文件写入观测证明目标零创建/零改字节，而非仅测函数名叫 ReadOnly。

## R4. Windows 排他与路径身份

官方证据均已读取，尚未在项目调用 API：

- [CreateFileW](<https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew>)：`dwShareMode=0` 阻止冲突的读/写/删除打开，共享选项持续至句柄关闭；DELETE sharing 涉及 rename。`OPEN_EXISTING` 不创建，`CREATE_NEW` 已存在则失败。元数据访问不都受相同限制。
- [LockFileEx](<https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex>)提供区域排他及 fail-immediate，但不阻止映射视图读写，异常退出后的 OS 解锁可能延迟。
- [进程 ID/句柄](<https://learn.microsoft.com/en-us/windows/win32/procthread/process-handles-and-identifiers>)与[进程终止](<https://learn.microsoft.com/en-us/windows/win32/procthread/terminating-a-process>)说明生命周期不同，子进程不必随父退出；进程退出/句柄消失不证明保存结果或排空。
- **proposed 选一种主门禁**：固定 writer.lock 的非继承 `GENERIC_READ|GENERIC_WRITE`、share=0、`OPEN_EXISTING` 句柄，整个可写打开周期持有；不再叠加另一套可争夺的 PID/命名 mutex/租约 owner。create 用 CREATE_NEW；缺失锁对象属不完整，不在 open 补建。持有期间不得 truncate、delete-on-close、删后重建或允许继承/复制给 worker。PID 仅诊断。
- SQLite 自身事务锁仍负责其事务，不与 Host 打开周期 owner 竞争。安装级固定 `host.lock` 防止同安装双 Host/身份与登记写者，范围不同于各区 writer.lock，不持全局临界区等待区内 IO。
- [FILE_ID_INFO](<https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info>)、[GetFileInformationByHandleEx](<https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfileinformationbyhandleex>)支持由句柄取得 volume + file ID；[BY_HANDLE_FILE_INFORMATION](<https://learn.microsoft.com/en-us/windows/win32/api/fileapi/ns-fileapi-by_handle_file_information>)警告 ID 可复用、ReFS 64位 ID 不足。物理 ID 是当前机器定位证据，不替代 workspace UUID。
- [Reparse Points](<https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-points>)与 CreateFileW 表明默认跟随链接；仅给最终分量加 OPEN_REPARSE_POINT 不能自动证明整条祖先路径安全。
- **proposed 首个支持 profile**：Windows 11 x64、Go windows/amd64、直接连接的本地 NTFS 普通目录，无重解析祖先/根/关键文件、无关键文件多硬链；拒 UNC、网络映射盘、ReFS/FAT、云同步管理目录、符号链接/junction、设备/原始卷路径。路径规范化仅作显示；授权和锁同时核对真实句柄目标/文件 ID，关键操作前后复查。不能可靠识别的云同步路径要求部署配置排除并 fail closed，不能自称自动识别所有同步软件。
- 同卷停机 rename 后显式 relink 可保留物理与逻辑身份；复制的 root/file ID 不同且 UUID 相同则拒绝。跨卷 move/clone 不在此首片工具范围。拒绝重解析是项目初期保守支持面，不是说 Windows API 无法支持其他文件系统。

**G4**：真实第二进程持续持有/同时竞争/kill 各至少10次，别名、删除/替换锁对象、异常退出解锁延迟、继承句柄、路径换目标、停机 rename/副本冲突。不能以两线程、文件存在或 PID 检查代替。

## R5. 本地认证、凭据与引导

- 回环地址只限定网络位置，不证明用户身份。[IsLocal](<https://learn.microsoft.com/en-us/dotnet/api/system.net.httplistenerrequest.islocal>)描述同机来源；[Kestrel Host filtering](<https://learn.microsoft.com/en-us/aspnet/core/fundamentals/servers/kestrel/host-filtering>)说明仅监听不自动验证 Host。这是边界旁证，不代表使用 .NET 或已验证 Go。
- [文件安全](<https://learn.microsoft.com/en-us/windows/win32/fileio/file-security-and-access-rights>)说明默认 ACL 继承且既有对象有自己 ACL；创建时传安全描述符不修复已存在对象。
- [DPAPI CryptProtectData](<https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata>)：用户作用域通常绑定登录凭据/机器，但 roaming profile 有例外；LOCAL_MACHINE 可使机器上其他用户解密，不作默认。不能把 DPAPI 宣称绝对不可跨机或防同用户恶意进程。
- **proposed**：Host 首次显式初始化用户级安装：不可预测随机材料、稳定 installation/tenant/owner principal ID；用户作用域 DPAPI 与明确且验证的当前 SID DACL。仅“确实不存在”才首建；已有但不可访问、解密失败、owner/ACL 不符、部分写入或身份不一致均拒绝，不 reset、不挑备用目录。
- 公共协议采用 loopback HTTP + 每请求 opaque bearer client session，服务器仅存其摘要/撤销状态并关联主体/权限；公开 DTO 不含安装根密钥、DPAPI blob 或原始凭据。
- **引导 transport 提案**：Host 自身的本机操作员控制面使用受当前用户 SID DACL 限制、拒远端的 named pipe，通过 impersonation 的 TokenUser 核对 SID，不以 PID/loopback 推断。操作员明确签发最小权限会话；会话经该受保护通道交付给获准本地客户端，不写命令行、URL、普通日志或 HTTP JSON。客户端不读安装秘密文件/数据库。控制面不能成为第二 DB owner；活 Host 统一处理。
- M1 不提供浏览器配对/CORS/前端。HTTP 精确校验 loopback listener authority/Host；有 Origin 的浏览器请求和 CORS preflight 默认拒绝，不用 cookie/任意 Origin 凭据访问。未来 React 的 HttpOnly 会话或配对另在 M6 设计，不能临时让浏览器读根凭据。
- Named pipe 的首次实例防抢占、server 身份核对、SID impersonation/复原、句柄最小权利、拒远端与 token 交付需要 Windows 技术门验证；此处是设计提案，不是官方文档已证明应用安全。若做不到，阻断可信连接交付，不退回“本机免认证”。
- 保护边界不包含管理员/内核或同 Windows 用户恶意代码完全隔离。具操作员 OS 身份者能够显式管理授权，不表示持有受限 HTTP 会话者可提升权限。

**G5**：首次双进程竞争、已有损坏/不安全 ACL/DPAPI 不可解密、错误 SID/远端/pipe 抢占、过期/撤销/伪造 bearer、无 B 权限、伪造请求体、Host/Origin 负例、敏感信息扫描。只使用合成身份与一次性材料。

## R6. 版本、摘要、登记和未知结果

- [SHA-256 标准库](<https://pkg.go.dev/crypto/sha256>)提供算法，不定义业务序列化或授权。JSON 字段顺序、换行、Unicode 不得隐式决定跨语言摘要。
- **proposed 三轴**：目录 formatVersion=2；新业务 schemaVersion=1；公共 API /v1。旧身份目录 v1 不扩表、不升级；比当前高、未知字段或类型不符明确拒绝。编号各自演进，不以目录2推算schema2/API2。
- 完整字节协议和兼容拒绝例在[数据模型](<data-model.md>)；结构契约在[公共协议](<contracts/api.md>)。同一仓权威，无并行手写 TS 契约；后续机器可读 OpenAPI/JSON Schema应从此设计收敛为唯一结构源，并用固定夹具约束生成绑定。
- 创建、目录描述、工作区库、安装登记不可能凭 SQLite 单事务获得跨文件/双库原子。采用安装意图→独占创建→工作区 genesis提交→描述发布→安装登记 finalize；每步关联 operationId，残留和未知显式拒绝，只有显式调查/完成登记才可继续。不是分布式事务框架。
- 工作区记录与提交 receipt 在同一库同一事务；候选及 accepted operation 在该库预先持久化。Commit 返回错误、超时或丢回执均不直接推断回滚；隔离受影响区写入，重获排他后按 operationId 查 receipt/候选与事务一致性。无 receipt 也不直接标 rejected，须证明旧事务物理结束且恢复完成。
- 客户端重复同一`(workspaceId, operationId)`只能调查已有结果，不能启动第二次业务提交；同区不同载荷/主体复用key拒绝。安装操作另以`(installationId, operationId)`为作用域，无跨区全局去重；调查不换区/换key。没有可证明结果时维持unknown，不自动重放。丢回执可证明已提交时报告唯一原receipt，不发布第二次成功。

**G6**：每个创建阶段崩溃/回执丢失、登记 finalize未知、数据库提交前/中/后失败、陈旧候选/并发确认、operationId误用/越权调查、版本拒绝和固定字节多语言夹具；旧业务迁移不在本轮。

## R7. 长期本地存储定位、单一实现与延期取舍

**designed；不是依赖或部署能力的实测结论。** 本节记录取舍依据；长期范围/触发条件由[总体范围 §3.1](<../../docs/scope.md#31-长期数据策略与单一实现范围designed未实现>)拥有，首片 ledger/兼容规则由[数据模型 §7](<data-model.md#7-首片数据实现范围与演进门designed未实现>)细化。本次不新增外网研究、安装或业务实验，R1–R6证据的原有局限不变。

- **选择**：SQLite 作为本地工作区长期默认，当前仅实现一种驱动/profile。其适用前提是本地文件系统、一个正式写入 owner、独立持久工作区，以及无须用户维护数据库服务；工作区状态经公共协议提供，而不暴露 SQL/表给客户端。完整产品 SQLite + 对象文件并不要求 M1 使用对象，首片文本仍直接入库。
- **没有选择**：当前不上 PostgreSQL、双写、动态换库、多数据库兼容或分布式事务。它们需要不同的部署/运维、owner、授权、迁移与恢复证明，尚无已批准消费者足以承担这些成本；不是认定服务型数据库永远不适用。
- **候选不等于最优证明**：R1 的 modernc 仍待 G1/G6，纯 Go 不证明更快、更可靠。SQLite 架构适配性、驱动选择、连接/profile 和实际容量是不同决策；不因选择 SQLite 就永久冻结单连接/DELETE，也不为预计未来需求立即切 WAL 或增加服务。只在对应阶段运行获得授权的合成验证，不要求本轮双驱动实验。
- **降低迁移耦合而非制造万能抽象**：业务端口表达提交版本/调查回执等用例语义；SQL/PRAGMA/连接/错误码留在适配器。允许使用合适的 SQLite 能力；未来换库仍须重验事务隔离、错误与取消语义、数据转换、约束、回执及生命周期，不承诺更换 adapter 即完成迁移。
- **版本化不等于交付升级器**：当前 D3 只初始化新库 schema=1 与各自 ledger/校验和，验证拒绝旧/未来/矛盾状态。通用升级/降级、旧数据导入与已有 schema 迁移须另定支持矩阵、准入/排空和中断恢复；不在 inspect/open 运行 migration，不靠删库或旧 binary 回退新 schema。
- **容量增长先定位负载**：正式事实、对象、候选、执行历史、临时进度与可重建索引应分开管理。先测量有消费者的查询/写入、活跃连接、资源占用和维护行为，再评估分页/索引、归档、读连接或 WAL；不存在未定义 SLA 已达标的结论，也不以数据量、Agent 数或前端种类单独决定换库。
- **生命周期与历史是迁移成本**：备份不是仅复制 DB；后续须验证 revision/对象清单/摘要、GC 保留及恢复版本。SQLite 事务不构成关闭/移动/备份 lease。恢复旧快照可能删除后来已提交操作的证据，不能自动继承同一未替换历史的去重保证或当前执行授权；未知外部副作用不自动重放。
- **重新评估的进入条件**：产品明确需要多机同区写入或服务端高可用，或本地方案在针对性查询/索引/维护调整后仍有可重复的已批准容量/延迟/资源门失败。届时另立 Spec Kit 功能，评估存储及部署模式；不靠网络共享目录、自动 fallback 或当前本地 owner 与远端并行双写解决。

对象/投影归 M2，Run/事件归 M5，快照/备份恢复工具归 M7；schema 升级、归档、专用索引及其他部署模式只记录设计约束和进入条件，不新增本片表、框架、命令或验收承诺。新增 ledger/版本负例纳入既有 G6/V7/V8；全部运行证据仍 unverified。

## 证据采集局限与后续冻结

- 本轮只完成官方资料阅读与静态文档检查；G1–G6 全部 **unverified**，不把资料上的平台列表当实测矩阵。
- Agent Reach v1.5.0：版本/doctor/check-update 已执行，check-update exit 0 表示已最新，不执行升级。Jina 匿名入口失败（40103 IP reputation/auth），经当前授权改用内置 web_fetch/官方直连读取正文；失败 URL 不作为支持证据。
- 少数大型 web_fetch 的工具层自动 spill 到系统临时目录，非本项目主动指定写入；未去修改/迁移外部文件。本项目自建诊断/证据只放 Git 忽略的 `.data/logs/`。工具临时文件不纳入交付。
- 研究交付时无remote，Go module path未确定；当前前置依据与权限仅按[授权记录](<authorization.md>)处理。研究阶段未创建go.mod/安装编译依赖；该历史事实不是当前禁止推进的授权副本。
- 发布/签名/安装升级方式留给后续有运行产物的工程决策，不妨碍本片先以明确数据根验证；已有安装/工作区数据的升级/回滚需要新授权和版本协议，禁止 open 自动修复。

## T001 / V001 实施前置核查（2026-10-18）

- 实际根与Git根均为 `E:\Repository\Github\aboard-host`；项目内Git配置无remote，`git remote -v`无输出。不能从目录名、旧仓或Git身份推定新仓所有者及module path。
- 实测Windows 11专业工作站版 `10.0.26300` / 64位，E卷为Fixed NTFS / Healthy；Git `2.52.0.windows.1`。本机Go仍为 `go1.27.0 windows/amd64`；`GOTOOLCHAIN=local`下 `go env` 返回GOOS=windows、GOARCH=amd64、CGO_ENABLED=0、GOMOD=NUL。目标1.27.2尚未准备或验证，不以本机旧补丁替代。
- 上述只读环境命令exit 0，原始输出保留在Git忽略的 `.data/logs/001-t001-environment.json`。卷信息不是锁、源零写、SID或数据库技术门通过证据。
- 初次核查时T001未完成、V001为unverified：缺module path依据，故当时未创建go.mod/业务代码或下载依赖。该历史停点已由下述用户新授权及实测解决，不改写为当时已通过。
- 用户随后明确要求创建远端（权限原文与边界仅见[授权记录](<authorization.md>)）。实际 `gh 2.97.0` 的 `gh api user` 确认账户 `abworks-dev`；目标查询先返回HTTP 404，再执行 `gh repo create abworks-dev/aboard-host --private --source=. --remote=origin --description 'Local-first standalone Go workspace host'`，exit 0。复查仓库private=true、size=0、分支数0，本地origin为 `https://github.com/abworks-dev/aboard-host.git`；未提交/push。module path据真实远端固定为 `github.com/abworks-dev/aboard-host`。证据：忽略的 `.data/logs/001-t001-remote.json`。
- 隔离环境 `GOENV=off GOTELEMETRY=off GOTOOLCHAIN=go1.27.2`，GOPATH/GOMODCACHE/GOCACHE在本项目 `.data/`，TEMP/TMP在 `.data/logs/tmp/`；`go version` 与 `go env` 均exit 0，实测 `go1.27.2 windows/amd64`、CGO_ENABLED=0。由现有Go的官方工具链下载机制准备，无全局安装/升级。项目局部go.exe SHA-256为 `b8cd508a0a7aac883cd750e9e75eb306d219e726bdf1cde29009490729437fbf`；证据 `.data/logs/001-t001-toolchain.json`。
- **T001 / V001 passed（仅工程身份与环境前置）**：真实module来源、固定工具、目标NTFS环境及合成验证授权已具备。允许沿T002→T004继续；当时G1–G6、SQLite和业务验收仍全部unverified，工具链可用不证明其通过。

## T004 / V003 存储前置实测与G2阻断（2026-10-18）

**该轮历史结论：V003 failed，G2 failed；当时T004未完成，停止T003/T005及全部后继依赖。** 后续获准最小修正与当前复验结果见本节末尾，原失败不删除或改写成通过。 下列正常路径通过不能抵销刷写失败反例。G1已有构建/版本/模块完整性证据，但不独立宣称全部G1前提通过；G3–G6和最终业务验收仍unverified。本节是当前执行结果，前文研究和设计交付时的unverified为历史截面。

### 已执行的局部验证，不是001验收

- 项目模块固定 `github.com/abworks-dev/aboard-host`，Go `1.27.2 windows/amd64`、CGO关闭；`modernc.org/sqlite v1.60.1` 使用其自身要求的 `modernc.org/libc v1.77.1`，实测engine `3.53.4`、VFS `win32`。未单独升级libc、改缓存、换驱动或改DELETE/EXTRA。
- `./tools/go.ps1 -GoArgs @('mod','verify')`、模块/目标源清单解析、存储包fmt/vet均exit 0。固定file URI及连接逐项回读测试通过；缺文件不创建、只读写拒绝、既有WAL拒绝而非转换、FK/约束错误、真实BUSY约1000ms及回滚不可见均有实际反例。
- 先前命令 `go test ./internal/storage/sqlite/... -run TestProfileGate -count=3 -v -timeout 90s` 在新增下述错误门之前exit 0：真实子进程提交/关闭；已确认提交后kill、未提交且存在非空rollback journal时kill，均Wait确认退出再重开验证。三轮日志固定各PID/时间线。这只证明相应进程故障场景，不证明硬断电、OS排他、Host/身份、物理排空或001闭环。
- 原始诊断均在Git忽略的 `.data/logs/001-validation/`：`v003-module-verify.txt`、`v003-modules.jsonl`、`v003-target-sources.jsonl`、`v003-profile-final.txt`。测试只使用该根下合成数据；无真实库、Provider或插件。

### 真实错误路径与已核实调用链

新增必需负例：[刷写错误门](<../../internal/storage/sqlite/profile_flush_windows_test.go>)。命令 `./tools/go.ps1 -GoArgs @('test','./internal/storage/sqlite/...','-run','^TestProfileGateFlushFailureContract$','-count=1','-v','-timeout','30s')` **exit 1 / failed**：对合成普通文件以只读访问打开的真实句柄，`windows.FlushFileBuffers` 返回 `ERROR_ACCESS_DENIED`（日志 `Access is denied.`）；同句柄经过 `libc.XFlushFileBuffers` 返回 **-1**，不是BOOL失败所需的0。没有改系统ACL、安全策略、设备或依赖；此为真实API负例，**不是已在SQL COMMIT内部注入刷写失败**。

固定模块源码及实际目标文件清单经两次只读核对（第二次专门审阅新错误链）：

1. `modernc.org/libc@v1.77.1/libc_windows.go:2734–2743`：`func XFlushFileBuffers(t *TLS, hFile uintptr) int32` 在 `err != nil` 时执行 `t.setErrno(err); return -1`，成功返回1。
2. `modernc.org/sqlite@v1.60.1/lib/sqlite_windows.go:130414–130424` 将该函数装入 `_aSyscall[13].FpCurrent`；Windows `TBOOL = int32`，实际编译目标包含这两个源文件，并非未使用的同名实现。
3. 同一SQLite文件 `:126577–126582`：`rc = ..._aSyscall[int32(13)]...; if rc != 0 { return SQLITE_OK }`，仅0进入 `SQLITE_IOERR_FSYNC`（1034）。`win32` 的xSync指向此 `_winSync`。故-1会被该分支当作成功，记录errno不能补救被跳过的错误路径。

这不是普通的“硬断电未验证”限定，而是固定候选会错误解释已发生的OS刷写错误，破坏G2要求的错误可观察性/确认前提；不能以EXTRA回读或正常kill通过来接受该组合。完整错误/退出码：`.data/logs/001-validation/v003-flush-failure.txt`、`v003-flush-failure-exit.json`；源/模块/测试指纹：`v003-failed-inputs.json`。直接负例保留在正常 `TestProfileGate` 名称选择范围内，不skip/删除来恢复绿色。

### 最小修正方向与停止条件

不改变SQLite本地事实owner、DELETE/EXTRA、receipt或unknown语义。先评估并固定一个能够正确传播Windows刷写失败的可审计依赖修正（上游已修复组合优先；不存在已核实版本时不编造），并重验G1/G2及新增负例；若需本地维护补丁或更换驱动，则先明确方案/批准与维护成本，不直接改module cache、单独抬高libc、引入双驱动或自动fallback。该历史截面尚未实施依赖修正；当时T004依赖分支保持停止，001未完成。

追加只读上游核对（2026-10-18，非升级/安装）：[Go代理的SQLite发布元数据](https://proxy.golang.org/modernc.org/sqlite/@latest)仍为 `v1.60.1`（commit `b122d0417c01508beb55158faedeb64a0c5bfd8a`）；[libc发布元数据](https://proxy.golang.org/modernc.org/libc/@latest)为 `v1.79.0`（commit `02bd2316433516d22b0cc2946cbd2ebc08fbd44f`）。实际读取[上游v1.79.0源码](https://gitlab.com/cznic/libc/-/raw/v1.79.0/libc_windows.go)，其 `XFlushFileBuffers` 仍在出错时 `return -1`，所以不能声称“升最新即可修复”，也未单独升级libc。不代表排查了所有分支/未发布提交；没有已核实可直接采用的官方修复组合。工具读取大型上游正文时自动spill至系统临时目录，非项目主动文件操作，未改删该外部文件；本项目证据仍仅指定忽略根。

### 获准最小兼容修正与当前复验（2026-10-18）

用户批准范围仅见[授权记录](<authorization.md>)。当前维护[固定替换输入](<../../tools/compat/libc-windows-flush.json>)：原始 `libc_windows.go` SHA-256 `cb863451675c6e3664c8eb403ad9e775eecb18c45c6419d523f24e8b1c5d3164`，唯一代码差异是 `XFlushFileBuffers` 的失败 `return -1` → `return 0`，保留 `t.setErrno(err)` 与成功值1；固定SQLite/libc版本不变。这是本项目兼容修正，不是官方已发布修复，也不更换driver、DELETE/EXTRA或正式保存语义。[许可证与重建/移除说明](<../../tools/compat/README.md>)记录维护成本、未来正式匹配组合修复后的退出条件。

初次工具尝试未到达测试，不能算产品成功：Go1.27拒绝GOMODCACHE overlay（`Files beneath GOMODCACHE ... must not be replaced.`，日志 `v003-patched-flush-initial.txt`）；随后私有副本生成遇到PowerShell单参Write重载、空byte[]被pipeline展开为null，修正后又遇到测试的 `unsafe.Sizeof(view)` 需转 `libc.Tsize_t` 的编译错误，均保留诊断。只清理经绝对路径核对的自身0字节临时产物；原模块缓存从未修改。

当前[runner](<../../tools/go.ps1>)采用[准备脚本](<../../tools/compat/prepare-module.ps1>)：固定版本与源SHA/唯一匹配，产生忽略目录中的可再生源码副本，逐文件核对仅一处不同，临时modfile以 `-mod=readonly` 指向它。副本4199文件/约86MB，仅本地构建缓存，不入Git、不维护第三方整库fork。单次runner持续持有独占构建输入句柄，半成品/多余文件/不符指纹/竞争明确失败；不靠覆盖未知缓存恢复，不宣称并行runner。Go模块原缓存 `go mod verify` 仍exit0，另存当前源/补丁/工具/模块图指纹。两种横线的覆盖参数均实际拒绝。

当前执行（全部真实本机Windows/amd64、NTFS、Go1.27.2、CGO=0；日志均在忽略的 `.data/logs/001-validation/`）：

- 原必需 `TestProfileGateFlushFailureContract` 保持相同断言，**passed**：真实只读合成句柄的原生错误仍为 `Access is denied.`，修正bridge现在BOOL=0，不再误入成功分支。
- 新[COMMIT故障测试](<../../internal/storage/sqlite/profile_commit_flush_windows_test.go>)经真实 `database/sql → driver → SQLite pager → win32 VFS`，只在测试binary使用VFS的xSetSystemCall。先测正常提交实际3次刷写，再分别在第1/2/3次将调用导向真实只读合成句柄，均得到 **`SQLITE_IOERR_FSYNC=1034`**，未返回成功。正常fresh视图业务/receipt=1/1；三个错误样本在物理连接关闭/恢复默认syscall、重新打开后均为0/0；这不是从error直接推断回滚，也不允许自动重试。固定driver `tx.go:35–53` 在COMMIT错误且仍在事务时可能内部尝试ROLLBACK，因此只称应用无重放，不能称驱动从不rollback或任意错误必然无提交。
- `./tools/go.ps1 -GoArgs @('test','./internal/storage/sqlite/...','-run','TestProfileGate','-count=3','-v','-timeout','90s')` **exit0**：上述故障门和原正常提交/关闭重开、URI/只读/缺文件/FK/WAL拒绝/BUSY及真实第二进程提交、已确认后kill、非空journal未提交kill三轮全过。kill均Wait确认终止再开库；不把裸 `exit status 1` 强杀标成业务失败。`vet`存储/负向fixture、`mod verify`均exit0。最终日志 `v003-patched-profile-final.txt`、`v003-patched-vet.txt`、`v003-patched-module-verify.txt`。
- [遗漏修正负向控制](<../../tests/fixtures/check_unpatched_guard/main.go>)在同一固定环境**故意不用临时modfile**执行 `go run ./tests/fixtures/check_unpatched_guard`，exit0证明[运行时检查](<../../internal/storage/sqlite/flush_guard_windows_amd64.go>)拒绝原始组合，且目标DB不存在；不是绕过门运行服务。日志 `v003-unpatched-guard-control.txt`。当前模块/源/工具指纹与命令状态为 `v003-patched-inputs.json`，模块实际替换图为 `v003-patched-modules.jsonl`。

**当前结果：V003 passed，T004完成；G1/G2在上述固定组合、进程故障和错误可观察性范围内通过，可继续T003/T005。** 不复用旧未修正组合的pass，不把原失败改为假阳性。G3–G6与最终Host验收仍unverified；Windows文件系统/设备刷写与硬断电耐久仍无运行证据，EXTRA回读/进程kill/受控IOERR不能替代硬件测试，不能承诺未经证明的掉电原子性。OS跨进程写者排他、认证、inspect源零写、保存未知/排空和正式001闭环仍须各自门；T009前不进入服务集成，001尚未交付。

### T003 — V002b 单源绑定与独立字节复验（2026-10-18）

当前 **T003完成、V002b passed**，只覆盖结构/字节前置。[生成器](<../../tests/fixtures/generate_protocol.py>)从唯一[OpenAPI](<../../contracts/openapi.json>)生成Go DTO/枚举/schema及16操作参数/请求绑定，`--check`检测漂移；[严格边界](<../../internal/protocol/strict.go>)在标准JSON解码前拒绝坏UTF-8/BOM/NUL/孤立surrogate/解码后重复key/尾随值，并校验精确字段、条件、required、字节/scalar上限和无float64截断的整数。只是当前schema子集的验证器，不是完整通用JSON-Schema引擎；未知生成keyword拒绝。

[摘要实现](<../../internal/digest/digest.go>)按模型§4固定域、UUID展示序原字节、U64BE、原UTF-8长度/字节、原32字节摘要与absence标志；不是hash JSON展示序，也不做换行/Unicode规范化。[fixture](<../../testdata/contracts/digest-vectors.json>)由Python stdlib独立冻结expected，并拒绝覆盖已有expected；SHA-256 `27764efb69c709e7b1e716cae1f6d0397e0f65e44e111a0141f1c5209ccfd2d2`。[独立核对器](<../../tests/fixtures/verify_digest_vectors.py>)当前exit0：16正例、42拒绝例、88字段变异；包含2^53+1、int64上界/溢出、UUID字节序、中文/emoji/组合字符、LF/CRLF、absence、非法编码。摘要/DTO从不授予当前权限，不证明同key业务至多一次。

父审发现子agent将已有 `time.Parse(time.RFC3339)` 错报为未做format断言；撤回该报告。原实现确有额外拒绝，且没有日期专项运行反例，不能伪造red历史。已删除该分支，恢复冻结OpenAPI3.1/2020-12的format annotation语义，不擅加契约限制。新增CandidateView.expiresAt的5类字符串结构接受（含小写t/z、闰秒、非日期、空串）及6类错误类型/缺字段拒绝，Go DTO往返/独立Python均覆盖；实际时间和过期检查仍由后续Host业务执行，不能把任意字符串结构接受宣称候选业务有效。

当前父执行 `python -B tests/fixtures/verify_openapi.py`、`generate_protocol.py --check`、`verify_digest_vectors.py`、runner `vet ./...` 及 `test ./... -count=3 -timeout 150s` 均exit0；后者仅当前Phase A包，无未来Host测试。新的日期修正已在本轮Go复验，不沿用旧pass。日志（全部忽略根）：`verify_openapi-current.txt`、`verify_digest_vectors-current.txt`、`v002b-drift-current.txt`、`v002b-v004-vet-current.txt`、`v002b-v004-all-current.txt`（其exit.json记录范围）。不是认证/存储提交/物理路径或完整001验收；不改变US/FR/SC与数据策略。

### T005 — V004 Windows锁与共同稳定路径原语（2026-10-18）

当前 **T005完成、V004 passed（G4原语范围）**，不是最终V012或Host双写验收。[适配器](<../../internal/platform/windows/directory_windows.go>)只支持已核实本地NTFS/Windows amd64，安装host.lock与区writer.lock分离，固定现有锁或显式CREATE_NEW、share=0、非继承；祖先/关键文件持句柄、volume+file ID与规范路径复核，拒UNC/device/ADS、别名、reparse与关键文件硬链。PID不代替排他，物理身份不授逻辑身份或当前权限。

初版真实负例曾发现：空祖先可被仅FILE_WRITE_ATTRIBUTES句柄原位设为junction，即使持续目录句柄仍在；只加强sharing或反复事后Validate不足以防创建穿越。保留原失败 `v004-empty-ancestor-initial.txt`；修正为逐组件相对持有父句柄的NtCreateFile，限定单一白名单子名、OBJ_DONT_REPARSE、OPEN_EXISTING/CREATE_NEW，保留子项防父目录变空。不是任意NT路径接口或OS沙箱；不修改系统策略、不依赖管理员权限。[必需测试](<../../internal/platform/windows/relative_probe_windows_test.go>)直接走生产使用的native对象属性：元数据抢先reparse后必须拒绝且目标树不变，保留子项时非空reparse必须失败；不再只从包装器事前Validate推论瞬间安全。

实际runner `fmt`、`vet ./internal/platform/windows/...` 及 `test ./internal/platform/windows/... -run TestDirectoryGate -count=3 -v -timeout 150s` exit0，日志 `v004-required-final.txt`、`v004-required-final-exit.json`（项目忽略的001-validation根）。三轮各含持续持有/同时竞争/强杀退出各10轮真实OS子进程及真实SQLite写入，失败竞争者不确认写；继承、锁删换、真实8.3别名、junction/硬链、关键文件冻结、共同稳定、同卷停机rename/副本均有断言。每轮另40次元数据/native-create竞争：实际均reparse先赢而create拒绝、目标树不变；不谎称随机覆盖了两种排序，另一排序由保留子项的确定性测试覆盖。原失败未删除、不删skip绕门。

G4仅证明上述适配器/当前合成故障范围；逻辑workspaceId冲突与授权、目录创建协议、SQLite物理排空/unknown、inspect源写操作观察、完整Host保存退出新进程重开均尚待对应任务。G3/G5/G6未过前不进入T009后的服务集成。

### T007 — V006 安装引导与本地连接安全原语（2026-10-18）

当前 **T007完成、V006 passed（G5最小原语组合）**，不等于完整Host/安装数据库当前授权或SC-004业务验收。[材料](<../../internal/installation/security/material_windows.go>)只新建或严格已有加载，安装lease后重查私有ACL、DPAPI user scope、稳定身份；部分/损坏/不可解密/宽ACL不重建。[pipe](<../../internal/installation/security/pipe_windows.go>) first-instance/单实例/reject-remote及私有ACL，真实client impersonation核SID，可信launch PID加真实server TokenUser核验，token须在复原后签发；复原/后置条件失败不解锁专用OS线程。异步取消先排空，交付缺ACK撤销。[短会话](<../../internal/installation/security/session_windows.go>)只保留hash、当前版本/过期/撤销/精确区与action检查，共享授权unknown清空且不复活旧cache。纯HTTP函数拒Host/Origin/OPTIONS/Cookie/重复认证等，不是已有HTTP服务。[私有目录](<../../internal/platform/windows/private_windows.go>)只创建/核验owner+SYSTEM protected DACL，不事后修复未知目录。

实际runner：`test ./internal/installation/security/... ./internal/platform/windows -run TestBootstrapGate|TestPrivateDirectoryGate -count=3 -v -timeout=90s`、两包`vet`及完整`test -count=1 -timeout=120s` exit0。对应忽略日志：`v006-native-repeat.log`、`v006-vet.log`、`v006-full-regression.log`；原始夹具失败及修正留在`v006-native-first.log`/`v006-native-repair.log`与详细报告。初两次fmt参数组合错误未改源；坏DACL夹具被Windows保护化、inherit-only夹具无法常规清理，均只修测试预置/清理，不放宽产品验证。仅两个已核对绝对路径的遗留空合成目录以owner已有DACL权限恢复后删除；失败Set-Acl缺SeSecurityPrivilege未提权/改策略/创建账户。

证据含真实首建双进程恰好一胜及稳定重开，真实anonymous token不能DPAPI解密/开pipe、匿名QoS不给session、原生TokenUser对错expected SID拒绝；真实第二进程抢占、错server PID、一次ACK会话交付、未ACK撤销与物理取消排空。远端不是skip：既有`\\127.0.0.1\pipe` SMB redirector同DACL permissive正控成功，随后硬化pipe拒绝，连续三轮及回归通过；未启动SMB或造新账户。仅父进程中的subprocess helper入口skip，其三种mode已真实子进程执行。不是非匿名第二账户或第二物理机器证明。

13份日志credential-shape扫描exit0/零发现，不是通用秘密证明。六源指纹`v006-source-fingerprints.json`由父重新核对一致，独立只读审阅核实关键源码与实际日志无阻断；只读审阅不是额外运行pass。RevertToSelf本身强制失败/线程销毁forensics未实测，诚实保留为代码策略；可信PID/grant来自未来控制面接线、安装DB当前授权与事务前重验仍属后续集成，内存session原型不是第二权威库。此截面G3/G6仍未过，T009前不进入服务集成。

### T006 — V005 冻结副本与有界源零写观测（2026-10-18）

当前 **T006完成、V005 passed（G3有限原语范围）**，不是完整Host或最终V009。[inspect](<../../internal/workspace/inspect/inspect_windows.go>)先持有既有只读workspace锁、冻结描述/DB及祖先，共同稳定后拒journal/WAL/SHM等，再从现有只读句柄拷贝到私有scratch；SQLite只打开副本`mode=ro`，新增`query_only=ON`逐连接回读，不作源恢复/补种/迁移/插件/Provider。[scratch](<../../internal/platform/windows/scratch_windows.go>)相对native固定子名创建、随机48hex目录、owner/SYSTEM最小ACL；零字节/缺源/不安全形态明确拒绝。描述/业务身份/ledger的最终判断仍由后续T015/T016组合负责，单个probe库不称业务工作区。

证据不只字节相同：[PID计数](<../../internal/workspace/inspect/io_counters_windows_test.go>)在Inspect紧前后（含全部defer、无测试输出/并行写worker）采样真实GetProcessIoCounters。每次成功非空≤64KiB的scratch Write独立记录次数下界及字节，固定Go poll.Write确实至少调用一次native WriteFile，短写/重试只能增加；OS总增量严格等于独立下界，不能再容纳可计数的额外源data-write，不扣噪声、不从总数反推预期。同长度无truncate原位1/65535/65536字节正控制均为1次/n字节；真实只读WriteFile AccessDenied是0/0，明确不声称检测到失败尝试。[源NTFS观察](<../../internal/workspace/inspect/inspect_windows_test.go>)保持文件名/目录名/大小/写时间/属性/security全部mask，正负同样2000ms；[五类元数据正控](<../../internal/workspace/inspect/metadata_controls_windows_test.go>)实际create/delete/rename/attrs/security通知均可见，而其PID Write计数均0/0，证明不能用data计数替代元数据观察。源前后递归清单和原始字节另比对。

保留真实失败：新metadata测试首编译`cannot use win.DACL_SECURITY_INFORMATION | flag (value of type uint32) as "golang.org/x/sys/windows".SECURITY_INFORMATION value in argument to win.SetNamedSecurityInfo`后仅修typed flag；CopiedACL随后仍真实失败（20/26字节通知），没有按计数通过忽略。解码实际action=3、name为`.aboard`/`data`；[独立清单诊断](<../../internal/workspace/inspect/inventory_diagnostic_windows_test.go>)完全不调用Inspect，仅清单准备置于watch内即三轮复现相同通知、PID data-write=0/0。故将该新夹具纠正为任务要求及其他测试一致的“样本→字节清单→观察→Inspect”，没有把Inspect移出窗口、过滤事件或改mask。只证明准备/清单路径足以复现，未断言NTFS内部具体读相关metadata机制。原始失败/解码/隔离诊断均保留为`v005-metadata-acl-initial.txt`、`v005-metadata-acl-recheck.txt`、`v005-notification-diagnostic.txt`、`v005-inventory-isolation-diagnostic.txt`及各exit记录。

独立审阅还发现清理时过早关全部祖先后按路径删除的身份窗口。已改PrepareCleanup仅释scratch root，私有parent/祖先pins留到删除完成；确切窗口同token实际rename必须共享/访问拒绝，全pins释放后相同rename成功并移回，排除无效刺激。[实际副本ACL测试](<../../internal/workspace/inspect/scratch_acl_windows_test.go>)核两个文件owner+SYSTEM且真实anonymous token读拒绝，线程复原后置验证。真实分享冲突使清理失败时，IntegrityOK和ScratchCleanupFailed分报，保留私有残留且不声称已删除；测试核对已知绝对路径再显式清除，不动源或修用户数据。

实际runner `fmt`、`vet ./internal/workspace/inspect/...`，`test ./internal/workspace/inspect/... -run TestInspectionGate|TestInspectionInventoryNotificationDiagnostic -count=3 -v -timeout 150s` exit0，94.034s，PID185552；日志`v005-required-final.txt`/`v005-required-final-exit.json`，相关当前源/runner/固定组合指纹`v005-current-inputs.json`，均在忽略的001-validation根。三轮各15个真实Inspect观察窗口；正常/清理故障/ACL为2写/8471字节scratch、坏库2/303、零库1/279、其余拒绝0/0，精确守恒且源通知零/清单不变；每轮11拒绝样本与冻结文件/目录、清理正反例全通过。独立只读复核实读最新日志无剩余具体阻断，不把复核算额外运行。

范围：这是固定Windows11本地NTFS/Go1.27.2/当前driver修正组合、当前合成样本及实现路径的零额外已完成可计数源写/有界metadata观察；不是逐path完整内核trace、失败无效果调用、任意映射/系统后台IO、硬断电或恶意同SID代码沙箱。尚需最终V009全坏目录/业务identity/ledger样本、HTTP授权接线及T008/V007等，T009全部门齐前仍不进入服务集成。

### T008 — V007 新库ledger与持久提交前置（2026-10-18）

当前 **T008完成、V007 passed（G6存储前置部分）**。[factory/ledger](<../../internal/storage/sqlite/migration.go>)仅CreateNew初始化两类[安装SQL](<../../internal/storage/sqlite/migrations/installation_001.sql>)/[区SQL](<../../internal/storage/sqlite/migrations/workspace_001.sql>)，DDL+ledger原字节SHA+metadata+user_version+identity+Head0/owner在单库同事务；既有只查询验证实际DDL、有序ledger、FK/quick_check、连续正式历史及内容摘要，不补种/迁移。安装SQL与区业务事实分离。校验和输入增加仓库局部[LF规则](<../../.gitattributes>)，防Windows autocrlf改变嵌入SQL及唯一契约/夹具字节；`git check-attr`已证text/lf，不改全局Git或实际SQL字节。

[CommitGate](<../../internal/storage/sqlite/commit_gate.go>)先独立确认持久admission，仅全新确认者得到single-use内存ticket；业务版本/指针/Head/receipt/operation/candidate/审计同事务，Commit=nil才Confirmed。原key当前主体/input调查先于epoch/expiry/applied/base；重开accepted/unknown围栏，不复活ticket。Fresh调查必须本gate EndPhysical已成功、调用方保留真实排他并证明同一未替换物理历史；缺admission仍Unknown，有原accepted+一致fresh历史无receipt才NotCommitted，均不自动重放/解围栏。该callback的真实目录/OS锁授权由后续用例负责，本次合成same-file proof不能代V014。

[真实测试](<../../internal/storage/sqlite/commit_gate_test.go>)覆盖create/update、丢弃真实成功响应后旧epoch/过期/applied原key调查、异主体/input拒用、缺receipt旧视图、重开围栏；admission、create、update、两类init分别测正常3次flush及逐ordinal1/2/3原生只读句柄FlushFileBuffers失败，实际SQLite返回SQLITE_IOERR_FSYNC=1034、unknown/fence且fresh无撕裂，不重放。[ledger测试](<../../internal/storage/sqlite/migration_test.go>)两库各有缺行/乱序/旧/未来/额外/checksum/state/metadata/user_version/DDL/身份反例，拒前后字节不变；两库各profile/schema/ledger/metadata/user_version/identity/genesis共14中间阶段是真实子进程ready→kill→Wait，残留明确拒绝而非修复。

独立只读审阅发现两项P2，签门前最小修复并定点关闭：(1) 初始化pre-COMMIT rollback失败原来未带ErrUnknown，现明确加入；此分支仅静态确认，未另造独立rollback-failure动态刺激，不把init COMMIT错误测试说成该分支。(2) 已确认admission的业务明确rollback后原先直接Rejected、DB仍accepted，重开会误围栏。现仅原single-use admission在Rollback=nil后另短事务以同key/principal/candidate/input、accepted且无receipt条件持久rejected+稳定failureCode，RowsAffected须1、记录Commit=nil才Rejected；该写入/提交不明仍Unknown+fence。新增双admission同head冲突→重启仍可查原拒绝且可新确认提交；真实SQL触发器在已插version后拒绝，验证回滚与WRITE_FAILED持久拒绝；rejection记录3处真实flush失败均unknown，fresh调查无重放。正面的durable rejection才是证据，绝不只据缺receipt推拒绝。

取消测试先证明100ms deadline已到而真实SQLite busy约1.01s才结束；再新增native COMMIT flush callback的有界channel悬置，ctx到期且原driver未返回时并发EndPhysical，在100ms观察内不得结束、物理OpenConnections仍1，放行后两者结束且连接0再fresh。三轮实际original=confirmed(nil)/fresh=confirmed，证明取消不直接意味着回滚；这是适配器mutex/连接层证据，不是OS锁、Host队列或完整V014。沿用固定driver可能内部ROLLBACK的事实，没有声称驱动绝不rollback或任意错误可安全重试。

实际runner窄回归、全`test ./internal/storage/... -count=3 -v`（19.508s，包含Migration与全部原profile/flush断言）及vet均exit0；六owned源SHA在`t008-review-owned-sha256.log`，全测试/专项/无诊断vet分别为`t008-review-storage-count3.log`、`t008-review-focused.log`、`t008-review-storage-vet.log`，此前中间截面日志未覆盖。均位于忽略的001-validation根；最终T009再组合复验并记录独立exit码/当前指纹。完整G6的跨文件/两库创建协调、Host当前授权和业务回执/重启、OS锁/队列/物理收尾分别仍归V008/V010/V014；无硬断电/真实数据/Provider/插件或完整Host验收结论。

### T009 — 前置组合签收（2026-10-18）

**历史首次T009签收：当时V001–V007全部在前置范围passed，允许进入Phase B/C；冻结后源码已变，本句不能充当当前pass，当前状态见下节复验记录。** 唯一profile冻结为：真实module `github.com/abworks-dev/aboard-host`；项目局部Go1.27.2、windows/amd64、CGO0、Windows11本地NTFS；modernc.org/sqlite1.60.1（实际SQLite3.53.4）及其libc1.77.1、仅获准的BOOL单行构建副本修正；一库一条串行连接、win32 VFS、DELETE/EXTRA/FK/busy1000、固定file URI不接用户DSN。构建仅经项目runner/固定临时modfile，不修改原模块缓存。路径沿持有父handle的native相对打开、只CREATE_NEW/OPEN_EXISTING、既有锁share0且非继承；inspect仅冻结副本mode=ro/query_only、私有ACL scratch。安全固定为owner/SYSTEM ACL、DPAPI用户scope、受保护first-instance/reject-remote pipe与SID/可信server PID校验、短期hash session；真实安装库当前授权仍需后续接线。

实际命令顺序与独立退出码均保留在忽略目录的`t009-command-exits.json`：独立OpenAPI检查、摘要夹具、生成绑定漂移检查各exit0；`mod verify`、`vet ./...`、`test ./... -count=3 -v -timeout 180s`各exit0。六包实际通过：digest0.310s、security1.074s、Windows10.669s、protocol0.609s、SQLite19.995s、inspect93.687s。仅bootstrap子进程helper在父测试入口按设计skip（真实子mode已执行）；遗漏补丁负向fixture无test函数不是验收pass，另有T004真实控制。当前共享源码/唯一契约/独立夹具/迁移SQL/runner/补丁/授权指纹及环境截面为`t009-frozen-inputs.json`；完整组合日志为`t009-all-three-rounds.txt`，原始失败和各专项证据不覆盖。

本签收不升级任何业务V008–V015，不把GetProcessIoCounters称逐path/失败调用trace，不把取消当物理结束、不把缺receipt当回滚，不宣称硬断电、恶意同SID代码隔离、真实用户数据或Provider/插件已验证。后续每次实现改动须重验受影响门；安装登记与目录创建、Host当前授权、完整重启及端到端排空仍由既有后继任务承担。

### T009 — 冻结后变更与重新验收（2026-10-18）

用户接受任务评审意见后恢复既有范围。旧`t009-frozen-inputs.json`/全部旧日志保留，不覆盖旧pass；T009重新打开，受影响V003/V006/V007在新证据齐前为unverified。修订任务T013/T014明确既有安装操作查询、原意图调查和显式reconcile/unknown归业务层，未新增协议或能力；历史首批与当前下一批分开，新的先后为重冻T009→T010→T012→T011→后继依赖。

两份冻结后源码与影响：
- [存储profile](<../../internal/storage/sqlite/profile.go>)：旧SHA `17703fab278a359adab4ab48dbee79a7e10fb0d2783008d3cb93763ec2ba919b`。新增物理关闭未知marker及跨Open失败清理传播；Close串行/幂等保留首次失败，不因database/sql丢弃连接后第二次nil而误报drain。影响factory失败、EndPhysical、安装owner保锁及inspect副本关闭，须重验V003/V007并全组合。未改变SQL、driver版本、BOOL补丁、journal或保存确认/unknown语义。[新增测试](<../../internal/storage/sqlite/profile_close_test.go>)区分真实SQLite重复关闭/重开与合成database/sql Close错误控制；后者证明错误粘滞，不是实际SQLite Close IO错误或所有工厂失败分支的端到端刺激。
- [安装材料](<../../internal/installation/security/material_windows.go>)：旧SHA `b9eb3abefe12f6fdae989f9cdf0719946fcf14044df723922e25e258de5612db`。新增安装/tenant/principal canonical UUIDv4及带实际host.lock/受跟踪pin的创建加载入口，保留DPAPI用户scope和owner/SYSTEM ACL；材料只绑定身份，不成为第二授权库。影响V006及新安装事实后续接线。[新增负例](<../../internal/installation/security/material_windows_test.go>)分别改三身份的格式/缺失、解码后未知字段，跨安装pin、错workspace scope、nil/closed/root，不修坏材料。独立只读审阅发现部分JSON解码失败时密钥副本清理注册过晚，已将闭包defer前置到解码之前；运行异常路径，但不把Go堆内存零化称动态观测或跨用户泄漏证明。

相关新增lease/pin适配器与草稿均纳入本次全仓编译/回归及源指纹；存在/可编译不等于T010/T012完成。旧冻结只有源SHA、没有原文diff，不重建伪diff。新源、唯一OpenAPI、独立摘要夹具、SQL原字节、Go依赖/runner/单行兼容manifest/实际modfile图的精确SHA见忽略目录`t009-revalidation-20261018-run-inputs.json`，实际命令与独立退出码另存同前缀`focused-exits.json`及`combined-exits.json`。

本次focused存储全包三轮、安全/ACL门三轮均exit0；随后2026-10-18 17:36:54–17:38:49+08完成独立单源/摘要/绑定漂移、mod verify、vet ./...、test ./... -count=3 -v -timeout180s及实际module图，七步全部exit0。真实native COMMIT逐flush1034、正常/强杀重开、ledger拒绝/中断、取消时实际收尾继续保留；组合含安装事实5.739s与inspect96.121s等真实测试。所有可执行源码/依赖/夹具/构建输入与运行快照一致；README/plan/research三份进度文档在运行中更新，逐项漂移另记，最初全文件核对据此拒绝，未当源码不变。无输出的vet/fmt可能未创建Tee日志，独立exit记录保留，不以空日志推断成功。

新冻结`t009-revalidation-20261018-refrozen-inputs.json`绑定run-inputs原SHA、focused/combined实际命令/退出码及存在日志的SHA，保留旧冻结与所有失败。**T009现重新passed，仅解除Phase B/C前置停止，继续T010→T012→T011；V008–V015仍unverified。** 运行通过不等于所有已编译草稿任务完成，更不是Host业务/硬断电验收。

### T010 / T012 — 安装事实与最小区模型局部接线（2026-10-18）

按用户指定顺序先完成T010，再完成T012，随后进入T011。安装[Store](<../../internal/installation/store.go>)持真实host.lock、固定材料/数据库pins，独立安装schema/ledger与DPAPI身份/SID/reference相互匹配；既有安装拒绝缺失、坏摘要/身份/secret/ACL/硬链与material-only，不补种。typed安装查询不开放SQL或将材料变成授权owner。独立只读审阅未见具体阻断；父补核已有`foreign_key_check`及owner-principal/tenant同读事务关联，并增加删除owner/错owner tenant/错owner三种真实封闭合成库破坏负例。未知关闭保锁测试人工设置状态，仅证明Store遇该状态拒放锁，不冒称实际SQLite factory Close故障全链已动态覆盖。

最小[区模型](<../../internal/workspace/model.go>)只含稳定scope、正式文本事实与typed读端口，不import SQLite/Windows/HTTP；[适配器](<../../internal/storage/sqlite/workspace_facts.go>)复用原CommitGate/同一连接/互斥，不开第二写者。真实SQLite证明预览不成正式资源，确认后Head/正文/摘要/创建者一致，物理关闭/新连接重开一致；两库同resource key不串区，A关闭后B仍可读，unknown普通视图拒绝。当前是内部存储接线，不替代未来每次当前授权或完整Host生命周期/重启验收；第二次有界独立只读复核确认同Gate连接/锁、复合scope与正式查询分离，未见具体阻断，未自行运行测试或扩查T011半成品。

实际`test`三轮和相关`vet`均exit0：安装3.510s、区typed事实0.490s、纯模型0.171s。初局部测试均过但vet报告三处`workspace.Scope struct literal uses unkeyed fields`，已改具名字段后重新执行，原失败及退出记录保留。命令/退出码、源码/依赖/SQL/契约/夹具/runner与日志SHA见忽略目录`t010-t012-fixed-command-exits.json`、`t010-t012-current-inputs.json`，真实日志`t010-t012-fixed-tests.txt`；T010/T012局部实现勾选，V008–V015仍unverified。

T011接线过程中，安装Store新增由同一互斥保护的`authUncertain`字段，仅区分安装授权持久结果未知与物理关闭状态；不复用`openFailure`假称IO未结束，不自动解除授权围栏。此后源码截面须重新执行受影响安装/安全回归，由T011实际证据签收，不能借用以上旧源局部pass；当前T011仍未勾。

T011首轮相关包三轮与vet实际通过（最终旧截面`regression-04`/`vet-02`），但独立只读审阅发现待修P2：pipe等待ACK时可并发Close，使随后失败ACK的撤销失去数据库；同类进程在Issue提交后、ACK前退出也可能使未确认token在重开后仍有效。此前pass按当时源码保留，不作为当前签门。范围内修正沿既有session表实现pending默认不可用、真实ACK后的当前授权活化，以及Store受互斥保护的pending交付计数/Close拒绝早释锁；不持互斥等待pipe、不添加新表或HTTP能力。已先取得旧实现真实红证据`t011-delivery-red-01` exit1：`Close released DB/lease during unacknowledged delivery <nil>`与`unacknowledged token survived cold reopen after process kill`；后者实际读取pipe payload但不ACK，Process.Kill+Wait后fresh Store.Open，并非模拟退出，日志不含token。当前修正及绿回归仍在执行，未以更新本摘要代替运行证据；激活后的末段3/4交付仍可能unknown/撤销，不宣称exactly-once或任何错误必然回滚。

### T011 当前集成证据（待父最终组合签收）

- 实现入口为[安装授权](<../../internal/installation/auth.go>)和[typed会话事务](<../../internal/storage/sqlite/installation_auth.go>)：当前DB主体/会话/版本/每区动作/registered定位逐次复验，`Authorization.CheckAction`要求可信owner scope及真实动作常量。Grant是当前主体minimum选择，不是token最大scope衰减；受限身份必须使用独立受限DB principal。每业务事务前仍须重验，不声称跨安装/工作区DB原子授权。
- DB只存随机opaque token的SHA256，不存raw token。Create默认`revoked=1`，私有issuer保留精确session/hash/主体/原expiry/版本/最低grant；收到真实ACK2后重新查当前owner/subject/grant/登记，以单事务精确且唯一pending行活化。所有终止revoke同时置过去expiry，阻止迟ACK复活。控制协议固定`1→payload→2→activate→3→4`，客户端看到3并发送4才返回token；Store pending生命周期包含失败撤销，Close在未完成时拒绝释放，整个pipe IO不持安装互斥。
- 子执行最终局部`test ./internal/installation/... ./internal/storage/sqlite/... ./internal/workspace -count=3 -v`于2026-10-18 18:28:37–18:29:13+08 exit0；同包vet于18:29:36–18:29:43 exit0。父已读取独立exit文件和最终hash清单，未将空vet stdout当证据。路径前缀`.data/logs/001-validation/t011-delivery-`保留regression-02、vet-02及hashes-final。初始ACK及最终ACK窗口Close、真实child kill+Wait后冷重开、末ACK错字节/取消、迟ACK/撤销/版本/expiry/grant变化及原G5均实际执行。新增Activate的native flush baseline实测active1/pending0，各ordinal1/2/3实测unknown+1034且fresh active0/pending1；只是本次观察，不推导错误必回滚。
- 独立只读定点审阅关闭原P2，未替代执行。真实native故障在SQL包、共享Store围栏在安装映射+真实DB依赖测试，仍是分层证据，不伪称单次native到活pipe的端到端注入。HTTP仅函数守卫，尚无listener/V013业务通过。
- 父进一步使[原生路径固定](<../../internal/platform/windows/directory_windows.go>)及[安装固定文件](<../../internal/platform/windows/installation_windows.go>)与exact root策略一致：相对NT打开不请求CASE_INSENSITIVE，normalized final DOS名和lease成员仅允许驱动字母大小写、目录/文件名称逐字节相同。更保守地拒绝合法别名，不把名称当身份；物理身份仍volume+FileID。先前`native-spelling-red`为夹具缺`.aboard`父导致失败，不充安全证据；修正夹具后的`red-02`实际失败`unproven case alias accepted by native pins`，修复后平台/安装/安全focused实际exit0。新测试见[精确拼写回归](<../../internal/platform/windows/directory_spelling_windows_test.go>)；未实测开启case-sensitive目录或穷尽Unicode/upcase/映射，不称OS沙箱。
- 父最终组合2026-10-18 18:36:15–18:38:12+08七步全部exit0：单源/字节/漂移/modverify/vet、`test ./... -count=3 -v -timeout180s`及真实module图。源码/SQL/契约/夹具/构建输入与开跑快照逐项一致，命令与输入前缀`t011-final-combined`；另`t011-final-checker-exits.json`将三Python检查器实际源码SHA及重复运行exit0绑定。全仓安装13.618s、安全4.149s、平台13.751s、SQLite30.060s、区模型0.721s、inspect95.592s；helper父入口的预期skip不是required反例跳过。平台40次竞争其中一轮真实created1/reparsed39，其余created0/reparsed40，确定性反排序测试仍保留。现才勾T011并继续T013，V008–V015仍未通过。原失败、runtime丢失job的UNKNOWN/partial、旧截面pass和新结果全部保留。

### T013 接手局部取消边界验证（2026-10-18；partial）

- 新增 [取消边界测试](<../../internal/installation/workspace_creation_windows_test.go>)：在 intentRecorded、directoryPrepared、genesisConfirmed、descriptorPublished 四个真实落盘阶段取消请求。验证后台新上下文仍记录同一原操作 rejected/原phase，未产生初始read grant，同key重试仅调查；意图阶段目标不存在，准备阶段库仍零字节，其余阶段真实 writer.lock 可重新取得，Store可实际关闭。不修改生产授权或创建语义；不是完整V008或HTTP取消验收。
- 首次定点三轮 `t013-cancel-01.txt` **failed**：新测试错误期待context.Canceled；既有 `Authenticator.currentLocked` 将存储/上下文查询错误统一映射为 unauthorized。读取该实现后改用其既有安全错误契约，保留持久事实/授权/残留/物理锁断言；未为测试改生产行为。第二次同一定点 `test ./internal/installation -run ^TestWorkspaceCreationCancellationPreservesOriginalIntent$ -count=3 -timeout 180s` **passed**，真实exit0，日志 `t013-cancel-02.txt`。
- 更新后 `test ./... -count=1 -timeout 240s` 与 `vet ./...` **passed**（20:05:53/20:05:57 +08，exit0），命令经既有 `tools/go.ps1`，GOPROXY=off、用户选择的完全权限、合成数据。证据位于忽略目录 `.data/logs/001-validation/`，日志 `takeover-test-02.txt` / `takeover-vet-02.txt`，退出记录 `takeover-regression-02.json`，四份创建/检查源码SHA256为 `t013-cancel-source-sha256.json`。仅证明该截面局部/回归范围；当时T013/T015仍待整体签收，T014未实现，V008–V015继续unverified。

### T013/T015 局部实现签收（2026-10-18；非最终业务验收）

- 只读独立审阅发现并关闭P2：已committed创建在真实业务保存后仍走pristine genesis检查，导致原登记reconcile错误拒绝。新[真实保存回归](<../../internal/installation/workspace_creation_windows_test.go#L286-L353>)通过同一真实CommitGate执行candidate、admission、confirmed commit和物理关闭，随后显式reconcile并比较源清单/字节。首个红测夹具kind错误，不作产品缺陷证据；修正kind为create-text并加失败路径清理后，`t013-used-reconcile-red-02.txt`真实重现`incomplete or incompatible database schema/history`，不是Mock推断。
- [创建协调](<../../internal/installation/workspace_creation.go#L529-L541>)仅为committed分支使用现有当前descriptor/schema/identity验证；pending finalize仍须原pristine genesis。原volume/FileID、当前授权、原phase、无sidecar、物理排空和安装事务内原登记/grant检查全部保留。不新增repair/replay路径，不改schema或授权边界。独立静态复审关闭P2，未替代执行。
- 修复后的安装/SQLite/工作区/inspect三轮真实测试 **passed**：39.635s/30.728s/0.175s/94.411s；日志 `t013-used-reconcile-green.txt` 与退出记录同前缀`-exit.json`。gofmt后最终`test ./... -count=1 -timeout 240s`及`vet ./...`均 **passed**（20:12:49/20:12:53+08，exit0）。命令使用既有`tools/go.ps1`、GOPROXY=off、完全权限、Windows合成数据；全部当前internal Go/SQL输入SHA清单`t013-t015-final-inputs.json`与命令退出`t013-t015-final-exits.json`均位于忽略目录`.data/logs/001-validation/`。格式化前后分别记录，不用旧源pass冒充新源。
- T013已具备原阶段失败/unknown、真实native admission不确定、当前授权撤销、共享owner排空、冷子进程调查/有限reconcile及物理替换拒绝局部证据。T015具备获准root/当前operator、冻结只读副本、租户隔离先于健康输出、坏样本错误映射、源字节/清单不变与scratch清理局部证据；复用T006已验证原语，不声称本轮新增完整OS逐路径写trace。按tasks§完成门区分，勾T013/T015局部实现；T014未实现，下一步按其依赖完成登记/relink与共用操作分派。完整V008/V009仍由T024/T025执行，V008–V015继续unverified，001仍partial。

### T014 共用安装操作查询前置（2026-10-18；partial）

- 新增[当前授权查询](<../../internal/storage/sqlite/installation_operation_query.go>)，以同一只读事务校验当前安装operator/主体/授权版本及原actor，直接读取唯一installation_operation，不再要求create专用intent JOIN；复验既有INSTALL-OP/1摘要。错误路径清空返回事实，查不到不生成操作、不授权重放；locator仅内部证据，未来业务分派仍须当前允许root策略后输出脱敏view。现有create用例尚未改接，register/relink写用例及显式reconcile也未实现，不以此查询宣称T014完成。
- [共用查询测试](<../../internal/storage/sqlite/installation_operation_query_windows_test.go>)用真实安装库中create/register/relink三类合成最小操作行（无creation_intent行），验证读取、原摘要篡改拒绝、缺记录保持缺记录、错误actor/安装/session/版本不返回事实，且不创建登记。只是存储查询适配器验证，不是登记或relink业务执行。首轮故障为坏摘要夹具用了NUL导致SQLite length CHECK拒绝，改为64个ASCII零后重验，未改生产约束，原日志保留。
- `tools/go.ps1 test ./internal/storage/sqlite ./internal/installation -count=3 -timeout 240s`与同包`vet`实际 **passed**，GOPROXY=off、Windows合成数据、完全权限，20:16:39/20:16:43+08 exit0。忽略证据前缀`.data/logs/001-validation/t014-common-query-02-`：test.txt/vet.txt/exits.json；旧失败为`t014-common-query-01.txt`。底层旧无授权读取仅抽共用SELECT，不改变现有调用语义。T014继续进行中，V008/V012未获得最终pass。

### T014 持久绑定意图及定位事务（2026-10-18；partial）

- 新增[绑定事务](<../../internal/storage/sqlite/directory_binding.go>)及[局部真实测试](<../../internal/storage/sqlite/directory_binding_windows_test.go>)：register/relink原意图固定当前actor/tenant、目标、允许位置、identity digest、volume和三个物理FileID、原locator及期望登记版本；原key仅调查，所有非committed同目标/身份残留阻止新key接管。当前授权在读/写事务内复验；register需当前安装owner，relink需当前workspace.operator，不能用安装角色代替区权限。原有INSTALL-OP/1不变，未入hash的字段也逐项持久比较。
- [安装新库SQL](<../../internal/storage/sqlite/migrations/installation_001.sql>)增加唯一installation_binding_intent表，不复用create字段编码旧版本/旧定位。保持首片fresh schema=1，嵌入SQL checksum/DDL校验会拒绝旧开发截面，不提供已有库升级/重建/补种；旧源证据保留，不能用此前checksum冻结证明此新源。没有真实数据操作。此次全仓复验覆盖当前存储ledger/genesis/原生故障、路径、安全及安装回归；不新增或声称完整T014 native COMMIT端到端证据。
- FinalizeBinding在同安装事务内只登记/定位CAS与更新原operation outcome；同UUID不同root、原locator/version/tenant/volume/FileID变化拒绝。relink保持原grant集合；register不从文件导入grant。unknown只能由显式reconcile携新物理证明调查/完成，缺记录绝不创建新意图。这里只提供存储适配器，物理/lifecycle证明仍须业务owner持有，尚未实现的业务层不能被这些方法代替。
- 独立审阅找到两项P2并取得实际SQLite红证据`t014-binding-review-red.txt`：`contradictory completion accepted: <nil>`和`new key adopted rejected original: <nil>`。已在read验证phase/outcome一致性，并将原key残留屏障扩大为所有非committed状态；两项负测当前通过，独立静态复审关闭，未替代测试。初版全仓三轮pass只代表旧截面，保留full-01，不作为修复后签收。
- 修复后 `tools/go.ps1 test ./... -count=3 -timeout 240s` 与 `vet ./...`实际 **passed**（20:25:52/20:25:57+08，exit0，GOPROXY=off、Windows合成数据、完全权限）。当前internal Go/SQL SHA256为忽略目录`.data/logs/001-validation/t014-binding-inputs-02.json`，实际日志/退出前缀`t014-binding-full-02-`。当前T013/T015未改业务语义且随新schema组合复验通过；T014仍未勾选，下一步补绑定原生COMMIT故障/冷重开负证据，业务层冻结物理身份/closed-drained-known准入及共用查询/reconcile分派。V008–V015仍unverified。

### T014 目录绑定业务接线（2026-10-18；implemented，联合验收仍unverified）

- [目录协调](<../../internal/installation/directory_registration.go>)持有既有Windows排他lease及root/DB/descriptor物理身份，以只读现存DB校验描述、schema/ledger、业务ready与持久accepted/unknown保存；不补文件、不恢复业务、不导入grant。真实同卷rename保持workspace ID，安装定位以原版本CAS更新。同UUID副本/活锁/缺库及不明保存结果拒绝，源递归清单/字节保持不变。
- [最小生命周期准入](<../../internal/workspace/lifecycle.go>)由安装Store按完整scope保留唯一实例，冻结、事务与物理cleanup在同一closed临界区；opening/open/draining/unknown拒绝绑定。cleanup失败在解锁前原子置unknown，不重构实例清围栏。它只是T014准入原语，不代表T016真实open或T021队列/epoch/32与64名额及shutdown已经实现；后继组合必须复用该owner并承担真实排空证明。
- create/register/relink共用业务查询/reconcile分派及原key资源表。既有key仅调查，不自动继续已中断finalize；显式reconcile才重新证明原物理目录/身份、允许root、当前授权与原phase，再完成原定位事务。安装授权写入unknown继续沿既有Store围栏阻断依赖。
- 独立只读审阅发现query/reconcile误用workspace.operator的P2，已修为契约要求的installation-scoped installation.operator；普通Relink仍明确要求原区workspace.operator，reconcile事务另验当前安装owner/原actor/tenant/version并保留原区当前授权检查。新增安装级调查读而不放宽普通绑定写权限。独立定点静态复审关闭该项，不代替运行证据。
- [真实局部测试](<../../internal/installation/directory_registration_windows_test.go>)覆盖未登记合法源且不导入授权、重复登记/共用create查询、同卷rename/定位版本递增/旧授权失效、生命周期opening及unknown拒绝、UUID副本拒绝且不落新意图、原key不重放、活锁/缺库不修复、持久accepted与unknown保存拒绝、workspace-only调查拒绝及installation调查正例；[生命周期测试](<../../internal/workspace/lifecycle_test.go>)另验scope、并发互斥和不明cleanup原子围栏。
- `tools/go.ps1 test ./... -count=1 -timeout 240s`及`vet ./...`实际passed，21:33:10/21:33:13+08 exit0，见[组合退出记录](<../../.data/logs/001-validation/t014-business-full-results.json>)及[全仓日志](<../../.data/logs/001-validation/t014-business-full-test.txt>)。随后只增加未登记源正例测试；最终三包定向`test -run 'TestDirectoryCoordinator|TestLifecycle|TestBinding' -count=3 -timeout 180s`passed，见[三轮日志](<../../.data/logs/001-validation/t014-business-03.txt>)。源码/SQL当前SHA256与环境见[输入指纹](<../../.data/logs/001-validation/t014-business-inputs.json>)。全部Windows合成数据/GOPROXY=off/用户已选完全权限；不是原沙箱恢复证明。
- 按既有局部完成门签收T014；完整Host下的原生COMMIT不确定/崩溃冷重开/第二进程竞争等V008/V012及V008–V015整体仍unverified，未宣称001完成或硬断电通过。下一依赖为T016；没有真实数据迁移、Provider/插件运行、权限策略更改、提交/push/发布。

### T016 既有区打开与保守日志恢复（2026-10-19；implemented，联合验收仍unverified）

- [工作区open owner](<../../internal/workspace/open.go>)复用T014保留的唯一生命周期，以workspace-local互斥串行open/物理drain，准入前与发布epoch前重验当前授权；[安装接线](<../../internal/installation/workspace_open.go>)只取当前登记、实际volume/FileID和既有writer.lock/descriptor/DB，不补文件、不创建第二写者。关闭失败、持久accepted/unknown保存或恢复结果不明保留原资源/区围栏，安装Close也不先释放host.lock。Drain只是可信物理原语，T021公开close/队列/32与64名额尚未实现。
- [历史校验](<../../internal/storage/sqlite/open_validation.go>)检查既有format/schema/ledger/身份/ready、正式Head/版本/receipt及applied候选摘要/基线/审计关系；未知保存不重放、不发布可写epoch，后续业务调查仍归T020。缺文件、坏描述/摘要、未来schema、副本root与不支持sidecar拒绝且不修复。
- [明确恢复](<../../internal/installation/workspace_recovery.go>)先由[原生冻结适配器](<../../internal/platform/windows/workspace_recovery_windows.go>)冻结源DB/journal，以私有ACL scratch执行SQLite恢复及完整历史校验；全部成立并重新授权后才允许原源SQLite恢复。仅支持可证明的已同步DELETE rollback journal；WAL/SHM、多库master、no-sync/零头/不完整尾段明确拒绝，不能保证所有崩溃日志均可自动恢复。SQLite拥有恢复，不自行补写业务或删除源日志。
- 已修复复制重复句柄关闭不明未围栏、scratch清理重试阶段混淆、未设恢复资源界限、SQLite容忍坏journal记录等边界。当前单次恢复copy及journal+max(原DB,恢复DB)预算256MiB、caller-linked预检查期限30秒；这不是普通DB大小限制或物理IO强制终止保证。独立只读复审再发现并窄修journal原页数可令SQLite恢复时超预算扩张，以及sector对齐吞掉未检查尾字节两项；增加增长预算与尾字节真实适配器负例。scratch/duplicate close物理不明保留围栏；已确认清理完成的普通拒绝不永久锁区。
- [真实安装测试](<../../internal/installation/workspace_open_windows_test.go>)与[恢复测试](<../../internal/installation/workspace_recovery_windows_test.go>)覆盖原生排他、正常重开epoch更新、同生命周期拒绝绑定、冷启动unknown保持锁直到子进程真正结束、坏源清单/字节不变、未分配epoch及scratch清理、过预算A不阻断B。恢复正例使用真实SQLite COMMIT，在真实成功native刷写后、journal删除提交点前退出子进程；不伪造/截断journal，不将进程退出当断电。中间因正例留下额外未同步segment而失败的`t016-recovery-07/08`日志保留；改正中断点，不放宽生产拒绝策略。
- 最终`tools/go.ps1 test ./internal/installation ./internal/platform/windows ./internal/storage/sqlite ./internal/workspace -count=3 -timeout=240s`、`test ./... -count=1 -timeout=240s`、`vet ./...`与`git diff --check`均exit0；[退出记录](<../../.data/logs/001-validation/t016-recovery-11-results.json>)、[三轮日志](<../../.data/logs/001-validation/t016-recovery-11-focused.txt>)、[全仓日志](<../../.data/logs/001-validation/t016-recovery-11-all.txt>)及[源码/环境指纹](<../../.data/logs/001-validation/t016-recovery-11-inputs.json>)保留当前证据。Windows/NTFS、固定工具/driver、GOPROXY=off、全为隔离合成数据；无新权限、真实迁移、Provider/插件、提交/push/发布。
- 按tasks既有局部完成门签收T016；该截面下一依赖T017。完整Host/HTTP与外部harness仍未接通，V008–V015及正式保存→完整Host退出→新进程重开的最终SC仍unverified，不以本次局部/全仓测试冒称完整Host联合验收通过。

### T017 当前正式记录读取（2026-10-19；implemented，V011/V013联合验收仍unverified）

- [读取用例](<../../internal/workspace/records.go>)只使用T016保留的workspace-local owner/连接，显式绑定installation/tenant/workspace、open且无unknown的可信视图；当前授权在IO前后检查，出错不交付部分记录。与物理drain串行，不持安装全局锁跨区数据库IO；closed不从打开时快照交付内容、不自行开库/恢复，也不影响B读取。
- [安装接线](<../../internal/installation/workspace_records.go>)对snapshot、限量列表和单记录均执行契约的`workspace.read`，不从DTO/游标推断权限。独立只读审阅发现单记录初稿错用`text.read`（固定安装授权不接受、与API不符），已改正；[真实权限测试](<../../internal/installation/workspace_records_windows_test.go>)证明仅workspace.read的受限主体可读、跨B授权及撤销后保留旧Authorization均拒绝，覆盖该问题而非仅owner成功。
- 列表limit为1–100，游标是严格有界base64url的版本+installation/tenant/workspace+最后resource UUID，不含SQL/路径/凭据，也不是授权令牌；原A游标不能换B、tenant或installation使用，伪造/超长/补padding拒绝。按当前正式resource key游标分页，完整一页可产生最终空页，不承诺跨请求快照。读取[相同正式指针SQL适配器](<../../internal/storage/sqlite/workspace_facts.go>)，增加operationId事实以满足已有TextRecordView，不新建DTO结构源/第二连接/跨区查询。
- 真实Windows/SQLite合成A/B使用相同resource/operation key但不同正文：持真实writer lease经CommitGate形成receipt-backed fixture并物理关库，再通过业务owner打开/读取；验证分页不串区、未确认第三候选不在正式列表、Head=2、operationId匹配、关闭A后A拒读而B正常、明确重开A后相同正式事实。fixture不代表T018/T019业务保存已实现，也不是完整Host进程重启验收。[纯owner测试](<../../internal/workspace/records_test.go>)另覆盖迟到撤权不泄露结果和非法输入不进入facts，只证明相应局部边界。
- 最终`tools/go.ps1 test ./internal/workspace ./internal/installation ./internal/storage/sqlite -run 'TestRecordRead|TestWorkspaceRecords|TestWorkspaceFacts' -count=3 -v -timeout=180s`、`test ./... -count=1 -timeout=240s`、`vet ./...`及`git diff --check`均exit0；[记录](<../../.data/logs/001-validation/t017-records-02-results.json>)、[focused日志](<../../.data/logs/001-validation/t017-records-02-focused.txt>)、[全仓日志](<../../.data/logs/001-validation/t017-records-02-all.txt>)与[输入指纹](<../../.data/logs/001-validation/t017-records-02-inputs.json>)。固定Windows/NTFS/SQLite/工具profile，GOPROXY=off，无下载/真实数据/权限策略变更/Provider插件/提交push发布。
- 按既有局部完成门签收T017，下一依赖T018；当前全仓回归覆盖本次model/facts/open组合变更，但历史独立T016指纹仍仅代表其旧截面。V011最终完整Host重启及V013容量/长IO等联合范围继续unverified，T027/T029责任不转移、不用Mock或本次green代替。

### T018 不可变持久预览（2026-10-19；implemented，V010/V011联合验收仍unverified）

- [预览用例](<../../internal/workspace/preview.go>)复用唯一生成协议验证文本/条件输入，使用当前打开epoch、区Head与update的当前contentDigest；Host给每次预览分配新的candidate UUID及create resource UUID，同名不合并。候选绑定当前主体、原样UTF-8文本/换行、规范content/candidate摘要，previewVersion=1、有效期15分钟；内容或基线变化必须新预览，既有候选不覆盖。Before来自当前正式记录，不由客户端提供。
- [安装接线](<../../internal/installation/workspace_preview.go>)要求当前workspace.write与确切principal/scope，持区owner串行且不持安装全局锁跨IO。当前授权在保存前及IO后重验，失败不交付部分预览。[持久适配器](<../../internal/storage/sqlite/workspace_preview.go>)复用原CommitGate连接，在同一事务内重验Head/base并仅INSERT非正式Candidate；不触碰Head、TextVersion、SaveOperation、Receipt或审计。提交/回滚物理结束不确定则围栏gate、保留pending资源并将owner置unknown，不能因它不是正式保存就释放锁。
- [真实Windows业务测试](<../../internal/installation/workspace_preview_windows_test.go>)覆盖A/B隔离、同名分配、正文变化需新候选、update差异、过时Head/base、只读身份拒写、另一主体仅取得自身候选、撤销旧会话拒绝、物理关闭/新epoch重开后未确认候选仍不生效；关库后实际查询持久候选及正式表计数。[真实SQLite测试](<../../internal/storage/sqlite/workspace_preview_windows_test.go>)验证事务基线重验，并对实测3个COMMIT刷写序号逐一通过只读native句柄触发SQLITE_IOERR_FSYNC；结果unknown、后续围栏且正式Head/版本/receipt始终零。owner注入测试只证明局部校验/迟到授权与unknown状态，不替代原生证据。独立只读静态审阅无可操作发现，不作为运行证明。
- 首轮和第二轮focused命令failed是新测试夹具编译错误（sql.Conn误用QueryRow、误用不存在的身份/helper名称），不是产品通过或产品缺陷证据；保留[首轮](<../../.data/logs/001-validation/t018-preview-01-focused.txt>)、[第二轮](<../../.data/logs/001-validation/t018-preview-02-focused.txt>)诊断。修正夹具后第三轮passed。最终`tools/go.ps1 test ./internal/workspace ./internal/installation ./internal/storage/sqlite -run 'TestPreview|TestWorkspacePreview' -count=3 -timeout=180s`、`test ./... -count=1 -timeout=240s`、`vet ./...`及`git diff --check`均exit0；[结果](<../../.data/logs/001-validation/t018-preview-04-results.json>)、[focused](<../../.data/logs/001-validation/t018-preview-04-focused.txt>)、[全仓](<../../.data/logs/001-validation/t018-preview-04-full.txt>)及[源码/Windows/NTFS指纹](<../../.data/logs/001-validation/t018-preview-04-inputs.json>)。GOPROXY=off，固定既有工具/SQLite profile，无下载、真实数据或外部副作用。
- 按tasks既有局部完成门签收T018；下一依赖T019正式保存、T020调查和T021排空/容量协调。当前尚无完整Host/HTTP/harness，V008–V015、完整Host重启与最终001验收继续unverified，不以本次全仓局部测试代替。

### T019 唯一正式保存路径（2026-10-19；implemented，V010/V011联合验收仍unverified）

- [区owner](<../../internal/workspace/commit.go>)通过当前授权及确切scope串行调用已有真实CommitGate；[安装接线](<../../internal/installation/workspace_commit.go>)只从当前身份形成可信SaveRequest，不接受客户端principal/tenant。先按区/key/原输入调查，再对NEW admission检查epoch、previewVersion、候选/主体、expiry与基线。同key相同输入在候选applied、旧epoch/旧base时返回原receipt；不同主体/候选/摘要拒OPERATION_KEY_REUSE，不泄露他人receipt。区与区可同operationId，不扩大为全局去重。
- 只有持久admission确认才铸造内部单次AcceptedSave并由原owner推进，业务事务仍由原CommitGate原子写版本、Head、receipt、candidate状态、operation与审计；没有第二writer。请求取消在admission前拒绝；admission后使用owner收尾context，取消仅改变交付，不假称回滚。IO后当前授权/原生资源重验，迟到撤权不交付receipt；结果交付失败与已确认正式事实分开。任何accepted/unknown或物理错误保留pending、生命周期围栏及安装/区lease，不能以close或新key续跑。T021队列/异步等待者未在此实现。
- [真实业务测试](<../../internal/installation/workspace_commit_windows_test.go>)经完整create/open/preview/confirm路径验证A/B同key不同正文、update、同旧base竞争、同key调查、异载荷/主体、只读/撤权、旧epoch只调查、错误previewVersion不获新admission、物理关闭重开后唯一正式事实；不是仅靠mock或低层seed。对实测admission3次+正式COMMIT3次native FlushFileBuffers逐序号1–6故障，子进程真实SQLITE_IOERR_FSYNC后unknown、原key只查、A拒close/新写且writer/install lease仍被占、B仍能新预览/提交；父进程仅在确认原进程退出后查原始计数。序号0健康对照。另在第4次实际成功native flush中取消调用者（已过持久admission），真实事务仍完成一次，返回confirmed+viewDeliveryFailed，后续同key取得唯一receipt，无重复版本。局部owner假端口仅补充迟到撤权和无部分交付边界。
- 独立只读审阅发现交付阶段误查`context.WithoutCancel(ctx)`的Err（永远nil），遗漏原调用者取消；已改为检查原`ctx.Err()`，补充owner及真实native flush内取消的回归。迟到撤权不泄露receipt另有owner测试。首轮focused failed另为新故障夹具漏声明子进程scope（非产品通过证据），保留[首轮诊断](<../../.data/logs/001-validation/t019-commit-01-focused.txt>)，修正后第二轮passed。最终`tools/go.ps1 test ./internal/workspace ./internal/installation ./internal/storage/sqlite -run 'TestCommitOwner|TestWorkspaceCommit|TestCommitGate' -count=3 -timeout=240s`、`test ./... -count=1 -timeout=240s`、`vet ./...`、`git diff --check`均exit0；[结果](<../../.data/logs/001-validation/t019-commit-03-results.json>)、[三轮focused](<../../.data/logs/001-validation/t019-commit-03-focused.txt>)、[全仓](<../../.data/logs/001-validation/t019-commit-03-full.txt>)及[源码/Windows/NTFS指纹](<../../.data/logs/001-validation/t019-commit-03-inputs.json>)。GOPROXY=off，固定既有Go/driver/SQLite profile和已授权BOOL修正，无依赖/系统安全变更。
- 按既有局部完成门签收T019；T020保存调查、T021排空/容量、T022完整Host及T023外部harness仍未接通。T026/V010、T027完整Host重启/V011及最终V008–V015/001仍unverified；局部子进程故障及全仓测试不冒充最终Host联合验收。

### T020 原保存结果调查与可信物理收尾（2026-10-19；implemented，V010/V014联合验收仍unverified）

- [调查用例](<../../internal/workspace/reconcile.go>)及[安装接线](<../../internal/installation/workspace_reconcile.go>)复用原区owner、当前workspace.read/同主体或显式workspace.operator权限。查询不自动open、不给缺记录推断rollback、不执行候选；原主体/摘要和SAVE/1重算由[既有区库适配器](<../../internal/storage/sqlite/workspace_operations.go>)校验。迟到撤权不交付receipt，交付取消/错误不改已确认持久事实。owner的查询/调查使用脱离HTTP取消的context，原ctx取消只报告viewDeliveryFailed，避免任意key的GET取消被误当保存不确定；低层适配器直接取消仍保守unknown，不能误称适配器本身已消除全部取消不确定性。
- 显式reconcile在同一原生writer lease、descriptor/DB/root身份pin和重新授权下，先证明原连接实际结束，再清理上次保留的精确scratch/冻结句柄、保守日志预恢复并打开全新可信视图。只有正面原admission及完整同历史中无receipt，才持久记录NOT_COMMITTED；没有admission/损坏/证据不足继续unknown。始终不重放原候选、不换key；完整历史还有其他pending则不解围栏。可信结果要求显式物理drain和新epoch重开，不从fresh查询结果伪造原连接仍可用。
- 修复审阅发现：pending资源即使查到正面receipt也不能跳过物理调查而永久无法Close；前次失败的恢复临时对象不能被新对象覆盖泄漏；重复freeze复用同一已验证data目录pin，避免每次调查无限增加句柄；查询等待取消不污染健康owner。最终独立只读复查无可操作发现，仅为静态审阅，不替代运行证据。
- [真实Windows测试](<../../internal/installation/workspace_reconcile_windows_test.go>)对实测admission/正式COMMIT六个native刷写序号逐项触发IO错误：保留原lease，实际结束原连接后调查同历史；缺admission仍unknown且拒drain，正面原admission可得唯一receipt或确定未提交，之后真实drain/重开与新key正式保存成功。另用生产CommitGate在真实lease下仅写持久admission、不调用正式事务，验证新物理open拒普通epoch、当前权限调查后NOT_COMMITTED、重开Head仍0且旧key不重放；此为冷资源局部测试，不冒充完整Host进程重启验收。受限主体不获他人receipt、普通write不获operator调查、随机缺key不阻断正常保存；真实delete-denying句柄验证preflight清理失败后重试不丢旧资源。[原生pin测试](<../../internal/platform/windows/workspace_recovery_windows_test.go>)覆盖无/有普通DB pin时各128次freeze/close且data pin恒为1；owner假端口补充迟到授权/取消交付维度。
- 初期focused失败保留原始日志：新owner夹具未提供ready身份、在仍活SQLite writer上错误模拟冻结、遗漏测试DB key pin关闭、cold夹具调用Admit遗漏时间参数均为夹具错误，未放宽产品约束；原实现的pending/重试/取消问题经上述修复与回归。历史日志保留于`.data/logs/001-validation/t020-reconcile-01`至`-06`前缀；`-04`/`-05`failed、`-06`针对最后两项修复passed，不用后续pass删除早期失败。
- 最终`tools/go.ps1 test ./internal/workspace ./internal/storage/sqlite ./internal/installation ./internal/platform/windows -run 'TestSaveOperation|TestWorkspaceReconcile|TestRecoveryFreeze' -count=3 -v -timeout=240s`、`test ./... -count=1 -timeout=240s`、`vet ./...`及`git diff --check`均exit0；[结果](<../../.data/logs/001-validation/t020-reconcile-07-results.json>)、[三轮focused](<../../.data/logs/001-validation/t020-reconcile-07-focused.txt>)、[全仓](<../../.data/logs/001-validation/t020-reconcile-07-full.txt>)、[源码/契约/Windows/NTFS指纹](<../../.data/logs/001-validation/t020-reconcile-07-inputs.json>)。GOPROXY=off，固定Go1.27.2、SQLite1.60.1/libc1.77.1与既有DELETE/EXTRA/BOOL修正，无下载或系统安全变更。
- 按既有局部完成门签收T020；下一依赖T021独立队列/32与64容量/公开close及shutdown，随后T022完整Host与T023外部harness。最终V008–V015及T027完整Host结束/新进程重开仍unverified，不能将本次全仓或局部故障子进程称为完整Host验收。

### T021 独立队列与真实close/shutdown排空（2026-10-19；implemented，V013/V014联合验收仍unverified）

- [有界队列](<../../internal/workspace/queue.go>)复用原区owner/唯一生命周期和每安装唯一容量；每区最多32、安装合计64，普通请求最多31/63，控制保留位包含在上述上限内而非额外扩容。普通FIFO、调查优先；只有实际可运行的控制操作可占共享保留位，等待A原生IO的控制不独占B调查所需共享保留位。本地控制保留位与共享位分别计数；取消/拒绝统一单次release。可能启动持久IO而结果未知的ticket保留名额，重复或任意缺key的fenced调查不能另占永久物理名额；已证物理结束且结果已知才解原名额，完整实际drain才释放其余保留资源。
- [close协调](<../../internal/workspace/close.go>)保存本进程原key/主体/epoch与独立done历史，不生成持久保存receipt；当前权限查询、同key载荷/主体不符拒绝，缺历史明确不可查。close接受时先拒新普通准入及未开始的排队请求；已dispatch但尚未开始保存/候选持久IO也在队列短锁下再次检查截止，既有accepted由原owner实际完成，不把HTTP取消作为回滚。pending-drain只保留调度意图，不预占等待容量；拒绝未开始/后续补充控制，等当前物理IO结束后优先实际排空，防连续轮询饿死关闭；unknown实际关闭失败后允许显式调查，不自行清围栏。终态历史/退役代际先发布，随后释放ticket再通知done，避免重开与shutdown误等旧代际。
- [安装close/shutdown接线](<../../internal/installation/workspace_close.go>)不持Store锁等待区IO；shutdown先将全部区标为stopping再分别等待，拒新owner/普通请求但保留调查入口。真实资源Close仍须通过pending/quarantine、SQLite连接、临时对象、pins与原生lease的完整收尾；安装Close仍拒任何liveWorkspaceHandles。等待超时只返回draining/unknown，不释放writer/host锁，不声明safeToMove；共享Host crash/OOM/磁盘满仍可影响全安装，不构成OS隔离。
- [实际Windows/SQLite用例](<../../internal/installation/workspace_close_windows_test.go>)在admission已持久化后的第四个成功原生FlushFileBuffers回调内暂停A正式COMMIT：A达到31普通名额即拒额外请求，B在A回调未返回时通过自己的真实DB/native flush完成保存和读取；close拒30个排队请求，取消保存等待者/30ms关闭等待超时均保留A原生writer与安装锁；放行真实IO后保持唯一committed事实，物理关闭后可重新取得锁，显式新epoch重开读回相同正文，旧close key只查旧结果、不关闭新epoch。对实测六个admission/正式COMMIT刷写故障分别运行子进程，unknown close保留原物理名额与锁，reserved控制调查仅在正面原admission及完整历史证实时解围并唤醒实际关闭；缺admission仍unknown直到子进程结束，不发明rollback。
- [队列单元](<../../internal/workspace/queue_test.go>)验证32/64硬上限、unknown计数、局部/共享保留位、取消/FIFO和控制补充不饿死drain；[owner单元](<../../internal/workspace/close_test.go>)用明确barrier验证dispatch未admit时截止、迟到accepted完成、代际发布前不dispatch新factory、shutdown等待新代际真实Close。假端口只证明调度/边界，不替代上述原生测试。独立只读审阅发现并修复close截止、共享位饥饿、代际handoff及连续控制饿死drain等竞态，最终复查无可操作发现；静态审阅不冒充运行证据。
- 历史`.data/logs/001-validation/t021-queue-01`失败为新增排队前授权后旧单元夹具的次数断言失配，修复夹具保留迟到拒绝/零交付本意；`-02`工作区三轮、`-03`真实IO close、`-04`原生unknown close/调查、`-05`最终排空优先与代际用例三轮均passed，保留原日志。最终通过同一`tools/go.ps1`串行执行`test ./internal/workspace ./internal/storage/sqlite ./internal/installation ./internal/platform/windows -count=3 -timeout=240s`、`test ./... -count=1 -timeout=240s`、`vet ./...`及`git diff --check`，全部exit0：[结果](<../../.data/logs/001-validation/t021-queue-06-results.json>)、[四包三轮](<../../.data/logs/001-validation/t021-queue-06-focused.txt>)、[全仓](<../../.data/logs/001-validation/t021-queue-06-full.txt>)、[源码/契约/环境指纹](<../../.data/logs/001-validation/t021-queue-06-inputs.json>)。GOPROXY=off，固定Go1.27.2、SQLite1.60.1/libc1.77.1与既有DELETE/EXTRA/BOOL修正，未下载或改系统安全。
- 按既有局部完成门签收T021；下一依赖T022完整Host/HTTP/受保护pipe，随后T023外部harness。当前无完整Host，32/64证明在同一生产用例队列与真实适配器层，不冒充HTTP端到端容量已验收；V008–V015及T027完整Host退出/新进程重开仍unverified。

### T022 接线前响应边界修复（2026-10-19；partial，Host/HTTP尚未接通）

- 接线检查发现：`ParseJSON`的请求1MiB上限被通用schema验证/生成DTO读写错误复用，合法64KiB正文经JSON转义且在候选完整before/after diff重复时，响应超过1MiB就被拒；含3条合法最大正文的列表也可触发。这是响应结构实现缺陷，不应缩减正文/差异或更改OpenAPI来适配。`.data/logs/001-validation/t022-response-size-01-test.txt`保存真实red：`valid candidate rejected by request-only limit invalid JSON encoding or structure`及合法RecordList的MarshalJSON失败；同轮请求夹具把空白放在root外，被encoding/json在调用DTO UnmarshalJSON前裁去，修正为root对象内部空白，不改变产品负向约束。
- [严格协议实现](<../../internal/protocol/strict.go>)保留`ParseJSON`原请求上限，通用schema按**既有生成operation角色**区分请求根与其他schema；请求根仍1MiB，响应结构另用64MiB解析防御预算，可容纳最大100记录页与转义文本。无需改唯一OpenAPI、生成绑定或生成脚本，不新增业务字段/规范owner；重复/未知键、UTF-8/NUL/surrogate、精确revision及全部原结构约束继续共用同一解析/验证链。[回归](<../../internal/protocol/response_test.go>)验证大候选及大列表encode/decode保留原字节，DTO请求内部超限、operation原始字节边界及请求parser仍拒超限。HTTP接线仍须先对全部raw请求字节验证，不能依赖encoding/json剥除外围空白后的DTO方法来计整个请求。
- `tools/go.ps1 test ./internal/protocol -count=3 -v -timeout=240s`、`test ./... -count=1 -timeout=240s`、`vet ./...`、`git diff --check`均实际exit0：[结果](<../../.data/logs/001-validation/t022-response-size-02-results.json>)、[协议三轮](<../../.data/logs/001-validation/t022-response-size-02-focused.txt>)、[全仓](<../../.data/logs/001-validation/t022-response-size-02-full.txt>)、[当前输入指纹](<../../.data/logs/001-validation/t022-response-size-02-inputs.json>)。沿用固定Go/SQLite/Windows NTFS环境、GOPROXY=off、串行runner。T021实现与测试未改变，本次全仓再次覆盖；T022仅此子项implemented，CLI/HTTP/受保护命令pipe/DTO事实投影/绝对URI拒绝等仍待实现，不勾T022，不宣称完整Host验收。
