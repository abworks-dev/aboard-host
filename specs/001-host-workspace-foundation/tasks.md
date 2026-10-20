# 001 Tasks — 独立 Go Host 与目录工作区基础闭环

**执行基线：001 / 2026-10-18；任务勾选与G/V状态只依据实际证据更新。**
当前功能授权与历史批准仅见[授权记录](<authorization.md>)。本文件的任务生成质量记录及原输入哈希保留历史截面，不宣称技术门或业务验收已通过。

权威输入：[规格](<spec.md>)、[宪章1.0.0](<../../.specify/memory/constitution.md>)、[计划](<plan.md>)、[研究](<research.md>)、[数据模型](<data-model.md>)、[公共协议](<contracts/api.md>)、[长期范围与唯一路线](<../../docs/scope.md#31-长期数据策略与单一实现范围designed未实现>)。沿用原架构/长期数据策略，不重新选型或分配feature编号。

## 输入版本与评审边界

仓库根 `E:\Repository\Github\aboard-host`；Git `master` 无提交、无remote，不能以commit固定输入。以下SHA-256固定任务生成时的历史输入截面，不冒充治理整理后的当前哈希；后续改变受影响设计时须重新检查相关任务/证据，不借用旧结果。

| 输入 | SHA-256 |
| --- | --- |
| 宪章 | `bcd3de58e06cf8c862805e6a1118e7add86f0c7322462ac15680d7207e564607` |
| spec | `41fcaff2fe838b06696d405e50ddd6421d409fd84078240cba550846da7b9f94` |
| plan | `8a92fa7dcc43ffa94784deab62c8127399125c1bda25ff485fe698ff04d3ea4e` |
| research | `3b05c241e8038e503c84a7ea8bd5153eab730792985f7698923479db13e078a6` |
| data-model | `391549704b2a114c85591d603b3dd60286105c3c59ea540ec0c702d82c871af5` |
| contracts/api | `9017999523207cbda366e2ac87c9eb2c91a28b8a0f25e1750e20280a767c7c0a` |
| scope | `ef8e866dd141e2f2c6dbe3ccef747bf8e644b3f86a20276e4cca517de716e7f3` |

评审确认：
- M1仍由001交付真实SQLite保存/更新 → 关闭 → 完整结束Host进程 → 新进程认证、重开并逐字段读取；基础设施子片只能称partial，不延期给M2。
- 安装身份/当前授权/登记唯一owner与每区正式提交者分离；安装host.lock不代替区writer.lock，SQLite事务不代替生命周期排他。
- 既有保存key先当前授权/同输入调查，再考虑全新写准入；receipt与正式效果同事务，unknown不自动重试，close不先释放物理资源。安装操作与区操作不共享全局去重表。
- 长期SQLite+对象策略不要求M1创建对象；本片文本入库，不实现完整对象、Run/SSE/事件平台、通用文件Apply、备份/克隆工具、已有库升级器或自动迁移。不更改旧项目、真实数据，不启动Provider/插件。
- 任务生成时G1–G6全部unverified；T004/V003曾因G2真实负例failed而停依赖，现经用户批准的最小修正和真实COMMIT错误复验已passed；其他未完成门仍unverified。完整历史与当前有限结论见Verification；任务存在不证明方案可行。

## 执行约束与门

1. 当前功能阶段与连续执行权限仅以[授权记录](<authorization.md>)为准；任务生成时复选框初始未勾选不是当前执行限制。只读analyze期间不写入，退出分析处理阻断后才推进依赖；不得以T001或技术spike绕过技术门。
2. `agent` 表示后续获准执行的主Agent；同一协调者拥有接口、集成和Git操作。涉及项目所有权/module path等外部事实由用户提供或真实仓库证据确认，Agent不能自造批准。所有Git提交/远端/发布另行授权。
3. 所列业务源路径是拟议实现位置，不因列出路径即视为已存在。现有文件改动前先读；实现需调整位置时保持任务ID/责任/验证映射，不制造重复owner。
4. 每个运行证据记录：输入/源码指纹、Go/driver/SQLite/Windows/文件系统版本、合成样本与故障阶段、命令/退出码、PID时间线、观察值及证据位置。结果只用passed/failed/skipped/unverified；错误诊断与业务未验证分开。开发诊断/运行日志及合成数据根仅Git忽略的 `.data/logs/001-validation/`，工具/缓存在忽略的 `.data/`；不使用真实用户数据。
5. G门失败或证据不足就停止依赖分支，保留失败及unknown围栏，报告最小设计修正；不得静默换驱动、WAL/FULL、放宽路径/认证、删库补种、自动重试或移除负例。涉及产品/权威变化须另获批准；重验受影响G/V，不沿用旧pass。
6. 硬断电/设备缓存耐久没有本轮可行运行证据，保持unverified并限制承诺；进程kill和EXTRA回读不替代硬件断电验证。未定义的性能SLA不作为退出门；已提议32/64的队列限额及B可推进必须实际验证。
7. **完成门与联合验收归属不同**：T001–T008须通过其前置V子门，T009收齐后才能集成。T010–T022的完成条件是相应实现、与已验证真实适配器的局部测试及代码检查，任务行的V引用标明后续集成/端到端归属，不要求在Host/harness尚未建成时运行整套V008–V015，也不因此将这些V标passed。T023仅建立并验证harness可控性；T024–T031才执行各自完整V，T032才收齐全闭环。局部证据和最终SC分开登记；缺最终V不得宣称001完成。

## Phase A — D0：先消除依赖方的技术风险

G门允许的未来工作只是受限合成验证与必要适配器，不能把spike称完整Host。T009完成前不得进入Phase B/C的服务集成。每个前置门在其依赖分支之前执行，而不是最后统一补测。

- [x] T001 [US1] 核实项目所有权与Go module path来源，记录固定Go 1.27.2、Windows 11 x64/本地NTFS支持profile及已有工具状态到[research](<research.md>)；无remote不编造用户名，不创建远端，不升级全局工具。Owner: agent. Depends on: none. 验收：V001；同时覆盖US2–6/FR-001–002/027。module path无可信依据、固定工具或测试权限不可得时停止代码/依赖准备，报告外部前置条件，不能假定已解决。
- [x] T002 [US1] 将[公共协议](<contracts/api.md>)结构收敛为同仓唯一 `contracts/openapi.json`，固定用例端口/错误/状态与生成或校验绑定方案；保留原文为行为说明并链接结构源，不并行手写TS权威。冻结完整语义输入与各作用域key、授权、epoch、未知结果、schema/ledger拒绝规则；不增加后期API。Owner: agent. Depends on: T001. 完成门：V002a静态冻结；V002b由T003完成，不是T004的前置。覆盖US1–6/FR-003/010/015/023/026/028。发现与已批准行为冲突则停下报告，不靠契约冻结修改需求。
- [x] T003 [US3] 在T004建立模块后，按冻结结构建立 `internal/protocol/` 唯一绑定及[模型§4](<data-model.md#4-摘要协议-v1本仓唯一权威>)的 `testdata/contracts/` 固定输入字节/独立expected摘要、`internal/digest/` 检查与 `tests/fixtures/verify_digest_vectors.py` 独立核对；覆盖UUID字节序、中文/emoji/组合字符、LF/CRLF、absence、2^53+1/上界/溢出和非法UTF-8/NUL。Owner: agent. Depends on: T002, T004. 完成门：V002b执行验证（以V002a为输入）；覆盖FR-010–013/026、SC-006/007/008；不得拿被测实现的输出生成自己的expected。
- [x] T004 [US3] 在授权实现环境创建有来源的 `go.mod`/`go.sum`，只验证首选modernc固定候选及 `internal/storage/sqlite/profile_test.go`：实际engine/Go兼容、许可证/修复、目标构建、真实库重开、DELETE/EXTRA/foreign_keys/busy回读、Windows VFS刷写边界。Owner: agent. Depends on: T001, T002. 验收：V003（G1/G2）；覆盖FR-011/014/018、SC-001/002的存储前提。版本或语义不符则停止存储依赖，不自动装备选/最新版本，不宣称硬断电通过。
- [x] T005 [US4] 在 `internal/platform/windows/` 实现并验证固定锁句柄与路径身份最小适配器/第二进程fixture：host.lock与writer.lock不同作用域，share=0、非继承、OPEN_EXISTING/CREATE_NEW、祖先/关键文件/硬链/volume+file ID稳定性及同卷rename。Owner: agent. Depends on: T004. 验收：V004（G4）；覆盖FR-005/008–009/018–019、SC-003/008前提；锁删除/替换、别名、重解析或继承能形成第二写者即停止所有可写集成，不用PID/mutex叠加掩盖失败。
- [x] T006 [US6] 在 `internal/workspace/inspect/` 验证源零写原型：只占已有锁/冻结源句柄，无journal才复制私有ACL scratch并用固定file URI只读查询；缺锁/活写者/日志/路径不稳明确拒绝，scratch销毁失败单独报告。Owner: agent. Depends on: T004, T005. 验收：V005（G3）；覆盖FR-007–008/026、SC-006。源写入观测或清单/字节改变、无法证明共同稳定性时停止inspect集成，不能改RW、immutable或删除源sidecar绕过。
- [x] T007 [US1] 在 `internal/installation/security/` 验证安装引导和受保护named pipe、SID/服务器核验、防抢占/远端拒绝、impersonation复原、DPAPI/DACL、短期session交付/撤销的最小组合；只用合成安装材料。Owner: agent. Depends on: T004, T005. 验收：V006（G5）；覆盖FR-002–003/026、SC-004。不可访问/坏ACL/解密失败/部分安装不得当首次；缺受控错误SID环境或认证证明失败时停止连接交付，不退回本机免认证，不擅建OS账户/提权。
- [x] T008 [US3] 在 `internal/storage/sqlite/migrations/` 和事务适配器建立新安装库/区库schema=1与ledger/genesis的最小可复用实现，验证同事务版本/Head/receipt、持久admission、驱动错误/取消/物理结束及新可信视图调查；用合成注入验证提交前/中/后、admission不明、缺receipt不能直接判回滚。Owner: agent. Depends on: T003, T004, T005. 验收：V007（G6存储前置部分）；覆盖FR-006/010–015/019、SC-006/007/008前提。未知不能被分类为安全重试或结果无法归因时停止业务提交依赖；只初始化新库，不写升级器。
- [x] T009 [US1] 汇总V001–V007的实际证据及G1–G6前置结论到[research](<research.md>)与[plan](<plan.md>)的对应记录，冻结唯一工具/驱动/连接/路径/安全profile及夹具指纹；明确G6创建协调与端到端部分仍由V008/V010/V014完成。Owner: agent. Depends on: T001, T002, T003, T004, T005, T006, T007, T008. 验收：V001–V007全部给出相应通过证据且无未解决依赖；任何required项failed/skipped/unverified则停止Phase B/C，不自勾外部批准，不把原型通过等同SC通过。

## Phase B — 安装与目录权威

- [x] T010 [US1] 将已验证原语集成到 `internal/installation/store.go`：稳定安装/租户/owner主体、单安装host.lock、独立安装ledger、DPAPI引用与安装操作记录；仅明确初始化新安装，已有不可读/不一致/未知状态fail closed，不切数据根或重置秘密。Owner: agent. Depends on: T009. 验收：V006/V008；FR-001–003/005/010，稳定身份不依赖区目录或客户端。
- [x] T011 [US1] 在 `internal/installation/auth.go` 及操作员控制面集成session签发/撤销、显式每区action授权与允许root，逐请求及业务事务开始前重验当前权限版本；安装授权写入未知时阻断真实依赖操作。Owner: agent. Depends on: T010, T002. 验收：V006/V013；FR-002–003/022/026、SC-004/005；受限HTTP身份不能因同机/路径/旧receipt提升权限。
- [x] T012 [US3] 在 `internal/workspace/` 建立最小文本用例模型及业务存储端口，接入T008的区库约束/复合FK、Head与不可变版本/候选/操作/回执/必要审计；每区实例携稳定身份和独立串行提交者，不造通用Repository、Run/对象表或全局currentWorkspace。Owner: agent. Depends on: T009, T002, T008. 验收：V002/V007/V010；FR-011–016/021/024/028，tenant/workspace归属由Host上下文验证。
- [x] T013 [US2] 在 `internal/installation/workspace_creation.go` 实现显式create：允许root+空目录 → 安装意图 → CREATE_NEW锁/新库genesis → 描述发布 → 登记/初始grant finalize，完整确认才成功；各阶段关联原operation，不覆盖残留、不补业务数据。业务层同时拥有create安装级操作查询、原意图/phase调查及显式reconcile：仅可证实原目录/身份/当前授权时完成原登记，证据不足维持unknown，不重放创建；HTTP仅调用用例。Owner: agent. Depends on: T010, T011, T012, T005. 验收：V008；FR-005–006/009–010/015、SC-001/006；保持跨文件/双库非原子和registrationPending边界。
- [x] T014 [US2] 在 `internal/installation/directory_registration.go` 实现register与同卷停机relink：同身份/物理root重复不复制事实，同UUID不同root拒副本；移动前closed/drained/known，校验登记版本、root file ID与当前授权，路径变化不生成新ID。该业务层拥有register/relink安装级操作查询、原意图调查及显式reconcile，与T013共用安装操作分派：只调查或完成可证明的原登记/定位收尾，unknown不因缺记录/超时变成可重放，禁止补库/改ID；不在HTTP层实现协调。Owner: agent. Depends on: T013. 验收：V008/V012；FR-005/008–009/018、SC-008；跨安装、跨卷迁移/克隆及真实数据修复明确不提供。
- [x] T015 [US6] 将T006接入受限目录inspect用例与安全错误映射，只对获准root检查冻结副本；输出与安装登记、写权限和业务就绪分离，目标零写且不把scratch变第二主库。Owner: agent. Depends on: T006, T011. 验收：V005/V009；FR-005/007/023/026、SC-006；任何不支持形态保留明确拒绝，不新增repair路径。

## Phase C — 正式保存、调查与生命周期

- [x] T016 [US4] 在 `internal/workspace/open.go` 集成OPEN_EXISTING、当前定位/授权、区锁/物理身份、format/schema/ledger/摘要/指针/回执检查；只有通过准入才发布当前epoch，重启遗留unknown先调查/围栏；仅对可证明的本片SQLite日志作明确open存储恢复，不重放业务。Owner: agent. Depends on: T009, T011, T012, T014. 验收：V009/V012/V014；FR-008/010/018–019/023；禁止缺文件补建、未知schema自动migration和未调查即可新写。
- [x] T017 [US3] 在 `internal/workspace/records.go` 实现当前正式文本读取/限量列表，查询显式绑定workspace/tenant与当前权限；游标不能换区，closed无可信读视图时需获准重开，不自行打开或读另一库。Owner: agent. Depends on: T016. 验收：V011/V013；FR-003/011/013–014/021、SC-002/004。
- [x] T018 [US3] 在 `internal/workspace/preview.go` 实现文本校验、Host分配resourceId、持久Candidate与差异预览，绑定区Head/baseDigest/主体/epoch/previewVersion/expiry；确认前不改正式Head，内容/基线改变须新预览。Owner: agent. Depends on: T012, T016, T017, T003. 验收：V010/V011；FR-011–013/017、SC-007/008；文件编辑/未确认候选始终不自动生效。
- [x] T019 [US3] 在 `internal/workspace/commit.go` 实现持久admission和唯一串行提交路径：当前授权后先调查同区同key/输入，旧epoch/候选applied/过期/旧base不得误拒既有receipt；新key才校验当前准入，版本/Head/receipt/Candidate/Operation/审计同事务且存储确认后才success。Owner: agent. Depends on: T018, T011, T008. 验收：V010/V011；FR-012–015/018、SC-007/008；同key异主体/载荷拒绝，不放大为跨区全局去重。
- [x] T020 [US3] 在 `internal/workspace/reconcile.go` 实现保存结果调查/unknown围栏、同key只查不重放、原事务物理结束与排他下新可信视图验证；无receipt不能单独证明未提交，损坏/证据不足维持unknown；正式提交与回执交付错误分报。Owner: agent. Depends on: T019, T016, T010. 验收：V007/V010/V014；FR-004/015/019/022、SC-007/009；权限变更重新检查，closed需获准重开调查，不自动换key/续跑。
- [x] T021 [US5] 在 `internal/workspace/lifecycle.go` 和安装协调中实现每区独立队列/handle/epoch、32每区/64安装未结束操作准入，close原子拒新准入、既有操作物理排空/结果已知后关DB释放锁、正常shutdown同规则；timeout/断连不释放未知名额或假称关闭成功。Owner: agent. Depends on: T016, T019, T020, T011. 验收：V013/V014；FR-004/019–025、SC-005/009；调查/排空不能被满队列永久饿死，旧epoch仅允许原accepted者受控收尾。close内存协调状态与持久保存receipt分开，重启缺历史明确不可查。
- [x] T022 [US1] 在 `cmd/aboard-host/`、`internal/transport/http/` 接通标准库HTTP命令/查询、受保护操作员pipe、严格Host/Origin/JSON校验、关联脱敏及capability unavailable；生命周期归Host，不直接用HTTP request context取消accepted业务。Owner: agent. Depends on: T011, T013, T014, T015, T017, T018, T019, T020, T021, T002. 验收：V008–V015；FR-001–004/023/026–028、SC-001/004/009；无前端/旧Host依赖，无公开故障注入、SQL/secret/任意FS入口或空取消API。

## Phase D — 真实验收证据，不用Mock替代闭环

- [x] T023 [US1] 在 `tests/acceptance/` 建立外部客户端/Host进程管理与合成故障harness；按测试名隔离安装根/目录/端口/材料，记录PID/版本/故障阶段，区锁竞争helper复用同一Windows适配器并在真实SQLite尝试保存。Owner: agent. Depends on: T022. 验收：V008–V015的环境前提；FR-027。测试注入只在内部/测试构建，合成前缀硬门禁，不能成为生产绕授权API；不得只测host.lock便宣称writer.lock已证。
- [x] T024 [US2] 执行V008并保留创建每阶段崩溃/丢回执、登记finalize未知、显式reconcile、重复登记、同卷relink及安装首次/既有异常的证据；校验同一操作关联和残留拒绝。Owner: agent. Depends on: T023, T013, T014, T015. 验收：V008；FR-005–006/009–010/015、SC-001/006/008；没有跨文件/双库原子或自动补种结论。
- [x] T025 [US6] 执行V009的全部坏目录/ledger/身份/路径/日志样本，比较inspect前后源递归清单/字节及归因写操作、外部执行计数，验证open明确拒绝、不修复。Owner: agent. Depends on: T023, T015, T016. 验收：V009；FR-007–010/023/026–028、SC-006；不以单个健康样本或接口名称ReadOnly替代。
- [x] T026 [US3] 执行V010：保存/admission前中后注入、回执丢失、busy/容量/IO故障、同旧base并发、同key异载荷/主体、候选过期/旧epoch后查原receipt、重启未知调查；逐项核对正式效果/receipt至多一次及unknown隔离。Owner: agent. Depends on: T023, T019, T020. 验收：V010；FR-004/011–015/017/019、SC-007/008/009；每类至少1例，无未确认success/自动重复提交。
- [x] T027 [US3] 执行V011的最终正式闭环：A/B各至少10条不同正文、各更新至少1条，关闭所有区并确认Host旧进程结束，再以新进程认证/重开，逐字段比较安装/主体/工作区/资源身份、revision/version/digest与正文。Owner: agent. Depends on: T023, T024, T026, T021. 验收：V011；FR-001/011–014/017/021、SC-001/002；不读取旧进程/客户端缓存，不把此归属移到M2或以部分基础设施宣称001完成。
- [x] T028 [US4] 执行V012真实第二进程写者持续持有/同时竞争/kill各至少10轮及正常释放、锁替换/继承/别名、副本与同卷移动负例，验证失败者零成功保存、重新排他和已确认数据保持。Owner: agent. Depends on: T023, T016, T020. 验收：V012；FR-008–009/018–019、SC-003/008；进程kill不是硬断电证据，不用两线程/Mock/PID代替。
- [x] T029 [US5] 执行V013：同名/同临时键A/B交错、伪造资源/候选/操作归属、受限主体、关闭A/停机坏A后B新增更新读取、A长IO/满队列、共享授权未知的真实依赖阻断与秘密扫描。Owner: agent. Depends on: T023, T020, T021, T025. 验收：V013；FR-002–003/021–026、SC-004/005；不为局部可用绕过安装安全，不称共享crash/OOM/磁盘满完全隔离。
- [x] T030 [US5] 执行V014：断连/取消等待者、close中保存悬置、Context超时但物理IO未结束、迟到回调/旧epoch、新进程未知调查及close历史不可查；记录锁/连接/队列/逻辑/物理/持久状态时间线。Owner: agent. Depends on: T023, T020, T021. 验收：V014；FR-004/015/019–024、SC-003/005/007/009；不得提前释放锁、误称safeToMove或将Run逻辑套在不存在的Run上。
- [x] T031 [US1] 执行V015的结构/字节/权限/范围回归与目标构建，检查当前公开绑定只有一个结构源、日志DTO不泄密、未引入对象/Run/备份/旧库升级/消息总线等延期设施；编译/race/Mock单列覆盖范围与环境不足。Owner: agent. Depends on: T024, T025, T026, T027, T028, T029, T030, T003. 验收：V015；FR-010/016–017/024–028、SC-001–009相关回归；静态或race通过不替代V008–V014。
- [x] T032 [US3] 按当前源/契约/夹具指纹汇总V001–V015及全部US/FR/SC实际证据到[plan](<plan.md>)现有验收记录，保留历史失败和未验证硬件边界，明确partial/implemented与用户验收分开；任何必需闭环/负例未验证就不宣称001完成。Owner: agent. Depends on: T009, T024, T025, T026, T027, T028, T029, T030, T031. 验收：V001–V015各责任任务证据齐全；正式保存/完整重启重开始终由T027承担；不自动退役旧Host、迁移真实数据、提交/push或发布。

**当前工程验收状态（2026-10-20T15:08:26+08）**：T001–T032固定支持profile全部完成，V001–V015（V002a/b）及六US/28FR/九SC责任证据已归并到[plan最终汇总](<plan.md#101-001最终工程验收汇总t032当前>)。T031全仓测试/normal与tag vet/目标四image及独立结构字节回归通过；T032另以当前image真实等待900.1647946秒（无TTL/时钟/SQL改动），原receipt保持/过期拒绝/fresh正控/完整重启通过；当前G4三轮与真实8.3、环境/modules verify亦0，261输入无漂移，136份选定JSON/本地链接和全部ID映射复核。race在固定CGO0实际exit2、未执行，硬断电/全局trace/SLA/跨profile仍未承诺；历史native233/Unauthorized/rename failed保留未定位，不以有限green声称永久可靠。这里签收工程责任门，不代替用户最终接受，不包含提交/push/发布、旧Host退役或真实数据迁移。

## 依赖、门与建议首批

- 前置链：T001 → T002（V002a静态冻结）→ T004（模块+G1/G2）→ T003（V002b执行验证）；T004后才能做T005；T006/T007分别验证inspect与安全，T008验证存储提交边界；T009收齐含V002a/V002b的全部前置门。T004不依赖T003，没有无模块先跑Go验证的隐性环。
- 目录链：T009 → T010/T011 → T013/T014；区事实T012也依赖T009；T015只有inspect权限，不因此获得可写访问。
- 保存链：T016 → T017/T018 → T019 → T020 → T021；T022汇合所有公共能力，T023再建设真实外部验证环境；T024–T030各自提供范围明确的证据，T031回归后才到T032汇总。
- 所有依赖均显式列在任务行。暂不标`[P]`：单一协调者、共享Win32/认证/存储契约与真实进程/故障环境，现阶段不承诺安全并行。将来确需并行，先冻结接口与binary指纹，分离写入所有权/合成根/端口/凭据/输出，再在任务评审中明确标记，不能只因文件名不同就并行。
- **历史首批（从零建立基线时）**：T001 → T002 → T004 → T003 → 其余T005–T009。该顺序保留原module来源/静态契约/建模块后验证字节的依赖，不是要求当前重复初始化或创建远端。
- **历史执行增补（T009恢复至T025签收截面；当前见任务行与最终汇总）**：两源变更已明确，受影响门与全仓组合三轮真实复验并新冻结T009；已依T010 → T012 → T011完成真实局部三轮与vet，T011交付竞态及原生路径别名窄修后又以最终源码全仓三轮/组合实际通过；T013/T015经局部真实回归和代码审阅签收（已修committed创建在真实保存后reconcile错误要求genesis）；T014已接入物理身份/closed准入/统一查询reconcile，权限审阅问题已修且真实局部三轮、全仓测试及vet通过；T016已完成既有区open/保守显式日志恢复及冷unknown围栏局部实现，最终四包三轮、全仓回归和vet通过；T017已接当前正式读取与作用域游标并以真实受限主体/A-B同键验证，focused三轮、全仓及vet通过；T018不可变持久预览已通过真实权限/SQLite刷写负例与focused三轮、全仓/vet；T019唯一正式保存已通过同key原receipt/跨区隔离与真实admission/正式COMMIT逐刷写故障、取消等待交付分离测试，focused三轮、全仓/vet通过；T020已接原key/当前权限调查、同lease原连接实际结束后fresh同历史验证、确定未提交持久收尾与不重放围栏，六个原生刷写故障及冷accepted/恢复清理重试经focused三轮、全仓/vet通过；T021已接独立有界队列、32/64准入、未知物理操作保留名额、调查/排空优先与close/shutdown代际收尾，真实native刷写悬置下满A/B独立保存、六个刷写错误后的unknown close/显式调查及四包三轮/全仓/vet通过；T022 Host/HTTP/受保护pipe、当前能力、已知物理停机和同区save/close key边界已通过各自真实局部回归，同一容量已接HTTP body前准入、原调查claim和真实Write/Flush后交付收尾，并通过实际31A/31B+installation慢body、拒绝小body/chunked及正式receipt交付失败回归；inspection原生Close-unknown的资源owner、原容量、工厂错误持有及停机围栏亦已补齐并经六包三轮、全仓/vet/CLI构建；按本文件第7条T022局部门完成；T023外部harness已通过真实生产exe启动/认证/单条保存/正常退出/新进程重开、独立写者真实SQLite保存/持锁、受控空闲进程kill与门禁负例的可控性检查，最终三轮/全仓/vet/CLI构建exit0。T024已实际完成五个创建持久checkpoint中断/丢回执→正常新进程调查/reconcile子集，tag门禁/定向三轮/全仓/vet/正常及tag构建通过；随后实际外部非空create、逐次重复登记、副本拒绝、公开closed/drained后两次同卷relink/CAS正反控、内核生存期重叠的唯一首建及七类既有安装异常亦经定向三轮/全仓/vet通过，修复非空与副本两项typed错误映射，并校正五阶段证据误写；随后登记finalize baseline实测三个native Flush边界逐项真实BOOL0/SQLite1034/ErrUnknown、同进程共享授权围栏/安装锁保留、实际旧进程结束后正常生产Host按原key显式reconcile，以及前三残留open/register/新key create拒绝亦已接入全部V008并经tag默认/legacy panic(nil)门禁三轮、定向三轮、全仓/正常及tag vet/两CLI构建exit0；按冻结固定profile签收T024/V008，不把强杀称排空或推成硬断电证明。T025又完成38例有限归因与安装/工作区各七个真实fixture初始化checkpoint，补窄拒绝安装未证明sidecar，最新三轮/全仓/双构建-vet通过；随后canonical missing/真实junction两例亦补齐原严格gate下归因，40例有限归因/完整V009三轮、全仓及正常/tag/helper构建与vet均0，按固定支持profile有限范围签收T025/V009；下一批T026/V010，最终V010–V015仍未签收，T027仍负责A/B各10条及更新的正式重启闭环。每项源码/命令/日志指纹和历史失败见[研究当前证据](<research.md>)。暂停时草稿不因存在而完成；不提前宣称001完整验收，不运行Provider/插件，不使用历史pass覆盖新源。
- 当前analyze/实现的阶段顺序与权限仅见[授权记录](<authorization.md>)。不改前置hook，不将任务文件存在视为analyze或技术门已通过；分析结果及后续修复须区分只读检查与退出后的写入。

## 需求与验证追踪

每项V均在后文定义；下表只表设计覆盖，不是运行结果。

| 需求/故事 | 责任任务 | 验证 |
| --- | --- | --- |
| FR-001；US1 | T001/T010/T022/T027 | V001/V006/V011 |
| FR-002–003；US1/5 | T007/T010/T011/T022/T029 | V006/V013 |
| FR-004；US1/3/5 | T019/T020/T021/T026/T030 | V010/V014 |
| FR-005–006；US2/6 | T013/T014/T015/T024 | V008/V009 |
| FR-007–008；US6/4 | T005/T006/T015/T016/T025/T028 | V004/V005/V009/V012 |
| FR-009；US2/4 | T013/T014/T024/T028 | V008/V012 |
| FR-010；US2/6 | T002/T003/T008/T016/T025 | V002/V007/V009 |
| FR-011–014；US3 | T003/T004/T012/T017/T018/T019/T027 | V002/V003/V010/V011 |
| FR-015；US3/2 | T008/T013/T019/T020/T024/T026/T030 | V007/V008/V010/V014 |
| FR-016–017；US3 | T012/T018/T027/T031 | V011/V015；无对象时只审范围，不称对象协议完成 |
| FR-018–019；US4 | T005/T016/T020/T028/T030 | V004/V010/V012/V014 |
| FR-020；US5 | T021/T029/T030 | V013/V014 |
| FR-021–022；US5 | T011/T012/T021/T027/T029 | V011/V013 |
| FR-023；US5/6 | T002/T015/T016/T021/T030 | V002/V009/V014 |
| FR-024–025；US5 | T021/T029/T031 | V013/V015；Run执行验收仍M4/M5，非本片运行检查 |
| FR-026–028；US1–6 | T002/T007/T015/T022/T023/T025/T031/T032 | V002/V006/V008–V015 |
| SC-001/002 | T027（完整闭环唯一最终责任）；T024提供创建前提 | V011；不得以V003原型或health替代 |
| SC-003 | T028/T030 | V012/V014 |
| SC-004 | T007/T029 | V006/V013 |
| SC-005 | T029/T030 | V013/V014 |
| SC-006 | T025；T024覆盖创建残留 | V009/V008 |
| SC-007 | T026/T030 | V010/V014 |
| SC-008 | T024/T026/T028 | V008/V010/V012 |
| SC-009 | T030/T026 | V014/V010 |

宪章逐条设计映射继续由[plan§6](<plan.md#6-constitution-check逐项设计覆盖运行通过>)拥有，不再复制一套。执行关联为：INV-01–05→T010–T014/T016/T028/T029；INV-06–08→T005/T006/T015/T025/T028；INV-09–14→T003/T008/T013/T018–T020/T024/T026；INV-15–17→T020/T021/T030；INV-18–21→T007/T011/T021/T029/T031。四层隔离及共享影响由T021/T029/T031验证其本片边界；不伪称后期执行器/对象/事件已经存在。

## Verification

**任务生成时V001–V015均为 `unverified`。冻结后profile.go/material_windows.go改变曾使V003/V006/V007及T009重新unverified；现已实际focused三轮及全仓组合三轮、vet/依赖/单源检查全部exit0，按新指纹重新冻结T009，V001–V007在有限前置范围passed。新记录为忽略目录`t009-revalidation-20261018-refrozen-inputs.json`，保留旧失败/冻结；三份进度文档在测试中更新单独记录，不作为源码不变。该T009截面V008–V015仍unverified，不从原语通过外推；后续当前结果以各V分节及责任任务为准。** 现有plan的V1–V8是验证分组，不是这些新V编号的历史pass：V1→V006/V011/V013；V2→V011；V3→V004/V012；V4→V007/V008/V010；V5→V013；V6→V014；V7→V005/V009；V8→V002/V015。G1→V003，G2→V003，G3→V005，G4→V004，G5→V006，G6→V007前置+V008/V010/V014集成。分组映射不自动授予技术或业务通过。

以下保留任务生成时约定及带日期的历次门状态；各条“当前”指其所标时间，不覆盖更晚的签收。最新完整结论以本文件任务行、当前工程状态与plan最终汇总为准。下面的Go包/测试/harness按各任务进度提供；T004局部适配器/测试已存在，但其他未实现入口不能把“无测试”当通过。若环境不足记录unverified及失败命令；不得用未执行测试、mock或静态文档检查补齐required门。

### V001 — 工程身份与环境前置

- 过程：在已核实根中确认module path的真实所有权依据，核对固定Go发布/本机工具，不安装或升级全局工具；未来准备项目局部工具后运行 `go version`、`go env GOOS GOARCH CGO_ENABLED`，记录Windows版本、NTFS卷/允许root与权限。
- 通过标准：module path有真实证据而非猜用户名；目标Go/平台可固定复现，具合成目录/进程/故障测试所需授权。缺任何required前提则停止T004及后继，不建立伪模块或擅自提权。
- 覆盖：FR-001–002/027；G1/G4/G5环境前提。责任：T001、T009 / agent；环境：Windows目标机、真实项目远端与隔离工具链。当前证据：**passed（仅V001）**；不将其当G1–G6或业务验收。
- 2026-10-18初查无remote且仅Go1.27.0，当时V001未通过并停止依赖。随后用户明确授权创建本项目远端；已验证私有 `abworks-dev/aboard-host`、origin与module来源、零远端分支/未push，并在项目局部取得 `go1.27.2 windows/amd64`。目录/Git根、Windows 11 x64、本地NTFS及Go环境命令exit 0；详见[研究T001记录](<research.md#t001--v001-实施前置核查2026-10-18>)。诊断 `.data/logs/001-t001-environment.json` 保留初查，`001-t001-remote.json` 和 `001-t001-toolchain.json` 固定后续来源/版本/二进制SHA；均在忽略的 `.data/logs/`。T001已完成，可继续T002，不绕过其他G门。

### V002 — 单一结构源与独立字节夹具

- **V002a（T002静态前置）**：解析/检查 `contracts/openapi.json` 的结构与引用闭合，逐项核对现有API/模型的字段、严格输入、错误、状态、授权及各作用域语义，固定绑定/校验方案。此门不要求Go模块、生成绑定或Go测试已存在；行为冲突则停，不据实现方便改spec。静态一致通过只允许T004准备模块，不表示V002b通过。
- **V002b（T003执行前置）**：在T004已建立并验证模块之后，生成/校验唯一绑定，运行未来 `go test ./internal/protocol/... ./internal/digest/...` 及 `python tests/fixtures/verify_digest_vectors.py`；逐例比较固定输入字节与独立expected，不从Go结果生成expected。模型§4所有边界匹配，重复键/未知字段/非法编码/非规范UUID/溢出拒绝；同语义JSON顺序不改变摘要；kind/主体/目标/载荷/基线改变不能借原key提交；revision无JS精度截断。
- 覆盖：FR-003/010–013/015/023/026；SC-006/007/008前提。责任：T002拥有V002a，T003拥有V002b，T009必须收齐两者 / agent；环境：V002a仅现有规范/静态JSON检查，V002b需T004工具链、Python及固定fixture来源/指纹。当前证据：**V002a passed；V002b passed（仅结构/固定字节前置）**。
- 2026-10-18执行 `python -B tests/fixtures/verify_openapi.py`，exit 0：16操作/32schema、全部32引用目标闭合、行为路径与输入字段一致、严格对象/身份边界及9个检查器负向反例。OpenAPI SHA-256 `895467d1ff6efe23ee2a12da65a82b83e9ebfff97372b4002e44bdd5de126ecd`；检查器SHA-256 `809b67fe2ab99ce1279c56b13c13faf927a6c189ef277ede6424f0610d7b09a3`；原始结果在忽略的 `.data/logs/001-validation/v002a.json`。Go生成/校验绑定方式见[契约§6](<contracts/api.md#6-t002结构冻结与绑定方式>)，未执行Go测试，未将静态检查当认证/持久化证据。T002完成时仅允许T004，不跳过V002b。
- 2026-10-18当前V002b：单源Go生成/漂移检查、独立Python 16正例/42拒绝例/88字段变异及Go协议/摘要测试、vet均exit0；固定fixture SHA-256 `27764efb69c709e7b1e716cae1f6d0397e0f65e44e111a0141f1c5209ccfd2d2`。父审发现日期format断言超出冻结annotation，已删除该分支并新增5字符串接受/6类型或缺字段拒绝，新的全前置包三轮复验exit0；未伪造此前red。详见[研究T003](<research.md#t003--v002b-单源绑定与独立字节复验2026-10-18>)。类型/字节证据不等于权限、key幂等提交或Host expiry业务校验。

### V003 — 驱动、真实SQLite与连接/持久性profile（G1/G2）

本次恢复执行：旧冻结历史保留；`profile.go`关闭错误传播/幂等变更已由存储三轮及全仓组合三轮真实复验，当前 **passed（仅原V003范围）**，新指纹/命令见Verification总览的重冻结记录。

- 过程：核对并记录Go/module校验和、engine/VFS、许可证/修复依据；未来 `go test ./internal/storage/sqlite/... -run TestProfileGate -count=1` 在真实NTFS文件上建库、提交、关连接、重开，逐连接回读SQLite版本及DELETE/EXTRA/foreign_keys/busy_timeout；覆盖错误/进程kill后的确认读回。
- 通过标准：固定首选组合可构建，实际engine与冻结记录一致且已含所需修复，DSN不接受用户pragma/SQL/ATTACH；设置不被忽略/静默变换；确认数据进程重开可见。Windows VFS刷写保证有可核实边界；硬断电仍无证据，不作该承诺。
- 覆盖：FR-010–015/018的存储前提，G1/G2；不单独满足SC-001/002。责任：T004/T009 / agent；环境：目标工具链、受支持NTFS、项目局部依赖、可控合成故障。当前证据：**passed（固定兼容修正后的G1/G2进程故障/错误可观察性范围）**；T004已完成，可继续T003/T005，不代表T009或SC通过。原失败保留；硬断电unverified，不允许自动换mattn/WAL/FULL。
- 2026-10-18：固定Go/driver实测engine=3.53.4、win32；模块校验/fmt/vet和正常profile/真实进程提交、kill前后重开三轮通过。随后新增必需 `TestProfileGateFlushFailureContract` 实际exit 1：只读访问的合成文件句柄使原生FlushFileBuffers报Access denied，而固定libc返回BOOL=-1；实际SQLite win32 xSync将非零当SQLITE_OK。直接API反例+源码调用链已确认，不冒称已经在COMMIT中注入错误。详情与指纹/日志见[研究T004阻断](<research.md#t004--v003-存储前置实测与g2阻断2026-10-18>)。保留失败负例；未改依赖、未换profile、未推进服务集成，须可审计修正并重验G1/G2后才能继续。

- 2026-10-18当前复验：原负例不删不弱化，兼容修正只将固定libc的FlushFileBuffers失败BOOL改为0。真实COMMIT正常3次刷写，分别第1/2/3次真实失败均收到SQLITE_IOERR_FSYNC(1034)，无成功；物理关连接后fresh视图检查业务/receipt均0/0。全TestProfileGate三轮、vet、mod verify exit0；另不带修正控制证明运行时guard拒绝开库且不创建DB。源码/工具指纹与完整历史见[当前复验](<research.md#获准最小兼容修正与当前复验2026-10-18>)，忽略日志 `v003-patched-profile-final.txt`、`v003-patched-inputs.json`。不是从任意COMMIT错误推定回滚/自动重试，也非硬断电、OS锁、认证或001闭环通过。

### V004 — 真实锁与物理路径原语（G4）

- 过程：未来 `go test ./internal/platform/windows/... -run TestDirectoryGate -count=1` 调用真实子进程/同一适配器及SQLite合成写入；持续持有/同时竞争/异常退出各至少10轮，另测正常释放、锁删换、继承、重解析祖先/别名/关键文件硬链、root替换、同卷rename/副本。
- 通过标准：任意时刻最多一个OS排他持有者，失败者零确认写入；重获锁后才检查/写库，异常释放延迟诚实报告；UUID/物理身份分离。无法稳定证明的路径拒绝，不能以不存在PID或新锁对象替代。
- 覆盖：FR-005/008–009/018–019；SC-003/008前提，最终生产路径仍须V012。责任：T005/T009 / agent；环境：同NTFS真实第二进程、可观测句柄/文件身份。当前证据：**passed（G4原语范围，非最终V012）**；保留空祖先metadata reparse原失败，修正后必需native瞬间断言及40次竞争、三类真实进程各10轮，完整三轮exit0。命令、限制与证据见[研究T005](<research.md#t005--v004-windows锁与共同稳定路径原语2026-10-18>)；该次签门时G3/G5/G6尚未通过，属历史时点，不覆盖Verification总览中的当前状态。

### V005 — 源零写inspect与敏感scratch（G3）

- 过程：未来 `go test ./internal/workspace/inspect/... -run TestInspectionGate -count=1`；先制备合成样本/字节清单，再在源写操作观测下检查冻结/拷贝/只读副本查询/清理；包括活写者、日志/缺锁、特殊文件名/DSN、路径替换与scratch清理故障。
- 通过标准：源零创建、零改字节、零归因写操作、无RW恢复/迁移/激活/Provider；私有scratch最小ACL且不流入日志/备份，不安全时明确不可检查。清理失败不伪称已删除，不能为成功检查改源。
- 覆盖：FR-007–008/026；SC-006前提，最终全套样本仍须V009。责任：T006/T009 / agent；环境：V003/V004通过、可靠OS文件写入观测和合成数据。观测能力不可用则门仍未验证，不只凭字节相同声称零写。当前证据：**passed（G3有限原语范围）**；固定Windows/NTFS/Go+driver三轮94.034s及vet exit0，PID可计数data-write总量严格等于独立scratch下界、源NTFS有界通知/清单不变，真实freeze/ACL/anonymous拒绝/清理故障及rename正反控通过。前次通知失败经不调用Inspect的清单诊断复现，修正预置顺序、未过滤事件；原失败保留。不是逐path/失败尝试完整trace或最终V009；机制、指纹和限制见[研究T006](<research.md#t006--v005-冻结副本与有界源零写观测2026-10-18>)，T009前仍不进入服务集成。

### V006 — 本地安装安全与引导（G5）

本次恢复执行：`material_windows.go`身份/lease入口变更后，已实际补齐材料往返/21类身份拒绝/错误lease负例并重跑完整安全门及组合三轮，当前 **passed（仅原G5范围）**；新记录见Verification总览，不沿用旧源pass。

- 过程：未来 `go test ./internal/installation/security/... -run TestBootstrapGate -count=1`；合成首次双进程竞争、已有部分/不可解密/不安全ACL、pipe抢占/错server/远端/错误SID、impersonation复原、会话过期/撤销/伪造及Host/Origin负例；实际控制面交付一次性token并扫描输出。
- 通过标准：未知既有安装不重建，安装身份稳定且凭据不外泄；授权失败在读取目标前拒绝，OS操作员与受限会话不混同；无法核验SID/server、DACL或DPAPI就不提供可信连接。失效授权/共享未知不得使用旧cache继续提交。
- 覆盖：FR-001–003/022/026；SC-004；与V013合并覆盖业务越权。责任：T007/T009及T010/T011集成 / agent；环境：获准的合成材料和受控SID/远端反例环境；若需新OS账户/系统配置须先另授权，缺环境不伪造pass。当前证据：**passed（G5原语范围，非完整服务授权）**；原生门三轮、security+platform回归、vet exit0，实际anonymous token/DPAPI/pipe、抢占及SMB redirector正控制后远端拒绝均有证据，required反例无skip。独立只读复核无阻断，父核六源SHA一致；边界与失败夹具修复见[研究T007](<research.md#t007--v006-安装引导与本地连接安全原语2026-10-18>)。T010/T011现已接入真实安装DB当前授权并以最终源码局部/全仓三轮、交付竞态和重开/原生错误测试通过；最终HTTP listener/工作区用例的V013仍unverified，不提供完整服务交付。

### V007 — 新库ledger、admission、receipt与驱动未知边界（G6前置）

本次恢复执行：物理关闭失败分类变更已补错误粘滞控制和真实SQLite重复关闭/重开，原提交/unknown/取消排空断言均在存储及组合三轮重跑，当前 **passed（仅原G6存储前置）**；factory全部失败分支的真实SQLite Close错误仍未独立注入，不扩大已证范围。

- 过程：已实际通过项目runner执行 `test ./internal/storage/... -count=3 -v`（包含TestMigration与TestCommitGate全组，不仅按CommitGate过滤）；在真实库验证两类新库genesis/ledger同事务，注入schema初始化、admission、业务Commit前/中/后、取消/连接错误、回执丢失与重开调查。
- 通过标准：正式版本/Head/receipt/操作结果原子一致；明确拒绝只在可证明未提交时给出；超时/错误不代表物理结束，未结束不释资源；旧snapshot缺receipt不触发重试。ledger/schema/user_version/校验和任一矛盾均拒绝、不修复；首次失败残留不补种。先建立错误映射和可观测结束边界，再接业务提交。
- 覆盖：FR-006/010–015/019、SC-006/007/008存储前提。责任：T008/T009 / agent；环境：V003/V004通过、真实SQLite故障控制、固定schema/字节fixture。当前证据：**passed（G6存储前置部分）**；全storage三轮19.508s及vet exit0，包含两库ledger/初始化14阶段真实子进程中断、create/update/admission/init/rejected记录各COMMIT三处原生刷写失败与unknown、原receipt调查优先、持久rejected重启、ctx到期但native未结束时并发EndPhysical等待。两项独立审阅P2已窄修复/定点关闭；初始化rollback-failure标记仅静态修复，未另独立动态注入该分支。不是OS锁/队列/Host排空或硬断电证据；过程、指纹、边界见[研究T008](<research.md#t008--v007-新库ledger与持久提交前置2026-10-18>)。完整G6仍需V008/V010/V014，T009收齐前不进入服务集成。

### V008 — 安装与创建跨载体协调

- 过程：由T023外部harness驱动新Host，在意图、锁/目录准备、genesis、描述、登记/初始grant各阶段各至少一次中断或丢回执；调查原installation key，分别验证仅查证/完成原登记或维持unknown。补测非空create拒绝、重复登记、副本、closed/drained/known同卷relink、首建竞争/既有安装失败。
- 通过标准：只有genesis+描述+登记/授权均确认才报完整成功；残留不被open/inspect补成新实例；同身份/同root不复制状态，副本不自动换ID/继权，移动身份/内容不变，旧位置不补建。两库/文件之间部分成功明确保留，不假称天然原子。
- 覆盖：US2；FR-005–006/009–010/015；SC-001/006/008，G6协调部分。责任：T024 / agent；环境：T023测试构建、全新合成安装/路径、真实子进程与阶段故障。当前证据：**passed（固定本地支持profile的完整V008，非硬断电）**；2026-10-19五个持久checkpoint真实进程中断/正常新进程调查、非空create/重复登记/副本/两次同卷rename与CAS/并发唯一首建/七类安装拒绝，以及实际登记finalize三个native Flush边界的BOOL0/SQLite1034/ErrUnknown→同进程共享授权围栏→实际原进程结束→正常生产新进程原key/原phase显式reconcile全部三轮passed。前三残留实际open403、register/换key create/inspect拒绝且无新操作/新授权、源清单/字节守恒，原UUID不补种；open403仅证明当前授权先拒绝，不冒充已授权存储恢复。正常/tag门禁（额外legacy panic(nil)）、完整V008四包定向三轮、全仓一次、正常/tag vet/CLI构建全部0；源码/真实映射修复/历史失败/证据校正/限制见[研究T024](<research.md>)。不自动删残留、把强杀称正常排空或把观察到的cold accepted偷换持久unknown；未另trace COMMIT/rollback具体错误分支，不宣称跨载体/硬件断电原子性。

### V009 — 坏目录拒绝与inspect最终副作用验收

- 过程：外部客户端对缺库、零字节、坏digest、伪造identity/描述、未来format/schema、旧身份v1、不存在路径每类至少1例；另测两库ledger缺失/未知校验和/顺序/状态、user_version不符、初始化中断、hot journal/WAL、路径/ACL/DPAPI不支持样本。先固定样本再比较inspect前后递归清单/字节/源写操作与外部调用计数，随后验证open拒绝。
- 通过标准：inspect零目标创建/改字节/归因写操作，零修复/migration/插件/Provider；open不补缺库/默认数据/改ID/降版本；需恢复日志的情况明确分类，不能用inspect做RW恢复；区故障与安装故障分别限定影响。
- 覆盖：US6；FR-007–010/023/026–028；SC-006及SC-004相关泄漏负例。责任：T025 / agent；环境：T023、受控坏样本/观察工具、V005前置；当前证据：**passed（固定支持profile的有限V009，不外推全局trace）**。2026-10-20 02:03最终补齐canonical missing/真实junction两例，40例有限归因与完整V009三轮、全仓一次、正常/tag/helper构建、tag门禁三轮及正常/tag vet/diff均exit0；源/命名空间守恒、实际PID/Job/defaultHTTP正控与原资源/Host全部结束证据见[最终T025签收](<research.md#t025v009-原生路径归因与有限验收签收2026-10-20passed固定支持profile>)。以下partial/仍不勾选/最小剩余是补齐前历史，不覆盖本次有限签收；旧failed和未定位原生认证根因仍保留，T026–T032未验收。2026-10-19正常生产独立exe三十类工作区/八类安装ledger-schema样本、已授权open明确拒绝、逐例B真实新增确认/读取、源递归清单/字节与有正控的NTFS合并窗口观察已定向三轮/全仓一次/正常及tag vet与构建通过；没有把合法unavailable样本强制判坏格式、把通用RECOVERY_REQUIRED当唯一诊断或把失败watcher强杀当正常排空。随后新增六类路径边界（已登记missing/真实junction及relative/UNC/device/越界）与三类ACL/三类DPAPI样本，正常生产外部负例、owner+DACL/源字节守恒、正常还原认证及逐局部负例B真实保存，经定向三轮/全仓/正常及tag构建-vet通过；ACL只改自有fixture、未改系统策略，native junction正控不依赖本机未解析的EvalSymlinks。随后独立实际写者在第三次成功原生DB刷写/DELETE日志删除前退出，正常生产Host的inspect拒绝且不改源；健康hot显式open恢复原正式记录/receipt/整源字节，预先损坏immutable历史的真正hot则拒绝恢复且源不变，B均实际新增确认读取。新增定向三轮/全仓/双构建-vet通过。随后instrumented独立exe的九类健康/坏目录补齐受控静默PID窗口、三个真实同字节原生WriteFile灵敏度控制、已完成总数据写=独立scratch下界及原生资源全部Close之后的计量；源NTFS/字节、窗口后B真实新增确认读取、实际Host正常退出均三轮通过。三包定向三轮/全仓一次/正常及tag构建-vet全0；仅后续tag-only测试teardown修复，重验门禁并逐文件证明其余输入不变。随后同九例在实际原Host上以严格Job成员/累计总数核对真实自子进程正控：1→2、完整inspect/原资源Close窗口2→2、正常Host结束2/0，三轮/全仓/双构建-vet通过；失败清理必须证明全部后代结束与watcher首次实际完成后才恢复/复用源。一次全仓在既有V008原生认证遇到未定位拒绝，保留失败，后续全新夹具20轮和最新全仓未复现，不称已修复。随后同九例补真实安装的默认HTTP delegate与nil-Transport client的loopback正控，原counter0→1、外部server恰一真请求、完整产品窗口1→1，实际server/连接与Host均须join；定向三轮、真实hot新夹具10轮、最新全仓/双构建-vet通过。保留前一全仓hot场景native precondition win32-233失败，未定位API/根因，不以新绿或重试原凭据交换抹去；随后真实早开早关client的native red定位同一pipe accept失败后的状态收尾缺陷；已将reset注册提前且仅在原native IO实际join后运行，同handle新session/metadata及32次连接取消顺序、全安全包三轮、受影响定向三轮、新进程hot/V008场景各五轮、全仓/双构建-vet通过。它没有定位先前233/unauthorized的来源，也未改变15s idle accept预算。随后将同一归因窗口扩展为健康+共享原三十类+七个实际工作区初始化checkpoint，共38例，并由独立实际initializer补七个安装中断startup守恒；真实red发现安装SQLite自动恢复sidecar会改源，已在原native lease/固定DB身份下、SQLite之前拒绝未证明日志，不新增安装恢复或owner。组件门禁三轮、正常/tag/helper构建、tag门禁三轮、38例/安装七例定向三轮、全仓一次及正常/tag vet均0（2026-10-20 01:36）。仍不勾选T025：最小剩余是canonical missing与真实junction的完整Host有限归因；四个lexical locator拒绝、真正hot同一filename-only分支及安装启动异常以其实际独立证据/关闭分支组合，不伪造不存在的inspect或Provider计数。成功Job后代/default delegate不外推失败OS尝试或全局调用；正常生产无额外出站/exec/Provider调用点不是新增dummy framework的理由。不得把测试fixture实际未提交初始化误称生产COMMIT/硬断电或coordinator checkpoint。历史原生取消超时、实际测试owner收尾、明确无界取消限制及最终冻结见[研究T025](<research.md>)。

### V010 — 正式保存、同key调查及未知结果

- 过程：T023客户端完成真实预览/确认；admission前失败/不明、提交前/中/后中断、丢ACK、busy/模拟满盘IO、同旧base并发、同key异输入/主体、不同区同operationId、候选applied/过期/旧epoch后的原key调查各至少1例；重启后重新认证/取得排他，用新可信视图调查，不自动重放。
- 通过标准：确认前正式状态不变；同base最多1方正式成功；同区同key最多1份正式效果及唯一receipt，改变语义输入拒绝，跨区不串查；未知围栏不靠超时解除。已经提交的原receipt不被旧epoch/base误拒，不能因missing receipt自动宣告回滚/重试；当前权限不满足不泄漏结果。已提交/视图交付失败分报，无提前success/依赖执行。
- 覆盖：US3；FR-004/011–015/017/019；SC-007/008/009，G6端到端部分。责任：T026 / agent；环境：T023、真实SQLite与阶段注入、固定候选/权限fixture。2026-10-20 02:40历史证据：**partial，当时完整V010仍unverified**（后续06:16有限签收见本节末条；当前汇总见plan）。2026-10-20 02:40正常生产外部同key/跨区/同旧base场景三轮、真实15分钟过期长门一次、全仓一次及正常/tag/helper构建、正常/tag vet/diff均exit0。长门实际等待900.155s，未改TTL/时钟/候选SQL，实际session续发与完整正常退出/新进程重开均验证原receipt不重放；短全仓明确skip长门，其证据来自单独实跑而非skip。原key绑定资源/作用域与候选逐项核对，未admit loser的历史不可查不伪造durable rejected，旧candidate epoch也不被caller新epoch覆盖。随后03:16已补原admission/formal各三实际flush边界BOOL0/SQLite1034/ErrUnknown、A围栏/B保存、精确旧进程实际结束及正常新Host同原key/fresh历史reconcile；正式阶段不得丢durable admission，未mint capability的admission失败不得有正式效果，缺history不猜rollback。真实cold unknown映射曾被joined lifecycle误盖为closed，已窄修并用实际六故障及policy正反控复验；原V008 native与受影响定向三轮、全仓一次、三构建/普通及legacy panicnil门禁/双vet/diff均0。随后03:48补同原admitted连接pager cap实际SQLite13/恢复cap但不撤销unknown、原key不重arm、A围栏/B保存及精确旧进程结束后正常新Host原key同历史known rejected且正式0；mixed uncertainty的baseline/conflict优先级policy先red后窄修。三构建/普通-tag-legacy门三轮、FULL/native/V008/SQLite/HTTP三轮、全仓一次、双vet/diff均0；只改单一FULL oracle后原exe与其余214个Go/SQL不变再实际FULL/native三轮0，正确owner选择器两包三轮0。完整证据及历史匹配空选择器限制见[FULL研究](<research.md#t026-同连接-pager-full-与-mixed-uncertainty-修正2026-10-20partial完整v010仍unverified>)。当时完整checkpoint前中后/丢ACK、busy/容量、不同主体及断连尚未完成；busy后续见下条。不勾选T026；native详情见[原生保存研究](<research.md#t026-admissionformal-原生刷写未知子集2026-10-20partial完整v010仍unverified>)，源码、历史oracle失败与实际冻结结果见[当前T026研究](<research.md#t026-正常生产保存与实际候选过期子集2026-10-20partial完整v010仍unverified>)。错误映射不明时仍围栏并停止依赖写入，不能更换key试到成功。

- 2026-10-20 04:12 busy增补：原durable admission后，tag-only同源mode=ro/query_only reader实际读取workspace_head持有SHARED，直至原formal Commit与apply实际返回SQLite5；不是429/OS lease或reader-open busy。A unknown/原key只调查且不重arm、B实际确认、精确旧进程实际结束后正常新Host认证/排他/fresh同原key调查到known rejected、正式0且B receipt保持，均定向三轮通过。正常/tag/helper三构建、normal/tag/legacy panicnil门三轮、busy/FULL/native/V008及相关四包三轮、全仓一次（外部241.037s）、双vet/diff全0；220个GoSQL与固定profile最终核对未变。[完整busy证据及限制](<research.md#t026-真实只读-shared-与原-commit-busy2026-10-20partial完整v010仍unverified>)；未重跑15分钟长门，不扩展公开只读模式/第二writer。当时T026仍partial：容量、不同主体、断连、明确checkpoint前中后/丢ACK仍待补齐；容量后续见下条。

- 2026-10-20 04:22容量增补：正常生产独立Host的31个A held-body claims均由首100证明，victim只发头不发有效JSON，首final429且无臆造原key/durable admission。B在A前后两次429之间真实保存读回，压力窗<12s/产品15s读超时未改；关闭socket不冒充全部handler join。可用后A正式0/原key无history，同一有效candidate/key随后首次admit并重复同receipt，B保持，最终两区drained与原Host实际正常结束均核对。V010三轮、全仓一次（外部238.951s）、normal/实际aboard_acceptance acceptance vet及diff全0，221明确输入不变；原220源码/profile/三exe完全相同而明确复用，不虚称重建。原vet未启用tag选择器及纠正实跑均保留。[完整容量证据](<research.md#t026-正常-host-正文前容量拒绝与-b-区保存2026-10-20partial完整-v010-仍-unverified>)。当时T026仍partial：不同主体、断连、明确checkpoint前中后/丢ACK待补齐；主体后续见下条。T027–T032及001未完成，长门本轮未重跑。

- 2026-10-20 04:42主体增补：正常Host接受停机期间严格普通_test helper经原owner授权API及受保护pipe完整ACK签发的合法R session（非目标Host PID受限签发证明）。R在A真实读写成功仍查owner原操作403、同body/key POST409；原两主体receipt与B实际保存保持。停机将同R改为仅open后，新Host/新epoch可认证但自己的原key GET/POST均403；旧session403前后保守窗口排除TTL自然过期。operator可GET调查而不能POST冒充原主体；不是live撤权证明。原测试误期望operator重放R200的实际409 red与修正完整保留。当前授权/guard三轮、全部短V010三轮（主体1.93/1.81/1.70s）、全仓一次（外部245.565s）、normal/实际aboard_acceptance vet/diff均0；223明确输入/profile/三原exe未变并声明复用，仅新helper独立构建。[完整主体证据](<research.md#t026-不同主体原key与当前权限2026-10-20partial完整v010仍unverified>)。当时T026不勾选：断连、明确checkpoint前中后/丢ACK待补齐；后续wrapper边界子集见下条。T027–T032及001未完成，15分钟长门未重跑。

- 2026-10-20 05:13 checkpoint增补（局部passed，完整V010仍partial）：tag-only原Admit前、durable Admit后/formal前、原confirmed后/HTTP交付前分别实际结束精确原进程；正常新Host当前认证/排他/原key查证，分别无history正式0、原admission后reconcile rejected正式0、原confirmed receipt与唯一正式效果保持，不重放。另两例after-admit/after-confirm真实TCP RST后release原调用，必须同原请求新correlation实际Write/Flush失败，不能把RST/EOF混作业务取消；业务继续确认，原receipt/唯一效果、B真实进度、正常排空结束及新进程同receipt均核对。修正测试类型、零值证据编码和事件归属后五例各三轮0（package25.000s）；05四构建及normal/tag/legacy/定向三轮0，但全仓旧V009场景复发233且未定位，失败保留。06只分离结果/marker文件名并明确复用原四exe，226输入/profile/exe冻结无差异；五例三轮25.208s、既有失败hot场景五轮41.287s、全仓一次外部249.426s及正常/实际tag vet/diff全0，结束05:13:05+08。新夹具复验不重试旧交换，不宣称233已修复。[具体边界与历史失败](<research.md#t026-原调用边界真实断连与已确认丢交付2026-10-20partialnative-mid-commit仍unverified>)。T026仍partial：wrapper边缘不等于native tx/COMMIT内部真实进程中断，后者仍待补；T027–T032及001未完成，长门未重跑。

- 2026-10-20 native formal内部增补（定向passed，完整V010仍partial）：同原未消费admission/request、原gate/永久VFS delegate，在原formal tx.Commit恰一次且in-flight/未返回、原journal第二次真实BOOL1 sync后持有；无前置非commit sync/DB sync，固定小fixture窗口，不外推零DB写/硬断电。精确路径在hold mutex前过滤，A仍持真实排他时B实际confirm/read；精确旧进程终止/Wait后正常新Host原key/fresh同历史reconcile rejected且正式0。另真实RST/release正控原业务继续确认、同请求Write/Flush失败、同receipt与唯一效果跨正常重启保持。七checkpoint及旧native各三轮0（68.002s），tag门三轮/四构建/正确legacy补门及双vet/diff均0，231输入/profile/四exe未变；但03全仓在既有V009 malformed-descriptor的inspect前原生认证unauthorized失败，后续正控/观察未执行，不能称全仓passed，T026不勾选。[具体失败、有限覆盖与下一诊断](<research.md#t026-原formal-commit内部真实journal同步中断2026-10-20partial定向passed全仓认证failed>)。该截面已不缺formal native midpoint；当时停止依赖并调查全仓认证red，后续受影响门复验与有限范围判断见下条，而非额外增加未要求的admission-mid强杀。T027–T032/001仍待验收，真实15分钟历史门未冒称重跑。

- 2026-10-20 06:16 **T026/V010有限签收**：上列全部明确子项逐类实跑，补非密认证stage/主因-reset分离与排空后ring64诊断。当前235 GoSQL/profile/四新exe冻结，security/Host/current-auth三轮0；normal同fixture/同pipe名64次新进程各三轮=192次原server完成+当前context安装operator+实际正常结束/句柄收尾，V009原失败/healthy及两hot分支三轮0、双vet/diff0。05明确复用同04精确输入/四exe，当前全仓一次external263.991s/全部exit0且fingerprints无差异；受影响认证/inspection/Host门重新验证后，按V010每类至少1及正式/receipt/unknown门作固定profile完成判断。**不宣称旧Unauthorized/233根因已修复**，原failed仍failed、无原交换重试/wire/15s/安全弱化；新red仍停止对应依赖并定位，不把本次有限通过作豁免。历史900.155s仅沿用不变TTL规则范围，不称当前exe再等/旧hash一致。可进入T027，T027最终A/B各10条及更新/完整重启仍必须独立证明，T028–T032/001未完成。[完整证据、范围与风险](<research.md#t026-受保护pipe认证stage调查2026-10-20partial非根因修复声明>)。

### V011 — 不可延期的正式保存/完整Host重启闭环

- 过程：在无前端/旧Host/Provider/插件的环境，用外部客户端创建并登记A/B，完成各至少10条不同正文及各至少1次更新；预览确认、读回、关所有区、完整退出Host并确认旧PID结束；新进程重新认证/重开并逐条比较。检查未确认候选/手改文件不生效，不用旧内存/浏览器缓存。
- 通过标准：安装/tenant/principal/workspace/resource身份、title/body、正式version/revision/digest全部一致；零已确认丢失、零串区。存在真实独立SQLite文件而非Map/空目录/健康检查；无需后期对象/Run功能才可完成。
- 覆盖：US1/2/3/5；FR-001/011–014/017/021；SC-001/002。责任：T027 / agent；环境：T023及V008/V010通过、真实Host binary/新旧PID和NTFS合成根。当前证据：**passed（2026-10-20T06:32:29+08，固定profile）**；normal独立Host两区各10条不同Unicode正文+各更新1条，head11/rows10/version2；两份真实SQLite/物理身份、两区全部close/旧Host正常结束、新进程/native重新认证/新epoch逐条HTTP比对全部身份/内容/version/revision/digest及22原receipt，未确认候选/手改普通文件不入正式。02新测试三轮3.30/3.15/3.40s，连同V010 focused138.604s，全仓一次全部0（external266.827s），acceptance normal/tag vet/diff0、236输入/profile/四复用exe无变化。01漏required OpenEpoch的测试序列化失败保留并修复，不称存储故障。[当前证据与边界](<research.md#t027-两区各十条与更新的完整正常重启2026-10-20implemented固定profile-v011-passed>)。这是V011签收，不代表001完成；HTTP preview status/列表默认值合同偏差留T031，T028–T032仍pending，不延期M2或称完整Host重写。

### V012 — 生产路径真实第二写者拒绝与恢复

- 过程：两个真实OS进程经同一Windows适配器竞争同区，持续持有/同时竞争/异常退出各至少10轮，失败方实际尝试保存；测试正常释放、PID/普通锁内容伪造、删换锁、句柄继承、root别名、同UUID副本/停机relink。单安装第二完整Host被host.lock挡住之外，专门隔离验证writer.lock及真实区库写入路径。
- 通过标准：任意时刻至多1实际写者、失败方零确认保存；正常释放/kill后只在真实排他与持久状态验证后开放写，已确认数据保持；副本零第二可写实例，移动不换ID。测试helper不向生产开放绕授权能力。
- 覆盖：US4；FR-008–009/018–019；SC-003/008。责任：T028 / agent；环境：T023及V004通过、可终止真实子进程/观测锁、同NTFS合成库；当前证据：**passed（2026-10-20T12:10:03+08，固定profile）**。最终ordinary矩阵三轮+全仓一轮，continuous/simultaneous/known-idle kill各40实际case；精确native busy/loser未开SQLite且零receipt/无operation、唯一winner正式confirm并持锁、真实结束/重新排他后normal新Host新epoch回读两条正文/原receipt。公开copy-relink与canonical普通copy替换各4例、三种PID/普通锁内容各4例、原V008两次同卷move/relink/CAS另4份；原语真实继承/删换/metadata/别名三轮重验，8.3实际短名拒绝，不冒称无短名覆盖。当前240 GoSQL/profile/工具/四复用exe共249输入未漂移，144份独立JSON结构/hash核对；三轮94.594s、全仓全部0（external291.541s）、全仓normal/tag vet、tag四包回归/diff0。02全仓一次停机rename Access is denied及更早233失败均保留未定位；03原rename20轮/全仓绿只是有限未复现，不宣称根因修复，不加交换或rename重试。源码、命令、指纹与边界见[本轮证据](<research.md#t028-v012补证与本轮复验2026-10-20implemented固定profile-v012-passed>)。kill不证明硬件断电安全；T029–T032/001仍未验收。

### V013 — 数据/授权/局部故障与资源隔离

- 过程：A/B使用相同显示名/临时键与不同正文，交错保存/读取；未认证/失效/只有A权限身份伪造目标、resource/candidate/operation等；分别关闭A及停机合成损坏A，B均新增/更新/读取至少1条；另测A长IO/32每区、64安装准入边界及共享授权提交未知。
- 通过标准：零串区/越权/目标内容泄漏，坏A/关A不取消或阻塞B独立操作；满额在admission前429，不把未物理结束/未知当已释放名额，B仍能推进，调查/排空不自饿死。共享安全未知只阻断真实依赖并解释范围，不能绕过当前权限；进程/磁盘/机器共享故障仍可能跨区。
- 覆盖：US1/5；FR-002–003/020–026；SC-004/005。责任：T029 / agent；环境：T023、受限会话/受控授权故障和独立合成根；当前证据：**passed（2026-10-20T13:05:58+08，固定profile）**。六类真实独立Host闭环各5次（首轮+三轮+全仓）：同名同key A/B精确row/receipt与伪造resource/candidate/operation归属、A-only真实主体/缺失伪造及真实3秒过期会话、closed/corrupt A下B新增更新读取且坏源不改、原native formal COMMIT在途加30A/31B/1installation慢body的31/63 ordinary边界、真实native COMMIT unknown后62慢body下原key调查仍保留名额/锁直至显式reconcile物理结束、实际安装finalize unknown阻断7个真实授权依赖/当前伪造会话仍拒绝/新Host原安装key显式reconcile。唯一生产窄修为ErrAuthorizationUnknown明确installation错误范围，不放宽授权。251源/profile/工具/四复用exe未漂移，30份JSON结构/hash另验；定向三轮32.816s、五包原生三轮、全仓全部0（external317.915s）、全仓normal/tag vet与tag四包/diff0；原01–04失败保留，源码/命令/扫描范围与证据见[本轮记录](<research.md#t029-v013独立进程隔离与共享依赖2026-10-20implemented固定profile-v013-passed>)。32/64含控制保留、是HTTP操作限额，不改变迁移基线Native/Build预算，不代表性能SLA或共享crash/OOM/磁盘故障完全隔离；T030–T032/001仍待验。

### V014 — 断连、物理排空、旧epoch与持久历史分界

- 过程：断连/取消等待者、close中保存未结束、Context到期但IO继续、迟到回调/旧epoch提交每类至少1例；采集DB调用结束、receipt已知性、队列/epoch、连接/OS锁释放的顺序；重启调查保存unknown及原close内存历史不可查。
- 通过标准：断连不取消accepted操作；close先拒新准入，未物理结束或结果未知只报告draining/unknown，不先关锁/谎称可移动；旧epoch新写拒绝，原accepted者受控收尾，B独立。保存receipt保持持久；close重启缺历史返回不可查而非成功/回滚，当前目录状态由重新验证报告。逻辑/物理/持久与业务/目录/访问模式分别观察。
- 覆盖：US1/3/4/5；FR-004/015/019–024；SC-003/005/007/009，G6收尾部分。责任：T030 / agent；环境：T023、可控延迟DB调用和真实重启；当前证据：**passed（2026-10-20T14:29:54+08，固定profile）**。真实native两次journal sync后分别client deadline/cancel，原IO仍悬置，公开close先拒新准入/原close metadata可查询、B新增更新读取且writer锁不提前释放；明确release后原保存committed/known、close drained但持久unavailable，实际排他重取后new epoch多维重开，旧epoch新key拒绝/原key同receipt；新normal Host显式open后持久save receipt相同、旧close历史不可查。10份独立JSON（两mode各5）结构/hash及253输入/profile/四新exe均核对无漂移；定向三轮、四包原生三轮、全仓全部0（external319.604s），normal/tag全仓vet、tag四包/diff0。原Host409及精确unit red保留，唯一窄修为drain期间精确原close只读metadata借原allocation，不放宽一般queue/授权；原未知IO/close及冷调查实际包回归仍保留围栏。详见[来源与边界](<research.md#t030-v014真实等待者结束与物理关闭2026-10-20implemented固定profile-v014-passed>)；T031/T032/001仍待验，不外推硬断电、永久可靠或物理safeToMove提前成立。

### V015 — 当前实现回归、秘密与范围核对

- 过程：未来运行 `go test ./...`、`go vet ./...`、`go build ./cmd/aboard-host`，重验V002及受影响V008–V014；有经验证Windows/cgo工具链才运行 `go test -race ./...`，否则单列skipped及环境原因，不能替代业务证据。检查唯一结构源/绑定漂移、本地链接、公开DTO/错误/日志/默认输出秘密，以及Git忽略/未跟踪文件、业务范围和证据指纹。
- 通过标准：目标构建/必需回归与严格反例提供当前证据，敏感材料/工具环境不进版本控制；对象/Run/事件/备份/已有schema升级/旧库导入/跨区协作保持明确unavailable/deferred，无假数据/空成功或第二写者。race/编译/静态只证明各自覆盖，不填补缺失的V008–V014；硬件断电/未定义SLA仍不作通过声明。
- 覆盖：US1–6；FR-010/016–017/024–028及受影响SC-001–009。责任：T031；T032汇总全部V / agent；环境：当前源码/契约/fixture指纹、T023验收环境；当前证据：**passed（2026-10-20T14:51:34+08，固定profile；race环境不足单列）**。公开真实red确认preview200/合同201与默认limit20/合同100，按唯一OpenAPI仅修正dispatch两处；独立201无正式效果、101条逐字段/receipt分页及受限能力用例首轮/三轮/全仓均通过，10份JSON结构/hash核对。静态16操作/32schema/引用闭合/9反例，单源生成漂移及独立16字节正例/42拒绝/88变异通过；五包三轮、全仓一次、全仓normal/tag vet与tag五包均0。四exe实际02重建、03仅修正未链接旧_test成功oracle后精确复用，261源码/契约/夹具/工具/profile/image输入无漂移；02旧Host测试want200的red保留。当前CGO0执行race实际exit2（`-race requires cgo`），未跑race且未安装/放宽系统工具。全局路径/失败尝试/自定义网络/硬断电等未承诺项保持局限，无必需SC被排除；本地链接与最终V001–V015归并由T032当前文档复核，不把静态当业务通过。详见[完整结果](<../../.data/logs/001-validation/t031-bindings-03-results.json>)与[独立证据复核](<../../.data/logs/001-validation/t031-bindings-03-evidence-check.json>)。

## 任务生成时的历史质量记录（非analyze或运行验收）

以下只描述任务生成交付时的事实与当时下一门；后续批准/执行不改写该历史，当前功能授权仅见[授权记录](<authorization.md>)。

- 上游七份权威输入指纹未变。主Agent与独立只读审阅均未发现阻断任务拆解的上游语义冲突；不是技术方案已验证。
- 独立复核曾发现T002/T003的Go验收时点与T004建立模块之间可能形成隐性环。已拆V002a静态冻结/V002b模块后执行，调整依赖及首批顺序；定点复核确认关闭。完成门与后续联合V归属也已明确分开，不靠提前勾任务消除依赖。
- 机械自查：32项均未勾选、15组V均保留unverified、依赖引用完整且无环、14个本地文档链接/锚点有效；全部六个US、28项FR与九项SC有任务/验证归属。该检查只证明文档结构和引用，不证明需求覆盖的运行正确性。
- 诊断仅存忽略的 `.data/logs/`。本轮无go.mod/代码/机器契约实现，无技术G/V执行；未调用speckit-analyze、未改其hook，未提交/push、未运行Provider/插件、未迁移真实数据。下一门是用户任务评审，再按规则只读analyze并另行取得实现授权。
