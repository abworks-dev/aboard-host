# 003 Plan — M6可信客户端集成与第二消费者

唯一当前进度、门状态与下一动作见[tasks](<tasks.md#current-state>)；本计划保留设计及各次执行截面，历史段落的“当前/仍待”不另构成当前状态表。R60–R64的单次预算、失败即停和另行运行审批仅为当时执行历史；开发目标范围内的修复与复验按[当前授权](<../001-host-workspace-foundation/authorization.md>)连续推进，不恢复或续签旧预算，也不以旧driver/标记作为调度入口。历史失败与技术/安全/验收要求不因此失效。

<a id="design-guide"></a>

## 设计阅读入口

**现行概要**：受信原生客户端通过受保护pipe取得活实例关联，再以精确TLS pin先验后发bearer；受管专用浏览器仅经窄桥使用有限动作。Host保持唯一授权/业务提交owner，私有journal只保存原key调查线索与一次发送围栏，Workbench只提供UI/消费适配/静态产物。

| 要找什么 | 设计与责任入口 |
| --- | --- |
| 技术与宪章 | §1固定Windows/001能力边界、§6宪章检查；无合格环境拒绝，不自动下载/降级 |
| 组件/独占范围 | §2责任图 → §4.4.4 A/B/C精确子文件；A串行集成共享Host/安装/HTTP/唯一OpenAPI，B原生/桥/journal，C独立CLI与联合；Workbench由外仓owner写，不代签 |
| 接口依赖与理由 | §3.1–3.3可信启动/精确文档/唯一公共接缝，§4.4冻结；精确pin而非端口/PID认证，有限桥而非任意代理；不复制业务协议 |
| 保存/失败设计 | §3.4原候选→唯一key→U转Q→完整Host结果→页面UI→独立ACK；传输失败不改业务终态，Q不恢复发送权 |
| 实施顺序 | §4原依赖 → T001–T006技术/设计门 → T007–T017本仓工程 → T018接收与H02/H04 → 原后置联合/验证任务；依赖和owner逐行见[tasks](<tasks.md>) |
| 当前技术切口/验证 | §6.1 H02/H04设计与组件，§6.3 WB036接收，§6.4候选与§6.5精确生产支持；§5原验证映射连到原G/V。实际接收/就绪与剩余门只看[tasks当前段](<tasks.md#current-state>)，不以历史失败或早期未验覆盖新证据 |

**历史阅读规则**：§3保留最初设计的义务与当时“未验”表述，实际结果沿原任务核对；§4.1–4.52保留各片设计/证据/限制，不是连续待执行清单。原编号在§5后回到§4.5，保留其标题和锚点以兼容旧引用，不按版面顺序重编号。ignored历史日志/旧绝对路径不保证在新checkout可用，本次仅文档检查，不复验业务。

<details>
<summary>展开截至R54的原历史进度摘要</summary>

**历史进度摘要（截至R54，非当前状态）：partial / T001–T005已在R11当前输入/Edge156复验；T006/G305按R11–R13及R14完成§4.4设计冻结；后续已实现/未验的唯一当前任务状态见tasks，§4.5–4.42分别保留R15–R54工程/局部证据与边界，§4.29原normal/tag合法4096已由R42实际补验，§4.31原Consume真实FSYNC未知/同Store调查已由R43按当前输入实现并验证，§4.32独立CLI已由R46以753当前输入/两个normal实际exe/十四步骤和三轮独立公共写读确认工程收口，§4.33当前Q交付证明/每结果UI/独立ACK正常DELETE已由R44验证，§4.34真实DELETE FSYNC及精确原句柄结束/同Store完整old调查已由R45补验，T016可信native来源和一次正式转发现由R47以756输入和真实Host.Run/TLS/原Connection补验，T016原精确renderer桥预览/确认装配已由R48以760冻结输入、normal/tag真实Host/Edge各三轮及两个有效原保存观察控制补验，T015原保存确认的页面UI/独立ACK装配当前已由R49以763输入、normal/tag实际Edge各三轮和两个真实DELETE观察控制补验；native接收可信页面UI声明、不独立证明DOM或React，T016六个有限查询/原key调查/显式close现由R50以768输入、normal/tag真实Host/Edge各三轮及合法epoch读取控制补验，原分页/close unavailable/closed save ErrorView不改，记录epoch只是前后观察而非原子快照；T016可见集合/合法用途前置/显式open现由R51以773输入、normal/tag真实Host/Edge各三轮和两个有效用户版本/私有路径观察控制补验，原snapshot/current会话/用户registration保持且前置零打开/失败闭态源完整守恒；原身份最小U/Q线索及当前原key调查已由R52以780输入、normal/tag两代实际Edge/当前Host/原Store各三轮、十六步骤与两个有效控制补验，冷线索不恢复发送/不退Q；原冻结16个有限动作依R34–R52完成T016工程，原T015明确409STALE_BASELINE拒绝现由R53以783输入/normal-tag真实Host/Edge各三及两个实际ACK/UI控制补验，generic拒绝可能抑制原已保存receipt而仍保Q不重发；原Q正常cold恢复后的当前页结果再交付现由R54以786输入/normal-tag两代实际Host/Edge各三轮补验，原GET200原key receipt、显示/UI分别Q1再独立ACK Q0、不恢复发送权，原T015工程据R44–R45/R49/R53–R54已收口；真正React/外仓、完整产品联合仍未验；不以早期“可开始T007”覆盖后续实现，完整产品组合仍未验。** 需求为已批准[003规格](<spec.md>)，当前授权仅见[统一记录](<../001-host-workspace-foundation/authorization.md>)；002核心交接见[最终矩阵](<../002-resource-object-commit/plan.md#62-t029v209-核心工程最终汇总implemented发布未就绪>)。下文2026-10-21/22“止于plan/不实施”等保留为对应阶段历史，不构成当前授权副本；不改002 locator、不写未授权外仓、不把旧原型或002完成称为003生产通过。

</details>

## 1. 技术上下文与范围

Windows本地、已初始化安装及已登记001工作区；Host保持独立运行。Go 1.27.2与当前固定SQLite/Windows安全适配器继续使用，不换驱动。React消费当前文本、预览/确认、原key调查和显式open/close；不提供create/register、历史正文、附件、AI运行或安装管理。

**2026-10-21设计取证（历史）**：当时工作树有002源码/契约/绑定与其他未提交变更，该轮仅静态阅读/设计，不能代签001或002技术门。当前真实002已收口截面和003复验边界见§4.1；仍不以HEAD代替未提交工作树，不修改已批准spec需求。

## 2. 设计及组件owner

唯一集成/Git协调者为本主会话。以下是2026-10-21设计时的组件候选位置，不表示已创建；当前T006精确生产子文件owner与串行边界以§4.4为准，只有各原任务依赖实际满足后才分派独占实施。

| 组件/候选位置 | 单一责任与owner | 不拥有 |
| --- | --- | --- |
| `cmd/aboard-client/`、`internal/clientnative/` | Host原生启动、可信发现、会话及退出；Host核心owner | 初始化/修复安装、停止Host、第二授权源 |
| `internal/clientbridge/` | 有限浏览器动作、可信静态交付、CSRF及会话围栏；原生客户端owner | 任意代理/文件路径、业务事实 |
| `internal/clientjournal/` | 最小非授权线索、有限持久名额、一次发送围栏；原生客户端owner | receipt/正式终态、业务重放 |
| `internal/installation/`、security、HTTP与公共契约 | 当前可见区集合、身份/能力、合法open前置、对应会话撤销及连接安全；Host核心owner统一集成 | 浏览器授权、自造工作区数据库副本 |
| `cmd/aboard-client/`独立CLI模式 | 第二消费者公共协议调用与互证；Host消费者owner | 私有提交路径 |
| Workbench既有UI与消费适配 | UI、生成绑定及静态生产产物；外仓owner，后续另获跨仓写权限 | Host协议第二权威、秘密或业务存储 |

多个owner角色当前均由本主会话负责设计；未来只有文件边界与接口冻结后才分派独占实现。共享auth、Store、生命周期、OpenAPI及生成物始终由唯一核心owner串行集成，不与002竞争修改。

## 3. 关键链路

### 3.1 启动、发现与退出

原生入口只读取受信本地配置；用户显式选择连接已运行实例或启动同一已初始化Host的serve。启动前验证安装/配置和可信可执行产物，不接受浏览器传来的路径、命令或初始化请求。实例含混/损坏/不支持拒绝；竞争启动交由原host.lock证明唯一写者，不能强杀已有进程或自动重启。

复用原受保护控制管道，核验当前OS用户、服务端及活进程关联，再获得该客户端专属Host会话。设计需增加精确当前会话撤销/确认，不调用按主体撤销全部会话；本次仅定义责任，不伪称已有生产撤销产品入口。每个客户端使用独立会话，全部页面共享该客户端退出边界。

退出先使桥会话拒绝所有页面新准入/迟到交付，再请求对应Host会话结束；只有确认才展示撤销成功。不可达/持久未知显示待确认。已accepted业务由Host继续，保留调查线索；关闭页面只是断开，不等于退出、close或停Host。

### 3.2 可信页面、窄桥与实际连接

原生层交付可追溯固定静态包，以用户已有且获验证浏览器的独立干净profile打开。候选Edge稳定版需冻结实测版本；无合格环境unavailable，不下载、不使用日常profile。

窄桥只允许身份/能力、可见区分页、open前置、当前记录、preview/confirm、原操作查询/reconcile及显式close/当前会话退出。每个动作有严格字段、body/输出界与当前会话检查；禁止任意URL、任意Host路由、文件或命令。浏览器不持有Host bearer；Origin/Cookie不转发给Host。精确同源、authority、CSRF、Fetch Metadata共同拒绝恶意页，静态fallback不得吞API错误。

**browser→bridge页面登记生命周期（designed，未验证）**与下述native→Host控制管道/TLS登记分开；干净profile或同源本身不建立桥会话，浏览器也不能凭自报身份加入：

1. **选定载体而非任意页面领票**：可信启动器只启动用户已有、获选版本/策略验证的受管专用浏览器进程，通过仅显式继承的私有OS管道控制其CDP；不附接任意现存Edge，不开放TCP调试端口、不读端口发现文件或采用TOFU。核验实际持有管道的浏览器进程PID/创建身份/可信image，而非短命启动stub；管道及命令owner须有明确实际结束证据。Windows Edge的该能力当前未实测，不能由“已安装Edge”推定支持。若需要新增扩展、运行时、下载或改系统策略，先回报用户，不暗加前提或fallback。
2. **绑定精确文档**：启动器核验所交付静态包与精确bridge Origin，建立browserContext/target/主frame/loader/不可混淆的唯一执行上下文到客户端代次的关联；只向这个受控文档上下文注入一次登记动作。不能先核验URL再仅按targetId或可复用的整数contextId执行。导航/redirect、上下文销毁或替换会撤销未用材料及旧页面资格；命令执行前后均复核文档代次，旧加载/迟到响应不得给新文档登记。不能证明封闭核验到执行竞态则unavailable，不靠放宽页面CSP补救。
3. **先受控建立client envelope**：原生层为有效客户端代次创建随机不透明的HttpOnly client-envelope cookie，选定经同一私有管道向正确browserContext的cookie存储受控设置；它不是页面身份或Host bearer。cookie操作由唯一控制owner串行，收到实际完成确认后才开放页面登记；代次切换先失效旧上下文并排空原cookie命令，再受控替换。原命令结果不明时不启动新代次来覆盖，不凭取消/超时假称其已结束。普通静态/登记/业务HTTP响应均不写Set-Cookie，从而不让旧HTTP响应覆盖新envelope；并发页面加入也不竞争改cookie。具体cookie属性及该私有API必须在获选环境验证，未证实保持不可用。
4. **一次性加入及独立页面上下文**：启动器经上述精确文档通道受控交付短时一次性材料，绑定Origin/静态包/客户端与文档代次；不从普通GET、cookie或页面自报身份签发，不进URL、argv、普通日志或浏览器持久存储。页面同源登记时，桥核验authority/Origin、Fetch Metadata、输入界、client envelope及材料全范围/有效期，原子核销后才建立page-id与独立CSRF。no-store登记响应仅在原文档内存接收CSRF，不推导cookie、不设cookie-only恢复接口；响应丢失须由控制器重新核验该文档并重新发一次材料，不能重放已消费材料。每次后续读写均校验envelope、page-id、CSRF和当前文档/客户端代次，POST另校验精确Origin；cookie不按端口隔离，SameSite不能代替这些检查。材料不是Host凭据，错误或重放不进入Host业务dispatch。
5. **刷新与多页**：刷新先使旧文档上下文和未用材料失效，进入“等待受信加入”，由启动器对新文档重新执行步骤2/4；同客户端新页同样办理，不复制旧材料、不仅凭残留cookie取回CSRF。有效客户端仍共享一个envelope/Host会话，不因新增页面旋转cookie或使其他有效页面退出；各page-id/CSRF和文档代次独立，全部页面共享该客户端退出边界。
6. **重登记**：Host会话过期或活实例变化不由刷新续权。经受信原生控制链重新核验安装/原身份及当前授权，建立新客户端连接代次，先失效旧加入材料/页面/CSRF/迟到交付，再按步骤3的串行实际完成规则建立新envelope与页面。旧页面不能自报新代次；查询缓存隔离，持久调查线索保留原范围，重验后仅按原key调查，不恢复发送权。
7. **退出**：立即截止全部页面及未用加入材料的新准入/迟到交付，再处理当前Host会话撤销；未确认仍显示待确认，不恢复旧上下文。旧cookie/CSRF/登记响应不能复活客户端，普通HTTP也无迟到Set-Cookie路径。再次使用须用户经可信启动器显式登记；关页不撤销Host会话或删除调查线索。管道/浏览器异常不被当成物理结束，未知保留控制owner且依赖暂停，不强杀外部进程。

这里只选定设计机制和时序，具体动作/字段随§3.3冻结，不由Workbench另建安全协议。新增验证覆盖首次可信加载、真实私有管道/实际浏览器进程、核验到执行间导航/文档替换、并发两页首次加入、刷新/实过期/一次核销竞争、仅cookie/跨端口引导拒绝、旧登记回包及重登记/退出时的cookie命令竞争；普通响应不得出现Set-Cookie。私有cookie/文档API、无外部调试口和受管进程/profile均unverified，未实现或运行。

Host连接在发bearer前核验实际活对端；连接复用仍需保持活实例关联。机制详见research R2，现为未验证技术前置，不能仅凭原管道签发成功就信任任意loopback连接。不能证明则暂停依赖，不放宽原HTTP安全策略。

可信连接决定（designed，未运行）：不再以TCP四元组/PID归属作为认证机制。选择Go标准库TLS 1.3、每次Host启动新生成的内存私钥/自签证书，以及通过既有受保护控制管道交付的精确证书公钥SHA-256 pin。控制回复绑定客户端随机nonce、安装ID、Host活实例随机ID、已核验进程创建身份、精确TLS authority及pin；客户端核验完整回复后才建立连接。不安装系统根证书，不采用TOFU，不接受浏览器提供的pin/端点。

Host新增仅loopback的TLS传输入口，复用同一业务HTTP handler/授权与提交owner，不维护第二业务协议；原001入口语义保持不变，003所获会话必须由Host强制限制到该TLS入口及绑定活实例，不能被拿到原明文入口使用。此为未来共享安全/控制契约的显式增量设计，不在本轮修改源码/OpenAPI。管道必须来自已验证可信Host可执行产物与活进程；附加TLS只能证明持有管道所指定私钥，不能替代管道与可信启动校验。

顺序：固定可信Host/控制管道→带nonce获取实例/authority/pin及专属会话→仅拨指定loopback端点→TLS握手证明精确pin私钥持有与证书用途→完成校验后才发送bearer→Host再校验当前会话/实例/传输绑定→交付前再检查。精确pin校验不得退回系统CA、忽略校验或明文；自签证书使用明确自定义验证并检查TLS版本/用途/有效期。禁代理、重定向、TLS会话恢复/0-RTT及自动正式POST重试；连接池按实例/pin/会话代次隔离。实例失效撤销该实例会话、关闭旧连接池，必须重新控制管道登记，不能更新pin后复用旧bearer。

验证门覆盖错误证书/私钥、错误nonce/实例/authority、过期材料、同端口/PID重用、Host退出后替换、连接池跨实例复用、重定向及明文入口误用。每条实际bearer连接须证明先完成正确pin握手；错误对端可见bearer为0，旧会话不能跨实例或降级使用。四元组仅诊断，不作为安全正控。无法验证上述链路则停止依赖，无备用自动fallback。

### 3.3 Host公开接缝与唯一契约

新增需求接缝：当前身份及实际能力、授权可见工作区有界集合、用途绑定open前置、当前会话结束确认。集合来自安装登记及当前授权，不扫描目录；分页绑定主体/授权版本，陈旧/越权受限拒绝，不泄漏路径和无权目标。

open沿用原WorkspaceOpener及登记版本CAS，不自动刷新用户确认基线。所有查询/回调/交付携带安装、主体、会话代次、目标区和epoch；UI仅在完整匹配时接受响应。

公共业务结构仍以Host OpenAPI为唯一源。2026-10-21设计阶段当时未证技术前置，故未冻结字段/签名；该历史限制不构成当前授权副本。当前T006增量设计冻结见§4.4，T007–009实施，T010写入唯一公共源并生成TS/Go绑定/漂移检查；本文不建立第二份OpenAPI或直接修改共享源。

### 3.4 保存、线索与恢复

同一次确认以Host候选关联加客户端持久确认句柄识别：安装/tenant/principal/workspace/candidate及候选预览版本共同绑定一个发送key，不以每次HTTP请求随机生成新key作为去重依据。同一候选的并发确认必须解析到同一持久关联；不同候选不合并。确认句柄由原生层在预览交付时建立并持久化，页面重挂载、响应丢失及正常冷重启按原候选查找，不能重新分配发送权。句柄不是授权，转发和调查仍检查当前会话/目标。建立句柄失败不允许确认；确认时关联与消耗发送权须在同一线索库事务内完成，提交unknown时停止转发并调查线索，不自动重试发送。候选过期或陈旧须按Host语义新预览，不能把旧确认自动迁到新候选。精确字段须对照现有候选契约冻结，缺必需关联则拒绝，不臆造Host DTO。

正常重启先恢复原线索及唯一关联，再接收确认动作；恢复失败拒新保存。显式退出不删除未确认关联。验收须覆盖两个页面同时提交同候选、确认响应丢失后再次点击、桥在落盘前/落盘后发送前/发送后回包前终止，逐例检查原候选到key的关联数及实际转发次数，而非仅检查同key结果。

预览只产生原候选，确认沿001唯一提交owner，正文不归一化、不自行重算替代摘要。一次确认先在原生私有SQLite线索库分配原key/目标/必要输入关联，并保守持久标记发送权已消耗；落盘确认前不转发。SQLite复用固定驱动/profile，但不写工作区业务库或新增安装业务事实。并发页面使用事务唯一约束/单owner防止两次发送。

落盘失败/未知、容量满、围栏证据丢失均拒绝新保存；未知不自动释放名额，不驱逐未确认线索。具体有限上限及记录字节界待验证后冻结，不沿用实验常数。不能可靠辨别发送是否发生时只调查；即使崩溃发生于落盘与发送之间，也不恢复发送。

冷重启从私有线索库恢复关联，先重验原身份和当前权限，再调用公共原key查询/显式reconcile。缺记录、历史不可用、超时均不等于可重试。持久线索不存正文、bearer、receipt或第二业务终态；损坏拒绝新保存，不自动重建。正常重启必须恢复；磁盘损坏、手动删除或卸载后仅诚实报告不保证恢复。

收到完整且作用域匹配的Host committed+receipt才确认保存；202 committed+viewDeliveryFailed仍保留保存成功并单列交付失败。传输RST/截断不改写Host业务状态。close记录仍是原易失协调状态，重启缺失不得伪造closed。

容量收敛决定（替代永久墓碑及连续退休窗口）：使用持久journal代次、单调64位已发行上界N、稀疏可发送表U和稀疏调查表Q。默认规则是“没有U中的精确有效行，就没有发送权”，而非“未见墓碑则允许”。每票据绑定代次/序号/作用域/候选并以原生私有持久密钥MAC保护；MAC仅证明完整性，不授权。发行事务同时推进N并插入U，提交确认后才交付票据；序号永不回绕或重发。新票据只由真实新预览响应的原生登记产生，不提供按浏览器旧candidate补票据接口；恢复只查询原关联，不能因缺行重建。原预览请求关联与候选唯一性在未退休记录中保留，旧响应不重新进入发行路径；候选重新预览必须是用户新的显式动作，不能自动重挂载触发。

首次发送使用同一事务将U中的精确记录删除并插入Q（保留原key/候选/身份及必要调查关联），提交确认后仅当前内存中的唯一owner获准尝试一次网络发送；该准许不持久恢复。事务unknown不发送，先查本地事实；即使查明U仍在，也不自动补发原请求，需显式恢复原确认且遵守原映射，不能后台重试。Q记录永远没有发送权。缺行、错误代次、N之外序号、MAC或作用域不匹配均拒绝，不能重新插U。未发送U可原子撤销并删除，其旧票据永久失去发送权。

**结果交付和本地回收是两次不同确认**：当前授权下取得可信Host完整确定结果，只使桥有资格向正确页面交付，不直接删除Q。顺序固定为“Host结果完整校验 → 当前有效页面再校验原范围/候选/key及结果 → 页面提交可观察的保存确认/确定结果UI状态 → 页面独立发送本地回收ACK → 桥核验交付证明并事务删除Q → 回复本地清理状态”。不能等ACK响应才显示或保持保存确认；native收到receipt、HTTP写成功或页面断开不等于页面已完成结果交付。未走到有效页面确认的Q仍保存原安装/主体/目标/key/候选及必要匹配关联，不由易失查询缓存承接。

桥为该次完整Host结果生成仅内存的交付确认标识，绑定原key/候选、客户端/page-id/文档代次和本次结果；页面只在完成校验并提交对应UI状态后ACK。桥接纳时重验有效页面及相同交付证明，串行执行本地删除；旧页面/错交付ACK不能使用新页面资格。ACK只表示客户端已接收并处理结果显示，不证明用户已阅读、不授业务权、不恢复U，也不返回第二份业务终态。仅accepted、未知/错配结果、成功状态码或missing/unavailable都不能产生可回收证明；字段随接缝冻结。ACK失败或丢失不能把已经核验的保存降为“保存未确认”，不能为重试清理而再save；调查或重交付也绝不转发原正式POST。

| 中断位置 | 保留、恢复与可见语义 |
| --- | --- |
| 桥已拿结果，页面未完整接收/校验/提交确认UI | 没有有效ACK，Q必须仍在且占额；刷新/正常冷重启后重验原身份/当前授权，沿原GET重新取得完整结果并交付 |
| 页面已确认结果，ACK请求未到桥或删除尚未提交 | Q仍在；ACK只是清理待确认，页面保留已核验业务确认；重启可沿尚存Q重新GET/交付，不因重复点击重发保存 |
| 删除已知提交，ACK响应丢失或随后正常退出 | Q可能已无，不声称“ACK丢失必定保Q”；原页面此前已确认结果，不能因缺清理回复降级它。正常冷重启不承诺重新列出全部已确认历史；缺Q本身从不证明业务成功，也不授发送权 |
| 删除COMMIT或实际物理结束不明 | 暂停依赖该journal可信性的发行、再删除与返额；先按下述原owner/fresh门调查本地事务，不发送业务、不另建恢复表绕额 |
| 旧页面/错key/错候选/错代次/错交付ACK | 拒绝，不删Q、不返额；新页面必须重新授权GET取得自己的完整交付证明 |

删除unknown时，原DB调用及其回调/句柄必须实际join；取消、超时或原调用仍可能完成时的一次SELECT不能当作结束。随后在同一私有单写者生命周期内核验原journal身份/代次、schema/完整性及该原删除事务的可信完整旧/新事实：原Q仍在则保留且照计；确证原删除已提交才按实际U/Q行数返该名额；不能判明则继续围栏。不能换库重建、生成新代次或盲重试DELETE来制造证明；不存在可定位原事务事实时也不把缺行倒推为已处理。旧新事实仅属本地保留事务，不持久receipt、业务终态或无界删除墓碑，不以改名H/未计额表转移未确认线索。

尚存Q的正常冷恢复仍须重验原身份及当前授权、原GET重新取结果；桥或页面内存里的交付标识不跨重启复用。若旧Q已经可信删除，任何本地“已退休/无记录”至多说明缺发送资格，不能代替新的Host结果或恢复原保存；本片保证找回未确认线索，不承诺持久保存所有已确认结果。发送权已消耗但实际发送前中断的Q仍是“发送发生未证实”，missing不表示未发送，不删不重发；它只占自身名额，不阻挡其他同时满足业务结果证明和客户端交付确认的Q独立事务删除。不存在退休前缀R/按序排空或永久逐条墓碑，旧已删除票据缺U持续拒发。

设计默认活动窗口4096条、单条编码metadata最大4096字节（不含正文/秘密），逻辑payload上界16MiB；这是待测试的产品参数，不是实测物理峰值。容量按|U|+|Q|<=4096计，不按历史N或最早未知序号计；U转Q不增加总名额，仅满足可信确定结果与客户端交付确认的Q事务删除后才返该记录名额。Host已确定但浏览器未确认交付的Q仍占一格，不以移到其他表/易失缓存或TTL绕过总界。单个长期未决或待交付关联仅占一个名额，后续已确定且完成交付确认的操作可持续复用其余名额；4096条全被未回收关联占用时仍拒新，这是保留未确认线索的必要边界，不声称任意unknown或长期不确认交付下可持续写。序号达到上界拒新、不回绕。SQLite主库/journal及维护额外空间需单独测界；不预留等量物理保证。压缩在既有事务内回收逻辑窗口，物理页复用，不自动VACUUM/迁移。满额仍保留调查、压缩与退出控制能力，具体控制余量须后续证明。

journal代次、MAC密钥、发行上界N及U/Q必须在同一私有单写者生命周期下持久保存；不按浏览器会话重置，不自动恢复旧快照/替换journal或生成新代次来解锁容量。缺失、损坏、回退不可证明或不支持格式拒新，恢复工具不在本片。正常重启恢复发行上界与U/Q；冷重启后已删除旧票据只能拒绝，未确认Q线索必须可找回。MAC密钥不进入浏览器/日志；本机制不承诺对恶意同SID进程手动回滚文件的防护。

新增验证门：保留一条长期未决Q，后续超过4096次已确定且完成交付确认的保存/回收仍进展且该Q不丢；发送前中断的Q在missing下保留但不阻挡其他名额回收；全部4096条未回收时拒新且调查/退出可用，包含Host结果已确定但未完成浏览器交付确认的积压。Host已返回committed+匹配receipt、页面尚未ACK时，分别注入桥响应丢失/截断、关页、正常浏览器及桥冷重启：原key仍可找回，经重新授权GET取回原receipt，原确认到Host正式POST总次数为1；未实际发出的Q则保持0，不为满足次数而补发。ACK请求未到、DELETE已提交但ACK响应丢失、DELETE结果/物理结束unknown三类分别验证：前者保Q，中者允许Q已无且原页面的保存确认不降级，末者必须实际join与原journal可信旧新事实；不能把“ACK丢失”合并为一种保留结论。另验证提交确认UI状态前绝不ACK、导航/重登记后的旧ACK拒绝、清理失败不驱动正式POST及已删除Q缺失不被解释为业务成功；不持久receipt或擅自返额。已删除旧票据跨重启零转发，错误代次/MAC/缺行不得补建；发行、U转Q及符合双条件的Q删除各事务前/中/后故障不得丢未确认线索或恢复发送权；同候选/预览响应并发映射唯一。4096条与单条4096字节边界、SQLite实际物理占用/空闲页复用及控制余量均待验证，不以公式冒充实测。

第二消费者固定为独立CLI进程，使用独立控制管道签发的Host会话与独立公共协议请求链；不得借React的桥会话、桥HTTP入口、发送围栏或消费适配器完成业务调用。允许共享生成绑定、字节夹具和底层可信连接原语，不共享浏览器动作调度和结果解释适配。两端分别记录进程、会话的脱敏关联及实际请求证据，交错写读与冲突时证明请求均到同一Host公共业务owner；同一可执行文件不同模式仅为打包选择，不单独证明消费者独立。

## 4. 依赖顺序与并行策略（非任务清单）

1. 计划评审后，先验证真实连接关联、专用浏览器边界、精确会话结束、持久线索失败/unknown与竞争四类关键假设，再冻结公共接缝和有限参数。
2. 共享Host接缝必须与002核心owner串行集成；每次固定实际源码/契约/绑定截面，重新判断受影响001/002证据，不合并两个过时契约快照。
3. 接缝冻结后可并行原生窄桥/线索、独立CLI、Workbench消费实现，前提是明确独占文件及跨仓授权。React不能以Mock结果冒充Host接通。
4. 最后对同一固定Host、原生层、React生产产物和第二消费者联合验收。首版不等待M2完整对象业务，但实际Host构建必须包含并回归其已集成002变化。

共享集成边界：未来核心owner独占`internal/host/`启动装配、`internal/installation/auth.go`/`control.go`/security、`internal/transport/http/`、`contracts/openapi.json`及生成绑定；不由003消费者分支直接修改。002仍拥有其正在实施的Store/schema/预算/维护链，003不得另建或修改该链。交接条件是002核心owner提供可定位工作树/契约/生成物/实际exe指纹和在途修改范围，确认共享文件暂停并明确集成窗口；集成后双方按受影响范围复验再恢复修改，不以主会话名称当交接证据。当前仅文档设计，不取得独占代码写权。

会话兼容矩阵：旧001会话保持原入口、TTL、当前授权及原key语义；新003会话仅接受指定活实例TLS入口，在旧明文入口一律拒绝；旧会话不因新增TLS自动取得新能力，跨入口支持默认拒绝，新增需要独立契约定义。Host重启后所有旧003会话/连接池失效，旧001会话保持原有重启规则，不擅自修改001语义；新003必须经原生控制管道重新登记，持久Q只作线索，不能恢复授权。

安装维护兼容：003会话签发、续登记与撤销必须通过原安装owner就绪/维护/共享unknown围栏；维护开始先截止新会话签发并排空已准入签发写入，再进入002排他升级，不另建锁或绕过维护。安装授权/持久性unknown时拒绝依赖的新签发及相关业务；撤销无法确认则本地拒新并显示待确认，不谎报成功。工作区单独关闭不能无理由阻断其他区会话，维护结束也不自动恢复已退出上下文。验证旧001全链回归、新003明文拒绝、重启旧003拒绝、维护前后签发竞争及unknown负例；必须使用同一冻结Host构建，未运行不计通过。

### 4.1 T006当前输入复验与002共享交接（partial / 复验implemented，G305未冻结）

Owner A，承接原T006/G305，依T002–T005的实际可行性而非旧checkbox。对应FR001–008/012–015/018–021/024–026共19项、SC002/003/005/007/009–011的早期技术局部；不代V005–V013产品验收。当前统一授权覆盖本仓继续；002原T001–T036已按R120工程收口，其[393交接](<../../.data/logs/002-validation/final-summary-393-current-results.json>)固定392源码/契约/四镜像和Doc4后续增量，不把已变化工作树叫旧392输入。A当前串行持有共享文件，先记录当前所有非忽略Git输入、旧002截面差异和本次三个003文档变更；消费者不得在复验中改共享生产代码。

本片复用54的原9连接/授权父测试各三轮、3个浏览器父测试、5个journal重点父测试各三轮及完整17父测试，normal/tag三包vet；由当前唯一runner离线重建三个真实TEST exe并在对应包CWD执行，所有源/契约/工具/依赖/当前浏览器SHA和实际版本及结束后集合均记录。新私有C只冻结输入/缓存和诊断，源不漂移、非空选择器/无SKIP及真实输出才可计通过。未变旧原型也需当前输入实际运行，不能用54的run内no-drift推当前通过；不改旧fixture，只有实际失败才纠正技术假设并重验。

当前已安装Edge从旧155.0.4283.62变为156.0.4314.42，必须实际私有继承pipe/原PID-birth-image/专用干净profile/精确文档/cookie命令及首结束复验；只声明此Windows本地headless早期可行性，不增加交互桌面、多浏览器或任意策略支持，不下载/发浏览器，不设公开CDP端口、普通HTTP cookie fallback或削弱原根/身份门。T002/T004仍直接用真实Store/Auth的隔离夹具，T003合成loopback，T005合成消费/ACK；这些不是newHost/React/生产桥接通，线索完整物理峰值/控制余量仍未验。[R11当前394](<research.md#r11--当前输入edge156复验与002共享截面implemented--t006仍partial>)已实际完成以上11步骤、9A各三轮/3B/5C各三轮+17C和normal/tag vet，无SKIP或输入/镜像漂移；只证明当前隔离可行性。G305/T006仍不勾选：下一步依真实证据冻结唯一公共接缝/有限参数/文件owner及后续受影响回归，不并行两个核心owner，不以原型代签生产入口或物理上界。

### 4.2 T006最大编码占满与控制操作实测（implemented / R12测量完成，最终控制参数未冻结）

Owner B，由A串行集成，原T006/G305子片，承接FR013/015/024及SC007/010的journal早期局部；T005/R11已实际证明逻辑4096及单条4096，但旧满额载体主要为312字节，且`delivery(..., false)`在读取Q之前拒绝，不能当作最大编码满额调查已验。此技术反馈不改需求/批准/旧原型，不新增发布或三态笛卡尔门。

仅新建`internal/clientjournal/prototype_capacity_envelope_windows_test.go`：在原真实SQLite单写者上，通过原`issue`逐条发行4096个**每条实际编码4096字节**的不同绑定，先取得满额all-U形状，再通过原`consume`逐条转为all-Q，其中一条长期未决且其余模拟结果已确定但未确认UI交付的积压均仍计额；记录中间mixed形状，不只测一个稀疏Q端点。不直接SQL批量补行或改变schema/SQLite配置来冒充发行。实际第4097条拒绝且全事实不变；满额时原Q的精确完整metadata查询和`delivery(..., true)`正控必须发生，未提交UI状态的ACK拒绝不删Q；其余一条原U→Q事务及合格ACK实际完成，只退该一格，重新发行回满，长期Q保持。真正已知原DB/pins/lease首次结束后冷重开，代次/N/全部UQ完整事实相等，旧permit不可复活；新owner能查询原Q、满额拒新且可正常退出。`delivery`的matched-result/UI/ACK资格仍是原合成控制，精确Q读取/SQLite回收和本地物理关闭不代签Host公开GET、真实页面确认/退出或T015/T025生产验收。

测试fixture/所有partial返回owner在fallible构造前有独立全局槽，先保存返回值再检查错误，未知保留；采样goroutine在启动前root，stop+实际join后才汇总，cleanup不得把取消当结束。每一阶段记录main/journal存在/大小及PRAGMA page_count/page_size/freelist_count和逻辑metadata总量；固定并发采样只报告**观测峰值与采样覆盖**，不声称采样之间没有更大值或建立物理硬上界，不改文件共享/加VHDX/制造真实磁盘满。满额已完成控制操作的增量是此固定组合的证据，不等于已证明任意ENOSPC下控制保证或可直接冻结最终物理quota。

唯一负控仅缩短合成填充数量，随即仍执行真实下一次发行；它必须因实际成功发行击穿“满额应拒绝”断言，而非空选择器/构建错误。当前离线冻结所有Git输入/精确新runner、R11报告和旧七prototype SHA、Go/SQLite/private compat profile，当前new TEST-only镜像执行正控一次及负控一次，normal/tag本包vet；原394的七文件和浏览器环境不变证据按实际SHA复用，不无依据重跑Edge/旧长循环。显式format是唯一可写新源码的验证模式，任何source/pathset漂移则不计pass；日志仅ignored`.data/logs/003-validation/`。本片结果只补有限参数的测量输入，G305/最终DTO、生产集成和物理控制余量的最终声明仍需后续据证明确冻结，不提前勾T006。

[R12/395实际结果](<research.md#r12--最大编码满窗本地控制与完整冷恢复implemented--t006仍partial>)已完成当前663输入/一NEW TEST-only源的8步骤：最大编码满窗all-U/mixed/all-Q、精确Q、本地consume/合格ACK退一格及补回、真实首次结束/完整同代次cold均PASS；短填充后第9条真实成功击穿满额断言的有效负控和normal/tag本包vet通过，原七prototype/生产/依赖/配置无改。实际逻辑16777216字节，main EOF最大20533248、rollback观测90800、顺序非原子观测最大和20624048，692940样本及每阶段存在/EOF/page统计分别记录；不是allocated bytes、未观测峰值或最终reserve/硬quota。最终有限桥/DTO/版本/owner和控制参数仍需T006后续冻结；G305未勾、不代签Host GET/真实UI或生产闭环。

### 4.3 T006有限main-page与控制headroom候选（implemented / R13固定形状实测，非生产原子H或物理quota）

Owner B/A串行集成，原T006/G305，FR013/015/024及SC007/010的journal技术局部，依R11/R12。仅NEW `internal/clientjournal/prototype_page_budget_windows_test.go`，不改原八TEST源/生产fixed profile/契约。原SQLite设置先逐项实际读回（DELETE/EXTRA、page_size、locking_mode、temp_store等），候选每次真实open后显式设置并读回main `max_page_count=8192`；当前候选仅支持page_size=4096，即main页限32MiB。发行候选H=6144已用页（page_count−freelist_count）、保守单次预留64页，留M−H=2048页为main控制候选。原issue前有界检查，issue成功后核实际已用页及增量；64只是本固定4096最大编码序列的实测候选，不是假证明任意SQLite操作最坏界。后续生产T014必须在真正事务中执行实际headroom约束，不以事后断言当生产原子限额。源response仅在该单线程合成预检通过后才构造，不存在guard拒后重入旧source；记录guard命中数/成功发行最大已用页、净增长和main增长，控制阶段pages/used分别记录。若满窗未达到H截止，则明确未测实际H拒绝，不能用32页SQLite负控或纯predicate测试代签生产准入。

使用原真实issue/consume/合格ACK及known-close/cold，最大编码4096全U→mixed→全Q、原Q精确读取/未确认ACK拒绝、完整cold、退一格/补回和长期Q均验证；每次open重设/核main cap，因为不得假设该PRAGMA跨连接持久。只对发行做候选准入，控制沿原事务、所有阶段main页<=M且记录剩余页，实际操作的页增长单列。max_page_count不是预留文件，不限制rollback-journal/temp/NTFS allocated clusters；页余量不保证ENOSPC，不新造等量16MiB物理要求，也不代Host GET/UI或G305完整接缝。

负控只把同一真实连接main cap降为32页（不改H/原issue），必须在原SQLite真实发行中因SQLITE_FULL/原error拒绝而击穿最大窗口成功断言，不接受readback/构造/空选择器代替。普通candidate完整一次、负控一次、normal/tag本包vet；离线新全Git/input/pathset与原R12/currentEdge/profile/TESTimage精确绑定。所有partial owner/首次关闭及sampler沿已root的原测量槽，未知不重试、不返额；无bulk造行/清空库/改journal模式/下载/系统策略/外仓写。结果只能冻结受支持的main-page候选范围，其他T006公开DTO/有限桥/版本/子文件owner仍需完成，未验不勾G305。

[R13/396](<research.md#r13--有限main-page候选与实际sqlite满页负控implemented--非生产原子h准入>)已在当前664输入、原八TEST/生产/profile未改的条件下实测8步骤7zero+primary13负控1，两次真实open均读回DELETE/EXTRA3/locking normal/temp0/page4096和显式M8192。原最大4096×4096及完整本地cold/控制全过，正控787.23秒、normal/tag vet过；32页负控原第22次SQL发行actual FULL13，而非配置/构造失败。实际issue最大used4931、净增长/main增长各5，控制阶段P5011/used4931；guard命中0，未碰H拒绝，pure边界unit不代签真实生产限额或普遍64。M−H2048只是候选差值、EOF采样非物理quota。此候选已提供T006设计数值输入；T014仍须原事务准入和产品控制验收，G305尚需唯一公开DTO/有限桥/版本/子文件owner冻结，不新增ENOSPC或三态门。

### 4.4 T006接口与有限参数冻结截面（implemented / R14设计冻结完成；不是生产实现）

Owner A，依T002–T005及R11当前输入、R12最大编码、R13有限main候选，承接原T006/G305的FR001–008/012–015/018–021/024–026共19项及SC002/003/005/007/009–011技术局部。002原R120/393核心已交接，当前无另一002共享代码写者；本片仅三个003文档和ignored记录。下列是A/B/C后续实现的唯一冻结输入，结构新增最终只进入原`contracts/openapi.json`，T010生成原Go/TS绑定；这里的冻结记录摘要**不是**新Host运行契约摘要，不创建第二份业务OpenAPI。T007/T008/T009实现后、T010生成再以真实产物验收，不将后置T023倒作T007前置。

#### 4.4.1 Host单owner接缝

- 复用原Store、Authenticator.currentLocked/当前DB会话、DirectoryCoordinator、WorkspaceOpener和原容量/维护/unknown门。003的实例/传输绑定是原Host的附加拒绝条件，不是许可来源；每次metadata查询和交付前仍验证原当前主体/授权版本。新增会话沿原pending delivery→ACK激活/失败撤销、`pendingDeliveries`与原Store生命周期；不另开SQLite连接或维护锁。原001 pipe/token/明文入口、TTL及重启语义不变。
- **会话种类隔离必须覆盖旧明文与重启**：T007不能只复制prototype的TLS handler私有map。冻结003 bearer为`ac1.`+canonical43字符base64url(raw32bytes)，DB `token_hash=lowerhex(SHA256("ABOARD-CLIENT-SESSION-V1\n"+canonicalInstallationUUID+"\n"+hostInstanceDigest+"\n"+raw43))`；各段canonical ASCII且禁换行，无歧义。legacy只走原43字符及SHA256(encoded)域；003验证失败不fallback，不同时存legacy hash，strip前缀后的旧路径无DB记录。派生必须贯穿签发、ConfirmDelivery、Revoke/RevokeSession与Authorization.hash，而非只改HTTPwrapper。Host持唯一不可请求伪造的live binding，Authorization持原引用并checkLocked复查，实际TLS1.3/精确authority/活实例、当前session generation（原DB session_id）和原DB授权同时成立才准入。重启新instance、同进程listener替换先使旧binding无效；不从持久文件恢复binding，不加自动schema迁移或第二授权库。
- 原client_session只有session_id/token_hash/principal/expires/revoked/grant_version，无transport列；以上沿原64hex域，不迁schema。零grant合成主体须真实身份可读/空集合：新增显式私有`client.identity`交付purpose，Create/Activate均在原owner当前授权/原subject/version下核验，legacy Issue/Grant.Action不能申请该purpose，绝不把空action扩成权限或造grant。之后具体业务仍原currentLocked/CheckAction查当前DB。新增Issue/ACK激活及metadata读取在原Store.mu下重核workspaceStopping/维护cutoff，沿pendingDeliveries和失败撤销/unknown；**不把cutoff全局塞入readyLocked**去取消已有session/accepted业务。
- Host新接口严格如下；`UUID/Digest/Revision/PositiveRevision/Capability/WorkspaceView/ErrorView`均引用原唯一契约；revision仍canonical十进制字符串，时间为UTC RFC3339Nano，额外字段/重复key/非法Unicode拒绝。原`/v1/context`、open/records/preview/commit/operation/close结构不被同名替换。

| 新Host路由 | request与唯一response字段 | owner/语义 |
| --- | --- | --- |
| `GET /v1/client-context` | `ClientContextView {protocolVersion:1, installationId:UUID, tenantId:UUID, principalId:UUID, sessionGeneration:UUID, hostInstanceId:Digest, grantVersion:PositiveRevision, capabilities:Capability[<=16], supportedWorkspaceProfiles:["m1-text"]}` | 当前DB主体/能力；generation由可信登记产生，不收浏览器自报身份，无私有path/bearer/pin |
| `GET /v1/visible-workspaces?limit&cursor` | limit默认20/max100；`VisibleWorkspacesView {grantVersion, sessionGeneration, items:[{workspaceId:UUID, registrationVersion:PositiveRevision}], nextCursor?:string<=1024UTF8}` | 原登记+当前授权有界SQL/keyset，空集有效；不探目录/提前open，不向无权者返回目标或路径 |
| `POST /v1/client-open-preconditions` | `{workspaceId:UUID}` → `ClientOpenPreconditionView {installationId,tenantId,principalId,sessionGeneration,hostInstanceId,workspaceId,purpose:"open",expectedRegistrationVersion:PositiveRevision,snapshotId:string<=1024,expiresAt}` | 只读原registration/CAS；30秒HMAC快照绑定全部范围、用途和基线，不是授权 |
| `POST /v1/client-workspace-opens` | `{workspaceId:UUID, expectedRegistrationVersion:PositiveRevision, snapshotId:string<=1024}` → 原`WorkspaceView` | 先重验快照/当前授权/原CAS再委托唯一WorkspaceOpener，不在拒绝时更新用户基线。003直接调用legacy open不能绕过该用途前置 |
| `POST /v1/client-session-end` | `{sessionGeneration:UUID}` → `ClientSessionEndView {sessionGeneration:UUID, ended:true}`；仅已确认原DB撤销返回200；否则原`ErrorView` | 只撤当前自身会话，非operator撤别人的入口；已accepted业务不取消。桥先本地截止，结果不明仍待确认、不恢复页面 |

分页cursor为不透明HMAC材料，绑定installation/principal/实际session hash（不外发）/generation/原grantVersion/lastWorkspaceId/limit，最多1024UTF8；伪造/跨主体403受限拒绝，真实已陈旧版本409 STALE_HANDLE，不披露其他主体/目标。open snapshot同样绑定实例及用途，过期/陈旧409 STALE_HANDLE且未获取新open epoch；不可把客户端UUID或缓存当授权。所有普通错误沿原no-store/correlation/脱敏ErrorView。快照和cursor只给当前已授权者，不用于目录/安装管理；均采用当前Host内存密钥的有限自含canonical签名token，不登记无界token表、不恢复旧实例密钥，验签后仍查询当前DB授权。

首版客户端只消费**m1-text、format2/schema1**当前文本能力；Host已集成M2照常独立运行，但003在实际open/profile证据出现前不猜兼容，M2/未知/未接线能力显示unavailable，不当作M1降级或开放附件/历史。visible item不预读物理profile或作“已兼容”承诺。现有字段/body/摘要/receipt/原key三轴保持原语义；TS业务结构只能从T010唯一生成物来。

#### 4.4.2 受保护控制、可信连接与有限私有桥

原SID/peer/server/PID-birth-image/first-instance原语承载新增独立kind `client-bootstrap-v1`，legacy控制帧原版本/接口保留；单帧uint32长度+strict JSON最大8192UTF8，未知kind/version不fallback。请求`{version:1,kind:"client-bootstrap-v1",nonce:Digest}`；回复`{version:1,kind,nonce,installationId,tenantId,principalId,hostInstanceId:Digest,sessionGeneration:UUID,authority,pin:Digest,process:{pid,birth:Revision,imageDigest:Digest},expiresAt,bearer}`，私有回复实际frame<=8192且不进URL/argv/renderer/普通日志。peer实际句柄/image路径在native内部验证，不作为浏览器字段。nonce一次、bootstrap有效30秒、会话TTL<=300秒，与原pending/ACK/ConfirmDelivery及失败撤销串联；同Host原Store最多64个未结束client binding（含pending/unknown），耗尽拒新，不淘汰未知。

Host仅loopback TLS1.3，临时内存证书/私钥及SPKI pin绑定原活实例。客户端每次新连接先验证证书用途/时间/pin和原进程存活，再发bearer；冻结首版**不复用连接**、无TLS会话恢复/0RTT/代理/重定向/HTTP2，正式POST body不可replay、GetBody=nil，不带Idempotency-Key自动重试提示。错误对端bearer可见0；每客户端单独登记/连接代次，不自动init/repair/restart/stop Host，不将关闭页面解释为结束会话。

私有bridge仅`POST /bridge/v1/join`和`POST /bridge/v1/actions`，及固定可信静态`/app/`，不反射HostURL/方法/路径。exact127.0.0.1 authority+Origin、`Sec-Fetch-Site:same-origin`/`Sec-Fetch-Mode:cors|same-origin`/`Sec-Fetch-Dest:empty`完整且逐项匹配、Content-Type application/json、pageId/CSRF/client-envelope每次检查；未知字段/action/version拒绝且不进入Hostdispatch，所有普通HTTP零Set-Cookie/CORS允许。join只能消费原私有CDP对**精确uniqueContextId/文档代次**交付的短时材料，不从GET/cookie恢复。join reply仅原文档内存持`{version:1,pageId:UUID,csrf:opaque256bit,clientGeneration:UUID,documentGeneration:UUID}`。client-envelope名称`aboard_client_envelope`，host-only/Path=/、HttpOnly/SameSite=Strict、当前已验loopback HTTP的Secure=false；只经已验Storage.setCookies并getCookies实际核对，不让旧HTTP回包覆盖。Cookie不按端口隔离，绝不单独授page资格。

动作包`{version:1,requestId:UUID,pageId:UUID,clientGeneration:UUID,documentGeneration:UUID,scope?:{installationId,principalId,sessionGeneration,workspaceId,openEpoch},action,input}`，`X-Aboard-CSRF`原内存随机值，`action`是下表穷尽union，input按action strict schema；所有目标必须在当前范围中重校。scope只用于关联/拒迟到，不授权限。workspaceStatus/listRecords/getRecord/previewText/confirmText/getOperation/reconcileOperation/closeWorkspace及已选目标的investigateClue/ackDelivery必须携完整五维scope并逐项相等，不能接受缺字段或从全局当前区补齐；context/visibleWorkspaces/exitClient无workspace scope，openPrecondition/openWorkspace使用目标及用途快照。输入/输出业务对象引用Host原生成类型，不手写同名正文/receipt规范。

| action | input / 输出与调度限制 |
| --- | --- |
| context / visibleWorkspaces | 空object / `{limit?:1..100,cursor?:<=1024}`，只返回当前Host投影 |
| openPrecondition / openWorkspace | `{workspaceId}` / `{workspaceId,expectedRegistrationVersion,snapshotId}`；显式用户动作，复用新唯一Host接缝 |
| workspaceStatus / listRecords / getRecord | 原workspace/epoch范围；list原limit默认20/max100和cursor，get原resourceId；只current文本，无历史/路径/附件 |
| previewText | 原`PreviewRecord`生成输入，原title<=256Unicode scalar/body<=65536UTF8且禁NUL、不归一化；预览结果在native先建立持久唯一confirmationHandle后才交页面 |
| confirmText | `{confirmationHandle:opaque<=256}`，原候选/base/operationId只从journal取；当前scope相符，U→Q持久确认后一次发送，绝不由页面补candidate/key/正文 |
| getOperation / reconcileOperation | 原workspace/epoch/key；reconcile必须明确用户动作并沿原DTO，无missing→retry或POST重放 |
| closeWorkspace | 原`CloseWorkspace`生成输入，原operationId/openEpoch；只close当前目标，不stop Host或其他会话 |
| listClues / investigateClue | list输入`{limit?:1..100,cursor?:string<=1024}`默认20，返回`{items:[{confirmationHandle,workspaceId,operationId}],nextCursor?}`仅当前原身份metadata；cursor HMAC绑定journal-generation/current-principal/client-generation/last-sequence/limit，伪造403、代次陈旧409，不因分页退休Q。investigate携confirmationHandle和explicitReconcile:boolean（默认false），先当前授权/原key调查，不恢复发送权 |
| ackDelivery | `{confirmationHandle,deliveryId:opaque256bit}`；页面已校验完整Host范围/receipt并提交确认UI后才发，独立本地删除，非业务确认/权限 |
| exitClient | 空object；先原子截止全部页面/材料/迟到交付，再只撤当前Hostsession；已确认/待确认分开，其他会话/Host不受影响 |

response为有限判别union，公共包`{version:1,requestId,action,scope?,kind:"host"|"local",bridgeError?:固定枚举}`，action必须回显原请求。bridgeError仅invalidInput/invalidContext/unavailable/limitReached/transportUnconfirmed/journalFenced；不得伪造Host logical/persistence/physical字段。
- `kind=host`附加`{transport:"complete"|"unconfirmed"|"notSent",hostStatus?:200..599,host?:原生成DTO,deliveryId?:opaque256bit,client?:按action的有限补充}`。`hostStatus/host/deliveryId`只有`complete`且完整TLS/类型/范围校验后才可出现；`unconfirmed`仅bridgeError=transportUnconfirmed、`notSent`仅固定bridgeError，两者无Host结果/状态/receipt/deliveryId/client，不把已发送但未知写成notSent。client只在previewText的complete合法，精确`{confirmationHandle}`，由native已验证原PreviewView并持久Issue后生成；不得改原Host DTO/重发预览或把不完整预览给页面。
- `kind=local`附加`{local?:按action有限结果}`，只允许listClues、ackDelivery、exitClient及输入/本地拒绝，**不存在transport/hostStatus/host/deliveryId/client**；成功时必须local且不得bridgeError，local按action精确为listClues的`{items:[{confirmationHandle,workspaceId,operationId}],nextCursor?}`、ackDelivery的`{retired:true}`（原本地DELETE已知确认）、exitClient的`{localCutoff:true,sessionEnd:"confirmed"|"unconfirmed"}`。confirmed仅原Host当前会话撤销完整确认后，unconfirmed仍本地截止；错误只bridgeError、无成功local。不得用local分支替代任何Host业务结果或给recovered Q补发送权。
`complete`只说明取到完整可信Host响应，不表示保存成功；200/201/202仍核原OperationView与匹配receipt；202 committed+viewDeliveryFailed保确认。未交付Q/清理失败不降级已确认，不为ACK再save。UI/日志不输出Host bearer/pin/私有定位或正文diagnostic。

有限参数冻结为：join JSON4096B、action JSON1048576B（实际读界前拒过量）、Host response原全量上界67108864B且各生成字段再限、bridge header16384B；最多16活page/32未用join材料（30秒）/8未结束action（无无界waitqueue）；同一CDP controller16待命令/16未结束writer/64事件，每帧4194304B**在分配/增长前**检查、JSON深度32，unknown槽仍计额、不得靠timeout/淘汰返额；溢出拒相应新入并保旧owner，不把丢事件说导航已处理。原T003后验ReadBytes长度检查只属历史prototype，T012按此实现生产有界parser。HTTP静态fallback只匹配GET/HEAD的可信资源/页面路径，不吞`/bridge/`、HostAPI或认证错误；固定manifest记录实际bundle SHA/来源/license，CSP不靠新增unsafe-eval或通配Origin维持功能，真实UI在T018–022完成。

#### 4.4.3 journal格式、有限页设计和交付签名

journal-format1，原单写者/真实localNTFS、DELETE/EXTRA3/1连接/win32/foreign_keys1/busy_timeout1000/trusted_schema0/mmap0；支持page4096，每open设置核M8192，逻辑U+Q<=4096、每行<=4096、N<=9223372036854775807不回绕。生产发行在原事务内读`P−F`，预留64后<=H6144，SQL成功后pre-COMMIT再验实际used<=H，否则ROLLBACK已知拒新；SQLite/物理结果unknown围栏不重试/不返额。64仅已验固定序列候选，事务内回滚才是生产界，T014/T025实际近H与故障验证负责，R13 guard-hit0不代签。控制consume/ACK/查询/退出仍沿原owner，M−H2048是main数值headroom，非预分配空间/rollback-temp-cluster quota或ENOSPC保证，不自动VACUUM/换库/改代次。

持久binding只含`installationId/tenantId/principalId/workspaceId/openEpoch/hostInstanceId/sessionGeneration/previewRequestId/candidateId/previewVersion/baseRevision/baseDigest?/operationId/candidateDigest`及原commit最小必要字段、journal generation/N/MAC；每行完整编码仍<=4096，不存正文/receipt/业务终态/Host秘密。candidate/preview响应原native唯一owner先验证来源再`Issue(binding)`，同候选/version在U/Q以unique关联；delete后不提供按浏览器旧candidate补票接口，旧source已消费或冷重启不存在，不能重新发行。`Consume(handle)->Attempt`同事务U→Q，落盘确认后当前内存一次Attempt，网络结束/取消不恢复；`Lookup(handle)->Clue`只调查；`Acknowledge(handle,deliveryId)`只有当前page/完整原结果交付证据才删Q；`Close()->known|fenced`首次结果缓存，不以一次SELECT或cancel当结束。事务unknown沿原DB/回调/pins首次实际end和原事务完整old/new证明，不能盲DELETE/重建。

冻结本地确定性编码：`metadata=uint32BE(payloadBytes)||payload||HMAC-SHA256(K,ASCII("ABOARD-JOURNAL-TICKET-V1\n")||uint32BE(payloadBytes)||payload)`，完整metadata<=4096。payload是无空白/无重复/无未知字段的UTF8 JSON；递归key按ASCII字节升序，UUID/digest/canonical正整数十进制均原canonical ASCII（整数域以十进制string），可选baseDigest缺省时省key，禁止null/非canonical escape或重编码替代；包含上述binding、`journalFormat:"1"/journalGeneration/sequence`。其中previewVersion就是原Host单调preview_version，不接受页面自报；最小commit原字段由原生成CommitRecord引用，不能包含正文。K为该journal代次原生随机32byte秘密，保留在仅该用户可读的本地journal状态，不进renderer/argv/log/Host；改变K必须新journalGeneration，旧Q不得因重置而抹去。confirmationHandle为canonical无pad base64url(`uuid16BE(journalGeneration)||uint64BE(sequence)||MAC32`)，精确75ASCII字符；先当前owner范围按generation/sequence精确查行，再constant-time核MAC/原metadata完整编码，MAC不是Host授权。唯一候选关联键为`SHA256(ASCII("ABOARD-JOURNAL-SOURCE-V1\n")||canonical(installationId,principalId,sessionGeneration,previewRequestId,candidateId,previewVersion,candidateDigest))`，沿同一无歧义payload编码，不用文本拼接或正文。T014必须补独立固定正反编码/别名向量和真实transaction H/unknown；T006设计记录不能代这些运行。

所有外部回调/HTTP结果携当前五维scope并在交付前复核；journal持久原scope与当前恢复认证scope分列，正常冷重启可在原installation/principal重新授权调查旧key，不把旧epoch当当前视图，不恢复旧permit。真实页面UI提交→独立ACK→本地delete和既有FR恢复时序不变；本接口冻结不是生产ACK/UI通过。

#### 4.4.4 ABC文件owner与输入/版本记录

| owner | 精确独占生产边界（尚待后任务创建） | 共享/停止规则 |
| --- | --- | --- |
| A | `internal/host/client_runtime_windows.go`及原runtime/config；`internal/installation/client_sessions_windows.go`、`client_context_windows.go`、`client_registry_windows.go`及原auth/control/Store、`internal/storage/sqlite/installation_auth.go`的原会话Create/Activate受限扩展；`internal/installation/security/client_control_windows.go`及原pipe；`internal/transport/http/client_routes_windows.go`、`client_views_windows.go`及server/dispatch；唯一OpenAPI/原生成器/全部生成GoTS；`internal/clientnative/bootstrap_windows.go`、`connection_windows.go`；`cmd/aboard-client/main_windows.go` | 原共享链只A串行；新文件名不授并发修改原Store/schema/预算；接口变化重核本冻结与受影响回归 |
| B | `internal/clientnative/browser_process_windows.go`、`browser_documents_windows.go`、`browser_cookie_windows.go`、`browser_rpc_windows.go`；`internal/clientbridge/`下server/actions/security/delivery/static生产子文件；`internal/clientjournal/`下store/codec/tickets生产子文件及各自新tests | 依T010/011后才T012/014并行，T015/016共享交付不擅自并行；不改原TEST-only九文件或产品Host数据库 |
| C | `cmd/aboard-client/cli_windows.go`和仅CLI独立消费/验收子文件；本仓前端产物清单/生成绑定消费记录 | 不借bridgeHTTP/journal/业务adapter；跨仓Workbench独占路径及写权仍T018实际核定，当前无外仓写权限，不用本表扩权 |

控制客户端、Host client API、bridge、journal分别版本1，协商必须显式相等，不按OpenAPI info.version或操作数猜；不识别即unavailable，legacy版本保原。首版支持当前已验Windows amd64/localNTFS/Go1.27.2固定SQLite、用户已有Edge156.0.4314.42专用headless技术截面；真实interactive生产支持须T024/T031实测，不把R11扩成全平台/全版本保证。静态来源/许可/外仓/ENOSPC/raceCGO0和用户最终验收/发布仍按原未验/deferred门。

以下JSON仅为本节**设计冻结索引/机器一致性检查**，不另定义业务结构或实现API；字段细节以上表与唯一Host契约增量为准。正式实现改变任一值需A更新此原plan及受影响tasks/analyze/回归，不静默fallback。

<!-- T006_FREEZE_V1_BEGIN -->
```json
{"version":1,"stage":"T006","productionVerified":false,"hostApiVersion":1,"controlVersion":1,"bridgeVersion":1,"journalVersion":1,"sessionEncoding":"ac1.raw43","sessionHashDomain":"ABOARD-CLIENT-SESSION-V1","identityDeliveryPurpose":"client.identity","journalMetadataEncoding":"u32be-canonical-ascii-json-hmac256-v1","journalMacDomain":"ABOARD-JOURNAL-TICKET-V1","journalSourceDomain":"ABOARD-JOURNAL-SOURCE-V1","completeHostTransport":"complete","bridgeResponseKinds":["host","local"],"localResultActions":["listClues","ackDelivery","exitClient"],"previewClientResult":"confirmationHandle","requiredWorkspaceActions":["workspaceStatus","listRecords","getRecord","previewText","confirmText","getOperation","reconcileOperation","closeWorkspace","investigateClue","ackDelivery"],"supportedWorkspaceProfiles":["m1-text"],"publicRoutes":["GET /v1/client-context","GET /v1/visible-workspaces","POST /v1/client-open-preconditions","POST /v1/client-workspace-opens","POST /v1/client-session-end"],"bridgeActions":["context","visibleWorkspaces","openPrecondition","openWorkspace","workspaceStatus","listRecords","getRecord","previewText","confirmText","getOperation","reconcileOperation","closeWorkspace","listClues","investigateClue","ackDelivery","exitClient"],"limits":{"visibleDefault":20,"visibleMax":100,"cursorBytes":1024,"openSnapshotSeconds":30,"controlFrameBytes":8192,"bootstrapSeconds":30,"sessionSeconds":300,"clientBindings":64,"clientBearerBytes":47,"confirmationHandleBytes":75,"joinBodyBytes":4096,"actionBodyBytes":1048576,"hostResponseBytes":67108864,"headerBytes":16384,"pages":16,"joinTickets":32,"actions":8,"cdpFrameBytes":4194304,"cdpPending":16,"cdpWriters":16,"cdpEvents":64,"jsonDepth":32,"journalRows":4096,"journalRowBytes":4096,"journalPageSize":4096,"journalMaxPages":8192,"journalIssueUsedPages":6144,"journalIssueAllowance":64},"owners":{"A":["internal/host/client_runtime_windows.go","internal/installation/client_sessions_windows.go","internal/installation/client_context_windows.go","internal/installation/client_registry_windows.go","internal/installation/security/client_control_windows.go","internal/transport/http/client_routes_windows.go","internal/transport/http/client_views_windows.go","internal/clientnative/bootstrap_windows.go","internal/clientnative/connection_windows.go","cmd/aboard-client/main_windows.go","internal/storage/sqlite/installation_auth.go","contracts/openapi.json"],"B":["internal/clientnative/browser_process_windows.go","internal/clientnative/browser_documents_windows.go","internal/clientnative/browser_cookie_windows.go","internal/clientnative/browser_rpc_windows.go","internal/clientbridge/","internal/clientjournal/"],"C":["cmd/aboard-client/cli_windows.go"]},"externalWorkbenchWrite":false,"evidence":["R11/394","R12/395","R13/396","002/R120/393"],"productionTasks":["T007","T008","T009","T010","T011","T012","T013","T014","T015","T016","T017"],"unchangedAuthorities":["contracts/openapi.json","specs/001-host-workspace-foundation/authorization.md","specs/003-m6-client-integration/spec.md"],"deferredClaims":["productionTlsBinding","productionAtomicHeadroom","realHostGetUiAck","interactiveBrowser","physicalDiskQuota","ENOSPC","externalWorkbench","release"]}
```
<!-- T006_FREEZE_V1_END -->

## 5. 验证映射与证据（产品组合未运行；隔离历史见research R10）

| 需求覆盖 | 验证与预期证据 |
| --- | --- |
| US1；FR001–005/018–022/024–026；SC001/009–012 | 启动/发现正控，坏安装/错误服务/实例替换拒绝，每实际bearer连接身份；私有CDP管道/实际浏览器进程及精确文档正控，核验到执行导航竞争；client-envelope受控建立/普通HTTP零Set-Cookie，并发首次两页、刷新、一次核销/实过期、仅cookie恢复拒绝、重新登记/退出旧命令迟到；同会话多页面退出与独立会话继续；真实Origin/CSRF/跨端口Cookie负例；秘密扫描 |
| US2；FR006–009/019/026；SC002–003 | owner/A-only/无权、空集合/分页/陈旧游标、串区拒绝；五类迟到响应隔离；open前置陈旧不替换CAS |
| US3；FR009–013/016；SC001/004/006–007 | 两端各A/B三条创建更新，当前内容/版本/receipt逐次核对；10轮同base竞争；200/202/截断映射；双击与重挂载不再次发送 |
| US4；FR008/011–015/018/024；SC003–005/007/010 | RST/截断/超时各≥3次，逐层实际POST发送为证；Host已确定而页面尚未提交确认UI时丢响应/正常冷重启仍按原key恢复；ACK请求丢失、已删除后的ACK回包丢失、DELETE unknown实际join/fresh分开；晚/错ACK拒绝，已确认结果不因清理失败降级；Q计额及正常冷恢复、满额/并发/错误身份拒绝；不以缺Q证明业务结果或补发 |
| US5；FR001/016–017/025；SC001/006/008 | 第二CLI双向互读当前正式文本/receipt，正常完整停机重开；accepted实际pending下取消等待/关页/退出浏览器，B继续，物理join后才释放 |
| US6；FR003–005/018–026；SC009–012 | 固定React生产包、绑定漂移、深链接/API隔离、键盘/focus/主题、许可notices、可信资源发布与全部安全负例 |

将来在本项目原runner执行：`tools/go.ps1 test ./... -count=1 -timeout=1200s`及受影响normal/tag测试、vet；专用M6选择器仅在测试实际存在后冻结，禁止空选择器/SKIP冒充pass。原生浏览器测试需另获执行权限与已有合格工具，不在本阶段安装。每次记录实际源码/依赖/契约/生成器/exe/浏览器版本、进程及发送次数、真实数据库效果；日志只放忽略`.data/logs/003-validation/`且使用合成数据。

<details>
<summary>展开§4.5–4.52逐片设计反馈与执行历史（保留原锚点；实际当前门见tasks）</summary>

### 4.5 T007生产接线首片：原DB域分离与活binding拒绝（implemented / R15局部，T007整体未验）

Owner A串行；FR001–003/005/018/020–021/024及SC009/010/011传输授权局部，原T007/V005，依T006/R14。先在原Store内增加唯一当前client-transport引用（authority+随机实例摘要，非权限/会话表），AuthorizeHTTP的ac1路径必须实际TLS1.3/完成握手/非resumption/精确当前authority并用原DB域hash；原Authorize和明文/strip路径只认legacy43。Authorization持原binding并在每次原checkLocked重查Store同引用，替换listener binding、Invalidate及Store关闭使旧authorization失效。原RevokeSession按同域原hash撤销，原current DB grant-version/expiry检查、location与unknown门保持，不另存legacy hash/原始token。

首片只增加client_transport.go/专用测试，串行改auth.go/Store字段；TEST构造真实原DB pending/ACK行与真实TLS HTTP，明确不是新增生产mint API。验证canonical域和安装/实例差异、真实TLS正控与明文/TLS1.2/换authority/strip/direct-Authorize拒绝、当前撤权/撤销/实际到期、旧binding晚到check和完整Store冷重开、旧真实pipe session仍可明文。后续原T007继续把原protected pipe pending/ACK/zero-grant purpose接到本域并装配真实Host TLS listener；当前片不提前勾T007。现有ingress解析明文且ConnContext识别原具体连接，TLS层次必须后续实际验证，不能直接外包旧listener解析ciphertext。生产issuer、64未结束session计额、精确SPKI/nonce/进程bootstrap、未知交付及真实Host重启仍未验；不改变冻结接口、schema或旧001语义。

R15已按[当前398完整输入](<../../.data/logs/003-validation/client-domain-398-full-current-domain-results.json>)完成本片：原Store/原DB生产边界已接，normal/tag新三个父测试各三次、原11个auth父一次、HTTP/Host normal/tag及两vet实际通过；真实TLS1.2拒绝与同Store第二Authenticator替换已测，incomplete/resumed只单元predicate。canonical域有独立stdlib Python固定向量。首focus空action测试错误/原失败记录保留，测试显式operator修正不改变产品权限。仅按影响复验，不重复长journal/Edge；旧397设计及002业务证明保历史但不称当前新增生产source仍等于旧输入。生产client issuer、专用pipe、Host TLS listener/SPKI/nonce/bootstrap及64未结束session容量仍在原T007后续，当前未勾T007。

### 4.6 T007原DB client.identity与有界pending发行（implemented / R16私有issuer局部，原bootstrap未接）

Owner A串行，FR001–003/005/018/020–021/024及SC009/010/011当前身份/交付局部，原T007/V005，依T006/R14和R15。先实现供后续受保护bootstrap唯一调用的私有client issuer，不新增HTTP/raw-mint或第二session表；目前不声称管道/Host TLS装配已经完成。原CreateInstallationSession/legacy Grant仍拒绝client.identity；原DB增加显式CreateClientSession，SessionDelivery携同包私有identity-purpose provenance，Create/Activate都核原当前owner、真实subject及grant-version；零grant主体只有身份，不获workspace/operator权限。原schema和legacy行为不变。

发行使用当前原Store ClientTransportBinding和ac1域，ACK再核同引用/原workspaceStopping cutoff、当前DB版本/到期；失败沿原背景撤销和authUnknown围栏。原Store.mu内登记最多64个client槽（仅hash/原绑定/expiry/交付状态，非权限缓存），在原SQL写之前登记并增加pendingDeliveries；SQL结果未知/撤销未知仍保槽，绝不淘汰。只有原交付流程实际返回后finish才结束pending计数；已知撤销且交付已结束，或已知完成交付后真实到期，才可移除计额。活会话、pending和unknown均计入，legacy不占此新域额度。实例替换不释放未结束交付，cleanup撤销不因cutoff禁止；正常Store关闭仍由原DB→pins→lease首次结束规则负责，不把session失效当IO排空。

本片验证真实DB的零grant身份与legacy拒绝、pending不可用/正确ACK后TLS可用、ACK时撤权/version或cutoff/换binding拒绝并撤销、64/65有界及未结束槽不返额、已知撤销+finish返一格、真实expiry、原legacy pipe/auth和受影响SQL/HTTP/Host normal/tag/vet。TEST直接调用私有issuer仅验证这个原owner机制，不冒充已接原受保护pipe；实际bootstrap frame/nonce/SPKI/peer/ACK物理结束及独立Host容量完整闭环保留原T007后续。只改本scope生产文件/新tests和三个状态文档，原保护pipe/旧测试不放宽；按当前输入冻结运行并记录新生产delta，不复用R15原source[]冒当前通过。

R16已按[当前399完整层](<../../.data/logs/003-validation/client-issuer-399-full-current-private-issuer-results.json>)与[同C原SQL父verbose](<../../.data/logs/003-validation/client-issuer-399-sql-verbose-results.json>)实测本私有发行片：668输入、13步骤0，normal/tag七client父各3、原11auth父、原四SQL授权父两模式、HTTP/Host和两vet均过。真实零grant身份/独立scope拒绝及owner成功、ACK当前变化、64/65未结束/真实expiry/重绑定不退/known revoke+finish返额均已验；unknown手动置位仅退休predicate，非新issuer原生故障或新pipe物理结束。独立复核发现的两oracle掩盖已用新真实scope和未用ID/hash排除，初format/build失败均保留；不改旧测试/生产权限。T007整体仍未勾，后续接专用受保护bootstrap/HostTLS/SPKI/nonce/原peer及实际ACK结束，完整新交付未知/独立Host容量/重启仍待。只读[399交接](<../../.data/logs/003-validation/client-issuer-399-handoff-current-results.json>)核新源/集合/报告/Doc3及原398/397链，不把原R15 source或本组件TEST算完整产品。

### 4.7 T007原受保护管道的独立bootstrap kind（implemented / R17 server局部，Host装配未验）

Owner A串行，FR001–003/005/018/020–021/024及SC009/010/011交付局部，原T007/V005依T006/R14和R15/R16。只在同一ControlPipe原serveControl中增加hello=6分支，在原verifyPipeCaller恢复impersonation后才读独立client-bootstrap-v1请求；旧hello1/5及原ReceiveSession的43字节协议不变，不另建并行pipe owner/回退/HTTP mint。沿readControlFrame原uint32 little-endian 1..8192预分配界、protocol.ParseJSON和精确顶层/嵌套键类型、canonical nonce/UUID/digest/时间/原process三元组；请求仅version/kind/nonce，回包严格§4.4.2字段、ac1 bearer，未知/重复/case/null/version或长度拒绝。新增ServeClientControl将原legacy issuer/metadata reader与私有bootstrap issuer复用同一次原connect/peer/reset，不复制或放宽pipeOperation的CancelIoEx+GetOverlappedResult join。

私有installation wrapper只接受可信调用者提供的当前binding及endpoint pin/process投影（不是管道请求字段，不等于最终Host真实证书/进程构造已装配）；当前Store/owner/真实subject/version在Issue/写reply前/ACK复核。bootstrap TTL材料至多30秒、session TTL至多300秒，原DB session_id成为sessionGeneration；nonce登记在原64槽元数据、在同mu/SQL之前拒绝同一仍计额nonce复用，不建无限nonce表，过期旧material由接收方时间/原nonce匹配拒绝。current principal/instance/authority来自原Store和binding，不接受请求自报。raw payload按原内存生命周期清零，不写普通诊断。交付原顺序6→bounded frame→2→原DB Activate→3→4，任何partial/write/ACK/current失败先Revoke原hash，只有原最后IO及DisconnectNamedPipe已返回后wrapper才finish，不能在ACK/取消请求时退pending或64槽；unknown继续原围栏。

本片只实现server kind、私有原DB接线及实际same-first-instance管道测试：严格schema/零超界预分配、真实pending→ACK→TLS可用、截断/取消/错误ACK后原DB撤销与原Serve返回、Store pending未提前退、同原pipe随后bootstrap及旧legacy1/5仍可用、旧auth/当前client normal-tag回归。新receiver/原进程PID-birth-image pin、先SPKI后bearer、真实Host TLS listener/证书生命周期属于原T007装配与T011 native消费者后续，不把本片可信endpoint测试输入或raw TEST peer当其通过；不新增后置T023前置门或新fault全排列。代码边界：A串行原security/pipe_windows.go及client_issuer.go、NEW security/client_bootstrap_windows.go和installation/client_bootstrap.go/tests；只读输入/normal-tag两TEST/原legacy selectors和root-C集合按当前source验证，显式format为唯一验证源码写mode。实现反馈仅修验收：TEST在任何请求前WaitNamedPipe等原reset后listening，不重试交付；nonce/错误ACK必须观察无reply/confirmation及精确unauthorized阶段，不能任何timeout当拒绝；metadata与独立Store/真实DB session_id/原binding/可信endpoint逐字段比，TLS TEST server在constructor前root及缓存首次Close。两同build TEST负控只令重复nonce成为真实fresh新请求/错误ACK成为真正2，使原业务真实发行/激活并击穿拒绝断言，不删除生产guard或在验证脚本改源码；正常测试再核全部first-end/复用。其需求/权限/产品接口不变，受影响8FR局部只读复核后当前重验。当前full暴露旧issuer测试以1ms TTL要求真实SQLite发行先完成而偶发返回expired/unauthorized；仅将两个过期case的建立窗口统一为2秒，再等原真实expiry测试同一未结束/已结束退休条件，不伪造时钟/放宽expiry或生产许可，原失败及此前通过均保留为各输入记录。

R17已按[当前400完整层](<../../.data/logs/003-validation/client-bootstrap-400-full-known-expiry-pipe-results.json>)与[只读交接](<../../.data/logs/003-validation/client-bootstrap-400-handoff-verified-results.json>)实测本server-kind局部：674输入17步骤15zero+两有效同build正向控制，normal/tag十client父各三、strict frame父各三、旧11auth/四SQL父两模式及security/HTTP/Host/vets全过。nonce真正新发行读4字节/正确首ACK读1字节击穿拒绝断言；正常精确阶段/原DB/current元数据、pending不提前退、实际取消/原native join及同pipe旧模式复用已验。首readiness/typed EOF/旧1ms测试建立窗口失败保留并定点修正，四oracle机制经过独立只读复核，未代签运行。仅可信合成endpoint的server测试，不是新receiver实际pin/SPKI或真实Host TLS listener装配；T007未勾，不能用当前TEST镜像或手工unknown位补签产品。后续原Host必须同业务handler/容量owner、保留legacy及两listener真实关闭/IO join，工作区/安装owner不变；无跨仓或新的业务权限。

### 4.8 T007真实Host双listener与同业务owner装配（implemented / R18实际Run及当前normal-tag验证）

Owner A串行，FR001–003/005/018/020–021/024和SC009/010/011局部，原T007/V005，依T006/R14及R15–R17。原Config四字段与旧HTTP/pipe行为保持；只显式可选`clientListen`启用127.0.0.1:canonical-port（允许0由系统实际分配），省略则不创建TLS/binding。不能从请求/文件自报证书pin或进程三元组；原Run只打开已初始化Store、不init/repair。新endpoint owner在任何listener/native image/file获取前完整预根，原当前self-process/GetProcessTimes/QueryFullProcessImageName和冻结非继承只读image句柄计算actual birth/imageSHA；不拿TEST birth=1代实际。新单启动随机自签名证书内存持有、TLS1.3 only、禁tickets/resumption/HTTP2/0RTT，SPKI SHA256取证书公钥，不发普通HTTP setter/秘密诊断。当前protected bootstrap返回真实Addr/instance/SPKI/PID-birth-image，接收方完整生产pin仍T011，TEST可原核这些实际事实并严格pin先bearer。

原HTTP Handler只New一次，原Store/WorkspaceOpener/creator/drain/安装及区请求claim不另建。增加只接受可信启动authority的facade：旧ServeHTTP固定旧authority，TLS facade固定该listener实际authority，两个原AuthorizeHTTP调用都用相同被选值，绝不由r.Host选择；实际r.TLS仍net/http从concrete tls.Conn取得。旧ConfigureIngress仅旧明文listener，不套TLS外/内层解析密文或伪造r.TLS。当前首版M1文本客户端的TLS不启用raw ingress认证，M2二进制upload按原rawObjectHeaderViolation对缺真实metadata fail-closed，不新增解密observer为本T007门；M2仍可走原独立Host路径。Host TLS/旧HTTP共享原business容量，不开放新的runnable-control HTTP入口。

原operatorServer唯一loop在启用TLS时沿原ServeClientControl复用同pipe/legacy1/5/hello6/原SID-peer/reset；不另开pipe接收者或把identity当operator权限。Run两Serve退出均进入原FreezeNewWork/BeginDrain/Workspace shutdown，原CheckShutdownReady/HTTP Pause/control.pause/recheck/resume未知调查循环不变；不得在draining_unknown失效唯一client调查路线。已知所有业务与issuance排空后Shutdown两server并实际join两Serve，停止原control+Close原pipe，精确binding失效与image/listener第一次结束全部known后原Store.Close；unknown首end保留完整slot/owner，不重试或把cancel/绑定失效当IO释放。启动失败结束所有已取得的双listener/image（首次结果缓存），未知则原retain而非返回/降级；只有明确省略配置才旧模式，不在TLS失败后回退明文新会话。

验证最小差集：真实normal Host启用可选TLS，TEST rawpeer原pipe验证完整bootstrap实际PID/birth/image/SPKI/当前DBsession，ACK前TLS拒、激活后TLS原业务200，明文/错authority/旧实例和旧SPKI拒，旧legacy pipe/HTTP仍通；新TLS自己先握手/精确SPKI才发bearer，不把syntheticendpoint或HTTP TLS mock当实际装配。原同handler可信authority和pool单位+normal/tag Host/HTTP/installation/security/两vet，原legacy相关独立case按实际影响选择，不重复journal/Edge长测量。普通关闭两Serve/control/原Store实际结束后normal重启新证书/实例使旧ac1失效，closed合成源/正式事实保留。只改本片shared A源/NEW endpoint与专用TEST/三状态doc，旧004/005 proposed草稿仅保留完整frozenSHA，不扩其实施范围；按新全Git/source-pathsets/exe/配置/命令记录，不复用R17源列表冒当前。未运行前不签T007整体，T008/009/010/生产receiver及UI均不提前代签，无下载/ACL策略/跨仓/提交push发布。

R18已按[401当前完整层](<../../.data/logs/003-validation/client-host-401-full-current-host-results.json>)与[只读交接](<../../.data/logs/003-validation/client-host-401-handoff-current-results.json>)完成原装配范围：678输入、13步骤全0，normal/tag七Host父各三、原十client父两模式、trusted-facade共享ordinary quota三轮及security/HTTP/Host两模式和两vet均实际通过；root/C内容、Git路径集合及两个TEST无漂移。真实Run两次、原pipe/实际self PID-birth-image和证书SPKI、pin先bearer、明文/错authority/旧instance拒绝、正常两Serve/control/原Store首次结束/旧legacy兼容均执行，不是R17合成endpoint复用。首焦点仅新unit已存在安装目录误用原Initialize规则失败，改本fixture的不存在子路径后重验，不改生产规则。原R15–R18组合使T007工程范围可勾，T008/009/010及T011最终receiver、后置独立CLI全兼容、浏览器/UI仍未验；TEST-only Close未知与本进程真实Run不扩大为硬断电、原生Close故障或完整独立产品。

### 4.9 T008当前client身份与有界可见集合首片（implemented / R19；公开route与open仍待）

Owner A串行，原T008/V005，FR001/006/008/018–020/024/026八项及SC002/003/010/011的Host metadata局部，依T007/R18。先实现原Store/DB中的只读owner方法，不新建权限/连接/目录扫描或自动open；用途snapshot/实际open及五路由的唯一OpenAPI/T010生成接线仍按原任务后续，首片不勾T008。临时内部投影视图只实现§4.4字段，不成为第二业务schema或生成物来源。

- 当前context来自原currentSession的真实session_id（不是token hash）与grantVersion，原安装/tenant/principal/live binding；能力仅投影当前原身份的installation.operator，全workspace能力仍按目标原业务检查，支持首客户端仅m1-text，不猜已有注册目录profile。原SQL零grant（包括owner显式无workspace grant）合法返回空，不复制TEST prototype的owner OR bypass。集合仅当前principal同tenant、registered且存在workspace.read/open的真实grant，UUID keyset/原registrationVersion/limit默认20/max100，SQL LIMIT limit+1并关闭rows/结束只读事务；原trusted roots仅作额外范围拒绝，client metadata最多64个可信配置根，SQL先按精确大小写/目录边界过滤（无根即空），原canonical allowedLocation/root containment/FileID16/volume/registrationVersion门在内部复核，不能用只取ID的SQL绕过既有location gate。私有root facts仅内部传递，返回中无rootLocator/allowedLocation/文件profile；cursor不会夹带被配置根拒绝的目标ID。
- 所有读取在原Store.mu下先当前client-binding/身份级Authorization、readiness/workspaceStopping及原DB当前授权，再SQL同只读快照，结果返回前重验；交付方必须调用同一CheckClientMetadata再交付，不能把旧view作权限。旧legacy Authorization即使同主体也不能调用这些client-only方法。真实撤销/version/expiry/重启/同Store替换旧binding拒绝，accepted业务原规则不变。
- 分页令牌为同live binding随机32byte密钥、HMAC-SHA256域`ABOARD-CLIENT-CURSOR-V1\n`的有限自含canonicalJSON/base64url token，不创建cursor表。绑定installation/principal/sessionGeneration/instance/grantVersion/lastWorkspaceId/limit；实际session hash只作为HMAC关联输入，不在可解码payload中外发。最多1024 bytes，canonical encoding/固定字段/无重复/extra拒；签名或跨session/主体/安装/instance/limit拒，认证先行拒绝仍403；同有效当前会话的已签陈旧version单列STALE_HANDLE，不把认证失败改409。没有无界签名缓存或目录探测。
- 新源拟为`internal/storage/sqlite/installation_client_metadata.go`、`internal/installation/client_metadata.go`及专用TEST，原client_transport.go只加当前binding随机metadata key；不改现有DBschema/业务DTO/HTTP路由/权限规则。以真实DB+真实TLS AuthorizeHTTP产生句柄测owner/A-only/真实zero-grant、分页/tamper/cross-session/limit、返回前授权变化/expiry/cutoff/unknown/binding替换、没有物理工作区仍可发现且没有owner/open side effects；新方法和当前auth/SQL/HTTP/Host normal/tag/vet按影响验证，记录当前全源集合和TEST镜像。专用TEST handler不是生产API完成，不假签T010/公开契约或独立客户端。定点复核反馈：路径扫描须覆盖JSON解码后的key/value及cursor payload，不能用未转义路径搜索raw JSON；MAC破坏须仍为canonical32byte编码以真正走签名校验。补原真实有效cursor→200击穿403以及仅观察副本注转义私有路径→原scanner拒绝两同build控制，不改产品响应/权限/密码学。无下载/ACL策略/跨仓/提交push发布。

R19已按[402实际完整层](<../../.data/logs/003-validation/client-metadata-402-full-current-metadata-results.json>)及[只读交接](<../../.data/logs/003-validation/client-metadata-402-handoff-current-results.json>)完成681输入/17步骤15zero+两有效控制，normal/tag四新父各三、原11auth/两模式十client及四SQL授权父、HTTP/Host两模式和两vet通过。真实session_id/无owner绕权/空grant、两页和被移动物理目录不影响SQL发现/原DB bytes不变、完整cursor范围/current DB/cutoff/expiry/同Store替换和解码隐私均核；原scanner路径转义及MAC语法假绿两缺口只在新TEST修正后重验，静态复核不代运行。source/pathsets/两TEST无漂移；生产公开路由/原CAS用途open/T010生成和T008整项仍未签，sharedunknown单位不是新COMMIT故障。

### 4.10 T008用途绑定open backend补片（implemented / R20；公开契约接线未验）

Owner A，原T008/V005，FR001/006–008/018–020/024/026九项及SC002/003/010/011局部，依T007/R18与R19。只在原Authenticator/WorkspaceOpener增加用途snapshot与原CAS委托，不先增公开HTTP或第二业务schema，T010后再生成/接线。30秒自含HMAC `ABOARD-CLIENT-OPEN-V1\n`（当前binding key+原hash关联）绑定原DB installation/tenant/principal/session_id/instance/grantVersion、workspace、purpose=open、原registrationVersion和expiry，<=1024 canonical JSON/base64url；当前实际workspace.open DBgrant/root门先于返回，不探物理目录。快照是基线非权限，不缓存授权/恢复旧binding。

- 执行先原Store.mu下重验身份级client当前binding/readiness/cutoff/currentDB，独立验证签名/全范围/用途/expiry和原registration CAS，再构造原当前workspace.open Authorization；跨scope/签名拒403，过期/用途或基线陈旧409，原认证先行失败仍403，不把新baseline写回请求。上述已无效/过期快照材料的前置拒绝在owner lookup/Open前完成，无新公开epoch/源写；后续真实IO中的当前DB变化/到期仍由原open每次授权及最终发布前继续复核，沿原resources首次结束与错误规则，不把IO后的late拒绝冒称从未进入原owner。
- `WorkspaceOpener.OpenWorkspace`对client Authorization明确拒绝，防003借旧HTTP open绕过snapshot；legacy43原逻辑不变。新OpenClientWorkspace只调用同一原private core，不另lease/数据库/排空owner。原factory取得descriptor并核identity后，在任何recovery/可写数据库之前要求m1 format2，完整SQL history后的schema1再核；M2/未知明确unavailable，原resources按真实首次结束清理且不发布epoch，不把内部随机nonce生成当已取得公开epoch。不在可见列表预探profile，不做M2→M1降级。
- 新专用真实TLS身份测试原M1create/open/drain、原key/源字节/DB FileID、未open时snapshot源零写，合法注册但物理不可用仍可取前置；MAC必须canonical翻位、跨session/workspace/instance无scope复用、实际过期/注册变化/当前grant/cutoff/binding、旧open拒、M2 descriptor拒/原lease已结束、当前legacy M1/M2仍可open。过期本地短时签名输入仅测expiry检查，不称真实等待生产30秒；当前operator能合法正控及真实zero/A-only反控。normal/tag专用父三轮、原metadata/client/auth/open/SQL/HTTP/Host及两vet按影响执行；同build合法材料代坏MAC得到真实成功击穿拒绝oracle、观察私有路径泄漏原scanner，记录完整输入/链而不称公开API或T008整项完成。只新client_open.go/_test及原workspace_open.go共享core/最小到期check，不改schema/契约/旧helper；no下载/ACL策略/跨仓/commitpushrelease。

R20已按[403当前非空历史完整层](<../../.data/logs/003-validation/client-open-403-full-current-nonempty-purpose-results.json>)和[只读交接](<../../.data/logs/003-validation/client-open-403-handoff-current-results.json>)以683输入完成21步骤19zero+两有效控制，normal/tag三新父各三、原metadata四父/client十父/SQL四父/原真实open五父两模式、旧11auth及HTTP/Host两模式/两vet均实过，root/C内容/路径集合/两TEST无漂移。先原legacy owner真实preview/SAVE非空文本并正常排空，再client用途open保持完整原receipt/正文/revision、全源字节/DB FileID，旧public open不能旁路；M2拒绝无epoch/源写且实际lease已结束，原legacy M1/M2仍通。不拿最初仅空M1的已过焦点/完整层代最终非空证据；过期快照为TEST当前key生成的短时输入，非实际等待生产30秒。内部方法已验，原T008仍待唯一公开route/契约与后置消费者，不提前勾任务。

### 4.11 T009对应自身会话结束backend与维护截止守恒（implemented / R21内部owner已验，公开接线未验）

Owner A，原T009/V005，FR001–003/005/018/020–021/024八项及SC009/010/011局部，依T007/R18和已有R15–R17生命周期。只原Authenticator/Store/DB增加EndClientSession(ctx,currentIdentity,sessionGeneration)；输入不含目标token/principal，必须当前同Store/live client binding、空workspace/action的真实身份，原DB实际session_id与请求相等后，仅原SQL RevokeInstallationSession(actorHash,actualVersion,actorHash)。即使operator也不能借该入口结束另一个session；legacy/scoped/过期/替换/跨Store拒。成功只在原事务known commit后返回当前sessionGeneration/ended=true，原current view随即失效；取消/版本改变/共享unknown/不可确认不返回ended，不重试或猜撤销，不迁schema/另建会话库。

- 与metadata/新Issue不同，self-end是原身份的cleanup，在workspaceStopping后仍可结束自身；仍必须原readyLocked/当前binding/DB授权，不绕shared persistence unknown或已实际closed。撤销成功仅标原client slot revoked，prune必须原deliveryEnded；不得减少pendingDeliveries、调用issuer.finish/pipe.Close、Invalidate整binding、停止Host/其他client或取消accepted工作。没有原slot也只能结束原DB自身，不能根据请求generation造slot。外层公开HTTP后须特殊处理原成功终态，而非用已撤身份再次授权后把确认伪造失败；迟到实例/页面交付仍各自当前代次检查。
- 新专用actual TLS测试原同主体两会话+owner第三会话，错误generation/legacy/scoped/过期/当前grant/替换/原fence拒而正控仍可用；一个known self-end后原Authorization/新请求拒、其他会话200、原slot仅自身符合known-end才返。真实原bootstrap管道ACK2后尚未ACK4时，从实际TLS得到身份，原BeginShutdownWorkspaces截止新Issue/ACK/metadata；self-end仍known成功，但原pending=1/Store.Close拒/slot不退，只有原ACK4+Serve真实返回和原client first Close后才pending0/slot退休。该测试不把无工作区的maintenance原join或手置unknown当新COMMIT/accepted-native证明，原业务未更改，影响回归保原生命周期。
- 复核反馈：maintenance截止本身会使metadata返回403，不能将该403自证SQL已撤销。新TEST必须在cutoff后先用原Authorization.Check正控仍有效，self-end后同check失效、原同一DB的CurrentSessionAuthorization明确ErrAuthorization，而同主体独立session仍check成功，排除metadata cutoff/globalfence的假绿，再核pending/first-end；不改production或另连接。
- 同build有效控制仅将TEST请求的错误generation替换为原合法自身generation，使实际成功击穿拒绝；另一控制在响应观察副本注私有路径击穿原scanner，不删production门。原response/body/server/native/serve-loop槽在fallible前root，首次结束缓存不重试unknown。当前所有Git源/路径集合、normal/tag TEST镜像及原client/open/metadata/SQL/auth/securityHTTPHost两模式/vets按影响验证，format为唯一显式源码写mode。无生产公开route/schema/原shared helper修改、下载/ACL策略/跨仓/提交发布，T009公开接线和端到端仍未签。

[R21/404当前完整层](<../../.data/logs/003-validation/client-session-end-404-full-current-own-end-results.json>)已以685输入、23步骤21zero+两有效控制完成原内部首片；normal/tag三个新父各三、原metadata/用途open/client/SQL授权/真实open及HTTP/Host受影响两模式、旧11auth和两vet实过，源/路径集合/两TEST无漂移。原TLS同主体/owner会话隔离、known SQL自身结束、cutoff后原Check及同DB读与独立peer正控、ACK4前pending/slot保留且Store.Close拒，原IO/Serve实际返回后退休均已验；原手置unknown仅predicate，不冒称accepted-native/公开API。静态发现的metadata截止遮蔽SQL oracle已定点补强，补强误猜私有DB.Conn编译失败保留后修原只读API，不改production。当前结果/精确R20两新增增量见[只读交接](<../../.data/logs/003-validation/client-session-end-404-handoff-current-results.json>)及[research R21](<research.md>)；T009公开接线未完成，整项仍未勾。

原任务依赖顺序说明：T010依T008/T009，不把其后置“生成”反作这两项HTTP接线的前置。A在原T008/T009公开阶段先对唯一contracts/openapi.json做增量并生成必要Host Go绑定/接线；T010随后完成Go/TS双消费者生成校验、固定摘要和漂移收口。当前backend私有projection不是第二schema，旧spec/tasks依赖图不改、不提前勾T008/T009/T010。

### 4.12 T008/T009唯一公开Host增量与已知终态交付（implemented / R22当前公开接线已验，双消费者及产品验收仍后续）

Owner A串行，原T008/T009、V005，FR001–003/005–008/018–021/024/026共13项，SC002/003/009–011局部；依已验T007/R18与原backend R19–R21，不反依赖后置T010。只在唯一contracts/openapi.json新增§4.4.1五条路由/精确Client DTO及请求，沿原UUID/PositiveRevision/Digest/Capability/WorkspaceView/ErrorView和认证先行/no-store/correlation/原错误集合；原29条结构/字节不变。必要Host Go由原generator显式生成，TS消费者与最终漂移仍T010，不另schema/手写路由表。原verify_openapi保原16+13精确集合及所有负突变，再加五client精确集合/字段/有界页/only-ac1用途与额外负突变，不能以放宽原freeze通过。

- 五route均原Handler.MatchRoute/身份先行/同唯一ordinary claim，体内workspaceId不能预body读取、不能第二池；backend取当前实际grant/CAS，原claimcontext贯穿workspace owner，不在request声称当前区。只当前ac1/live TLS binding身份可用五路由，legacy使用原API不变。metadata/前置回复转换为原新生成DTO，交付前原CheckClientMetadata再次核；用途open后按实际workspaceId的原当前capability/已派生workspace.open检查投影原WorkspaceView，不因输入body缺path参数生成空scope。
- 新错误只映射现有INVALID_INPUT400、STALE_HANDLE409、CAPABILITY_UNAVAILABLE501及原FORBIDDEN/共享unknown/容量，JSON strict/body/cursor/duplicate query由唯一生成校验器；不把metadata材料当权限或自动刷新原CAS。自含snapshot/cursor拒source零写；M2无epoch/无源写/lease不泄漏，legacyM1/M2仍可用。
- self-end列原调查/cleanup而非新mutation，原maintenance截止后可准入同ordinary claim（满ordinary仍429，不新control逃生）。known EndClientSession后不得调用已撤身份Check把终态必然改403；仅此成功分支以原生成终态DTO和专用当前delivery检查：同不可伪造原identity/同Store/live binding、原ready/sharedfence/当前主体版本及tenant/owner复核，但不把已知撤销session当仍有效session。其他success仍原当前DB授权交付；未知/取消/错误generation不走此分支。原finalizer继续拥有body首次Close/真实Flush/claim返还，不finish原pipe/停止其他client/取消accepted。
- 分层证据由原R19–R21 backend/current授权与本片真实Run TLS1.3+原受保护bootstrap/真实SQL/grants、同一Handler旧明文/新可信facade共同承接；不把backend变更/维护组合冒称新Host父全部重演：五路由合法全闭环及两页/emptyzero/当前version，M1先原非空SAVE/原receipt再client snapshot/open和正常close，原M2 unavailable/legacy兼容；错误MAC/expiry/旧CAS/跨session/改query/duplicateunknownJSON/legacy身份/明文/authority/Origin/Cookie拒且原source不变。self-end必须HTTP200 endedtrue，后原token403而独立peer仍200，maintenance后仍known终态；未知仅沿原已验predicate，真实COMMIT组合留原V005后续。rawresponse/body/TLSserver/原pipe进程任务均fallible前root/首次结束缓存，所有秘密扫描decoded body/key/cursor/header/trailer，全Host最终日志。
- 同build控制仅把损坏MAC的snapshot换为原合法snapshot使真实open成功击穿拒绝，及响应观察副本注私有路径击穿原scanner；不改产品guard/造错误。当前所有非忽略Git输入+runner私有离线C、schema/generator/生成物SHA、Python --check/独立旧新bytes/当前protocol normal-tag、专用HTTP父各三、原auth/client/open/Host受影响normal-tag/vets；显式generate及format是独立源码写步骤，验证不隐改源码。R22/405完整层已按当前688输入、21步骤19zero+两有效expected1完成；原29path/schema由原R21不可变C逐项深相等复核，唯五新path。新验收曾误猜旧close必须200及wire闭态closed，失败均保留；只新test改为唯一原POST接受200/202、原key调查到真实committed/drained/noReceipt，以及原registeredClosed枚举，旧产品/HTTP状态和helper不放宽。保留所有失败/skip/deferred，CGO0 race skipped；不跨仓/下载/ACL策略/提交发布。不以本片代T011receiver/真实UI/CLI/完整产品。

### 4.13 T010唯一源TS绑定与严格JSON双消费者首片（implemented / R23当时raw局部，当前字节收口见R24）

Owner A串行，原T010/V005，FR009/011/020/024及SC004/012的绑定技术局部，依T008/T009已验R22。不新业务schema或HTTP传输owner：原generate_protocol.py在同一OpenAPI解释模型显式新增bindings/host.ts，保原Go生成字节不变，--check对Go/TS都查漂移；TS类型、schema与operation metadata来自唯一OpenAPI，通用严格运行器不是另一份DTO权威。

- TS运行器先在原始UTF8/JSON上拒duplicate decoded key、坏UTF8/BOM/NUL/孤surrogate、trailing和depth128，数字保原lexeme再用有界BigInt实施原int64数学整数语义，不经float舍入。scalar/UTF8双界、ref siblings/conditional/x-unique/x-decimal-order全沿原Go解释子集；format仍annotation，无默认注入/trim/NFC/CRLF变换。公开Title中FEFF合法，不误套字节preimage更严格规则；JS结尾锚与Go保持同义。默认request1MiB/response64MiB按原operation body角色派生。
- 从同模型产生34操作的body/parameter/response metadata有限API，无fetch/retry/bearer/权限。no-body拒任何字节、binary不假JSON；query重复/未知与原Go同样拒。同一手定expected原始hex正反corpus由Go/TS各独立消费，覆盖现有数值/Unicode/组合关系与新增client DTO/参数，解码正文和revision保持完整。有效控制只向消费副本加入一个本来合法值击穿拒绝，或篡改单一生成物使--check真失败，不修改批准OpenAPI/旧Go验证语义。
- 使用已安装Node24.21.0原生erasable TypeScript实际执行，无下载依赖；若无tsc则明确未做完整静态type-check，不把Node运行当tsc。R23/406已按当前695输入实际完成16项检查（15zero+一个有效lossy-UTF8 expected1）：同一113条raw corpus由TS及Go normal/tag各三轮独立消费，原五Python/Go bytes、protocol/digest/HTTP normal-tag及两vet通过；Go生成物与原R22字节精确不变。原runner在所有测试结束后误把baseline文件名全局改成不存在的406-public而失败，该原JSON保持false；单独只读finalize按正确R22原报告完成原输入/root-C集合、工具/日志、Go字节与Doc3交接核验，不重跑或把原失败改绿。原TS未知字段prototype属性绕过已真实复现后修，参数UTF8测试替换原字节已由实际ff反例修；数字参数末换行静态疑点未复现，不计漏洞。R23当时独立TS字节摘要与完整兼容仍后续、本片未勾T010；当前R24已补绑定工程范围，消费者产品接线仍后置任务，不能倒签。所有格式/生成是独立源码写步骤，验证只读并冻结当前输入/源集合/工具/契约/生成物/hash和R22影响差异；只ignored日志，不改依赖/SQL/批准规格/宪章/002locator，不跨仓/下载/策略/提交发布。

### 4.14 T010独立TS字节摘要与兼容截面收口（implemented / R24绑定工程已验，非产品验收）

Owner A串行，原T010/V005，FR009/011/020/024及SC004/012绑定局部，依T008/T009与R23现证据。唯一OpenAPI和旧Go生成物/字节实现/冻结fixtures不改。新增独立`bindings/host_digest.ts`从既有001/002 data-model字节布局及Go行为实现domain+NUL、精确BE uint63、显示序UUID、原样UTF8长度、optional presence、稳定UUID排序/精确名称唯一、固定profile及各kind字段顺序；独立WebCrypto SHA256，不调用Go/Python产生expected。内部preimage更严格S leading FEFF不扩大公共Title校验；private identity的整数原词法与公共数学整数分列，所有大整数不用Number归一化。

- 两份旧vectors原rawhex/明确expected_stream_hex/expected_sha256由TS独立消费；M1 rejected只Schema行走同源raw公共校验、Kind行走内部布局，raw object不当JSON。UUID显示序由正向explicit bytes证明，不把一个非法v4 UUID负例叫GUID端序实验。额外TS局部边界核M2 locator32767/32768（旧Python未核上界），不用放宽原Go或改冻结fixtures。共享113条raw corpus/原Python/Go双bytes、当前normal/tag协议/HTTP与vet按影响重验。
- 唯一原generator增加兼容manifest：原schemaSHA、原Go字节SHA、生成TS类型/runtime/声明、独立TS摘要源SHA和layout-version/协议版本；显式生成后--check零写核全部截面，旧Go token相等保完整原字节，旧OpenAPI不变。薄number-lexeme访问器只在raw Numeric实例返回原词法，不能将普通`{raw:...}`对象当number，不授权业务。
- 有效控制只对测试观察中的实际preimage翻转一byte/错domain，必须击穿冻结expected；单生成TS副本改一byte须漂移拒，不能改主schema/旧expected换绿。使用既有Node24.21.0原生TS及WebCrypto，未装tsc就明确未静态类型检查、不下载依赖。raw schema与独立摘要完成后据实际兼容/生成证据收口原T010，不把这套纯绑定当Host/原生receiver/浏览器或完整产品通过。测试和诊断仅合成/ignored日志，freeze当前allGit/source-pathsets/Node/Go/所有输入和R23差异，生成/format独立，验证不得隐式改源码；不改依赖/SQL/Host权限/批准spec/宪章/002locator，不跨仓/下载/策略/提交发布。[R24/407](<research.md>)已以699当前输入完成20项实际检查（17zero+三个有效expected1）：TS独立消费两旧vectors的15种布局45正/83反、完整预镜像和WebCrypto摘要；113raw双消费者、原Python/Go bytes、normal/tag协议/digest/HTTP及两vet通过。私有生成副本baseline --check成功后只改单生成TS，实际--check exit1；source及OpenAPI/旧Go字节不变。兼容manifest绑定真实schema/generator/runtime/三生成物/独立digest源和版本；M2 locator32767/32768、不能伪装Numeric及私有/公共FEFF分界局部正反已验。静态decimal末换行疑点在installed Node实测严格拒，原预期red失败保留、独立结果记未复现，未为假设改生产。原R23末尾路径失败和postcheck保持原记录；当前407独立full实际Succeededtrue、root/C源和路径集合无漂移，precheck核准后据R23/R24组合勾T010，仍未运行tsc静态检查或生产receiver/桥/UI/完整产品。

### 4.15 T011受保护bootstrap接收与原活进程/exe/无bearer TLS核验（implemented / R25生产接收首片，正式HTTP/入口仍未验）

Owner A，原T011/V005，FR001–003/005/018/020–021/024及SC009–011可信连接局部，依T010/R24和原T007/R18。当前首片只生产私有接收器及真实Run合成验收；cmd启动/发现、完整每请求HTTP会话客户端/代次池、浏览器与独立CLI仍原T011后续，不提前勾。调用者提供独立可信安装UUID和原PID/birth/exe摘要，不从reply自证，当前不init/repair/自动start/restart/stop Host。

- NEW security receiver复用原localPipeName/currentSID/private DACL/verifyPipeServer和同readExact/writeExact/frame/取消join。原verifyPipeServer可选retained-owner hook在原OpenProcess/Token后立刻登记完整槽，旧legacy调用行为不变；新owner缓存token/process/pipe/image每一次firstClose，失败partial同owner返回，不第二次OpenProcess按reply PID重找，不重试凭据/nonce/ACK或未知结束。全局最多64未结束receiverroot，在任何nativeAcquire前登记，unknown保依赖且不驱逐。
- 对原kernel server返回的唯一process handle读取真实birth/image路径，native shareREAD image handle拒新写/删除、sole os.File transfer、bounded64MiB SHA、原process仍live；比独立期望及strictreply.Process完全一致，匹配expectedInstallation/nonce/窗口。原frame8192在分配前拒，未知kind/version/duplicatekey不fallback；原legacy1/5完全保留。
- ACK2前，对strictreply loopback authority发起**无HTTP/bearer**实际TLS1.3握手：SPKI/证书时间/IP127.0.0.1/serverAuth/数字签名/no resume，禁代理/H2/连接复用；raw/TLS同first-end槽并knownClose后重核原process与30秒窗口，再ACK2/读3/ACK4。只有完整握手才允许返回native私有材料；失败不输出token/私有路径，不自动重试或撤别的session，原server负责未delivered撤销。每次真正业务新连接的同样live/pin检查在后续HTTP客户端实现，不能把本次probe当永久权限。
- NEW host TEST只复用原真实Run/pipe/currentidentity和原selfpin，独立legacy context提供安装、原实际process/exe指纹；成功生产receiver后用已存在TEST TLS消费者确认原session-generation/原公开client-context，错误expected安装/birth/image先拒、原legacy兼容、取消/坏frame及首end单位定点核。TEST不代cmd/生产HTTP/独立进程替换/浏览器或V005完整产品。normal/tag对应packages/原管道授权及Host影响回归和vet；全部当前Git输入/私有C/镜像/源路径集合及R24变化单列，不旧source冒当前。验证脚本不隐式改源码、仅explicitformat，原spec/OpenAPI/生成物/DB权限/schema/依赖不变，不下载/ACL策略/跨仓/提交发布。[R25/408](<research.md>)已以702当前输入完成14步骤全0：normal/tag真实Run生产receiver父各3次、独立安装/birth/image三个反例每次均实际拒、同pin TLS正控/错SPKI实际握手拒且HTTP计数0、fake-firstend predicate各3、原10client父两mode及security/HTTP/Host两mode和两vet通过。原pipe返回的同一个process handle持续pin，exe原file handle有界hash，strictframe/nonce/current窗口和实际无bearer TLS先于ACK；ACK2/读3/ACK4+原pipe首Closeknown后才Material。正常case随后用TEST HTTP消费者核公开client-context，不冒称新生产HTTP会话。新30秒接收上界仍由原native取消/实际join，不用timeout替代first-end。Run退出不代表同TEST进程死亡，Alive仅OS live predicate；Close单位假unknown不称nativeClose故障。cmd启动/发现、正式每请求live/pin和独立CLI/browser/完整产品仍按原T011后续，T011未勾。

### 4.16 T011已核原实例上的一次性JSON HTTP原语（implemented / R26当前HTTP局部；入口与完整产品未验）

Owner A，原T011/V005，FR001–003/005/009/011/018/020–021/024及SC009–011可信消费局部，依T010/R24及T011接收首片R25。本片只原`ClientBootstrapConnection`上的生产一次性HTTP/1.1 JSON请求及真实Run验收；cmd发现/显式serve、浏览器/桥/独立CLI仍后续，不勾整T011。不新增HTTP业务路由/DTO/授权/DB/schema或第二会话owner。

- Request只收原生成metadata可识别的method+相对RequestURI及有界原JSON bytes，不收URL/任意header/Origin/Cookie/bearer。path/query原严格校验，binary/no-body沿原ValidateOperationBody拒，不开放raw上传；response原OperationResponse/ValidateJSON，64MiB、header16KiB、request1MiB，不归一化正文。响应须原JSON媒体、no-store、correlation、无redirect/1xx/压缩/升级，不按状态码认定业务成功。当前唯一Host JSON总有显式Content-Length：首片只支持单一canonical CL及Connection: close，在16KiB增长前保留原头并拒重复CL/TE/折行/trailer，不让ReadResponse合并抹去证据；完整体后须真正peer EOF。将来chunked不在此片支持，不新schema/fallback。初synthetic peer误用唯一契约不存在的UNAUTHORIZED导致schema拒绝，修测试为原FORBIDDEN，不放宽生产校验；真实Run首focus已通过。结束未知必须把complete降unconfirmed并清status/body，纯单位与实际网络分别记录。
- 每次独立TCP→原活process仍pin→TLS1.3/实际证书SPKI/时间/IP/serverAuth/no-resume→再live核才一次写HTTP和原bearer。复用R25同一个TLS验证函数而非复制第二信任模型；不用http.Transport/GetBody/自动redirect/代理/H2/连接或TLS恢复。零重试，POST半写/取消/截断/非法schema均unconfirmed，不带完整Host结果；HTTP写前失败notSent。complete只取完整原类型/全部frame和known网络first-end，不等于保存提交。
- 请求槽在任何Dial/response/取消回调前全局root，最多64未结束，unknown保槽/不淘汰。原连接mu.TryLock拒绝并发等待队列，串行持原process/image到实际网络结束；这不是桥8action或Host容量替代。每response/rawTLS用唯一首次end，取消AfterFunc仅打断原socket并实际join，socket/Body关闭缓存，unknown fence原会话且不释放依赖。最多30秒本次IO界，timeout不是网络/原Host事务已结束证据。
- 实际Run保护pipe生产接收→生产HTTP client-context/visible/self-end peer仍有效、旧session拒；真实TLS错误pin时server HTTP/bearer计数0、redirect不跟随/POST不重发、坏header/body/framing/schema及首end谓词与当前normal/tag影响回归。测试只制造synthetic peer且区分实际网络/构造unknown，不把组件当独立CLI/browser或accepted-native组合。原SQL/权限/唯一OpenAPI及GoTS字节不变，privateofflineC/allGit/pathsets/镜像/原R25差异记录；仅explicitformat写源码，验证不隐改源，不下载/ACL策略/跨仓/提交发布。

[R26/409](<research.md>)已完成705冻结输入下14个实际步骤全exit0：normal/tag真实Run父各3、三个HTTP边界/结束谓词单位各3、原十client父各mode一次、security/HTTP/Host两mode及两vet。真实每次新TCP/错SPKI零HTTP、302不跟随、POST收后截断一次不重发、合法大于1MiB且小于64MiB原RecordList字节完整通过；pure结束失败降unconfirmed不称native故障。synthetic peer失败路径cleanup可能unknown且原root保留，其通过不证明失败IO已排空；真实Host正控明确要求Close成功和Run join。原full末尾因并行治理文档两修改/一新增导致指纹检查false，原JSON保留；单独只读交接逐源/集合/镜像重核，精确三路径/哈希和新授权继续003分工已读，不无依据重跑14项、不放宽任意源码漂移。首两个synthetic测试分别错误ErrorCode与不在旧契约的openEpoch query，仅修新测试；旧失败保留。T011仍待入口/显式serve与完整独立产品，不提前勾。

### 4.17 T011显式可信原生配置与一次连接检查入口（implemented / R27实际Run与normal exe构建；独立exe使用/显式serve未验）

当前证据见[R27](<research.md#L258>)及[712输入实际完整层](<../../.data/logs/003-validation/client-entry-410-full-current-native-entry-results.json>)：16步骤全0、normal/tag原生检查父/三个错误预期子例及native/cmd单位各3、原security/HTTP/Host两mode/两vet，实际normal exe及profile构建，root/C字节与路径集合、镜像无漂移。仅六NEW源及三个003状态文档，不改原生产或权限/契约/依赖；构建exe不等独立产品运行，T011整体仍partial。

Owner A，原T011/V005，FR001–003/005/011/018–021/024共10项、SC009–011可信连接局部，依T010/R24和R25/R26。只新增本仓`internal/clientnative/`的A独占连接/配置文件及`cmd/aboard-client/`入口，不与B浏览器或C后续业务CLI文件重叠。当前命令仅`aboard-client inspect-connection --config <trusted-native.json>`：用户显式提供独立可信期望，不扫描目录/发现文件，不init/repair/start/restart/stop Host，不接受URL/任意业务方法/正文或bearer参数。显式serve另按原T011实现；本片不勾整项，不把命令检查称完整CLI文本消费或浏览器产品。

- 原生配置最大8192字节，strict UTF-8/duplicate/unknown-field拒绝，精确`{version:1,pipe:string,installationId:UUID,process:{pid:uint32>0,birth:PositiveRevision,imageDigest:Digest}}`；pipe只本地名称、每字段有界，不含authority/pin/token/安装路径或自动初始化许可。配置是用户选择的可信原生输入，不从不可信bootstrap reply派生；读取完整regular文件及首次Close成功后才准入native连接，不能以解析成功掩盖读/结束错误。原生配置文件路径允许argv，但不记录其值，不输出定位信息。
- 新连接owner在任何配置Open/Receive/Dial前完整登记，最多64未结束；保存partial配置句柄和原receiver，即便工厂error也保留。读取/firstClose/receiver.Close首次结果缓存，unknown不retry/不淘汰、不宣称Host原业务结束；原receiver和RequestJSON是唯一可信连接/HTTP原语，不暴露原bearer或重新实现TLS。连接只持typed public context/session-generation及private原owner，不第二DB/权限/业务结果库。
- 原生入口只有一个实际流程：严格配置→原production receiver独立安装/process核验和完整ACK→原production RequestJSON GET client-context，检查实际安装/同bootstrap主体/实例/session（不是shape自证）→显式一次POST client-session-end，必须完整200/原generated typed ended:true+同generation→原receiver首Close已知后才输出有界公开检查结果。不能将metadata200当保存证据，错误/取消/截断/未确认不输出Host结果、不自动重发或停止Host。正常退出是本命令明确结束自己的会话，不把浏览器关闭等同退出。原Host/其他session应继续可用。
- 测试只用合成已初始化真实Host Run及原peer/pin，正常检查含当前身份/原Host与独立session仍可用、错误独立安装/birth/image/config字段/超界/不存在或坏文件拒且不init/修复/重放；公开stdout及固定错误逐值/decoded扫描，raw配置/response/原receiver/Run与CLI文件所有首次end和实际join完整root。单元包含unknown first-end predicate和独立有效配置正控，不把fake Close当native故障。normal/tag当前两个包/原receiver-HTTP/Host/两vet定点，构建真实normal`aboard-client.exe`（不是TEST镜像）并核版本/源/依赖profile；命令execute测试与真实exe构建分列，不冒独立进程CLI产品验收。
- 所有Git输入/精确runner/原R26报告及治理三路径指纹/私有C/镜像/源路径集合实际记录，验证无源码写入（只有explicit format模式），旧失败不覆盖。不下载/改ACL或系统策略/跨仓/提交push发布；唯一OpenAPI/GoTS/旧SQL/Host生产不变。T011显式serve、完整启动发现/独立exe命令运行、T012+浏览器/桥/UI/T017正式CLI仍按原任务后续，未验不倒签。

### 4.18 T011独立normal Host与原生检查exe验收（implemented / R28独立使用已验，T011仍partial）

Owner A/C，原T011/V005，FR001–003/005/011/018/020–021/024共10项、SC009–011可信连接局部，依T010/R24及R25–R27。仅新独立acceptance与ignored runner，不改产品/契约/权限/依赖/旧helper。原R27构建与同TEST进程Run不代本片：同一冻结输入分别构建真正normal `aboard-host.exe`及`aboard-client.exe`，不带acceptance tag；独立Host用原已初始化合成fixture，显式启用clientListen，原legacy protected pipe确认安装/身份，由已pin原Host进程得到实际PID/birth/exe SHA作为独立配置期望，不从新bootstrap reply反推。

- 唯一命令仍`inspect-connection --config`，原生子进程独立于Host和parent。父完整slot在stdout/stderr文件Open/Start/原process Pin前登记；仅独立O_EXCL私有输出文件，不exec隐式copy pipe。保原Cmd/partial output/process handle，pin原PID/birth/image，不按PID重新寻找；新增child直接把OpenProcess返回写入预根槽再查询元数据，查询失败也保留该partial handle由唯一first-end关闭，不复用会内部丢失未知Close结果的旧helper；原Wait实际完成后输出文件各唯一首次Close已知，再有界读取并扫描原始/decoded key/value所有合成秘密。新JSONL观察器遇疑似JSON行解码失败必须拒，不能静默跳过重复键中的转义秘密；先原测试真实red再局部修复，纯观察unit与独立进程三轮分别记录，不宣告生产泄漏。失败cleanup只终止本次已启动child并Wait，不把该收集当成功、Host排空或退款；未知首次end缓存不重试/保root。
- 先发行一个独立peer原生产receiver并实际GET currentcontext，另命令一次获取本身新session、输出只有原public Inspection+真实匹配安装/tenant/principal/instance且generation不同，HostSessionEnded=true必须来自原known确认。命令actualexit0/Wait与所有输出first-end已知，peer再新GET200证明不是Hoststop/撤别人。错误独立安装/birth/image三个配置均actual非零、stdout空、固定错误不泄私有定位，peer仍可发新请求。普通legacy原context仍200；最后known结束peer、原Host正常interrupt/真实stopped与Wait。后原normal Host重开、旧process配置拒、新原配置成功并完整正常结束，覆盖真正独立进程替换，不把同TEST进程stop当processdead。
- 所有Host/client启动/配置仅TEST驱动合成输入，不是新产品自动启动、init或repair许可；本片没有`serve`命令/业务CLI/浏览器/UI/全产品通过。显式serve仍原T011下一片，不提前勾。原host/acceptance夹具及production完全保留，不用TEST issuer/DB直写/替代验证。每一原子测试/3重复、actualnormal二exe与Go buildprofile、各原PID/birth/image/一次等待/源文件指纹、全部Git输入和私有C路径集合/复用R27影响源界实际记录；验证不写源码，显式format仅NEW测试，不下载/改策略/跨仓/提交发布。

### 4.19 T011用户显式前台serve既有安装（implemented / R29当前独立使用完成原T011工程范围）

Owner A/C，原T011/V005，FR001–003/005/011/018–021/024共10项、SC009–011连接/显式启动局部，依T010/R24及R25–R28。唯一新增入口`aboard-client serve --config <trusted-host.json>`是用户直接执行的**前台Host角色**，复用原`host.Run`及原保护pipe、TLS、单Store/handler/排空；不另启动后台进程、自动发现、init/repair/restart/stop命令，不因后续检查客户端退出而取消Host。对该serve进程本身的明确interrupt按原Host规则正常排空，与客户端退出/自身session-end严格不同。无隐式默认serve，旧inspect-connection行为不变。

- 读取原Host四/五字段可信配置，8192上限、regular/完整读取、全槽在Open前、首次Close已知才交原唯一纯`host.DecodeConfig`。只抽取原解析体以共用，不复制配置schema；原`host.LoadConfig`及旧init/serve行为保持。新入口不引入clientnative→host包依赖（同包Host测试已消费clientnative），生产组合只在cmd。配置的installationRoot必须已经初始化，原`Run`只Open而不Initialize；失败不fallback。
- 公开stdout仅原Host Event、固定安全错误；任何私有路径/token/raw错误不输出。原Run可能unknown保留时不伪返回“stopped”，不新增强制close/kill语义或第二DB。新配置文件owner仅负责自己首次读/结束，不持安装锁；未知首次结束保根且不retry，不开始Run。
- 显式format与零写验证分离。新独立acceptance以同输入normal Host仅初始化合成安装，随后真正normal client exe的serve分支提供Host服务；再独立inspect exe取得当前身份/结束自身，原Host和另一peer仍可用。用户对serve原进程明确interrupt才实际stopped+Wait，再新进程重开。原未初始化/损坏（仅合成副本）/活实例占用拒绝不补种或重启、全部源bytes守恒；失败仅原child收集不算排空。两个真实normal镜像/profile、进程PID/birth/image及所有file/process/response first-end与join、秘密扫描/新命令单位/原Host与client影响normal/tag及vet记录当前证据，不能由构建或组件调用代签独立使用。
- 技术反馈：独立正控命令在任何Acquire/凭据前可有界等待同一原pipe可连接（单次Win32 `WaitNamedPipe`，不Sleep、不重发失败命令、凭据或HTTP）。前一拒绝连接的原reset完成与新的命令开始区分；初始重开正控仅返回固定不可用，未观测具体native原因，不以猜测改生产接收器或放宽信任。所有失败JSON/C保留，重新验证当前source。
- 不重做R28已验连接矩阵、不增加物理ENOSPC/断电/三态门；T012浏览器、T013桥、T017业务CLI及T018外仓授权仍原后续。[R29](<research.md#L275>)以716冻结输入/14actual步骤全部0、两个真正normal产品exe完成三轮独立显式serve/inspect/原peer继续/源守恒/原interrupt正常停止和重开，当前normal/tagcmd单位、Host影响及vet均过；只读precheck核原R28及当前全输入/集合/镜像/源增量后，据R25–R29签原T011。原T012–T032产品/浏览器/桥/journal/业务CLI与发布范围仍未验，不冒称完整M6产品。

### 4.20 T012受管Edge原进程与预分配前有界CDP首片（implemented / R30当前生产首片已验，精确文档与cookie尚后续）

Owner B（本会话串行集成），依T010/R24与T011/R29；FR003–005/018–021/024共8项，SC009–011浏览器技术局部，原V002。仅新`internal/clientnative/browser_process_windows.go`、`browser_rpc_windows.go`及各自新TEST，原九个prototype/Host/公开schema/权限/依赖不改。当前用户已有Edge156.0.4314.42及R11真实私有pipe证据可复用为可行性，不作为新生产pass；不下载浏览器、不附接任意既有实例、不加CDP TCP/普通HTTP cookie fallback。首片只启动/真实身份与transport探测，T012文档/一次join/串行cookie和T013桥仍原任务后续，不提前勾T012。

- 可信原生调用提供绝对Edge exe和全新专用profile路径；拒reparse/已存在profile，image非继承只读shareREAD pin+64MiB有界全hash保留至真实结束，不能照抄原`os.ReadFile`/PID重开杀进程。使用原Win32 `CreateProcess`/`StartupInfoEx`显式HANDLE_LIST取得**原创建ProcessInformation**（process/thread先登记），原handle取birth/image，原handle Wait/Terminate而非按PID重开。四pipe槽在CreatePipe前完整root；仅child读/写与单NUL标准句柄显式继承，父端清inherit；真实HANDLE数值通过已验`--remote-debugging-io-pipes`传递（不是Unix fd3/4）。`os.NewFile`转移仅一次，raw槽转交后不双Close。profile/temp仅专用本地原生cache，环境只必要OS变量/专用TEMP，不把父凭据环境传browser；起始仅about:blank及固定headless/无插件/背景联网禁/loopback代理参数，没有任意renderer命令入口。
- 进程root（最多64、unknown仍计额）在任何Acquire前；启动即保原process+thread+唯一Wait owner，身份查询失败亦保留创建句柄。只有`Browser.getVersion`及`SystemInfo.getProcessInfo`实际browser holder==原创建PID、原handle仍活/birth/image等独立期待后才标verified；无DevToolsActivePort、无debug-port参数。对外仅必要pid/birth/digest/version和是否known结束，不打印原profile/exe私有路径/CDP params/正文。
- 新私有`browserRPC`仅可信原生内部transport，不增加bridge通用动作。固定16 pending/16 writer/64事件；每个新调用在goroutine/编码/写入前同时占槽，无额拒新且不排无界队列。method/session有界，params须已有限rawJSON；总帧长度先算再分配（4MiB），不先任意Marshal再判界。incoming固定小buffer分片读取NUL，累计长度在每次copy/增长前检查，capacity不超4MiB；depth32在解码前扫描，原严格JSON拒duplicate/非法UTF8/孤surrogate。response/event有自己有限槽的bytes所有权，不悬挂复用reader buffer。事件满即fence并保旧事件，不能丢事件声称导航已处理；只有消费者显式取走事件才退其槽。
- response匹配原command ID/session，unknown/无匹配/重复reply/frame/read/write/timeout/cancel使当前controller截止新命令、保所有未明pending与相关根。writer只有原Write真实返回后才记录physical ended；未明不靠timeout/迟到reply返新额度，不积累writeEnded无限历史。已知完整CDP error只是本命令rejected，不伪Host/business unknown。call结果不是Host权限/receipt；文档代次授权还未实现。
- Close先原子截止外部新call；仅verified且controller known时经唯一内部通道发`Browser.close`，否则只可对原创建handle必要异常Terminate，严格分列graceful/abnormal。原Process真正Wait结束、reader及所有writer实际join后才首次Close父pipe/image/process；任一结束/Close未知保全root/计额，缓存首次结果不重试。首次结果仅说明本owner，不声称整个Edge进程树/桌面沙箱或accepted Host业务结束。
- 验证先纯frame/depth/duplicate/16/64/unknown-first-end有效正反，再真实installed Edge调用新生产process/RPC取得version/holder，独立记录同一原创建handle的PID/birth/image及actualBrowser指纹，只回收自己原child；当前六个normal/tag正常实例实际Browser.close、真实Wait和全部IO first-end已验，异常Terminate分支及任意image替换不据此签。normal/tagclientnative及vet、精确allGit/input/pathset/browser指纹/旧prototype字节复用按影响核。旧长journal/Host全回归不无依据重跑。固定环境无浏览器明确unverified/unavailable，不以SKIP签成功；真实cookie/文档/UI不由本片代签。技术反馈：首实跑已证明原PI/image及真实Edge正控，但原实现仅检查product非空，不能代当前支持矩阵；在任何Verified/对外owner返回前精确要求`Edg/156.0.4314.42`，未知product/版本fail closed，不自动下载或扩大支持。仅补该production分支与有效正反单位，原已通过报告保留；受影响FR019/当前八FR映射及未知实际结束规则不变，重新currentnormal/tag片，不以旧源码通过覆盖新源码。

当前结果见[R30](<research.md#L284>)及[完整记录](<../../.data/logs/003-validation/browser-core-413-full-supported-browser-results.json>)：720冻结输入/十步骤全部0，normal/tag八新父各3+原native三父各3/vet，六个原创建Edge实例真实graceful+Process/reader/writer join；两个TEST/installedEdge/原源与pathset无漂移。pure16容量与假unknown单位只各自层次；exact文档/一次join/串行cookie/bridge/interactive/UI/异常Terminate及整个进程树保证未验，不提前签整T012。

### 4.21 T012精确文档与一次短材料首片（implemented / R31原文档及一次材料已验，cookie与bridge尚后续）

Owner B/A串行，FR003–005/018–021/024（8项）、SC009–011文档技术局部，依T010/R24/T011/R29及R30。仅新`browser_documents_windows.go`及新TEST、原process文件增加唯一文档owner与Close前截止；不改原prototype/Host/业务schema/权限/依赖，不重复R30长容量或journal。只支持已验Edge/headless，trusted native固定canonical loopback HTTP origin与`/app/`发布路径，无任意renderer调用桥。

- 一个原browserProcess最多一个文档controller，原owner先登记再创建browserContext/target；最多16 page及32活/未明join材料，材料30秒，client/document generation原生随机UUID、join随机256bit，不含Host bearer/安装私有定位。所有目标/session/上下文元数据和部分创建结果有有限槽；未知root不淘汰、不重发创建。controller只管理原process，不产生第二process/权限owner。
- 精确document不可由cookie/URL/target单项建立：当前browserContext+target+flattened session、根frame+loader、default main world的`uniqueContextId`、exact origin+完整URL及client/document generation共同校验。根frame/loader/tree与Target.getTargetInfo真实调用；原process持有唯一controller及一个有界事件pump（不保原始历史，截止后实际join）；持续消费原RPC事件到每页当前态，不从空状态重放被takeEvents清走的事件。context destroyed/cleared、根frame导航、target/session分离使旧document与未用材料立即退休，已知一页变化不无故退出另一页。子frame不能领根文档材料，只有默认主world；跨端口/不同URL即使共享cookie也不合格。不建历史无限context表。首轮实际焦点已走过reload/跨端口与B独立交付，但旧测试对已跨端口失效A再查close属于遮蔽；仅新测试先恢复A并证明current正控，真实close接受后等原target/session退休事件，不把命令接受当物理结束，不再为“拒绝”已知退休身份向消失session发命令。生产current/snapshot先同步原pump并对已知旧代次/退休作本地必拒，正例仍须原Target/FrameTree实际复核；不放宽原RPC错session/timeout的unknown围栏。原失败日志不足定位具体8秒等待根因，不宣称已证实CDP错误实现。
- 只有可信native方法在前后相同精确snapshot的`Runtime.evaluate(uniqueContextId)`中交付固定形状join材料；不用数字contextId执行、不提供通用JS/URL代理。每document至多一次Issue，30秒内Consume必须同时匹配token/原client+document代次且再次查精确snapshot；开始调查前占attempt，实际未知不靠超时/重新mint返额，已知Consume后replay拒。仅此桥加入资格，不是Host授权。旧页面刷新/重登记须新精确document及新材料，旧材料不恢复。Close先截止所有page/material，再原Browser.close/真实Process与全部IO join；不因浏览器退出停止Host或倒签accepted。
- 真实Edge测试：专用合成loopback静态服务器（非T013产品bridge/React），两个独立page、一个iframe，材料仅主uniqueContext收到；wrong token/代次/跨page拒，成功一次后重放拒；刷新/同target新loader与unique、跨端口导航、关闭target、全controller cutoff拒旧材料，同时另一合法page仍可用。本两页case不代容量/TTL边界或最大并发负载验证，仍留原T012后续；当前normal/tag各三、原R30 process/RPC影响及vet、全Git源/pathsets/images/实际Edge指纹。cookie设置/未知cookieACK、生产bridgeHTTP、真实UI、interactive和完整T012仍后续，不提前勾任务。脚本显式format唯一新Go写，不隐改源码。

当前[R31](<research.md#L294>)已以722冻结输入/十actual步骤全部0完成normal/tag九浏览器父各3、原native三父各3及两vet。原process/pump单owner、精确root前后snapshot/一次Consume、两page与iframe/reload/cross-port/已知close退休/B持续、cutoff和实际Process/IO join已验；两个TEST和installedEdge/所有root-C内容及路径集合无漂移。只合成静态HTTP及root-world own属性注入，不代renderer同源内存隔离、cookie/CSRF/产品bridge/UI/16或32槽最大load/真实30秒等待/nativeClose故障；当前T012仍partial，旧R30“exact文档待验”保对应历史。

### 4.22 T012串行client-envelope及实际未确认回包首片（implemented / R32原cookie及迟到原ACK已验，T012仍partial）

Owner B/A串行，FR003–005/018–021/024及SC009–011浏览器技术局部，依T010/T011/R30/R31。只新browser_cookie及TEST、原document必要状态/Issue守卫和原process私有可观察transport工厂（原RPC仅in/out参数改为io.Writer/io.Reader接口，解析、发送及结束算法不改，raw File仍是原process sole owner）；原九prototype/Host/公开契约/权限/依赖不改，不重复journal长测量。

- 原document controller持唯一cookie状态，clientGeneration对应随机256bit `aboard_client_envelope`，不含Host凭据/定位。复用同op TryLock串行，原mu中在任何CDP调用前记录attempt；Storage.setCookies只root-session+原browserContextId、url=可信origin+`/`、无domain字段、Path=/、HttpOnly=true/SameSite=Strict/Secure=false。完成后Storage.getCookies在同原context完整核唯一name/value、domain精确127.0.0.1、path/HttpOnly/SameSite/Secure及session标记；仅完整当前scope回读才known。任何未确认/错回读保原attempt/值/代次并拒新，不因晚回包重新写或恢复；known再次调用也不发送第二set。无普通HTTP Set-Cookie或cookie授权fallback。
- 只有该原cookie已known且同clientGeneration才Issue/Consume短材料；原页面刷新只退对应document，不整体旋转cookie/退出B。先document current/原unique snapshot再交付，cookie匹配只关联当前controller，不能独立授页资格。cutoff同时拒cookie和新短材料，原process/pump/RPC仍唯一物理owner；真正unknown不自动退额。
- 为实际未知证据，只在原startBrowserProcess的私有内部transport工厂增加默认nil直通参数，原公开配置/argv/权限不增加。TEST工厂只包同原writer/reader，原raw File仍归process；先记录原Storage.setCookies命令ID，原Edge真实同ID响应在原reader首次读到后有界暂缓，绝不改/造字节。deadline使原RPC未知后才释放同原完整响应，原callback实际join；晚成功不得恢复cookie或发第二set。已未知Browser.close未确认时只原创建handle必要异常stop+actualProcess/reader/writerjoin，明确不是graceful或Host排空。不得另reader/重开PID/取消native原语或用手置布尔冒此证据。
- 正控为真实Edge正常set/readback、JS document.cookie不可见、同context A/B且B保持、跨端口请求实际携cookie但短材料/原精确document拒、普通静态HTTP零SetCookie。原R31test需显式known cookie后才能Issue；真实未确认包单独case/正常与异常join分列。pure属性正反及wrong值/cutoff局部、normal/tag各3、原process/RPC/doc/native影响及vet，完整Git输入/路径集合/两个TEST/installedEdge/原R31报告链逐一核；format与验证分开。产品bridge/CSRF/React/业务请求和完整T012及物理故障不由本片代签。

当前[R32/415](<research.md#L303>)已以725冻结输入、十actual步骤全部0验证本片：normal/tag十五个浏览器父各3、原native三个父各3和两vet；原Edge156.0.4314.42的两页/完整cookie属性/HttpOnly和网络携带/cross-port不授页、一次资格与原R31文档均实际走。未知case原writer实际发送后，暂缓同ID真实成功`result:{}`，deadline后释放原完整字节，原reader实际结束，pending仍保留且arrived=false；cookie仍unknown/set-count=1，没有恢复或第二次写。该case原handle必要异常stop后Process/IO实际join，graceful=false，不能当Host排空或未知退款；正常casegraceful=true分列。完整root/C源与Git路径集合、两个TEST及installedEdge无漂移，precheck725/Errors[]后记录；未重跑R12/R13长测量，不改原RPC未知接收策略。产品bridge/CSRF/业务/UI、16/32最大负载/30秒等待或nativeClose故障仍未由本片签，T012不勾。

### 4.23 T012有限准入、真实短材料过期及原生新代次收口（implemented / R33原T012工程已验，非后置产品）

Owner B/A串行，FR003–005/018–021/024及SC009–011浏览器技术局部，依T010/T011及R30–R32。只新增`browser_admission_windows_test.go`，原production/九prototype/Host/契约/依赖不变，不另造page owner或把后置T013/024产品作为前置。

- 原已验Edge/专用profile、当前cookie和真实document上创建16页，原第17页准入拒且Target.getTargets的同context目标集合不变；闭页后的保守页槽不强制回收，不新增优化门。真实Issue后按原ticket单调deadline实际等待30秒，不改时钟/TTL；计时起点在Issue调用之前，并核原deadline处于调用前后各加30秒的界内，不能以Issue返回后的剩余等待误拒合法慢交付。新原进程身份用原PID+birth联合，不把数值PID必不同当OS保证；过期Consume拒、同document不重新mint，而独立B仍能真实Issue/Consume。当前page/pump/reader和原进程已知结束后，全新原nativeprocess/profile/controller代次必须与旧不同；旧doc/material/cookie关联拒，新的真实材料可一次消费。
- 32材料槽边界单列合成计额检查：仅TEST在同mu下放31个标明synthetic的attempted metadata槽，真实未Issue的C正控通过原snapshot/evaluate填第32；另一当前且未issued的D必须拒且不标issued/不新增槽，C原合法票仍可Consume。不是已实际产生32个未知CDP交付，不能以此签未知退款或最大并发；不改原production guard/retire算法。
- 新case normal/tag各一次真实30秒等待，旧R30–R32十五父各三及native三个父各三、两vet按影响当前运行。每一个browser/HTTPstatic/partialowner先root，actualfirstend/join后才结束；只合成静态页面，不代真实React/bridge/Host业务/renderer隔离/ENOSPC/nativeClose故障。当前全Git输入/路径集合/两个TEST/installedEdge、R32报告与handoff按SHA核，format仅显式NEWtest，验证零写。
- 据R30–R32及本片结果只复核原T012工程范围，成功才勾原任务；T013有限桥/CSRF、T014journal、T015/016业务交接、T024完整组合仍依原图，未运行不签，不新增发布前置或笛卡尔实验。

当前[R33/416](<research.md#L313>)已以726冻结输入完成十二actual步骤全部0：normal/tag新case各一次原30秒（30011/30011ms），旧十五浏览器父及三个native父各三/两vet。实际16页及第17无目标创建、A真实到期不remint/B仍有效、known原Process和全部IO结束后新nativeprofile/generation拒旧且新票可一次消费均走原生产；32计额层仅synthetic31+真实32/33，不签32未知交付/最大并发/退款。原first计时从Issue返回才开始会误拒慢交付的TEST问题经只读定位，只修新TEST为原调用前计时及deadline区间、原PID+birth联合，旧pass保持历史，当前input另完整重验。原production/Host/契约/依赖均不变，root/C源和Git集合/两个TEST/currentEdge无漂移；只读precheck726/Errors[]后以R30–R33复核签原T012工程。T013有限桥/CSRF、T014–16业务/持久线索、T018跨仓权与UI、T024同一产品完整组合未由本片代签，原R30–R32的“当时T012未勾”是历史边界。

### 4.24 T013有限HTTP Handler、可信静态输入与本地page/退出门首片（implemented / R34首片已验；当时实际装配未签，当前收口见R35）

Owner B/A串行，FR003–005/018–020/022/024八项、SC009–011桥技术局部，原T013/V006，依已验T012/R30–R33。不新业务OpenAPI/HTTP代理/Host权限，不先要求T018外仓或React。仅NEW `internal/clientbridge/`静态输入/Handler/security及其TEST；首片是可装配Handler而不是已装配的产品listener、浏览器启动或完整T013。

- `New(authority, Package, Hooks)`仅可信native构造，authority来自实际loopback监听地址而非请求Host。Package必须给有限来源/许可声明及每个固定`/app/`asset的SHA256/媒体类型/原bytes；构造验证并复制，最多128项、单项2MiB/总8MiB。来源tag和声明只记录，不等于权利人发布放行。GET/HEAD只可信assets；安全的`/app/`深链接才fallback index，未知asset/`/bridge/`/Host API/认证错误绝不SPA。CSP只self，无unsafe-eval/inline、任意代理或SetCookie/CORS。真实React及文件发布装配T018/T022/T031仍未验。
- 原plan4.4.2的join4096/action1MiB/header16KiB增长前界、exact authority/Origin及三个Fetch Metadata、精确Content-Type/Cookie全部先于消费；decoded重复/未知字段/别名/坏代次拒。最多16页/8个未结束API请求，body原Close失败保请求槽并fence，不由超时淘汰。普通满额包括exit可429，不伪造保留公共逃生通道；已获准exit执行下条原子截止。
- Hooks是native私有接缝而非产品授权：核原known cookie值、消费原精确document一次材料并返回当前ClientGeneration/DocumentGeneration及该原doc的current检查；校验cookie/current、源材料、代次后才发行独立随机pageId/CSRF。join未被完整确认不以cookie恢复/补票。每action查page、CSRF、client/document代次和原current；这些只浏览器关联，不能代Host授权。生产callback装配及actualEdge到此Handler留下一片，不用合成native hook称实际Host身份。
- 当前只接`context`和`exitClient`的有限输入/输出：context只回原生成ClientContextView并核原SessionGeneration，前后原current/本地cutoff检查；callback失败保守unconfirmed且无Host状态/结果。exit在同local mutex先截止/清全部page与CSRF、原native未用材料Cutoff，再至多一次原对应End确认；断连不回滚local截止，known原generation+ended才confirmed，否则unconfirmed，独立Host/peer不受影响。绝不等待页面或以结束browser取消Hostaccepted业务。其余十四冻结动作明确unavailable、不进入业务dispatch，strict业务输入/原结果交接按T015/T016再接，不签完整动作适配。
- 新TEST用真实TCP HTTP/有效page和独立CSRF正控、scope/current晚拒/双页退出与延迟context不交付、真实cross-origin/authority/Fetch/Cookie/body/未知动作负例及计数证明拒绝不进入callback；纯known包SHA/篡改、深链接与API区分、响应零SetCookie/无秘密、8槽保留和bodyfirstend分列。有效观察负控分别用真实合法CSRF替坏值触实际callback、响应观察副本私有路径触原decoded扫描，不改产品guard/旧expected。只synthetic已追溯静态包/可信hook，不称React、实际cookie生产装配或nativeClose故障。
- 实施反馈（原八FR范围不变）：实际pre-repair Handler两个负例已命中——合法cookie通过后读body期间撤销仍可join/End，以及Body首次Close已返回unknown但错误Write阻塞时新请求仍准入。严格修为body known结束后、join消费/发page及exit不可逆截止前再核原cookie/current；未知Close返回即在local mutex同步fence并保原q，再做任何响应Write，oversize路径也先处理原first-end。受控body/阻塞Writer证明这些机制，不称nativeClose故障；旧actual-TCP初绿和红日志保留，修后当前输入重验。来源声明另保私有source/license及绑定content fingerprint的provenance hash，不把hash称法律许可。
- [R34/417](<research.md#L323>)已以730冻结输入完成当前十actual步骤（八0与合法CSRF/私有观察两有效1），normal/tag十三父各三、原protocol/HTTP影响两mode和两vet全部通过；原body两机制pre-repair red先实际失败再修复、当前受控body/阻塞Writer各三次通过。显式format仅newGo，验证零写/offline/privateC；原R33全部源/路径集合/报告SHA只允许本new四源和Doc3，root/C输入与集合无漂移，precheck730/Errors[]才记录。R34本片包go test不代产品exe或实际native装配，当时原T013仍unchecked；当前R35已按原任务实际装配收口，T014journal并行依赖仍T010，T015/T016/T018/完整V006不代签。

### 4.25 T013原native文档/Host连接与实际listener装配（implemented / R35据R34与当前实际装配完成原T013工程范围，非后置产品）

Owner B/A串行，FR003–005/018–020/022/024八项、SC009–011桥技术局部，原T013/V006，依T012/R30–R33和R34。只新增clientnative内BridgeSession组合及TEST，原Handler/Host/契约/SQL/依赖不变；原Connection由可信调用者独立持有，本组合不init/start/repair/stop Host。可信Package沿R34复制/hash，actuallistener取127.0.0.1:0的真实authority；有限http.Server配置头16KiB/读头及读写时限，原Handler同一owner，不造第二bridge权限表。

- `StartBridge(ctx, connection, BridgeConfig, Package)`在任何listener/browser工厂前登记最多64完整partialowner，必须先原Connection.Context真实当前身份；不转出bearer/安装locator。原browserProcess/Image/私有pipe、唯一browserDocuments/cookie仍原owner；Cookie/ConsumeJoin/Current接原native函数，Context/End只原Connection，sessionGeneration取真实context。其他十四动作仍R34 unavailable，不冒T015/016业务已接。
- `OpenPage(ctx, /app/path)`只原newPage/issueJoin，将真实一次材料交原uniqueContext。每个最多16页slot在工厂前root；HTTP join可能早于原issueJoin最终snapshot返回，故按原client/document代次在既有slot里用单次ready完成门有界等待，不新增票/重试材料、不独立reader/权限cache。ready只有原issueJoin known成功和完整material相等才可消费，失败/cutoff立即拒；pending保留至原owner结束。
- Handler exit先本地截止并原documents.cutoff，然后一次原Connection.End；不关闭Host/peer、不取消accepted。BridgeSession.Close只本组合截止、实际Server.Shutdown/Serve return、cached原listener firstClose、原Browser.Close与process/pump/reader/writer join；任何未知保完整root/不重试/不退依赖，不把Shutdown请求/逻辑session-end当物理结束。外部Connection不由本组合Close，调用者原槽保持。
- 新实际TEST启动原Host.Run合成安装，原受保护pipe+生产Connection、真实Edge/privateCDP和原Handler；合成可追溯外部script静态包让实际renderer发join/context/exit，普通HTTP零SetCookie、坏CSRF/跨端口cookie不授权/双页退旧及peer仍200分层，所有原Run/browser/listener/HTTP/body/原生handles先root并实际首次结束。不是React/外仓/UI或独立产品CLI；固有16/8/帧/TTL旧边界按源不变R30–R34精确复用，不重复长实验。显式format/验证分离，当前所有Git+exactrunner/privateC、actualEdge与normal/tagTEST指纹、normal/tag新case及旧bridge/native影响和vet，原R34输入/集合精确增量。实施前不提前勾任务；当前复核原T013定义后，按R34与R35的实际证据签原工程范围，不把后置T015/T016业务或T018实际前端反设为本任务门。
- [R35/418](<research.md#L332>)当前733冻结输入、2026-10-26 03:01:52–03:04:09 +08，十二actual步骤全部exit0；normal/tag新组合亲本和四个本地owner/ready单位、原十五浏览器亲本各三，原native三个亲本各三、旧有限Handler十三亲本各三及两vet全部通过。真实生产Connection借用/原protectedbootstrap、actualEdge两个精确document/known cookie、同一个Handler/实际loopback与Host当前context和对应ownEnd已贯通，独立peer仍200且原Host未停；原Server/Serve/listener及原Browser process/reader/writers、caller-owned Connection均known首次结束。当前只有三个NEW native组合源，原Handler/Host/contract/SQL/依赖字节不变，source/root-C集合/两个TEST/currentEdge无漂移，precheck733/Errors[]才记录。只合成外部script静态包、非React/外仓/业务十四动作/独立产品CLI或原生Close故障，完整V006联合门仍留原后续。

### 4.26 T014生产纯codec、非授权关联与独立编码向量首片（implemented / R36纯字节首片已验，原T014持久与生命周期仍partial）

Owner B/A串行，FR003/008/011/013–015/020–021/024九项，SC007/010/012编码局部，原T014/V007，依已验T010/R24；不将T013/React/Workbench作为前置。本片只新增clientjournal纯codec及新tests/fixtures独立向量/检查器，旧五TEST/Host/SQL/契约/依赖不改，不发行U/Q或发送权，不提前勾整T014。

- 复用原生成`protocol.CommitRecord`保存operationId/openEpoch/candidateId/candidateDigest/previewVersion，不手写同名业务DTO。Binding仅补installationId/tenantId/principalId/workspaceId/hostInstanceId/sessionGeneration/previewRequestId/baseRevision/baseDigest?。这些字段只非授权的原调查关联；来源验证、唯一原source latch/落盘/当前身份及恢复查询仍由后续原T014/T016负责。hostInstanceId是原64hex实例摘要，sessionGeneration是原DB UUID。
- `Record{Generation,Sequence,Binding}`编码为§4.4.3已冻结的十六必选ASCII key/string字段和可选baseDigest；`journalFormat:"1"`、journalGeneration、sequence显式包含。所有UUID/digest/Revision按原生成标量规则，最小CommitRecord按原生成结构校验，不擅自归一化或增加Host业务约束。canonical payload无空白、ASCII key字节排序；Decode严格ParseJSON、exactkeys/string类型、constant-time MAC及重编码逐字节相同；拒duplicate/unknown/转义别名/非canonicaldecimal/null/大小写/长度及tail，单条metadata<=4096、sequence正且<=int64max。
- 纯接口EncodeMetadata/DecodeMetadata、ConfirmationHandle/MatchHandle、DecodeHandleReference、SourceDigest；MAC key为原journal代次的32byte值参数，不进renderer/argv/log。handle固定UUID16BE+N8BE+原metadata MAC32的canonical无padbase64url75字节。DecodeHandleReference只不可信定位，不是MAC证明；未来Store必须先按当前owner/generation/sequence查原行再MatchHandle，不能拿可读MAC或纯函数授予权限。唯一source digest仅冻结的七字段canonical payload加SOURCE域，不把workspace目标/operationId替代原source身份，也不从页面重建来源权。
- 新独立stdlib Python作者从冻结layout和合成值直接产固定canonical payload/带长度前缀的MAC预镜像/metadata/handle/source预镜像及SHA，禁止读取Go结果来造expected；Go逐byte对比全部布局，再标准HMAC/SHA。明确absent与present baseDigest、int64边界/UUID显示序/关键字段变化；保持key/正文/receipt不进入普通诊断。两个实际控制：只观察preimage单byte翻转应被固定向量拒、有效编码替负例应实际解码成功击穿拒绝；不改production guard。局部测试给全部反例重新算合法MAC，避免仅MAC失败遮蔽raw/canonical边界。
- 显式format只新Go，verify只读且Python-B；当前allGit+精确runner/privateC/offlinecache/normal-tag包和vet/旧prototype影响按源和记录复用或定点重验；完整预镜像、输入/工具/原产物和R35交接分别记录。该片不签真实SQL落盘、事务H/pre-COMMITrollback、full4096、source唯一发行/U→Q、Attempt/cold恢复、native1034/回调排空或T015 UIACK/DELETE，这些继续原T014后续，不新增ENOSPC/三态笛卡尔或后置UI门。
- [R36/419](<research.md#L341>)已以737冻结输入、2026-10-26 03:18:04–03:19:23 +08完成十三actual步骤（十一exit0、两实际观察控制expected1），独立Python五固定向量与Go逐字节正控、normal/tag四新亲本各三、原protocol/digest两mode和两vet通过；原公开契约/Go绑定/旧五prototype/Host/SQL/依赖字节不变，root/C全部输入及路径集合无漂移，precheck737/Errors[]后记录。本地句柄定位明确非认证，MAC也非Host权限；重新MAC的raw反例避免仅MAC遮蔽canonical检查，极小surrogate例还有结构无效条件，不能宣称它单独隔离原parser机制。旧SQLite和物理排空证据不据此冒成新生产Store，T014仍未勾。

### 4.27 T014真实持久Store、唯一关联、U→Q与正常冷开首片（implemented / R37正常闭环局部）

Owner B/A串行，FR003/008/011/013–015/020–021/024九项、SC007/010/012持久局部，原T014/V007，依T010/R24及R36 codec；不反等UI或React。本片只NEW clientjournal Store/行事实/测试子文件，原codec、五旧prototype、Host/SQL适配器/契约/依赖不改。

- Store是原native唯一owner：可信绝对本地NTFS新私有目录或既有目录，原private DACL检查和原native Acquire固定锁/祖先pin，dbPin标识后由原fixed SQLite单checked Conn维护。全局64完整Store槽在任何fallible Acquisition前登记；partial owner与error一同保留，fixed.Open返回nil但ErrPhysicalCloseUnknown仍保原lease/根，不可按nil当已释放。首次Close按DB→dbPin→lease及祖先/锁，任一结果unknown缓存、保余下依赖，不重试或删除/重建库。普通existing-open须原native caller明确已知旧owner物理结束，不能从锁取得或库可读推断；残留journal/WAL/SHM或损坏拒绝不自动恢复，正常cold只查旧线索、不生成旧Attempt。
- 新format1固定严格meta/U/Q表及跨表唯一source/sequence/combined4096约束；meta保存原generation/key/N，key仅本地私有状态。每open核精确sqlite_schema、integrity、完整有界每行codec/MAC/source键/序列与N，拒未知schema/跨表重复/损坏；原page4096/DELETE/EXTRA等与M8192每open实际readback。完整事实仅一个定长hash+N/U/Q，不存第二业务结果/receipt或墓碑列表。
- 生产发行核心接受同native生命周期不可重建的原source owner、先CAS原单向latch再任何准入/SQL；本片该私有owner只用合成TEST构造，T016负责真实Host preview/当前身份来源集成，不提供按浏览器candidate重建source的公开factory。SQL唯一source已有且整binding相同返回同原handle而不加N，改目标/key/最小输入拒；新source在原事务内N+1及INSERT U且元数据完整R36编码。事务内先P−F+64<=H6144，修改后实际used<=H，超界knownROLLBACK且完整N/U/Q事实不变；control U→Q仍M界。任何COMMIT不明或rollback不明保原pending/连接/租约，不新SQL/退款/强关。
- Consume先当前owner查原U整行并MatchHandle，原事务DELETE U/INSERT Q，只有COMMIT已知后给当前内存唯一Attempt；Attempt原Store/本次owner/精确Q+一次CAS，不做网络/Host POST，取消和重用不返权。原source latch不重置；本片无DELETE，后T015仍须原完整结果/当前页面UIACK。Lookup只返回原非授权Clue，冷开U/Q均调查、不恢复issued map或Attempt，原generation/N/完整元数据、原DB FileID持久守恒；旧Store Attempt在closed/fenced或新owner拒。
- 实施技术反馈：只读review发现导出Attempt的内联atomic.Bool可被普通Go值复制成两个一次权，以及row Scan把真实SQL错误与sql.ErrNoRows一并压成Denied；未修生产前两个实际Go用例分别命中复制后第二次TryUse成功、DROP U后Lookup错误被伪装missing。当前统一原source/Attempt使用私有共享一次状态，sql.ErrNoRows才普通拒绝，其余DB错误保原cause并fence，Consume不二次抹错；这是同FR013–015/原未知边界的最小修复，不变需求或授权。红阶段仅为执行含内联atomic值复制用例显式-vet=off，最终当前两vet必须正常过，不能用红脚本关闭vet代验收。
- 最小真实正反：Create/同目录实际竞争；多source同原binding只一N/U；同source再次或改输入拒；真实U→Q/并发Consume一胜/Attempt一次、原knownDB/pin/lease关闭后新Store只调查、完整原N/U/Q/MAC/DB FileID；损坏/额外schema/无旧结束证据拒不修复。当前normal/tag与两有效观察控制/原codec和protocol/digest影响、工具/所有输入/路径集合/原R36链。full4096近H、真实native1034/原callback未知实际排空/原事务old-new调查、T015UIACK与T016正式发送仍原后续，不能把本正常片称原T014整体通过，不加ENOSPC/未知三态笛卡尔/React前置。

- [R37/420](<research.md#L350>)已以741冻结输入完成2026-10-26 03:46:47–03:48:11 +08十三actual步骤（十一exit0、两有效expected1），normal/tag九亲本各三、原codec五固定向量/独立Python、原protocol/digest两mode和两vet通过。真实native竞争、SQL唯一source/N、并发Consume一胜、共享一次状态、未用旧Attempt在knownClose/cold后拒、完整N/U/Q/MAC/DB FileID、损坏/额外schema拒均已验；冷U控制手置当前issued后真实Consume成功击穿拒，改输入控制真实恢复原绑定击穿拒，生产guard/fixtureexpected未变。原codec/Host/SQL适配器/契约/依赖字节不变，root/C全部源和路径集合无漂移；precheck741/Errors[]后记录。两个pre-repair实际红、遗漏Attempt指针字段的中间compile失败均保留，当前两vet正常通过。这里只签本正常Store片，原T014仍unchecked；full4096/实际近H、native1034/原库未知事务调查、真实Host来源和后置UIACK/正式POST未签，不追加ENOSPC/三态笛卡尔门。

### 4.28 T014实际post-H拒绝与原事务known rollback最小片（implemented / R38原事务primitive局部）

Owner B/A串行，FR003/008/011/013–015/020–021/024九项与SC007/010/012容量局部，原T014/V007，依T010/R24及R36–R37，不反等UI。只NEW生产包TEST文件，生产Store/codec/原测试和依赖不改。

- 当前冻结合法codec每字段固定UUID/digest/revision，完整有效metadata上限960字节（含baseDigest，未带为880），不是任意可填4096字节。新Store的N1–4096其sequence最多4位，合法最大945。4096字节仍拒绝上界；旧proto填4096样本不能移作生产业务正控，也不因此降低已批准安全界。用全合法max字段与独立逐项长度和作正控，不改原向量expected。
- 原Store.transaction(ctx,true,mutate)实际production post-H原语：原非空U/facts/issued/DB FileID/gen已记录，TEST-only回调在同原事务中创建临时于该事务的main压力表并插入约25MiB zeroblob，实际PRAGMA used必须大于H6144且小于M8192、回调无SQL错误。随后必须原post guard ErrStoreFull、原tx真正ROLLBACK成功、pendingnil/notfenced、精确五schema/完整N/U/Q facts/issued/identity不变；原U仍能正常Consume及当前一次Attempt，正常Close/cold只查旧Q。压力表不是合法冻结业务schema/真正发行来源，回滚后不可残留；此片只证明同一私有事务primitive的post界与完整回滚，不称4096合法业务满额或硬件ENOSPC/COMMIT unknown。
- 同build TEST观察控制仅将压力blob减至小值，原production事务真实成功应击穿ErrStoreFull断言；不能删post guard或模拟SQLite错误。normal/tag新亲本/原九亲本与codec影响、当前工具/输入/pathsets/只读交接分层执行。full4096、真实native1034/原库未知调查、T015UIACK和T016真实Host source/POST仍原后续，不加所有故障三态笛卡尔或物理盘门。
- [R38/421](<research.md#L359>)已以742冻结输入、2026-10-26 04:03:04–04:04:34 +08完成十二actual步骤十一0及一个有效expected1；normal/tag十一亲本各三、Python五原codec向量、protocol/digest两mode和两vet通过。合法max960/880/初序号4096的945逐byte正控；同原事务回调成功后实际used6413>H6144且<M8192，原post guard真实known rollback，精确五schema/完整facts/issued/DB FileID/gen不变，原U可正常Consume/一次Attempt，known原Close后coldQ只查。小blob实际COMMIT成功击穿拒绝oracle；无生产/旧向量改变，root/C源及路径集合/两TEST无漂移，precheck742/Errors[]后只Doc3记录。压力表仅TEST在事务内临时存在，回滚后无残留；未签合法4096业务发行/pre-H64、硬件ENOSPC、未知COMMIT/恢复或后置UI/Host业务，原T014仍unchecked。

### 4.29 T014合法原发行4096条满额与正常冷调查（implemented / R42实际normal与tag各一次已验，非整T014）

Owner B/A串行，FR003/008/011/013–015/020–021/024九项与SC007/010/012容量局部，原T014/V007，依T010/R24及R36–R38；只NEW生产包TEST，不改生产事务/旧校验或安全上界。

- 原newStore按公开冻结合法最大字段（baseDigest有值、baseRevision/previewVersion为int64上界），4096个不同原PreviewRequestID分别经原source owner/CAS、SQL唯一关联与原issue完整事务；每条完整metadata为941+本次序号十进制位数、最多945字节，不用旧prototype填4096或批量SQL代发行。每512条仅记录实际elapsed和P/F，保留原完整MAC/有序facts及事务前后校验成本，不为了测试跳过或合并安全检查。
- 满额N4096/U4096/Q0时新增原source须ErrStoreFull、source已burn且完整facts/issued/FileID/gen守恒；原同source相同完整绑定仍同handle且N不增，改关联拒。正常Consume一条到Q仍合计4096，不返额/不允许新发行；保留一个尚未TryUse的原Attempt。正常DB→pin→lease首次结束后cold相同完整facts/FileID/gen，U/Q都只能调查、旧未用Attempt与冷U不能Consume；不把可读/可锁当旧owner结束。
- 同build控制只把TEST填充次数改为8，下一条原事务必须真实成功击穿“full journal did not reject a new distinct source”断言，不修改产品4096或expect。当前normal/tag新case各一次（原发行和完整校验为结构性O(N²)，不无依据各三次长跑），旧十一父各三及原codec/protocol/digest/vet按影响定点。actual约束包括每个原事务、计额不被U→Q或正常重启静默退还；不是全Q4096或最大并发/native1034/硬件ENOSPC/COMMIT unknown或UIACK/POST证明。
- 新case分别有明确90分钟Go超时与每512条进度，不以时间预测或运行中进度当通过。若实际限额/完整性被推翻，保留原库和失败并按证据修复，不把容量阈值改小或添加React/三态全排列前置。原T014仍未勾，未知事务/可信原库调查继续原后续。
- [R39/422](<research.md#L368>)实际长测未完成：当前TEST修正issued完整map守恒后启动原发行，04:26:17时原child已运行六分余、CPU819.578125秒仍无第一512进度；04:28管理取消、实际收取exit1并核原PID已结束，原false报告/私有库保留，不签4096或cold通过。独立ignored诊断clone保原code+一benchmark，实际原DecodeMetadata+SourceDigest均值777074ns/op、1152451B/op、11676allocs/op；原CPU采样regexp.compile累积38.83%，证实固定schema反复编译的可改技术成本，不把该微基准当全发行预测或容量验收。下一片先在原单源schema语义下做有限不可变compiled-pattern等价修正与正反回归，再当前重跑；不得删完整行/MAC/source/事务事实或改4096/H/SQL profile，R39当时还没有生产修正；随后R40完成下面§4.30同语义优化；R41随后仅记录normal已验、同一job顺序tag进行中的历史阶段。[R42/424](<research.md#L392>)当前已完成原normal/tag各一次，实际PASS2764.34/2782.42秒，Started2026-10-26 04:55:23.1906563+08至Ended06:27:51.9429856+08，两个原mode Exit0/Passedtrue、父Succeeded/CapacityPassed均true，745原输入/两镜像/完整路径集合不变，onlyDoc3差异。各八顺序进度及完整原事务/计额/真实DB→pin→lease首次结束/coldfacts与FileID/旧Attempt拒通过，预检与最终只读交接核全部源/产物/日志/链接。不补签allQ/最大并发/pre-H/真实unknown/原库调查/后置UI业务，T014仍未勾；原§4.31未知设计和§4.32CLI设计未实施，待其各自门和原依赖继续串行代码。原R39false和所有未完成检查点保留，不因本次通过改旧报告。

### 4.30 T014原完整校验的有限固定schema正则预编译（implemented / R40语义与成本已验，R42另补实际4096双模式）

Owner A/B串行，仍原T014/V007与§4.29九FR003/008/011/013–015/020–021/024、SC007/010/012容量技术局部，依T010/R24及R36–R39。只把原唯一生成schemaJSON中固定pattern一次编译为私有只读有限表；不缓存输入/校验结果/当前授权，不另建schema，不减少原ParseJSON、整字段、每行MAC/source/顺序事实和每次SQL事务前后检查。

- `internal/protocol/strict.go`原regexp.Compile(p)+MatchString改调同语义私有helper。表只遍历启动时原generated schemas里的pattern strings去重，条目不超过固定schema输入；invalid pattern保nil/拒绝，未知或非生成pattern原regexp.Compile后匹配且不存入表，因此内部通用测试语义和无界输入内存均不变。只读表发布后不再写，原Go正则引擎/锚点/UTF8与schema边界一字不改，public OpenAPI/旧Go和TS生成物指纹必须精确相同。
- NEW单位逐固定pattern与原独立regexp.Compile/MatchString作正反差分、Unicode/尾换行/边界/巨大固定字符串、invalid和unknown fallback不增长表、并发读；完整原113raw语义corpus、固定预镜像/bytes及normal/tag受影响protocol/journal/vet实跑。观察控制只把TEST一个拒绝结果改为true，原差分oracle必须实际失败，不通过改生产pattern或expected取绿。没有tsc/race/DB native故障或产品UI声明。
- 新privateC保持原R39诊断追加benchmark完全相同代码/向量/参数和工具，记录before/after CPU及bytes/allocs而不承诺固定加速比；性能达标不能代容量通过。语义/差分/影响回归和实际benchmark完成后，重跑§4.29原4096实际normal/tag各一次（原90m单case、完整issued map/facts/cold及有效short8控制不放宽），长测结果如实分列。
- 原R39取消/false报告/原库及无效PowerShell参数诊断均保留。若仍不能在实际窗口完成，继续以原代码profile定位最小等价成本，不删安全校验、不降低4096/H/M或改SQLite profile，不把此技术反馈加成新的业务要求/跨仓权限或React/ENOSPC/三态笛卡尔门。T014仍未勾，原nativeunknown/原库可信调查/T015/016后续不代签。
- [R40/423](<research.md#L377>)已以745当前输入完成十九actual检查十七0/两有效1，normal/tag固定八pattern与原compiler差分/unknown无增长/并发/113raw各三、原十一journal父各三/独立五向量、protocol-digest及HTTPHost两mode和全部受影响vet过。原OpenAPI/生成GoTS/Store/codec/SQL/依赖指纹不变，只strict.go调用同算法有限表及两个新Go。相同原benchmark源/向量/参数实测777074→82202ns/op、1152451→54418B/op、11676→725allocs/op（本地约9.45倍，非固定SLA），不减少任何MAC/source/事务事实。原R39未完成/false保持；R40本片当时未签容量，当前R42已另以同镜像实际完成原normal/tag4096各一次及完整cold，不由优化或微基准替代容量。

### 4.31 T014原提交未知、真实首次结束及有限原库调查（implemented / R43原T014工程已验，非后置业务或发布）

Owner B/A串行，仍原T014/V007、FR003/008/011/013–015/020–021/024九项与SC007/010/012本地持久局部，依T010/R24及R36–R40。原唯一容量任务`bash-536059db9b094c2a`现已按R42完成normal/tag各一次，原冻结代码/检查/镜像未改、没有第二容量用例。本节原designed/任务同步和九FR/SC007010012局部只读门已在原技术门内由[R43](<research.md#L400>)实施验证：747当前输入、十二actual步骤十一0/一个无故障真实成功控制expected1，normal/tag十五父各三、独立codec Python/原protocol-digest两mode/两vet通过；原Consume ordinal0三flush成功，ordinal1真实只读句柄BOOL0→driver1034/无Attempt/原ref和排他保持、原DB实际first-end后同FileID完整old调查及调查句柄/原pinlease全部结束、新cold只查UQ。六个实际故障观察均whole-old，未签未观察的next/unresolved或nativeClose故障。原R42正常容量按输入/检查/产物链复用，不重跑或由静态映射代签。T015/016和完整产品仍后续，逐条机制/边界如下。

- 原单owner：只扩展`internal/clientjournal`原Store/pending生命周期，不创建第二Store、第二lease或可由renderer调用的恢复API。目标是已存在原事务的完整`old`/`next`事实；未知初始化、没有可信完整next、原DB首次结束不明或事实不匹配均保留`unresolved`。不能按错误码、`sql.ErrTxDone`、可读/可锁、缺行或ctx取消判回滚。原source/Attempt状态不重建，结果只是本地调查线索，不是Host receipt、业务终态或重发权。
- 原COMMIT/ROLLBACK不明继续保原key、generation、DB FileID、native lease/pins、原pending/tx与完整old/next，拒普通Issue/Consume/Lookup/ACK。必要的完整next标志只能在原事务事实真正取得后设置；不能拿零值冒可信next。实际SQL callback未返回/原s.mu仍忙时不得绕锁调查或再入写者。
- 受信native调查入口先在原mu内登记一个完整、有限、不可重试的物理结束/调查槽，再释放mu等待；原DB唯一首次`fixed.Handle.Close`可由已根保留的worker执行。观察者取消只停止等待，不取消原Close、不启动第二Close、不清pending、不释放pin/lease。只有该原Close实际返回nil才承认原物理连接已结束；失败、未知或未join保完整owner/原cause与首次结果，禁止用第二次nil洗白。普通Store.Close仍不得越过pending强关。
- 仍持原lease及dbPin并核原身份，最多取得一个新的原fixed SQLite现有库句柄；它是同一Store的调查阶段，不是新权威/新业务准入。调查句柄在工厂前预根，nil+`ErrPhysicalCloseUnknown`仍保owner；不得删journal、复制覆盖、创建/补种、迁移、重放原mutation或用新key重新提交。复用原严格schema检查及原key/generation的完整有序MAC/source/N/U/Q事实与integrity，调查前后实际FileID/lease保持一致。只接受整份old或整份next；不能只看计数、单行或business key。固定SQLite正常打开可能的原引擎事务收尾按实际记录，不称只读inspect或业务重放。
- 整份匹配也不复活旧Store：调查句柄自身首次Close已知后，才按原dbPin→lease/祖先首次结束顺序退休原owner，清易失issued与Attempt关联。只有全部实际结束确认后，受信caller才可给正常新OpenStore传`PreviousEndKnown`；新owner仍cold U/Q仅调查，不恢复旧发送权。调查句柄/原pin/lease结束失败则保完整root，不返回“已安全重开”。不匹配/损坏/未知格式保unresolved和所有权，不自动退款/返槽或反复新开调查句柄。
- 最小实际证据：同原生产Consume事务从U→Q，另保不相干U/Q；使用原受控SQLite真实Win32只读句柄FlushFileBuffers失败证明BOOL0→driver1034，而不是ctx取消、Go error替代或create/SAVE故障。先证原pending/old/next、无Attempt、原排他与拒新；其后证原DB实际首次结束、同FileID原库完整匹配和正常新owner仅查原线索。ordinal0真实正常结果是负控。实际只记录观察到的old/new/unresolved，不新设所有ordinal×三态笛卡尔。取消等待/首次Close不明的单位或受控回调只能证明各自机制，不冒充native Close故障；原source/Attempt复制及SQL错误保cause的R37红保持。
- 影响范围只原Store/facts、必要NEW调查子文件/TEST；原codec/严格profile/OpenAPI/GoTS/Host/依赖不变。显式format与只读验证分离，先最小真实故障/原样正控，当前normal/tag、原Store/codec/protocol-digest回归与vet、全部输入/路径集合/镜像/原R40及最终容量报告链核对。T015完整结果/UIACK/DELETE、T016真实Host来源及POST仍依原后续；不新增React、硬件ENOSPC、系统策略、跨仓或发布许可。本节当前已按R43运行上述范围；取消观察/首次结束错误与故意fullFacts不匹配只是受控单位，不代native故障。R36–R43共同覆盖原T014工程动作，原任务可签，UIACK/DELETE、Host来源/POST和最终产品未验项仍不变。

本节逐原则静态核对：I/INV01–05保持原Store及原身份/FileID，无第二owner或从路径授予业务权；II/INV06–08普通open/inspect不自动修复，显式调查不创建补种，跨进程排他仍原native证据；III/INV09–14不将未知当回滚/重发，不发布Attempt/Host receipt，不承诺断电原子；IV/INV15–17取消观察不等实际Close，所有真实首次结束和join先于退休；V/INV18–21当前授权仍在真实Host调查/发送前重验，本地MAC/线索不授予权，64有限roots/不泄key路径，不宣称同SID沙箱；VI局部journal故障不主动停止Host或另一会话，原共享不可信边界仍保留；VII/工作流仅复用原固定SQLite与公共业务契约，后续真实原生/独立客户端验收，不用文档静态检查代运行。本段设计核对不是实现或验收通过；原正式受影响analyze已在代码前完成，R43实际结果另见research，不由静态原则映射代签。

### 4.32 T017独立文本CLI的公共链、显式确认与原key调查（implemented / R46原T017工程已验，非联合产品）

Owner C：`cmd/aboard-client/cli_windows.go`及其独立消费/验收子文件；A仅串行提供原可信`Connection`单次JSON转发接缝和现有main入口分派，不碰B的bridge/journal/浏览器。原T017，FR001/003/006–008/010–014/016–021/024共17项及SC001/006/008/012的CLI工程局部，主要V011、V009/V010/V012结果/传输/生命周期局部；依T010/R24及T011/R25–R29，不反设T014/T015/T016、React或外仓写权为本任务前置。原唯一容量任务`bash-536059db9b094c2a`已按R42完成两个原mode和全部输入核对，未重发/更改冻结生产源码、检查或镜像。本节仍是designed未实施：原T017任务已同步，17FR及SC001006008012局部的严格只读一致性门已完成；后续仅在原门与当前设计覆盖内串行实施，不以容量或设计门代CLI/React联合产品通过。

- 采用显式一次性业务命令，每次新CLI进程/自己的受保护pipe签发会话/同一原Host公共JSON链。不同CLI调用可共享Host已打开工作区的原epoch，而不能共享bearer或客户端连接。已有原`Candidate`只绑定主体与`PreviewEpoch`、当前基线及15分钟有效期，不绑定client session generation；因此同一**当前仍授权**主体的独立preview与confirm调用可有效，不能为了复用候选而共享React会话、隐式重开或更新epoch。原`OpenOwner`仍由Host按workspaceID唯一持有；CLI结束只撤自身会话/关闭自身连接，不关闭工作区或Host，不取消accepted业务。反向新epoch或陈旧基线由原Host拒绝，CLI不自动刷新、合并或重做预览。来源依据为原`internal/workspace/preview.go:20–30,128–142`、`internal/storage/sqlite/commit_gate.go:181–209,300–312`及`internal/installation/workspace_open.go:45–65`，不是客户端自己放宽权限。
- 原可信连接只新增一个A独占的受信native单次JSON转发方法：原`Connection`的TryLock/ready/closed/ended门及其唯一private原生owner，委托原`ClientBootstrapConnection.RequestJSON`，不造第二HTTP栈/连接池/凭据源。只返回原transport/status/raw生成校验结果，不加入浏览器动作、确认解释、journal或SQL。底层仍每次新pinned TLS、原live/PID/image/SPKI/authority先于bearer、一次HTTP写、完整有界读取和实际first-end；不接受renderer/外部自报authority/headers/bearer。CLI的参数只能选择下表固定命令，**不提供raw request、任意URL/method/path/headers、安装管理、M2附件或私有业务writer**。C自行解释原生成DTO/字节，不import或调用bridge动作调度、journal发送围栏或浏览器结果适配器；打包依赖存在不等于运行借用，验收须记录实际独立PID/session和公共请求。

| 固定命令 | 仅构造原公共操作；重要边界 |
|---|---|
| `context` | 原当前client-context；精确核独立可信安装/实例/主体/本会话generation，不从配置或响应自证 |
| `workspaces` | 原visible分页；limit默认20、最大100；单次调用内至多16页，cursor只在同一连接内原样续页，不接受前次调用的session-bound cursor或物理扫描 |
| `open` | 用户明确workspaceID及期望registrationVersion；同一连接获取原用途前置，必须与用户版本相等才POST原snapshot。陈旧拒绝，不把新读取的版本静默替换用户基线；其他命令不隐式open |
| `status` / `records` / `record` | 原GET工作区状态/当前文本页/原resource；记录当前workspace与epoch范围，分页同一连接有限；闭态/损坏/unknown按Host门，不自动修复或重开，不暴露排除的历史/附件/管理能力 |
| `preview-text` | stdin原生成`PreviewRecord`，显式原epoch/baseRevision/baseDigest/title/body；只POST预览，不自动确认。输出保留原`CandidateView`以及实际安装/主体/工作区与**请求的原epoch**；原CandidateView没有epoch字段，不能捏造其公共字段 |
| `confirm-text` | 与preview独立的用户显式确认命令；stdin为CLI intent包装的原`CommitRecord`、原完整`CandidateView`和原安装/主体/workspace/epoch范围。分别用唯一生成schema校验两DTO，关联candidateID/digest/previewVersion/workspace/epoch，当前安装/主体须匹配；只把CommitRecord发Host，不把包装当另一业务规范/权限。原key必须用户显式给出，不生成新key来修复失败，不重新preview、open、改基线或进行第二正式POST |
| `operation` | 原workspaceID+originalKey GET；missing/unavailable/unknown/rejected各按原typed结果，不把缺receipt当rollback、缺key当可重发或closed历史不可用当成功 |
| `reconcile` | 用户显式选择原key的唯一原POST调查，按当前workspace.operator/原排他/原物理结束门，绝不重放保存/修复/自动重试；也不因GET missing自动调用 |
| `close-workspace` | 用户明确原workspaceID/openEpoch/operationKey，唯一原close；200/202、逻辑/持久/物理三轴照原结果显示，不把accepted/executing或客户端Wait结束当closed，也不停止其他工作区 |

- 输入只有固定flags及stdin：可信配置沿原8192界/known文件first-end，business stdin最多1MiB+1探界，原strict ParseJSON/重复字段/UTF8/深度/生成DTO规则，零擅自trim/换行/NUL修复。没有任意文件读取业务参数、URL代理或shell命令；command固定scope字段的UUID/版本沿原生成语义。`confirm-text`的CLI intent包装仅用于当前操作关联，不是共享bridge `confirmationHandle`或Host新DTO：`scope`精确包含原installationId/principalId/workspaceId/openEpoch，`candidate`和`commit`是上述原生成结构。原preview session generation可以不同，但不允许换安装/主体或用新epoch替原epoch；所有Host权限仍由当前会话逐请求检查。
- C独立结果解释，不共享React/桥消费适配器。先分原`notSent`、`unconfirmed`、`complete`；前两种无Host status/完整result，不将网络RST/截断/timeout/输出或清理失败改成业务unknown/rejected/rollback。完整HTTP错误仅原`ErrorView`，不得按进程exit0、200/201/202单独判断保存。confirm的200原`Receipt`须匹配workspace/operation/candidate/resource/contentDigest及合法新版本；202原`OperationView`须是同workspace/operation/`kind=save`，只有`logicalState=committed`、`persistenceState=known`及同一完整匹配receipt才保留保存确认，即使physical仍executing或`viewDeliveryFailed`也不再save。仅accepted/无匹配receipt保持未确认，原各轴不改。原key调查的完整receipt也需scope/key/内部字段一致，不猜原输入；输入关联不足时如实只展示Host调查结果，不捏造匹配。
- 每个命令拥有自己的Connection/root、stdin、response、stdout/diagnostic及原子单次attempt槽，先根保留再Acquire；`confirm-text`在本次唯一传输前消耗原命令的once，任何返回不重置/重发。不同显式进程不是自动retry，重复原key仍交Host唯一历史；CLI不建立离线mutation队列/第二业务库、不复制React journal/Attempt权，不因结果missing恢复发送权。命令结束只尝试一次原自身`End`，再其原Connection.Close；未知first-end保完整owner/首次错误，不用第二nil洗白。显式open后的workspace由Host保有，不每调用自动close；连接/等待取消、stdin/输出断开都不发送Host stop或取消accepted业务。
- 正常公开输出允许用户授权的原正文/候选/receipt，仍不能包含bearer、bootstrap、MAC key、私有配置路径、原native handles。stdout用有限CLI传输/当前publicscope/原Host DTO的外包装区分业务与清理；取得实际Context后，外包装另保原公共`sessionGeneration`，用于独立PID/birth/本会话关联，preview/confirm intent仍只含上述精确四维scope，不把前一会话代次当候选权限。包装不是另一业务规范或保存库。已验证保存确认必须在自身session-end/物理cleanup失败时保留，另报cleanup待确认，不能抹成save失败或重发。工具exitcode不是业务终态：0仅工具已知正常交付，1工具/传输/清理/输出未确认，2本地输入/命令不可用；完整Host拒绝/accepted也须解析原typed结果，不能用code0作保存确认。stdout写失败仅固定脱敏诊断/原operation关联，不进行第二HTTP写；诊断不含正文/paths/秘密。有限参数为单响应/累计原响应最多64MiB、页最多16，最终编码stdout含wrapper、JSON转义、重复末页和末尾换行同样最多64MiB；超界不写任何stdout字节，原完整已确认receipt仍保在原owner，不降为保存失败或重发。R73只读复核发现仅计原响应会漏重复末页，已先以十个合法大`RecordList`页实际触发原输出断言，再定点补最终写前界；这不是物理内存峰值或Host契约变化。
- 批准验证范围（现R46已补CLI工程实际，React及完整组合仍未验）：同一真实normal Host与actualnormal `aboard-client.exe`独立进程/独立会话，显式open不重开既有owner；preview→新调用显式confirm→独立public消费者读取同版本/receipt，再反向public写后CLI读。两个CLI同base各preview，first确认胜、second陈旧，原keyGET/reconcile和正常Host停机/new trusted config/open后原receipt对应；旧candidate不在新epoch重新确认。实际当前身份/zero-grant/跨区/错误scope/版本能力、无自动init/repair/start/reopen、普通JSON/字节和raw诊断负例。响应解释200/202 committed+receipt+viewDeliveryFailed/仅accepted/错配/截断、cleanup不倒退已确认保存分层；真实POST发送次数/无自动retry、第一write后故障不得第二写，组件受控peer与实际Host accepted-pending分别归原V009/V010/V012，不拼接为React产品通过。React联合双向A/B三条与十轮冲突、物理pending关页/浏览器退出、跨安装/主体/代次/区/epoch迟到仍原T027–T030的同组合验收，未由本计划代签。
- 原全Git工作树/契约/GoTS/字节夹具/工具/normal与TEST镜像/实际PID-birth-session关联/每个原输入及完整路径集合须记录；显式generate/format与只读验证分离，只重跑受影响可信连接/CLI/public链和原保护负例，不重做无关长容量。只允许现有已安装离线依赖；Windowsamd64/固定CGO0 race skipped如实分列。原T014正常容量双mode已R42完成，不重跑。本节原17FR只读门后现由[R46](<research.md#L431>)完成工程：753输入、十四步骤全0、normal/tag CLI与原五命令/原native回归及vet、两个actualnormal exe独立公共链三轮/完整receipt/同base胜负/原key/正常cold已验；预修合法大页序列化溢出实际红后定点补最终stdout64MiB写前界。原已确认receipt不因输出/清理失败变更或重发。源码/完整路径集合/镜像及原R45影响链由[最终交接](<../../.data/logs/003-validation/text-cli-428-handoff-final-results.json>)核，原T017据此勾，不代React联合/十轮/accepted物理pending/全native清理故障。

逐原则静态核对：I/INV01–05原Host/公共当前身份和原稳定ID为唯一业务owner，CLI包装非第二数据库/权限；II/INV06–08不init/repair/自动open/目录扫描，实际独立PID/真实排他及原close保留；III/INV09–14显式preview/confirm和原key、完整receipt及当前基线、unknown仅调查，不承诺跨文件/硬件原子；IV/INV15–17CLI退出只结束本会话/IO，accepted与workspace排空仍Host，不取消B；V/INV18–21原TLS/pin/currentDB授权、有限输入/页/64原连接和once/root、无secret日志，无恶意同SID沙箱声明；VI局部CLI/区错误不故意停其他区/Host，共享不可信依原Host围栏；VII真实独立公共消费者、不共享桥业务适配，联合React/许可/固定产品验收保持未运行。原批准范围无需求/宪章变更；上述设计当时不构成实现通过，现工程实际限R46明确范围，后置产品/外仓/发布仍不代签。

### 4.33 T015当前Q交付证明与独立ACK正常删除首片（implemented / R44正常组件已验，原T015仍partial）

原T015/V008，Owner B/A串行，FR008/011–015/018/020–021/024十项及SC004/007/010/012交付本地局部，依T013/R34–35和T014/R36–43。最小首片只生产journal正常组件，不把旧proto的completeMatchedResult布尔和synthetic hash当Host receipt验证，也不以受控page/UI标志称真实React提交。当前不接bridge业务、正式POST、来源adapter/真实浏览器UI或新fault API。[R44](<research.md#L411>)已以749当前输入、十三步骤十一0/两有效观察expected1、normal/tag二十父各三和原codec/protocol-digest/vet补齐本正常片；原page-wide UI bit缺陷真实先红后修为每份result独立共享marker，同页A不被重置、B不得继承。真实DELETE native未知、真实Host/React/UI仍未验，不能由R43 Consume故障补签。

- 新私有delivery source仅未来可信native结果owner可构造，含原Store/owner/handle/full Binding、当前page pointer及双代次、原完整200 Receipt或202 OperationView原字节、原native预期resourceId/contentDigest。先原生成schema严格解析，核原workspace/key/candidate/resource/content及committed+known+receipt（物理状态/视图失败独立，不要求drained才保确认），再与同owner精确Q全metadata对齐。首片仅已确认保存结果；其他完整Host错误/accepted及恢复适配由后续T016明确承接，不假称本片覆盖。source一次latch先于准入，禁止浏览器构造/source digest当权；原完整result只在调用中验证和hash，不进持久行或proof。当前TEST合成合法结果和受控page，不签TLS来源/真实UI。
- 每Store最多4096个原Q的一份交付证明，随机256bit deliveryId，proof只保原owner/handle/完整metadata摘要、结果摘要、原page pointer/不可变client和doc代次与共享一次状态。UI提交必须绑定该份delivery/result的独立共享状态，不是page可复用bit，也不通过重置page bit破坏同页另一已提交结果。本轮旧页bit实现已由同页两Q实际反例先红证实，随后只改proof状态和对应反例，不改原UI先于独立ACK意图；实际React接线仍后续。原binding原scope不改，page.Current回调必须原native当前scope/会话/文档检查、不得重入本Store；独立UI提交标记只由可信实际UI交付owner设置，ACK请求不是自行设置该标记。复制proof仍指向同一次状态，旧/错page、ID/handle、未提交UI、当前检查失败/冷owner拒，Q不变；暂不从本组件自动重颁旧proof。
- 独立ACK在原Store.mu/lock及原complete facts、精确Q row复核后，沿原transaction(false,…) DELETE仅serial+source+原完整metadata，RowsAffected=1，成功known COMMIT才标retired并移出有限proof表；N不退，其他U/Q/旧source/receipt事实不改。未知保持原pending/fence/proof、不重试DELETE或发save，后续原Investigate只完整old/next且实际first-end后退休，不复活proof；knownClose和调查退休均清旧proof内存。原T015真实DELETE FSYNC未知/原库调查留下一片实际运行，不能由R43 Consume故障代签。
- 本片新增delivery_windows.go/_test，最小Store字段及两已知退休点清理，不改schema/codec/原transaction/Host/契约/旧probe/依赖。真实正常SQL、scope/receipt各单坏、两个同主体page/提前UI/迟到/current false/duplicate/copied proof、原U不得创建proof、wrong ACK不消耗合法proof、Q已知删除/其他行+N守恒/cold旧proof拒；原native/容量结果按完整source/影响链复用，normal/tag当前亲本/codec/protocol-digest/vet及所有输入/路径集合/产物核。两个实际观察控制仅合法current proof替坏ID及UI已提交代未提交，使原断言失败，不删生产guard。fake页/布尔不是UI或原生故障。T015整体未勾，T016实际Host结果和未来React/独立产品/ENOSPC等仍原后续，不加React或所有fault×UI矩阵前置。

原则I–VII/INV1–21：既有规格/唯一原任务；原DB单owner/当前scope；query不写，只有原ACK明确DELETE；未知不退款/不重试、所有first-end原R43机制保留；本片有限proof表不第二结果库、不保存receipt/正文/秘密，日志只必要metadata；旧dirty/失败/并行proposed保留，本仓无下载/ACL系统策略/跨仓/提交push发布。该静态核对不代运行，实施前按原tasks同步并strict readonly analyze。

### 4.34 T015原独立ACK DELETE真实FSYNC未知与已提交UI不回退（implemented / R45故障组件已验，真实UI/Host来源装配仍后续）

Owner B/A串行，仍原T015/V008、FR008/011–015/018/020–021/024十项及SC004/007/010/012交付本地局部，依T013/R34–35、T014/R36–43及R44正常proof。只新增`internal/clientjournal/delivery_native_windows_test.go`复用原生产`acknowledge`和原`Store.Investigate`，不改生产代码/旧fixture/DBschema/唯一契约/依赖；只有实际发现实现缺陷时才回原技术阶段作最小修正。本片UI/结果源仍受控，不代真实React/Host来源或所有fault×UI组合。

- 从R44真实Store/Q/proof fixture起，额外保留一条U、一条无关Q及其未使用Attempt和未退休proof，完整facts/原row/gen/FileID/issued/交付表及原已提交UI marker先取证。先ordinal0原ACK真实known成功且实际flush>0，只有原目标Q删/该proof退休，N及无关行/结果不变。只选正控已见的ordinal1，用既有只读合成句柄在原VFS中实际FlushFileBuffers BOOL0→driver1034，调用同一原ACK/ctx/参数，不造SQLiteerror/ctx错误替代，不用Consume故障代签DELETE。
- 实际故障须ErrStoreUnknown+driver1034/failed1，无known退Q或退休proof，不回退已提交UI、不调用save/重发或重试DELETE；原callback/goroutine确切返回且active0后才disarm/restore，restore不是join。原pending含完整old/next、同Store/原DB+pin+lease和全部未结束proof/Attempt保留，ordinary入口和重复ACK拒、Store.Close未开始DBend、真实第二native owner排他被拒。未用无关Attempt也须fenced，无关proof UI状态不得借目标改变。
- 不通过普通OpenStore/Close洗白未知。直接沿R43同owner `Investigate`：原DB实际首次结束、仍持原pin/lease最多一候选同FileID现有库句柄、精确原schema/integrity/完整有序MAC/source/N/U/Q必须整old或next；候选→pin→lease全实际known结束才退休旧owner并清旧proof。只签实际观察到的old或next，不按缺row/可锁推回滚，不补所有ordinal×三态。正常fresh同原FileID/gen/fullfacts，目标若old仍Q、若next精确缺失，无关U/Q及N完整；cold线索不能Consume，旧proof/原unusedAttempt不得恢复。即使UI已确认，调查也不发布新交付证明/发送权或Host receipt。
- 同build有效控制只省原native刺激，原ACK确切COMMIT成功且proof退休，必须击穿“原DELETE没有真实未知”断言；不删除guard/弱化expected。显式format仅NEW TEST，独立只读验证冻结allGit+exactrunner/privateC/offlinecache/原Go1.27.2/windowsamd64/CGO0，normal/tag新native父各三加原二十父各三、独立codec Python/protocol-digest两mode/两vet、原R43与R44证据及R42实际容量指纹精确复用。新TEST不能重跑两个46分钟完整容量或修改旧报告；noSource/pathset/image漂移和零selectedskip后才记录。本片已由[R45](<research.md#L421>)按750冻结输入/十二actual步骤十一0+一个真实无故障成功expected1、normal/tag二十一父各三/原codec Python/protocol-digest两mode/两vet完成；六次原ACK真1034均whole-old。精确原/candidate SQL连接与保存的原pin在调查前可用、结束后实际不可用已补强，不只信flags/冷开。旧首pass和中间TEST API编译失败保历史；实际Host/React来源、有效页面UI及bridge装配未由当前controlled proof代签，原T015仍partial不勾。

原则I–VII/INV1–21：只原已批准任务/唯一journal DB与原UIproof；真实当前page及结果关联沿R44，不把private marker当UI；known DELETE与COMMITunknown分离、逻辑确认不被清理反转；原排他/partialroot/唯一first-end与IOjoin沿R43，无第二owner/第二次结束洗白；private材料不外发、诊断仅必要计数/合成原key；容量旧正常事实精确引用，unknown不退额；旧dirty/失败/并行工作保留、不跨仓/下载/ACL策略/提交push发布。设计静态核对不代运行，原T015任务同步和十FR只读门已完成后才NEW TEST。首次实际full虽过，窄审指出新TEST只信first-end flags/字段清零与cold可开，可能漏无操作Close；同十FR技术反馈仅增强新TEST：保原真实DB指针和native pin，结束前各可用正控，调查后精确op.original同指针、原/candidate SQL连接实际ErrConnDone和原pin用既有GetFileInformationByHandle实际ERROR_INVALID_HANDLE，不调用另一次Close或重试；源生产真实Close链不改。受影响设计复核不改意图/权限/任务依赖，无新增故障矩阵或后置UI门，增强后已按R45当前format/full实际再验，旧pass只保历史；这些受控结果/UI和原生句柄观察不代真实Host/React来源装配。

### 4.35 T016原可信连接预览来源、持久句柄及一次正式转发首片（implemented / R47 native局部已验，原T016仍partial）

Owner B/A串行，原T016/V009–V010业务局部，FR003/005/008/010–015/018–021/024十四项及SC004/005/007/010/012局部，依T008/R19–22、T011/R25–29、T013/R34–35和T014/R36–43。先实现`clientjournal`内最小可信native文本会话适配，不在renderer暴露Binding→source工厂，也不新增HTTP/权限/schema或第二Connection；后续由原BridgeSession串行装配。同包可直接使用原私有source/issue、原完整row及Consume/Attempt，不循环import clientnative。可信Host接口只原Context和原RequestJSON；生产调用者须传原Connection，不接受browser选择method/path/header，TEST替身不构成来源证明。

- 每个native会话有界64、原Store/Host均借用；每会话最多4096条最小来源记录，只保原binding、resource/content摘要、候选expiry及原native page指针/不可变双代次，不存正文/receipt/业务终态。Page.Current来自原document owner；scope安装/主体/真实Host session/workspace/epoch五维显式提供并严格校核，绝不全局补目标。每次准入重新原Host Context，页当前门前后复核；本地cutoff仅退本会话准入，不取消已accepted或结束原Host/Store。
- `Preview`只原生成PreviewRecord发一次固定workspace preview路径。仅complete 201、严格原CandidateView、目标/kind/base/字节/当前主体对应的原Content和Candidate摘要一致、合法future expiry后，才构造一个私有稳定source；native随机previewRequestId及operationId在原持久binding中固定，UI不能选择或补key。原source latch在任何Store准入前烧，Issue已知提交N/U后才可返回原CandidateView与confirmationHandle。页/当前身份失效或持久化失败不给预览/handle，不因此重发预览或正式保存；已持久旧范围线索不抹。
- `Confirm`只接受当前native page/五维scope和原handle；没有renderer candidate/commit正文。先原row完整绑定与当前source metadata相等，再Consume U→Q已落盘，原Attempt.TryUse烧一次且当前page/context/候选期限再检，最后只原Connection一次POST原CommitRecord。失败/取消/不完整/缺clue/cold都不重发、不换key/preview/epoch、不复活Attempt。返回原传输分层；complete严格原OperationResponse schema，已匹配200 receipt或202 committed+known+receipt才记内部保存确认，physical/viewDeliveryFailed独立；错误/accepted/未确认仍保Q，绝不按HTTP状态重存。当前只向native返回结果，不把晚页面交付或真实UI/独立ACK假装完成。
- 验证：新broker单位真实Store+受控Host观察确认U→Q在正式callback前且并发/重复只一次、wrong scope/page/MAC/变更结果/冷恢复/未知传输/满额或fence零正式POST；原正常Host.Run+生产Connection+真实公共preview/commit/receipt/正式读取和原key、非空更新/基线冲突、原Store冷重启线索不恢复发送。实际TLS原通道保留，不用仅Host去重推转发一次。显式format与验证分离，normal/tag当前影响/源和完整路径集合/镜像/原R45及容量证据链；两个观察控制使用实际原有效输入令拒绝oracle失败，不删产品guard。本native首片已据[R47](<research.md>)以756冻结输入、2026-10-26 09:12:54–09:15:48 +08、十九actual步骤十七0加两个真实有效expected1完成；normal/tag四新组件亲本各三及真实Host.Run/TLS原Connection亲本各三，原journal/HTTPHost两mode和两vet通过。实际非空create/update的CRLF/非ASCII、原receipt/record/原key、持久U在返回handle前/Q在正式callback前、并发两调用仅一次POST、旧page/scope/冷恢复拒权、借用owner仍活与正常Run结束均已观察；lateContext页面窗口经实际修前红后定点复核修复。原完整输入/路径集合/四个TEST镜像和R46链已核，未重跑旧长容量。真实renderer五维桥、React UI与T015交付ACK装配、全部16动作/真实网络故障三重复仍原后续，原T016不提前勾。

本片源码反馈：初焦点虽过，窄审发现current在阻塞原Host.Context之前检查页面、之后未再核；新增受控真实Go用例在最终Context期间退休原页，修前实际formal callback被执行。仅在原Context/Store等待后及最终POST前再核页面/取消/候选deadline，拒绝保已知Q和已烧Attempt不恢复；同十四FR只读影响复核无新意图/权限/依赖变化，原红和旧pass保留。完整传输与保存确认分离不改：schema合法但receipt关联不符仍可保持complete传输而SaveConfirmed=false、原Q保留；该静态疑点不认作第二漏洞或伪改传输未知。

原则I–VII/INV1–21：唯一Host正式owner/原公共类型与当前DB权限；journal只是非授权关联且来源native持有，MAC不授权；完整receipt和传输/三轴分离；原IO/Store所有权借用、未知保留且没有重试/取消accepted；固定64/4096及原元数据界；原页scope/旧clue不跨代授新准入；真实Host加受控观察分列，不把组件当React/发布。批准spec/宪章/跨仓权限不变；先原T016任务同步和十四FR只读分析，再新source与当前验证。

### 4.36 T016原桥精确页面至native预览/一次确认装配（implemented / R48 renderer局部已验，非完整T016）

Owner B/A串行，原T016/V009–V010首个renderer业务局部，沿§4.35十四FR与SC004/005/007/010/012局部，依T008/T011/T013/T014真实工程及R47。仅把已有可信TextSession/原借用Store、Connection接到原BridgeSession/唯一Handler；不新增Host API/业务DTO/权限/第二HTTP栈，不实施未授权Workbench或T015 UIACK。

- 原BridgeConfig可选可信native Journal指针，原BridgeSession借用而不Close；给定才在已验证原Context下建立唯一TextSession，partial错误仍原BridgeSession完整root。旧无Journal的context/exit行为和其他十四动作unavailable不变。关闭先原localcutoff与HTTP全部实际结束，再仅TextSession本地截止，原Browser物理结束；Store/Connection仍由原caller实际结束，accepted业务不取消。
- Handler只有新增可选有限Text hook支持previewText/confirmText，输入分别原生成PreviewRecord与精确confirmationHandle；scope必须完整五维、每项canonical。原authority/Origin/Fetch/Cookie/CSRF/pageId/双代次/bodyfirstend/8原request槽门不变，hook前后及交付前原current/cookie复核。preview返回原CandidateView和仅client.confirmationHandle；confirm返回原完整DTO/传输分层，未确认无Host状态或body。没有method/path/header透传或原Host bearer/Origin/Cookie转发。
- hook从原已ConsumeJoin的PageBinding定位原有限bridgePage槽，惰性建立一次同指针TextPage，Current只回原document owner，不按renderer元组造新native文档。原page/Store/Connection双generation关联及R47晚Context门全部保留。页面scope不补全局当前workspace；误scope/跨页handle/重放/冷恢复不发正式POST。原U返回前落盘、Q在正式POST前落盘，未知/取消不恢复Attempt。
- 当前确认结果仍native匹配原receipt，但本片不发deliveryId或删除Q，不把DOM显示/HTML标记等价已实现独立UIACK；后续原T015把完整结果交付证明和实际提交UI后独立ackDelivery接线。当前合成外部脚本仅实际Edge业务消费者，不宣称真实React/静态外仓许可或完整V009/V010。
- 验证仅NEW有限hook TEST及真实Host.Run/productionConnection/actualEdge/原Bridge/真实Store的公开非空文本预览→handle→独立确认→原receipt/旧公共读和原key、Q保留、重复与另一页面/错scope/错CSRF零正式POST，peer会话保持、原Server/Browser和callerStore/Connection全部实际结束。原不含Journal旧R35组合及原R47/nativejournal/Handler影响回归分层；normal/tag当前源/全部pathsets/镜像/原R47与R42容量链复用，显式format唯一源码写步骤。两个实际控制只把另一页改回原合法同页、或把坏CSRF改回该原页合法CSRF，原路径均真实保存200、完整receipt且实际N1U0Q1后击穿原拒绝oracle，不删除guard。未知结果和页面退休依原机制，不新故障笛卡尔/ENOSPC或后置UI门。本片据[R48](<research.md>)以760冻结输入、2026-10-26 09:59:18–10:02:12 +08、十八actual步骤十六0加两个有效expected1签局部；normal/tag二十一native/browser亲本各三（含真实renderer预览/确认六个原PID-birth-image）、三native兼容亲本各三、十六桥亲本各三、原四来源亲本和真实Host来源各三及两vet全部通过。原Host公开非空CRLF/非ASCII文本的Candidate/receipt/GETrecord/原key一致，双页错scope/CSRF/跨页handle及重复拒绝、原Q保留不删、peer200、全部原owner实际结束及正常coldQ仅线索已验；四个NEW源和两个原接线文件的完整输入/路径集合、两个TEST及installedEdge/R47链在只读交接核，不把synthetic静态JS当React/外仓许可/实际UIACK或原T016所有动作。初无控full虽过仍保旧结果，增加本来计划的两有效控制后新TEST/runner再当前全验，无运行中源码变更。

原则I–VII/INV1–21：唯一原Host业务DB/原journal关联owner，当前身份/五维scope与原公共生成DTO不重定义；精确原生文档而非Cookie单项授权，持久Q先于至多一次正式写；结果与UIACK/清理分层不倒退确认；借用资源和unknown实际结束遵循原owner，有限16页/8请求/64native会话/4096来源不增第二池；包文件owner串行、秘密不外发、合成环境/真实进程和代码影响证据分列；不跨仓、不下载、无提交发布。先同步原T016并严格只读分析后才实施。

### 4.37 T015原完整保存结果至精确页面UI提交及独立ACK装配（implemented / R49保存确认局部已验，原T015仍partial）

Owner B/A串行，原T015/V008，FR008/011–015/018/020–021/024十项、SC004/007/010/012交付局部，依T013/R34–35、T014/R36–43、R44–45正常/DELETE故障组件及R47–48真实来源。沿同一原Store/Q/Connection/精确TextPage，不第二DB、receipt持久缓存、Host API、browser凭据或新的私有动作。

- 原TextSession.Confirm在原完整200Receipt或202 committed+known+匹配receipt处才用原native binding/resource/content、同页Current mint一次私有deliveryProof；TextResult→原native bridge→有限响应仅增加可选deliveryId。结果/错误/accepted/未确认不得伪mint。proof只原handle/owner/page/双代次及metadata/result hash，仍4096界；结果网络写回、DOM收到或保存确认本身绝不自动mark UI或DELETE，proof创建失败不倒退已经确认的Host结果，Q继续留存。
- 冻结ackDelivery输入仍只有confirmationHandle/deliveryId及完整五维scope、原pageId/CSRF/Cookie/文档门。原native当前身份/实际session/原owner、原TextPage指针及原handle/scope、proof原map指针/id/page/metadata全部重校后，才把**这一个结果**的uiCommitted置真，再调用原独立acknowledge事务。该显式ACK是当前可信页面“已完成结果UI提交”的声明，不是native可独立证明DOM/React状态；不得按page-wide bit、收到响应、旧/错ACK或其它结果继承。ACK不附候选expiry门，已保存结果可晚于preview过期；unknown DELETE保原pending/proof/fence不重试/不再save，known后只local retired:true，重复拒。
- 原Handler仅为现有ackDelivery加有限分支和可选DeliveryID，仍原8请求/16页/authority/Origin/Fetch/CSRF/Cookie/交付前Current；成功ACK为kind local，无hostStatus/host/transport/client/deliveryId混入，不改变原Host DTO或其余未接动作。原Bridge借Store/Connection，Close顺序和unknown保根不变。
- 新actualEdge外部合成静态脚本独立验证原scope/receipt/candidate/resource/content后，先显示完整保存确认；另一个明确UI提交动作修改DOM并跨实际render frame，最后第三个独立ACK动作才发原请求。测试在response显示后和UI提交后分别观察原Q仍在、ACK后目标Q才退；另一页/错ID/错handle/错scope/重放不得删，page A的UI不赋page B或另一proof权。原Host原key/record/receipt不变、peer200、原Owner真正结束及cold无已退行；不把合成页面当React/外仓许可，非确认/错误/accepted显示和全部故障组合留原后续。
- 先原组件单位与真实Host/Edge/精确页/journal正常链、两个有效观察控制（valid deliveryId代错值真正DELETE，以及合成UI gate提前允许ACK使Q在UI前退休），再current normal/tag、R44–45原ACK native和R47–48来源/renderer影响回归、全输入/路径集合/镜像/原R42容量链，只读交接。显式format唯一源码写步骤，验证脚本不隐改旧fixture/源或重跑两个46分钟正常容量；未运行设计不称passed，原T015仍partial直到实际范围签收。当前首focus新真实三步UI/ACK已实际通过，但旧R48 TEST仍把任何deliveryId视为该历史partial的错误；只对已完整confirmed200的confirmText允许并校验canonical64hex可选ID，preview/拒绝/未知仍零ID，原Q不自动删除与cold守恒断言全部保留。仅同步此已批准增量后的旧验收预期，不删除原保护门、不改产品，旧失败report/C保留并对新source重验。
- [R49/431](<research.md>)已以763冻结输入、2026-10-26 10:51:01–10:54:24 +08、十八actual步骤十六0/两个有效expected1补本保存确认装配：normal/tag二十二browser/native父各三、原来源/三new交付单位/每结果UI及原Consume与DELETE真实1034影响各三、有限桥/真实Host来源和两vet通过。六个actualEdge各原PID/birth/image/精确文档-cookie-CSRF、原Host Run/Connection/Store，原receipt显示与另DOM提交跨frame时Q1，第三独立ACK knownDELETE后Q0/N不退，wrong/crosspage/scope/id/duplicate拒，正式结果不变/peer200/全原Owner实际knownend/cold无旧Q；两个控制分别原合法deliveryId与合成UI门绕开，真实Q0才击穿拒绝/UI顺序oracle。当前为synthetic已追溯外部静态包而非React/外仓许可，明确UI声明不是native观察DOM。完整源/路径集合、两TEST/currentEdge和原R48/R44–45/R42影响链在[只读当前交接](<../../.data/logs/003-validation/renderer-delivery-431-handoff-current-results.json>)核，run后仅Doc3，验证不隐式格式化；固定CGO0 race skipped。T015其它完整结果/accepted调查和真正React/全故障产品仍未签，不提前勾T015或T016。

原则I–VII/INV1–21：Host唯一正式结果与原DB当前身份、journal唯一Q owner和准确关联不变；不从页面/缓存mint Host权限、不持久receipt；UI确认/业务确认/DELETE/原生结束分层，失败不抹已确认结果、unknown不退额；保有限页/请求/证明及原partial owner/firstend；所有数据合成、诊断脱敏，包owner串行；T018外仓权/真实React/完整产品/物理ENOSPC不被本片代签。同步原T015并严格只读分析后实施。

### 4.38 T016有限工作区查询、原key调查与显式close（implemented / R50查询局部已验，原T016仍partial）

Owner B/A串行，原T016/V009–V010，FR003/005–008/011–014/017–021/024共15项及SC002/003/004/009–012局部，依已验T008/T011/T013/T014。只接冻结workspaceStatus/listRecords/getRecord/getOperation/reconcileOperation/closeWorkspace六动作；visible/open及journal线索仍后片。复用唯一原Connection，不新Host API/权限/DTO/HTTP栈/Store或发送权。
- 每个动作原完整五维scope、精确page/CSRF/cookie/header/8槽及准入/交付current门保留；native在原已join文档中定位，原Context等待后及实际请求后再核当前安装/主体/session/instance/grant和原页面，不持session/doc锁跨网络。输入各有限：status空、list limit默认20最大100/cursor<=1024、get原resourceId、key/reconcile原operationId、close原生成CloseWorkspace且epoch等scope。唯固定method/path由原generated route/parameter/body再校验，不接URL/方法/header。reconcile和close各只一次原POST，不从missing推重放或换key。
- records公共API没有conditional epoch：原status前后核workspace/tenant/open生命周期/原epoch，任一不符丢原正文而非虚构原子GET；status返回原目标且非空epoch必须等scope。getOperation/reconcile允许调查已closed原key，不隐式open/刷新scope，核完整原OperationView.workspace/key及若有receipt的save/committed/known和同workspace/key；close可返回202，不能说closed或自动重发。原Host完整ErrorView按类型保留，不按状态码推保存、rollback、safeRetry。非complete不外发Host状态/正文，不mint deliveryId、Q证明或ACK。
- 新query通用有限路由/结果检查与native适配、组件单位和一个真实Host/Edge合成页面验收；默认无Journal也能查询。真实公开非空M1/receipt/页查询、原key和显式reconcile、一次close/GET调查、旧epoch读拒、两文档/越界输入/迟到页面拒及peer保持；一个同build合法epoch替旧epoch控制使真实GET成功击穿原拒绝oracle。normal/tag新亲本及受影响桥/浏览器/Host回归、明确format和只读验证分离，原R49完整源/路径集/镜像/原R42容量链不重跑长测。代码和验收未运行前不签T016或React/完整SC联合。

首次实际focus已通过旧组合但新查询TEST假定一条记录且limit=1应无cursor，原Host `workspace/records.go:141–144`明确满页可有cursor并随后空终页；仅新TEST改为跟随真实cursor验证空终页、按原所有item/正文/receipt而非分页包装比较正式事实，保留旧失败，不改Host/契约或新增分页规则。该技术反馈仍原15FR/七SC局部，意图/权限/依赖与只读门映射无变化，修正后对新source显式format/实际再验。随后真实查询已到close-key committed/drained，新TEST又误把save的persistence known要求加给仅内存close；原`http/views.go:148–157`精确保持close.persistence=unavailable/无receipt，只把实际Closed映为committed/drained。仅新TEST显式要求该unavailable并原key GET见drained，不改业务三轴、不伪造持久close或改Host。再读原`workspace/reconcile.go:72–77`，save-key调查只在open/unknown且有原resources合法，真正closed返回原409/WORKSPACE_CLOSED；只有close-key仍可直接调查。新TEST不再强求closed save 200，而与原公开GET同ErrorView逐字段（仅每请求correlation归一）核，重开后同原key完整receipt必须仍相等；不得自动open或用不可用/缺行当rollback。以上只是新TEST精确对齐原业务，不改生产/契约/准入门。

- [R50/432当前证据](<research.md#L474>)已按768冻结输入、2026-10-26 11:45:16–11:48:21 +08、十三actual步骤十二0/一个合法epoch有效expected1验本查询局部：normal/tag二十五browser/native父各三、原native三父各三、有限桥/三个新query单位各三和两vet通过；六个actualEdge/原Host.Run/Connection/精确doc-cookie-CSRF，真实原receipt/正文/页终止、一次reconcile与原close-key GET drained、closed save原ErrorView、原用途重开后完整receipt、错误/旧epoch拒、peer保持及所有原owner实际knownend。记录epoch仅前后观察，原close.persistence仍unavailable/无receipt，不称持久close/原子GET；ErrorView不当保存或重放依据。包内容synthetic非React/外仓许可，HTTP参数词法额外源码疑点仅私有观察未另实验，不补签all-spellings。原R49/R47/R42完整源链、当前全源/路径集合/两TEST/rawlogs及[只读交接](<../../.data/logs/003-validation/renderer-query-432-handoff-current-results.json>)核，run后仅Doc3、固定CGO0 race skipped。visible/open/localClues、其它结果/真正React与全产品后续，T016仍不勾。

原则I–VII/INV1–21：Host唯一DB/currentauth和原业务三轴不变；scope是关联非授权、原key只调查；无自动open/repair/正式POST重试、无缓存业务库；当前页面/实例失效时晚结果不交付，close未物理结束不称closed；有限输入/响应/页/claim和原first-end及unknown保留；诊断不回显私有定位/正文/秘密；原生层B/A单写者及C独立CLI不混；synthetic页面非React/外仓许可/完整产品。同步原任务并严格只读分析后实施。

### 4.39 T016可见集合、合法用途前置及显式打开（implemented / R51选择局部已验，原T016仍partial）

Owner B/A串行，原T016/V009–V010，FR003/005–008/011–014/017–021/024十五项及SC002/003/004/009–012选择/打开局部，依已验T008/T011/T013/T014/R50。仅补冻结visibleWorkspaces/openPrecondition/openWorkspace三个动作，不混本地线索、其它结果/React或新增Host API/权限/DB/业务schema。
- 所有三动作只无workspace scope，目标在严格原生成input；visible limit默认20/最大100、cursor<=1024，原Host分页可空集合，不提前探物理profile或自动选择。两个POST仅原ClientOpenPreconditionRequest/ClientWorkspaceOpen，用户expectedRegistrationVersion与原snapshot保持原字节，不能遇409重新前置/更新基线/重试打开；每动作仅一个原请求。
- 复用原Handler authority/Origin/Fetch/Cookie/CSRF/page/双代次/8槽和正文首次结束/交付前当前门，native只原已join精确文档和唯一Connection。实际Context等待后/原请求完成后重核安装/tenant/主体/session/instance/grant及当前页；完整Host类型与该input目标、visible session/grant、前置完整身份和purposeopen、成功open同tenant/m1-text/format2/schema1/非空epoch复核。不把metadata或快照当授权、visible不预承诺M2兼容，真实open由原Host独立再授权/CAS；ErrorView保持原类型/三轴，非complete零Host状态/正文，无confirmationHandle/deliveryId/Q/新发送权。
- 当前trusted native只固定生成route/parameters/body投影，不接受URL/method/header或全局当前目标；没有新的Host调用栈、owner或自动open/close。生成类型继续唯一OpenAPI；其它已有sixquery/Text/ACK/exit不变，无Journal也可metadata/open。数学参数词法仍原合法输入，不为旧Host数字读取源码疑点偷偷改产品；本片实际limit1分页，不声明额外all-spellings行为。
- 先有限投影/错scope/typedforeign/未确认无结果单位与实际原调用计数/Context期间退页；再真实Host.Run/Connection/Edge两page，真实visible分页、前置不打开且闭态源bytes/FileID保持、其它会话snapshot不能用/错registration和篡改拒不新epoch、原snapshot合法显式打开/旧snapshot重放不再打开、M2不可降级、原正式文本/receipt不变/peer保持/所有原ownerknown结束。一个同build合法原registration替坏值控制使原open200击穿拒绝oracle；另私有路径观察控制须被真实decoded扫描拒。验证explicitformat和零写full分离，冻结所有Git输入/root-C源与两完整集合/两TEST/实际Edge及原R50/R49/R42证据链，normal/tag新亲本各三/旧桥浏览器受影响回归及两vet后才签本局部，不提前勾T016。

- [R51/433当前证据](<research.md>)已按773冻结输入、2026-10-26 12:17:14–12:20:42 +08、十四actual步骤十二0/两个有效expected1验本选择局部；normal/tag二十八browser/native亲本各三、旧native三父及有限桥和两vet通过。原Host.Run/TLS/独立Connection/actualEdge双page中，visible实际limit1/cursor完整集合、用途前置零打开/闭态全bytes和完整FileID不变、另一session原snapshot原public与renderer403且peer自己的snapshot原public200正控，用户旧registration409不新epoch，合法原snapshot显式open200新epoch/m1/2/1且replay409，原正式文本/receipt/原key、Bclosed源及peer200守恒，所有原owner已知结束。两个控制分别原合法用户版本实际open200击穿拒绝、响应观察副本私有路径被真实decodedscanner拒；产品门/Host/契约不变。完整[只读当前交接](<../../.data/logs/003-validation/renderer-selection-433-handoff-current-results.json>)核原R50全部输入/pathsets和精确增量、当前源/两完整集合/两TEST/六原process/installedEdge/Doc3及本地links，非业务重跑，固定CGO0 race skipped。合成静态页面不是React/外仓许可；本地线索/其它完整结果和全产品仍未验，T016不勾。

原则I–VII/INV1–21：Host唯一授权/登记/用途CAS/业务owner，原session和当前grant而非页面ID授权；前置零physicalopen且失败不补种/不更新用户基线；原accepted/关闭排空不改、unknown及首次结束保留；有界分页/头体/槽和原未知计额；秘密留native、视图只原公开metadata/DTO，普通日志无正文路径；A/B共享边界串行、C CLI保持独立；synthetic同源页面不是React/外仓或完整SC联合。同步原T016子项并严格只读分析后实施。

### 4.40 T016当前身份最小线索分页与原key显式调查（implemented / R52原T016工程收口，非完整产品）

Owner B/A串行；原T016/V009–V010，FR003/005–008/011–015/017–021/024十六项及SC002/003/004/009–012本地调查局部，依T008/T011/T013/T014/R47–R51。只完成冻结listClues/investigateClue，不把最小线索当Host结果/权限/新发送权，不反设React或T015其它结果为门。

- 原Store同mu及原lease/dbpin下先完整facts（全U/Q、schema/MAC/source/N/顺序/上限），再有界读取至多limit+1个符合原installation/tenant/principal的metadata；不得先身份WHERE/LIMIT跳过整库完整性。only投影handle/workspaceId/operationId，无正文、receipt、原epoch/session/instance或持久状态。按原sequence keyset，默认20/max100，原HMAC游标<=1024绑定journal generation/当前principal/client generation/last sequence/limit（附当前安装tenant），canonical编码/constant-time；MAC不可认证403、认证后代次不符409，不保存无界cursor缓存/不退Q。正常cold原generation/key不变、旧client generation游标陈旧；损坏/手动重建key不可认证不承诺可区分其年代。
- 当前TextSession借唯一原Connection，原Context/current原页面前后再核。list与原公开visible-workspaces current read-or-open集合求交；最多64页、每页100、cursor1024，达到上界仍有next则明确limitReached，不静默当完整。此集合仅过滤metadata，不授GET-key/operator权；新grant/session/instance或page失效不交付。可以空页且有opaque next，不输出隐藏目标。没有Journal时明确unavailable，不创建/repair/补种第二Store。
- investigateClue输入只confirmationHandle+explicitReconcile:boolean默认false，完整当前五scope与原clue安装/tenant/主体/目标匹配，但旧session/instance/epoch仅历史关联不得作为当前许可。只有原handle经当前owner完整MAC取原workspace/operation，默认一次原公开GET，显式true才一次原POST reconcile；原Host read/operator/原actor权限裁决，闭区原409/unknown/missing/rejected完整ErrorView不映成rollback，不自动open/refresh/issue/Consume/TryUse/ACK/DELETE/换key/重发save。成功OperationView同workspace/key，若receipt必须save committed known且同candidate；R52此首片仅调查不mintSaveConfirmed或deliveryId；当前§4.42仅为原Q的可信原key committed/known匹配receipt增加独立恢复交付，原U/未决及其它结果仍无可退证明。返回前原Context/grant及精确page再核；未确认无Hoststatus/body，原currentepoch关联与clue旧epoch分列。
- only三个NEW生产子文件及必要Handler/Bridge接线、三个组件TEST与独立actualEdgeTEST，唯一公共OpenAPI/原SQLschema/codec/原发送/事务/调查firstend及依赖不变。组件验证真实Store U/Q、完整facts/owner/分页篡改/跨身份/代次/limit/整库后页损坏不跳过、cold map空与无任何重发；真实Run/Connection/Edge原未确认线索、正常cold新客户端/list/原key查询及显式reconcile、closed ErrorView/用户显式重开新epoch、无权过滤/peer/源和Q守恒。实际UI为synthetic外部包非React，原容量/故障证据按源/路径集合/镜像影响链复用，不重跑两个46分钟。正反控制必须真实合法cursor替坏MAC被原拒绝oracle击穿，以及私有路径观察被decodedscanner拒。

- 原全schema前置技术反馈：当前新列表首稿只调原 `facts`，其既有定义不校 `sqlite_schema`，不能代本节明确要求的 schema＋完整facts。原生产未修时同原正常Store增加一张未知schema表，U/Q与MAC不变，实际Go用例击穿“changed original schema escaped full clue validation before authorization”；private红记录保留。仅在新 `clueCandidates` 原Store.mu内、原facts和任何候选SELECT/Host可见集合之前调现有 `s.schema(ctx)`，任何schema错误沿同facts分支保原cause并fence，不改原公共facts/DDL/原发送权。对原16FR/7SC的影响复核仅补已批准前置，意图/依赖/权限/宪章不变，当前源码仍须按影响重新format和验证。

- [R52当前完整证据](<research.md>)已以780冻结输入、2026-10-26 13:07:14–13:11:57 +08、十六actual步骤十四0/两个有效expected1验本片：normal/tag三十browser/native亲本各三、旧native三父各三、有限桥新两clue及旧查询/选择/text、四journal父各三和两vet过。原schema漏验先真实Go红再同原锁补，TEST后页损坏长度一字节被原CHECK正确拒后仅改保长度翻MAC，失败/C/日志保留。真实Host/Connection/原UQ/currentgrant+visible、actualEdge两代各两精确页/原Cookie-CSRF、旧cursor409而原handle可调查不可确认、一次原keyGET/显式一次reconcile、closed原ErrorView/完整源FileID守恒/用户显式新epoch、peer200及原所有owner knownend均实际。正反控分别原合法cursor实际200/原item击穿坏MAC403，以及私有路径观察真实decodedscanner拒。原R51/R49/R47/R44–45/R42影响按完整input/pathset/镜像链核，不重跑容量；[只读当前交接](<../../.data/logs/003-validation/renderer-clues-434-handoff-current-results.json>)绑定全源/两TEST/十二原与冷EdgePID-birth-image/真实schema红和当前Doc3，CGO0 race skipped。
- 该源链加原Context/exit、Text/ACK、六query与三selection覆盖冻结十六动作，原T016/V009–V010工程收口。原T015其它完整结果/accepted调查、真正React/外仓许可、完整十轮及全故障联合产品仍后续；不由本地clue或syntheticJS扩大权限，不将schema单位当native故障，也不把当前工程收口叫完整003/发布。

原则I–VII/INV1–21：Host唯一授权/业务结果；原Store唯一线索owner/真实firstend/unknown保留，cursor不是权限，scope明确且不全局current，旧线索仅调查。授权内容不被诊断清洗，日志有界无秘密，固定有限分页/全facts不删验证。原设计映射和R52运行证据分列，原T016据声明范围已验，不倒签后置产品。

### 4.41 T015明确拒绝结果的页面交付及未决线索保留（implemented / R53陈旧基线拒绝局部已验，原T015仍partial）

Owner B/A串行，原T015/V008、FR008/011–015/018/020–021/024十项及SC004/007/010/012结果与交付局部，依T013/T014/R44–R52。最小补原保存确认之外的完整Host拒绝结果；不把完整传输误当保存，也不因显示accepted/unknown而删除FR015要求的未确认线索。

- 唯一可信来源仍原TextSession原key的那一次POST及原generated commitRecord响应。已匹配200或202 committed+known+receipt保原SaveConfirmed和现有proof；额外只允许原同base冲突形态HTTP409、严格原ErrorView.code=STALE_BASELINE、outcome=rejected、retryPolicy=newPreview、scope缺省或workspace、可选operationId若有须匹配原key，作为“原陈旧基线拒绝、非保存确认”的可交付结果。独立原resource/content及整份Q metadata关联仍核，不从renderer提供result/receipt/key，不缓存原结果正文。原Host返回的其它合法ErrorView仍照常显示，不因本地不可退休而改变其语义。
- 原源码反馈：commit.go:236–249在真实保存后若授权交付失败会抑制receipt，原failure仍可返回FORBIDDEN/rejected/none。因此“HTTP完整且rejected”不证明业务未提交，不能普遍退休；本片仅上项原STALE_BASELINE/newPreview，其它FORBIDDEN等完整拒绝照原typed结果显示、仍保Q/key调查，不新增Host语义或宣告回滚。202仅accepted、unknown/undecided持久性、investigate重试策略、原key不匹配或截断/错配receipt、不完整传输均不得发可删除Q的deliveryId；保原typed完整结果/SaveConfirmed=false或未确认无状态正文、Q及原key可调查。拒绝结果的ACK仅退本地原Q，不声明回滚、安全重试或赋予重新保存权；同原handle的原Attempt已烧，不恢复U/新key。这样的保守未决保留不增加原Host重试/后置React门。
- 新proof继续每结果独立状态和原page pointer/双代次、原owner/rowhash/一次source；收到响应不自动UI或DELETE。当前可信页面单独展示明确拒绝（不显示保存确认），另DOM提交后再独立ACK；全部原scope/CSRF/cookie/current/id检查先于mark，known原事务DELETE才retire、unknown仍保原pending/物理排他且不重复DELETE/save。已知保存确认不因proof/清理失败倒退，旧R45真实DELETE first-end与old-next证据按完整源码影响复用。
- 影响只原delivery/textsource、有限桥proof资格判断及必要NEW组件/实际Host-Edge TEST；原SQLschema/codec/生成OpenAPI/Host/权限/依赖及旧完整结果expected不改。真实同base候选竞争：原Host另一确认先成功使本原key返回明确409拒绝，实际页面核原ErrorView后显示→独立UI提交→ACK，Q仅最后退，原正式winnerreceipt/正文/key及peer不变；synthetic外部JS不代React/发布。组件同时覆盖accepted/unknown/缺失或错配receipt无proof/Q保、正确confirmed结果不降级、wrong ACK不消费。
- 必须同source冻结normal/tag新组件与实际Run/原Connection/Edge、旧journal/有限桥/Host影响及vet；实际两个观察控制仅把坏deliveryId换回原合法ID使原DELETE known成功后击穿拒绝，以及synthetic应用UI门提前使未提交拒绝DOM时原ACK真删Q后击穿UI顺序；其它accepted/unknown/generic拒绝保留由当前组件正反核，不把未接入full的accepted替stale变量称已执行。原R42容量不重新跑，源/完整路径集合/镜像/原R52及旧故障链必须核。任何发现的机制缺陷先实际红再定点修；未观察状态不补签。原T015其它恢复/真实React/联合场景仍据原任务，不以此局部提前勾全任务。

- [R53/435当前证据](<research.md>)已按783冻结输入、2026-10-26 13:44:14–13:49:08 +08、十六actual步骤十四0/两个有效expected1补验本片。normal/tag三十一browser/native亲本各三、原native三父及全部桥/当前Text、交付与原Consume/DELETE真实1034选集各三和两vet通过；真实Host先原winner200后原renderer409STALE_BASELINE，拒绝显示不保存/Q1、另DOM提交+rAF仍Q1、第三独立ACK才Q0/N不退，原winner全文/receipt/key及peer200不变、全部原owner knownfirstend/coldQ0。六个actualEdge原PID-birth-image及普通/诊断TEST、原R52所有源/路径集合/R44–45R47–49和R42容量链在[只读交接](<../../.data/logs/003-validation/renderer-rejection-435-handoff-current-results.json>)核，run后只有Doc3变化，CGO0 race skipped。页面是synthetic外部JS非React/外仓许可，ACK为可信页UI声明非native独立DOM证明；泛拒绝可能抑制已保存receipt，仍保Q不重发。原T015其它恢复交付及完整产品后续，不提前勾原T015或整003。

原则I–VII/INV1–21：唯一Host结果/DB及原key，完整类型与保存确认分层、未决线索不丢弃；原current范围/权限及精确文档复查、不借cookie或receipt授权；先原Q后一次POST、显示/UI/ACK分离、未知不重试/退款且实际物理结束沿R43/R45；有限4096证明和既有体积界/脱敏，旧失败和并行工作保留。无第二schema/结果库/发送端，无跨仓/下载/ACL策略/提交push发布。设计静态核对不等于实施或运行，原任务同步及严格只读分析后才代码。

### 4.42 T015原Q正常冷恢复的当前页结果再交付（implemented / R54原T015工程收口，非完整产品）

Owner B/A串行，原T015/V008，FR008/011–015/018/020–021/024十项与SC004/007/010/012恢复交付局部，依T013/T014/R44–R53。原R52调查只读结果、不mintproof，冷Q无法沿热Preview originals ACK；本片仅补原任务的恢复交付，不恢复保存权、不改原Host/SQL/codec/公开API。

- 原Q的完整MAC/绑定包含原安装/tenant/主体/workspace/op/candidate/旧session和epoch，但没有resource/content明文，不能从同一receipt反填并冒称独立重算原preview。只在原InvestigateClue当前身份/页面/原handle核验、固定原key GET或显式一次reconcile并得到真实200 OperationView时，额外认可save/committed/known/非nilReceipt、外层与receipt的workspace/op/candidate均匹配原Q为恢复交付来源。resource/content来自当前authenticated原Host原key权威；旧绑定字节不变、旧会话/epoch仅历史，不授权当前页。该私有来源分支排他，不落入原POST专属stale拒绝资格，也不修改现场200Receipt/202OperationView matcher。accepted/unknown/404/closed/genericFORBIDDEN/错配/不完整仍原typed显示或未确认，无proof/Q保留。
- 复用原Store.delivery完整Q metadata/resultHash/随机ID/每结果UI状态和原独立DELETE。TextSession另有界恢复交付上下文（最多4096）绑定原handle、新实际TextPage指针、当前完整五scope及那一份proof.page；不写originals、issued、Attempt、U、N或新业务key。ACK先当前身份/页面/新scope，然后仍原proof/page/owner/id/row校验，已知DELETE后才退；候选过期不加清理门。current方法对epoch只是关联校验，不声称这里独立查询Host当前epoch；实际原key权限仍原Host。已有同handle proof继续拒，不以Current=false猜永久退休/回收未知；正常已知Store结束后cold map空是本片首保证，未签活proof替换优化。来源/ID创建失败不得覆盖热originals或旧合法ACK。
- 原investigateClue可带optional deliveryId，只上述成功原OperationView和正确scope可有；local list仍三字段无Host结果/ID，调查不返回confirmationHandle/新保存许可。独立ackDelivery仍当前可信页的UI已提交声明，native不独立证明DOM。可信页面核原操作/receipt后显示→另DOM提交跨frame→第三ACK，调查收到/HTTP完成不自动UI/DELETE；retired本地结果不宣称新的Host保存或重试权。未知保留/oldproof不复活沿既有原语，不新增receipt正文存储。
- 实际测试：原Host保存后尚无UI/ACK的Q，正常Bridge/Store/Connection已知结束后新nativeprofile/session/client/document，原Q查询权经当前Host重验，原key调查真实receipt→Q仍1→另UI仍1→独立ACK/Q0；旧页/旧scope/错ID/跨页拒且不mark，原保存POST计数不增、原正文/key/receipt及peer不变，caller各物理owner已知结束/cold无旧Q。组件补Q限定/完整currentOperation/错candidate/accepted等无proof、existingproof不被替换、raw绑定不改/Confirm不能由恢复上下文准入。两个有效控制仅合法ACK替错ID、synthetic应用UI门提前使真实Q0再击穿原拒绝/UI顺序，不改产品guard。synthetic不是React/外仓权，实际DELETE故障/R42容量按原源链复用不重复长测，不加native故障全排列。
- 影响仅新增私有恢复来源、原调查/ACK和有限桥ID投影及最小旧阶段TEST预期更新，生产/单位/实际Run-Edge normal/tag与影响回归、全源与路径集合/原R53链通过才签；本片现由[R54](<research.md>)及[当前只读交接](<../../.data/logs/003-validation/renderer-recovered-436-handoff-current-results.json>)按786冻结输入、2026-10-26 15:08:02–15:15:37 +08、十六actual步骤十四0/两个有效expected1补验，normal/tag三十二browser/native父各三、原桥/恢复三组件/原Text/交付及Consume-DELETE1034影响和两vet过。六次两代actualEdge同原独立image/版本、真正原Server/Browser/IO/Store/Connection首次结束后cold新身份，默认原key GET200+完整原OperationView/receipt、N2U1Q1显示和独立UI、ACK才N2U1Q0，原formal POST事件正控1且不增，oldscope/handle确认不恢复发送，原formal正文/key/receipt及peer200/所有knownend和最终cold守恒。两个合法ID/提前合成UI门控制都actualQ0击穿oracle；synthetic声明非nativeDOM/React证明，组件显式reconcile路径不冒称renderer本轮执行。旧锚空白/原missing-key404误猜/Attempt目录保护拒绝各失败保留，产品未放宽，run后onlyDoc3；固定CGO0 race skipped。原T015据R44–R45/R49/R53–R54覆盖原完整结果交接/精确UIACK/DELETE未知调查与cold恢复后工程勾选，后置React/用户最终验收/完整产品联合不代签。原则I–VII/INV1–21：原Host/DB/current授权唯一、状态与保存权分层、原Q后独立UIACK/真实首次结束不洗白、有限对象与脱敏、无第二schema/权限/结果库，不跨仓下载/改策略/提交发布。该静态设计不是实现或验收，原任务同步与严格只读分析后才代码。

### 4.43 T023当前Host影响域及同镜像独立客户端回归首片（implemented / R55局部已验，不代表全部V005）

Owner A，原T023/V005首片，依T008/T009/T010/T011已验。承接FR001–003/005–009/011/018–021/024–026共16项的受影响回归及SC001–004/009–012局部；不是这些需求的最终产品验收。当前T018真实React输入/来源许可/notices和本仓独占整合路径未提供，保留unverified，不用现有synthetic字符串包/CC0标签代React或跨仓授权；此缺口不阻止ready T023后端首片。

- 验证边界为当前源冻结的八个Host影响域：`./internal/installation/...`、`./internal/host/...`、`./internal/transport/http/...`、`./internal/protocol/...`、`./internal/digest/...`、`./internal/workspace/...`、`./internal/platform/...`、`./cmd/...`，normal及acceptance-tag全选测试和两次vet；**不称全仓 `./...` 通过**。clientjournal完整4096父无skip，normal/tag已各实际46分钟；原prototype还有大回收/最大metadata，不能把它们装入默认1200秒或short-fill冒通过。本片不改旧检查定义/生产/契约/依赖，也不将当前未复验的journal/browser组合代签；其既有证据只在源/定义/环境精确匹配后分层引用。
- 原唯一OpenAPI生成器 `--check`、原OpenAPI/字节两个独立Python与Go消费者、TS raw113/字节45正83反和副本漂移按现成入口当前只读执行。生成/格式化不在验证中隐式写源；Node运行不是缺失tsc静态检查。固定Go1.27.2/windows-amd64/CGO0/SQLite3.53.4及原compat指纹，race明确skipped，不下载编译器/依赖或改驱动。
- 同一全部非忽略源/契约/GoTS/构建输入及精确runner冻结到私有C，离线缓存/原`tools/go.ps1`串行使用兼容准备锁，构normal Host、normal client、tag Host、normal SQLite TEST与installation TEST五镜像，记录源/依赖/profile/nm和每镜像SHA。五镜像均在该C `.data/logs/001-validation/`下，验收TEST从对应包CWD执行，原根与PID/birth/image/原管道/actualWait/首次结束门不放宽。
- 当前同五镜像运行旧001既定21父，设置原四镜像变量和 `ABOARD_ACCEPTANCE_EXPIRY_WAIT=1`，40分钟整体超时、原真实900秒/14–16分钟断言、每个父真实RUN/PASS零skip；不是用旧R119或package条件skip代当前。再设置独立client镜像执行三父 `TestV005IndependentNativeClientTrustedConnection`、`TestV005IndependentExplicitClientServe`、`TestV011IndependentTextCLIOriginalPublicChain`，each once，逐父原正常排空/重开和公共链不改。
- 不拼接单位为维护/sharedunknown的独立新client通过：现client issuer cutoff/pending pipe/revoke单位是真执行，但升级原Store.mu+pending拒绝本身不证明独立Host的维护前后新bootstrap签发/ACK/撤销竞争；原V205真安装1034没有新ClientListen/ac1组合，当前unknown手置predicate也不代native。原T023保持partial，下一最小用例按真实同Host维护或共享1034补这两差集，不新增后置前端或故障全排列门。
- 全检查实际命令/耗时/退出、选中父及条件skip、原rawlog、所有原始输入和两完整路径集合、五镜像/工具/compat/依赖SHA前后核；失败保留，不修改expected/重复启动旧长测。独立输出只在ignored `.data/logs/`，成功后Doc3更新及只读交接核only允许状态文档增量，不能把本片green勾T023/T018或宣告React/发布。

原则I–VII/INV1–21静态核对：I/INV01–05保原Host/DB/身份和唯一OpenAPI且只读契约；II/INV06–08同源真实子进程/原SID-PID-birth-image与native排他、不自动初始化或迁入旧真实库；III/INV09–14原candidate/key/typed结果/unknown及实际IO排空断言不改，skip与未验独立记；IV当前权限/私有管道/TLS/secret负例与原lease-firstend门保持，维护组合尚未运行不自签；V安装/工作区/runtime与逻辑容量分层，原4096不缩或错误超时，物理ENOSPC/峰值deferred不因本片增减；VI实际源/依赖/compat/镜像profile和合成NTFS/当前浏览器环境可追溯，无下载/系统策略/跨仓；VII真实生产前端/许可与用户最终验收/发布分开，T018材料缺失保留，任务/分析门完成前不实施。这里是设计，不是运行证据。

R55实际执行结果及证据摘要见[research R55](<research.md#r55--原t023受影响host与同镜像独立消费者局部implemented--非完整v005>)；上文检查定义保留，未完成的maintenance/sharedunknown组合不由该首片代签。

### 4.44 T023同Host新client与原安装FSYNC unknown的最小组合（implemented / R56局部验证）

Owner A串行集成，依T008–T011及R55已核源码/工具/镜像基线。原T023/V005、FR002/006/012/014/017/018/025，SC009–012的共享围栏局部。新增[共享unknown验收](<../../tests/acceptance/client_shared_unknown_windows_test.go>)，复用现有finalizeFixture/readFinalizeEvidence/joinOriginalSessionDelivery及nativeEntryRun的实际子进程/可信配置/连接；仅另在[旧可信连接验收](<../../tests/acceptance/client_native_entry_windows_test.go>)补下述既有就绪前置。产品/原故障guard不改，不引入React、schema或维护平台。

受影响旧验收夹具修正（2026-10-26，implemented / R56局部验证）：同源normal客户端在第二Host的stale配置拒绝后立刻执行fresh正控两次失败（首批一次、3次有界复验其中一次），旧失败保留。只读定位发现单实例pipe、拒绝后50ms冷却、接收端仅一次open，已有peer的HTTP200不是pipe就绪屏障；原生231未捕获，因此不宣称已证实产品根因。仅在`TestV005IndependentNativeClientTrustedConnection`各个NEW bootstrap/child之前复用既有`explicitServePipeReady`（WaitNamedPipeW，5秒上限，不open、不发送、不重试交换）明确验收前置，正反断言不变；不修改产品接收端或掩盖busy行为。重新编译normal/tag验收镜像、该旧父两mode各3次及其余两受影响旧父各1次、新父各1次，独立记录真实退出及每步状态。仍需另行观察产品首次open Stage/Native，不能以夹具绿证明产品竞态已修。

技术路径修正（2026-10-26，implemented / R56局部验证）：原checkedExecutable只允许该测试源码根的001诊断目录；creationProbeRoot又使用tag Host编译源的runtime.Caller根，因此不能把c437的tag故障镜像直接用于新源码根的fixture，更不放宽guard或伪造源路径。normal Host/client/两helper仅复制已核原字节到本根新诊断目录并核SHA；只有tag Host从相同未变生产源码、原工具/依赖/profile在本根重建，记录新SHA/profile与实际受影响用例。R55原结果仍属原镜像，不称新镜像重跑旧21父。generated build.mod只允许经原consumer bytes核对的一处绝对replace路径重定位，其余compat输入精确不变。此前准备阶段失败保留，不算产品正反控。

- 同一tag Host启ClientListen，完成ac1 peer真实bootstrap/ACK与原pipe交付join；同trusted config真实独立normal client inspect先成功且peer仍可用。无pending ACK或live workspace owner时，原workspace-creations的原key/input触发原creationFinalize FlushFileBuffers故障，必须观察原PID/key/Injected=1/NativeBOOL=0/SQLite1034/Unknown/NativeControlFailed/CallbackDrained及原HTTP503 installation unknown，而非手置authUncertain。
- 同peer的`/v1/client-context`、`/v1/visible-workspaces`及self-end均须完整503 typed installation unknown，不返回ended=true；真实独立normal inspect精确exit1、空stdout和固定安全诊断。每次调用及首次self-end在原owner先root/latch。以另一fresh合成fixture的ordinal0观察原create201、peer和独立inspect成功及known self-end/Host正常结束，反证环境本身不可用。若原物理lease仍在必须实际sharing violation；unknown只按原精确child异常收集/Wait，明确不是正常drain或退款，不删journal/换库/重发。
- focused selector先对实际test exe `-test.list`精确命中父名，再真实RUN/PASS、无selected SKIP/empty，原始故障标记和进程身份落日志；compile、tag-test及受影响acceptance vet按实际执行记。复用R55 normal Host/client/两helper字节及重建本根tag Host前核生产输入/依赖/工具指纹，只有新验收源和状态文档差异；新tag记录实际profile和受影响回归，不因源根重定位或文档变化重复旧900秒/4096长测。新检查器采用合成负例验证自身，不替代原native用例。
- 安装维护签发/ACK/撤销竞争仍是原T023未验差集，本片不将mutex串行或分散组件叙事代替该竞争。真正React输入、具体跨仓路径和写许可仍缺，当前只做独立本仓增量。

宪章I–VII：复用唯一Host/DB/当前身份/原key；所有数据新建合成；不重放unknown或以missing当回滚；原物理首结束/借用所有权与异常收集分开；不改64/4096/预算；沿原受保护pipe/TLS和共享围栏；真实独立normal消费者与tag故障Host组合分列，不签React/发布。计划/任务完成后只读分析再实现；此段不是运行结果。

### 4.45 T023原在线升级与在途client ACK/撤销互斥（implemented / R57工程范围已验）

Owner A串行集成；依T008–T011/R55/R56，映射原T023/V005、US1、FR001–002/006–008/018–021/025–026和SC001–002/009–010/012的维护差集。仅新建 `tests/acceptance/client_maintenance_windows_test.go`，复用 `schemaUpgradeLegacySeed`、原normal Host、installation TEST helper和nativeEntryRun/production peer；新TEST raw hello6 peer只控制原管道ACK阶段，不是第二发行/授权owner。原normal/tag工具、profile及生产源保持，复用核准Host/client/SQLite镜像；installation TEST helper的既有runtime.Caller守卫要求fixture与产物落在其编译源码根，原c437 helper不能用于本根schema1 seed，因此仅从未变源码在本根重建normal helper，置于原001诊断目录并记录新SHA/profile，不放宽守卫或改写旧证据。不下载/改迁移/改已批准spec或活动locator。

源码证据纠正：`schema_upgrade.go`原升级全程持Store.mu，pendingDeliveries非0即拒绝；不存在“已进入升级事务同时有在途ACK”的生产时序，也没有可逆在线维护截止状态。不为验收引入该状态；原停机workspaceStopping不能冒充可继续服务的在线维护。这里仅验真实可达的维护拒绝、原producer结束及前后会话隔离，不降低原V005，也不新增推进前置。

- 每个fresh schema1合成安装的原TEST producer必须known退出后才启真实Host/ClientListen；已ACK的production ac1 peer和独立normal inspect先成功。原operator用公开preview/原key构造原schema1→2升级，不自动关闭任何工作区。
- raw TEST peer在原single-instance pipe请求hello6，收到完整有界reply后停在ACK2前，原升级必须409 RECOVERY_REQUIRED、原key404，已有peer仍200；真正发2且读到3后停在ACK4前，升级仍409。操作必须在原15秒Serve期限内完成，不靠sleep；任何提前断连/超时都失败，不能当竞争正控。
- 正常分支发4；撤销分支在已收到3后实际断开原pipe。记录原first Close结果且未知不重试；都必须等待同一管道的下一次Serve就绪（不能以ACK4 write/HTTP200或单独disconnect代替finish）。撤销分支用绑定同Host/pin/TLS1.3、期限与证书IP/EKU的有限一次性TEST TLS GET观测原token403，不能拿cutoff或超时当撤销；不打印私有reply、token或路径。生产peer始终原session/context不变。再以未消费的同preview/原key显式升级200，核原keyknown/drained及peer仍200；新独立inspect正常、显式self-end及正常Host stop/Wait。
- 实际test exe先-list精确父名，再normal/tag focused各1次、零selected SKIP；原normal independent trusted父受影响回归一次、acceptance normal/tag vet。以同build提前完成ACK的观察控制击穿“hold窗口升级应409”断言，保留失败，不修改产品或expected。输入/镜像/工具全量指纹与源集合前后核，新增源/授权记录及状态文档分列；R55旧900秒、R42/其他4096长测没有受影响，不重复。

宪章I：仍唯一Store/DB/原key；II：fresh合成schema1、无repair/补种真实安装；III：合法preview原确认，409不称回滚/无任何DB写；IV：ACK/本地close/producer finish/正常Host Wait分层；V：已有TTL/容量/身份不改，TEST secret零公开；VI：原mutex维护拒绝及原peer隔离，未知first-end不重试；VII：真实Host/native消费者与TEST原始观察者标明，非React/联合产品/发布。只读分析通过后实施。[R57](<research.md#r57--原t023维护ack撤销差集完成2026-10-26>)已以787输入、11实际步骤验证：normal/tag各1次父的completed/revoked均过、同build提前ACK实际升级200击穿409 oracle、原可信连接与V207混合升级影响父各1、两验收vet通过；输入/镜像/工具无漂移。原helper编译源码根不符的首批失败保留，仅重建未变normal helper并核profile，不改生产守卫或广回归。R55–R57共同收口原T023工程，不签React/G306/完整003或已deferred原生故障。

### 4.46 H01受信静态目录装载与固定URL初开（implemented，R58本片已验）

Owner A；沿原T018/G306早期交接、T013可信Package及FR001–003/009/018/020–026、SC009–012局部。仅本仓设计/实现/合成验证，不写Workbench，不扩大H02导航重登记/H03实际React等待与context/H04可见入口。详细技术切口只在本节维护。

- 新增可信native `StartPackagedBridge(ctx,c,cfg,input)` API。input含固定本地绝对clean目录、有限显式assets（既有URL path、规范MIME、预期size/SHA256）、Source/License标签及独立固定content/provenance指纹。expected不能仅从待载目录自报；不新增JSON manifest、业务DTO、扫描发现、MIME猜测、任意URL/文件代理或CLI命令。入口恒为 `/app/index.html`，不收起始URL。
- 用os.OpenRoot约束原根，只读既有staticPath接受的显式regular文件；拒reparse根/路径、大小/SHA错配，读前限128项/单2MiB/总8MiB。未声明文件不访问/发布，不walk。文件/root在原BridgeSession的loader槽保留，每个first Close独立latch；未知不重试、不丢owner/退槽。文件/root全known结束后才可启动browser。
- 复用唯一 `clientbridge.New` 的owned copy/verifyPackage及同一Handler。新增只读值类型PackageIdentity（Source、License、ContentFingerprint、ProvenanceFingerprint），不导出bytes/map。原StartBridge的hooks+New移到实际listener authority取得后、browser工厂前；documents未就绪的hook fail-closed，Serve仍在browser/documents/envelope完成后才启。新入口比较同一immutable快照与独立expected，错配不得调用browser工厂，不验证一个副本却启动另一个mutable Package；旧StartBridge沿同一接线但不新增expected要求。
- 新入口在原bridgeSessions/root64下装载并固定OpenPage；任何load/start/OpenPage失败返回同一partial owner，不自动重开/补票/结束借用Connection或Journal/stop Host。Close先cutoff并确认loader槽first-end，再沿原Serve实际join→listener→TextSession→browser原进程/reader/writers；loader未知保根，重复查询只返回原first-end结果而不重调native Close。
- 保留Headless=true、Windows/amd64及已验installed Edge组合。初页不等于导航/刷新重登记、React join/context、可见入口、支持矩阵或T018/G306通过。工程指纹不混作Host内容指纹；标签及hash不冒充法律许可，标签到来源/notices的关联需可信记录冻结，不编造真实provenance。
- 有界验证：loader合成正反（path/MIME/size/hash/count/未声明不发布、修改原文件不改owned bytes）；独立固定清单下只读核真实交付；错误SHA/expected而合法上下文时browser工厂调用0及profile未创建；获准本仓真实Host+Connection+原Edge的固定精确document URL、静态响应/指纹、known物理结束及peer保活。真实React未join只签初页不签业务。partial/first-end/借用owner影响回归，normal/tag新父及相关vet；先实际-list精确命中、零selected SKIP、原始命令/退出/输入SHA和失败保留。不重复无影响30秒TTL/16/32容量/4096journal长测，不改CSP、业务契约、权限、旧expected或批准spec/locator。

静态宪章核对：唯一Host/DB/身份及immutable静态owner不变；浏览器输入不能选根/expected；秘密留native/精确document，静态能力不授业务权限；unknown first-end保物理根/借用关系。任务/只读分析完成后实施。[R58](<research.md#r58--h01受信装载与固定url初开implemented非react验收2026-10-27>)以15实际步骤验证本片：normal/tag五loader父及真实Host/Connection/原Edge固定初页、三种坏包先拒后启、同build有效identity观察控、原StartBridge/owner与静态影响父及两vet；实际Workbench三asset只读装载，源/镜像/工具/浏览器无漂移。原缺显式browser输入的SKIP报告保留、不计通过；本片不是实际React运行、导航重登记、可见入口或T018/G306验收，实际运行provenance关联仍需冻结。

### 4.47 A003来源/notices可信接收与H03最小bootstrap交接（implemented，R59接收局部已验，运行消费者未验）

Owner A负责Host可信记录/只读装载验证/接口，Workbench owner在自身仓承担本次明确获准的WB008最小切片；沿T018/G306、US1/US6、FR003–005/019–025、SC009–012，不新建路线或loader。

- 固定既有A003原交付，不把旧静态壳改称新bootstrap消费者。可信Source=`aboard-workbench-a003-handoff-eb3bebb866e7b6dbeb6722e4548fcf91ad47a10bd872b4593983fa336a9b83a1`；License=`notices-58ecedca09b6fbebbfba323e16c813f48648eb953ca4bd52db157c5ee84bedc2-rights-unaccepted`。标签不是SPDX或发行许可；`rights-unaccepted`保留明确未放行。独立content=`60a39ce445854c8f8933562f0a6ead8f07418aa28b86df1cac5a1332aced6f5b`，依原算法的provenance=`f5920413cce27a52ba5d5c8a60ca4eb1df32748626d0b0f7d02397fe64a00cac`。唯一expected来自Host接收记录，不从WB目录读候选manifest自证。
- Source中的raw handoff SHA关联其9旁件（build报告、inventory、notices、lock、补充source/license等）及build报告14声明输入、3实际assets；License中的完整notices SHA关联128包inventory段落。沿原审计器独立重核全部有限bytes/size/SHA和声明关联，记录工程指纹`e742c113233f360b73824dde58a554f35aba9058f48ebac601c09213dcb208d0`。这证明接收时所列输入/关联，不证明穷尽输入、重现构建或法律完整。2026-10-27首次严格重核发现旧`host-contract-snapshot.json`的3份治理文档和H01改过的`static.go`/`bridge_windows.go`共5项已陈旧，原失败保留；接收记录单列该历史快照`current=false`，不删其检查、不改旧expected。当前生产输入另与已验R58逐项绑定，公共契约/生成绑定必须仍无漂移；只据此接收未变A003 bytes/notices及交接当前wire，不将旧Host快照称当前。Workbench下一次新构建/交接须刷新相关快照。
- 新增沿原opt-in机制的[有限接收test](<../../internal/clientnative/workbench_source_windows_test.go>)，保留R58旧readonly test不改：固定上述标签及expected，用既有loader和同一Handler实际观察identity；冻结的9旁件与raw manifest在装载前以固定预期核验，不执行Workbench代码、构建、Host或浏览器。不将sidecar加入/app；通用生产loader仍只读显式assets，不声称它每次启动实时核notices。合成的来源/notice负例证明旁件错配在有限接收核验拒绝、标签改变保旧expected拒绝；不改旧expected、静态白名单或CSP。已有H01的安全/first-end/工厂前拒绝证据按未变生产源码复用，不重跑长测。
- H03冻结既有wire事实：顶层当前document仅注入own属性`window.__aboardJoin`（token、clientGeneration、documentGeneration）；没有native ready事件/Promise/subscribe/剩余TTL字段。票自native issue起30秒，不从renderer读取起算。Workbench在React树外持唯一in-flight/私有页binding，采用本地有界等待（前端实现自行记录具体界，不把旧合成6秒参数当产品SLA），StrictMode/多订阅者不得重复join。先缺失为等待，非法/超时/已消费/过期/传输不明明确未就绪，不自动重join、换票或reload重认证。
- 精确相对POST `/bridge/v1/join`，`credentials:'same-origin'`、`Content-Type:application/json`，body仅原3材料字段。浏览器负责Origin/Fetch Metadata/HttpOnly cookie，renderer不设置Authorization/Host/Cookie。200验证version=1、pageId UUID、csrf 64小写hex、双代次与原材料一致；秘密只留私有闭包，不落DOM/日志/存储/URL；本地快照后清window副本不赋予重发权。
- 只读context为相对POST `/bridge/v1/actions`，固定`X-Aboard-CSRF`；body `{version:1,requestId,pageId,clientGeneration,documentGeneration,action:'context',input:{}}`，不带scope。完整结果须version/requestId/action、kind=host、transport=complete、hostStatus=200及原生成ClientContextView严格校验；HTTP200/transport=unconfirmed不是空身份或成功。提交UI前再校验原捕获页与当前单例关系。公共DTO/runtime/digest/compat只接收Host原生成物，不造第二业务规范或renderer直连Host。
- 新消费者必须由Workbench owner按本次授权实施并重新构建/交接；新raw manifest/源/asset/content/notices任一变更均重新冻结Source/License/expected，旧A003收据不能签新包。新包可做一次已授权Headless合成安装、原Connection/原Edge、固定URL→真实React初登/context→正常物理结束及peer保活的局部联调；此前不执行旧壳来冒充接通，不签H02导航、可见产品、业务保存、完整T018/G306或发布。

静态宪章I/II：不新增业务owner/安装修复；III/IV：只读context/一票一次，超时不算撤销或物理结束；V/VI：秘密留native/页私有、精确同源/代次及原容量不变；VII：区分真实bytes只读核验、既有合成wire证据和未验真实React。已先tasks/只读分析，再由[R59](<research.md#r59--a003接收时来源notices冻结与bootstrap交接2026-10-27>)实际验证7步：normal/tag两精确父各1及逐成员SHA/notices size/标签旧expected负例、两build/两vet/version，selected零SKIP与源/镜像/工具无漂移。只签接收关联/同Handler identity，未启动Workbench/Host/browser；旧5项快照current=false严格失败保留，运行wire及原生成保真解码/授权范围见[最小交接回执](<../../.data/logs/003-validation/a003-source-notices-20261027/bootstrap-join-handoff.md>)。后续等Workbench owner新包，不以旧A003代签。

### 4.48 A005-r2接收与一次真实React只读组合（implemented / R60接收已验，真实探针failed）

沿原T018/V014、US1/US6、FR003–005/019–025及SC009–012局部，仅Owner A写Host；Workbench仍只读，不修改原生成物、桥/DTO、生产代码、CSP、旧expected或A003记录。

- 两阶段接收：只认用户指定r2 raw SHA `a269c1b1a6529372109c07e3634184fe9ca0c359f4b1462769d227db9394385a`，不读候选handoff-check作为权威。独立逐字核14固定旁车、22声明build输入、36源指纹、原Host 5生成/8receipt输入、精确3资产327669字节及128包/135notice文本关联。Host快照实为33普通输入+1既有交接记录=34，全部核验，不删项凑33；接收时差异为0。来源/构建关联不证明穷尽输入、可重现构建或发行权。
- 新Source=`aboard-workbench-a005-r2-handoff-a269c1b1a6529372109c07e3634184fe9ca0c359f4b1462769d227db9394385a`；License=`notices-e914cdf541fa9f4371cc18dbb744fa42cba389a0a346792cdec5264fd862c111-rights-unaccepted`；独立content=`eb33f4f599ad5614c6f3cbc9820021db185099c99b7f20b97f9635aad6666b36`，provenance=`0553e47decdea1a31d0254a21516be5401f8872fc2d578bfd033a8e076d5ad69`。只将既有dist三项以明确Path/MIME/size/SHA交原trusted loader，旁车/notices/生成源/仓根不发布。新增TEST硬编码Host已核期望，不在运行时采信候选manifest；来源标签保留rights-unaccepted。
- 先新增有限只读接收父（normal/tag）验证原loader→同Handler identity及原source handles known结束，成员SHA/size或Source/License保旧expected错配真实拒绝；继承未改H01安全/first-end证据，不重跑全部原生容量/TTL/长测。raw/14旁车及外仓声明源前后独立SHA核验；新增本仓TEST和本次auth/plan/tasks/research差异单独绑定，接收时current=true不泛化到新运行截面。
- 只运行一次normal模式真实组合：复用原合成Host/原production Connection两独立会话/已有受信Edge及生产StartPackagedBridge。固定初页必须精确 `/app/index.html`，且Handler identity等于本次独立expected。真实未改React自身等待原native材料，调用原join及context；CDP只读取原unique document的DOM/有限performance元数据和既有native标识，不代发请求、改fetch/票/DOM，不读写私有闭包或注入应用代码。
- 观测以白名单状态/计数为主：真实React达到ready、原own材料已清除、安全摘要计数与当前原Connection一致；该document实际resource请求中原join=1、actions=1，无其它业务/API/外源请求，静态JS/CSS来自本次白名单。性能条目不含POST正文，不将计数单独冒称严格DTO证明；严格解码链须与冻结源码/原生成runtime及真实ready共同关联。只观察固定时间窗，不将其证明为无限期无重试。
- 正常收尾沿原first-end：原Server/Serve/listener及原browser process/reader/all-writers known结束，registry不留已known根；借用main及独立peer仍current，再由caller结束原Connection，原Host实际Wait。只记录布尔/阶段/计数/镜像SHA，不输出token/csrf/pageId/原生定位值；非法/缺失/拒绝/unconfirmed/旧document不得由probe改成ready，前端本地负例记录与本次真实正控分列，未运行真实浏览器负例不得补签。
- 有界构建/验收：原offline Go1.27.2/windows-amd64/CGO0及compat，normal/tag两TEST、精确-list、RUN/PASS零selected SKIP、两本包vet；实际React组合只normal一次，不重复装配tag、不重跑900秒/4096/16或32容量/30秒TTL。冻结源集合/工具/浏览器/复用helper及原日志，任何未声明漂移先拒绝，原失败保留，不修改前端凑绿。

[R60实际结果](<research.md#r60--a005-r2接收已验单次真实react探针failed2026-10-27>)：接收审计零差异；normal/tag有限只读父及各四种错配负例、两TEST构建/两本包vet通过。一次真实normal父到ready、join/context各1且均HTTP200、固定URL/材料删除/DOM秘密布尔均过，但首个资源及摘要复合断言记录resources=5/other=1而failed。未存额外条目的脱敏类别，不能先归因favicon/预加载或产品问题；350ms第二观测、独立摘要精确一致确认、专用正常收尾及peer保持未到达。原Cleanup返回且无追加错误，不冒称这些未到达断言已验。原失败、治理Doc3预检失败与补充vet分别保留，未重跑第二次真实探针、未放宽断言。

静态宪章核对：原Host/DB/授权owner与秘密、first-end及借用关系不变；React只是普通只读消费者；新标签/哈希不授发行权。按本节tasks及只读一致性分析后实施，接收审计不是运行通过，单次context组合也不是T018/G306/其余15动作或完整003验收。

### 4.49 R60测试专用有限脱敏补观测（implemented / R61静态合成已验；R62单次真实补观测failed并已停）

沿T018/V014、US1/US6、FR019–025及SC009–012的失败可诊断性，授权只引用001唯一记录；Owner A独占Host TEST与本次证据，Workbench只读，不改任何产品或冻结输入。

- 原 `a005Observation`、`a005ObserveExpression`、`a005AssertReady`、ready轮询及 `first != second` 条件和350ms timer原样保留；JS/CSS/summaryItems及capabilities/profiles匹配布尔仅取已有核心字段/原Connection预期，不补猜原失败缺项。新增独立诊断结构不进入两次核心比较。
- 在first ready返回之后、首个原断言之前，仅一次独立只读performance诊断，派生2秒context、不重试，失败非Fatal且继续全部原断言；context超时不等于浏览器Runtime.evaluate已被强制中断或原IO已结束，结束仍依原owner链。顺序只分类前min(N,32)条，输出其中前4个other；只投影同源/query/hash/credentials布尔、封闭path/initiator/reason及归一HTTP整数/-1，显式标记扫描/输出截断。沿用原精确路径与initiator、同源及无query/hash/凭据判定，不扩大允许规则；URL/错误/路径原文/身份/响应体/DOM不返回。getEntriesByType仍取得浏览器整表，32限制的是后续分类，不谎称浏览器取表内存有界。
- 诊断是first之后的独立快照，不代表first同时刻，也不重写first.Other；原350ms timer仍在该诊断之后，因此first→second实际间隔会增加诊断等待（使用2秒context，非硬终止保证）及调度耗时，不能称总间隔仅350ms。全probe最多一次诊断/32扫描/4详情，第二观测只补已有核心计数日志。
- Go端严格校验必填/非null/类型/未知或重复字段、枚举、HTTP与32/4数量关系及4KiB结果界；不可用仅输出固定原因，不打印解码/CDP原错误，不把零值或空样本冒充收到。成功日志去掉原PID/birth与任意版本原文，仅保留既有断言及安全布尔/受信摘要；这不是删除验证。
- 实施文件仅现有 `internal/clientnative/workbench_a005_windows_test.go` 的日志/诊断调用，新增 `workbench_a005_diagnostics_windows_test.go` 和纯合成验证 `workbench_a005_diagnostics_validation_windows_test.go`；不触碰冻结r2、raw/旧expected、产品bridge/CSP或原资源规则。Node标准库VM评估实际诊断表达式的合成performance表；Go仅选择新纯诊断父，normal/tag构建、精确-list与本包vet均不得选真实/原接收父，也不运行Workbench脚本/工具下载。
- 合成覆盖31/32/33、4/5 other、第33条才异常、精确路径错initiator/错源端口/query/hash/userinfo、invalid/throwing URL条目及秘密输入、缺失/边界/非整数HTTP；Go覆盖缺字段/null/未知或重复字段/坏枚举/超长、CDP错误/超时及诊断可用/不可用均不改核心值。固定原核心块SHA、原失败及全部外仓r2输入；新TEST单列于R60后的Host差异，旧receiving current不继承到新构建。
- 仅生成后续待确认的单次normal只读补观测方案，绑定同一r2与原合成Host/helper/Edge、原新TEST/源码/工具及输出新目录；本轮实际执行次数必须为0，不启动真实Host/native/browser，不以静态/合成结果签React恢复、T018/G306或发行。原failed/rights-unaccepted保留。

宪章I/II/III：无业务状态或正式owner变更；IV：first-end/原断言保持且诊断失败不冒充结束；V/VI：秘密不出边界、工作有界、不新增权限/路由；VII：合成只证诊断，不代真实浏览器。先tasks与只读一致性分析，再实施。[R61静态/合成证据](<../../.data/logs/003-validation/a005-r2-diagnostics/static-synthetic/checked.json>)已完成8步：62个Node VM用例、normal/tag构建、两mode各2个纯父/132个子case及两vet，原核心4块SHA及比较/timer、外仓r2/旧失败/locator无漂移。仅诊断TEST已验；没有执行真实/接收父或第二次真实Host/native/browser，旧R60仍failed，额外条目未获实际分类。交叉字段仅诊断投影，不作为新的安全断言或资源允许规则。

R62状态（2026-10-28，非新增设计或再次运行许可）：用户随后确认上述同r2单次normal方案，原预构建TEST/Headless合成Host与Connection/Edge、原父count1实际只运行一次，3.07秒/exit1、零重试/零SKIP/未超时。首ready仍join/context各1且200、JS/CSS各1、summary3与能力/profile匹配，但resources5/other1使原首断言failed。其后的独立诊断（在原Fatal前）扫描5/输出1：同源、无query/hash/凭据、root-other/other、path-not-allowlisted/HTTP404；不是确切路径或来源，不能反推旧R60。第二观测/专用正常结束/peer保持断言未到达，不签恢复、T018/G306或发行。新r2/旧expected/规则/产品/前端与TEST完全不改，运行前后源/工具/冻结输入及旧失败一致；失败即停，现已停，后续诊断/修复/探针须另行确认。[R62结果及交接](<../../.data/logs/003-validation/a005-r2-diagnostics/real-once-after-confirmation/frontend-handoff.md>)保留原R60 failed与R61当次零真实运行历史，不将运行后Doc3同步冒充运行输入。

### 4.50 R63 图标候选与原始 initiator 投影（implemented；仅 TEST 与纯合成已验）

- 范围：原 T018/V014 下补充 R62 只读归因缺口。既有 `root-other`/initiator `other` 不能证明根目录图标或原始 initiator 字面值；不据此修复或放宽产品。仅改 `workbench_a005_diagnostics_windows_test.go` 及其纯验证 TEST，不改核心 `a005Observation`、资源表达式、ready 断言、first/second 比较和冻结 r2。
- 诊断新增封闭 `iconClass`：仅解析后的规范 `/favicon.ico` 路径标 `default-icon-path`，其它可解析路径 `other-path`，无法解析/读取 `unavailable`；此值不证明是浏览器自动请求。新增 `initiatorLiteralOther` 布尔，只有原始值严格等于字面 `other` 才为真；不输出原始字符串。沿用 32 扫描/4 条、4096 字节、既有 reason/status/布尔字段边界，不把候选或类别变为资源白名单。
- 顺序与 owner：当前主会话独占接口、Go TEST 与文档；独立代理只改新忽略目录中的 Node 合成脚本。先完整任务与只读 analyze，再实现。验证原 initiator 已知/未知/不可读、规范图标与相似/编码/跨源/query/hash/credentials/无效 URL、越界、脱敏及未知/缺失/null/错误类型/literal-other 与 initiator 枚举不一致拒绝，核心四块逐字节不变。
- [R63 纯验证](<../../.data/logs/003-validation/a005-r2-diagnostic-classes/static-synthetic-fixture-fixed/checked.json>)实际8步：Node 138（完整保留旧62并新增76）、normal/tag各两个纯诊断父共148子case（135验证+13收集）、两构建/两vet，零SKIP；58项旧失败/r2/核心文件保护、原四块/locator/兼容输入无漂移。前置回执字段引用错误与新增合成夹具切片别名失败均保留，修的是验证接线/夹具，不是原断言。未调用真实/接收父或Host/native/browser；R60/R62仍failed，不据合成签实际图标或发起来源。
- V014 本轮仅纯 Node 合成、两模式测试编译/指定纯 Go 父测试与 vet（复用已缓存工具与 compatibility，不下载）；不得运行真实父测试、Host/native/Edge 或任何旧真实 runner。历史 R60/R62 和 r2/receiving 冻结前后不变；实际补观测仍未执行，必须另获明确授权。编译不代运行，合成不代真实归因；不勾 T018/G306。宪章沿 §4.49 的测试专用/同一 owner/无权限扩张证据，分层验证不重复业务长测。

R64状态（2026-10-28，执行记录，不新增修复许可）：用户明确允许启动先前因日志目录受阻而未运行的新一次normal观测。本会话既有权限下原新目录创建成功，未调整任何系统权限；795项、固定Workbench50/Edge、R63 normal镜像及原核心前后核一致，仅唯一授权记录精确新增段。实际原父count1、5.42秒/exit1、零SKIP/零重试/未超时；ready与join/context各1且200、JS/CSS各1/summary3/能力和profile匹配，仍resources5/other1。独立快照扫描5/输出1实际命中`default-icon-path`与`initiatorLiteralOther=true`、同源/无query/hash/credentials、HTTP404、原path-not-allowlisted；不能据此证明浏览器自动发起，也不反推R60/R62。原首断言仍failed，350ms第二观测/专用正常结束/peer断言未到达；产品/TEST/前端/r2/规则/expected不改，无重建或追加组合。[R64原结果](<../../.data/logs/003-validation/a005-r2-diagnostic-classes/real-once-r63-after-confirmation/results.json>)及日志保留，预算已消耗；当前停止，不勾T018/G306，不自动修复或再运行。

## 4.51 R64资源假设纠正与连续修复（implemented，R65受影响范围已验，2026-10-28）

R64已证明same-origin、无query/hash/credentials的规范默认图标路径、原始initiator=`other`及404，但不能证明请求自动发起。原“总资源必须恰好四条”把浏览器惯例资源与业务资源混为一谈，导致join/context成功后即拒绝、正常结束断言无法到达。按统一授权继续修复，不新增单次审批门；原失败不改写。

- Host私有静态Handler对精确GET/HEAD `/favicon.ico`提供空204响应，沿原authority及无query/RawPath/fragment校验；无磁盘查找、重定向、cookie/CORS、HTML回退、身份或Host调用。它是空的传输惯例应答，不是包新增图标资产，原三文件/Source/License/content/provenance保持。其它路径、方法及业务/API拒绝行为不放宽。
- TEST将该精确路径单独计数，只允许同源/无query/hash/credentials、原始initiator=`other`且HTTP严格204的0或1条；总资源必须等于原四条加该计数。join/context各1、JS/CSS各1、other0、32上限、所有身份/秘密/物理结束/peer门和first-second原值比较保持，不把未知资源统称图标或直接忽略。重复图标、错路径/initiator/status/跨源/query等仍失败；新规则覆盖真实新修复，不反写R60/R62/R64 expected。
- Owner A串行集成。T018/V014对应FR-003–005/022/024–025、US1/US6；先HTTP边界及实际JS表达式合成正反，再normal真实React闭环，发现新问题继续修复，随后tag/受影响Handler与native-owner回归。使用原冻结r2和已有工具/兼容输入，不重跑无影响存储容量长测；每次命令保留独立日志及实际退出，选择器精确且SKIP不算通过。[R65实际14步](<../../.data/logs/003-validation/favicon-fix/native-inputs-fixed/checked.json>)已完成：Node实际表达式78正反例、Handler全27父（新增34个HTTP边界子case）、normal/tag各3个纯/接收父与真实React父及6个受影响native-owner父、3构建/2vet/version。两个真实React父均到达350ms第二快照与原正常结束/peer断言；resources5、icon1/HTTP204、other0、join/context各1且200，原r2及受保护输入无漂移。早期两次driver环境错误保留（尚未启动真实Host/浏览器），修正后连续复验，不新增许可门。只完成当前接缝，完整T018/G306/浏览器负例与业务UI/发行仍未签。
- 宪章I：无业务写入/新事实owner；II：无文件访问、初始化或修复数据；III：不改保存/receipt；IV：正常原IO结束与独立peer断言保持；V：原authority、授权/秘密与容量不变、空应答无能力；VI：图标请求不转成共享故障；VII：Host仍独立、冻结React包不改、不用模拟成功。现有002 locator不改，仍需当前增量spec/plan/tasks只读一致性分析后实施。

## 4.52 T018剩余静态交接与Workbench owner新业务包衔接（文档交接，不是联合验收，2026-10-28）

沿原T018/V014、US1/US2/US6与既定§4.4接口整理[单一交接索引](<workbench-handoff.md>)，不新增协议、业务需求、逐次运行审批或竞争任务表。唯一当前状态仍在tasks；实际实施只在各自仓既有授权与独占路径内。本仓串行写此索引和plan/tasks/research，不写Workbench或重冻r2。

- 已有context/visibleWorkspaces/openPrecondition/openWorkspace/workspaceStatus/listRecords/getRecord不只列allowlist，原native实际接Context/Query/Selection；下一最小业务片可先消费它们，不强依赖Text/journal或新API。显式open取得epoch，不能称整片无副作用；record GET仅前后status观察拒迟到，不是原子快照。桥拒绝/transport与原Host结果、完整五维scope/大整数/十进制字符串均依原生成绑定和wire，不建第二DTO。
- T018工程已完成的接收/真实只读范围与完整产品严格分列。H02刷新/导航及H04可见入口仍未完成：当前native拒绝Headless=false、CLI无生产Workbench launcher。TEST镜像仅作其历史证据，不冒充独立产品入口；不把全部剩余工作转嫁Workbench。
- Workbench owner先在自身唯一授权/任务确认下一业务片与精确独占文件，再回传新版本raw/旁车、源/锁/构建输入、完整assets/来源/notices/权利状态、消费Host指纹与实际动作/验证边界。原候选目录只列待确认项，不把Host文档当跨仓写许可；本次不声称已联系该owner、收到回执或核当前外仓字节。
- 新包到达后Host独立核关联与有限字节，重冻新来源/许可/内容及当前输入，按实际新动作补精确联合父/构建/环境/超时与原owner known-end/peer验证；旧r2/bootstrap父不能签新业务。保留旧raw/expected/失败，普通相称修复复验沿现有授权连续推进；只有真实权限、必要输入或未决产品选择才需介入。
- 静态核对：不改变Host单事实owner/用途CAS/保存receipt/生命周期/秘密与总容量/共享围栏/独立消费者，不动002 locator、数据库、公共契约或生成物；不因静态交接勾T018/G306/T019–T022或签法律/用户验收。[R66机器包](<../../.data/logs/003-validation/t018-owner-handoff-20261028/owner-packet.json>)仅证明本仓精确输入/绑定及历史引用保持，没有新构建、测试或Host/native/browser执行。

</details>

## 6. 宪章逐原则检查（设计静态核对）

| 原则 | 设计依据与未验证门 |
| --- | --- |
| I 单事实owner/身份 | §2/3保留Host业务写者与显式三重身份；线索不存业务终态；越权待实测 |
| II inspect无副作用/拒绝补种 | §3.1仅serve既有安装，排除create/register/repair；损坏拒绝，真实负例待测 |
| III 提交/候选诚实 | §3.4先预览后确认、原key调查及202 receipt解释，不读历史正文；发送围栏/unknown待证 |
| IV 生命周期分离 | §3.1/3.4页面/退出不取消accepted，close遵循原owner排空；pending与join待测 |
| V 当前授权/秘密/总容量 | §3.2–3.4秘密留原生，重验授权，有限线索不复制Host执行容量；连接/持久性尚未证明 |
| VI 隔离与共享影响 | §5真实A/B/无权及共享围栏，不承诺恶意同SID沙箱、硬断电或无限可用性 |
| VII 独立引擎/真实客户端 | §2独立Host、React+CLI同公共协议；生产包/许可/双消费者真实验收未执行 |

未发现需要修改宪章的设计选择；静态一致性不是技术门pass。外仓许可/静态产物及安全机制未验证前不得发布。

## 6.1 T018/V014 当前独立接收准备及 H02/H04 增量（2026-10-29，partial implemented；静态/退休组件已验，真实刷新门未过）

沿唯一授权与原T018/V014推进，不新增竞争任务，不重开已完成T023。当前唯一写者为本会话A；Workbench及旧根只读，002 locator保留，003始终显式指定。三历史缺项仅作有界原件取证，不重建旧packet或替换冻结包expected；新当前清单须独立schema、完整union delta及逐项SHA/bytes，旧797未知bytes保持null。此前800清单是当时快照，不自动继承给本次源码/治理。

**第一片（可独立实施）**：在新的Host `.data/logs/003-validation/t018-reception-prep-20261029/`内建立有界只读取证工具与合成自测；复用已审静态inventory边界，只读取显式三个Workbench冻结包的raw清单及被列举旁车/资源（路径、reparse、重复、大小、SHA门），不运行外仓产包/构建/脚本。原旧797与上次800分别对当前全部union逐项delta。记录包三资源的sorted length-prefix triples投影、旁车匹配/缺项、7绑定与39引用的当前差异；包自报标签/投影不成为trusted expected。对路径逃逸、重复、错SHA/bytes、额外磁盘资源、前后漂移用合成fixture拒绝。新报告仅接收准备，不签来源/许可关联、loader接受、兼容、React联合或发行。新产包consumer映射只阻该分支，已有包仍可独立检查。

**H02设计边界（依第一片及当前只读门；完整重登记尚未实施，退休首片实际结果见下）**：复用原browser document pump，由BridgeSession持有有界串行重登记worker及合并通知；pump锁内不回调issue，先精确退休旧代次/handler准入，再对受信静态入口、fresh原unique/context与当前Host身份重验，一新精确document只issue一次。复用target槽并移除可信已退休document的handler页，避免顺序刷新耗尽16活页；TryLock busy/Current=false/未知RPC不构成退休证明，8在途actions/pending/unknown不退额。新页独立pageId/CSRF，不转旧票、不旋转同client cookie，B页不受A刷新影响。same-document history/hash变更当前仍退休并unavailable，不把单次bootstrap当可再消费；真正新document reload/受信导航才重登记。该选择不放宽未重新登记的拒绝要求，也不声称SPA导航体验已完成。

H02还须串行处理同桥receipt交付后未ACK刷新：只在native不可逆证明旧document退休且Store pending/DELETE unknown/first-end门允许时退休其**内存**delivery proof与热page关联，保留Q/原key/N/U/Q/MAC和原Attempt发送latch。新页当前认证+原keyGET完整匹配结果后才能mint新proof，经新UI提交及独立ACK正常DELETE；旧ACK永拒。不能凭刷新删Q、重POST或把旧身份当授权。Owner A拟独占：`internal/clientnative/bridge_windows.go`、`browser_documents_windows.go`、`bridge_text_windows.go`，`internal/clientbridge/handler.go`，`internal/clientjournal/delivery_windows.go`、`recovered_delivery_windows.go`、`text_source_windows.go`、`text_delivery_windows.go`及各自新导航测试/registration子文件；实施前再列精确新增文件并重核无人写入，不交叉委派。

**H02首实施子片：退休发布门（2026-10-29，implemented / component-tested；真实reload未过）**：先只接native已知退休→Handler发布/回收，不提前启动重登记worker或触及journal。新增`internal/clientbridge/document_lifetime.go`：可信Go owner持每代独立`*DocumentLifetime`不可逆latch；`Retire`只锁该latch、不回调native/handler、不持锁等待RPC。`PageBinding.Lifetime`可选（旧测试/注入nil不作为退休证明），生产native每次snapshot首次生成Generation同时生成latch，已知retire先置退休后清代次；未知RPC/busy不退休。Handler在join容量判断前仅扫已证latch退休页，final join/deliver在h.mu→lifetime锁内核验并发布；退休不能让晚join重新发布，原joining/8请求/unknown占用不退。锁序d.mu→lifetime、h.mu→lifetime，lifetime锁内绝不取d.mu或执行Current/Host调用。首片不称完整H02：尚缺串行重登记、native target槽回用及Q-proof恢复，旧Q与发送latch完全不动。

本子片A本会话精确独占既有`internal/clientbridge/handler.go`、`internal/clientbridge/handler_windows_test.go`（仅旧unkeyed PageBinding改keyed，不改断言）、`internal/clientnative/browser_documents_windows.go`、`internal/clientnative/bridge_windows.go`；新增`internal/clientbridge/document_lifetime.go`、`internal/clientbridge/document_lifetime_windows_test.go`、`internal/clientnative/browser_retirement_windows_test.go`；治理仅plan/tasks/workbench-handoff与本仓新`.data/logs/003-validation/h02-retirement-20261029/`。未列核心/Workbench/生成物/锁/locator全不改。先当前只读analyze再实施，normal/tag新父+旧Handler影响父/native工程父与两包vet/实际测试exe指纹，合成测试不能代真实刷新/联合。

**H04设计边界（不依赖Workbench重新产包，但实际产品包须先独立接收）**：同时解除原native bridge和browser process的Headless=false拒绝，复用原private pipe、显式继承、实际PID/birth/image/CDP版本、fresh profile和原owner首次结束；不启debug TCP或日常profile。拟A串行独占上述bridge、`internal/clientnative/browser_process_windows.go`、`cmd/aboard-client/main_windows.go`、NEW `cmd/aboard-client/launch_windows.go`及相应新测试。launcher只组合原OpenConnection/StartPackagedBridge/原journal，固定/app/index.html，可信native配置提供冻结Source/License和独立Expected，不从待验包提取expected、不接受任意JS/URL/命令。显式exit需Server/listener、worker、Process及全部IO实际join；unknown first-end保原根，不第二Close。真正可见受管窗口与正常launcher必须实际观察，不以进程启动、headless或TEST exe代签H04。

**映射与验证要求（实际已执行范围单列下文；不整体签通过）**：T018/V014、H02兼原T012；US1/US4/US5/US6；H02覆盖FR002–005/008/012–015/018/020–025、SC003/007/009–011局部；H04覆盖FR001–005/017–026、SC008–012局部。先工具合成正反/完整delta与保护指纹，再按影响normal/tag原Handler/journal/native owner。H02真实同Handler双页A刷新>16轮而B保活、16并存第17拒、旧票/page/CSRF/ACK拒、未知不remint、原key恢复Q保持到新UI+ACK且POST总1；H04真实headful窗口/目标关联、坏browser/profile/package/Host拒、argv与秘密边界、显式退出/独立peer正常结束。当前组合/合成权限/生命周期门未核齐不联合，不复用旧driver或单次预算，不无变化重跑、不放宽断言。

**接收准备首片实际结果（2026-10-29，implemented / static-only）**：[新工具/报告](<../../.data/logs/003-validation/t018-reception-prep-20261029/README.md>)沿原T018/V014完成800项当前输入及对797/上轮800逐项双delta、三历史缺项边界和Workbench三冻结包字节闭包；24项合成工具测试通过，三包各3asset与78/95/105旁车、精确83/100/110文件、source/build字节关联及Host content独立核对。每包39引用31同/5文档变/3缺，producer当前性不整体判绿；未创建trustedExpected、不签来源/许可/loader/真实联合或发行。当前plan/tasks/handoff回写另列delta并重采，旧证据不覆盖。H02/H04设计及运行仍unverified，下一片分别核当前实施门，不以consumer旧schema映射全局停工。

**H02退休首片实际结果（2026-10-29，partial）**：[独占证据与剩余门](<../../.data/logs/003-validation/h02-retirement-20261029/README.md>)记录先plan/tasks/当前只读analyze再实施的精确七源码文件，以及基线/实际四TEST镜像/原日志。native每新document独立不可逆lifetime；已知retire先置latch再清代次与票；Handler只凭正向退休证据回收页槽，final join/deliver与Retire同锁串行，nil/Current=false/busy/unknown不回收，不改joining/requests返额。normal/tag全部Handler父、新退休组件和受影响原native owner/pipe/边界父及两包vet均通过，零选中SKIP；合成40轮不是40轮真实Bridge刷新，不代签worker、Q-proof恢复、完整H02或联合。

真实reload父在首轮即因runner使用非规范Windows executable路径失败（0秒，未启动浏览器）；原failed日志保留。runner路径已改规范，另行只读发现当前安装Edge为156.0.4314.50，SHA256=`f87d927bed3597f19c70af67bc786dd22e4eca3b5c7cc050336a016bb42f7fda`，而生产支持门仍严格只允Edg/156.0.4314.42。未用同一失败输入重跑、未放宽版本断言、未把metadata当实际CDP证明，也没有声称修正后真实父已运行。当前真实浏览器组合待受支持版本/新版本独立技术资格门，不能拿R65的旧Edge截面补签。完整H02串行重登记与同桥proof退休仍待实现；H04受信包Source/License/Expected及正常入口/实际可见与生命周期门仍待。只停相应真实浏览器/包分支，不回滚本次静态准备或组件证据。

宪章逐项：I沿唯一Host/显式身份；II静态准备无业务副作用、launcher不init/repair；III候选与完整结果/UI/ACK分开、unknown保Q；IV正常关页不取消accepted、原owner actual join；V当前认证/秘密原生/既有16与8总量门不逃额；VI保留dirty、只本仓写、真实负例与共享影响分层；VII原Host+独立CLI/Workbench边界与许可/发行未验诚实记录。此设计不改spec/宪章/契约/生成绑定/锁/授权；实现前须对当前spec/plan/tasks只读analyze，后续每片按实际输入重验。

## 6.2 本批交接机器输入、浏览器资格拆门及H02首片补证（2026-10-29，implemented / static-and-component-tested；真实资格未执行）

沿原T018/V014及H02，不扩规格、不重开T023。A独占治理仅本plan、tasks、workbench-handoff、唯一authorization，证据仅新`.data/logs/003-validation/t018-machine-handoff-20261029/`。机器导出复用已核audit-post-h02，不重采并冒充803：manifest保留`aboard-host-static-current-inputs/v1`；packet=`aboard-host-selected-owner-packet/v1`，delta=`aboard-host-selected-delta/v1`，三者以snapshot_id、manifest_sha256和精确文件SHA/bytes绑定原audit、原800基线、审过的inventory helper及完整797/800逐项差异。required_inputs只是消费入口索引，全部manifest仍binding。首版导出helper缺path的失败/原产物保留，v2消费位置另立，不覆盖。不创建trustedExpected。提出90分钟静态工具适配窗口，待Workbench owner确认；可以并行Host工作，但产包/接收前重核选定三件和全部live inventory，非零漂移须显式重选/重绑或评审固定源归档，不声称旧截面current，不静默混用；过期不自动延长。最终完整live drift另记，不固定项目数。

浏览器：仅有界只读查安装根/注册路径及已有缓存候选，版本/文件SHA不是实际CDP支持证明；现有严格Edg/156.0.4314.42不改、原失败不覆写。未找到合规.42则列.50独立精确组合资格方案，分开静态metadata、隔离的精确候选技术验证与正式生产支持门；本批不执行受阻真实分支，不下载/替换/降级/关更新/改系统策略。Source/License/provenance/trustedExpected按独立技术事实、需要包owner补充的可核输入、需权利人/用户发行决策逐项分类；技术接收不等于最终发行批准。

**唯一代码改动为测试补证**：A独占既有`internal/clientbridge/document_lifetime_windows_test.go`，新增`TestHandlerRetirementSerializesFinalJoin`覆盖§6.1既有h.mu→lifetime发布序列。用原production Handler和合成trusted binding/受控ResponseWriter，join已进入WriteHeader后原latch必须实际被持有；另一goroutine Retire不得穿过发布，释放后join/retire两worker真正join。原已开始join必须完整200与原pageId/CSRF/generation，不能伪称撤回；之后旧page/context拒403且Host dispatch不增，新代join给新page/CSRF/代次并仅回收旧槽，独立B继续200。不新增生产机制、不触native/journal/worker/target复用/Q-proof。复用Once-release与有界超时保护，不t.Parallel；超时/失败真实记录，不弱化断言。实施前对当前spec/plan/tasks/宪章只读analyze；相称验证normal/tag完整clientbridge父（零selected SKIP）、该包vet、镜像/源码指纹，不选真实reload、Workbench/React父或无关容量长测。映射T018/H02/V014、US1/US4/US6、FR002–005/008/012/018/020/022/024–025与SC003/007/009–011局部；full H02/Q-proof/H04和真实验收独立未完。

**本批实际结果**：[机器交接与检查证据](<../../.data/logs/003-validation/t018-machine-handoff-20261029/README.md>)记录选定803 manifest/packet/双delta、16项绑定正反通过及另列最终live drift；没有覆盖旧输入或expected。完成本批当前只读analyze后新增纯合成final-join对称测试，normal/tag全部clientbridge各34父通过、零selected SKIP、两构建/两vet及源码镜像指纹守恒；只补首片证据，不冒充真实刷新或完整H02。常规安装/注册范围未找到合格.42，.50独立资格仅方案、严.42未改、真实浏览器分支未运行。Source/License/provenance/Expected可由Host独立技术核后建立，现有owner声明/notices不全缺，语义接收仍待；最终发行权另列而非泛停技术接收。完整重登记/Q-proof/H04/真实验收分别未完。后续新技术切口另核必要设计/tasks/analyze，不借本批门跨越。

## 6.3 固定consumer样本后的WB036技术接收（2026-10-29，designed；执行结果另记）

沿T018/V014与既有§6.1/6.2，不修改spec、契约、锁、生产loader或单一权限owner。selected-post-h02-v2三件/字段语义冻结为Workbench工具适配样本，原窗口保留历史且不续期；不因适配另冻live或开倒计时。独立接收已有WB036-r3时以其精确包闭包和当前Host39引用/完整漂移单列，不宣称旧包天然当前/兼容；未来真实重产选择待Workbench selected预检通过再协调。

A精确独占：新`.data/logs/003-validation/wb036-technical-reception-20261029/`（本仓stdlib工具/合成自测/独立技术关联/新expected/执行日志），新`internal/clientnative/packaged_workbench_reception_windows_test.go`，以及本plan、tasks、workbench-handoff和唯一authorization；Workbench只读。映射US1/US6、FR003–005/019–025、SC009–012局部，依T010–T013/T018字节闭包准备；不以本片满足完整T018、H02/H04或最终发行。

1. 原raw/check/105旁车/65source/37build inputs/3assets与实际110文件全部重新核对；用原独立算法重算content，比较候选值仅作旁证。原39项Host引用逐项对当前，七契约/绑定逐字节和生产wire另核；三历史缺项标有界旧证据而不恢复，治理/测试/native差异解释接收范围，不整体判兼容。
2. 来源以现有owner独立编写/no-copy/阶段交付声明和捕获source-build闭包关联。lock精确解析193registry/integrity，128installed集合、其余平台差集、直接依赖精确匹配。notices逐包/逐文件核唯一段集合、Source/Integrity/SPDX/file名、完整文本与inventory raw SHA关联；生成器trimEnd导致末尾信息丢失时，仅允许预列有限后缀候选以SHA精确恢复且记录实际匹配，不以SPDX/非空代原文；saxes补件直验、rolldown借用原文/lock关系单列证据强度。Host generated provenance尾段另核原generated receipt与本仓七项、scoped transfer声明，不当第129个npm包。源作者/registry tarball未联网复验、superset非runtime闭包/build可复现及最终rights未放行诚实列限制。
3. 工具先合成正反例验证缺段/重复/错Source/Integrity/文本/SHA/直接依赖/lock/借用/Host notice都拒，再对原包执行独立语义核验。通过后由Host技术owner在新目录冻结Source（关联raw完整SHA）/License（关联notices完整SHA且明确rights-unaccepted）、重算content/provenance与独立trusted-input/expected，精确引用语义报告；不从Root自读expected、不覆盖旧expected，不新增每次启动notice准入owner。
4. 新normal/tag测试只读独立可信记录（编译常量固定记录SHA），调用原loadTrustedPackage→原Handler，比较独立expected、三静态实际bytes/MIME/CSP、不可变owned copy/未知资源拒绝及原file/root首次known end。原startBridge-before-factory序列用合成Host当前原Connection/独立peer及不启动browser的sentinel factory观察：正控到厂1但明确非bridge启动成功；错成员/SHA/Source/License/content/provenance在factory前拒0；原partial session保持并known Close、借用连接/peer保持。不执行受阻.50或React。相称normal/tag四构建、selected零SKIP、原TrustedPackageLoader/Input/首次unknown影响父、两包vet；不重跑无关存储容量。所有临时合成数据仅本仓.data/logs，日志记录源码/工具/TEST镜像/包前后SHA与实际父清单。

宪章I显式身份唯一Host；II只读包、合成Host不init真实安装；III–V无业务提交/ACK/真实secret、原生命周期unknown不洗白且不增owner；VI先设计/tasks/只读analyze、实际正反分层、只本仓写；VII内部技术接收与外仓owner/最终rights分开。实施前核本节当前analyze；后续.50隔离TEST切口须单独具体设计/tasks/analyze，不用本片接收门越权开生产browser。

## 6.4 精确.50候选资格切口（2026-10-29，implemented / candidate-qualified；不代生产/联合）

前置§6.3已实测：WB036原loader/Handler及before-factory正控1/九负例0，normal/tag每mode七native父与全部34Handler父、四构建/两vet通过；三asset不变、原文件/partial/合成Host首次结束及Run join已知。证据为新`wb036-technical-reception-20261029`，技术关联不消除Host39引用漂移，不签发行或联合。固定consumer三件与旧窗口不改、不另冻live。

本片沿T003/T018/V014及原§6.2候选→精确支持→生产复验顺序。A独占精确源码：`internal/clientnative/browser_process_windows.go`、`internal/clientnative/browser_documents_windows.go`（仅私有构造准入接缝及物理结束已探测标记）；`internal/clientnative/browser_documents_windows_test.go`、`internal/clientnative/browser_admission_windows_test.go`、`internal/clientnative/browser_cookie_edge_windows_test.go`、`internal/clientnative/browser_retirement_windows_test.go`（把既有断言主体参数化，原Test入口仍用原生产构造）；新`internal/clientnative/browser_candidate_windows_test.go`（候选proof只在TEST闭包中）。治理仅本plan/tasks/workbench-handoff/唯一authorization；新证据仅`.data/logs/003-validation/browser-candidate50-20261029/`。原production支持predicate仍严格`Edg/156.0.4314.42`；不引入用户可控选项、第二进程/文档/资源owner、环境放行或公开路由。

- 原`startBrowserProcessWithTransport`继续以nil候选接缝调用同一物理实现；只有TEST构造传独立有限候选检查。它在持有原image文件并重算SHA后、mkdir/管道/CreateProcess前核预冻精确image SHA，实际CDP回报后再核精确product/PID/birth/image。CreateProcess、精简环境、private pipe、显式三继承handle、实际image/PID/birth、sole browser holder、无DevToolsActivePort、所有上限/超时/unknown/根保留均原样。候选成功仍`Verified=false`；仅物理探测已知标记允许同一原Close请求Browser.close并join，绝不授业务准入。
- 原`newBrowserDocuments`仍仅接受生产Verified；私有共享构造可供TEST传候选proof核验闭包，闭包只接受本次原start已完成全部物理探测的精确指针、digest/product/PID/birth且Verified=false；不得把getVersion/metadata或一张任意bool当准入。仍同一个b.documentsOwner/原pump/原cutoff/首结束，不伪造Verified，不重置未知，不把候选接入Bridge/Host生产业务。
- 不另写弱版矩阵：既有exact-document/one-join、client-envelope/pages、held-original-ACK unknown、16页/17拒绝无target、实际30s到期与新client代次、真实reload/lifetime/B保活断言主体复用；只有start/doc构造依赖参数化。原TEST入口维持生产构造，候选入口另名且输出明确candidate-not-production。32票场景中的31项仍明确合成计数，不伪称真实unknown投递。cleanup只读取既有首次结果，不对已attempted原owner再调用Close，更不retry未知。正控与错误product/image、原生产构造不接候选/第二documents owner拒绝分别证明，不增加断言容忍、deadline或capacity。
- normal/tag先编译与非browser owner/pipe/document影响父、原loader/Handler回归；预冻测试镜像、完整相关源码、支持predicate、准确路径/PE metadata/imageSHA、命令、精简child环境约束/实参及无相关活进程。然后normal/tag各运行选定候选矩阵一次（不是次数许可），零selected SKIP，实际CDP.50、读/写/进程/pump首次结束和join逐父记录。任何失败保留并诊断，有改变才相称复验；不得自动转生产通过或借旧R65补签。
- 候选全部门通过才另在同一原`browserSupportedProduct`加精确`.50`，不得改为prefix/range或信任任意Chrome；先增量plan/tasks/analyze并冻结确切predicate测试文件及生产复验清单。候选证据固定在支持增量前源码/镜像组合；其后真实生产原构造、完整受影响矩阵及WB036原包集成分开验证，失败不签生产。支持增量和H02串行worker/Q-proof/H04/headful/React/联合/发行均非本候选PASS的推论。

映射US1/US4/US6、FR003–005/018–025、SC003/007/009–012局部。宪章I–V不增加业务权力、工作区、真实数据或owner；VI先具体设计/tasks/current只读analyze再改源码，VII不把内部技术资格当外仓验收/发行。不开第二server、不下载/替换/降级/关更新或改策略，002 locator及T023保持。

## 6.5 候选已证后的精确生产支持增量（2026-10-29，implemented / scoped-production-tested；非完整联合）

§6.4原组合已在新`browser-candidate50-20261029/candidate-01`完成normal/tag：每mode原六矩阵子项全部PASS/零selected SKIP，包含真实30s到期/16页17拒/held-original-ACK unknown/一票一join/新client代次/导航退休B保活。实际CDP严格`Edg/156.0.4314.50`、image SHA `f87d927bed3597f19c70af67bc786dd22e4eca3b5c7cc050336a016bb42f7fda`、PID/birth/image/sole-holder/原首次结束及process/RPC/pump join均逐项保留；错误SHA/product拒绝、候选始终Verified=false。候选前后源码/镜像/工具不变；四构建、normal/tag原owner/loader影响父、全部34Handler父及两vet已过。旧首轮失败不改，不能由候选通过直接签生产。

本片A独占精确源码：只修改`internal/clientnative/browser_process_windows.go`的原`browserSupportedProduct`，从严格`.42`增加OR严格`.50`；只修改`internal/clientnative/browser_process_windows_test.go`的精确支持单测（保留`.42`正控、加`.50`正控，保留其余负控并加相邻版本/Chrome.50/前后缀/空白拒绝）；新增`internal/clientnative/workbench_wb036_production_windows_test.go`，复用已独立冻结的`wb036TrustedInput`、原`StartPackagedBridge`/原物理browser及真实合成Host。仅治理plan/tasks/workbench-handoff可更新；新忽略证据仅`.data/logs/003-validation/browser-production50-20261029/`。不再改候选接缝/旧矩阵主体/loader/journal/Bridge机制，候选闭包严查本组合现在被生产接纳时不会混作候选PASS。

当前只读analyze后才修改以上三源码。normal/tag先四构建、两vet、所有受影响owner/原pipe/原loader及34Handler父；随后原生产Test入口完整exact-doc/cookie/held-ACK/16页30s/真实reload矩阵各一次，必须Verified=true且所有原断言原样。另运行WB036原冻结3asset/独立Expected的生产构造React就绪正控：原URL、native seed删除、有限资源/重复join/action拒绝、精确静态路径/MIME/CSP、safe-summary及唯一当前公开主体与原native Context精确匹配（值不打印）；仅在唯一精确侧栏节点且原DOM中恰好一次出现时，才从脱离页面的克隆移除此已授权公开文本后扫描全部其余DOM/属性/秘密，其他UUID/token/key/路径零豁免，既有A005断言不改。冻结`context-store.ts:184–209`只有一次join/一次context action；`workspace-session.ts:138`初订阅会reset→safeIdentity（:104–105），`App.tsx:89`可公开当前principal，无额外业务读取/第二次action。FR-006明确身份由Host授权交付，不能把公开主体误认bearer，也不能放过额外身份或秘密。不点击业务、不触真实数据或新建操作权限；原Bridge/listener/browser首结束及process/reader/writer/pump/Host Run实际join、借用Connection/peer保活逐项证实。失败保留诊断，不改包、不放宽断言、无改变不重跑。整个生产复验不等于完整T018/G306联合、H02 worker/Q-proof、H04或最终发行权；固定consumer格式/旧窗口/selected截面和T023/002不变。

实施结果：normal/tag四TEST构建、两包vet及所选owner/loader/Handler影响父通过；原normal五项真实生产矩阵在production-01通过，新WB036 oracle初版失败保留。完整订阅路径诊断后只改新WB036观察器（一次action、精确唯一当前公开主体、不放过其余秘密），production-02再次失败记录保留；production-03 normal仅重跑受影响WB036正控通过，未运行的tag六项全矩阵通过，均零selected SKIP。借用main/peer保持、原Bridge/listener/browser/文档pump/Host Run首次结束和实际join通过；所有已选输入/镜像前后守恒，未重复已通过且未变的normal五项。具体边界和失败诊断见新`browser-production50-20261029/README.md`及`analyze-and-failure.md`，不因组件就绪签完整T018/G306、H02 worker/Q-proof、H04或发行。

映射US1/US4/US6、FR003–005/018–025、SC003/007/009–012局部；宪章I–VII沿原单owner、有限环境/输入/资源、不扩业务权限及诚实分层证据。

## 7. 恢复、限制与阶段交付

业务结果不明仅调查，不换key；原生线索库损坏不删库重建、不把新库当无历史。客户端升级保持可读线索格式，未知格式拒绝新保存；若需格式演进必须预览/排他/明确恢复策略，不能借客户端回退回放操作。任何安装/工作区schema变更另按对应owner评审，本设计不授予迁移权限。

**2026-10-21设计交付（历史，非当前授权/运行状态）**：该轮完成设计草案及评审修订，无构建、业务实验、联网核验或运行验收。选择了私有CDP管道与精确文档登记、受控client-envelope及独立页面上下文，但精确浏览器/profile、真实继承pipe/cookie/唯一文档API与实际完成证据仍待验证；不能用未验证机制扩大支持声明。管道引导TLS pin、单会话结束接缝、4096条U/Q及记录字节界、浏览器确认UI至独立ACK/DELETE的恢复交接和外仓许可同样未验。仅修改本计划及research，不改已批准spec、002授权/locator/共享文件，不生成tasks或进入003实现；差异送回复审，不把文档检查称运行pass。
