# 001 数据模型与字节协议

**proposed/designed；设计不是实现证据。** 需求见[规格](<spec.md>)；设计入口见[计划](<plan.md>)；当前功能授权与历史批准仅见[授权记录](<authorization.md>)。
本片所有正式文本直接进入工作区 SQLite；不创建空对象文件，不声称完整对象/投影协议完成。

## 1. 三种版本与目录最小落地

| 轴 | 提议首值 | 校验与演进 |
| --- | --- | --- |
| 目录 `formatVersion` | 2 | 完整业务格式，不兼容旧身份 v1，不由 open 迁移 |
| 工作区/安装库 schema | 各自 1 | 分别有显式 schema 元数据及 migration ledger；`user_version` 必须一致；未来值拒绝 |
| HTTP 结构 | `/v1` | 不从目录/schema 推算；字节摘要另有协议标签 v1 |

create 只建立有真实消费者的 `aboard.workspace.json`、`data/workspace.sqlite`、`.aboard/locks/writer.lock`；未用的目标业务目录不预建。
JSON 描述严格 UTF-8、无 BOM、拒重复/未知字段；字段为 formatVersion、workspaceId、tenantId、databaseRelativePath（固定 `data/workspace.sqlite`）、identityDigestVersion（1）、identityDigest。
**不含 principal 授权、密钥、业务 revision、绝对路径或可独立修改的业务状态。** 格式非签名/授权 seal；“摘要正确”不证明来源可信。
数据库 identity 的同字段与 descriptor 匹配；正文候选文件不参与 open。描述不含随保存变化的 revision，因此单次文本提交不需要跨文件更新。
writer.lock 内容无授权/owner 权威，可空；文件对象必须已存在，句柄排他才证明占用。允许 SQLite 自己管理 DELETE journal；异常遗留不是可手删缓存。

## 2. 安装级模型（同一安装协调者唯一写入）

| 实体 | 最小字段/约束 | 归属 |
| --- | --- | --- |
| InstallationIdentity | installationId、tenantId、ownerPrincipalId、Windows SID 绑定、schemaVersion；单行 | 安装身份，不由目录生成 |
| Principal / Grant | 稳定 principalId、tenantId、权限版本、显式 workspaceId/action 集合、撤销状态 | 当前权限；不在工作区副本中 |
| ClientSession | sessionId、tokenHash、principalId、expiresAt、revoked、grantVersion | 原始随机 token 不入库/日志/公开 DTO |
| DirectoryRegistration | workspaceId 唯一、tenantId、root locator、volume/root file ID、允许位置、登记版本、state | 仅定位/安全绑定，不存文本/业务 revision |
| InstallationOperation | operationId、actor、target locator、workspaceId、kind、inputDigest、phase、outcome | 创建/登记/relink 的协调事实，不复制工作区业务 |

安装秘密由用户级 DPAPI 包装于严格 ACL 文件；安装库只存引用/校验信息，不让普通客户端直读。
InstallationOperation 阶段：`intentRecorded → directoryPrepared → genesisConfirmed → descriptorPublished → registrationConfirmed`，失败可 `rejected` 或 `unknown/needsReview`。
文件/两库之间没有单事务；安装返回完整 create 成功必须同时确认目录 genesis 与登记 finalize。任何阶段不明不自动删除、重建、换 ID 或重放；显式调查/完成登记仅限当前操作员权限。
若安装登记 finalize 未确认，目录仍不可作为本安装已就绪实例打开；返回 `REGISTRATION_PENDING`。独立 inspect 只能描述磁盘证据，不能据此宣布安装操作成功。

## 3. 每工作区 SQLite（每行明确归属）

