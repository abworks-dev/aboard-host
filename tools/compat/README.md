# 固定 Windows 刷写兼容修正

本目录仅维护 `modernc.org/sqlite v1.60.1` 所配 `modernc.org/libc v1.77.1` 的一处修正：`XFlushFileBuffers` 失败从 `-1` 改为 Win32 BOOL 的 `0`，保留 errno 和成功值 `1`。原始失败、批准及复验见[001研究证据](<../../specs/001-host-workspace-foundation/research.md>)；上游版权与条款见[许可证](<LICENSE.libc>)。这不是官方发布修复，也不宣称硬断电耐久性。

从项目根运行：

```powershell
./tools/go.ps1 -GoArgs @('mod','download')
./tools/go.ps1 -GoArgs @('test','./internal/storage/sqlite/...','-run','TestProfileGate','-count=3','-v','-timeout','90s')
```

构建/test/vet/run/list必须使用[项目runner](<../go.ps1>)。Go 1.27禁止覆盖GOMODCACHE，因此不是直接 `-overlay`：准备脚本校验固定版本与原文件SHA，逐文件产生/核对忽略目录内的源码副本，仅该函数有差异，再生成临时modfile和sum，使用 `-mod=readonly`。不修改原模块缓存、共享go.mod/sum、全局Go、第三方仓库或生成Spec Kit工具。约86MB/4199文件的本地副本是可再生构建缓存，不进入Git，不维护源码fork；每次全树哈希有开销。版本/源不符、额外文件、半成品或并发准备不默默修复/覆盖，而是报错停止。runner持独占构建输入句柄至Go退出，不提供并行runner保证；中断半成品只能在核对明确绝对路径后清理重建。

[运行时检查](<../../internal/storage/sqlite/flush_guard_windows_amd64.go>)在任何建库前验证真实失败返回，遗漏修正的普通Go构建拒绝打开/创建，而非静默降级。失败注入仅位于测试文件；不是生产开关。负向控制[独立程序](<../../tests/fixtures/check_unpatched_guard/main.go>)须在runner设置的项目局部环境中故意不带临时modfile运行，不应作为正常客户端启动。

维护与移除：不孤立升级libc、不自动换驱动或profile。核实未来SQLite正式要求的组合确实修复BOOL错误及兼容性后，重新固定版本/校验和，先保留并通过BOOL负例、真实COMMIT逐刷写故障、正常/强杀重开、遗漏修正控制及受影响回归，再删除此准备机制。版本/匹配不符即停；任何进一步依赖分叉或扩大补丁需另行决策。发行时仍须履行固定依赖的原始第三方许可证义务，不能仅包含本目录的一份许可证。
