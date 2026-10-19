# AB Agent-first Baseline

一个面向中文使用者的 Spec Kit 工程提示词 preset：让 Agent 在现有项目规则和已授权范围内，从一句自然语言目标推进实现、验证、修复与交付，必要时才打断用户。

- **标识 / 版本**：`ab-engineering-baseline` / `0.1.0`，见 [preset.yml](<preset.yml>)。
- **兼容基线**：官方 Spec Kit **1.2.1**；开发依赖固定提交 `9dca9bf42a031765ee5e5f2371137f2f26967e11`。
- **交付形态**：新增 `speckit.ab.run`，以 `replace` 替换 `speckit.specify`、`speckit.plan`、`speckit.tasks`、`speckit.implement`。只有这一个 preset，不需要先安装 Lean。
- **不提供**：运行时、后台守护进程、扩展依赖、Hook、MCP、原生权限控制、硬预算或跨会话自动续跑；不覆盖项目宪法或贡献者规则。

> **验证状态**：首版尚未完成独立的真实宿主 / LLM 行为验收。构建、静态检查和官方 CLI 隔离安装测试，不等于模型行为或真实业务已通过；实测命令、修订与结果以[验证记录](<docs/verification.md>)为准。`0.1.0` 是包版本，不表示已经发布 GitHub Release。

## 快速开始：克隆源码，安装干净的本地包