| 实体 | 拟议字段/约束 | 用途 |
| --- | --- | --- |
| WorkspaceIdentity | workspaceId、tenantId、formatVersion、schemaVersion、identityDigest、displayName、businessState；单行 | 正式身份；不得通过编辑描述改写 |
| WorkspaceHead | workspaceId、revision；初始0 | 全区文本提交的单调序列，区内候选基线 |
| TextRecord | workspaceId、tenantId、resourceId、currentVersion；复合唯一键 | 当前正式资源归属，不靠全局 ID 查询 |
| TextVersion | workspaceId、resourceId、version、workspaceRevision、title、body、contentDigest、operationId、createdBy | 不可变正式版本；当前指针与版本同事务 |
| Candidate | workspaceId、candidateId、principalId、resourceId、kind、baseRevision、baseDigest、title/body、candidateDigest、previewVersion、expiresAt、state | 预览保存的非正式候选；永不自动 Apply |
| SaveOperation | workspaceId、operationId、principalId、candidateId、inputDigest、admissionEpoch、state、failureCode | 已接受/拒绝/待调查关联；accepted 先持久化，不等于正式保存 |
| CommitReceipt | workspaceId、operationId 唯一、inputDigest、principalId、candidateId、resourceId、newVersion、newRevision、contentDigest | 与正式版本/指针/Head 原子提交；调查权需重新授权 |
| AuditEntry | workspaceId、principalId、operationId、action、outcomeCode、revision | 同库脱敏审计，不保存正文/秘密；记录未知不能伪造成功 |

所有关联同时绑定 workspaceId，FK 不能只按 resourceId；使用 NOT NULL/CHECK/UNIQUE 与复合 FK；连接回读 foreign_keys=1。tenantId 与 identity 必须一致，身份来自 Host 上下文而非请求体。
新增候选由 Host 分配 resourceId；同名/同客户端临时键不合并实体。M1 不提供删除、通用历史 UI/回滚或任意 SQL。
文本标题 1–256 Unicode scalar，正文至多65536 UTF-8字节；拒绝无效 UTF-8、孤立 surrogate 和 NUL，不做换行/Unicode规范化。请求上限1MiB，解析拒重复及未知键。限制是待评审技术提案，不改变最小记录语义。
revision/version 为非负64位范围内整数，JSON 对外用十进制字符串；0为初始/创建基线，更新后>=1，溢出拒绝不回绕。时间不决定顺序或权限。

## 4. 摘要协议 v1（本仓唯一权威）

不直接 hash JSON、Go struct、路径、展示名、时间或平台换行。所有 byte stream 采用：ASCII 域标签 + 单个 `00` 字节，再按下述固定顺序编码字段。

- `S(x)`：`uint64 big-endian(len(UTF8(x))) || UTF8(x)`；不做 NFC/NFD/CRLF 转换；合法 UTF-8 无 BOM。
- `U(n)`：8字节 unsigned big-endian，协议仍限制 n<=2^63-1。
- UUID：Host生成 UUIDv4，展示小写标准带连字符；序列化为16个 UUID原始字节（按展示hex顺序，不用 Windows GUID 内存字节序）。任何大小写/非规范输入拒绝。
- `D(x)`：32个原始 SHA-256 字节；对外呈现64个小写hex，不 hash hex字符串。
- 可选值：一个 presence 字节 `00` 或 `01`，若01紧跟字段编码；不用空串替代 absence。

| 摘要 | 域标签与固定字段顺序 |
| --- | --- |
| identityDigest | `ABOARD/IDENTITY/1`：U(formatVersion), workspaceId, tenantId, S(databaseRelativePath) |
| contentDigest | `ABOARD/TEXT/1`：workspaceId, resourceId, S(title), S(body) |
| candidateDigest | `ABOARD/CANDIDATE/1`：workspaceId, principalId, candidateId, resourceId, S(kind), U(baseRevision), optional D(baseDigest), D(contentDigest), U(previewVersion) |
| save inputDigest | `ABOARD/SAVE/1`：installationId, tenantId, principalId, workspaceId, operationId, candidateId, D(candidateDigest) |
| install inputDigest | `ABOARD/INSTALL-OP/1`：installationId, principalId, operationId, S(kind), workspaceId, S(canonicalLocator), optional U(registrationVersion) |

create-text 的 baseDigest absent；update-text 的 baseDigest 必须是当前文本 contentDigest，baseRevision 是区级 Head。确认时重算而非相信客户端提供的 digest。
身份摘要不覆盖 DB 全文件：SQLite页布局、journal与业务写入会变化；不把数据库文件hash当正式版本。open校验identity及当前正式TextVersion的字节摘要、指针/receipt一致性；资源不多时首片逐一验证，有坏条目拒绝该区写入，不无理由停 B。

