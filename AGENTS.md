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
- 优先验证可能推翻方案的最小端到端闭环；非阻断优化不新增为推进前置。
- 验证按变更影响分层执行，遵守既定验收要求，避免无依据重复广回归；验证脚本不得隐式修改源码。

## Spec Kit 文档维护规则

- 开工先读宪章、唯一授权、目标功能及 locator/进程覆盖；本规则不新增业务授权或第二套需求/任务系统。README保留产品、当前状态、设计追踪三条阅读路，只引用原owner，不复制进度表。
- spec承载用户问题、范围/非目标、US/FR/SC及批准状态，不混工程日志；plan承载设计、依赖、验证与决策；tasks是本仓任务进度唯一owner；analysis只读复核，不隐式修复；authorization是权限唯一记录。research保留有界实验/证据，不代当前进度。
- 顺序为事实校准 → 结构导航 → 需求/设计/任务/场景追踪。保留原编号、任务复选框整行、承诺及旧标题/显式anchor兼容；不重建、不重编号、不删搬带日期历史，不改旧绝对证据。当前概述与历史分层，历史“未验”不覆盖后来真实结果，后来绿也不抹旧失败。
- 新需求须有来源和批准，并映射设计、原任务与验收场景；缺口明确标为unmapped，不编造覆盖。implemented/partial/unverified与真实联合、发行、用户验收分轴陈述；语义和授权由人/只读一致性复核，不能按关键词或静态绿灯推断。
- 跨仓只引用契约/状态/证据的原owner，不拷贝第二owner，不续用冻结交接签名。每次文档delta仍须按实际交接核输入/漂移；本规则和检查工具不能重签旧packet或扩大权限。
- 文档维护完成前必须执行本项目[静态检查](<tools/check_docs.py>)及[合成工具测试](<tools/test_check_docs.py>)，用法/解析子集见[README](<README.md#文档维护静态检查>)。默认维护模式保护原任务整行；真实业务开发可显式用development模式并记录原因，但不因此证明任务/验收完成。限定文件检查必须声明非全仓及未检查的反向引用，不为绿灯偷偷换base、筛掉错误或加allowlist；失败保留、按原owner/范围处理。
- 已新增[文档静态检查CI配置](<.github/workflows/docs-check.yml>)，按[事件不可变基线规则](<README.md#文档静态检查ci>)执行全域maintenance及专用合成工具测试；本地配置未提交推送不等于远端Actions已运行，也未配置分支保护/必需检查、push hook或其他服务端强制门。静态检查、规格质量checklist、只读一致性analysis与业务验收互不替代；纯文档不触发无依据的业务广回归，也不豁免实际受影响的既有技术门。
