# 独立 Host 验收 harness

状态：implemented（T023 可控性）；各项完整 V008–V015 仍由 T024–T031 执行，不因 smoke 通过而自动通过。

## 运行

在仓库根的 Windows amd64、本地 NTFS 和已验证 Go/SQLite profile 下串行执行，所有构建、配置和合成数据只放 Git 忽略的诊断目录：

```powershell
$env:GOPROXY = 'off'
& ./tools/go.ps1 -GoArgs @('build','-o','.data/logs/001-validation/acceptance-host.exe','./cmd/aboard-host')
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
$env:ABOARD_ACCEPTANCE_HOST_EXE = (Resolve-Path '.data/logs/001-validation/acceptance-host.exe').Path
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-count=3','-v','-timeout=900s')
if ($LASTEXITCODE -ne 0) { throw 'acceptance harness failed' }
```

未配置可执行文件时，独立 Host 场景明确 skip，不是验收通过。helper 的顶层 skip 表示仅由实际子进程专用模式调用；父场景必须取得子进程证据及实际退出结果才通过。`tools/go.ps1` 的准备锁要求一次只运行一个 runner。

## 边界与证据

- 客户端启动正常生产 CLI 的显式 init/serve；只通过实际受保护 named pipe 取得 bearer/只读前置版本，再用真实 HTTP 预览、确认、读取，不调用内存 Host Run/内部存储代替业务链。bearer 不进参数、环境或文件。
- 每个 fixture 有独立 `t023-*` 根、安装、allowed root、pipe 名和动态 loopback 端口。启动时冻结实际 PID、内核 creation FILETIME、image 和单独进程 handle；不相信 ready 报告的 PID 单独构成身份。可执行文件 SHA256、Go 版本、真实起止时间及安全 Host 事件写入合成根的 process 证据。
- 正常停机使用独立 console helper，核对原进程身份、隔离 console 恰含 Host/helper，再发 BREAK，沿生产 os.Interrupt 路径排空。要求正常 exit、stopped 和实际锁释放；超时后的测试强杀只记 forced，绝非 graceful/safeToMove。
- fault helper 仅在测试二进制中，验证合成前缀、实际 image/FILETIME 后终止已知空闲 Host，等待实际结束；这是进程 kill 可控性，不是事务阶段、FSYNC 故障或硬断电验证。后续矩阵的阶段刺激/收据观测必须另有真实证据。
- writer helper 同样只存在于测试构建，实际子进程先用相同 Windows writer.lock 适配器取排他，失败者不得打开 SQLite；成功者对合成既有库执行固定真实预览/admission/正式提交，持续持锁到显式释放。须取得相同 receipt、真实进程结束并由新 Host HTTP 读回正文。这里不是只测 host.lock，也不把单轮 helper 可控性当十轮 V012。
- fixture、可执行文件及 helper 目标拒绝越界和 reparse 祖先；真实 junction、错误 FILETIME/image 负例必须不触发错误目标。该硬门禁不宣称抵抗恶意同 SID 并发替换或建立系统沙箱。
- 有界 helper 失败输出只保留在合成诊断根，默认错误不回显任意子进程字节。保留失败 fixture，不自动删库、换 key 或重放未知操作。

现有 smoke 是单区单条记录的完整旧进程结束/新进程重开及原 key 同 receipt 基础证明；A/B 各十条、更新、十轮锁竞争及隔离/排空矩阵均仍属于对应后续责任任务。

## T024/V008 五阶段中断子集

此子集另构建严格限定的测试 Host，不把故障环境解析或控制命令放入正常生产构建：

```powershell
& ./tools/go.ps1 -GoArgs @('build','-tags=aboard_acceptance','-o','.data/logs/001-validation/acceptance-stage-host.exe','./cmd/aboard-host')
if ($LASTEXITCODE -ne 0) { throw 'stage build failed' }
$env:ABOARD_ACCEPTANCE_STAGE_EXE = (Resolve-Path '.data/logs/001-validation/acceptance-stage-host.exe').Path
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','TestV008CreationDurablePhaseCrashes','-count=3','-v','-timeout=180s')
if ($LASTEXITCODE -ne 0) { throw 'creation phase subset failed' }
```

普通 `ABOARD_ACCEPTANCE_HOST_EXE` 仍必须配置成上节的生产构建，且没有额外 build tag。每个阶段使用新合成安装：测试 Host 在实际原创建请求的 `intentRecorded` / `directoryPrepared` / `genesisConfirmed` / `descriptorPublished` / `registrationConfirmed` 持久 checkpoint 后停住，外部 helper 用已固定的 PID/FILETIME/image 中断，再由**未武装的正常生产 Host**重新认证/查询/显式 reconcile。回调只捕获该请求的原 key/路径，复用同一个 Store/资源 owner；没有全局当前操作、HTTP 故障 API、SQL 旁路或新增生产配置字段。

有界 JSON 完成并关闭后才创建零字节 ready 文件；读取方/helper 仅在 ready 存在后读 JSON，不在活动原生祖先 pin 下做 rename，不把文件刚出现视为写完。两个诊断文件位于合成 fixture 根，源工作区清单/每文件字节单独比较。前三阶段只查证/拒绝补建，描述发布后可完成原登记，已登记结果只核实而不重放。未完成安装的强杀收尾明确记录 `unresolved-installation-cleanup`，不是正常排空证明。