**固定夹具要求（设计；T003的当前生成/独立执行证据见[研究记录](<research.md>)）**：ASCII、中文、emoji/组合字符、LF与CRLF不同、空正文、嵌入NUL拒绝、UUID byte order、revision 2^53+1、最大值/溢出、可选absence、伪造hex、字段重排JSON同一语义。先固定输入十六进制与expected digest，再让 Go/独立脚本/未来TS消费者逐字节对照；禁止自算自证或两仓各写独立“规范”。

## 5. 候选与保存状态

1. preview：当前授权+打开epoch；校验基线/文本，在该区库保存 Candidate 及预览版本；返回完整待确认文本/差异。不是正式TextVersion，不发 saved。
2. confirm：先每次重新授权并按workspaceId/operationId查既有SaveOperation/Receipt，验证同principal/inputDigest；已有结果仅返回原receipt/进行中/unknown，不重新准入，不因候选applied/过期、旧base或旧epoch而重做/误拒已提交结果。若区已关闭/无可信读视图，报告需获准重开调查，不自动打开。只有全新operationId才校验当前openEpoch、candidate归属、digest/previewVersion/未过期与基线，持久化SaveOperation=accepted；admission结果不明返回unknown，不开始业务提交。
3. 区串行提交者在短事务中再次先查同 key Receipt（当前授权/输入一致才返回），再校验基线；同原子事务写 TextVersion、TextRecord、Head、Receipt、Operation=committed、Candidate=applied 与成功审计。
4. Commit 获明确成功才发送 committed 回执；丢回执可能客户端 unknown/服务端 committed。确认前失败不写正式版本；可证明回滚才记录 rejected。错误、超时、连接关闭不直接等于回滚。
5. 有未结束事务或不能证明持久结果，SaveOperation 的外部视图为 unknown；封住该区新写，保留锁/运行epoch，调查至可证明边界。receipt先于旧base判断，因此已提交的同 key 调查不被后来revision误判为冲突。

保存key作用域为`(workspaceId, operationId)`；安装操作为`(installationId, operationId)`，没有安装级全工作区去重表。跨区同operationId不关联；调查必须携原workspace+key，不可换区重试未知操作。receipt/operation不自动过期删除；本片无历史裁剪/备份回滚/clone工具。保证限于同一未替换的持久历史中、同区同key的至多一次正式状态效果；不声称网络exactly-once、跨区查重、外部副作用幂等或换key安全。
受损库或恢复后仍无可信证据时维持unknown；“没找到receipt”只有在原事务已物理结束、获得排他并恢复到可验证一致历史后才可能证明未提交。该证明不能自行触发重放，用户须新预览与确认。

## 6. 独立状态维度

| 维度 | 允许含义 |
| --- | --- |
| businessState | 本片文本能力 `ready`；其他业务能力 `unavailable`；未来archived独立，本片不提供归档命令 |
| directoryLifecycle | `unregistered / registeredClosed / opening / open / draining / closed / rejected` |
| accessMode | `none / writable`；不承诺持久readOnly open，inspect永远none |
| integrity | `unchecked / valid / corrupt / incompatible / incomplete / conflict` |
| persistenceHealth | `known / unknown / unavailable`，作用域workpace或installation，附dependency理由 |
| operation logical | `accepted / committed / rejected / unknown`，不是Run状态 |
| operation physical | `queued / executing / reconciling / drained`；逻辑结束不能冒充drained |

