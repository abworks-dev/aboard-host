# Aboard Host 仓库规则

- 当前是新独立项目 `aboard-host`，不是旧 Host 的转发壳。状态与唯一总体路线见 [总体范围](<docs/scope.md>)。
- 治理权威：[宪章](<.specify/memory/constitution.md>)；需求权威：[当前首片规格](<specs/001-host-workspace-foundation/spec.md>)。规则冲突先报告，不静默增加 fallback 或第二 owner。
- 使用 Spec Kit：constitution → specify → clarify → 用户批准 → plan/research → tasks → analyze → implement。不建立重复竞争的规划体系；默认不改生成技能、模板与工具脚本。
- 当前功能授权与历史批准只在 [001 授权记录](<specs/001-host-workspace-foundation/authorization.md>) 维护；其他文档的带日期阶段记录不构成当前授权副本。安装 preset 不扩大权限；每次推进按该记录及实际技术/验收门执行。
- 未来每个业务操作显式携带并校验工作区与当前身份；不使用全局当前工作区。数据库是正式状态 owner；文件编辑是候选，需预览、校验、显式 Apply。
- 改已有文件前先读。覆盖、合并、未知工作先预览并询问；未经确认不使用 `--force`。所有写入限本项目；不修改全局 Git 身份或系统安全策略。
- 本仓开发/验证产生的日志与诊断仅放 Git 忽略的 `.data/logs/`；工具、缓存、机器状态、凭据不得进入版本控制。所有测试使用合成数据。
- 旧项目仅作为有界只读参考；观测范围及容量证据见 [迁移基线](<docs/migration-baseline.md>)。不复制真实库、密钥、活授权或旧审批。
- 文档标明 proposed/designed/implemented/partial/deferred/retired；未执行设计不得称 accepted ADR，未运行测试不得称通过，M1 不等于完整重写完成。
- 验证按真实闭环：保存/重开、第二进程竞争、失败/未知结果、串区/越权与实际排空；编译、Mock 和健康检查仅证明各自范围。