下面是 **PowerShell** 示例。需要 Git、Python 3.11+、uv，以及能够识别项目 skills 的实际 Agent 宿主。[源码仓库](<https://github.com/abworks-dev/spec-kit-preset-ab>)默认为私有，需要访问权限才能克隆；源码已推送不表示 GitHub Release 已发布。

```powershell
# 1. 获取源码并生成可安装 payload；不要把源码仓库根目录交给 --dev
 git clone https://github.com/abworks-dev/spec-kit-preset-ab.git
 cd spec-kit-preset-ab
 uv sync --locked
 uv run python tools/package.py
 $presetPayload = Join-Path (Get-Location).Path 'dist/ab-engineering-baseline'

# 2. 安装固定版本的官方 CLI，创建与源码仓库分离的演示项目
 uv tool install specify-cli --from git+https://github.com/github/spec-kit.git@v1.2.1
 cd ..
 specify init ab-demo --integration dsh --script ps
 cd ab-demo

# 3. 只从 allowlist payload 安装，检查元数据与实际解析来源
 specify preset add --dev "$presetPayload" --priority 10
 specify preset info ab-engineering-baseline --json
 specify preset resolve speckit.ab.run
 specify preset resolve speckit.specify
```

`uv run python tools/package.py` 是**本项目开发工具**，不是 Spec Kit 原生命令。它生成干净的 payload 目录 `dist/ab-engineering-baseline/`、ZIP 包 `dist/ab-engineering-baseline-0.1.0.zip` 及校验文件 `dist/ab-engineering-baseline-0.1.0.zip.sha256`；这些是构建后的输出路径，新克隆中不存在。首次从提交完整的源码打包即可；修改提示词源后，应先运行 `uv run python tools/build.py` 再打包。

**为什么不直接 `--dev .`？** 官方 v1.2.1 的本地安装会复制整个输入目录，不过滤开发目录。直接传源码根可能连同 Git 历史、虚拟环境及无关文件一起复制。只使用打包工具准备的 payload；`--dev` 是复制安装，不是软链接，也不会热更新。

如本机已有其他版本 CLI，请先确认所用环境确为 1.2.1，不要因本教程盲目覆盖共享工具环境。CLI 版本匹配只说明安装接口兼容；宿主是否实际加载 skill，仍须在该宿主中确认。安装到已有仓库前，先阅读[采纳指南](<docs/adoption.md>)，不要在生产仓库直接试装。

### 在宿主中启动

打开 `ab-demo` 作为宿主工作目录。官方 v1.2.1 生成的是 **skills**，不是旧版 Claude commands：

| integration | 新入口的生成位置（演示项目内） | 宿主内调用 |
| --- | --- | --- |
| `dsh` | `.dsh/skills/speckit-ab-run/SKILL.md` | `/speckit-ab-run` |
| `claude` | `.claude/skills/speckit-ab-run/SKILL.md` | `/speckit-ab-run` |
| `codex` | `.agents/skills/speckit-ab-run/SKILL.md` | `$speckit-ab-run` |

例如在 DSH / Claude 对话中输入一段目标即可开始，不要求先手写三份文档或提供固定参数：

```text
/speckit-ab-run 修复搜索结果在切换筛选条件后仍显示旧数据的问题，遵循本仓库规则，补充回归测试；不要修改公开接口，不要提交或推送。
```

Codex 使用相同自然语言，将入口改为 `$speckit-ab-run`。单阶段仍可单独调用：DSH / Claude 使用 `/speckit-specify`、`/speckit-plan`、`/speckit-tasks`、`/speckit-implement`；Codex 使用对应的 `$speckit-…`。**单阶段只完成该阶段，不得据此擅自继续后续阶段。**

输入 `speckit.ab.run` 是说明 manifest 中的逻辑命令名，并不是新增 shell 可执行命令。`generic` integration 在 v1.2.1 不注册 preset command / skill 覆盖，不在本包支持范围内。

## 工作方式与边界

- **采纳，而非另立规则**：先识别项目已有指导、宪法、构建与测试约定。preset 是执行方法，不是第二个权威来源；冲突不得静默改写本地规则。
- **以现有 feature 主线为准**：沿用 `specs/` 中的规格、计划与任务，并遵循 `SPECIFY_FEATURE_DIRECTORY` 优先于 `.specify/feature.json` 的 `feature_directory` 定位契约，不依赖分支名另造主线。简单修复按需要保留最小证据，不强制生成 `spec.md`、`plan.md`、`tasks.md` 三件套。
- **先接入项目流程，再自主推进**：项目声明的澄清、明确批准、清单、分析门禁优先于通用四阶段与小修路径；“端到端完成”不等于规格批准。已被识别且覆盖当前对象的有效批准不重复索取；没有这些项目门禁时，不凭空增加人工审批。
- **默认推进，例外询问**：在已授权范围内自行检索、实现、测试、定位问题并修复；仅在项目要求的人工门禁、关键歧义、规则冲突、缺失权限或真实外部阻塞等情况请求决定。
- **复用意图，有界反馈**：从项目已有且适用的产品 / brief / 架构 / 决策记录理解 Why，不强制新建 Motivation 文档。端到端授权内可依据新证据局部修订技术计划及受影响任务；改变目标、业务语义、验收或权限仍须人决定，单阶段请求不因此获得跨阶段修改权。
- **最小同步，证据重验**：保留稳定 ID 和无关有效工作，只同步受影响部分。历史通过不自动证明修改后的对象通过：保留原结果和上下文，对失去覆盖的当前结论标明 `unverified`，重做必要验证；不能重跑就如实交接，不抹掉历史或全量清零。
- **报告可核验事实**：区分运行过的命令、失败与修复、未覆盖范围、自检和独立审查。没有运行的行为用例不能写成通过。
- **权限仍在宿主**：文件 / 网络 / 凭据隔离、时间与 token 硬上限、自动续跑由宿主配置。本包的 `run` 是自包含 prompt，不是 daemon，也不会扩大现有授权。

完整输入、状态与交付约定见[执行契约](<docs/runner-contract.md>)。

## 安装后务必知道

1. `.specify/templates/overrides/` 的项目本地覆盖优先于 preset；preset 之间数字更小的 priority 优先。同优先级按 preset ID 排序。安装成功不等于本包已赢得实际解析，须检查 `resolve` 和生成 skill。
2. 正常初始化的项目只向当前 active integration 注册覆盖。切换 integration 时由官方 CLI 重新物化；不要以另一个宿主目录中的旧文件判断当前状态。
3. `preset disable` 只影响解析，**不会卸掉已经生成的命令**。要移除本包及恢复其命令来源，使用 `specify preset remove ab-engineering-baseline`，之后仍须检查实际文件；这不回滚业务代码。
4. `preset update` 是先 remove 再 add，没有候选来源预检、事务或自动回滚。更新前保存旧包与安装元数据，在隔离临时项目预演；详细恢复步骤见[采纳指南](<docs/adoption.md>)。

## 维护与验证

以下均在**本源码仓库**运行，是本项目工具而非 Spec Kit 内置命令：

```powershell
uv sync --locked
uv run python tools/build.py
uv run python tools/build.py --check
uv run python tools/validate.py
uv run pytest -q
uv run python tools/test_install.py --integrations dsh claude codex --report "$env:TEMP/ab-preset-install-report.json"
uv run python tools/test_install.py --integrations dsh --script ps --report "$env:TEMP/ab-preset-dsh-ps-report.json"
uv run python tools/package.py --output-dir dist
```

安装测试仅使用固定官方 CLI 和确定性的隔离临时 fixture，不调用 LLM、不使用真实凭据或生产环境。[行为用例](<tests/behavioral>)只定义真实 Agent 应验证的情形，不代表已执行或通过。验证事实由[验证记录](<docs/verification.md>)记载；包审查与手动发布流程见[发布指南](<docs/release.md>)。打包或普通源码推送不会自动创建 tag、Release 或公开仓库。

## 文档与许可

- [现有项目采纳指南](<docs/adoption.md>)
- [Host 类项目接入与验证分层](<docs/host-adoption.md>)
- [执行契约](<docs/runner-contract.md>)
- [发布指南](<docs/release.md>)
- [变更记录](<CHANGELOG.md>)
- [MIT 许可证](<LICENSE>) · [第三方归因与完整许可声明](<THIRD_PARTY_NOTICES.md>)

设计参考了 [github/spec-kit 的 Lean preset（固定提交）](<https://github.com/github/spec-kit/tree/9dca9bf42a031765ee5e5f2371137f2f26967e11/presets/lean>) 的精简、自包含命令思路；本包不内置上游 CLI，CLI 仅作为本仓库开发与安装核验依赖。不包含任何私有业务项目原文或机器路径。