运行时对象携带不可复用openEpoch；DB连接/锁/工作队列归该区handle，所有提交回查epoch和admission gate。epoch非稳定身份/授权，关闭后旧handle/延迟回调失效。
close的LifecycleOperation是Host内存协调记录（workspaceId、operationId、principalId、openEpoch、kind=close、logical/physical/persistence），不是SaveOperation/正式提交receipt；同进程保留至退出，接受时查同区保存/生命周期key冲突且先重授权。它负责关闭后仍可调查实际排空，不另开库写“已关闭”。重启后不能从内存缺记录推断原close成功/回滚，返回OPERATION_HISTORY_UNAVAILABLE；需重新验证目录与锁后报告当前状态，不伪造原close回执。保存receipt的持久保留保证不因此降级。
Run逻辑/物理/持久状态保留为未来契约要求，当前不存在Run表、Run API、Provider worker或SSE业务事件；不以通用operation状态声称Run已实现。

## 7. 首片数据实现范围与演进门（designed，未实现）

长期分类与延期项只在[总体范围 §3.1](<../../docs/scope.md#31-长期数据策略与单一实现范围designed未实现>)维护；本节只约束 001，不增加第二套业务模型或阶段路线。

| 001 后续必须落实 | 本片不实现 |
| --- | --- |
| 安装库与每区 SQLite 的身份、实体、复合归属/约束、版本/摘要与回执；正文直接入库 | 通用业务资源模型、对象/附件引用表、Run/事件表、搜索/向量索引 |
| 一种驱动及一种固定且回读验证的连接 profile；区内串行提交与独立 handle/epoch | 双驱动上线、运行时换库、WAL/读连接优化、通用 CRUD Repository |
| 新库 schema=1 的版本化 SQL、安装/工作区各自 ledger、初始化与严格版本检查 | 已有业务 schema 升级/降级器、旧库导入、open/inspect 自动迁移 |
| accepted/正式提交/未知的持久关联、原 key 调查、关闭与新进程重开 | 历史裁剪、自动删除 receipt、备份历史替换后的去重保证 |
| 明确排空/未知结果门、拒绝无证据的安全判断 | 备份恢复、移动/克隆工具、对象 GC；首片同卷停机 relink 仍按原规格验证 |

### 7.1 新库 ledger 与兼容拒绝

- 安装库和每区库分别维护 schema 元数据与 migration ledger，不共享业务迁移状态；SQL migration 版本按确定顺序排列。ledger 最小记录版本、对应 SQL 校验和及成功应用状态；时间只供审计，不用于排序/权限判断。
- M1 只在显式 create/安装初始化的全新库中应用 schema=1。schema、初始身份与成功 ledger 记录在各自库内同一初始化事务中一致建立；不把这一点推广为安装库/工作区库/目录描述之间原子。创建中断继续遵循安装意图及残留/unknown 调查规则。
- open 核对 schema 元数据、user_version、支持版本及 ledger 的完整性、顺序、成功状态与已知校验和；任一缺失/矛盾、未完成、旧业务版本或未来版本均拒绝，不修正 ledger、不补表、不把无法识别的库当新库。inspect 只能在其已冻结副本上报告，不执行 migration。
- 后续 schema 升级须另行明确支持的源/目标版本、排他/排空、原子范围、失败恢复和兼容策略；不得仅改变 user_version、删库重建，或用旧 binary 打开新库来回退。本片不创建升级命令或迁移框架空骨架。

### 7.2 后续数据功能必须继承的限制

本片不预建对象、事件、归档或多数据库抽象。未来对象正式引用前必须证明最终字节与摘要；GC/备份必须保护正式版本及有效快照所需对象，不能把临时 staging 或缓存当唯一数据。恢复需验证数据库与对象清单/版本的一致性，并重新确定可写身份与当前授权。恢复旧历史可能缺失后来已提交回执，不得据此重放原操作或延续原历史去重承诺；恢复/分叉协议归 M7，保持 §5 的保证范围。

未来索引、投影和跨区聚合可重建且必须授权，不可直接写其他区数据库。SQL、PRAGMA、连接和驱动错误映射留在存储适配器，用例依赖有业务语义的端口；这只降低演进耦合，不构成任意数据库兼容承诺。

**新增静态设计反例（待工程验证，不是通过记录）**：两库各自 ledger 缺失/校验和或状态不符、user_version 与 schema 冲突、旧/未来 schema、初始化事务中断。分别纳入现有 G6/V7/V8；保持零写 inspect、拒绝隐式迁移与残留调查，不增加真实库迁移实验。
