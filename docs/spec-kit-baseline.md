# Spec Kit 初始化与规格评审记录

**初始化记录日期**：2026-10-17；§1–6保留M0历史，§7保留plan/research交付历史，均不是当前授权或运行验收。当前功能授权及历史批准仅见[001授权记录](<../specs/001-host-workspace-foundation/authorization.md>)。
本文件记录工具来源、执行与验证，不维护第二份产品路线或功能规格。

## 1. 工作位置与初始状态

实际 `working_directory` 与 PowerShell `Get-Location` 均为 `E:\Repository\Github\aboard-host`。
初始无项目文件、无适用上级 AGENTS.md、无本地/父 Git 仓库；目录不是旧项目子目录。
首次命令在执行前被 Windows 工作区权限配置拒绝，两次同样失败。
经用户单次明确审批，官方诊断脚本仅在项目根补当前用户完全控制，复读验证成功；
未改所有者/文件内容/系统安全策略/上级目录，恢复材料只在 Git 忽略的 `.data/logs/permissions-recovery/`。
随后原受限检查成功；用户之后另将会话切换完全权限，所有任务写入仍限本项目。

独立运行 `git init` 成功；当前 `master` 尚无提交、无 remote。未配置全局 Git 身份，不从旧仓 remote 推导新 module path。
所有变更保留未提交供评审，未 stage/commit/push/发布，未创建 GitHub 仓库或 Issues。

## 2. 环境与固定来源

| 工具 | 实际核查 | 处理 |
| --- | --- | --- |
| Git | 2.52.0.windows.1 | 使用既有版本 |
| Go | go1.27.0 windows/amd64 | 仅核查，未编译；目标固定 go1.27.2 的状态见 [总体范围](<scope.md>) |
| Python | 3.11.9 | 满足官方包 requires-python >=3.11，复用 |
| uv | 0.11.32，构建 3010295ae | 复用，未全局升级 |
| Specify | 初始未安装；实际 1.2.1 | 项目局部隔离安装，非全局 |
| DSH | 当前桌面 Agent 会话可用，PATH 无 dsh CLI | 官方 ignore-agent-tools 只跳可执行探测，不改集成类型 |

