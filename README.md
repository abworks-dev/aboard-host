# Aboard Host

本地优先、独立运行的 Go 工作区后端引擎：**共享一套 Aboard 引擎，一份目录承载一套长期工作区，前端可以更换。**

## 当前状态

当前功能为001；唯一当前授权与历史批准见[授权记录](<specs/001-host-workspace-foundation/authorization.md>)，执行基线见[任务清单](<specs/001-host-workspace-foundation/tasks.md>)。官方 GitHub Spec Kit 1.2.1 已接入 DSH，使用已安装的 AB Agent-first Baseline 0.1.0 preset；工具安装不是运行证据。
已建立有真实远端来源的[Go模块](<go.mod>)；T001–T008已完成历史前置任务，含[获准的单行刷写兼容修正](<tools/compat/README.md>)、真实跨进程锁、冻结副本源写观测、原生身份引导及事务/ledger门。旧T009冻结后的两源变更已真实复验受影响门及全仓组合三轮，以新证据重新冻结；随后依T010→T012完成局部三轮测试与vet，当前T011安装授权接线仍在聚焦验证，未提前勾选。详情与历史失败见[研究证据](<specs/001-host-workspace-foundation/research.md>)。尚无业务Host、运行安装包或已通过的业务运行验收。这里不是旧项目的子目录或迁移副本。

## 先读

- [项目宪章](<.specify/memory/constitution.md>)：不变量、权威与隔离边界。
- [总体范围与唯一路线](<docs/scope.md>)：定位、目标目录、阶段归属、未决技术验证。
- [迁移观察基线](<docs/migration-baseline.md>)：旧项目已核对能力、容量、证据与未迁移项。
- [首片规格](<specs/001-host-workspace-foundation/spec.md>) 与 [计划评审入口](<specs/001-host-workspace-foundation/plan.md>)（含研究、模型、协议和验证门）。
- [规格质量检查](<specs/001-host-workspace-foundation/checklists/requirements.md>) 与 [初始化/评审记录](<docs/spec-kit-baseline.md>)。
- [仓库持久规则](<AGENTS.md>)。

## 流程入口

在 DSH Agent 会话使用 `/speckit-constitution`、`/speckit-specify`、`/speckit-clarify`。
按[唯一授权记录](<specs/001-host-workspace-foundation/authorization.md>)和任务依赖选择当前阶段，不从本文推导额外批准。技能在Agent会话执行，不是终端可执行程序。
首片路径由本机 Spec Kit feature pointer 选择；在新 checkout 中可显式选择
`specs/001-host-workspace-foundation`，不依赖 Git 分支名。

当前会话已自动发现新增技能并实际加载，无需刷新。其他会话若看不到技能，刷新/新建项目会话再检查，不能把文件存在当加载成功。
本项目的 Specify 安装在 Git 忽略的项目局部工具环境，未修改全局 PATH；可在项目终端使用：

```powershell
& .\.data\tools-bin\specify.exe version
& .\.data\tools-bin\specify.exe integration status
```

该机器路径不是共享依赖。重新安装的固定来源、版本、命令与结果见 [工具基线](<docs/spec-kit-baseline.md>)。

## 非目标

本feature业务范围见[规格](<specs/001-host-workspace-foundation/spec.md>)；允许执行的工作与禁止事项见[唯一授权记录](<specs/001-host-workspace-foundation/authorization.md>)。
首期不承诺网络盘、云同步、多 Host 协同写、跨机自动继承权限、未知副作用自动续跑、跨平台锁或恶意原生代码沙箱。
SQLite/锁/本地身份已有固定Windows profile的前置证据，最终业务组合、故障恢复和发布仍须对应验收；共享 Host 进程并非完全故障独立。
Go module path的前置来源规则见[授权记录](<specs/001-host-workspace-foundation/authorization.md>)，未确定前不虚构模块归属。
