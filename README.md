# Aboard Host

Aboard Host 是本地优先、独立运行的 Go AI 工作区内核：**共享一套引擎，一份目录保存一套长期工作区，客户端可以更换。** 它负责身份与权限、正式内容与版本、保存结果及实际生命周期，不是旧 Host 的转发壳。文本和附件是先验证内核的载体，完整 AI/插件/运行能力仍按[唯一路线](<docs/scope.md#9-唯一阶段路线与退出证据>)逐步交付。

### 两仓分工与用户闭环

- **Host**拥有公共业务协议、数据库和正式结果，也负责可信原生连接与窄桥；**Workbench**拥有React界面、生成绑定的消费适配和静态产物。界面不能直接写库或代授权限。
- 用户要完成的是：可信连接 → 看见当前获准工作区 → 明确打开 → 读取/预览 → 确认保存 → 核验回执；断线只调查原操作，不自动重存。独立CLI应能互读同一正式事实。这个产品目标不等于当前React全闭环已验收，见[003当前状态](<specs/003-m6-client-integration/tasks.md#current-state>)。

### 三条阅读路

| 想了解什么 | 从哪里开始 |
| --- | --- |
| 产品为什么做、做什么/不做什么 | [总体范围](<docs/scope.md>) → [001基础行为](<specs/001-host-workspace-foundation/spec.md>) / [002对象与完整版本](<specs/002-resource-object-commit/spec.md>) / [003客户端体验](<specs/003-m6-client-integration/spec.md>) |
| 现在做到哪里、什么没有验 | [001工程状态](<specs/001-host-workspace-foundation/tasks.md#current-state>)、[002当前状态](<specs/002-resource-object-commit/tasks.md#当前状态>)、[003唯一当前状态](<specs/003-m6-client-integration/tasks.md#current-state>)；不把内部技术接收、组件测试、真实联合和发行混为一件事 |
| 设计与开发如何追踪 | 各功能的 [001计划](<specs/001-host-workspace-foundation/plan.md#design-guide>) / [002计划](<specs/002-resource-object-commit/plan.md#design-guide>) / [003计划](<specs/003-m6-client-integration/plan.md#design-guide>) → 原任务的Owner/依赖/US/验证入口 → 原证据；先核[授权](<specs/001-host-workspace-foundation/authorization.md>)与[Spec Kit入口](<#spec-kit-流程入口>) |

## 当前状态

只读上面的三份任务状态入口，不在README复制第二进度表。001完整保存/停机重开由原T027承担，汇总证据见[001最终工程验收](<specs/001-host-workspace-foundation/plan.md#101-001最终工程验收汇总t032当前>)；002核心范围与物理发布缺项见[002最终覆盖](<specs/002-resource-object-commit/plan.md#62-t029v209-核心工程最终汇总implemented发布未就绪>)；003区分本仓工程、WB036技术接收/精确生产就绪与完整T018/G306、联合及发行，见[003状态](<specs/003-m6-client-integration/tasks.md#current-state>)和[验证范围](<specs/003-m6-client-integration/tasks.md#verification>)。

这些入口引用既有工程记录，本次文档整理未重跑业务验证；历史失败与工件仍留在各功能plan/research。ignored本机日志、工具和image不随新checkout提供，旧绝对路径不自动在新位置有效；工程完成不等于用户接受、安装包发布、旧Host退役或真实数据迁移。

## 先读

- [总体范围与唯一路线](<docs/scope.md>)：产品定位、长期目标、阶段归属与非目标；其中带日期的历史状态不覆盖 001 最新工程汇总。
- [001 规格](<specs/001-host-workspace-foundation/spec.md>)、[计划](<specs/001-host-workspace-foundation/plan.md>)与[任务清单](<specs/001-host-workspace-foundation/tasks.md>)：行为、设计与工程证据归属。
- [项目宪章](<.specify/memory/constitution.md>)与[仓库规则](<AGENTS.md>)：不变量、权威与隔离边界。
- [迁移观察基线](<docs/migration-baseline.md>)：旧项目已核对能力与未迁移项；001 完成不代表已替换旧 Host。
- [规格质量检查](<specs/001-host-workspace-foundation/checklists/requirements.md>)与[初始化/评审记录](<docs/spec-kit-baseline.md>)。

## 源码使用入口与前提

目前没有面向普通用户的安装包/交互控制台，也没有独立的 `demo` 子命令。当前树不存在 `specs/001-host-workspace-foundation/quickstart.md`；以下仅说明已核对的源码入口，不冒充开箱即用 quickstart。

### 构建（开发入口，须先具备本地环境）

工程验收支持范围为 Windows amd64、本地直连 NTFS、普通非重解析目录；固定 Go 1.27.2、SQLite 3.53.4，以及配套的 [Windows 刷写兼容修正](<tools/compat/README.md>)。构建必须从项目根经[项目 runner](<tools/go.ps1>)执行，不能用遗漏兼容输入的普通 Go 构建替代。

下例仅供已准备好固定 Go 工具链、依赖缓存和兼容源的开发环境；`GOPROXY=off` 不会补齐缺失依赖。新 checkout/worktree 没有主目录的 ignored 缓存，缺环境应先停止核对，不自动下载、安装或借用别处缓存冒充验证。本次未执行这些命令。

```powershell
$env:GOPROXY = 'off'
& ./tools/go.ps1 -GoArgs @('build','-o','.data/logs/001-validation/acceptance-host.exe','./cmd/aboard-host')
if ($LASTEXITCODE -ne 0) { throw 'build failed' }
```

### init / serve / 认证

[生产 CLI](<cmd/aboard-host/main_windows.go#L18-L42>)与[显式安装恢复入口](<cmd/aboard-host/recovery_windows.go#L56-L89>)接受以下语法（占位符不是现成配置）：

```text
aboard-host init --config <trusted-config.json>
aboard-host serve --config <trusted-config.json>
aboard-host recover-installation --config <trusted-config.json> --request <original-upgrade.json>
```

- [受信配置](<internal/host/config_windows.go#L22-L79>)严格包含 `installationRoot`、`allowedRoots`、`listen`、`pipeName` 四项；要求规范的 Windows 本地绝对路径、loopback 监听地址及 `aboard-g5-` 前缀 pipe 名。配置来自受信操作员，不是 HTTP 请求；`allowedRoots` 只限制位置，不自行授予业务权限。
- `init` 显式创建全新安装，要求安装根尚不存在；不是工作区 create，也不是残留修复/重新初始化命令。`serve` 只打开既有有效安装，不因缺库/坏材料自动补建，见[安装生命周期](<internal/installation/store.go#L62-L150>)。
- `recover-installation`仅供原升级中断后的显式维护：真实当前操作员、原key、预览后精确确认及原owner排空；不重放升级、不自动修复普通open/serve，也不是通用备份恢复。约束见[002设计](<specs/002-resource-object-commit/plan.md#421-安装中断后的显式冷恢复引导>)。
- loopback 不等于免认证。[受保护 named pipe](<internal/installation/security/pipe_windows.go#L183-L260>)核验本机身份、DACL 与预期服务端进程后交付短期 bearer；随后 HTTP 逐请求校验当前权限及明确工作区/action，拒绝 Origin/Cookie 等浏览器入口，见[实际授权边界](<internal/installation/auth.go#L260-L281>)。bearer 不应进入命令参数、环境、文件或日志。
- 未提供 `console`/`login` CLI；现有认证客户端在[验收 harness](<tests/acceptance/harness_windows_test.go#L461-L501>)中，不是可直接分发的用户客户端。[console helper](<tests/acceptance/console_windows_test.go>)仅为测试进程正常停机服务，不是交互控制台。取消等待或终止进程也不能冒称业务已回滚或物理排空。

### 纯合成闭环入口（工程演示，不是产品 demo）

[既有验收说明](<tests/acceptance/README.md>)记录构建与环境变量前提；其中带日期的 partial 截面保留为历史，当前完成状态以 001 最终汇总为准。[完整两区保存/重启场景](<tests/acceptance/save_restart_windows_test.go#L84-L105>) `TestV011IndependentCompleteSaveRestart` 使用正常生产 CLI、受保护认证与公共 HTTP，自动在 Git 忽略的诊断目录创建全新合成安装/工作区；不是打开用户现有目录或真实数据库。

运行此单场景前必须已有符合要求的生产 image，并按既有说明配置 `ABOARD_ACCEPTANCE_HOST_EXE`；未配置会 **skip，不是通过**。其他场景可能另需 tag/helper image，不能把这一个场景或 smoke 当作整套验收。此处只提供已有入口，不新增演示程序、测试 fixture 使用路线或第二验收体系。

## 支持与限制

- 已通过结论仅限上述固定 Windows/NTFS profile；Linux/macOS、网络盘、云同步、多 Host 同区协同写、跨机继承权限均未因此获得支持。
- 固定 `CGO_ENABLED=0` 下实际 race 命令 exit 2（`-race requires cgo`），**race 未执行**，不记为通过。硬断电、全系统 IO trace、未定义 SLA、跨版本/跨 profile 与恶意原生代码沙箱不在通过声明内。
- 历史 native/old pipe 233、Unauthorized 及停机 rename AccessDenied 尚未定位根因；后续有限复验通过不代表永久可靠或已修复这些历史问题，详见[保留风险](<specs/001-host-workspace-foundation/plan.md#L340>)。
- 工作区隔离不等于共享 Host 进程故障独立；crash/OOM/磁盘满可能影响多个区。对象功能的实现及未验范围只见[002当前状态](<specs/002-resource-object-commit/tasks.md#当前状态>)；通用文件 Apply、Build/Studio、插件/Provider/Run、通用备份恢复与旧库迁移仍按[总体范围](<docs/scope.md>)归属后续阶段。

## Spec Kit 流程入口

父目录会话负责派单，Host 子会话以本仓为工作目录执行所属 Spec Kit 阶段。在该会话确认 `/speckit-constitution`、`/speckit-specify`、`/speckit-clarify` 等技能实际可见，再按[唯一授权记录](<specs/001-host-workspace-foundation/authorization.md>)和目标任务推进；技能不是终端程序。父目录不会自动聚合子仓技能，工作目录切换不等于技能已加载。

[locator](<.specify/feature.json>)保留 `specs/002-resource-object-commit`；003客户端任务必须显式指定 `specs/003-m6-client-integration`，不因派单切换共享指针。`SPECIFY_INIT_DIR`、`SPECIFY_FEATURE_DIRECTORY` 或 `SPECIFY_FEATURE` 与目标冲突时先调查；不静默忽略旧环境或用Git分支名猜功能。

[项目自有工具](<tools/spec-kit.ps1>)仅提供 Context、Check、Template 检查；不改官方生成脚本、模板和技能，不执行 specify/plan/tasks/implement，也不替代阶段授权或正式 analyze。须从本仓根运行，例如：

```powershell
# 002仍为默认locator；本次调用临时覆盖显式003供子进程继承，结束恢复，不落盘切换。
& ./tools/spec-kit.ps1 -FeatureDirectory specs/003-m6-client-integration -Action Context
& ./tools/spec-kit.ps1 -FeatureDirectory specs/003-m6-client-integration -Action Check
& ./tools/spec-kit.ps1 -FeatureDirectory specs/003-m6-client-integration -Action Template -TemplateName spec-template
# 工具回归，不运行Host/native/browser或产品测试。
& ./tools/spec-kit.tests.ps1
```

入口按 `-PythonExecutable` → 进程级 `SPECKIT_PYTHON_EXECUTABLE` → 兼容旧变量 `SPECKIT_PYTHON` → PATH 中 `python` 的顺序选择已有解释器，并核验Python3且可导入PyYAML；拒绝商店占位程序，不安装依赖。任一显式配置无效则失败、不静默回退。入口临时设置解释器、功能目录、`SPECIFY_FEATURE_NO_PERSIST=1` 和本仓忽略临时目录，结束后恢复调用进程环境；检查输出不等于规格质量或业务验收通过。

Spec Kit 1.2.1 / AB Agent-first Baseline 0.1.0 的安装属于[已有工具基线记录](<docs/spec-kit-baseline.md>)，不是 Host 运行证据。2026-10-29核对新位置的 `.data/tools-bin/specify.exe` 缺失；既有技能/脚本仍可按上面检查，CLI安装维护命令不能照抄历史机器路径或擅自重初始化。已生成的 bundled workflow 不等于当前已启用流程，不能用它省略本仓 clarify/批准/analyze 门。

## 文档维护静态检查

维护规则见[仓库规则](<AGENTS.md#spec-kit-文档维护规则>)，阶段授权仍只见[唯一授权](<specs/001-host-workspace-foundation/authorization.md>)。本项目自有[检查器](<tools/check_docs.py>)与[合成测试](<tools/test_check_docs.py>)只需已有Python 3.9+和Git；不下载工具、不依赖PyYAML、不启动Host/native/browser。检查器只读，不改文件/任务/locator/授权，不调用Spec Kit生成脚本；测试只在本仓ignored诊断目录写合成Git fixture，使用进程级提交身份，不改全局Git配置。

```powershell
# 从本仓根运行；默认base=HEAD、mode=maintenance，基线应明确记录。
python -B tools/check_docs.py
python -B tools/test_check_docs.py
# 跨提交维护时应使用实际开始前的提交；无效base硬失败，不降级。
python -B tools/check_docs.py --base <baseline-commit>
# 已有业务授权且核对全部任务差异/证据后，记录此次混合工作树的检查原因；不是维护/CI通过。
python -B tools/check_docs.py --base <same-baseline-commit> --mode development --reviewed-task-updates
# 局部诊断：只核这些源文档，不声称完成全仓或其他源反向引用检查。
python -B tools/check_docs.py --files README.md AGENTS.md
```

默认检查Git tracked及未忽略untracked中的根README/AGENTS、docs/specs树Markdown，另显式包含宪章；不递归本机日志、`.git`、缓存、依赖、交付包、archive/experiments等冻结输出。历史保留的受维护plan/research仍是输入；不扫描或验证其绝对机器路径证据。输出列出实际scope、base/mode和文件；exit 0仅表示所选静态检查无新增硬失败，exit 1表示回归，exit 2表示参数/输入/Git失败。`WARN`是同一base中已坏的历史问题（不静默忽略）；`UNVERIFIED`是越界/不支持目标，不可解读为链接有效。已补充下述CI配置；本地维护完成前仍须代理/人工必跑，配置存在不等于远端已执行或已有推送强制门。

**任务更新边界**：纯维护不得改变原任务整行；既有授权的实施/验证进度回填即使只改Markdown，也按业务进度核原授权、任务承诺和证据，不恢复取消的门或回退真实状态求绿。混合工作树先保留同base、全域maintenance的真实失败；明确上述原因后才独立运行development，不自动fallback。有任何任务整行差异（含新增）但无`--reviewed-task-updates`时exit 1；有声明则逐条输出`REVIEW task-line-removed/added`与完整原行/计数（匿名/重复行不猜配对）。无差异无需声明；maintenance与声明组合exit 2，不能解除保护。声明仅记录调用者已依既有授权完成语义核对，不产生授权、不证明人工确已复核或验收，不增加逐任务人工审批。development不自动保护任务正文承诺；裸T编号、anchor、断链等原结构硬错仍失败，匿名任务行等不能由稳定编号保护的差异只作展示，不声称被语义拦截；复合切片沿用既有编号解析子集，仍可能触发原结构检查。`task_difference_rows`是差异记录数，不是产品进展；exit 0仅为development结构子集无硬错，不能称maintenance或自动PR/push CI通过；手动dispatch按下文显式政策单独报告。

受支持子集：普通inline/reference链接、图片引用、空格/中文/URL转义路径、ATX标题和同文档/跨文档fragment、显式HTML id/name；忽略围栏/缩进/inline代码及HTML注释。检查相对目标存在、Markdown fragment、新重复标题/anchor、原定义US/FR/SC/T/V/G编号和维护模式下原复选框整行；默认全域还能发现未改源文档指向被删目标的回归。只把spec/复选框tasks/明确验证标题或场景/门定义表当定义，不把正文引用或导航表当重复定义。

这是有界静态子集，不是完整CommonMark/渲染器：不承诺Setext、复杂嵌套括号/多行链接、HTML href、所有Unicode标点slug或未定义shortcut-reference的完整解析；显式未定义full/collapsed reference报`UNVERIFIED`，GitHub源文件`#L…`行号也只报未核验而不假验行数。HTTP/绝对/跨仓/冻结目标只列未核验且不访问；符号链接/越界不能逃出根目录。迁移标题请保留旧显式anchor，编号/任务/带日期历史仍按规则保护；静态绿灯不能证明用户价值、追踪语义、授权、真实联合、发行或业务验收。本仓原[规格质量checklist](<specs/001-host-workspace-foundation/checklists/requirements.md>)、只读一致性analysis及实际受影响业务门仍各按原职责执行。

### 文档静态检查CI

状态：**implemented（本地配置与合成测试），远端执行unverified**。[GitHub Actions配置](<.github/workflows/docs-check.yml>)触发 `pull_request`、`push` 和 `workflow_dispatch`，不使用路径过滤：未改源文档指向被删目标也须检查。只给 `contents: read`，不用secrets或`pull_request_target`，checkout完整历史且不保留凭据；明确使用 `actions/checkout@v4.2.2`、`actions/setup-python@v5.6.0`、`ubuntu-24.04`、Python `3.12`。这些版本tag本轮未联网核验，也未下载action或安装本地依赖。

[只读CI helper](<tools/check_docs_ci.py>)从标准 `GITHUB_EVENT_NAME` / `GITHUB_EVENT_PATH` 读取事件JSON（本地可用 `--event-name` / `--event-path` 指定合成输入），根目录只取工具自身位置，不信任环境变量指定另一仓：

| 事件 | 唯一基线 | 实际受检内容 |
| --- | --- | --- |
| `pull_request` | `pull_request.base.sha` | checkout后的实际HEAD（通常为Actions合并提交），不是PR head SHA代替基线 |
| `push` | `before` | checkout后的实际HEAD；不以当前HEAD或HEAD父提交回退 |
| `workflow_dispatch` | 必填 `inputs.base_sha`，完整40位commit SHA | 所选ref checkout后的实际HEAD |

缺失、全零、无效、对象不可用、非commit或等于实际HEAD的base均exit 2；helper不fetch、不修复、不回退。**首次新分支push的零before会失败**，应采用PR或手动触发并提供明确有效且非自身的基线；强推后before对象若未在checkout中取得也保留失败，不悄悄改为别的base。日志记录event、实际HEAD、base、全域scope、实际mode/调用者声明、命令与结果；本地重现需事件JSON和对应Git历史，不能以默认HEAD自比较冒充CI。

helper全域调用 `python -B tools/check_docs.py --base <event-commit> --mode <explicit-mode>`，不传`--files`或allowlist。自动PR/push固定maintenance，忽略夹带的mode/声明；手动dispatch默认maintenance，调用者可显式选择development，`reviewed_task_updates`只接受布尔或精确true/false字符串。maintenance+声明为exit 2；有任务差异的development必须声明，否则exit 1；无差异不强迫声明。声明只记录已有授权/证据核对，不产生授权或验收证明，不能失败后自动降级。混合业务任务更新仍可能使自动maintenance失败，手动development不能替代该结果。此为[本轮已授权的最小政策选择](<specs/001-host-workspace-foundation/authorization.md>)，不追溯改变历史严格维护记录。随后独立steps运行[原检查器合成测试](<tools/test_check_docs.py>)和[CI helper合成测试](<tools/test_check_docs_ci.py>)；静态失败不短路这两项，所有步骤保留真实退出码，无`continue-on-error`。只用已有Git/Python标准库，无pnpm/Go依赖安装、业务build或Host/native/browser测试。控制台保留结果，`pipefail`与`tee`将本次CI输出副本留在job工作区ignored`.data/logs/docs-ci/`，不上传event/token；fixture在各专用测试的ignored诊断目录。取消或基础设施失败可能阻断执行，不算通过。本地政策/测试不证明远端Actions或服务端强制门，实际配置仍须独立发布与验证。

**历史记录（文档静态检查CI接入阶段）**：当时本机CI helper专用合成测试为23/23通过（Windows已有Python 3.12.14）；只证明该次本机合成基线与退出码路径，不证明Ubuntu或远端Actions。该阶段本地提交授权见原记录，不是本轮提交许可，实际提交状态以Git为准；未推送或改远端设置，也未启用分支保护或必需检查、push hook；当时的真实维护检查如实保留了既有003/T018任务整行相对HEAD的失败，不为接CI修改业务任务或换base。授权只在[原记录](<specs/001-host-workspace-foundation/authorization.md>)，CI绿也不能取代规格质量、只读一致性复核、联合/发行或用户验收。