此刺激使前四个持久结果仍为 **accepted**，第五个已为 **committed**；请求交付未知不被冒充成持久 unknown。该子集不证明原生 COMMIT/FSYNC/finalize-unknown 故障、硬断电或完整 V008；登记/同卷移动及安装异常由下列独立矩阵补充，不能借用阶段中断结果。

## T024/V008 登记、移动与安装异常子集

使用上述**正常生产构建**，无需 stage build：

```powershell
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','TestV008Nonempty|TestV008Registration|TestV008ConcurrentFirst|TestV008InvalidInstallations','-count=3','-v','-timeout=180s')
if ($LASTEXITCODE -ne 0) { throw 'binding/installation subset failed' }
```

- 非空原目录逐文件清单/字节守恒、初次 `DIRECTORY_NOT_EMPTY`；按 intent-first 设计查到原已拒绝安装 key，重复不重新创建，而不是要求安装库无记录。
- 同 root 同身份以原 key/新 key 重复 register，逐次登记版本不变；正常停机后只复制**本测试所有的合成工作区**作为副本拒绝刺激，`IDENTITY_CONFLICT` 且两份字节不变。它不是备份、迁移或跨安装授权功能。
- 实际公开 close→查 operation 直到 physical drained→同卷 `os.Rename`，原生 root FileID/卷不变；显式 relink 增版本、无隐式 open。第二次合法 rename 的 stale version 负例与 current version 正控分离，不用“已是当前路径”冒充 CAS 验证；新 epoch 重开仍读到同一正式正文/原 receipt，旧定位不补建。
- 两个实际 init CLI 原生进程 FILETIME 生存期必须重叠，仅一个 initialized；失败者及重复 init 均是固定 exit1/HOST_UNAVAILABLE。重叠不是精确 native lock-stage barrier，writer.lock 同时竞争的十轮职责仍在 T028。
- 缺安装、空安装、缺 material/DB/host.lock、坏 material/DB 七类均要求 clean refusal、stdout 无 ready/其他事件、两流有界且不含 locator、源清单/字节不变；deadline 强杀或任意非零崩溃不是通过。

证据保留在各自合成根的 `nonempty-evidence.json`、`registration-relink-evidence.json`、`initialization-race-evidence.json` 与 `invalid-installation-evidence.json`；没有 SQL 客户端旁路或 bearer 落盘。本节不包含登记 finalize 的原生未知负例；该证据须由下列独立原生故障场景补足，不能借用本节成功路径。

## T024/V008 原生 finalize 未知与残留拒绝

仍使用前两节配置的**正常生产 exe + 独立 aboard_acceptance exe**：

```powershell
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','TestV008','-count=3','-v','-timeout=240s')
if ($LASTEXITCODE -ne 0) { throw 'V008 failed' }
```

- `TestV008NativeFinalizeFlushUnknown` 在 tagged Host 的**原请求、原 Store.mu、原安装连接、原 FinalizeCreation**调用周围武装，只限完整合成 fixture、确定 key、固定 target、fresh 单 owner、源 DB 已真实 Close 且无其他区/inspection/控制交付。正常构建 helper 仅原样调用 adapter，忽略两种故障环境；没有新 HTTP/pipe 指令或生产配置字段。
- Tagged 包在 main 前只设置**一次**进程常驻的 win32 VFS 原生 delegate；不能在活动 SQL 间反复换函数指针。普通调用仍执行原生 FlushFileBuffers；只有已武装原请求的精确安装 DB/`-journal` 实际 handle 路径才计数，在指定 ordinal 对真实只读、单链接、普通 disk 配置文件 handle 调用原生 FlushFileBuffers。要求真实 ERROR_ACCESS_DENIED 控制、返回 BOOL0、SQLite 1034 与 ErrUnknown；没有伪造 errno、返回 -1、改依赖或SQL结果。
- 回调不触 Store/SQL；atomic disarm 后还须以状态 mutex 等真实回调结束，inactive 后才 Close 刺激。默认/legacy panic(nil)也做相同收尾，panic只得保守未知、没有伪造SQLite1034。配置/生命周期/粘滞Close的policy单测不被当成原生故障证据。
- baseline0 观察本固定组合三个刷写边界，然后各一个全新安装/真实外部进程逐项失败；同进程原 key 与context仍因共享授权未知被围栏，工作区物理锁可重取但安装锁不释放。显式强杀仅为原生身份固定的测试收尾。**正常生产 Host**全新进程重新认证、读取原durable key/phase并显式reconcile，原UUID、完整初始授权、genesis/descriptor字节不变且记录仍空；不重放创建或保存。`finalize-evidence.json`是该tagged Host的有界安全原生证据，`native-finalize-result.json`关联客户端冷调查及源指纹，不含bearer/SQL/native错误文本。
- 观测精确为“原 finalize 返回真实 FSYNC1034/ErrUnknown”，并未另给 COMMIT-vs-rollback-error 分支装trace，不把未知推成已回滚或断电耐久证明。前三个早期残留亦实际请求未登记open（故意非法的最大版本负例，不猜有效前置版本）、新key register、新key create、inspect，拒绝且不授新权/不补建；再核对原操作和源清单/字节。open 的403证明当前授权门先拒绝，不把它称为已授权源码恢复测试。