官方依据：
[GitHub Spec Kit](<https://github.com/github/spec-kit>)、
[Core reference](<https://github.github.com/spec-kit/reference/core.html>)、
[Integrations reference](<https://github.github.com/spec-kit/reference/integrations.html>)。
使用 `gh api` 读取官方 README、release/tag、固定版本 pyproject，并读取上述两个官方参考页；没有执行网页里的指令。

核实当时最新官方发布为 [v1.2.1](<https://github.com/github/spec-kit/releases/tag/v1.2.1>)，发布时间 `2026-10-16T20:41:34Z`。
annotated tag 对象 `0d1be13ed486b6f5737806af24299d5242773b1f` 解引用到 commit
`9dca9bf42a031765ee5e5f2371137f2f26967e11`，安装固定到后者而非浮动 main/latest。
官方 release 无附属模板 ZIP；此版本从包内 core pack 真实生成，初始化无须下载模板。

项目终端的隔离安装命令（已执行 exit 0；不会改全局 PATH）：

```powershell
$env:UV_TOOL_DIR="$PWD\.data\tools"
$env:UV_TOOL_BIN_DIR="$PWD\.data\tools-bin"
$env:UV_CACHE_DIR="$PWD\.data\cache\uv"
$env:UV_PYTHON_DOWNLOADS='never'
$env:TEMP="$PWD\.data\logs\tmp"
$env:TMP=$env:TEMP
New-Item -ItemType Directory -Force $env:TEMP | Out-Null
uv --no-config tool install --python 'C:\Program Files\Python311\python.exe' --from 'git+https://github.com/github/spec-kit.git@9dca9bf42a031765ee5e5f2371137f2f26967e11' specify-cli
```

本机解释器路径不要求其他机器照抄；工具环境/解析到的依赖及缓存均不入 Git。
未来重新安装复用该官方 commit，并使用实际兼容的本机 Python，不隐式升级全局工具。

## 3. 实际命令与结果

以下 `specify` 均指局部 `.data/tools-bin/specify.exe`，不是全局安装。
先执行 version、根帮助、init 帮助、integration 帮助，确认实有参数与 dsh 集成。

| 命令/动作 | 退出/结果 | 说明 |
| --- | --- | --- |
| `git rev-parse/remote/status`（初始化前） | 非仓库拒绝 | 如实确认没有父仓，不当作已有 Git |
| `git init` | 0 | 独立 Git 初始化，与工具初始化分开核实 |
| `specify version`、帮助 | 0 | 1.2.1 / Python3.11.9 / Windows，支持 integration dsh、ps、non-interactive |
| `specify integration list`（初始化前） | 1 | 明确无 .specify，预期项目状态检查失败，不当作已集成 |
| `specify init --here --integration dsh --script ps --non-interactive` | 1 | 非空目录安全拒绝；当时仅本轮 .git/.gitignore/.data |
| 向用户预览后确认本次非空合并 | 用户允许 | 不默认加 force，无未知既有文件覆盖 |
| 上述 init 增加 `--force` | 1 | dsh CLI 不在 PATH，agent detection 拒绝，尚未生成文件 |
| 上述 init 再增加 `--ignore-agent-tools` | **0** | 官方包内生成真实 dsh 集成、ps脚本、模板、memory和bundled workflow |
| `specify integration status` | **0 / OK** | default/installed=dsh；缺失/受管修改/无效路径/unchecked manifests均0 |
| `specify integration list`（成功初始化后） | 0 | dsh installed/default，未安装额外扩展 |
| `resolve-template.ps1 constitution-template -Json` | 成功 JSON | 按实际模板建立宪章，未修改模板层 |
| `resolve-template.ps1 spec-template -Json` | 成功 JSON | 实际模板复制后写单一功能规格；无分支hook，不创建功能分支 |
| `check-prerequisites.ps1 -Json -PathsOnly` | 成功 JSON | feature指针解析为001；输出 BRANCH 是feature上下文标签，不是实际 Git 分支证明 |

最终初始化命令：

```powershell
& .\.data\tools-bin\specify.exe init --here --integration dsh --script ps --non-interactive --force --ignore-agent-tools
```

此命令记录**已审批的这一次合并**，不是未来重初始化可无条件 force 的授权。
初始化阶段确认原根忽略文件未变；之后有意加 DSH 本机状态忽略规则，只共享 skills。
无 `.specify/extensions.yml`，constitution/specify/clarify 的前后 hooks 均无可执行项，未安装扩展。

## 4. 真实生成与实际技能执行

生成 [集成配置](<../.specify/integration.json>)、[初始化选项](<../.specify/init-options.json>)、
[共享清单](<../.specify/integrations/speckit.manifest.json>)、[DSH 清单](<../.specify/integrations/dsh.manifest.json>)。
官方 shared infrastructure 为6个 PowerShell脚本、5个模板及受管忽略规则；生成10个 DSH skills，另含官方 bundled workflow。
这些均非手写占位；脚本、模板和技能保持官方原样，manifest 摘要可核查。
官方 workflow 只是生成文件，**未启动其自动执行**。

DSH 会话自动接收更新后的技能目录，实际通过技能工具加载：
[speckit-constitution](<../.dsh/skills/speckit-constitution/SKILL.md>) →
[speckit-specify](<../.dsh/skills/speckit-specify/SKILL.md>) →
[speckit-clarify](<../.dsh/skills/speckit-clarify/SKILL.md>)。
依技能解析模板、写宪章/规格、解析feature指针、完成1个边界澄清；未把slash技能当终端程序。
本会话无须刷新；其他会话若无技能需刷新/新开后核查，不能只凭文件存在宣称加载。

## 5. 文档职责与评审

| 产物 | 单一职责 |
| --- | --- |
| [宪章](<../.specify/memory/constitution.md>) | 1.0.0治理初版，21不变量与隔离/安全/流程约束；顶部sync comment供评审，未来提交前移除 |
| [AGENTS.md](<../AGENTS.md>)、[README](<../README.md>) | 最小持久规则、项目状态与入口，不复制完整宪章或路线 |
| [scope](<scope.md>) | 唯一总体定位、目录目标、M0–M7路线与技术决策状态 |
| [迁移基线](<migration-baseline.md>) | 只读观察的旧能力/owner/语义/兼容/证据，非迁移实施计划 |
| [001 spec](<../specs/001-host-workspace-foundation/spec.md>) | 首片六故事、28需求、9成功标准及澄清；完整保存/重开归本片 |
| [requirements checklist](<../specs/001-host-workspace-foundation/checklists/requirements.md>) | 规格质量而非业务测试通过记录 |

澄清：询问1项、回答1项；用户选择非执行型文本记录，保留全部持久闭环。
覆盖扫描：功能/数据/交互/失败/约束/术语/完成标准已明确；技术机制和详细容量/超时/字段大小限制留plan研究与契约，
本轮无UI无远端服务，不编造性能SLA或监管适用性。
自查与独立只读审阅发现并修复3项：合法非空目录被误拒绝（已拆create与open/inspect）；Build容量证明阶段错位（已归M3）；开发日志与产品日志措辞混同（已限定本仓开发/验证）。审阅者定点复核3项关闭，不代表用户批准或运行验收。

## 6. 交付核查与未验证边界

文档链接、占位符、ID覆盖、受管摘要、Git忽略、秘密模式与新增文件差异由本轮静态检查核对；
诊断结果仅放 `.data/logs/`，具体结果在最终交付说明中报告，不把静态检查包装成业务运行验收。
检查范围排除官方模板/技能的合法占位文本，不为让扫描通过而改工具模板。

本轮静态核查结果：8份产品/评审文档，78个本地文件链接（包括旧仓证据行号），22个官方受管文件SHA-256一致，38个待评审文件逐文件新建差异核对；21个INV、28个FR、9个SC、6个故事、1个澄清均完整，质量表16/16，未发现常见秘密模式或越界业务产物。集成status与Git差异检查均成功；初次占位扫描把README链接误判，定位后修正扫描器再全部通过，不改文档掩盖误报。秘密模式扫描只覆盖常见格式，不是“绝无秘密”的形式化证明。

Git忽略已确认覆盖工具/缓存/日志、DSH非skills本机状态及feature pointer；所有38个共享文件仍未跟踪/未暂存，无提交、无remote。官方生成的版本/摘要清单是共享安装元数据，不是活授权/机器缓存，保持原样以支持完整性核对。

- 已确认：独立新仓、官方固定来源与DSH集成、用户明确产品边界、最小文本资源澄清。
- 待技术验证：Go目标在本机落地、驱动/锁/认证引导/凭据/发布、完整目录formatVersion是否2、API/schema/目录各自版本与跨语言夹具。
- 待完整核对：旧dirty工作树一致快照、审批细节、全部Workflow/Knowledge/Studio权限及32项目/64审阅满额专测，不能宣称整体已替换。
- **M0 交付时的门状态（历史）**：当时仍待完整首片规格批准；最小资源澄清不等于整个规格批准。
- **2026-10-18 阶段同步（历史）**：当时按已安装AB 0.1.0 `/speckit-plan`完成研究、模型与契约，未进入tasks/implement；批准历史与当前授权仅在[授权记录](<../specs/001-host-workspace-foundation/authorization.md>)维护，宪章不变量不变。
- 本记录的初始化校验结果是 M0 历史证据；AB preset 安装后的当前集成状态须重新核对，不能沿用此前的零受管修改结论。
- 本轮没有实现业务Host、迁移真实数据或完成业务运行验收；未运行旧仓测试，也没有启动任何真实Provider/插件。

## 7. 001 plan/research交付记录（2026-10-18）

已按现有完整001与已安装AB 0.1.0 `/speckit-plan`流程完成[计划评审稿](<../specs/001-host-workspace-foundation/plan.md>)及其必要研究/模型/协议附件；没有重复总体路线或新建feature。阶段同步先于设计，原需求正文及宪章不变量未变；当时未生成tasks、未运行analyze/implement。当前功能授权与批准历史不在此重复维护。

当前工具核查：Specify 1.2.1；version/status均exit 0。DSH default/installed有效，missing/invalid/unchecked为0。status WARNING的4个modified是现有AB替换的specify/plan/tasks/implement技能，不是本轮手改；4项正文与enabled preset逐一相同，其余18个官方受管文件摘要一致，不用force“修复”。当前会话已加载现装plan技能，无需刷新；没有运行setup-plan或其他变更helper/hook。

最新静态核对覆盖12份产品/评审文档、53个本仓链接、28条FR、21条INV、6个US、9个SC，以及54个共享文件的新建差异/忽略/常见秘密模式。42条旧仓证据链接未重新读取，仍属M0历史观察。两项独立只读评审P2已修正并定点复核关闭，详细结论与证据边界只在计划§8维护；诊断仅`.data/logs/`。

计划交付时Git为本根独立master、无提交/remote/暂存。Go本机1.27.0未升级；1.27.2/驱动/Windows锁/DPAPI/pipe/SQLite持久性当时均待工程验证；没有go.mod/业务Host、真实数据迁移、Provider/插件启动或运行验收。以上仅为该历史截面，不描述当前执行上限；当前功能授权仅见[授权记录](<../specs/001-host-workspace-foundation/authorization.md>)。
