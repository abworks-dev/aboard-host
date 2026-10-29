# Implementation Plan: 002 完整资源版本、不可变对象与正式提交

当前实现/验收状态只引用[tasks当前状态](<tasks.md#当前状态>)；需求见[spec](<spec.md>)，授权只见[统一记录](<../001-host-workspace-foundation/authorization.md>)。本plan拥有设计与验证义务，不维护另一份当前进度。

<a id="design-guide"></a>

## 设计阅读入口

**概要**：对象字节先就绪，再由原工作区提交owner一次确认完整资源版本；正式内容/receipt与派生投影分离。保留001能力，新M2只显式新建，安装预算演进另走显式维护，不让open/inspect变迁移器。

| 阅读问题 | 现行设计/追踪入口 |
| --- | --- |
| 固定技术与宪章约束 | §2有限profile、§3 Constitution Check；参数/逻辑计费不等于物理盘满证明 |
| 组件与独占责任 | §4.1：A共享核心/SQL/公共契约，B原生对象/投影适配器，C独立消费者/外部验收；共享文件只A集成 |
| 接口、依赖与取舍 | §4.2兼容/显式安装维护 → §4.3对象/正式提交 → 后续历史/投影/生命周期设计；[契约](<contracts.md>)与[数据模型](<data-model.md>)仍为原入口 |
| 实施顺序与验证 | [原任务](<tasks.md>)按依赖从技术门到业务与独立Host；本页§5验证映射、§6最终覆盖连到证据，历史增量不另当当前待办 |

选择单owner/原key调查是为了保留提交与unknown归属；投影再生不重存、unknown不自动返额。实际完成及真实ENOSPC/完整物理峰值deferred、race skipped等限制只以tasks当前段和最终矩阵为准；不因本次文档维护继承旧验收。

## 历史执行摘要

<details>
<summary>展开截至R70的原执行汇编（历史，非当前设计摘要）</summary>

下行是截至R70的历史增量汇编，内部“当前/仍待”仅对应各轮截面，不覆盖tasks当前状态。

**Date**: 2026-10-20设计 / 2026-10-23执行与设计反馈 · **Status**: partial / R70已接通当前正式version投影health并按工程范围完成T016：同一原owner/运行ticket在正式对象及SQL只读事务实际结束后，选择所请求version的最新已记录generation并以原native Gate冻结/全hash/首次Close；只反馈派生health、不改正式内容/receipt/Head或计费。已知物理结束的纯投影观测失败为unknown而非正式读取失败；当前授权/原owner/物理unknown仍拒绝并保留应持资源。508输入333焦点三轮、四包完整normal/tag、两种vet/diff均0、SourceMismatch=[]；实际文件篡改/缺失/再生成及同进程新epoch重开已验，单点Validate/Close故障与独立Host验收不代签。T017–T029未完成，003 T006未授权；证据见research R70。以下R69为历史切片：R69已完成T030–T033保行为首片及对应证据：原physical child私有共用WriteFrom/error/length/Flush/Identity/首次Close，未移动freeze/ready/unknown/最终owner结束；504输入323四包完整normal/tag、两种vet及相关native回归均0，SourceMismatch=[]。首片不代签T016当前health、V201–V209或发布门；003 T006未授权。以下R68为历史切片：R68已在原Gate/同一只读事务接通projection原键完整metadata及原HTTP GET/known辅助调查；精确原actor/command/source/charge，不执行生成、退款、原SQL/native结束或SAVE清理。实际accepted-pending及healthy但held两状态的fresh SQL view均不报drained，先于cleanup拒绝；当前独立DB operator+native token已知metadata查询通过。493输入322受影响SQL/workspace/installation/HTTP完整normal/tag、两种vet/diff均0；T016仍待正式version当前投影文件health读取，003 T006未授权。以下R67为历史切片：R67已把冻结POST明确version/projections入口接到原Generate链：当前主体/路径身份、原键优先、独立generation和health DTO，实际历史v1在当前v2时仍不追latest，403在body前。491输入318当前HTTP完整normal/tag、两种vet/diff均0；低层314未变证据复用。T016仍未勾选：正式资源的当前投影health读取及projection原键辅助查询待实现，不把POST原结果当当前文件可用证书，003 T006未授权。以下R66为历史切片：R66已把新M2正式confirmed后的独立投影接到同一原OpenOwner/mutex/运行ticket/分配，不递归新增队列或重放SAVE。正式receipt/status保持不变，M1/原键/拒绝不生成；已知failed或容量拒绝不伪造SAVE unknown，辅助unknown/异常保留同一物理owner/ticket。488输入314受影响workspace/installation/HTTP完整normal/tag、两种vet/diff均0；实际原生自动manifest及旧SAVE键不再生成已验证。T016仍待产品/HTTP显式再生成及health交付，003 T006未授权。以下R65为历史切片：R65已在原Gate及同一native adapter增加正向已知结束的CREATE_NEW目的地冲突health：只记录failed/rejected及有界metadata费用，不覆盖/收养/删除既有文件；原键冷开只查，新键独立generation，unknown或已进入ready callback绝不借此退hold。484输入310当前SQL/workspace/installation完整normal/tag、两种vet/diff均0；SQL synthetic end与真实native primitive冲突分层记录，不宣称公开generation注入的组合证书。T016仍待postcommit及HTTP/产品再生成接线，003 T006未授权。以下R64为历史切片：R64已在原OpenOwner/队列/普通分配/原Gate上接通显式正式version的原生CREATE_NEW projection：完整派生manifest写/Flush/同FileID全hash/冻结至SQL结束/已知Close后结算，真实三代（历史v1两代、零附件v2）及排空重开原键只查通过。已修复已知完成后晚撤权/取消丢失原键的冲突：仅私有cause标记保留denial索引，不误持有unknown ticket或泄露结果。481输入306受影响SQL/workspace/installation完整normal/tag、两种vet/diff均0；T016仍待known失败health/显式处置、postcommit与HTTP，003 T006未授权。以下R63为历史切片：R63已在原Gate/同一Conn完成显式已提交version的独立projection generation SQL切片：完整确定性manifest/source digests、原键只查、pending→healthy与已知physical-return后结算；unknown/rollback/异常控制流保留完整原事务及hold，不重放。477输入298的受影响SQL/installation完整normal/tag、两种vet/diff均0；297五个父例各三轮通过。尚未接原生CREATE_NEW投影、known失败health/显式再生成、postcommit与HTTP，T016仍未勾选；T001–T015完成，003 T006未授权。以下R62为历史切片：R62已把T015 current/exact完整资源、metadata keyset页/cursor和正式附件下载接入原HTTP/Host鉴权及唯一request claim；先原生完整hash后发头、逐块当前权限，已开始的binary错误只abort不拼JSON，原finalizer仍在实际Close/Flush后退claim。471输入291当前HTTP/Host完整normal/tag、两种vet/diff均0；未变下层287/279证据复用。T015按原任务范围完成，T001–T015勾选；T016投影health仍明确unknown，独立Host/完整V202与实际CloseHandle故障未代签，003 T006未授权。以下R61为历史切片：R61已接通精确正式version/attachment解析的原Owner受控下载流：仅所选对象同句柄完整hash后调用同步consumer，私有64KiB块在实际Read前后当前鉴权/ctx验证后才复制，callback结束实际join并撤销reader。469输入287当前受影响workspace/installation完整normal/tag、两种vet/diff通过，未变SQL/HTTP复用既有证据；284纯反例修复post-read零计数仍泄漏caller buffer及Goexit提前退原ticket。T015仍未勾选，尚待HTTP资源读/页/流接线及相应交付验证，003 T006未授权。以下R60为历史切片：R60已在同一原Gate/一个只读快照及原OpenOwner当前读权限下接通metadata keyset页，默认20/最大50、limit+1 sentinel与固定首读upper；同一原owner的临时HMAC cursor绑定完整身份/主体/目标/epoch/upper/位置，不是权限或跨请求事务快照。463输入279当前受影响SQL/workspace/installation完整normal/tag、两种vet/diff均0；metadata不读取title/body/附件/对象，坏v1物理字节不毒化metadata页。T015仍未勾选：正式关系下载流、逐块鉴权及HTTP尚待接线，003 T006未授权。以下R59为历史切片：R59已在原Gate/同一只读事务、原OpenOwner/ordinary allocation/native lease下接通完整资源current/exact版本读取：逐项核对所选正式version/receipt/manifest及ready对象绑定，再逐一全hash冻结全部对象、已知排空后交付，旧v1损坏不污染无关好v2。458输入274的受影响SQL/workspace/installation完整normal/tag、两种vet/diff均通过，旧451输入完全未变；pure unknown反例272修复为保留原ticket/生命周期，非原生Close故障证书。T015仍未勾选：metadata keyset页/cursor、正式关系下载逐块鉴权及HTTP尚未接通；T001–T014保持完成，003 T006未授权。以下R58为已完成T014历史快照：R58已补齐原HTTP/1 Host listener与Handler原鉴权/唯一claim之间的有界全帧metadata观察，严格原始重复关键头/TE/CE不预读body；451输入的实际native/raw TCP、keepalive/chunked/trailer、opaque payload、旧key HTTP1.0交付及HTTP/Host完整normal/tag/vet/diff通过。M2零附件旧文本只映射公共wire kind而保持RESOURCE持久域，confirmed-delivery 202保持原key相对Location。T014原任务范围已完成/T001–T014勾选；T015/后续Host矩阵、原生CloseHandle故障与003 T006未代签，未变下层259证据复用。以下R57为历史快照：R57已在原workspace-operation入口补齐object-put精确原键GET与已知operator元数据查询；同一Gate/DB只读核对原actor/input、transfer及charge，普通查询不执行/结算/退额。fresh accepted/unready及committed-held不得继承drained，SAVE调查在任何SQL/native结束前拒绝待定object。446输入259的SQL/workspace/installation/HTTP完整normal/tag、两种vet/diff均通过。T014仍未勾选，raw等值重复CL/被移除TE入口约束未签收；未新增object物理调查/Host矩阵或003 T006。以下R56为历史快照：R56已在原HTTP鉴权、唯一request claim和原native/SQL owner中接通对象binary上传、完整资源预览与正式CommitRecord/原键receipt的实际TCP最小链；不缓冲/预hash/body限长假EOF、不另设配额。444输入253当前受影响tag及vet、installation normal通过；未变SQL/workspace/HTTP normal复用252各自实际通过项。T014仍未勾选：Go预处理后的等值重复Content-Length/被移除TE不可冒充raw ingress严格证书，object/resource辅助原键查询仍未接通；完整Host矩阵及003 T006不代签。以下R55为已完成T013历史快照：R55已在原operator调查链完成实际原冻结集合首次结束、原SQL实际结束后同一lease下fresh全量原键证明与精确hold结算；普通元数据查询不结束资源/不退槽，原lease仍待显式Drain释放。440输入244当前SQL/installation完整normal/tag及两种vet/diff通过，未变workspace/platform复用既有证据。T013按原任务范围完成，T001–T013勾选；T014+、完整V203、CloseHandle故障及独立Host矩阵未代签，003 T006未授权。以下R54为历史快照：R54已将M2正式保存接入原CommitRecord/OpenOwner/队列与原Gate，逐一native全hash冻结完整清单贯穿实际SQL结束，known排空后结算；实际双会话同基线并发恰一正式效果、receipt/同opener重开及原key通过。发现并修复原key空新提示被前置拒绝、元数据误称物理结束而释放普通槽位；健康新key仍保持原M1严格校验且无storage IO。437输入最终239受影响normal/tag及vet通过，SQL normal复用237未变部分。T013仍未勾选：保留依赖的正向实排空/fresh调查收尾及后续Host/真实故障矩阵未代签；003 T006未授权。以下R53为历史SQL切片：R53已在同一原CommitGate/连接/单次Admission落实M2正式提交SQL切片：原SAVE/1 key先于新候选/epoch/预算，持久accepted与hold、完整version/引用/Head/receipt/Audit同事务，正向冻结回调结束后结算；失败旧键读取、未获正向物理结束及cold保留hold不推断拒绝/退额。434输入228的SQL/installation normal/tag与vet通过，最终230三个私有SQL路径修复仅重跑SQL焦点三轮、完整normal/tag与vet通过；其他层复用未变证据。T013仍未勾选：原native正式保存消费者接线和后续真实故障/Host验收未代签，003 T006未授权。以下R52为已完成T012快照：R52已在同一原OpenOwner/队列/ordinary allocation/native Lease与Gate接通完整资源预览；SQL派生全部ready对象并逐一同句柄全hash冻结，整个集合贯穿实际SQL结束，已知物理排空后才结算，unknown保留原owner/hold。零附件M2旧文本入口使用RESOURCE域，已有附件目标与查询失败均拒绝，不回落空清单。429输入219的workspace/SQL/installation焦点三轮、完整normal/tag、vet和diff均0。T012按原范围完成，T001–T012勾选；T013正式提交、后续HTTP/完整V202未代签，003 T006未授权。以下R51是历史SQL切片：R51在原CommitGate/连接新增完整资源candidate SQL切片：规范全清单、RESOURCE/RESOURCE-CANDIDATE域、全部ready对象证据、同事务slots/字节与双行持久化、物理unknown/cold/TTL依赖围栏；425输入214受影响SQL/installation normal/tag及vet、最终216仅SQL修复增量normal/tag及vet均0。冻结集合仍是合成回调，尚未接原OpenOwner生产预览/native冻结与M2旧文本适配，T012保持未勾选；T001–T011范围已完成，003 T006未授权。以下R50为已完成T011快照：R50在原OpenOwner/队列/ordinary allocation/native Lease/SQL Gate完成T011真实对象传输及冷开调查围栏；同FileID全hash冻结贯穿ready SQL，只有正向物理结束后才结算。421输入206四受影响包normal/tag及vet、最终210两受影响包normal/tag及vet通过。T001–T011按任务范围完成；T012及后续候选/正式提交/Host验收未代签，003 T006未授权。以下R49是历史SQL切片：R49当时已在同一原CommitGate/连接接入T011 SQL传输协议切片：新key的accepted/hold与unready原子持久化，单次冻结证据回调中验证精确length/hash/FileID及当前权限后记ready，只有回调实际返回已知结束才在原Gate事务结算；旧key只查、无物理回调。418输入下SQL完整normal/tag与vet、既有native准入基础三轮通过；新增冻结证据仍为合成值，生产native传输适配/unknown调查及完整T011未完成，T011不勾选。以下R48为已完成预算基础快照：R48已按原任务图完成T009预算/原准入记账基础，T001–T010已勾选；同一原事务新增有效候选slots/字节共同约束，193的413输入当前SQLite完整normal/tag及vet通过；196的416输入对新增当前卷磁盘条件观察与预算调用接线，受影响platform/installation/HTTP完整normal/tag及vet通过。磁盘观察不保证物理写成功。T011物理消费者、T012候选生产及T025完整Host V206仍按后续依赖验收，不倒置为T009新前置，也不计passed；已有分配时任意profile重投影仍unsupported，不冒称通用编辑能力。R47当时已在真实注册M2目录的原OpenOwner、原Lifecycle、原native lease与原队列接通冻结ObjectPut的持久accepted/hold：当前实际DB writer权限与安装分配只在原key缺失后参与新准入，调查保留同一ordinary allocation，旧key不要求新epoch或assignment、撤权不披露；结果仍是数据而非物理执行capability。412输入下受影响SQL/workspace/installation焦点、完整normal/tag及vet通过；T009仍未勾选，后续物理消费者、已有分配profile兼容投影及完整V206不由本轮替代。R46当时已在原CommitGate/原连接拥有一次事务，冻结ObjectPut输入后按原业务key优先读取、仅新key读取当前安装assignment并原子提交accepted意图与完整hold，晚权限失败实际回滚；旧key/冷开不生成新实体或计费，accepted仍围栏而非执行capability。409输入下仅受影响SQLite焦点三轮、完整normal/tag和vet通过；原native lease/OpenOwner适配、物理消费者及完整T009仍未签。R45当时已允许无旧工作区分配时的有限profile与总额/新reserved行在同一原认证CAS中提交；规划器与SQL写入器共用effective profile，并保持当前权限/原key后、profile资格前的旧revision冲突优先。407输入下178受影响SQL/installation/HTTP normal/tag及vet通过；最终183仅两文件SQL优先级修复增量，当前SQL+HTTP焦点三轮、SQL完整normal/tag及vet通过。已有分配的profile重投影及完整T009仍未签，T009未勾选。R44当时已在原CommitGate调用者事务绑定资源业务原key与持久hold，并复用原SAVE/resource拒绝索引及M2冷开围栏；405输入下受影响SQL/workspace/installation normal/tag及vet通过。仍不是T011物理传输、公开M2准入或完整T009。R43为当前workspace.write身份的目标安装分配投影历史快照，403输入下SQL/installation/workspace normal/tag及vet通过，数据投影本身不是跨库原子提交或权限。R42当时为同一原CommitGate事务的区内预留/结算/unknown记账基础，399输入的SQLite normal/tag及vet通过；不是生产M2准入、当前安装分配授权或完整T009/V206。T001–T008固定profile局部实施与验证完成；G204物理发布子项deferred，不计pass。T008公开预览/确认、原kind GET/reconcile与独立normal Host已知升级重启已实际验证；37轮313输入及normal/tag回归0。40轮新增真实事务中断后，正常新Host因原安装journal门拒绝ready，release正控通过，中断恢复failed；后续全仓步骤未执行。该历史失败随后由R33–R34真实显式冷恢复补齐；T008现按本项范围完成。T009已进入total-only SQL切片：当前operator、总量CAS、精确原key/冷重开、完整committed历史校验与unknown份额不退已局部验证，现按R36在既有Store接通当前operator预览确认/原key/冷开服务，383输入受影响SQL/installation normal/tag与vet为0（R36历史服务快照）；R37已接冻结HTTP总量GET/预览/CAS及原key GET/reconcile，386当前输入下受影响HTTP/installation normal/tag与vet为0；profile/每区调额显式501；R38已测预算事务九次真实FSYNC unknown、原allocation保留及实际关库后新身份原key调查（正控完整new、故障完整old且不重执行），R39已在同一认证CAS中支持未物化新UUID的静态bytes/slots reserved分配及原key冷重读，391输入下受影响SQL/installation/HTTP normal/tag与vet均0；既有分配仍精确保留；R40为M2保留事实与物理冻结的393输入历史快照；R41已在同一Store原操作allocation、排序嵌套的原Lifecycle.WithClosed及连续native pins下接通已注册M2既有分配的fresh读取→同一认证CAS→实际source排空、原key和冷开；397输入受影响SQL/installation/HTTP normal/tag及vet均0。profile修改、完整区内持久预留/独立预算Host矩阵和完整T009仍欠；T010原物理端口按当前代码局部复验完成。379输入下受影响SQLite/installation/platform normal/tag及vet均0（R35），不是V201/V206或完整T009。显式冷恢复已依统一授权进入分段实现与测试：已贯通只读来源→有界私有副本→完整old/new及当前撤权事实证明，并完成获批准的同值metadata事务SQL组件；103轮360输入、受影响五包normal/tag及vet为0；已有副本精确回收、真实内核调用者受限投影、当前副本operator只读校验，以及同一原lease下冻结转写桥、真实SQLite句柄结束登记和原journal精确处置组件。生产连接专属VFS已接既有固定profile/Finalizer，在合成schema1/2执行一次同值metadata事务、完整事实和普通新连接复验，并补18次真实FSYNC故障的unknown/零proof/完整owner保留（R30）；这不是生产恢复闭环。新增显式预检owner已持同一原Store排他lease及其唯一普通allocation，在有界副本上完成实际内核调用者/当前权限预览、短期nonce和实际scratch回收计费组件；109轮363输入五包normal/tag及vet为0（R31历史预检快照）；现已贯通实际内核调用者下单次Confirm、原库一次等值事务、实际结束后新原库连接完整证明及最终排空（R32）。112轮365输入五包normal/tag与vet为0；仅新增故障测试后114轮366输入按影响范围验证安装包normal/tag及vet为0，确认链18次真实FSYNC失败保持unknown、完整owner和原allocation，不重执行。新增recover-installation显式入口及实际原升级COMMIT前进程中断→拒绝确认不写→确认恢复→独立normal Host/新认证/M1历史闭环，118重建四exe首次通过，119在相同370输入/四exe上三轮已知与中断闭环、受影响cmd/installation normal/tag及vet均0（R33）。旧40失败夹具原样保留。R123/124在374输入及四个新建实际exe下补齐新增维护事务COMMIT前固定进程中断/正常release正控、同一原库新normal CLI调查确认及独立normal Host/新认证/原key/M1事实；SQL normal/tag及vet、相邻三类闭环各三轮与指纹核对均0（R34）。T008局部实施/验证收口，T009与可独占文件的T010依赖已满足；不代签完整V207/T026或全部FR/SC，不放宽普通启动、当前权限/unknown或运行容量。最终业务/发布验收未签。
**设计边界**：原Q1–Q3不变，容量及窄安装演进按已批准需求执行。真实盘满/完整物理上界延至发布/相关支持声明前验收，不以核心实现代签；每次设计/任务修订后先做受影响的只读analyze，不借历史无阻断结论跳过新检查。

</details>

## 1. 总结与范围

在原独立Go Host的同一授权/运行容量/工作区队列/正式提交owner中增加本片完整文本附件版本、不可变对象、精确历史读取与提交后清单投影。保留001文本目录及正式历史；M2只对显式新建format3目录开放。为了持久预算，另做用户已批准的仅安装schema1→2显式维护，不升级工作区、不做通用migration平台。

一次确认是**单资源完整一版**，不跨资源/跨库原子发布。对象ready≠正式引用；正式confirmed≠投影ready；取消/断连≠物理结束；缺receipt≠可重试。对象存在/投影/缓存均不是第二事实源。

前序最小校准已完成：scope/migration-baseline补独立AI内核与领域待确认落点；不改M0–M8唯一路线。文本附件只是本片验证载体，RESOURCE/1不是Agent/Workflow/Knowledge/页面的领域公共schema；不新增空平台/泛型框架。M3 Apply/Build/Studio、M4插件、M5 Provider/Run、M6前端、M7通用迁移/备份/清理、M8跨平台均不实施。

## 2. Technical Context 与有限支持profile

| 项目 | 计划约束与证据性质 |
| --- | --- |
| 语言/平台 | 既有Go1.27.2、windows/amd64、本地直连NTFS、单安装；不升级工具链/下载依赖 |
| 持久化 | 固定SQLite3.53.4/现有driver与项目内Windows Flush错误映射兼容profile；DELETE/EXTRA、实际错误传播与unknown门继续验证 |
| 实现结构 | 原cmd/Host/installation/workspace/sqlite/http/protocol/digest；对象原语是窄platform适配器，无新数据库owner/全局当前区 |
| 协议 | API v1增量、唯一OpenAPI/生成模型；旧域/向量不变，新域一份固定bytes/hash及Go/Python独立消费者 |
| 内存/传输 | 对象64KiB块，禁止对象ReadAll；JSON仍1MiB。63普通operation的直接流缓冲约3.94MiB，不是总RSS/SLA保证 |
| 容量产品参数 | 唯一技术参数表在[research R4](<research.md#r4--已确认恢复方向与待评审有限产品profile>)：默认16MiB/最大64MiB对象、默认32项64MiB/最大64项256MiB一版；默认1GiB/最大16GiB区分配、默认8GiB/最大64GiB安装总额。**拟议待评审，未测支持声明** |
| 运行容量 | 沿原区32/安装64、ordinary31/63及控制reserve，不因对象/分目录再开池；真实join后释放 |
| 测试工具 | 现有tools/go.ps1离线串行runner，Go单元/真实SQLite/Win32/外部Host与helper；Python契约/字节/链接检查；仅合成数据 |
| race | 固定CGO0无法运行；如仍不满足条件明确skipped/unverified，不以无race编译代替，不偷偷启CGO/装C工具链 |
| 不承诺 | 任意磁盘/Windows/硬断电保证、零故障SLA、恶意代码OS沙箱、跨安装导入、无限保留或无GC永不写满 |

默认与最大为有限工程profile及边界目标，不是本次用户实测需求；改变产品值要同步参数/契约/持久配置及边界夹具。1MiB只可为隔离small-profile测试值，不能成为生产默认或用小夹具代替真实默认/最大边界验证。

## 3. Constitution Check（设计符合性，不是通过技术门）

| 原则 | 设计落实/必须验证 |
| --- | --- |
| INV-01–05 | 同一正式owner、显式区/tenant/principal、稳定身份；DB正式版本/引用、对象精确字节；安装分配/投影不是第二保存器 |
| INV-06–08 | inspect零源写/open不修复；001旧SQL/ledger不改；真句柄排他、祖先/文件ID、拒别名/reparse/hardlink；复制不获权 |
| INV-09–14 | 当前确认/基线、原key持久唯一结果、fresh调查不重放；对象先验证后引用；投影分离不覆盖；不称跨文件硬原子 |
| INV-15–17 | 直接对象IO真实pending与join、close排空全部资源/unknown，保留B独立操作/原close调查；不以ctx取消强解锁 |
| INV-18–21 | 每次/每流块当前授权、秘密脱敏、单安装32/64与有限持久预算、当前operator明确确认维护；不扩执行权限 |
| 四层隔离 | 正式对象坏依赖精确围栏；无法界定则A新写封锁；独立B继续；共享授权/安装unknown标安装scope并阻断依赖 |
| 治理/证据 | 无新增宪章例外；schema演进为用户已批准的窄边界而非隐式upgrade。历史正式analyze及G证据见research各轮记录；当前状态只见tasks，新修订须受影响只读分析；G204物理发布项明确deferred、不计pass，宪章VI.4共享盘满边界不变；局部green不代替核心全门/用户验收 |

若门证明需要扩大权限、改变Q1–Q3/单owner或超出固定安装演进，停止并提出差异，不静默fallback/降级规格。每次设计改变重新核对本表。

## 4. 结构与关键设计

### 4.1 文档与将来代码边界

本片仅[spec](<spec.md>)、本plan、[research](<research.md>)、[data-model](<data-model.md>)、[contracts](<contracts.md>)及[tasks](<tasks.md>)六个权威层次；不另建竞争路线/决策日志。旧001授权记录仍为唯一当前授权owner。

未来代码落点：A负责原storage/sqlite、installation、workspace、transport/http、protocol/digest；B独占新增platform/windows对象/投影窄适配器及其本地测试；C独占新外部acceptance及独立契约消费者。已有共享文件/SQL/migrations/工具仅A集成。具体落点见tasks；T002契约/双消费者和T003隔离实验、T004窄原语及T005可行性夹具已有局部源码，不表示后续业务链已接通。

### 4.2 profiles、安装演进与兼容

安装schema1普通兼容模式继续001功能，只能显式进入1→2维护；新安装init显式构造2。维护需可信原生认证/当前安装operator、无副作用preview、原key与精确源/目标ledger、关闭并实际排空所有workspace/IO/流/DB owner、持续安装排他。旧SQL保留，新表/迁移条目/升级原key结果同一安装事务。fresh证明完整旧状态/完整新状态与原key归属；不完整保unknown，不自动重放或换库，详见R1。

001工作区始终format2/schema1，M2命令unavailable。新M2 format3/schema2完整genesis；两者不能交叉套SQL/字节协议。原意图profile恰一恢复、create/register/relink/reconcile均验证绑定；M2外来安装导入不开放。兼容不依赖另跑旧binary。新M2旧文本路由只服务零附件资源，同一gate、不丢附件；旧client先识别capability。

### 4.2.1 安装中断后的显式冷恢复引导

本节保留设计约束；下面R23“未实施/未签”是历史截面，后续实现与真实中断闭环见[research R33–R34](<research.md#r34--新维护事务自身中断原库fresh闭环与t008局部收口implemented--fixed-profile-verified2026-10-22>)，不等于V207完成。当前状态只见[tasks](<tasks.md#当前状态>)。

**历史问题及R23当时边界（非现行进度）**：实际40轮在升级事务已写入、COMMIT前固定子进程中断；正常新Host因未证明的安装journal拒绝ready。37轮已知提交后的独立Host正常重启通过，不能替代此失败。最初先完成设计评审，现依[统一授权](<../001-host-workspace-foundation/authorization.md>)推进实现与测试；低层前置的实际证据见R23，不代表原库恢复入口已经完成。保留普通启动的sidecar拒绝，不删除journal、不把只读原生precondition扩成写入口。映射FR-026/SC-013、T008及后续V207，不延期或代签fresh义务。

1. **独立且显式的入口**：同一Host binary的`recover-installation --config <trusted-config> --request <original-upgrade.json>`，与普通serve/open/inspect分开。config四字段及原公开UpgradeInstallation DTO不变；请求最多8192字节，身份只来自已验证原owner。先经实际保护pipe/current caller交付六字段recovery_preview（无nonce/session），仅在随后收到单行`CONFIRM <operationId>`才重新取得实时native caller执行Confirm。拒绝/EOF已知结束后exit2、recovery_declined；未知结束不得当已知拒绝，exit1并保留完整error owner至进程结束；只有原owner实际关闭后才recovery_completed/exit0。不启动正常业务HTTP、工作区或会话产品，不自动重启/初始化/补种，无备用pipe/路径或新认证协议。未完成genesis、未来/坏schema或不能证明原安装的残留不冒充升级恢复。
2. **先固定来源，不先修原库**：原Store/安装owner取得真实安装排他，固定bootstrap、原DB/journal及祖先的原生身份，检查非reparse/非多链接及有界输入。前一物理owner必须已结束；PID/锁内容不足以证明。原库尚不通过SQLite writable open；通过同一批受控句柄取得稳定、有界的私有验证副本，记录全输入关联与来源指纹。副本只用于重建可信视图，不登记成工作区/新安装，不签发普通业务会话或成为第二正式事实源。
3. **验证副本再核当前权限**：只在副本上让固定SQLite处理其journal；核实精确bootstrap关联、完整编译schema/ledger、原安装/tenant/principal及当前installation.operator、原升级key/精确输入和typed结果。原生实际调用者身份与上述当前权限两者都要成立；同SID/DPAPI解封、自报principal、旧bearer/receipt或原key本身均不授权恢复。副本无法得到可信视图即停止，不能用“恢复需要权限，所以先给权限”循环授信。
4. **预览不报告恢复成功**：仅交付拟恢复范围、原key/输入、来源代次、可证明的old/new或unknown分类及其风险；绑定短期、单次、仅内存的确认上下文与当前操作员/授权版本。副本中的成功不当成原库操作结果或原库恢复已完成，不能使普通业务可用。源码/源文件身份或内容/当前权限/原输入任一变化，原预览失效。
5. **显式确认才接触原库写路径**：再次核验原来源/当前权限/排他及所有阶段的物理结束，才由原安装owner进入窄的SQLite原journal恢复。对于经严格cold判别及完整事实证明的zero12残留，用户已在统一授权明确允许一次同值user_version维护事务：保持DELETE/EXTRA，事务内再核完整schema/ledger、当前operator/授权版本和原key-input/typed结果；仅赋已验证的当前1或2，不改变业务/schema/原结果。它是新的物理事务，不冒称原事务playback或无写入，须验证真实故障/中断/unknown及实际join后的原库fresh。其余不重新执行升级DDL、换key、复制副本覆盖原库或绕过原操作owner，不临时转换journal模式或手删残留。需要从冻结读取切换到写句柄时必须先有真实Close/join，再核对原生身份及来源指纹；不能把关闭未知当切换成功。
6. **原库fresh证明后才恢复相关功能**：原恢复调用/回调/句柄实际join后，在原安装验证完整旧schema/ledger且原效果未录入，或完整新schema/ledger且原key/actor/输入/结果精确相符；缺key、0/0新genesis或有部分新表均不能单独证明成功或安全重试。不能证明仍unknown，保留原owner/证据/围栏；不清标志假装可用。原预览不恢复发送权，原升级命令不自动重放。恢复完成也不复活旧会话；正常业务要另经正常启动及新当前认证。
7. **有界性与实现前未证项**：所有阶段复用原安装OperationCapacity/物理owner，不开第二许可池；副本/metadata/控制记录必须有有限输入及临时资源界，失败/unknown不得靠重试累增。来源未可信前如何证明可用控制余量、恢复认证投影的最小读接口、源句柄切换竞态和原库实际恢复后的双重鉴权均须在实现前技术证明；不能先透支再补账、暗加独立预算/运行时/系统策略或声称任意安装都可恢复。无法满足则明确unavailable并回报，不以设计获批代替可行性。

**原设计验证义务及R23历史未验项（后续证据见R33–R34）**：实际before-COMMIT中断与已提交正控；验证阶段原库全清单/字节不变；不同身份/撤权/原key-input错配/未来与坏ledger/未完成genesis/源替换拒绝；原恢复失败及Close未知保留全部owner；完整old/new/仍unknown三态；原001工作区字节/历史不变；全流程零自动DDL重放。原库恢复全链仍unverified；已实施并验证的只读源/有限输入/私有SQL证明见R23，未执行原库journal恢复，不将组件通过代签本段全流程。临时副本不是M7备份/迁移，真实盘满/完整物理上界仍单独deferred。

### 4.2.2 T019公开M2创建的原owner/profile/预算接线

implemented / R89已完成SQL与公开HTTP局部链及604输入normal/tag/vet，未代签T019完整兼容或独立产品Host。以下保留设计约束。

Owner A沿原T019（依赖T008/T009/T017/T018）及FR021–026、SC010/012/013局部，先补公开新M2创建，不动001目录、旧SQL/INSTALL-OP/1或唯一OpenAPI。当前已有format3/schema2 genesis、INSTALL-M2/1向量、默认分配字段及原创建owner，不能以这些组件冒称HTTP已开放。新增请求仅转交已冻结的可选profile；缺省/显式m1保持原行为，schema1安装对M2明确unavailable，未知profile拒绝。

1. `CreateWorkspaceRequest`携显式profile，仍由原Store-key owner生成首次UUID并持久冻结。原typed创建意图由UUID/tenant/identityDigest和相应INSTALL-OP/1或INSTALL-M2/1恰一匹配已知profile；M2摘要绑定原displayName/路径/3/2/1等全部冻结字段。旧key先查当前权限及原意图，再比所请求profile；不因缺目录/旧epoch猜默认、不换UUID、不重放创建。普通安装operation查询保留001的INSTALL-OP/1公共分派行合同（不新加强制typed join）；M2的独立摘要必须由同一typed意图恰一验证，缺typed行/域不符拒绝，不把M2套旧域或猜profile；随后原业务owner仍校验typed事实与当前路径政策。其余register/relink不改域。
2. 新M2意图在原安装事务、触目录之前，从当前已验证有限budget/profile取得WorkspaceDefaultBytes/CandidateSlotsPerWorkspace，检查总承诺/名额/元数据控制余量、原disk guard及原namespace；原intent、静态workspace_allocation、预算revision及保守控制元数据charge原子提交。分配关联同一create原key，不开预算adjust第二key/owner，不自动扩大总额。计费涵盖原意图、分配及后续有界阶段/初始grants元数据：固定逻辑单位按17次正常变更＋至多2次单调interrupt共19次预留，encoded字段另计；同结果interrupt在当前授权/原意图检查后只读返回，unknown不降级。保守承诺在失败/unknown/关闭后保留，不自动回收。原key重查不再计费，M1不新增allocation。已有调额/新安装总量/CAS规则不放宽。
3. 原创建物理链按已绑定profile构造descriptor及genesis；M2才由原FreshWorkspace准备完整布局，校验及pending reconcile采用相同profile，已提交reconcile仍验证实际完整历史。不改原native Close/unknown/排他及阶段owner，open/inspect不升级、补种或发额度。首次open后的原writer只投影已持久承诺，不臆造默认分配。
4. 先只读spec/plan/tasks分析，再写正常构建可编译的SQL/HTTP正反测试，原实现对新M2真实501/拒绝为有效red；不是compile fail。SQL验恰一域/原key/单次分配与charge、容量不足回滚零行、schema1拒绝、M1不分配；HTTP验默认001仍可用，M2公开创建→原key查询/错profile→open→零附件旧文本及附件完整资源实际保存/读取/known关闭。旧contract-only拒绝测试仅在本片真实实现后改为001能力边界。跑受影响sqlite/installation/HTTP normal/tag及vet、产品构建/隔离；来源与失败均保留。本片不是完整T019/V207/独立产品Host验收，登记/移动及坏目录/完整重启差集仍留原任务。

### 4.2.3 T019独立普通Host的M1/M2兼容及M2登记/移动

状态：implemented / [R90](<research.md>)605输入普通产品局部验证，非整项T019或V207签收。下列是该片实际设计，当前残余见tasks及§4.2.4。

Owner A/C串行沿T019、US6/FR013/021–022/024–026、SC010/012/013局部与V207补公开差集：复用现有acceptance外部CLI harness和真实protected-pipe session/precondition，以当前源码重新构建的普通Host先公开创建默认M1和显式M2、保存非空文本；M2再由真实HTTP上传含零字节/CRLF的附件并确认完整第二版。原进程正常interrupt/drain/exit后，以不同PID/birth及新认证的普通Host读取原M1、登记已知M2原root、受控同卷rename后显式relink，再open读取原M2两版/附件/原key。第三普通Host再验证持久定位、旧key零重放/零双分配，旧路径不重建；仅在已验证owned合成root且workspace closed时移动，不写库/伪造descriptor/复用旧进程或会话。

M2登记/relink已经支持真实format3，不为假设缺陷改产品。原未知/未来/缺库零补种及外来M2受限拒绝按现有Store/schema测试的实际输入和受影响回归复用；若发现真差集另补原T019，不重复T008。新test的有效负控只把合法M2创建改为合法默认M1，要求真实format/profile断言失败，而不是网络/编译失败；随后普通Host焦点三轮、既有M1独立smoke/登记移动回归和acceptance vet、指纹/脱敏核对。此项不是T020–T027完整矩阵，不代签002全验收或物理pending九例。

### 4.2.4 T019 context及open/create的可信profile/version投影

状态：implemented / [R91–R92](<research.md>)605输入完整受影响层及606输入实际schema1/closed源边界、当前同SHA普通产品三进程链已验证；当时剩余的非空M2矩阵已由R93补齐并按工程范围完成T019。下列保留本片设计依据，完整独立Host矩阵仍见tasks。

Owner A，原T019/FR021–023、SC010/012局部，依赖R89/R90。复用既有DTO，不生成新契约。`Authenticator`在同Store锁与当前会话/grant检查下，以既有DB `ReadInstallationIdentity`精确读schema：1只报M1，2报M1/M2；unknown、未知schema或读取失败明确拒绝，不用缓存、启动默认或`>=2`。Available列表只代表安装支持，不是operator授权、预算可用或创建承诺。不得持Store锁重入`Authorization.Check`。

open及open GET只从既有可信`Snapshot.Identity`映射精确pair：M1=(2,1)，M2=(3,2)及objectProtocol=1；M1省略objectProtocol。普通closed GET/register/relink、opening/draining/unknown无快照不补造格式、ready或完整性，不隐式inspect/open。fresh及原key重复的committed+RegistrationConfirmed create可从原`DirectoryOperation.CreationProfile()`重验typed身份，补固定profile/version；该事实不证明closed当前文件健康，仍unchecked/none，当前open快照优先。保留delivery授权复验、原key/unknown/实际排空，不因投影失败重放创建。

预计四个生产文件（auth、creation结果、HTTP view、dispatch）及既有测试收紧。先只读analyze，再把当前optional断言改为create/open必填而取有效red；正反覆盖精确M1/M2映射、M1无对象协议、closed无文件读取、schema1/2及unknown当前支持。验证按安装/HTTP受影响层与实际普通产品链，不复用旧exe哈希冒充新产品；未运行项保持unverified，整项T019及Host矩阵不预勾。

### 4.2.5 T019非空M2的原生拒绝矩阵（implemented / R93）

[R93](<research.md>)实际607输入七样本normal/tag各三轮及vet通过，全部源树bytes守恒且没有锁豁免；结合R89–R92完成T019工程，完整T026/V207仍按原图。下列保留已执行的设计依据。

Owner A沿原T019/FR021–022、SC010局部补唯一工程差集，新增安装测试，复用原complete M2 fixture的真实对象上传、完整resource/receipt、原native正控和已知Drain；不以仅genesis或schema2 inspect的unavailable冒充坏ledger校验。各独立样本在原owner实际关闭后，仅在自己的合成root准备未来format、未来schema、坏ledger、缺库及未完成sidecar，保存篡改后的源树基线，再经原Open拒绝且无epoch、无补种/修复，已结束的SQL fixture变更不称产品写入。缺库只移至同owned root备用名、不删除；不恢复后重开、不重试unknown结束。外来租户用第二真实安装、当前operator与显式含源的allowed roots经原Register拒绝，检查无正式registration/grant/intent及源业务bytes变化；实际读锁metadata若有变化须精确列明，不能扫描豁免。先只读分析，再有效去刺激反例与normal/tag焦点、受影响验证；无产品变更时按原输入与产品SHA复用R91/R92，不重复T008或已通过Host链。仍非T026/V207完整产品验收。

### 4.2.6 T026当前普通Host同安装M1升级与M2对象闭环（implemented / R115，T026仍partial）

实际证据见[R115](<research.md>)：655输入，原normal strict seed/完整closed bytes与FileID、当前权限/唯一显式升级、同Host M1继续及公开M2二进制两版、完整正常冷开原key/receipt/bytes均三轮；同build省升级/隐私两个有效red及vet。Positive最终first-end roots已补齐，仅新acceptance与Doc3变化。以下保留设计；原真实升级FSYNC/进程中断/cold恢复仍需当前独立Host证据，不勾T026。

Owner C沿原T026/US6、FR013/021–022/024/026、SC010/013/V207，补原T008正常外部case明确未做的同Host M2差集，不改旧helper/产品/API。严格normal installation helper只在新owned合成root产生旧schema1安装并已知首次结束；新ordinary case经当前protected pipe创建非空M1两版。真实open-owner拒绝升级且原epoch不变；关闭后全源/原FileID冻结，正确preview/显式confirm仅升级安装库，错误ledger/target或伪preview拒绝，原typed key GET/重发不重放。

同一普通Host的新epoch继续M1正式文本并保留原回执，再公开显式创建M2、实际上传含NUL/CRLF对象并两版正式保存。全部正常关闭/停止后第二普通Host重新认证，整个closed工作区源bytes/FileID未替换，原安装key和M1/M2全部正式版本/附件/receipt守恒，M1仍能继续新文本写。既有strict read-write principal helper仅在实际停机后形成真实非operator，安装升级preview/确认/原key读取403，不能借旧结果/摘要授权。正向HTTP复用已验first-end transport，拒绝用独立原wire，closed inventory全文件bounded known-end读取且不豁免锁，日志秘密在所有进程结束后覆盖早失败路径。

先严格只读analyze后实现。正常case三轮与两个同build有效控：只省略真正升级使真实schema1击穿schema2 oracle、实际拒绝观察header放原M1正文使scanner失败；相关vet、当前来源/路径集合/原报告链精确核对后复用未变生产包。旧正常T008不再重复造同等矩阵；真实native故障/进程中断与cold恢复继续复用当前构建既有严格外部case及原SQL/Store组件，逐项标实际old/new/unresolved，不把源码中的分支视为已运行，不新增三态全排列或绕journal门。本普通片不勾整个T026；完整V207差集与原T027–T029继续原图。

### 4.2.7 T026原安装升级writer真实FSYNC unknown与既有中断恢复（implemented / R116–R117，原V207完成）

当前[R117](<research.md>)已完成660输入、原三个中断/恢复各三轮、同build省实际恢复有效red、普通mixed/current vet及原Store首次结束四ordinal；仅两个acceptance及Doc3改变，严格原CLI/普通journal拒开和产品/旧helper权限不变。原T026现完成，以下R116失败保留为当时历史与技术反馈依据，不是当前未验声明。

实际证据见[R116](<research.md>)：660输入，当前DB只读诊断门修复先有真实双preview red，ordinal0/1实际三轮、nofault有效red、fresh完整normal/tag四包与两vet通过。旧case原main-root和helper runtime.Caller限制原样保留，主源集合/内容等同C、两产品复制同SHA且main TEST helper另编译。普通mixed和两旧cold恢复case/释放正控通过，直接cold-open的旧interrupt分支遇保留journal未ready，关联整步failed，不能签T026；以下保留设计与下一片原恢复对齐义务。

R28最小技术反馈修正（designed，C/原T026/V207，FR013/021–022/024/026、SC010/013）：只改旧interrupt验收，不改产品/既有helper。实际pre-Commit中断、Wait及HTTP join后必须有原非空journal；直接复用installationUpgradeNativeColdJournal，在普通serve真实拒绝且DB/J/bootstrap零写后，用原command经现有strict CLI preview/排他/精确CONFIRM证明complete-old，再允许新normal会话。release-positive原路径不变，原closed M1全源/FileID、身份/新epoch、旧SAVE原receipt按值DeepEqual守恒，未知不删J/复制恢复。省此显式恢复的同build负控必须在原普通拒开处失败；当前三旧case与mixed、原Store first-end组件定点复验。普通/diagnostic产品与原helper可按全部未变生产/依赖/定义/源集合及R116报告SHA复用，主源只新增验收修正与Doc3；TEST helper保持main-root原严格条件。无需再做native故障笛卡尔或放宽任何guard，只有原T026闭环全满足才勾。只读复核还发现旧case的原HTTP caller失败路径未join、竞争者异常owner丢弃、checkpoint/inventory首次Close无完整root和两个receipt用指针结构地址比较；本片同验收边界补全原response/caller/竞争者retained、所有接收唯一join及有界known-end reader/closed inventory，两个旧receipt仅改为按值DeepEqual（保全部字段），FileInfo/os.SameFile另证原源身份。只影响验收源，三个受影响旧case都需重验；不修改生成DTO或共享helper权限/业务语义。

Owner A/C沿原T026/V207、FR002/011/013/018–022/023–024/026及SC010/013，最小补R115明确未验的原升级COMMIT返回unknown独立Host，不借预算/create故障。原schema_upgrade.go维护准入及单一ref之后、唯一db.UpgradeInstallation调用处加normal直通/tag窄诊断；原schemaUncertainLocked分类后void报告，不改SQL/key/权限/取消/keep/ref释放。tag必须纯image/config门先于IO，绑定原Store/当前operator/完整INSTALL-UPGRADE输入digest、原DB/pin/lease/schema1、原ref Usage1及零其他owner/inspection/delivery/creation/unknown；同唯一受限VFS工具对精确installation.sqlite(+journal)的第一个原Flush用真实只读Win32句柄BOOL0→1034。完整root先Acquire，disarm+join回调后才首次结束刺激句柄，end未知保留原Store/nativeFailure；报告Write/Sync/首次Close已知后marker发布，不缓存业务授权，不增产品fault API。配置明确八字段（version/Fixture/InstallationID/PrincipalID/OperationID/FromLedgerDigest/TargetMigrationDigest/Ordinal），normal预览提供静态source/target；诊断Host随机preview nonce不可启动前预知，故不以零nonce选择器伪称full-input digest。实际桥只从原Store已验证的当前principal/grantVersion/未过期preview再核完整From/To/source/target/PreviewDigest，报告重新计算真实含nonce的INSTALL-UPGRADE/1，父用此次真正preview和发送command独立计算逐字比较。此为同原输入的只读观察，不缓存授权或替换运行配置。静态复核发现同Store成功升级只删除所用preview，其他1→2 preview可仍留存；因此历史preview仅证明原输入，不证明当前schema1。任何诊断Acquire/arm之前，须在原Store锁/原ref下调用原db.PlanInstallationUpgrade只读核当前完整source/target仍为1→2，旧preview跨过成功升级后必须在该门拒绝；新增实际两个preview→正常成功升级→剩余preview的原Store反例及From/To单坏事实，不改正常产品或旧preview生命周期。

外部独立case由normal严格新schema1 seed与非空M1/receipt/closed全source建立原事实，普通权限/ledger/live-owner不重复铺矩阵；新diagnostic零workspace owner/current session，原preview+confirm后核实际1034、同key/fullinput/ref Usage1、auth fence、native排他/原workspace source未动。ordinal0正控与ordinal1实错；去实际native刺激必须在unknown oracle失败。同key GET/reconfirm/reconcile只报告共享unknown、不清ref/重放/删journal。精确旧helper异常收集仅证明该PID/birth/image结束，不称StoreClose/退款；新普通Host若正常可开，严格全old/no-key拒旧确认重放或全new exacttypedkey；若journal/未知拒开，则保持源，沿既有普通CLI显式冷恢复及原key调查，不能删journal/复制替换。所有当前状态完整identity/schema/ledger与key输入相符才允许后续M1新epoch/旧receipt/bytes，禁止凭缺key证明old。只验实际观测分支，不新增所有ordinal×三态排列或live-owner优化门。

桥/实际Store有效正控与每个单坏事实units、same-build native red→三轮；当前四受影响包normal/tag和两vet需fresh。另以相同C源/对应current构建执行既有三个严格外部中断/恢复selector（真实preCommit Kill+Wait/完整old及release-new、普通CLI DECLINE零写/CONFIRM、维护事务中断）、原schema-upgrade组件真实Store first-end及whole-state证明；它们不冒称新normal产品fault运行。只有原T026全部SC013闭环满足才勾，不将R115或本片green当整项；物理power-loss/ENOSPC仍原deferred。分析后才实现，不新增审批。

### 4.3 对象、候选与正式提交

durable transfer/预算→CREATE_NEW unready→流写/真实Flush→同对象冻结全hash→ready。新candidate绑定规范完整清单及旧基线；确认原key优先后再新准入，冻结全部对象校验持有到正式SQL实际结束。一个事务写完整version/引用/Head/receipt/Audit，无第二保存器。known拒绝留下的对象不变正式资源，unknown不删除/退额/重放。

下载只经resource/version/attachment授权关系；无objectId/hash公开下载。完整JSON先核全对象；字节流先全hash、同句柄seek后分块重鉴权；撤权停止后续，不宣称撤回已发数据。历史cursor绑定完整身份/资源与固定上界，不复用绑定不足的旧cursor。

### 4.3.1 T020独立普通Host字节正控载体（implemented / R94，非完整V201）

原任务/V201的最小外部载体已由[R94](<research.md>)实际验证：仅新增acceptance测试，A/B各三同名资源×两完整附件版本，v1两对象/v2替换移除，全部byte夹具、current/精确历史/原receipt在完整正常退出及新原生认证后守恒，焦点三轮通过。C使用验SHA的私有源码快照、离线独立缓存/合成root及自身构建的normal实际exe；末端重新核root/C完整源路径集合和内容，不复用A活动目录或产物。独立expected-byte负控只是oracle自检，不冒称实际对象腐败；R94当时尚缺的T020反例/产品边界后来已由R95–R97补齐，当前完整T020状态见tasks。T021多页/坏版本影响及其余T022–T029仍原图，均不由这个历史正控单独勾选或新增前置。

### 4.3.2 T020普通Host的坏引用与引用前替换（implemented / R95，V201仍partial）

原T020/V201、FR004–009/014、SC003由C在私有冻结快照补外部反例，不改产品或新增故障API。每例真实双M2先保存独立完好基线，再真实上传A未正式引用的对象。实际实验确认不仅上传producer，namespace pins在此NTFS也合法阻止live rename（既有primitive测试亦如此）；必须先完整正常退出，在所有原owner结束、尚未新open时，分别把文件移至同root备用名保留，或保留原文件并以相同字节创建不同FileID，再用新Host/认证/当前epoch验证引用拒绝，不能用陈旧候选/epoch的拒绝冒称FileID验证。另两例新Host上实际原FileID截短、原FileID/同长改字节，完整preview拒绝；截短ready文件明确不冒充“实际未完成上传”。第五例先在新epoch取得有效完整preview，再以相同FileID/长度实际改写其bytes，原确认必须重新全hash并拒绝、无成功receipt。路径精确匹配本例已返回ObjectId及原objects/v1，验证own root、普通文件及备用名不存在；绝不绕过/放宽pins或添加normal产品测试接口。

检查A原current/精确旧版/原SAVE receipt/真实Head守恒，无新正式引用；错误必须非成功、带typed封套且不泄秘密，不把HTTP unknown或缺receipt单独当作“未提交”证明。已知preview拒绝不误封原完好A内容。有效preview后正式SAVE已准入的失败按既有协议保留COMMIT_UNKNOWN/原owner，不要求此时公开Head或直接closed：先证明B真实新增/更新/读取，再经原key显式reconcile取得原物理结束及fresh rejected事实，之后才正常完整退出/新epoch重开核A Head/原回执和健康后续写。不得凭缺receipt或fresh metadata替代原owner收尾。允许拒绝操作按合法原意图产生审计/逻辑计费，不统一要求所有DB物理bytes不变；真实正常退出后新认证重开再核正式基线/Head，保留被移动或损坏的未引用证据而不自动修复。先只读一致性核对，去实际文件刺激的有效red，再焦点和三轮受影响外部验证、vet/指纹闭合；R95当时待验的实际未完成上传/产品边界后来已由R96/R97补齐；本片仍不代签T021–T029或物理排空九例。

### 4.3.3 T020普通Host真实网络短上传（implemented / R96，V201仍partial）

实施与真实运行见[R96](<research.md>)；以下保留设计及实测纠正来由，当前任务状态仅以tasks为准。

Owner C沿原T020/V201、FR004–006/011/017–020、SC003/005/009局部，复用R94普通产品和双M2载体，原TCP发合法65537长度/hash/epoch/key及Expect头，真实100后只写65536 bytes并CloseWrite，保留读半端取得实际响应，不用截短ready文件、注入EOF或替换产品Reader。检查503/COMMIT_UNKNOWN与原key查询的accepted/unready、无receipt、physical reconciling；读取本例objectId下的真实65536残留，全hash前缀。至少两次原key/区内预算只读调查不退reserved/unknown占用、不推广ready；辅助reconcile拒绝及B真实新增/更新/读取后再次核同key/hold守恒。pending时产品明确拒绝完整资源读，不伪造Head可读门：本片以原已确认receipt及明确正式读取拒绝界定可观察范围，完整正式Head/bytes验收仍沿原任务，不以缺receipt推定未提交。原辅助child reconcile应拒绝且不清理；不沿R95的SAVE reconcile路径，也不把正常客户端socket结束当Host内部owner结束。

完整外部producer记录在fallible网络工厂前root，含原fixture/Host PID、birth、image、socket/读半端、首个write/CloseWrite/Read/Close结果及操作key/声明/实际字节；close首次结果缓存，unknown不重试。测试完成后仅为释放实验进程，经既有独立fault helper新增精确object-upload-short-unknown分支：读取本owned fixture内CREATE_NEW、严格有界的观察证明，匹配同PID/birth/key、unready/objectId/长度、真实残留hash、reconcile拒绝及B进展，再复用原pinned process身份检查实际终止/等待。记录abnormal Forced，不称closed/Graceful/safeToMove/预算退款，不重开或删残留；不是产品恢复能力。失败清理由既有原producer收集，亦不冒充正常结束。不新增fault API。真实367首次与typed probe已经取得100/实际短流/COMMIT_UNKNOWN，但GET原key因VIEW_DELIVERY_FAILED失败：内部unready合法，而唯一OpenAPI的ObjectTransferView仅准ready，saveView共用成功DTO导致序列化失败。本片按FR011/022纠正这个既有需求缺口：唯一OpenAPI新增ObjectTransferOperationView（ready/unready）仅供OperationView.objectTransfer，原上传成功ObjectTransferView仍只准ready；显式生成Go绑定并只改原saveView投影，不改存储/原key/owner/hold/cleanup。先保存可编译协议与实际Host有效red、只读analyze，再实施并补协议、HTTP及普通Host分层验证；不能用把原key503放宽当200或允许上传201/unready来消除失败。第二次真实焦点已到合法原key200，但预算仍503；源证实原ObjectRetention准入即主动设置g.fenced（并非普通可读SQL），成功的显式key调查不授予普通事实读取。临时去pending guard未令实验通过，已恢复，不绕过closed/fenced/close-attempted、换连接或造预算成功。为直接观察本key实际保留而非伪造整个预算可读，新操作DTO只在unready时带原持久reservation.heldBytes；ready响应省略该新可选字段，保持既有ready操作wire。schema在unready时要求heldBytes，saveView从既有证据投影；上传成功DTO不增加此字段。测试核hold正数及拒绝/B进展前后相等，同时整区预算与正式内容在fenced期间明确失败，不把无法读取的全区计费说成零或已退额。owner/hold/cleanup和原key身份授权不变。

先只读一致性分析，去短上传刺激（发送完整声明bytes）必须在typed unknown断言有效失败，随后焦点与三轮原正常产品验证、vet和root/C源码内容/集合指纹。R96当时的产品边界后由R97补齐并勾完整T020；T021–T029/真正pending九例仍沿原图，不由本片单独代签。

### 4.3.4 T020普通Host真实默认/最大容量边界（implemented / R97，V201完成）

实际证据见[R97](<research.md>)：613输入，默认/最大各三轮，相关三包normal/tag全回归及同实际普通exe重验R94–R96均通过；原T020已按完整任务勾选。以下保留设计及真实失败的纠正来由，不是新的未运行状态；T021–T029及003前置未代签。

Owner C沿原T020/V201、FR004–009/012–013/018/024–025及SC002/009/012局部，新增独立acceptance测试，使用原ordinary exe、私有冻结消费者和真实protected-pipe会话；不改产品限额或新增fault API。两个独立安装：默认先读回16MiB对象/32附件/64MiB版，最大安装须在任何allocation前，经当前operator的原预算preview→adjust明确配置64MiB/64/256MiB并读回，再公开创建双M2。静态上限不提前称已测支持，不在已有allocation后改profile，不用1MiB合成上限代签真实产品值。

单对象恰好16/64MiB真实流写、就绪、正式保存与逐块下载；大payload由确定字节Reader产生，SHA及下载逐块核对，内存64KiB块而非把64MiB下载塞进原1MiB helper。对象+1经真实TCP/Expect只发合法完整header，正常必须429（默认profile）/413（绝对wire）且在100/body前拒绝；原key无新成功、区保留/预留/slots不变。若意外100，须送足有界声明bytes并读完/首个end，不能人为留下half-upload来掩盖拒绝失败。去超限刺激负控改成界内完整上传，应201后在同拒绝断言有效失败，保持fixture健康。

附件正控实际接收32/64个独立ready ObjectId，以唯一attachmentId/name形成完整正式版并逐个读回；默认33项429，最大65项由wire先400，不误称纯profile负例。版payload按逐附件累加，同一真实大对象的四个合法别名达64/256MiB，再添真实ready一字节对象作精确+1（数量远低于count cap），拒绝429；不篡改length/hash，也不让旧基线或预算不足遮住目标边界。JSON原请求1MiB可用合法尾随空白达界并获得非正式preview，+1字节拒绝413，正式Head/receipt守恒。每个拒绝前后读原区预算及当前正式事实，正常B CRUD；最后完整正常退出、新原生认证/epoch，逐版/原receipt及所有附件再次读回。保留完整producer及首个end，不把拒绝审计或逻辑计费变化一概当物理DB不变。

先只读一致性分析、有效界内负控、单轮焦点，再三轮真实普通Host验证、相关vet及root/C内容和路径集合校验。无产品变更时，以实际输入复用R96协议/HTTP/SQL回归，不重复无关广回归。T020只在剩余原义务全部有证据后勾选，T021–T029及003前置不由本片代签。

368真实焦点及独立profile-red已证最大profile安装中新M2首个合法64MiB上传409：原创建准入读取真实安装profile并保留allocation，但genesis硬种默认profile。修复只沿原创建owner/reservation、当前operator/s.mu，在原intent成功且fresh FS前读取既有ReadInstallationBudget的完整typed Profile并核原区保留allocation；已有allocation禁止改profile的约束继续保证该值稳定，不改原key/hash。以Genesis独立typed RetentionProfile value传入原fresh事务，默认/M1/安装seed不变，本地allocation/counters仍初始0；pristine VerifyCreationGenesis比较完整期望profile，未提交reconcile同样从当前原operator取得期望，已提交分支仍允许合法业务历史。缺失/不可信拒绝，不猜hash、不open修库、不修失败实验已有库、不改准入digest/revision/bytes/slots守卫。先只读analyze，再按实际原生红/绿及新增SQL pristine/非法profile/零补种负例，扩至受影响SQL、installation、HTTP回归与实际最大Host验证。默认/最大产品值不改变。

### 4.3.5 T021普通Host历史页、当前主体与旧版损坏隔离（implemented / R98）

实际证据见[R98](<research.md>)：614输入，同一冻结普通产品及独立typed helper的两类有效red、T021和受影响原V201各三轮及相关vet完成，原T021/V202已勾选。生产/包源与R97逐项精确核对后才复用normal/tag，不重复无关广回归；以下保留设计和证明边界，不代签T022–T029或003实际共享前置。

Owner C沿原T021/V202，依赖已验T020，映射FR002/012–014/023–024、SC001–003/008/012。复用普通产品双M2三资源×两完整附件版本及原receipt载体；只将测试save/verify helper推广到精确下一version/实际最新指针，不改产品。A目标仍用原principal-helper严格要求的`principal-target`目录，helper只在整个原Host正常结束后经原typed生产API给不同真实DB主体发read-write会话，原pinned PID/birth/Store/pipe结束后新normal Host才启动；不放宽其root guard，不声称该phase是read-only或Host签发R，也不新建系统身份。

在这个新活owner内，A同资源保存六版、用limit2首次取得固定upper的cursor；当前R无cursor正控200后借A cursor须400/INVALID_INPUT，A借R cursor同理，再A原cursor仍200。补同owner不同resource、已初始化B cursor key下的跨workspace、篡改cursor拒绝；正控之后实际追加第七版，旧页链仍只1–6无重漏，新页链得1–7。元数据逐项比对正式version/revision/manifest/content/operation，不将页当完整附件或跨请求事务。跨主体证据必须同活owner/epoch，不能用冷重启随机key失效掩盖。会话5分钟内完成并核真实PrincipalId，未调用的helper不算测试。

完整正常Host退出/新认证/epoch后复验A/B全部正式内容、附件bytes、原receipt，闭态源树仅豁免精确writer.lock metadata。随后在另一个已正常打开的owner内，只同FileID/同长度改A v1独占对象；v1完整读/该附件流与依赖坏对象的新preview须明确拒绝且零成功DTO/receipt，不返回少附件成功。v2/更新版及其他资源继续精确读取，metadata页可读只证明关系不证明对象；B真实新增/更新/读取继续，原v1 receipt不变、损坏源不被投影补种。保留对象与实际失败，不要求授权后拒绝操作全库物理bytes不变，也不删历史。

先只读analyze，再两类有效去刺激red（R请求改为A自有cursor应200；不改对象应完整v1读200使原拒绝断言失败）、焦点及冻结普通Host三轮/相关vet。仅测试变更时按实际生产文件指纹复用R97normal/tag，不新增无关广回归；共享测试helper受影响的原V201正控必须在同当前exe再验。原设计的未运行项当时不勾；现已由R98完成T021，不代签T022–T029或003前置。

### 4.3.6 T022普通Host并发、原key及丢回执首片（implemented / R99首片历史）

实际证据见[R99](<research.md>)：615冻结输入，两类有效负控、普通Host十轮case三轮及相关vet完成。失败路径先收齐并join全部producer，原始未知JSON字段/解码路径/headers隐私检查已补齐并单独负控；没有产品改动。该首片当时不勾T022；后续真实unknown已由下节R100补齐，当前状态只见任务摘要，以下保留首片设计与当时剩余边界。

Owner C沿原T022/V203、FR008–011/013/023–024、SC004及SC005已确认丢交付部分，依赖已验T021。复用普通产品及真实双M2附件，原Host完全正常结束后经既有严格principal helper发不同真实DB主体的read-write会话，helper完全结束才新开Host；两个另行protected-pipe连接签发的同原主体会话、两个实际HTTP客户端，在每轮同当前基线生成两个不同、对象完备的有效update候选，启动屏障并发确认共十轮，无额外故障。逐轮恰一200唯一receipt与一409/STALE_BASELINE，实际Head/版本只增一；完整旧内容/附件与输赢输入严格核对，不用两次顺序新preview代签并发。

原winner同key/同输入再确认及GET必须为原receipt，改变候选输入为409/OPERATION_KEY_REUSE；实际当前read-write其他主体GET原key为403且同key确认不得获原结果（按原key绑定拒绝）。B先查询A key无历史，再用B自己的合法候选及同operationId真实保存，A原结果不变；全部错误封套脱敏。另有新合法确认经真实TCP收到200 headers但不消费receipt body即关闭，之后只查询原key证明唯一正式效果并按原输入调查、不自动重放；明确这是应用未取得回执，不声称TCP字节未到达。完整正常退出/新原生认证/新epoch后所有版本、bytes及原receipt守恒，原旧epoch同key结果调查优先于新准入检查。有效负控只把第二次确认改为在第一成功后新preview/current baseline，两次合法成功应让原“一成功一冲突”oracle失败。

先只读analyze，再normal编译/焦点/有效负控/三轮当前普通Host及相关vet。仅新acceptance源时按当前冻结字节/集合复用R98和其精确继承的R97包回归，不重建每个负控或跑无关广回归。该首片当时不足以勾整个T022：真实admission/formal unknown与fresh调查现由R100补齐，不以本片丢交付或旧M1故障实验代签。

### 4.3.7 T022真实M2 admission/formal未知及fresh原key调查（implemented / R100，T022签收）

实际证据见[R100](<research.md>)：616输入，M2 admission三处/formal六处原生FSYNC故障，去注入有效red后矩阵三轮/相关vet完成；明确tag诊断Host与普通fresh调查Host分开，两种实际exe及输入冻结。原SC005与R99正式冲突/丢应用交付、R95坏引用组合，T022已按任务范围勾选；T023及后续未代签。以下保留设计，不扩为真实盘满/硬断电或T024实际pending九例。

Owner C沿原T022/V203、FR008–013/018–020/023–024及SC005，复用既有saveNativeFixture/readSaveProbe、原VFS FlushFileBuffers和workspaceSaveAdmit/Commit链；新增独立外部测试，不改产品/故障API、sharing或unknown语义。A真实M2根限定原fixture/allowed/save-target，先用normal产品创建非空完整v1和另一个ready替换对象，再正常退出；B独立M2。相同当前主体/新原生会话与epoch，在tag诊断Host发有效完整update候选。各phase先ordinal0测原调用次数及真实只读句柄control，再逐个既有1..count位置证明SQLite FSYNC1034/BOOL0/恰一注入、unknown和原callback结束；不同phase/ordinal使用独立合成安装。tag证据明确不冒称normal产品故障行为，normal产品build/nm另核。

同原key再确认不改变原native报告或重放；unknown期间实际writer排他仍占用，当前身份/安装共享依赖仍可信，B完成附件新增/更新/完整读取。已就绪旧/新对象原bytes与原引用保留。仅在现有严格report允许的精确PID/birth/phase/key处收集原进程，不称正常closed或硬断电；此后normal新Host/当前认证以原key调查，不重放业务。原admission确无持久行只报history unavailable并核原正式v1，不由缺行推定安全重试；formal不得丢已知admission。原key存在则显式reconcile，只允许可信原唯一committed或rejected，不能证明时须如实unknown；核原完整旧版、目标结果/receipt/新epoch、B版本和全部附件bytes，未引用对象不自动GC。fresh相同原输入不能新增版本。

先只读analyze，再normal编译/焦点/同build去注入的有效red/三轮两个phase及相关vet。仅新acceptance源，生产和原包源按371/R98/R97精确指纹复用，不新增无关广回归；冻结C另建normal及tag exe并分别记录SHA。有效red只将请求的probe ordinal改0，在正常成功状态处使原unknown断言失败；不伪造unknown/改产品错误。原SC005四类须合并R95对象就绪后拒绝/R99应用丢回执和本片真实unknown证据才勾T022，不代签T023–T029/003前置。

### 4.3.8 T027真实第二进程排他与M2恢复后才新写（implemented / R118）

实际见[R118](<research.md>)：662当前输入，普通M2三模式各十轮及原M1三模式各十轮、同build两个有效red、pure首次结束三次和当前vet通过；完整源/集合/报告哈希及normal产品SHA冻结。Helper不连接SQLite，不强加测试赢家formal提交；普通Host重获排他后先验全部原历史/字节/receipt再public新写和cold，正常与known-idle异常边界分列。原T027现签，以下保留设计；不代T028最终回归或发布/硬断电声明。

Owner C沿原T027/V208，FR006/019/024、SC011，依已验T026。保留原M1固定SQLite writer与矩阵，不让其create-text误写M2，也不新造M2第二提交API。只新增TEST-only严格lease contender和普通M2 matrix：原synthetic root/单allowed root/无reparse/显式scope及当前M2 descriptor、固定descriptor/DB pin；原native Acquire失败者在SQLite连接之前返回busy、零正式保存。赢家仅实际hold lease/pins而不打开SQLite，因此所有helper都无formal write；恢复后正常Host先真实Acquire并完整校验原M2 current/history/附件原bytes/receipt，再用原公开preview/confirm新写，符合原SC011且不加“测试helper必须提交”的需求。当前真实授权在公开Host操作处检验，合成helper不是安全沙箱/产品授权API。

三模式各十个独立非空M2：continuous中活Host先持锁、真实第二进程失败，正常关闭后contender获锁；simultaneous两个独立真实child在Acquire前ready、pin不同PID/birth/image后共同开始，恰一held一busy；killed先已确认SAVE/完整读和drained原key、无请求/SQL/派生活动的已知idle Host按原pinned精确phase异常结束并Wait，再contender持锁、普通新Host恢复原身份。赢家hold时Host open必须409/WRITER_BUSY且无epoch；已知descriptor/DB pin/lease first Close以及实际child end后才新epoch。原helper进程树、pipe两端/reader/线程、process pin在fallible acquisition前完整root，所有结束第一次结果缓存，未知不重试；失败保留partial owner和producer直到自身实际结束，不用socket/marker当native end。最早ready保证父pin原真实进程，不从复用PID重找。

每轮在无Host owner/已有存活contender结束后的同一closed基线比全正式源（仅精确writer.lock允许native metadata），核DB os.SameFile；失败者无SQLiteAttempt/no receipt，零新增formal/resource/key；正常重获后原各版DTO、所有附件bytes及唯一SAVE receipt不变，再新增下一版并完整正常cold复验。普通异常结束仅发生已知idle，不称硬断电/事务crash、unknown不作graceful。正向HTTP复用原已知first-end transport，全Host最后日志/秘密及所有helper bounded原输出持续检查。

先只读analyze再实现；同build有效负控仅提前正常释放真实winner并Wait，使本应busy的Host open真正200击穿409断言，不伪造错误/绕root。当前普通三模式各十轮、纯first-end/相关vet与既有M1三模式兼容各十轮在原V208范围定点执行，保留所有失败。仅新acceptance与Doc3，生产/依赖/旧helper不变时以完整source/pathsets/报告hash复用R116实际normal/tag包证据；普通product及TEST-helper语义分别标，不构造诊断product故障或扩大其他阶段门。

### 4.4 投影与生命周期

正式confirmed后同owner/原allocation生成独立清单代；CREATE_NEW且不覆盖，目标不可信/预算不足只影响health，不能抹receipt或补一次正式commit。再生成指定正式version/new key，不调用AdmitSave；无可变latest/外部Apply。

对象/投影/流句柄、原生请求、回调、postcommit和数据库owner实际join后才释放pins/最后writer lease；unknown按原持久证据调查。close准确元数据查询可借原有界分配，不把普通新业务变control。SC-007实际IO九例是门，不允许仅marker悬置替代。

### 4.4.1 T017原owner首次leaf Close故障的合成切口

implemented / R76已验证上传writer与附件Frozen，R77增补投影writer/generation Frozen/health Frozen三条原owner的真实首次native Close失败；仅局部生命周期证据，T017仍partial。具体输入、失败、分层normal/tag回归及普通产品排除测试切口见[research R76–R77](<research.md>)，未覆盖范围继续按下文保留。

R74的独立工厂首次Close失败不是安装原操作组合证据；公开上传source.Read及附件consume同步回调已由原owner持有真实leaf，但没有可安全定位该leaf的测试入口。沿[contracts合成夹具边界](<contracts.md>)，由A协调、B独占新增`internal/platform/windows/object_owner_close_acceptance.go`（仅`windows && aboard_acceptance`）及相邻负例，A新增`internal/workspace/open_resources_acceptance.go`与`internal/installation/workspace_owner_close_unknown_windows_test.go`。这仅完善T017/US5、FR019–020、SC007/009的局部生命周期与容量保留证据及V205，不改需求、任务ID、生产物理接口/普通构建或SQL/schema/预算语义。

平台只允许在原writer source同步回调、原Frozen一次Read返回后的consume回调，或下文明确锁序与栈白名单的原投影授权回调中，对当前有效lease/gate持有的唯一disk文件leaf设置首次`PROTECT_FROM_CLOSE`。要求`.test.exe`、明确child环境、由源码定位本仓`.data/logs/002-validation/`严格后代、拒别名/reparse祖先及dir/closed/quarantined/已protected/可继承句柄，并核同namespace/FileID/lease；不接受外部handle或任意路径，不枚举/复制/替换handle，不碰pins/lease.lock，不Close/撤保护/重试，不伪造unknown。writer已由WriteFrom持锁，切口只在同步source回调使用且不再取writer锁；Frozen仅在单次Read已返回后取原锁，不在Read内部递归。workspace桥仅以同步visitor触及原OpenResources，不返回/留存资源，不获取已持有的owner锁；这是合成测试协作能力而非安全沙箱。

先验无child/nil/无效根/别名/重复刺激等拒绝及未刺激正控仍能正常首次Close；两个独立child分别验证上传writer首次Close失败与正式附件stream首次Close失败，经真实原生API和原完整操作链自然形成sticky unknown。要求原ticket/epoch/lease/DB/pins不释放、cutoff拒绝新准入、原key不重读source、原保留/预留不误退、另一已登记B真实CRUD并正常关闭；未知A不Drain/cleanup/标graceful，保留至该child进程结束。上传与已正式附件的预算义务分别核，不给后者臆造新hold。normal构建必须不包含新增API，tag焦点及受影响回归另跑并冻结输入。无法准确安全刺激时保unverified，不以test helper返回error代替真实故障。

**原投影授权回调扩展（implemented / R77三类局部验证）**：R76后静态核对表明generation与health并非完全没有可控同步回调。A仅在同一acceptance-tag测试中，用`WorkspaceOpener.readOwner`取得真实当前check后包装，仍将其交给原`OpenOwner.GenerateResourceProjection`/`ReadResourceVersion`与原allocation、SQL、容量和native链，不替换权限或事实端口。先执行真实check，再用`runtime.CallersFrames`精确白名单识别安装原`(*objectTransferAuthorizedReader).Read`（writer）或原`(*openResources).GenerateResourceProjection`/`resourceProjectionHealth`（Frozen），仅在原owner已持锁且原child实际存在时同步visit；beginOperation等早期未持owner锁的check必须直接返回，不能无条件visit/探资源。writer仅在原WriteFrom source边界且不重取writer锁；generation/health Frozen在原check已返回、首次Validate/Close前且没有持Frozen锁时使用现有取锁切口。只同步调整tag-only合作注释，不新增产品接口/normal代码/锁或错误注入。每类独立child验证真实首次Close与原owner/capacity/lease保留、B CRUD、第二进程排他及零成功交付；去除刺激须在原断言失败。writer记录已准入基线零退款；generation Frozen允许原ready事实在Close前正常提交，不错误要求其整个DB相对pending基线不变，只核故障后的原DB无后续清理/退额且不另开SQL连接。health为只读前后DB守恒。全包变更限tag测试/注释，按影响复验安装tag包/相关native焦点与vet，未变normal产品证据须有输入指纹依据复用。Owner A，仍属原T017/US5、FR019–020、SC007/009局部与V205，不扩任务或提前执行T018。

该扩展仍不覆盖projection精确Validate故障、WriteFile/Flush真实pending或SC007取消/超时/断连九例；T017与V205未覆盖义务继续显式保留。宪章核对：I原事实owner/身份不变；II不改变inspect/open或排他；III不改变正式保存/回执/调查；IV故障通过原首次Close自然产生、未知保留真实owner；V保持当前授权/原容量/秘密且不宣称沙箱；VI每例B独立；VII不变公共协议与正常Host。无例外，需现有spec/plan/tasks只读analyze后实施。

### 4.4.2 T017原发布／投影最终冻结的真实pending观测（implemented / R78局部验证）

**纠正的技术假设与范围**：SC-007要求发布、校验、投影三个阶段各有真实未结束物理IO，并未指定必须挂住`WriteFile`或`FlushFileBuffers`。R5已批准的`publication-final-freeze`属于原Write→Flush→Close→同FileID最终冻结→ready发布链，旧G203平台pending证据仍仅证明原语；本片要接回原安装owner，不能改标签重用校验/health记录。将残余描述的“publication写入/Flush”澄清为原发布链真实pending；单点WriteFile/Flush仍未验证，但不额外提升为规格之外的新推进前置。T017、T024/SC-007九例、精确Validate故障、独立产品Host与T018–T029原依赖全部保持，局部通过不等于T017完成。

**最小接缝与理由**：上传与generation原`writeFlushAndClose`到`FreezeObject/FreezeProjection`之间没有既有可确定拦截的同步回调；source/授权回调此时还持有writer，外部batch oplock不能可靠获批，目录RH通知也不等于pending。拒绝sleep竞态、全局registry、枚举/替换/额外Close产品handle、测试返回错误或另一物理owner。由A串行修改原`ObjectGate`：仅嵌入按build tag定义的观测状态，在两处原finalFreeze的`g.begin`成功后、原native open前调用无结果通知；normal为零状态空实现，不暴露激活API，必须重建普通产品并核nm。tag状态只属于这一原gate，保存唯一原writer引用、namespace、opaque ID与Identity，只有一次目标FileID的同步通知；两Freeze也用于校验/health，因此通知必须正向核同一writer确已成功Close且非quarantine，不能仅靠expected或g.files推断发布阶段。Projection通知位于已取得本次generation pin之后、原manifest leaf open之前，pending日志必须准确指向原leaf IO。不新增运行容量、SQL、授权或生产错误分支。

**tag合作边界**：新增平台tag-only注册API仅可在原writer同步source回调注册；先验证`.test.exe`、明确隔离child、本仓忽略日志根严格后代/无别名或reparse祖先、当前lease/gate及唯一writer的namespace/FileID，拒nil、重复、closed/quarantine、任意路径/外部handle。不递归writer或owner锁，注册时不改产品handle；通知只在同一gate、同一namespace/opaque ID/expected FileID及原writer成功关闭匹配时一次消费。注册已处于w.mu内，严禁重取；通知先释放观测锁，再按原w.mu→g.mu顺序核原状态，不能持g.mu反取w.mu，之后释放所有writer/gate/观测锁才回调。测试回调只为该明确合成leaf建立自身的batch-oplock blocker，核实际FileID，然后必须返回；真正被挂住的必须是随后原生open，而不是通知回调。通知无返回值，不能替代原结果、权限、ctx或native调用。合成接缝是合作测试能力，不是OS沙箱；错误保留完整fixture，不隐式清理unknown。

**实施与验证顺序**：Owner A，沿原T017/US5、FR-019–020、SC-007–009局部与V205，不新增竞争任务/接口owner。先tag拒绝/无刺激正控，再分别在原公开上传与原generation链记录oplock grant/break、原OS线程实际pending和原gate/lease busy；实际释放后原IO join/持久化/Close按原协议进行。每片覆盖A关闭或Shutdown期间原cutoff/metadata有界claim/第二进程排他及B真实CRUD，不在本地fixture冒称独立产品Host九例。去除观测刺激应在真实pending断言失败，恢复源码后再跑焦点及受影响platform/workspace/installation normal与tag回归、vet、普通产品build/nm；冻结输入并保留所有失败/SKIP。先现spec/plan/tasks只读analyze，无阻断后才实施。

**R78实施状态**：上述窄观测已实现，[345全套](<../../.data/logs/002-validation/final-freeze-pending-345-full-results.json>)在591输入下完成两阶段各三轮真实pending、三包完整normal/tag与两种vet、普通产品build/nm，八步均0；去除通知的两阶段负控均真实失败，恢复测试SHA与通过的focus完全相同。原publication和显式generation分别由原Close接回实际IO/SQL join、第二进程排他、B独立CRUD及新epoch/hold结算，不将显式再生成冒称自动postcommit，不将隔离test child冒称独立产品Host九例。normal只有零状态空通知，无注册/激活API。截至R78当时，精确Validate、自动postcommit的真实pending与T024九例仍未签（自动子片后续证据见R79），T017及原后续依赖保持未完成；单点WriteFile/Flush仍未测而非新增前置。

**自动postcommit增补（implemented / R79局部验证）**：R78显式再生成不是自动派生证据。A仅扩展原安装acceptance-tag夹具，从全新M2原fixture的真实空附件PreviewResource进入原`CommitRecordWithAllocation`，包装`readOwner`取得的真实当前check并保留原allocation；精确识别原postcommit栈及原generation writer source边界，才注册已经实现的同gate最终冻结观测。不直接调用Generate、不预先完成SAVE、不新增平台接缝或正常产品改动。原SAVE ticket与owner/gate锁持续持有，不能重入opener/gate事实查询；仅在原同步visitor中经同一`r.database.QuerySaveOperation`读已确认SAVE回执、经同一`Database.ValidateOpenHistory`只读核尚存accepted/held调查围栏，并拍持久文件基线。先由pristine夹具及准入前零reserved证明没有旧unknown/hold，精确自动postcommit writer栈与正式回执共同约束本片归因；该布尔围栏不是对随机projection key或具体held数额的枚举。禁止把要求WithClosed的`ReadWorkspaceRetentionEvidence`挪作在-open读面。这不是额外连接、fresh证书、预算CAS或另一事实owner；任何观测错误只记测试失败，不替换原权限/IO结果。真正阻塞仍须由另一OS线程的原blocker IRP已完成而原SAVE线程仍kernel-pending证明。期间验证原cutoff/claim、第二进程排他及B CRUD；实际释放、原SAVE/派生IO/SQL join后，核原精确SAVE回执、不可变正式版本与唯一自动generation manifest，显式关闭重开后同SAVE key仅查原回执且不再生成，原hold才可按known证据结算。去刺激必须失败，恢复后按输入跑焦点/受影响normal/tag与vet；原ID/依赖/T017 partial及T024九例不变。此为T017/US5、FR019–020、SC007–009局部/V205，先同步任务说明并只读analyze再实施。

**R79实施状态**：[347 full](<../../.data/logs/002-validation/postcommit-pending-347-full-results.json>)在591输入下覆盖三阶段各三轮真实pending/join、27次第二进程排他；自动子片三次均先读精确正式回执及pending围栏，重开后旧SAVE只查，原manifest身份/内容/mtime及唯一generation不变。去除自动通知后原SAVE confirmed却无pending，真实负控失败。原installation完整tag与tag vet、普通产品build/nm均0；逐输入与R78比较仅该tag测试及三份文档变化，重建普通产品SHA完全相同，故复用原normal/platform/workspace证据，不称本轮重跑全部包。当前只组合Close，不能外推Shutdown、精确Validate或独立产品Host九例；T017 partial与硬依赖不变，详见[research R79](<research.md>)。

宪章核对：I不新增事实owner或身份推导；II不改inspect/open及lease排他；III原写刷关/冻结/ready/回执顺序与错误完全保留，自动派生必须在正式确认之后；IV通知回调不是pending证据，实际IO与join才算，unknown不释放；V原授权/分配/预算不变且无秘密输出；VI每例B独立、原共享unknown规则不变；VII无客户端依赖/公共协议或可激活普通产品API。无豁免，不改需求/宪章或当前授权。

### 4.4.3 精确Validate故障的有界可行性检查与原owner组合

R79后剩余的是同一实际Frozen的`Validate`失败，不是另一次hash失败。A先只在独立acceptance-tag平台测试及本仓忽略日志根下的合成fixture，检查原冻结句柄持有期间两种外部元数据变更能否合法发生：新增hardlink、经新建且自有的`FILE_WRITE_ATTRIBUTES`元数据句柄设置不解析目标的非Microsoft GUID reparse数据。只使用已有Win32接口与有界buffer；禁止枚举/复制/关闭/替换原产品handle、修改原File的期望身份或path、故意关闭lease、伪造Validate错误、注册过滤驱动或改系统策略。新句柄只由实验拥有并显式Close，原Frozen/owner仍按既有次序结束；不删除或遍历新reparse目标。该测试文件仅tag可见，不在正常产品添加接缝。

先核正常`Validate`及完整内容，再实际尝试外部变更，记录精确Win32返回/同原Frozen的实际Validate与known结束。若系统以共享或访问控制拒绝刺激，只证明此方案不可用，不能记作已覆盖的Validate故障；不能以SKIP或期望拒绝的绿测试勾T017。若变更成功且原Validate确实失败，才以实际机制另行设计安装owner组合，仍不直接代签生命周期/SC007。严格限定一次两机制实验，不在相同拒绝上无限重试。映射T017/US5/FR019–020/SC007–009局部/V205；原ID、依赖、验收义务不变。先最小任务说明与只读analyze，无阻断后才运行；全部实验输出仅`.data/logs/002-validation/`。宪章I–VII：不变事实owner、排他/保存/身份权限/预算/跨区与公共协议，不新增运行容量；metadata异常与结束证据分别记录，失败不等于unknown释放，无豁免。

**平台可行性结果（R80 / implemented）**：349一次两机制检查及正常/tag正控、tag vet均0，591输入无漂移。hardlink实际被共享冲突32拒绝，未覆盖该刺激；自有metadata句柄的GUID reparse成功，使同一个Frozen的原Validate报`reparse point or hard-linked key object`，原冻结句柄实际Close已知。仅平台实验，不代签安装组合或T017。

**安装owner组合（implemented / R81局部验证；以下保留设计义务）**：A仅新增acceptance-tag平台合作函数与安装测试，复用原同步visitor、原readOwner当前check/ordinary allocation与原Frozen；独立测试child、项目合成根/无别名、原namespace/唯一leaf/lease/父pins/完整identity均须正面核验。只对自行新建且同identity的metadata句柄设置上述GUID数据并首次Close，不改原handle/path/期望身份、不保护或提前关闭原leaf、不伪造错误/权限、不增加正常产品API或池。平台函数须拒绝nil/错误进程/重复/非原冻结边界；回调不将刺激错误塞作业务错误，成功后仍由原产品链调用真实Validate。

覆盖三个原链：正式附件stream在真实完整读取后变更leaf、下一次原reader校验必须拒绝且零额外字节，reader失效及原首次Close已知后可归还本次只读ticket；独立generation在原全hash返回后/ready witness前变更，真实Validate拒绝并保留原accepted/hold/ordinary/DB/pins/lease，不把进程结束当drain；正式version的health在原全hash后变更，原Validate失败而首次Close已知，正式version/Head/receipt不变，只返回unknown health，不误封完好正式对象。generation受损原owner的Close/cutoff/第二进程排他/B独立CRUD依既有夹具核验；只读两例核零DB写、原ordinary返还、原Close实际完成与第二进程重新取得排他。三例均须实际非空选择器、去刺激负控失败、恢复后焦点及受影响tag/normal/vet、普通产品build/nm输入对比。此为原T017/US5、FR019–020/SC007–009局部/V205，不勾T024独立产品Host九例；T018及后续原依赖不变。宪章I–VII逐项沿前述核对，无新owner、恢复权、预算、协议或授权，不修改规格与验收强度。

**R81实际结果**：[350 full](<../../.data/logs/002-validation/owner-validate-350-full-results.json>)在594输入下完成三类原owner各三轮、两包完整tag与vet、普通产品build/nm，七步均0、Diff=0、SourceMismatch=[]；去刺激时三类原断言均有效失败，未改验证时源码。stream/health实际首次Close已知、零DB写及原ticket返还，第二进程重新取得排他；generation相对已准入DB基线无清理/退款，保持原pending/ordinary/epoch/DB/pins/lease及B独立。普通产品SHA与R79完全相同、无新测试切口符号，复用未变normal证据而不冒称重跑全部normal包。按工程实现范围完成T017；T018/T019及T024独立产品Host九例仍须各自验收，见[research R81](<research.md>)。

### 4.4.4 T018首片：health原冻结后的当前撤权、自然过期与取消

**状态：implemented / R82局部验证，T018仍partial。** [352 full](<../../.data/logs/002-validation/health-authority-352-full-results.json>)在595输入下完成三种各三轮、installation完整tag/vet及普通产品build/nm，六步均0、无输入漂移；去刺激三种原断言均有效失败。普通产品SHA与350相同，不外推成全部normal重跑、TCP断连或完整T018。详细边界及残余见[research R82](<research.md>)。

T017工程组合完成后，Owner A沿原T018/US5、FR002/019/023、SC008–009局部/V205，先在新增安装acceptance-tag测试中补三类health交付边界，不重做已有stream撤权测试。复用原readOwner的真实当前check、原ordinary/lease/SQL/native链；只在既有精确health同步栈且原Frozen已经成功全hash、尚未首次Close时，通过既有tag visitor确认刺激点。不同隔离child分别调用真实RevokeSession、让刚签发的短会话自然到期（有界真实timer，不改clock/SQL expiry），或取消本次原context；另有未刺激ready正控。此slice不新增平台或正常产品API，不返回伪造授权错误，不在callback重入原OpenOwner或持有其资源逃逸；check每次仍真实调用当前授权，刺激后由原产品后续check自然拒绝。

要求三例零正式内容交付（完整零ResourceVersion），不是返回正文带unknown health；本次health首次Close须known、原ordinary归还，原owner仍open/epoch不变，正式源DB/descriptor/receipt守恒。撤权只允许其真实安装会话变更，核撤权后的安装bytes为新基线；取消/自然过期不写安装库。利用仍有效的另一当前会话读取精确正式内容及ready，证明局部交付拒绝未误封正式结果；原资源实际Close并验证第二进程排他释放。自然过期必须在刺激点前授权仍有效，等待后真实Check拒绝，不以测试错误或过早过期代签。先只读spec/plan/tasks分析，再取去刺激有效red和正常焦点/受影响tag回归、vet；未变normal产品按实际字节指纹复用。该片只是T018局部，全部route授权/串区/共享unknown/公开脱敏、binary已开始终止及独立产品Host九例仍按原任务验收；不勾T018/T024。

### 4.4.5 T018 binary首Write后的取消与交付写失败

工程实现与实际分层验证见[research R83](<research.md>)；本节保留原设计义务，不替代tasks当前状态。

Owner A延续T018/US5、FR002/019/023、SC008局部/V205，先补现有binary错误控制流中尚未实测的两个分支，继而回到route/shared-unknown矩阵；不重复R62已有首块后真实会话撤权，不以本片替代共享unknown整链。新增仅HTTP测试，复用真实DPAPI/授权会话/Store/M2 source、实际HTTP上传与正式preview/commit路径，生成65537 bytes附件。在同原handler的ResponseWriter边界，三个子例于首Write分别取消原request context、仅交付确定前缀后返回真实writer错误、短写但nil error；先完整未故障读取为正控。记录实际接受前缀，均必须触发http.ErrAbortHandler，恰一次Write、200二进制headers/原全长度、没有第二JSON或后续字节。请求取消不伪造成会话撤权，writer错误/short write只证明交付适配器边界，不冒称TCP ACK、真实断连或native Close故障。

仍有效原会话在同owner读取正式version/ready及相同附件；原Close/WaitClose实际完成后重开，证明known物理结束和原ordinary/租约无悬挂，不增清理/退款端口，不把另开owner读成功当旧owner已排空。先只读分析，再取显式去刺激的三种有效red、恢复焦点及受影响HTTP normal/tag/vet；验证不改源码，普通产品SHA/build/nm按输入复用/核对。仍不勾T018/T024，route矩阵、真实共享unknown、安装权限分离及独立Host九例保持原任务义务。

### 4.4.6 T018共享安装unknown下的已打开非空M2

工程实现与实际分层验证见[research R84](<research.md>)；本节保留原设计义务，公开HTTP及跨区/权限分离仍由T018承接。

Owner A沿T018/US5、FR002/020/023、SC008局部/V205，新增安装测试：隔离child复用实际upgraded Store、当前独立writer、原M2 owner、两版/两真实附件/旧key/独立投影和有效新candidate。先以原授权走通current/exact/history/stream/预算/原key及真实生成正控，再经已有测试VFS syscall机制，使同Store的另一项安装创建准入实际FlushFileBuffers失败（原生只读句柄ERROR_ACCESS_DENIED，经SQLite映射为SQLITE_IOERR_FSYNC/unknown）；不直接置authUncertain/物理pending、改权限实现或新造owner。无故障child确认相同安装准入正常，不把纯error-return代签原生失败。

失败后使用原已签发能力分别调用M2 current/exact/history、legacy current/list、附件stream、preview/new commit、对象旧key、projection旧/新key、SAVE/object/projection原key调查、区内预算/分配、安装预算及原授权Check：必须共享ErrAuthorizationUnknown、零业务内容/receipt（新commit仅允许SaveRejected零receipt），stream callback/source reader均不调用。对每次拒绝核同一原pool/epoch/lifecycle/owner/installation lease与精确unknown占用不变，源workspace全文件及失败后的安装bytes不变；不把共享安全unknown错误地说成A局部故障而让B继续。原安装/工作区原生排他必须仍有效、ShutdownReady拒绝；unknown child完整root custody不调用Drain/Close/退款/恢复，进程退出也不称实际排空。正常child实际Drain/首次Store Close后结束。

该片只证明service/current-capability矩阵，不代签HTTP 503/envelope脱敏、安装权限分离、B跨区请求或独立产品Host。先只读一致性分析，再新增测试与显式去故障有效red、原生焦点和受影响installation normal/tag/vet；未变普通产品据实际build/nm/SHA与原证据复用。仍不勾T018/T024/T029，不新增已批准盘满延期以外的验收延期。

### 4.4.7 T018公开当前权限与安装角色分离的最小差集

工程实现与598输入的实际分层验证见[research R85](<research.md>)；本节保留设计义务，不替代tasks当前状态。

Owner A依已完成T017，沿原T018/US5、FR002/023/025、SC008局部/V205，在原HTTP测试中复用实际当前session、M2原Store/handler/登记/两版与上传正控，不新增权限实现。只补尚缺的公开入口：真实read-only或撤权会话对object旧/新key、完整preview、commit旧/新key、projection旧/新key；真实非原actor writer/撤权调查者对三类原key GET/reconcile；一个未认证M2代表。用同一安装当前DB中的实际不同主体（不以Grant.Action给owner降权），把workspace writer-only对安装budget GET/preview/adjust、schema preview/confirm、原安装key GET/reconcile的拒绝纳入同一用例。最后移除实际owner全部workspace grants，重新签发当前installation.operator会话，证明安装预算仍可读、M2写不可借owner身份绕过。已有reader投影/stream与writer不能reconcile等例据当前源码复用，不再创建竞争矩阵。

原body sentinel证明缺少当前route grant的拒绝先于读体；已经具备workspace.read的非原actor原key GET以合法无body请求验证业务层主体拒绝，不把非法带体GET的400当鉴权反例，也不要求查询原key前解析不合法请求。无route grant时existing/missing给同一受限错误；有当前read grant时，原key的actor拒绝与历史unavailable仍分别遵守既有契约，不把unavailable当不存在/回滚证明。wire body/header/Event检查token、raw/JSON escaped路径及无权内容（合法授权内容保持完整）。拒绝前后核正式workspace全文件/receipt/对象/预算与原epoch/可读性守恒；鉴权设置只在明确基线前写入真实安装DB。去权限刺激负控只改测试参数，不能改产品源码；先只读一致性分析，再焦点与受影响HTTP normal/tag/vet、普通产品build/nm/SHA。该差集仍不代替真实双区关系搬用、HTTP共享unknown envelope、binary首块自然到期或独立Host；不得因此勾T018/T024。

### 4.4.8 T018 binary首块后的真实会话自然过期

Owner A沿既有T018/US5、FR002/019/023、SC008局部/V205，复用R83真实附件/原handler及原完整custody。仅给既有HTTP测试session helper增加可选短TTL（旧调用仍5分钟），在全部上传/正式保存及完整TCP正控后签发3秒真实protected-pipe会话。第一次Write实际接受65536 bytes时，当前授权必须仍有效；在同一同步Write回调有界等待到签发返回后TTL+裕量，再由真实Authorize确认已自然过期，原request context保持有效。不改clock、SQL expiry，不调用RevokeSession，不返回伪造auth错误；原产品下一轮check须自然触发http.ErrAbortHandler，恰一次Write、零第二块/JSON。ResponseWriter边界只证明交付适配器，不冒称TCP断连/ACK或kernel pending。

仍有效会话在同原owner取完整正式version及相同附件，再原Close/WaitClose实际join后新epoch重开；未结束资源不靠重开自证。去等待刺激负控必须在原截断断言失败，不先以缺刺激失败；只读分析后焦点/HTTP分层回归，普通产品同字节证据据输入复用。仅测试helper和新增测试，不新增产品API、授权或物理owner；不勾T018/T024，双区与HTTP共享unknown仍由原任务承接。实现与356实跑证据见[research R86](<research.md>)；三轮焦点、HTTP完整normal/tag/vet及普通产品逐字节隔离已通过，保持边界声明。

### 4.4.9 T018真实安装unknown到原HTTP封套

实现及600输入的实际red/分层验证见[research R87](<research.md>)；本节保留设计义务，不替代tasks当前状态。

Owner A沿原T018/US5、FR002/020/023、SC008局部/V205，复用现有HTTP合成M2、真实protected-pipe会话、非空正式附件/候选/投影和原handler。两个有界独立test child使用同一安装创建准入：healthy child真实创建成功并走已知fixture收尾；unknown child仅在已有固定SQLite win32 VFS ABI，以实际只读原生句柄令第一次FlushFileBuffers失败，随后恢复syscall原表；必须记录真实失败及原Authorizer的ErrAuthorizationUnknown，禁止直接设置authUncertain或伪造权限/业务错误。不新增产品故障API、driver、权限或物理owner。

在unknown child触发前将完整fixture/custody持久root，触发后禁止Drain/Close/重开/恢复/退款/清理；子进程退出不称actual drain。对原current/exact/history/content/metadata、候选/对象/提交/投影及原key调查、生命周期和安装/区内budget等二十余项路由逐次经原handler及真实TCP要求503/INSTALLATION_STATE_UNKNOWN、unknown outcome、installation scope、investigate/no-store和正确关联，POST body零读、零业务内容/receipt。每次核故障后全部源bytes（仅精确原share=0锁metadata）、实际安装/workspace排他仍占用、ShutdownReady pending，body/header/Event脱敏；精确内部held=1/ordinary=1继续引用R84，不以HTTP外部断言冒充读到内部计数。去native故障负控必须在原HTTP拒绝断言失败，首例GET无非法body；先只读analyze，再焦点与受影响HTTP normal/tag/vet/产品隔离。不勾T018/T024；真实双区关系和独立产品Host仍按原图。

### 4.4.10 T018同一安装内真实M2 A/B公开关系搬用

实现及601输入的实际red/分层验证见[research R88](<research.md>)；与R82–R87合并收口T018工程范围。下文保留设计义务，不替代tasks当前状态或独立Host验收。

Owner A依T017，沿原T018/US5、FR002/023、SC008局部/V205，新增一个HTTP测试复用同一真实安装、当前会话、原预算CAS/登记/open/上传/preview/commit/投影。既有合成source helper仅抽出接受两个明确ID/leaf的私有构造，旧调用不变、保留完整partial-producer首次结束/root规则；既有inventory只增精确第二workspace锁metadata，除此全文件hash。A/B各自真实format3/schema2、不同root/descriptor/open epoch/正式带附件版本；相同对象操作key、相同原始字节在两区须得到不同本区objectId和各自原key结果，不新造全局hash能力。

先真实DB创建仅有A grants的不同当前主体，并正向读A；对B existing/missing的object/preview/commit、current/exact/list/content、projection及原key GET/reconcile，经原handler和真实TCP均403/FORBIDDEN/rejected/none、workspace scope，POST可观察body零读，A/B源bytes/预算/epoch守恒，安全封套不因目标存在而改变。再用已有A/B权限的原主体和正确B epoch/当前基线做关系层反例：A ready objectId/精确hash放B预览409 STALE_BASELINE；A未应用有效candidate放B fresh-key commit同409；A resource/version或仅A attachmentId置B正式关系读取/投影404 NOT_FOUND；B本地正式version套A摘要投影409。原A-save key在B无同key须OPERATION_HISTORY_UNAVAILABLE，不借缺key推断回滚；相同对象key已有B自己的结果正控。

两层不混：授权后拒绝只要求无错误candidate/generation/receipt成功、两区正式版本/附件/原key及epoch守恒，允许合法审计/计费及底层DB计数变化，不统一套全库bytes不变。B自己的同字节object可正常预览，最后原A candidate仍可按A新key确认，证明没有消费外区候选；拒绝的header/body/Event扫描私有token/路径/两区内容，合法授权成功不列为泄漏。先只读analyze，再分别用实际增B grants、替换为B object/B candidate/B资源四个去刺激有效red和三轮green、HTTP normal/tag/vet、普通产品隔离；不改错误枚举或权限产品。T018按整体覆盖核对后才能工程签收；T019及T024独立产品Host九例/业务验收不由本片代签。

### 4.4.11 T023普通Host派生health、旧版再生及候选守恒首片（implemented / R101，T023仍partial）

实际证据见[R101](<research.md>)：617输入，两类同build有效负控、普通CLI完整case三轮/相关vet完成；仅新acceptance源，没有产品改动。明确projection POST返回generation而非现场ResourceVersion，live inventory绑定实际fixture且打开前只豁免精确share=0锁，错误脱敏已修正。真实Create/Write失败及目标占用仍待；独立Win32范围锁实验仅是未来技术输入，不计Host验收。以下保留本片设计。

Owner C，原T023/V204、US4、FR015–017及SC006局部；T022已验。新增独立外部acceptance测试，复用原普通CLI/pipe/双M2和完整附件SAVE载体，不改产品/故障API。实际v1/v2生成后，以公开health提供的精确generation定位已知结束的合成manifest leaf；只在owned root内，同FileID/同长度写入未应用真实candidate的可识别bytes，以及把真实完整v1清单写入v2原leaf形成实际陈旧；另将最新v1的精确已读leaf改名保留形成缺失，不删除目录、不改sharing或猜generation。所有自有file首次Close结果缓存；同一正式版本的健康观察/read/原keymetadata不得修复文件、改Head/receipt或消费candidate，缺失/坏投影不妨碍完好正式内容和B业务。

在current为v2时明确新key重生v1，要求新generation/完整确定性v1清单、保留原不可信旧leaf及v2清单；旧key只查，不把其保存时的metadata health当现场文件证明。再分别恢复陈旧v2和缺失v1到新generation，旧坏/缺失路径仍不补种。真实未应用candidate直到所有投影步骤结束后仍可原样显式提交，此后才允许预期的新正式revision。正常完整退出/新原生身份及epoch后，全旧版/receipt/附件bytes、派生health和被保留文件再次核对。允许投影SQL/保留计费变化，不要求全DB bytes假守恒；不把旧v1自然存在说成stale health enum。

先只读analyze，再编译/焦点→去实际文件刺激有效red及错误目标version oracle red→同冻结普通exe三轮/相关vet；未变生产/包源按R100/R99/R97精确内容与集合复用。该R101首片当时不勾T023：随机新generation保留旧候选不等于原CREATE_NEW目标碰撞；实际原目标冲突后续见R102，真正WriteFile失败仍须精确补证。只读已知ACL继承假设、.test.exe-only Close切口或自选ID primitive不能冒称普通Host运行，不扩大系统策略/权限，不把T024九例新增为本片前置。

### 4.4.12 T023原随机generation的真实CREATE_NEW目标占用（implemented / R102，T023仍partial）

实际证据见[R102](<research.md>)：622输入，同build有效去占用red及实际碰撞三轮；当前normal/tag四包、两种vet与同normal exe的T020–T022关联代表链复验完成。诊断过滤前IO/报告custody及公开枚举oracle的缺口已修正并复验，不改真实native错误。以下保留设计；本片不替代原WriteFile失败或T024九例，不勾T023。

Owner A串行集成/C外部验收，原T023/V204、US4、FR015–019/023–024、SC006；沿现有physical callback，在原owner/root/当前check与generation解析成功后、原CreateProjection调用前与其返回后，各调用同一个无返回值同步观察，返回后的error按值只读。normal变体函数为空，不接收环境或产生IO；aboard_acceptance变体仅在指定诊断image、严格合成fixture/实际安装root、精确workspace/principal/operation匹配且原r.Validate成功时执行。epoch沿原OpenOwner已验证门，配置不能授予或自报epoch；外部原operation view及正常冷开验证真实epoch语义。生成ID只来自当前原SQL callback，不猜下个随机ID、不修改SQL/正式状态/授权/异常分类，不复用全局当前操作。

tag观察把完整诊断producer先发布到只用于持有的root，再在原目标创建目录与小型未应用candidate文件；原CREATE_NEW必须真正返回系统占用，由现有projectionCreateConflictKnown路径处理。所有文件第一次Close在调用前记attempt并缓存；未知/partial保留完整producer，不能retry Close或失败后删路径。配置拒绝/刺激失败只写有界脱敏诊断，不替换原业务结果；没有返回伪造error。报告绑定实际current scope、原key、opaque generation、真实native错误分类、candidate digest及首次Close事实，epoch只取外部真实operation view，不记录bearer/私有locator。原key查询必须绑定同代不再IO，新key重生必须独立成功且旧candidate bytes保留；正式Head/receipt/对象、B CRUD及完整正常cold open守恒。

先只读analyze，tag helper拒绝边界/正常noop验证→去占用有效red→实际外部诊断Host三轮；同时构建冻结独立normal CLI、证明normal无探针符号，并复验受影响installation normal/tag及原T020–T022相关普通Host链。此诊断Host不是普通产品，证据分开；源码变化后受影响当前证明在重验前保持unverified，旧历史不删除。真正WriteFile失败仍按R101/374的原生范围锁证据另片接线，不由本片或随机新代代签；不引入HTTP故障API/新权限/ACL/系统策略，不把T024九例变成新前置。

### 4.4.13 T023原manifest的真实WriteFile范围锁失败（implemented / R103，T023原任务范围完成）

实际证据见[R103](<research.md>)：627输入，原Write33/同FileID/peer首次Close已知而原writer未结束，辅助原key/reconcile不清理、预算/排他保留与B CRUD；同build去lock有效red后实际三轮、当前normal/tag四包/两种vet及关联Host层完成。proof-rekey与零字节oracle缺口已修正。结合R101–R102签原T023，以下保留设计；异常收集不是实际排空，T024九例与fresh/newepoch仍需原验收。

Owner A/C，沿原T023/V204、FR015–019/023–024、SC006/未知保留局部，复用4.4.12的严格diagnostic image/当前scope/key及pure拒绝门。原CreateProjection成功后、原writeFlushAndClose之前发布完整诊断peer root，在同manifest以只读共享句柄获取独占byte-range lock；核FileID与原writer.Identity相同，原Win32 WriteFile自然返回ERROR_LOCK_VIOLATION，不替换函数或业务error。新void同步观察只接在原write序列前/后，normal空函数不读环境/不IO；tag helper只释放自身peer首次Close，不调用原writer.Close、不改原objectTransfer/SQL/预算。after只按值观测原实际error、writerEndAttempted及scope/generation，记录peer/rooted报告的首次结束；失败/partial持续保留，报告失败不能改产品结果。

独立Host对去lock合法ready取有效red、真实lock case三轮；核原辅助key/生成pending无receipt、原SAVE receipt不变、零长度原manifest不被调查重写、GET/辅助reconcile不冒称清理、A pending/排他/保留预算不提前返、B新增更新读。原writer在WriteFrom错误后未Close是既有真实保留，不能由测试关闭。精确异常进程收集只回收test child，不称正常drain/closed或安全退额；剩余fresh/实际排空按原义务续验。异常收集使用现有严格fault helper的独立相位proof（PID/birth/image/key/真实Native33及B进展），不泛化kill权限。先readonly analyze，再helper门/normal noop及有效red/green；生产callback改变须当前normal/tag受影响四包与vet及同normal exe关联代表链，历史R102碰撞作为相关例，不用旧输入签新源码。上述原T023义务在R103实际完成后方签T023；不提前勾T024或将异常收集称drain。

### 4.4.14 T024正式附件verify的实际pending三例（implemented / R104，T024仍partial）

当前实际证据见[R104](<research.md>)：632输入，cancel/deadline/TCP断连三模式各三轮，真实不同进程kernel pending/实际join/Close/newepoch与B CRUD通过；同build去oplock有效red，当前normal/tag四包/两种vet和关联Host层完成。报告首次end/reader重试、有效current事实oracle及Windows报告发布命名冲突的测试缺口已修正；以下保留设计与失败反馈。R104本片当时不勾T024，另外六例中projection后续见R105；publication后续见R106，现原安全共享边界仍沿原义务。

Owner A/C沿原T024/US5、FR002/012–014/018–020/023–024、SC007验证阶段及SC008局部/V205，先实现三例，不把publication final-freeze/投影health当verify。原HTTP附件content GET经原SQL正式关系选择Object/长度/hash，再执行原withResourceObjects/FreezeObject。新增本仓strict tag只读同步观察桥：normal context透传/空函数；tag只在指定diagnostic image、现有私有fixture、实际当前Authorization principal、A scope、精确resource/version/attachment/Object/FileID及原r.Validate成功后，锁住原调用OS线程并记录before，真实返回/原清理后记录after并解锁；不替换错误/句柄/授权，不新建普通业务入口或放宽平台注册guard。

阻塞由外部验收进程在已正式对象上持有真实batch oplock，完整file/event/OVERLAPPED/Pinner/线程probe在fallible acquisition前root。独立进程的blocker IRP先触发break且GetOverlappedResult证明完成，再以before给出的原Host PID/birth/TID打开只读thread probe，核该原线程持续GetThreadIOPendingFlag、HTTP未返回；不能用blocker自身pending充数。三种动作分别真实Context.Cancel、实际Deadline及真实TCP断连，只撤销等待/交付，不CancelSynchronousIo/Close原handle/kill来结束原IO。每例再核原线程仍pending、A close只executing/notclosed、实际第二进程排他拒绝、旧epoch新写cutoff、B新增/更新/读。然后仅释放自有oplock：其原IRP实际join/first Close，原调用after证明返回/原附件first Close，close已drained、真实重新Acquire、明示reopen新epoch及完整正式bytes/旧receipt，最后完整正常Host结束。

借用既有001-validation/t023-*合成fixture及有界report reader/producer，不伪称平台002-validation writer注册已适用；本片verify不调用该writer注册。配置中formal tuple不是新的下载能力，仅匹配已通过真实SQL/授权的调用；其他主体/区/对象/坏配置在文件阶段前拒绝。normal noop、tag拒绝unit及去掉真实oplock的有效red先建；同一冻结build正常三例三轮和当前normal/tag受影响包/vet/相关Host代表链；源码与产物/root/C集合如实冻结，所有旧失败保留。未知native首次end留root并失败，异常收集从不算drain。本片不勾T024，publish/projection另六例及全route安全/共享unknown仍按原义务。

报告修正的实际技术证据：无原Host pin的tag unit可Move发布，但实际Host三例的MoveFileEx及随后独立hardlink方案均返回32，且明确是发布阶段、原文件首次Close已知；两个机制证伪保留，不重试或放宽sharing/删除保护。当前固定将报告写入`.pending`、Write/Sync及第一次Close均known之后，仅以同root一次Mkdir创建`.known`完成标记（不返回活句柄、不覆盖）；reader只在marker为目录时读取不可变原`.pending`文件，marker不存在才轮询，单元校验完整original+marker类型。不是失败后切换fallback，不是原对象的发布或业务修复。已Acquire报告的first-end/读取/JSON错误不重试，只有未Acquire的NotExist轮询；纯current事实门有有效正控与每字段独立负例。该修改仅诊断报告交付，仍须原native Freeze真实pending与join，不能用report IO当原物理阶段。

### 4.4.15 T024投影final-freeze实际pending三例（implemented / R105，T024仍partial）

实际证据见[R105](<research.md>)：637输入，三个真实交付模式各三轮、同build去oplock有效red、当前normal/tag四包/两种vet及关联Host层完成。原writer先已知Close，再独立parent IRP完成+原线程实际pending，排序等待不算物理证据；accepted POST不因交付取消而取消业务，最后真实join/drained/newepoch/原key同generation/bytes与receipt守恒。probe thread/partial lease的独立首次end，以及有效配置/原key事实unit缺口均已修正。R105本片当时不勾T024，publication后续见R106，现原安全共享边界仍待；以下保留设计。

Owner A/C沿原T024/US5、FR008/011/015–020/023–024、SC007投影及SC008局部/V205，依赖已验T023及R104共享blocker验证法。只在原投影writer Write/Flush/已知第一次Close之后、原FreezeProjection前后增加normal void空观察与strict tag观察。tag纯image/config及真实当前scope/principal/key/正式resource/version、原writer已结束/原FileID等门先于文件阶段，完整root记录PID/birth/原OS线程/generation/FileID及原physical child。以R104的known-ended不可变pending报告+完成目录交付before，再有界等待parent的就绪目录，仅供新随机generation上真实外部batch oplock建立的排序；这段回调等待不算pending。parent实际授予oplock后发布ready，观察返回，唯有原native open打破目标oplock且GetOverlappedResult证实parent IRP已完成，同时原不同进程TID持续GetThreadIOPendingFlag，才记录真实pending；不放宽旧平台注册guard或新造error/业务入口。

三例分别真实取消客户端Context、客户端3秒Deadline、TCP RST。已验证POST原本WithoutCancel，交付结束不能取消已接收业务或伪造rollback；测试不更改该语义。期间A close executing/实际排他、旧epoch新写cutoff，B新增更新完整读取。只释放自有blocker，确认原Freeze/ready/原Frozen首次Close返回的after与原physical事实，再Close drained/实际Acquire重新成功/newepoch；新epoch原投影key已知结果、同正式版本/原SAVE receipt/附件bytes及原generation健康一致，不重放新key生成。parent file/event/OVERLAPPED/Pinner/thread/partial lease全在fallible acquisition前root；不kill/CancelSynchronousIo代替join，首次结束未知保留并失败。

strict helper有效事实正控/逐字段破坏、normal空行为、报告已知first-end及reader不吞错误；先只读analyze→去真实oplock有效red→同build三模式各三轮，当前normal/tag受影响四包/vet及同normal/diagnostic关联链。报告/ordering超时只拒绝诊断证据，不替换业务结果；observer不关闭原writer/Frozen。R105当时publication三例及原安全义务继续，publication后续见R106；原全route安全共享仍待，不勾T024，不将九次projection运行称全阶段九例。

### 4.4.16 T024对象发布final-freeze实际pending三例（implemented / R106，T024安全部分仍待）

当前实际证据见[R106](<research.md>)：642输入，原Gate准入后WithoutCancel与原FreezeObject真正kernel pending、三交付模式各三轮/有效去oplock red、actual join/Close/newepoch及原key同ObjectID ready无receipt、实际v2正式引用bytes完成。当前normal/tag四包/两种vet、同build旧verify/projection六组合及其他关联链通过。最初错误公开Name字段和缺对象长度header均保留为无效attempt，不冒称产品问题。R104–R106物理九组合均已验证，以下保留设计；原全route安全/共享unknown仍须完成，不勾T024。

Owner A/C沿原T024/US5、FR002/004–006/011/018–020/023–024、SC007 publication与SC008局部/V205，依赖已验T023及R104–R105方法。原binary POST不走通用WithoutCancel，但原CommitGate.TransferObject在新持久准入后已用WithoutCancel调用唯一physical；不改此边界。仅在原object writer成功Write/Flush/第一次Close之后、原FreezeObject前后加normal void/strict tag观察，配置匹配原key/principal/scope/declared length/hash和原lease/pin/owner，ObjectID只来自原SQL回调。锁住原OS线程并发布其PID/birth/TID/FileID及原ObjectID；报告已知首次end后完成目录交付，有界ready目录只排序parent在新对象上设立真实oplock，不算IO。原native open打断外部blocker的IRP且实际完成、原不同进程thread持续kernel pending才是阶段证据。

三个交付模式为context cancel、真实deadline与TCP RST，发送完整准确body后在已知writer关闭/真正final-freeze pending时结束交付，不将短上传混入本片。原accepted callback仍按原finish上下文继续；A close executing/实际排他/旧epoch准入cutoff，B每例新增更新读。parent file/event/overlapped/pins/thread/每次partial Acquire各有完整独立root/first-end，未知保留不重试。仅释放自有blocker后核原Frozen第一次Close/physical返回和ready持久结果、actualdrained/原lease重获/显式新epoch/原object-put key同ObjectID且无receipt，原正式v1/bytes/receipt不变；再将该已就绪对象经真实preview/confirm作为v2附件读取原bytes，不能只以metadata或raw object路径代签可引用性。若实际出现unknown按原语义保留并诊断，不凭取消假定rollback/退款，也不以kill代替join。

沿R105已修正probe与producer/reader：pure image/config及当前真实事实门先于IO，有效decode/current facts正控+十字段单破坏、normal noop；不放宽平台注册guard、不造error/新业务能力。先只读analyze再实现，去oplock有效red、同build三模式各三轮、当前四包normal/tag/vet及同normal/diagnostic关联代表；输入/source-set/两类exe分列。本片完成前不勾T024；九种物理阶段组合之外的原全route auth/共享unknown/脱敏义务也仍须真实覆盖，不新增规格外推进门。

### 4.4.17 T024独立Host安装安全unknown与M2全route拒绝（implemented / R107，普通授权与foreign关系仍待）

实际证据见[R107](<research.md>)：643输入，原installation FSYNC1034/零open owner门，26实际route与六原TCP before-body拒绝、源/实际lease保留、精确异常收集及原安装key cold显式调查/target与旧candidate拒绝源守恒、fresh预览及B写通过。真实body custody、两区秘密集合、target重放oracle三处验收缺口已补；同build去注入及B正文/body-header-trailer共四有效red，完整三轮/相关vet与同normal代表链完成。未改产品或旧检查定义，按逐源/集合/报告哈希复用R106完整normal/tag；以下保留设计，不勾T024，普通当前授权及有权限后的foreign关系仍按原任务验。

Owner C沿原T024/US5、FR002/011/020/023–024及SC008共享依赖/V205；依赖T023及物理九组合已验。本片只新增外部acceptance源，不改产品/契约或故障API。normal CLI先在同一安装持久化非空真实M2 A/B、完整正式附件/原SAVE/投影及未应用candidate，记录原bytes/receipt/有效body后正常close并结束全部原Host；复用既有finalizeFixture(t,f,key,1)、joinOriginalSessionDelivery，在diagnostic新Host不open任何workspace，对严格allowed/finalize-target的原create使真实installation FSYNC1034产生unknown。保留zero-open-owner/原handle原owner全部guard；不把workspace saveNativeFixture当安装安全故障，不额外要求live-held M2票据组合。

实际context、A/B完整读/历史/legacy/附件、旧object/SAVE/projection key、status/区内budget、object上传/preview/confirm/再生/三类调查/close及安装budget/view/原key调查共二十余个合法入口，全部须503/INSTALLATION_STATE_UNKNOWN、unknown/installation/investigate/no-store/关联一致，零内容/receipt/秘密。POST追加Expect:100-continue原TCP合法header但不发body的证据，首响应必须终态503而非100，证明共享auth拒绝先于读取body及closed/旧epoch业务判断；非body路由不冒称零读指标。对原JSON/未知字段/解码路径/headers/trailers及Host事件扫描，A/B全部source bytes不变、安装真实排他仍占用；不要求安装库在真实故障本身后等于故障前，也不将未知Host终止称drain。

复用既有unresolved-installation-cleanup精确helper只收集该已证实PID/birth/实际unknown子进程，然后normal新Host认证、GET原安装key及显式reconcile取得原已知结果，不重放create。重新open A/B验证原所有正式内容/对象bytes/receipt；旧candidate的preview epoch按原resource_commit.go:102已陈旧，必须拒绝且不消费，重新预览同完整输入后才可确认，B继续新增更新读、全部正常结束。此处在实现前按原代码校正测试假设，不改变原epoch/候选语义。去真实安装注入用原fixture ordinal0，仅到首个共享拒绝断言应实际失败，不把成功DTO误解码造成的失败当有效red。先只读analyze，再compile/focus/同build red/三轮/相关vet；生产源/旧检查定义逐文件和path-set未变才复用R106当前normal/tag，另同normal exe重验M2对象/历史/并发代表。不勾T024，普通授权/外区关系独立Host差集仍在原义务，不新增门。

### 4.4.18 T024普通Host真实A-only、当前版本撤销及自然过期（implemented / R108，foreign关系仍待）

实际见[R108](<research.md>)：644输入，普通Host完整case三轮、每轮125拒绝/30before-body、真实A读写/旧未到期grant_version失效/短token先正控后自然过期；同build owner bearer及秘密观察两个有效red、相关vet通过。预算去除每readcorrelation后保持其余状态全比，空bearer client仅新测试构造、不改旧helper。以下保留设计，T024仍不勾；当前有目标权限的foreign关系按原图继续。

Owner C沿原T024/V205、FR002/020/023–024、SC008普通权限补差；R106物理九组合与R107安装unknown不重做。仅新增普通acceptance源，复用既有strict principalFixtureSession两次离线helper、三次Host生命周期：普通CLI持久化M2 A(实际principal-target)/B后完整停机；read-write phase以真实独立DB主体A读写正控，B全适用合法route/不存在目标及安装operator入口拒绝；完整停机后read-write-short同grants替换递增grant_version，旧5分钟token仍在期限内却在context及适用route被拒，新3秒token先取得实际A正控，再真实到期后全route拒绝。没有read-only phase，不伪造Grant.Action降权或新建权限接口；现reader-only负例R85按精确源指纹复用，不扩SC008为全角色笛卡尔积。

复用resourceSharedWire真实transport/root/首次Body和socket end，合法对象与JSON的Expect-before-body须第一响应403而非100，统一FORBIDDEN/rejected/none/no-store/关联且不泄existing/missing内容；无认证/坏token另有真实普通HTTP负例。A/B实时全源inventory仅豁免精确writer锁metadata，正式DTO/receipt/epoch及预算保持；合法正控/授权替换建立各自新baseline，不要求合法审计/授权动作全库不变。上下文认证拒绝发生在route匹配前，scope按原实现校验，不把安装管理缺operator说成B权限。所有公开原JSON/未知字段/解码路径/header/trailer及完整日志用既有隐私检查；结束后正常close/stop。

先只读analyze，再通过仅把第一条拒绝的观察请求换原owner bearer取有效200负控，不改正控身份或产品grant；另以原隐私scanner观察副本私有内容取有效red，普通完整例三轮/相关vet。同build保留principal helper与当前normal符号检查，按源/集合/报告哈希复用R107/R106未变生产包回归；不改旧helper/test定义。此片只覆盖SC008列出的A-only/未认证/过期与授权版本撤销，T024有目标权限后的foreign关系仍单独剩余，不勾整项。

### 4.4.19 T024普通Host已有B权限后的foreign关系拒绝（implemented / R109，原T024范围完成）

实际见[R109](<research.md>)：645输入，普通当前B权限/正确epoch及base的十关系拒绝、本区同bytes正控、两原candidate分别确认及完整冷开case三轮，两类同build有效red/vet通过。新源码仅acceptance，递归精确复用R108–R106生产/包证据；只读复核无范围内剩余机制阻断。组合R104–R108签原T024/V205，不把本片扩成新角色/故障门。以下保留原设计；T025后续任务仍须其独立真实验收。

Owner C，原T024/FR002–003/005/008/010/012/015/023–024、SC008/V205最后独立Host补差；依赖已验T023，接R108身份层不重复A-only矩阵。仅新acceptance源，现有normal CLI/既有pipe当前owner真实拥有A/B权限；两真实M2相同对象operationId/相同payload/name仍独立ObjectID/附件/正式resource，分别原SAVE正控。A有效未应用完整candidate、B正确当前epoch/基线；搬A ready ObjectID至B完整preview、搬A candidate/digest/previewVersion至B新key确认均409/STALE_BASELINE，不以403/陈旧handle遮盖；B本地同字节ObjectID能preview201。A resource/version/attachment组合搬至B current/exact/history/content/projection应404，B本地resource配仅A attachment也404，B本地正式版配A两个digest再生409；B查仅A原SAVE key得409/HISTORY_UNAVAILABLE而非A receipt。

每个真实原TCP拒绝先查精确status/code/outcome/retry/scope/no-store/correlation，再扫原body/未知JSON/decoded私有路径/headers/trailers；原resourceSharedWire先root response/body并缓存首次end，不沿用legacy预Close transport。每例完整双区正式DTO/所有附件原bytes、原receipt/Head/current epoch守恒；允许合法审计/预算/DB计数变化，不把所有授权后拒绝误套全库bytes不变。末尾两原candidate分别按本区key真正确认，证明未跨区消费，完整正常冷开再读新旧bytes/receipt。去foreign ObjectID刺激（换真实B对象）应在原拒绝断言看到201有效red，秘密观察副本进原scanner另red；同冻结normal build两red后case三轮/相关vet，生产/旧验证未变按R108→R107→R106精确源集合/报告链复用。先只读analyze再执行，不新增角色/helper/故障API；成功后才按原T024全部R104–R109证据整项签收，未运行时不预勾，T025–T029及003继续实际依赖。

### 4.5 持久预算与恢复

安装唯一字节总额及有效候选总限＋每区已承诺bytes/candidateSlots，候选slots合计≤安装总限，字节分配合计＋安装增量元数据保留/unknown预留＋有限控制证据预留≤安装字节总额；区DB只计本区预留/真实保留/有效候选依赖，不跨库每操作双扣。关闭/重启不送新分配，终态只退区内有效占用；调小安装名额承诺也需closed/排空/fresh/CAS，unknown依赖不能退。调额是当前operator、preview/CAS/original key事务，区额改变需closed/排空/fresh保留事实；不得下调至已承诺/unknown依赖以下。无GC/清理，只有增加空间＋显式有限扩容，硬最大后如实需后续批准。元数据/失败残留也计；未使用预留退还须正面证据，真实保留不因TTL退额。G204核心证明有限逻辑计费、元数据/失败残留/unknown保守预留与控制证据余量，不能当纯payload计数器；完整SQL/NTFS物理峰值上界及真实盘满为G204延后发布子项，不表示磁盘写成功保证。核心逻辑计费模型须在T006明确可计算单位/增长及原key不双计，T009落实到生产Store/唯一namespace/control，T025真实Host验证，不要求T006原型提前完成尚未实施的T009整链。

### 4.5.1 FR-025预算可观察性补缺

这是既有FR-025的读面补齐，不重开规格、不把T009内部记账抹成未完成，也不将内部计数当公共交付。由A在T034–T036串行集成，先只读分析再实现。下文保留设计与验证义务，当前状态只见[tasks](<tasks.md#当前状态>)；实际实现与分层验证记录见[research R75](<research.md#r75--fr-025本区逻辑预算只读链implemented--v212局部验证不代签v206>)，不代签T025/V206。

- 唯一OpenAPI增量冻结`GET /v1/workspaces/{workspaceId}/retention-budget`与独立响应schema，不在用于调额输入的`WorkspaceRetentionAllocation`加入只读计数。当前`workspace.read`、显式scope、原HTTP claim/ordinary队列/原OpenOwner/原native lease；不借安装operator或writer权限，不开第二DB/owner/pool，不把普通预算读变control。
- 原Gate/Conn的一次只读快照取本区`charged_bytes/reserved_bytes`及保留charge聚合；保守retained=`max(charged_bytes,sum(amount))`，reserved=`max(reserved_bytes,sum(held_bytes))`，unknown retained/reserved是各自子集，**不得再加一遍**。available=`allocation-retained-reserved`，全部checked非负整数/十进制字符串，溢出或不一致拒绝，不夹成0。快照实际结束失败保留原owner/fence，无部分成功、清理、退款或新权限。
- revision语义：安装budget全局revision、目标assignment自身revision与区applied revision不是同一代次；全局revision推进而目标bytes/slots/profile不变是合法状态，不要求三者相等，不要求为GET新写同步projection或全安装停机。T034须固定字段来源及目标值/profile对应关系，再按现有closed/fresh门核验。
- 必须区分安装当前assignment与区内已应用assignment/profile/revision；不把陈旧本地allocation冒称当前可用额度。实现需在同一原工作区操作/连续生命周期排他下，从现有安装owner以当前读权限投影目标assignment，再在本区快照核对；安装调额必须closed/fresh的既有门不能被绕过。不能证明一致则unknown/unavailable，不拼无关联的两库数值为原子或缩额证书；只读GET不更新本地projection。具体字段在T034的唯一OpenAPI冻结并测试，不另手写一套DTO。
- 在可信SQL metadata和原lease仍可读时允许报告未结束/unknown逻辑预留，但不宣称物理已结束。未知SQL事务、失效lease、Close未知/共享授权unknown不能读时如实拒绝，不显示零占用/全额可用。LifecycleUnknown读面不得清除NeedsInvestigation、原hold、fence、待调查key或close门。安装GET仍是安装承诺/控制余量，不汇总伪跨库实时快照。
- 有效red→green覆盖：正常及边界算术、metadata/失败残留计入、unknown不双加不退额、assignment不一致与撤权/串区/旧profile/未打开拒绝、读取前后SQL/目录/原hold不变；当前真实HTTP与原生持锁读、公开DTO脱敏、重复读不双计。T036复验契约生成/双消费者及受影响normal/tag/vet，记录输入/命令/退出码/非空选择器与SKIP。最终真实Host满额/调额/完整重启由原T025/V206验收，不拿组件测试代签。

### 4.5.2 T025普通Host预算并发CAS、原key与非空区fresh调额（implemented / R110，T025仍partial）

当前实际证据见[R110](<research.md>)：646输入，同冻结普通Host两有效负控与并发/fresh/cold/无operator case三轮、相关vet完成；旧key snapshot不双计，未变包按精确源/集合/原报告链复用。以下保留设计及实测校正，不把首片签成完整V206。

Owner C沿原T025/V206，FR002/011/018/020/023–025、SC009管理局部/SC013预算正常与并发/重启。依赖T024已验，先补最小真实生产链，不把组件内部记账或runtime32/64与持久CandidateSlots混为一谈。普通CLI创建非空M2 A/B（A使用既有principal-target严格helper目录），两独立当前operator会话在同一revision准备两个不同总额preview，证明preview不改正式预算/工作区源；实际并发确认必须仅一正式CAS胜、一精确冲突，两producer全部join后再断言。完整Body/response在Do前分别root，first Close缓存，错误不重试；不在worker调用testing方法。

胜者原key相同输入重发/GET/显式reconcile返原typed budget、installation scope、committed/drained/known、无SAVE receipt；改digest/陈旧revision精确拒绝。然后正常关闭A，仅改A原allocation bytes（小增量、不造大文件），沿现有closed/fresh原生命周期/native pins至同CAS；B原分配及正式内容不改。此处必须实际覆盖retention_budget_fresh.go分支，不用新UUID静态预留冒充非空A。完整正常结束Host/新原生会话、新epoch后核预算revision/profile/bytes/slots/charged/unknown未双计、两区原正式bytes/receipt，并在进程内preview cache已消失时重发两个已知原adjust key/GET/reconcile，不能因旧revision重做CAS或退款。仅忽略请求correlation，不抹计费/持久预留字段。

最后复用原strict principalFixtureSession的read-write阶段，在实际正常结束后给A-only真实DB主体；先A读正控，安装budget GET及两POST拒绝、原TCP Expect100须在body前403，三次角色操作不改预算/正式源，raw/decoded/header/trailer/日志脱敏。无需新helper phase或产品API。两有效观察负控分别用合法fresh-baseline第二CAS制造真实双成功击穿一胜一冲突oracle，以及在真实拒绝观察header放B正文击穿原privacy scanner；不改产品响应/SQL/正式授权。先只读analyze，再焦点→同build两red→三轮normal case/vet；仅新acceptance，按完整source/pathset/报告链复用R109–R106未变normal/tag。

实测反馈的测试假设校正：安装resize成功并不使区内已应用assignment自动变新；按既有§4.5.1，纯GET必须拒绝不一致且不能写同步；原ReadRetentionBudget把未适用值不一致返回ErrSchema，现有HTTP精确映射409/UNSUPPORTED_SCHEMA，不凭默认failure码猜RECOVERY_REQUIRED。新增验收先核拒绝与源不变，再以原A真实有界preview/SAVE让原writer适用新assignment，之后才要求区预算新值；不为green改产品GET/ordinary-owner语义。

本片不勾T025：持久slots安装全分配但各区未满、区有效候选/metadata空间分离、runtime32/64真实full、A满B与全满管理/close、真正budget事务unknown/fresh三态另依原义务续验。已只读定位原组件真实FSYNC1034及尚无budget专用Host tag桥；不拿creation finalize或workspace SAVE故障代签，不增live-owner组合门；原生盘满/物理上界仍deferred发布验收。

### 4.5.3 T025普通Host持久slots全分配、区内候选与空间分离（implemented / R111，T025仍partial）

实际证据见[R111](<research.md>)：647输入，安装4096/16384 slots全承诺而A/B各一有效候选仍未本区满、A closed/fresh缩1并保留全安装承诺、B CRUD/管理原key/close、终态slot与retained分离及cold候选不清零；两有效负控后两个profile各三轮和纯first-end/decoded-key回归/相关vet通过。正向response与全Host最终日志机制缺口已局部修正；以下保留设计，不把安装slots全额称runtime/保留bytes满额或整个T025完成。

Owner C沿原T025/V206，FR002/011/017–020/023–025、SC009及SC013局部，T024依赖已满足。仅新增普通acceptance源，既有生产/契约/helper不改。default slots128/4096与显式确认的maximum-slots512/16384、control64MiB两个有限profile分别测；max须在allocation空时原operator preview/adjust，其他上限不冒称全部max。两个真实非空M2 A/B保留原bytes分配，新增30个不对应目录的新UUID零bytes/slotted正式安装承诺，总32条恰占满安装slots。A/B各一实际有效candidate仍低于各自slots，完整formal版本/receipt/bytes/epoch不变；再加一slot的operator preview须429/无新正式预算，不能把runtime或64进程previewcache当此额度。

先实际确认两原candidate，证明有效slots退回但retained bytes不退/不删原key/正式历史；关闭A并沿原fresh把其slots缩到1，另加一新UUID承接释放的S-1使安装仍满（不改已有占位承诺）。新epoch原writer产生一A候选即区满，下一候选429且零错误成功；B仍真实新增/更新/完整读取。全slot分配下原operator仍可查看、原key/显式调查和有界total调额，A/B已知SAVE调查及关闭可用。留一个A有效候选，完整正常停止/冷开后安装全承诺及区slot1/retained不清零、旧epoch新确认仍拒绝，再核B及完整历史。未知/依赖优先于TTL已有源规则在后续真实unknown另验，本片不凭15分钟TTL快进或重启清理。

所有新response/socket/producer沿既有first-end/root规则；私有原始错误/未知JSON/decoded Windows路径/headers/trailers/日志继续扫描。实测首焦点纠正错误码假设：全安装提议总slots超profile上界在原validateInstallationBudget先返回ErrSchema→409/UNSUPPORTED_SCHEMA，本区有效候选满仍429/CAPACITY_LIMIT；不为测试重写原错误枚举。负控一少留一slot使新preview实际201击穿安装409拒绝oracle，另一只在真实拒绝观察header放B正文击穿原scanner。只读复核指出旧成功helper忽略Body首次Close、日志仅扫末个Host；本片以不预读Body的test-local正向transport在真实RoundTrip前完整root，partial response及首次end缓存并阻止未知后新请求，拒绝wire仍用原nil-transport client；全Host最终bounded capture在既有process cleanup之后连同提前失败路径扫描，纯unit验证未知end只试一次及原/decoded秘密；不改旧共享helper或产品。先只读analyze，再焦点、同build两有效red、两个profile各三轮及相关vet，逐源/集合/报告SHA复用R110–R106未变包；readonly交接仅Doc3差异。保持T025未勾：runtime32/64、精确持久bytes/metadata满额、真正预算unknown与未结束依赖以及物理盘满deferred仍按原图，不把本片slot全分配称卷物理全满。

### 4.5.4 T025原预算writer的真实安装FSYNC未知首片（implemented / R112，T025仍partial）

实际证据见[R112](<research.md>)：652输入，原static预算SQL writer真实1034与同ref/Usage1/安装fence/source/排他保留，已知0控制和1实际unknown各三轮；去native有效red、当前normal/tag四包/单位/两vet、同build普通对象/历史/并发及先前预算/slots关联通过。fresh本轮只观测完整old/no-key，不以异常收集当Store first-end/refund，不外推new/unresolved或fresh-resize全部分支。真实Store/Owner/quiet单位、整份new期望及positive first-end/全部最终日志缺口已局部修补；以下保留设计，未勾整项。

Owner A/C沿原T025/V206，FR002/011/013/018–020/023–025、SC009未知保留局部/SC013，依赖T024已满足。首片只覆盖原static total CAS的新预算事务，fresh resize另按实际后续证据；不以create/SAVE故障代签。先普通CLI持久化非空M2 A/B及原receipt/完整bytes，所有workspace真实关闭、正常结束Host；诊断重新原生认证但不open workspace，原public preview提出同profile/allocations的新total。预算专用严格tag桥只在原retention_budget.go静态writer调用边界包装原回调，normal为直通；原current operator/Store.mu/immutable BUDGET-ADJUST/1实际input、预期revision/key、installation DB/pin/lease/零owner/no other probe先验证，再复用既有永久VFS delegate及CAS arm，在精确安装DB/journal的真实原FSYNC上以已验证只读config句柄触发Win32 BOOL0→SQLite1034。不得恢复正在用的VFS指针、伪造SQL错误、新增HTTP/pipe接口或改变普通授权/owner。

诊断完整root在任何native Acquire前建立，stimulus句柄首次Close缓存，不重试未知；CAS disarm后join原callback再结束其自有stimulus。原owner先处理真实sqlite.ErrUnknown并保留原ref，另一个normal空/tag只读观察点在原Store.mu下记录实际key/full inputDigest/native1034/authUncertain/原reservation同一性/ordinary Usage1；观察不得Close原Store、清unknown、放大runtime或释放原hold。报告同R104 known Write/Sync/firstClose后单次目录marker，私有bounded，不使用report rename/link fallback。guard/config不匹配不得arm，image/完整有效facts正控及逐字段反例必须分开测试。

独立test先ordinal0实际已知正控，再ordinal1实际事务unknown，保留完整原输入、source/实际安装排他和原key GET/retry/reconcile均共享unknown不重放。仅在精确原PID/birth/image、report原key/input及native未知已验证后复用既有unresolved-installation-cleanup异常收集；明确不是正常drain、Store first-end或退款。新normal Host必须从整个可信安装事实读原budget及原key：完整old无key只允许明确不可重放（旧preview cache已丢），完整new须同原typed输入/result重发不双计；普通fresh无法可信打开则保留journal/源并报告unresolved，不从缺key推old，不删残留或自动重放。现有组件Store真实Close/ref释放证据仅作为其边界，不能替代这次独立Host异常结束。

先严格只读analyze，再可编译guard/单位与焦点。去真实故障ordinal0使原unknown断言实际失败，同build正常三轮；生产调用边界变更须当前受影响normal/tag四包、两vet及原普通代表链，不仅复用R111旧源。原runtime/保留bytes/metadata满额、未结束依赖与更广fresh三态/延后物理ENOSPC仍原T025，不提前勾选。

### 4.5.5 T025普通Host真实保留floor、安装逻辑bytes满额与有界管理（implemented / R113，T025仍partial）

实际证据见[R113](<research.md>)：653输入，非空A精确retained+reserved floor/meta大于payload、F−1拒而F成功、A新metadata和byte均429且B CRUD、安装请求total全额后一byte429、旧key/关闭/有限fresh扩容和cold不退款；两有效负控后整例三轮及first-end纯回归/vet通过。合法text与请求前完整秘密登记已修，global请求total映射按实际外层plan纠正而非改产品。以下保留设计，runtime/依赖余项及整个T025不代签。

Owner C，沿原T025/V206、FR002/011/017–020/023–025，SC009/SC013局部，依原T024已验。只新增独立normal acceptance，不改产品/契约/helper。原A/B有正式附件和receipt，公开读A retained+reserved并证明metadata计费不止payload；正常关闭A后以原fresh source将原allocation缩至精确F，F−1须501/CAPABILITY_UNAVAILABLE且不动预算/源。新epoch下assignment未由writer应用时纯GET仍409/UNSUPPORTED_SCHEMA、不做repair；A原合法preview和实际字节上传均429/CAPACITY_LIMIT，零新candidate/正式写，旧key与完整正式读取仍可用，B实际新增/更新/完整读。

安装total显式缩到controlReserve+installationCharged+unknown+所有原allocation精确和，不填卷/不删承诺；再请求新UUID一byte/零slots的提议在原common plan请求total边界429/CAPACITY_LIMIT：installation_budget_plan.go:52–53将相对请求total的校验失败映为ErrCapacity；这与installation_budget_fresh.go:39–43先按profile ceiling校验slots而报409不同，不能凭低层validate返回值猜外层错误，也不能把本片逻辑bytes满额说成runtime饱和。保留原结果key和精确预算/源，已分配B仍可写、原key/close与控制reserve内有限扩容可用。A真实关闭后原fresh同时有界扩total和A allocation，再由正常writer应用并实际保存，确认metadata继续计费；完整正常停机/冷开核全部预算及分配不清零、retained不退和原typed结果/正式bytes/receipt。控制reserve有限，本片只验有限恢复能力，不保证无限管理。

正向复用R111 root/first-end transport，拒绝仍用独立原nil-transport wire，提前注册全Host最终日志/raw及decoded keys/values、headers/trailers隐私，所有expected来自前态与精确算术而非observed自证。负控仅在本片A floor额外留1MiB使真实preview201击穿429；另一在原拒绝观察header放B正文击穿原scanner。先只读analyze，再焦点/同build两red/三轮及vet；仅新acceptance与Doc3，按源/集合/报告SHA复用R112当前normal/tag。不代runtime32/64、未执行TTL快进/未知变体或物理ENOSPC，上述余项仍T025，不提前勾。

### 4.5.6 T025普通M2 Host的runtime ordinary满额与实际结束后返额（implemented / R114，T025原任务范围完成）

实际见[R114](<research.md>)：654输入，普通M2 A31/BCRUD/global63、原body400/已知first-end/公开返额再占及正常cold三轮；同build两个有效red、原32/64 control组件三轮/vet通过。当前C补原SQL TTL/unknown依赖组件各三轮、原native Store实际first-end四ordinal及同普通预算/slots/floor代表链，四步全0。结合R110–R113签原T025，不把runtime ordinary、内部control、逻辑保留空间或物理盘满混称；以下设计保留历史，原T026及后续仍按前置。

Owner C沿原T025/V206、FR002/011/018–020/023–025八项、SC009及SC013运行界局部，依原T024已验。只新增普通acceptance，生产/契约/旧helper不改。normal CLI真实非空M2 A/B各有正式附件/receipt，预登记全部已知秘密、正向response完整root/first-end和最终所有Host流。独立原TCP每条holder在Dial前完整root，保留唯一bufio.Reader和各自interim/final Body首次end槽；合法GET带Content-Length1/Expect100但不发body，实际100证明ordinary准入。A恰31条时第32条final429且不发body，B实际新增/更新/完整读后A仍429；再B31和安装context1恰global63，额外context/安装管理请求、终态旧SAVE key和新close的适用请求均真实prebody429/原路径key关联，不编造unknown。原HTTP控制路由也ordinary，不要求第32/64条HTTP成功或宣称这些满额下known terminal key绕过池；持久空间满管理已由R113另验。

全压力从开始到完成检查须小于12s，早于既有15s ReadTimeout，原holder socket没有被提前关闭。只给原context holder补齐一space，读原final400/INVALID_INPUT并记录真实Body及socket第一次Close；后以有界真实GET重试仅429直到200证明原handler finalizer实际返额，不以单纯socketClose当join，再用新context holder把同一席位重新占满并复验global429。接着逐个补齐A/B原body、读400/known end，公开正式事实/epoch/source无误改、原key不重放，实际close/正常Host结束及新epoch完整cold正式bytes/receipt。failure cleanup只结束自有socket、保留unknown first-end且报错，不将失败cleanup补签join。

先只读analyze后实现。same-build负控只不持有最后globalcontext，使实际100击穿429 oracle；秘密观察header击穿原scanner，恢复后三轮及相关vet。仅为含control总32/64补跑现有TestReservationPrebodyControlCannotPinEitherReserve当前组件三轮，明确它不是新HTTP逃生通道，未改定义。完整normal/tag按源/集合/报告SHA复用R112，原unknown/TTL依赖按既有实际证据逐项映射，不新增live-owner优化门/产品故障API或把普通header等待说成SC007物理IO。T025整项在本片及原剩余证据复核完成后才签，不代T026–T029或003前置。

### 4.6 已接受架构方案的近期保行为切片

**R69历史实施截面：首片implemented / 仅局部验证。** 实际源指纹、有效red、恢复后green与受影响回归见[research R69](<research.md>)和[323回归记录](<../../.data/logs/002-validation/refactor-writer-323-regression-results.json>)；T016及原后续独立验收不由此完成。以下保留首片的设计依据与原验收义务，不把设计符合性或只读审阅当运行证据。

总体目标、插件与领域边界、五个切口及长期语义升级仅在[scope §2.1–2.2](<../../docs/scope.md#21-保留内核的分阶段架构重构designed实现状态见所属任务>)维护；当前权限只见[统一记录](<../001-host-workspace-foundation/authorization.md>)。本节拥有002的具体实现与验证，不另建新feature/路线，不修改spec/Q1–Q3或原T001–T029编号。首片开始时的历史基线为R68的dirty工作树：T001–T015完成、T016仍partial；当前health源码等已有工作全部保留，不据静态存在勾选T016。

**首片选择**：在`internal/installation/workspace_object_transfer.go`的原`objectTransferPhysical`增加私有`writeFlushAndClose(source io.Reader, length int64) (platform.Identity, error)`，只复用两处相同的 `WriteFrom → error → length → Flush → Identity → writerEndAttempted=true → Close`。`workspace_resource_projection.go`调用同一方法并移除不再需要的io import。仍用原具体native writer，不新增interface/factory/参数框架，不创建新包、连接、owner、registry或容量池。责任人A（主会话）串行集成；其他会话仅只读审阅。

**为什么不抽完整child runner**：projection有nil-child且精确CREATE_NEW冲突、再次授权后可知拒绝的独有分支；上传没有；health的known-freeze失败又可在已知清理后释放marker。将其统一会扩大失败分类。首片不移动factory/root-before-native/check/逐Read授权、长度与摘要解析、FreezeObject/FreezeProjection、ready witness、frozen.Close、marker清除或openResources.Close，不加defer Close/重试/补偿。WriteFrom即使n==length而尾部探测报错也必须先报错；失败不自动关闭writer，首次Close紧邻记录attempted。

**两种unknown不可混写**：ready(witness)返回unknown仍持有原frozen/marker；physical已正向结束后SQL settlement再unknown时marker可已nil，但原Gate/lease/OpenOwner/运行ticket仍保留。SQL metadata不证明native结束，helper既不分类unknown也不退款、删除、清marker或决定再准入。公共端口/DTO/SQL/schema/ledger/digest/profile及旧001语义均不变。

**执行与证据**（US1/US4/US5；FR004–006/015–020/024–025；SC002–003/006–009/012）：T030先补受影响native正反特征测试与基线；T031实施最小抽取，并以受控错误顺序变体取得有效red再恢复原语义验证green（red是新测试的敏感性证据，不伪称旧实现有bug）；T032复验受影响生产与协议；T033汇总源指纹/命令/退出码/未验项。检查见tasks的V210/V211，设计时均unverified；当前已按R69完成首片局部验证，SKIP与完整Host未验范围显式保留。新增完整payload后EOF探测失败负例，要求原错误、零成功结果、unknown原owner/hold/ticket、原键不读source、真实竞争者拒绝，不排空/删残留/自动返额；保留整个合成fixture到进程结束。

**影响层次**：首片改变上传与投影写步骤，必须重跑其native正控/失败、projection冲突/health、SQL proof/ready/settle unknown、workspace postcommit/生命周期和HTTP公开用例的受影响测试，normal/tag与vet；原生层未改但原WriteFrom/Flush相关负例随实际范围复验。不以这些局部检查代替V201–V209/SC007九例或第二进程完整验收；不重置T001–T015完成记录，改后涉及的当前证明在重验前为unverified，原历史保留。无公共契约/字节变更，旧向量不凭空失效。固定离线工具链/兼容profile/合成NTFS与忽略日志目录不变；CGO0的race保持skipped，不装工具。

**后续渐进顺序**：本首片通过后继续T016→T017→T018→T019及原独立验收，不把整体架构清理变成业务推进前置。运行时资源组合、私有操作围栏、预算fresh职责、profile纯解析再按稳定交接和真实消费者逐个细化任务/分析/复验；Host/HTTP装配涉及003共享接缝时服从实际交接与003权限，不自启T006。插件网关/注册代次、资源CAS/依赖、GC和新恢复状态表达不在此首片实现。

**安全回退**：这是源码内单一路径机械抽取，不迁移数据/换schema，不并行跑新旧owner；发现回归时仅撤回本切片自身修改，保留全部已有dirty及历史，不用git reset/clean、不回退数据库或重放原操作。运行中的未知fixture保留原owner至进程结束，不为回退制造known终态。

**宪章逐项核对**：I/INV01–05保留同正式owner与显式scope/身份；II/INV06–08不动inspect/open/factory/真实排他；III/INV09–14不动正式事务/基线/引用/回执，先error再length且冻结仍覆盖SQL；IV/INV15–17首次Close与实际结束轴不合并、不因取消释放；V/INV18–21保留逐Read当前授权、秘密边界和原安装容量，不宣称沙箱；VI保留A局部/共享unknown真实范围，无第二资源池；VII不动独立Host/公共协议/声明式客户端。以上是设计符合性，不是运行通过；未新增例外。

## 5. 阻断技术门与冻结次序

2026-10-21用户明确批准分阶段验收：下表G204分核心逻辑技术门与延后物理发布子项；不准备VHDX，不再以Ready清单阻断核心开发。T001–T005局部技术完成；现有隔离SQL/native故障/CAS/候选/既有Store围栏及有限物理样本按R7–R14原范围保留。T006核心逻辑计费与G205已按R16的288输入/六实际镜像/双模式原生运行局部技术签收；生产M2预算Store接线必须在T009/后续验收完成，不能借M1实验冒称通过。延后物理项仍未验证，不能签全G204或发布物理支持声明。历史停止线见research，当前次序以下表为准。

| 门/任务 | 必须先证明 | 失败影响 |
| --- | --- | --- |
| G201 / T003 | 原安装1→2显式真实事务、当前operator/维护排空、原ledger保留、commit故障/进程中断/fresh三态；双workspace profile与原key绑定/旧001原能力 | 停安装/M2创建/预算及所有依赖业务，不另补旧表/新库fallback |
| G202 / T004 | 同正式适配器真实NTFS CREATE_NEW/Flush/冻结/全hash、别名/删换/硬链/reparse/非继承、失败/中断 | 停对象/投影及正式引用依赖 |
| G203 / T005 | 发布/校验/投影均可观测真实未结束原生IO，释放后实际join；不是逻辑等待marker；生产正控 | 无法可靠构造则相关SC-007未满足，停依赖，不降低目标/装驱动改策略 |
| G204 / T006 | **核心技术门**：持久分配/可计算逻辑metadata计费及保守预留、candidateSlots、native事务unknown/fresh、CAS/重启与控制owner接口；实际生产接线/公平/全逻辑满额由T009/T025验收。**延后发布子项deferred**：完整SQL页/journal/temp/NTFS物理峰值上界与原生真实满盘/控制恢复 | 核心技术门未满足停T007；物理deferred不阻断核心开发，但禁止物理硬配额/盘满保证与相关发布就绪结论；不能以局部样本签全G204 |
| G205 / T007 | G201–203及G204核心技术门、T002字节/契约正反一致，冻结具体端口签名/责任与profile/指纹，重核宪章/规格 | 未满足不得启动共享核心并行；记录具体失败而非笼统blocked |

门的原型/实验也必须同真实profile/适配器并隔离合成夹具，不做另一套成功逻辑。技术门局部通过不等于最终Host验收，后续代码变更须重跑受影响门并更新指纹；不能复用旧binary冒充当前输入。

## 6. 真实验收与需求映射

| 验证 | 需求/目标 | 已批准的闭环义务（原设计；实际签收与限制见§6.2及tasks当前状态） |
| --- | --- | --- |
| V201 / T020 | US1，FR001/003–009，SC001–003 | 真实上传/预览无正式变化/确认，缺失未完成hash错及校验后替换零坏引用；字节正反及实际产品边界 |
| V202 / T021 | US2，FR012–014，SC001–003 | A/B各3资源2版，v1两对象/v2替换移除；全Host正常结束/新认证进程全历史/字节/receipt及多页游标；v1坏v2好精确影响 |
| V203 / T022 | US3，FR008–011，SC004–005 | 同基线两真实客户端10轮恰一成功；跨区/换主体/输入key；admission/commit unknown/丢receipt真实故障与fresh原key调查，零重放 |
| V204 / T023 | US4，FR015–017，SC006 | 实际投影写错/候选冲突/缺失陈旧；正式receipt/Head/候选字节守恒，显式再生成真实恢复 |
| V205 / T024 | US5，FR002/014/019–020/023，SC007–008 | 三阶段×取消/超时/断连9例真实pending；未释放前busy/notclosed、B每例新增更新读取、真实join后close/newepoch；撤权/过期/串区/共享unknown/秘密扫描 |
| V206 / T025 | US5–6，FR018/025，SC009/013 | 逻辑字节/元数据/名额及产品边界，retained全满仍原key/close/有限调额（R113）；runtime普通A31/global63适用HTTP也429，含control总32/64仅内部组件（R114），不混称公开逃生；A满B继续/unknown不退，CAS/fresh/cold不双计；原生满盘和完整物理界独立deferred |
| V207 / T026 | US6，FR021–022/026，SC010/013 | 非空001旧安装显式升级仅改安装库；原工作区检查源不变＋原文本新增更新读/全重启，M2新目录及旧profile拒绝，安装真实故障/进程中断恢复 |
| V208 / T027 | FR006/019/024，SC011 | 持续/同时/已知安全退出后三模式各10实际第二进程；失败零正式保存，恢复真实排他及对象/持久历史验证后再写 |
| V209 / T028–029 | 全FR/SC012 | 唯一OpenAPI生成漂移/状态/头/页默认、固定bytes双消费者，normal/tag vet/full回归、条件race、所有当前inputs/images/profile/记录/本地链接；001受影响安全保存生命周期回归；分别给核心工程结论与延后物理发布未就绪结论 |

所有验证冻结源码/依赖/SQL/契约/工具/原生profile/实际exe，记录命令、退出码、时间、真实进程PID/birth/fileID及安全关联；日志只在Git忽略.data/logs。没有正常退出的unknown进程不得写成graceful end。旧233/授权/rename失败原文保留，有限green不称RCA。核心工程完成≠发布就绪、用户业务验收、旧Host退役或整个M3–M8完成。真实原生盘满和完整物理界未验收前，不对该能力作支持声明；有限guard/控制reserve不承诺磁盘可写。

### 6.1 T028/V209 当前全输入回归（implemented / R119；最终汇总见§6.2）

承接原T028（A执行、C复核记录），依赖T020–T027已验；覆盖全FR的兼容回归及SC012，不重建需求或重复业务矩阵。冻结所有Git非忽略源码/契约/SQL/工具/go.mod/go.sum及精确诊断runner到私有C，离线缓存、固定Go/SQLite/libc兼容profile不变。先运行零写入OpenAPI结构/生成漂移、新旧bytes Python独立消费者及其测试，再当前normal和aboard_acceptance全Go包测试、两种vet；不配置外部Host时的fixture/helper条件SKIP逐条记录，不能冒充业务执行。重建普通/诊断Host及normal SQLite/installation TEST helper，记录版本、go env、modules/compat输入和实际镜像SHA与normal探针隔离；根/C内容及路径集合双比对。

用同批实际镜像执行受影响001保存/原key/实际native故障与capacity、完整冷重启、身份权限/隔离、writer别名边界、排空、HTTP状态/默认分页的独立Host选择器。R118已实际完成M1/M2三模式各十，不添加重复十轮门；本片保留该冻结证据。候选真实900s TTL按原V209规则：若不能证明语义未变则当前单独一次真实等待，不把默认SKIP称通过；固定CGO0故race明确skipped，不改环境或下载。普通全仓和tag全仓的内部/平台/security/Host覆盖与独立Host链分列；任一真实失败保留并定位、最小修复后重验受影响层。R119实际冻结662输入，二十步契约/bytes/全normal-tag/vet/profile均0；同批四镜像的原001二十一个父case零SKIP，真实900s门901.02s及离线mod verify通过，完整失败/修复/复用范围见[research R119](<research.md>)。包内条件SKIP与独立业务执行分开，原V201–V208只按精确输入/影响及报告链复用，不冒称392重跑；race固定CGO0仍skipped。R119当时完成原T028而T029尚待，最终覆盖/本地链接和核心工程/物理发布deferred已在§6.2分开汇总；003仍须原T006实际共享交接和当前技术门。

### 6.2 T029/V209 核心工程最终汇总（implemented；发布未就绪）

本节是原T029的证据汇总，不是新增规格/验收体系。**002已批准的核心逻辑范围工程完成；不是用户最终业务验收、发布就绪、旧Host退役或003/M3–M8完成。** 当前唯一摘要仍在[tasks](<tasks.md#当前状态>)，原命令、退出码、时间、过程/身份关联、source/image/hash与未验边界在[research R94–R120](<research.md>)及其私有ignored报告。表中“已验”限每条实际闭环与明确的支持profile，不签原deferred物理项。

#### 用户场景、原任务与实际入口

| 场景/任务 | 核心实际证据 | 声明边界 |
| --- | --- | --- |
| US1 / T020、V201 | R94–R97普通产品对象/完整预览确认、五类坏引用、实际网络短上传、默认/最大产品边界及全bytes/cold | 上传不是正式保存；短流unknown原key调查，异常收集不算退款 |
| US2 / T021、V202 | R98 A/B各3资源2版以上、两对象替换移除、全Host退出/新主体全历史bytes/receipt、固定upper游标/损坏隔离 | 不承诺多资源快照；坏v1不借投影修复、不误封B |
| US3 / T022、V203 | R99十轮同base真实并发/原key/丢回执，R100真实M2准入/正式FSYNC与普通fresh调查 | admission/formal的实际完整结果分别记录；缺key单独不证明未提交，不自动换key或重放 |
| US4 / T023、V204 | R101旧版再生/缺失陈旧/坏候选，R102实际CREATE_NEW冲突，R103同FileID真实WriteFile33/hold/辅助调查 | 原Write33 writer未first-end，精确异常collection不是drain；正式receipt/Head独立不变 |
| US5 / T024、V205 | R104–R106发布/校验/投影×取消/超时/断连9种实际IO pending及join、每例B新增更新读；R107–R109共享unknown/当前角色/foreign关系 | callback排序不是IO证据；IRP与原不同进程TID kernel pending先验；shared依赖准确installation阻断 |
| US5–6 / T025、V206 | R110预算CAS/fresh/cold，R111两profile持久slots，R112真实预算1034，R113metadata/bytes floor与global承诺，R114 ordinary runtime/控制与依赖/原Store end | 逻辑保留full仍旧key/close/有限管理；ordinary A31/global63时适用HTTP也429，control总32/64不是公开逃生；物理ENOSPC未验 |
| US6 / T026、V207 | R115同普通Host M1+M2，R116真实升级1034/当前DB门，R117原中断journal普通拒开+strict显式恢复/fresh及Store end | 仅安装schema演进；旧目录未迁移/补种；旧R116失败不改写为通过；不称硬断电 |
| US5 / T027、V208 | R118 M2与兼容M1的持续/同时/known-idle退出后三模式各十、真正native失败零formal、重获/全历史后才新写 | M2 TEST-only contender真实lease/pins无SQL，正常Host才public新写；异常不是unknown graceful end |
| 全US / T028–T029、V209 | R119当前五Python/Go双bytes、全normal/tag/两vet、四镜像/deps/profile、21个001父case零SKIP/真901.02s；R120只读当前覆盖/链接/输入桥核对 | package条件SKIP不代V201–V208独立业务；race固定CGO0 skipped；003原型unit不代生产 |

#### 全FR实际覆盖（26/26；不改变需求）

| FR | 原验证/实际研究入口 | 当前核心判定 |
| --- | --- | --- |
| FR-001 | V201、V209 / R94、R97、R119 | 同一独立Go Host/正式owner，无内容执行器 |
| FR-002 | V205、V206 / R107–R110及R85、R88 | 当前主体/显式区/每路由权限与有权后关系分别验证 |
| FR-003 | V201–V202 / R94、R98 | 完整一版/稳定资源及区级revision、全历史关联 |
| FR-004 | V201、V209 / R94–R97、R119 | 原bytes与独立正反消费者，不归一化 |
| FR-005 | V201、V203 / R94–R95、R99–R100 | 接收/候选/正式提交分离 |
| FR-006 | V201、V208 / R95、R100、R118 | 引用时最终核验/替换拒绝与真实第二进程排他 |
| FR-007 | V201、V203 / R94–R95、R99 | 精确输入/base/主体候选，不扩大旧确认 |
| FR-008 | V203、V205 / R99–R100、R109 | durable原key调查先于新准入/旧epoch，陈旧候选拒绝 |
| FR-009 | V201、V203 / R94、R100 | 正式版本/Head/引用/receipt/结果原事务一致 |
| FR-010 | V203、V205 / R99–R100、R109 | 同key唯一效果及换输入/主体/区拒绝 |
| FR-011 | V203、V206–V207 / R100、R112、R116–R117 | 确定拒绝/已确认/unknown分层，原key不重放 |
| FR-012 | V202、V205 / R98、R109 | 当前与明确版本全内容/bytes、有界同主体游标 |
| FR-013 | V202、V207 / R98、R115–R117 | 完整新进程/新认证/原持久事实，不依赖投影 |
| FR-014 | V202、V205 / R98、R104–R106 | 依赖坏对象精确拒绝/范围隔离，不返回少附件伪完整 |
| FR-015 | V204 / R101–R103 | 投影/再生独立于正式receipt及Head |
| FR-016 | V204 / R101–R103 | 原候选/不可信目的地守恒，缺失不修正式事实 |
| FR-017 | V201、V204、V206 / R95、R103、R111–R114 | 残留不自动提升/GC，证据保留和逻辑容量可观察 |
| FR-018 | V201、V205–V206 / R97、R104–R114 | 默认/最大/流式/metadata/slots/runtime有限，unknown不退 |
| FR-019 | V205、V208 / R104–R106、R118 | 真IO pending/截止准入/原first-end+join后新epoch |
| FR-020 | V202、V205 / R98、R104–R109 | A局部/B进展及真实installation共享unknown分列 |
| FR-021 | V207 / R89–R93、R115–R117 | M1/M2支持拒绝矩阵/零隐式目录升级 |
| FR-022 | V207、V209 / R89–R93、R115、R119 | format/schema/API分离/ledger/单一契约与bytes |
| FR-023 | V205、V209 / R107–R109、R119 | 原raw/decoded/headers/trailers/日志正反scanner，不裁剪合法内容 |
| FR-024 | V201–V209 / R94–R119 | 合成真实IO/OS进程/正常重启与当前source/profile/条件SKIP明示 |
| FR-025 | V206、V212 / R75、R110–R114 | 有限安装/区分配、13field观察、原key/CAS/fresh/依赖；物理子项deferred |
| FR-026 | V207 / R115–R117 | 当前operator/原key/显式仅安装schema/真实unknown及strict恢复 |

#### 全SC实际覆盖（13项的已批准核心范围；非物理子项全通过声明）

| SC | 核心证据 | 限定 |
| --- | --- | --- |
| SC-001 | R94、R98、R118 | A/B完整正式历史/对象/receipt、全停新认证重开 |
| SC-002 | R94–R97、R119 | 空/零/同长异bytes/中文组合/LF-CRLF/集合标签，独立Python+Go正反 |
| SC-003 | R95–R96、R98 | 缺/未完成/hash错/核验后替换及坏v1好v2，零坏正式引用 |
| SC-004 | R99 | 两真客户端10轮恰一胜及原key输入/主体/跨区隔离 |
| SC-005 | R95、R99–R100 | 确定拒绝、真实admission/formal unknown、丢回执按原key实际fresh结果；不补签未观察三态 |
| SC-006 | R101–R103 | 实际Write33/CREATE_NEW、缺失陈旧/显式再生及正式/候选守恒；失败writer保留非drain |
| SC-007 | R104–R106 | 三阶段×三交付九种真实kernel pending、B每例CRUD、释放后实际join/新epoch |
| SC-008 | R107–R109 | 26共享unknown路由/六beforebody、普通当前权限与到期撤权、十foreign关系/合法正控 |
| SC-009 | R97、R110–R114 | 产品默认/最大/请求响应、逻辑bytes/metadata/slots满额与runtime背压；物理满盘另列 |
| SC-010 | R89–R93、R115–R117 | 非空M1原源零改/显式安装升级、同Host M2/旧profile unavailable/坏未来边界 |
| SC-011 | R118 | 每模式10真实OS进程、known-idle退出、原lease排他/全旧历史后public新写 |
| SC-012 | R119–R120 | 当前源/4镜像/依赖/过程及本地链接、同批00121case/真900s；批准不代用户验收 |
| SC-013 | R110–R114、R116–R117 | 调额/冲突/失败unknown/真Store首次结束及cold；原升级journal显式恢复，不换库重放 |

#### 命令、构建输入与过程复核入口

| 实际过程 | 原命令/退出/时间、input/image与PID/birth/FileID记录入口 |
| --- | --- |
| 独立对象/历史/并发/投影 | R94–R103链接的各full/related JSON：原`pwsh -NoProfile -File .data/logs/002-validation/...ps1 -Mode full -Attempt ...`及每步真实`tools/go.ps1`参数；有效red为预期exit1，其余0，不从模板猜命令 |
| 实际pending/安全 | R104–R109对应verify/projection/publication/shared/authorization/foreign full JSON及逐case私有process.json/logs；原TID/IRP/原key/first-end、异常child收集与drain分列 |
| 预算/升级/排他 | R110–R118各full/related记录同原key/CAS/schema/nativeordinal；R117明确main源/同SHA产品复制与main TEST helper，R118 30+30轮不加重跑门 |
| 当前核心层 | [392 core](<../../.data/logs/002-validation/final-core-392-explicit-prototype-root-results.json>)20步，14:24:29–14:34:34+08；每步精确Args/Log/Tests/Skips/开始结束/Exit、662 inputs与root/C路径集合 |
| 当前独立001与TTL | [392 affected001](<../../.data/logs/002-validation/final-001-392-current-affected-results.json>)原21父case/901.02s，14:35:06–14:51:23+08，四同C镜像/同源/新认证/原native/profile，零selectedskip |
| 依赖/profile | [392 mod verify](<../../.data/logs/002-validation/final-dependencies-392-current-results.json>)exit0/all modules verified；Go1.27.2/windows-amd64/CGO0、SQLite3.53.4、libc1.77.1/sqlite1.60.1、单一privatecompat指纹/完整modules，非下载 |
| 只读最终交接 | [393 final](<../../.data/logs/002-validation/final-summary-393-current-results.json>)核26FR/13SC与全部任务、本地链接/源集合/四镜像、392→当前仅四doc差异；不是新业务测试，也不代签003 |

实际主普通产品SHA=`C7E9EEBAF312004AFA2443F3BDE49F3642865516EAC82CAC3A99BB9ECA302C83`，无测试切口。diagnostic=`C44AE3AAD5E6C98BCAA491597A2E199C200C4E56C8F1D4645006B3D8E40B184A`，SQLite TEST=`87F26700142CD98276F77DFEA18487BEA6917ABBC93D6936CC5075E1D86CEBE9`，installation TEST=`5BB4A91F5F6627C535536BB24612C516FC0229DB00571F5843306024772242E4`分别定位，不是把tag/TEST算normal。早期core GOSUMDB误配、Python参数未实际执行目标而取消、003原型显式root漏配以及更早native/pipe/权限/rename等失败全部保留；后续green不声称旧环境错误永久RCA。

#### 未运行/支持界与发布触发（deferred / 未就绪）

- 固定Windows amd64、本地直连NTFS、普通非重解析目录和上述离线Go/SQLite兼容profile；新checkout不含ignored依赖缓存、exe或私有合成证据，不能据文档直接claim本机重现通过。无Linux/macOS/网络盘/云同步/多Host同区协同写支持。
- **物理发布门未通过**：完整SQLite页/journal/temp/NTFS共享元数据实际峰值写集上界、真实原生ENOSPC和盘满下必要控制写、支持卷/driver/扇区等所需环境仍deferred。后续须另有用户确认的有界独占合成环境/卷身份和可追溯物理负载，在发布或相应物理硬配额/盘满支持声明前实际验收；当前不准备VHDX、不填宿主卷、不改ACL/系统策略，不借逻辑guard/FSYNC1034/有限快照代签。
- race固定CGO0 **skipped**，不安装compiler或改profile；公开scope/原生错误没有穷尽所有syscall组合/硬件断电或完整物理峰值。当前包条件SKIP清单保留在392，真正业务案例仅按各已运行入口与影响复用链声明；没有签用户最终业务验收、已分发安装包、旧Host退役、真实数据迁移或任何跨仓变更。
- 003后续采用本当前共享基线与统一授权，在其唯一plan/tasks按实际T006/技术门冻结端口、单owner共享串行变更并重验受影响002；历史T001–005隔离可行性不代当前生产TLS/浏览器/bridge/journal/CLI/React组合。Workbench跨仓路径和写权限未授，不擅自接通外仓；本工程完成不撤销该停止范围。

## 7. 协作、顺序与实现前置

历史前置：分阶段决议后的只读analyze无阻断，T006/T007已按R16完成核心技术冻结；这不是当前停在T008，也不替后续修订签分析门。当前推进顺序由tasks状态/依赖决定；T009/T018/T025必须完成实际预算Store/权限/原key/control/IO依赖接线，不把原型当正式实现。物理发布项deferred保留，不要求准备VHDX。

原任务图的依赖规则（非尚未实施声明）：T001–007先按依赖做技术门，A集成共享profiles/预算/正式core，B对象适配器，C独立消费者/外部测试。T007前不做共享核心并行；之后只在独占文件/冻结端口上并行，任何共同文件回A串行集成。工作树隔离不是OS安全沙箱。

现有tools/go.ps1在每工作树持私有prepare.lock；不禁用锁争抢同cache。每session/worktree独立cache/tmp/合成安装/工作区/端口/pipe/exe/log目录；当前pointer只有主会话可维护。worker每次工具调用显式SPECIFY_FEATURE_DIRECTORY且SPECIFY_FEATURE_NO_PERSIST=1，不能依赖一次pwsh环境跨进程保留。不得复制真实安装/秘密/活授权或复用持有中的锁。

交付首片可先达到US1/US2的独立验证，但不得借此跳过剩余US3–6/全部技术门宣称002完成。任务依赖与具体owner见[tasks](<tasks.md>)；无新增项目、引擎替代、工具/技能改写或宪章例外。