## T025/V009 坏目录、两库 ledger 与有界观察（有限验收passed，固定profile）

使用正常生产构建；完整验收包同时配置前文 stage exe 与下文 `ABOARD_ACCEPTANCE_SQLITE_HELPER_EXE`，以免 V008 或实际初始化场景被 skip：

```powershell
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','TestV009','-count=3','-v','-timeout=900s')
if ($LASTEXITCODE -ne 0) { throw 'V009 bad-directory subset failed' }
```

- [工作区/安装矩阵](<inspection_windows_test.go>)用实际独立生产进程、受保护 session/版本和 HTTP 检查三十类工作区样本及八类安装 ledger/schema 样本。受限 SQL helper 只生成本测试已正常停机的合成坏样本，先取得同一原生 lease；它不读业务结果、不代替客户端操作或补建库。
- 工作区逐例检查 inspect 分类、原登记已授权 open 的明确拒绝、前后递归清单/每文件字节；恢复刺激后的测试清单也必须等于原基线。有效但 business unavailable 的描述允许 inspect 成功，但仍拒绝 open，不能把它伪装成坏格式。部分 open 仍使用通用 RECOVERY_REQUIRED 映射，这些行只证明拒绝，不宣称唯一底层诊断。每个 A 负例后，B 都实际预览、确认并读取一条新正式记录，而非仅查询健康状态。安装负例要求固定 clean refusal 和源守恒，不以超时/任意崩溃替代。
- [原生 NTFS 观察](<inspection_observer_windows_test.go>)对 inspect、原版本查询、清单读取及随后 open 的**合并窗口**监测递归变更；不按 PID 或单一操作归因。原字节重写、创建/删除、rename、属性以及已存在嵌套文件重写正控验证灵敏度；idle/GC 控制验证异步 buffer 固定与实际取消/完成加入。两秒是通知等待门，不是物理取消或排空时限；未完成 IO 不得通过提前 Unpin/关句柄伪称结束。观察先结束才恢复坏样本或提交 B。
- [路径/私有材料补充](<inspection_boundaries_windows_test.go>)覆盖已登记源消失、真实 junction 与相对/UNC/device/out-of-allowed-root locator；missing/junction 实际调用当前登记的授权 open 并拒绝，不以授权403冒充存储拒绝。别名正控用 native REPARSE_POINT 加有意跟随**两个确定测试路径**后的卷号/FileIndex 比较；本机 Go EvalSymlinks 曾保留 junction 原拼写，不能借它的返回值假称 referent 已证。harness 的 no-reparse 硬门禁不改变。每个局部负例后 B 实际 open/预览/确认/读回；做目录移动/alias 刺激前先正常 close B，避免测试自身违反 native ancestor pin 的命名空间边界。
- ACL 刺激仅更改测试自有安装根/材料的 DACL，不改系统策略、owner、SACL 或权限提升。父目录继承可影响子文件，故捕获根、bootstrap、安装DB、锁四个对象的 owner+DACL；产品拒绝前后必须逐字相同。root case 的子文件先固定成 owner/SYSTEM protected 的合成基线，并实际正常启动/认证/停机确认这份基线可用。测试收尾只忽略 SetSecurityInfo 已观察到的 AUTO_INHERITED bookkeeping 位差异，不能忽略 owner、protection、ACE顺序/权限/SID；这不是产品 ACL 修复能力。
- DPAPI 三例分别是 tampered ciphertext 的实际 native Unprotect 拒绝、可正常 current-user DPAPI roundtrip 的坏 JSON、可解密却与安装数据库绑定不同的合成身份/密钥。没有解密原始真实材料、没有 wrong-user/系统DPAPI不可用结论，也不把 tamper 拒绝指认为特定 MAC 内部分支；原刺激恢复后正常生产 CLI 仍须真实认证/退出。所有 startup 拒绝要求 clean exit1/HOST_UNAVAILABLE、无ready/locator泄漏及原安装字节不变。
- [真实 hot 日志](<hot_journal_windows_test.go>)由独立测试写者取得实际 writer lease/DB pin，在固定 SQLite 上执行未确认的巨大 display_name 更新。父进程先固定 helper 内核身份/FILETIME/image 才发 stdin 许可；进程常驻静态 delegate 仅转发真正 FlushFileBuffers，在第三个成功刷写且实际 handle 路径为源 DB 时直接退出，保留真实同步 DELETE journal。没有合成/截断日志、假造 BOOL/errno、生产 hook 或硬断电结论。
- 随后全新**正常生产 Host**的 inspect 必须 RECOVERY_REQUIRED，源字节/有界通知均不变。健康原历史的显式授权 open 应恢复，而非一律拒绝：逐字段读回原正文/版本/摘要/receipt，原 key/旧 epoch 仍同 receipt，revision与唯一记录不变，正常退出后整个源树恢复到事务前精确字节（包括未公开的 display_name）。另例在真实 hot 事务前损坏已确认 immutable 内容，故私有副本 rollback 后仍须拒绝坏历史；源热日志与全部字节不变，实际源 writer 排他可重取。两例 A hot 时 B 均真实新增确认/读回，不把 B 的写混入 A 通知窗口。`actual-hot-*.json`记录原生成功刷写/日志头/实际进程身份及源指纹，日志头观察本身不冒充全 journal 校验；正常 open 的实际恢复才是正控。
- 可单独复验 `-run '^TestV009IndependentGenuineHotJournal$' -count=3 -timeout=180s`；helper 模式只由场景自动用于严格合成目标，不是公开故障 API。
- [原生路径完整归因补充](<inspection_path_io_windows_test.go>)在同一严格 `observed-source` gate 下单独覆盖已登记 canonical missing 与真实 junction，和下文38例合计40个具体归因样本。移动前原Host实际正常结束；只观察 parked 实际源树及父目录浅清单/属性，不递归穿越junction。真正reparse属性和两个确定路径的native卷号/FileIndex为别名正控。inspect须零scratch/额外PID数据写，Job/defaultHTTP保持原真实正控后的数值；随后原登记授权open须拒绝，父目录合并通知窗/源树/命名空间不变。B再实际预览/确认/读回；原Host、Job成员、watcher和HTTP server/连接全部实际结束后才恢复源，失败路径不defer修改或删除它。最后新正常Host重开原UUID/genesis0并保持原字节。可单测 `-run '^TestV009IndependentInspectorNativePathIO$' -count=3 -timeout=180s`；不扩大observer任意locator权限或宣称全局trace。
- [独立PID写守恒](<inspection_io_windows_test.go>)须另配置前文 `ABOARD_ACCEPTANCE_STAGE_EXE`；可单测 `-run '^TestV009IndependentInspectorKernelIO$' -count=3 -timeout=900s`。instrumented构建仅私有严格门禁启用；正常生产忽略观察环境。健康、共享原三十类坏目录及七个实际工作区初始化checkpoint，共38例均比较真实Host PID的完整inspection/资源Close前后WriteOperation/WriteTransfer与独立scratch成功写下界，三个预填充同字节原生1/65535/65536写控制各须1次/n字节，未跟踪的一次写必须破坏守恒。控制和结果发布在计量窗外；源通知与字节仍单独核对。B只在窗外实际新增确认读取，最终Host必须真正正常stopped/exit0，不能用ready可见代替诊断句柄Close。tag门禁另用真实受保护native handle验证未知保留原Store/quota，仅已证的测试native结束可清测试围栏。
- [同窗口成功后代观察](<inspection_process_windows_test.go>)已合并进上条 `TestV009IndependentInspectorKernelIO` 的38例，同一命令复验，同时配置正常/tag exe。实际原Host在请求前加入显式无名Job，核查无breakaway/kill-on-close/active限制和成员身份；tag-only真实同exe固定无效CLI自子进程正控先实际Wait/exit2，parent与Host均须观察累计总数1→2、完整inspect窗口2→2/active1、原Host正常结束2/0。故可抓短命成功子进程，不把单看active零当没有创建；不覆盖startup、失败CreateProcess、broker或任意全局执行。子进程固定参数在config/Host前被拒绝，不是新增CLI模式/Provider能力；测试没有伪造零counter。失败后仅全部Job成员actual结束且原watcher首次物理完成才可恢复/复用源，强杀只记failed cleanup。
- [默认HTTP delegate观察](<inspection_http_windows_test.go>)也合并到同一38例/命令：tag构建在main execute前捕获并委托真实http.DefaultTransport，计数实际RoundTrip。严格当前operator/fixture门下、产品Before前一次真实nil-Transport/Jar client到测试自有127.0.0.1端口/随机challenge的GET须使counter0→1，外部server也恰有一次请求，且不存在Authorization/Cookie/Proxy-Authorization任何header值；完整产品窗口1→1。禁止redirect，有界精确body/EOF与Close，未证结束保留原inspectionOwner/quota。实际server/handler/connection全部join后才恢复/复用源，最终仍要求Host真实stopped/exit0。正常构建不安装观察器或发请求，不是新增Provider能力。它不计raw/native/custom客户端、DNS、子进程网络或delegate内部重试，不能叫全局网络调用零。
- 清单/字节、有界通知、受控PID数据写守恒、Job成功后代和限定默认HTTP delegate**不等于**失败写/创建尝试、全路径trace、全部样本计量或全局网络/Provider调用。本轮38例包含实际工作区初始化中断；路径/ACL/DPAPI、真正hot与安装startup样本的独立证据不自动升级成同一38例的PID/Job/defaultHTTP计量。当前正常构建未发现额外出站HTTP/Dial/exec/Provider实现，不能凭空增加dummy counter或以假想通路扩大本片；也不能外推失败OS尝试、全部传输或整个Host生命周期。历史过程`-03`全仓V008未定位认证拒绝、HTTP `-02`全仓真实hot场景native precondition `win32-233`均保留failed，不互认根因、不对失败凭据交换自动重试。另以真实client早开早关native red定位并修复同handle accept失败的状态收尾空洞：reset仅在原IO实际join后运行，不重建first-instance或放宽身份；同handle session/metadata、真实pending取消和32次顺序用例及全仓已复验。该缺陷并非先前233/unauthorized的根因证明；15s从idle accept前起算的预算边缘未在本节修改，新绿也不能说间歇失败根因已定位。各子集与最终同源冻结结果见[主研究记录](<../../specs/001-host-workspace-foundation/research.md#t025v009-原生路径归因与有限验收签收2026-10-20passed固定支持profile>)。2026-10-20 02:03的完整V009三轮、全仓一次、全部构建/门禁/vet已通过：上述38例加原生路径2例及其他类别独立真实证据组成有限V009签收，不把所有类别虚称同一artifact或全局trace。T026–T032及整个001仍未完成。

## T025 实际 SQLite fixture 初始化中断

```powershell
& ./tools/go.ps1 -GoArgs @('test','-c','-o','.data/logs/001-validation/acceptance-sqlite-helper.exe','./internal/storage/sqlite')
if ($LASTEXITCODE -ne 0) { throw 'sqlite helper build failed' }
$env:ABOARD_ACCEPTANCE_SQLITE_HELPER_EXE = (Resolve-Path '.data/logs/001-validation/acceptance-sqlite-helper.exe').Path
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','TestV009IndependentInterruptedInstallationInitialization|TestV009IndependentInspectorKernelIO','-count=3','-v','-timeout=900s')
if ($LASTEXITCODE -ne 0) { throw 'interrupted initialization acceptance failed' }
```

- [测试专用 initializer](<../../internal/storage/sqlite/acceptance_initializer_windows_test.go>)与原组件 helper 共享实际语句序列：configured-open、schema、ledger、schema metadata、user_version、identity、Head/owner principal七个checkpoint。安装/工作区各七个独立子进程；全部在COMMIT之前，不把它们冒充不同durable phase、生产initialize函数注入、硬断电或T024 coordinator checkpoint。
- launcher先固定实际kernel process handle/FILETIME/image再发stdin许可；helper仅在严格有界config、准确未reparse合成fixture/root/fixedDB与合法Genesis后取同一native writer lease和DB pin。只截断已正常关闭的本测试合成DB，保持原锁到外部实际中断/Wait；无用户SQL、secret落盘、生产hook或新公开指令。
- 工作区七例在同一38例的实际PID写守恒/Job/defaultHTTP/原源字节与通知窗口检查inspect与open拒绝；窗口后B须真正新增确认/读回，正常Host实际结束。安装七例由[独立启动场景](<initialization_windows_test.go>)要求固定clean refusal和全源清单/字节不变，不借用工作区observer的计量。
- 这些安装样本曾真实暴露启动时SQLite自动恢复sidecar改源。现有安装Open在SQLite之前通过原native lease/固定文件身份检查拒绝未经证明的journal/WAL/SHM；没有新增安装恢复、补建、默认数据或降级路径。测试仅在所有原资源首次Close已证后删除fixture；返回nil但带native/SQLite physical-close-unknown同样保留所有权和源。

## T026/V010 正常保存、旧 key 与实际过期（partial）

- [保存矩阵](<save_matrix_windows_test.go>)只运行正常生产exe和公开HTTP/受保护认证：确认前Head/记录均不变；A/B同operationId独立确认；变candidate的原key拒绝；已应用/旧epoch/调用者previewVersion改变仍查询原receipt；两个同时发出的同旧base更新至多一个确认，实际结束旧进程后重新认证/打开仍无重放。核对请求key、workspace/resource/version/revision/digest/完整正文和精确记录集合，不以row与receipt相互一致替代候选resource身份。
- 当前完整owner串行命令使同base loser在admission前拒绝，故GET该key是409 `OPERATION_HISTORY_UNAVAILABLE`，不是伪造durable rejected row或rollback证明。原候选自身epoch也已退休，仅替换调用者epoch不能把它静默重基线；仍须`STALE_HANDLE`。记录实际并发`STALE_BASELINE`，不误写后续重开探测的错误码。
- [实际过期长门](<save_expiry_windows_test.go>)显式等待正常产品15分钟TTL；不改时钟、TTL、候选SQL或存储模型。未确认候选保持当前base/live epoch；到期后通过同Host的真实native pipe重新取得短期session，分别检查过期新key拒绝、已确认且已过期原key相同receipt，再以全新候选真实确认作正控，完整退出/新进程重开后再查原receipt/精确正文。实际运行结果以研究冻结记录为准，门未执行不得称过期通过。
- 长门默认明确skip，避免普通短回归隐式等待15分钟；完整V010证据必须另运行此门。它不能由组件`Admit(now+48h)`、旧epoch拒绝或睡几秒冒充。正常生产exe仍须先按前文设置`ABOARD_ACCEPTANCE_HOST_EXE`。

```powershell
$env:GOPROXY='off'
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentSaveOriginalKeysAndScopes$|^TestV010IndependentSameOldBaseHasOneWinner$','-count=3','-v','-timeout=180s')
if ($LASTEXITCODE -ne 0) { throw 'normal save subset failed' }
$env:ABOARD_ACCEPTANCE_EXPIRY_WAIT='1'
try {
  & ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentRealCandidateExpiry$','-count=1','-v','-timeout=22m')
  if ($LASTEXITCODE -ne 0) { throw 'actual expiry gate failed' }
} finally { $env:ABOARD_ACCEPTANCE_EXPIRY_WAIT='' }
```

- 当前`t026-save-matrix-03`实际长门一次case901.27s（artifact实等900.155s）、正常保存三轮、全仓一次、三构建及正常/tag vet/diff全部exit0；[冻结研究](<../../specs/001-host-workspace-foundation/research.md#t026-正常生产保存与实际候选过期子集2026-10-20partial完整v010仍unverified>)保留源码、命令和实际时间。长门是单独一次，不把短全仓skip写成多轮过期通过。
- 本节不替代admission/formal原生故障、IO/full、实际SQLite busy、容量、不同主体、保存中断、断连及重启unknown调查。其他外部子集须逐项承担这些类别；整个V010/001不得提前签收。未定位的历史native认证错误继续保留，不对连接后的凭据交换自动重试。

## T026 admission/formal 原生刷写故障（partial）

- [实际原gate场景](<save_native_windows_test.go>)分别测admission/formal的真实flush baseline，再对每个ordinal用新fixture实际readonly FlushFileBuffers失败，检查BOOL0/SQLite1034/ErrUnknown、同key不再arm、A读及writer排他围栏、B真正确认读回、原进程精确强制结束和正常生产新进程原key查询/reconcile。基线当前各3，不能把总ordinal1–6当永久API。两个CLI均须配置；故障仅tag严格合成fixture/store/workspace/principal/key/当前gate epoch/固定save-target可用，正常构建忽略环境并原apply一次。
- formal必须保留此前durable admission；admission失败未mint capability，所以不得出现正式效果或receipt。缺admission仅可归于admission故障且保持history-unavailable，零head不叫已证明rollback。cold查询可信receipt须原样保持；无receipt的cold历史不能被reconcile变成业务提交。强杀不是正常排空/断电；完成后必须重新认证、实际重新取得排他与fresh history，不能换key试到成功。
- 共用原唯一永久VFS delegate/CAS，记录callback drained不等于原DB/锁已排空。刺激/report的Close未知留原openResources/quota/安装停机围栏；不建立第二业务写者、通用故障端点或恢复工具。真实cold unknown的HTTP错误曾被合并ErrLifecycle误盖成WORKSPACE_CLOSED，现unknown优先；普通known closed正控保留。

```powershell
$env:GOPROXY='off'
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentNativeSaveFlushUnknown$|^TestV008NativeFinalizeFlushUnknown$','-count=3','-v','-timeout=600s')
if ($LASTEXITCODE -ne 0) { throw 'scoped native save/creation subset failed' }
```

- 当前`t026-native-save-06`正常/tag/helper构建、普通及tag/legacy panicnil门禁三轮、原V008 native与新六故障/两baseline及受影响四包三轮、全仓一次、正常/tag vet/diff全部exit0；外部定向42.367s/全仓233.224s，只是实测非SLA。完整输入/运行结果见[原生保存研究](<../../specs/001-host-workspace-foundation/research.md#t026-admissionformal-原生刷写未知子集2026-10-20partial完整v010仍unverified>)。本节不覆盖明确提交前中后checkpoint/丢ACK、真实SQLite busy/FULL、容量、不同主体和断连；完整T026/V010及001保持未完成。

## T026 同连接真实 pager FULL（partial）

- [实际FULL场景](<save_full_windows_test.go>)在正常原preview/admission之后，严格tag门使用原capability/epoch/连接实测page/free/cap，free0下暂设max_page_count=current page_count，仅原CommitAccepted一次真正得到SQLite13。旧cap恢复仍保留原unknown；不是OS磁盘耗尽、假Go错误、第二业务写者或持久header补丁。正常构建仍忽略全部控制输入并原apply一次。
- A原key关联/receipt缺失、unknown不伪装rejected、同key不rearm、原锁围栏、B实际确认与读回均核对；精确旧PID/birth/image及实际FULL13报告后才允许私有终止，实际Wait/重新排他后正常生产新Host重认证/新epoch查询原durable admission并显式reconcile。无receipt历史只收尾known rejected/Head0/rows0，不重放，B原receipt保持。强杀不称safeToMove或正常排空。
- 配置既有正常/tag/helper exe后执行：

```powershell
$env:GOPROXY='off'
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentSavePagerFULL$|^TestV010IndependentNativeSaveFlushUnknown$','-count=3','-v','-timeout=180s')
if ($LASTEXITCODE -ne 0) { throw 'scoped FULL/native subset failed' }
```

- `t026-full-03`三构建、正常/tag/legacy门及定向三轮、全仓一次和双vet/diff全0；复审只加强FULL oracle，沿用原exe、其余214个Go/SQL不变，`-04`实际FULL/native三轮36.389s通过；`-05`补正确owner前缀两包三轮且215个输入未变。全仓证据归`-03`，不称`-04`再次全仓。mixed-baseline/unknown policy red后只窄修HTTP优先级，未知仍隔离。输入/运行/实际page44/free0/code13与限制见[当前FULL研究](<../../specs/001-host-workspace-foundation/research.md#t026-同连接-pager-full-与-mixed-uncertainty-修正2026-10-20partial完整v010仍unverified>)。当时真实SQLite busy、容量、不同主体、断连、checkpoint前中后/丢ACK仍待执行；随后busy/容量已局部复验，主体后续见下节。T026/V010及001不提前签收。

## V010 不同主体原key及当前权限（partial）

- [实际外部场景](<save_principal_windows_test.go>)运行正常Host。仅在Host实际正常退出后，[普通installation测试进程](<../../internal/installation/acceptance_principal_windows_test.go>)通过原owner受保护session/授权API设置精确合成主体R，并以原pipe完整ACK向父进程交付仅内存bearer；必须成功关闭pipe/Store并实际退出才重启Host。不是公开授权入口、SQL、tag绕过或目标Host PID签发R的证明。
- R在A真实可读写，仍不得查owner原操作（403）或用完全相同body/key重放（409）；自身保存及原receipt/B进度保持。停机后把R改为仅open，正常新进程可认证但查/重放自己旧结果均403，旧session403且前后保守五分钟窗口排除自然过期。operator可以GET调查R原receipt，不能以管理员bearer POST替代R原主体。停机替换权限不是live撤权证明。
- 在已有正常/tag/SQLite helper配置之外，显式构建新的普通installation测试helper：

```powershell
$env:GOPROXY='off'
$env:ABOARD_ACCEPTANCE_INSTALLATION_HELPER_EXE = Join-Path (Get-Location).Path '.data/logs/001-validation/principal-installation-helper.exe'
& ./tools/go.ps1 -GoArgs @('test','-c','-o',$env:ABOARD_ACCEPTANCE_INSTALLATION_HELPER_EXE,'./internal/installation')
if ($LASTEXITCODE -ne 0) { throw 'installation helper build failed' }
& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentPrincipalKeyAndCurrentAuthorization$','-count=3','-v','-timeout=3m')
if ($LASTEXITCODE -ne 0) { throw 'principal subset failed' }
```

- 未配置此helper明确skip，不冒充通过；helper硬门禁/uncertain sticky为policy测试，不冒充真实native Close失败。实际02错误测试预期409/200、修正和03通过已保留于[主体研究](<../../specs/001-host-workspace-foundation/research.md#t026-不同主体原key与当前权限2026-10-20partial完整v010仍unverified>)。04最终于2026-10-20T04:42:01+08:00局部passed：expiry排除后短V010三轮58.356s（主体1.93/1.81/1.70s）、当前授权/guard三轮2.141s、全仓一次（外部245.565s）、正常/实际aboard_acceptance双vet及diff全0，223输入/profile/复用三exe冻结无差异，新增helper单独构建。当时断连、明确checkpoint前中后/丢ACK以及T027–T032未签收；wrapper边界后续见下节。15分钟长门本轮未重跑。

## V010 原调用checkpoint与真实交付失败（原五例局部证据）

- [原五个独立Host场景，现另增两native例见下节](<save_checkpoint_windows_test.go>)使用现有正常/tag exe配置：原Admit前、durable Admit后/formal前、confirmed后/HTTP响应前各实际结束精确进程；正常新Host重认证、排他及原key查证，不换key重放。另两例after-admit/after-confirm真实TCP RST后release，仅在该原调用唯一新correlation上观察真实Write/Flush失败才通过；业务仍确认且原receipt唯一，B实际保存与完整正常结束/新进程同receipt分别核对。不是同process撤权、native COMMIT中点或硬断电证明。
- 私有tag配置仍是原七字段并绑定原fixture/store/workspace/principal/key/epoch，仅增加ordinal0的三个phase。marker是观察不是持久权威；默认生产忽略环境。RST/EOF不冒充交付失败事件、handler结束或回滚。checkpoint等待不按超时释放owner，不产生新admission/writer。
- 定向命令（已有正常/tag配置）：`& ./tools/go.ps1 -GoArgs @('test','./tests/acceptance','-run','^TestV010IndependentSaveCheckpointAndLostDelivery$','-count=3','-v','-timeout=180s')`。04五例各三轮passed；05定向通过但全仓复发旧win32-233，不能称全绿。06只改结果名为`save-checkpoint-result.json`以保留原`save-checkpoint.json` marker，226输入/profile/四exe冻结，五例各三轮25.208s、既有失败hot场景五轮41.287s、全仓一次249.426s及正常/实际tag vet/diff均0（2026-10-20T05:13:05+08）；不宣称233根因已定位/修复。[历史失败和证据边界](<../../specs/001-host-workspace-foundation/research.md#t026-原调用边界真实断连与已确认丢交付2026-10-20partialnative-mid-commit仍unverified>)。当时T026 native mid-COMMIT仍未验收，随后实现及当前实跑见下节；T027–T032仍待独立验收，15分钟长门不重复计数。

## V010 原formal COMMIT内部第二journal sync（原03截面partial，后续签收见下）

- 现有checkpoint表新增`formal-inflight-process-end`与`formal-inflight-disconnect-release`，合计七例，命令/正常与tag配置不变。需要重新构建tag exe以包含同原capability的read-only COMMIT观察与原永久VFS delegate扩展；没有新增公开API、SQL或writer。
- 原actual handle native BOOL成功后才持有：原CommitCalls1/InFlight true/Returned false、JournalCalls=JournalSuccesses=2、DB sync0；所有false结束字段必须明确存在，严格marker拒绝重复/缺失/null/非法摘要。固定小fixture无先前非commit sync；DB sync0不是通用DB写0/硬断电证明。精确B路径先于hold mutex过滤，B真实保存仍须完成。
- 实际结束精确旧PID后正常Host原key/fresh调查为known rejected、正式0；单独RST/release正控则实际commit且同请求Write/Flush失败、原receipt唯一并跨完整正常重启保持。两例02首跑1.60/1.27s passed；03冻结231输入/profile/四exe，七例与旧native三轮0（68.002s），但全仓在V009 malformed-descriptor的inspect前原生认证unauthorized失败。04补正确legacy选择器三轮及normal/实际tag vet均0，diff0/输入未变；不能用静态绿覆盖全仓red，T026不勾选。后续非密pipe诊断与实际复验见下节，不重试旧交换或放宽握手；T027–T032仍未完成。[实现、失败与证据边界](<../../specs/001-host-workspace-foundation/research.md#t026-原formal-commit内部真实journal同步中断2026-10-20partial定向passed全仓认证failed>)。

## 受保护pipe诊断与有限重启观察（partial，未声明旧根因修复）

- 固定stage/outcome/context/native/elapsed/attempt/reset信息，不渲染原error/token/DTO/locator；仅裸CreateFile BUSY仍可等，其他错误terminal，原wire/15s/SID/PID/DACL/cancel-drain不变。primary cause与reset分离；operator ring64+覆盖数只在HTTP/inspection/control实际排空后flush，不能扣除inspection计量噪声。失败后的原pinned收集保留原Fatal并标记shutdown可能造成server取消。
- [真实重启测试](<pipe_restart_windows_test.go>)：normal Host同fixture同pipe名64次×3=192真实进程，当前HTTP context安装operator、原server完成及实际正常结束/scanner与句柄收尾后下一轮；04 14.88/14.69/15.14s passed。同进程确定preaccept元数据三轮也成功，不支持旧PID0假说，但不是跨进程身份或永久无故障证明。400同instance立即复用、primary/reset及ring/runtime边界门三轮、两原失败V009分支/两hot分支三轮、双vet/diff/235输入/profile/四新exe均0；05明确复用04同235输入/profile/四exe，当前全仓一次exit0（external263.991s，06:16:12+08）、无指纹差异/diff0。逐类明确V010门与受影响门复验满足后，按固定profile有限签收T026、进入T027；不借此签收T027–T032/001。历史900.155s只沿用未变规则范围，非当前exe再等。旧233/Unauthorized未复现，02不同stdout诊断截面的全仓通过不拼作04版本结果。[完整观察与历史失败](<../../specs/001-host-workspace-foundation/research.md#t026-受保护pipe认证stage调查2026-10-20partial非根因修复声明>)。

## V011 normal Host 两区完整保存/正常重启（implemented；固定profile passed）

- [真实完整重启测试](<save_restart_windows_test.go>)仅normal生产CLI/native会话/public HTTP；A/B各10不同Unicode/CRLF正文+各1条同resource更新，head11/rows10，逐记录title/body/identity/version/revision/digest一致。两份实际SQLite头+FileInfo排除同文件，关闭两区后旧Host实际正常结束，新进程重新认证/新HTTP/newepoch重开原数据库，再逐条读取全集合和22个原receipt；原epoch已确认key只调查，不重放。
- 未确认create/update及实际手改普通候选形状JSON前后不入正式状态；不是generic Apply/file-import新功能。所有正常停机必须exit0/stopped/Wait/scanner join，失败强杀不算通过。合成结果含实际fresh reopened rows/close结果/PID+birth/normal image hash，不含bearer。
- 01因测试文件序列化遗漏required OpenEpoch failed，已补原epoch，保留原失败。02冻结236输入/profile，四exe逐项hash与前轮相同、明确复用不冒称重建，V010/V011各三轮exit0（新V011 3.30/3.15/3.40s，focused138.604s），当前全仓一次全部0（external266.827s）、acceptance normal/tag vet/diff0/指纹无变化，06:32:29+08完成。T027按V011固定profile签收，不是T028–T032或001完成；真实900s TTL门此轮显式SKIP、仅保留未变规则的历史语义证据。[实际结果与边界](<../../specs/001-host-workspace-foundation/research.md#t027-两区各十条与更新的完整正常重启2026-10-20implemented固定profile-v011-passed>)。HTTP preview200/合同201及默认limit20/合同100的已知偏差留T031/V015，当前不是全合同一致性声明。
