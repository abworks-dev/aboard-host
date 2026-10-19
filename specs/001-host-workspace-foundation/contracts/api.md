# 001 公共协议设计

**proposed/designed；非已实现 API。** 结构权威已按T002收敛为同仓唯一[OpenAPI源](<../../../contracts/openapi.json>)；本文件保留行为解释，[数据模型](<../data-model.md>)拥有摘要字节规范；[计划](<../plan.md>)拥有顺序与验证门。
这份行为契约是当前设计权威；Go绑定由唯一OpenAPI源生成并校验漂移，未来TS消费者使用同一源，不在另一仓手写独立TS规范。当前功能授权与历史批准仅见[授权记录](<../authorization.md>)；设计交付时未生成客户端、服务桩或业务接口，不构成当前阶段上限。

## 1. 连接、身份与通用约束

- 仅明确配置的回环 HTTP authority；默认不监听0.0.0.0/公网。Host header必须与实际listener authority严格匹配，拒absolute-form/重复歧义Host；本片拒有Origin及浏览器preflight，不开放CORS/cookie认证。
- 每个公开业务请求带 `Authorization: Bearer <opaque session>`；从Host验证取得installationId/tenantId/principalId与当前grant。客户端不能选择安装/主体上下文，JSON含这些身份字段则拒绝，不“忽略后相信路由”。
- 会话由Host本地操作员控制面显式签发，经受SID保护且验证对端的named pipe交付；非公开HTTP登录/秘密读取API。密钥/token不在URL、命令行、日志、普通DTO。完整引导验证门见[研究R5](<../research.md#r5-本地认证凭据与引导>)。
- JSON UTF-8、拒重复/未知字段/非法编码；请求<=1MiB；仅允许固定Content-Type application/json。UUID规范、十进制字符串revision、sha256小写hex见模型。所有ID由Host生成或按同格式校验，ID不是权限。
- 每次查询/修改/状态调查都重新授权；DB访问先绑定workspaceId，查询使用复合键。未授权/不存在目标统一受限NOT_FOUND或FORBIDDEN，不泄漏存在、路径、标题、错误摘要。
- create/register/inspect/relink是受限安装操作；仅有A业务权限者不能借path探测B或任意服务器文件系统。操作员获准root集合与“可创建新工作区”能力单独检查；请求path只是待核验定位，不能扩大允许root。
- 公开DTO默认不含绝对路径/Win32原始错误/SQL/正文日志。获授权目录操作员可在受保护控制面看自身定位；HTTP仅返回workspaceId与脱敏locationLabel。
- 仅HTTP查询/命令足够本片，持久SSE事件/Run订阅不实现；返回capabilities明确其unavailable。断连不构成业务取消。没有`cancelRun`空操作。

## 2. 共同 DTO

```text
PrincipalView = { installationId, tenantId, principalId, capabilities[] }
WorkspaceView = {
  workspaceId, tenantId, locationLabel,
  businessState, directoryLifecycle, accessMode, integrity,
  persistenceHealth: { state, scope, reasonCode? },
  openEpoch?, revision?, availableCapabilities[]
}
TextRecordView = {
  workspaceId, resourceId, version, workspaceRevision,
  title, body, contentDigest, operationId
}
CandidateView = {
  workspaceId, candidateId, resourceId, kind,
  baseRevision, baseDigest?, title, body, contentDigest,
  candidateDigest, previewVersion, expiresAt, validation,
  diff: { beforeVersion?, beforeTitle?, beforeBody?, afterTitle, afterBody }
}
OperationView = {
  operationId, scope, workspaceId?, kind,
  logicalState, physicalState, persistenceState,
  receipt?, reasonCode?, correlationId
}
Receipt = {
  workspaceId, operationId, candidateId, resourceId,
  newVersion, newRevision, contentDigest
}
ErrorView = {
  code, outcome: rejected|unknown, correlationId,
  operationId?, scope?, retryPolicy: none|investigate|newPreview,
  safeMessage
}
```

所有返回的正文/候选/receipt属于当前已授权目标，不可从另区拼接。operation的scope仅installation或指定workspace，不是泛查所有区。
Operation logical/physical/persistence是三轴；同一operation可`logicalState=unknown, physicalState=reconciling, persistenceState=unknown`。客户端没收到committed只能自认unknown；不能篡改服务器的确证committed状态。非执行文本能力ready不代表Agent/Workflow/插件/Provider业务ready。

## 3. 最小操作表（路径是设计，不是本轮可执行命令）

| 方法/路径 | 输入核心字段 | 行为与权限 |
| --- | --- | --- |
| GET `/v1/context` | 无 | 已认证主体及可用能力；不列无权工作区 |
| POST `/v1/workspace-creations` | operationId, path, displayName | 安装operator+允许root；仅全新空目录；显式创建并finalize登记/初始授权，完全确认后返回workspaceId |
| POST `/v1/workspace-inspections` | path | 安装operator+允许root；只读目标检查，不登记/授权/补建；活库/不能冻结返回unavailable |
| POST `/v1/workspace-registrations` | operationId, path, expectedWorkspaceId, expectedIdentityDigest | 安装operator；只登记合法、已授权租户且无冲突目录；不自动从文件导入授权，不接受任意tenant/principal |
| POST `/v1/workspaces/{workspaceId}/relink` | operationId, path, expectedRegistrationVersion | 原工作区当前operator权限+允许root；必须closed/drained/known；同卷停机rename物理身份一致；副本/旧位仍活拒绝 |
| POST `/v1/workspaces/{workspaceId}/open` | expectedRegistrationVersion | workspace.open+当前定位授权；本片仅writable，锁/格式/身份/摘要均验证后返回openEpoch |
| GET `/v1/workspaces/{workspaceId}` | 无 | workspace.read；返回独立状态，未open不假称业务可读 |
| POST `/v1/workspaces/{workspaceId}/close` | operationId, openEpoch | workspace.close；先拒新准入，再按实际排空收尾；202不表示closed |
| GET `/v1/workspaces/{workspaceId}/records` | limit<=100, cursor? | workspace.read；只该区当前正式记录，分页不能切换目标；无任何全局currentWorkspace |
| GET `/v1/workspaces/{workspaceId}/records/{resourceId}` | 无 | workspace.read；核归属，外区ID拒绝 |
| POST `/v1/workspaces/{workspaceId}/record-previews` | openEpoch, kind, resourceId?, baseRevision, baseDigest?, title, body | workspace.write；create无resourceId/baseDigest，update两者必填；保存候选并返回预览；不写正式版本 |
| POST `/v1/workspaces/{workspaceId}/record-commits` | operationId, openEpoch, candidateId, candidateDigest, previewVersion | workspace.write+同主体候选；显式确认；重新授权/校验，先持久admission，随后区owner事务提交 |
| GET `/v1/workspaces/{workspaceId}/operations/{operationId}` | 无 | workspace.read+同主体或显式operator调查权限；查持久保存关联或本进程close协调状态，不触发重放/执行 |
| POST `/v1/workspaces/{workspaceId}/operations/{operationId}/reconcile` | 无 | 显式workspace.operator；仅调查保存/close状态，守排他、物理结束、DB恢复门；不是repair/migration/retry |
| GET `/v1/installation-operations/{operationId}` | 无 | 当前installation.operator；调查create/register/relink，查证据不重放创建 |
| POST `/v1/installation-operations/{operationId}/reconcile` | expectedPhase | 显式installation.operator；仅在原意图/目录/身份/当前授权均可证明时完成登记；不补业务库/重建目录/改ID，不知则维持unknown |

register相同身份/相同物理root/同授权重复请求可返回现有登记，无第二事实；相同UUID不同root拒IDENTITY_CONFLICT。其他安装持目录不获原授权；本片无跨安装导入/配对路径。
create的新workspaceId由Host在意图中分配，不能在授权前依靠客户端自报；完成登记/初始grant之前目录不开放业务请求。
全新写准入及内部延迟回调必须与当前活openEpoch相同；旧epoch返回STALE_HANDLE。同operationId重复confirm先按当前授权/相同输入调查既有结果，属于只返回原关联，不因旧epoch/已applied候选/过期或旧base再发起写入；这不是旧句柄获得提交权。保存读取/调查不自行打开目录；closed或无可信读视图时返回WORKSPACE_CLOSED/需重开调查，不伪造回滚。close协调记录在本进程完成关闭后仍可授权查询；该内存状态不构成持久保存receipt，重启丢失时返回OPERATION_HISTORY_UNAVAILABLE，不把缺记录当原close成功。

## 4. 回执、重复请求与错误

- 200/201只表示其返回的已确认结果；异步接收返回202 + OperationView/Location（仅operation路径，不含token/原路径）。202 accepted不是正式保存成功。
- 保存key的唯一作用域是`(workspaceId, operationId)`；安装操作key是`(installationId, operationId)`，二者不是全局ID注册表。record-commits在同一workspace作用域相同key+相同输入时，先重新授权/查既有关联；已committed返回唯一旧receipt，执行中/unknown返回现状，不第二次提交。同作用域更换载荷、主体、candidate/resource目标拒`OPERATION_KEY_REUSE`，不泄漏另一主体receipt。不同工作区可碰巧使用同operationId，彼此无查重/原子承诺；客户端调查必须保留原workspace+key，严禁把换区/换key当未知结果重试。
- 新operation在admission前校验失败可返回明确rejected；admission事务本身或后续commit结果不明，返回`COMMIT_UNKNOWN`或连接中断后由operation查询调查。若连accepted都不确定，客户端保留原operationId，不改key自动重发。
- 同key“调查”不等于授权重试；unknown无receipt不能因为超时到达就变rejected。持久化与原事务物理结束均得到证据后才可给已提交/确定未提交结论。
- 正式保存成功与后续响应序列化/视图错误分开：返回/调查receipt保留committed，另报viewDeliveryFailed；不能把正式事务称回滚，不自动再save。M1无生成投影写入，更不覆盖候选文件。

| HTTP/类别 | code例 | 结果与动作 |
| --- | --- | --- |
| 401/403/受限404 | UNAUTHENTICATED, FORBIDDEN, NOT_FOUND | rejected；不返回目标存在/内容或秘密 |
| 400/413/422 | INVALID_INPUT, REQUEST_TOO_LARGE, INVALID_CANDIDATE | rejected；重新输入/预览，不改正式事实 |
| 409 | STALE_BASELINE, IDENTITY_CONFLICT, WRITER_BUSY, DIRECTORY_NOT_EMPTY, STALE_HANDLE, OPERATION_KEY_REUSE | 当前请求rejected；busy需新准入，不自动重放提交 |
| 409/422 | INCOMPLETE_DIRECTORY, MISSING_DATABASE, CORRUPT_DATABASE, DIGEST_MISMATCH, UNSUPPORTED_FORMAT, UNSUPPORTED_SCHEMA, UNSUPPORTED_PATH | 拒绝；不创建/恢复/降级，损坏仅影响对应区 |
| 409/503 | INSPECTION_UNAVAILABLE, RECOVERY_REQUIRED, REGISTRATION_PENDING, WORKSPACE_CLOSED, WORKSPACE_DRAINING, OPERATION_HISTORY_UNAVAILABLE | 对应范围不可用；inspect绝不切RW |
| 503 | COMMIT_UNKNOWN, INSTALLATION_STATE_UNKNOWN | outcome=unknown、investigate；解释作用域/依赖，不返回safe-to-retry |
| 429 | CAPACITY_LIMIT | 准入前rejected；只约束本片HTTP队列，不改Native/Build容量 |
| 501 | CAPABILITY_UNAVAILABLE | 通用文件Apply/Build/Studio/Run/插件/跨区协作不可用；无假数据/空成功 |

code完整枚举在工程结构契约冻结时扩充必须保持上述语义；不能把所有SQLite异常压成500后让客户端自动retry。内部Win32/SQL详细错误只写脱敏诊断，绝不带正文/token。

## 5. 生命周期与连接

close接受后原子关闭A准入；其已accepted操作继续由A owner推进/调查，HTTP context取消仅关闭等待者。close返回202期间保留epoch/锁；即使close等待者离线也不取消排空。
只有A全部物理DB操作/回调已结束、结果已知、DB连接关闭、锁句柄释放后，close才committed/closed/accessMode=none；等待超出设计等待窗口只能返回draining/unknown，不能强关锁或宣称safeToMove。
close中旧epoch的新commit拒绝；close前已accepted者只能由原epoch受控收尾。B的独立队列/DB不等A，不继承A的关闭context。
Host正常停机对所有区执行同规则；强杀可终止进程但不伪造排空记录。重启必须重新取得安装/区锁、重新认证并验证历史；未确认操作不自动重放，候选不自动生效。

## 6. T002结构冻结与绑定方式

- 唯一机器结构源为[OpenAPI 3.1.0](<../../../contracts/openapi.json>)：16个操作、32个schema；本文件的表是行为说明，不另生成第二套TS结构权威。
- T003将从该源生成Go DTO/枚举并提供生成结果漂移检查；严格JSON解析/结构边界验证以该源为输入，另外验证UUID/十进制上界、UTF-8字节与scalar限制、create/update条件、重复/未知键/非法编码。`x-*`承载的授权/字节/生命周期约束必须落实与测试，不能把普通schema验证当其执行证明。
- 操作持久轴使用known/unknown/unavailable，独立于逻辑accepted/committed/rejected/unknown及物理queued/executing/reconciling/drained。HTTP 202是OperationView而非Receipt/closed；无输入的POST拒请求体。受限错误不公开路径、SQL或目标正文。
- 静态检查入口为[verify_openapi.py](<../../../tests/fixtures/verify_openapi.py>)，仅用Python标准库核对行为路径/输入、引用闭合、严格对象、鉴权元数据、作用域与拒绝边界，包含检查器负向变异。它不是完整OpenAPI实现合规验证、Go绑定验证或任何认证/业务运行证据；V002b留待T003。
