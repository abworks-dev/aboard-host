# 第三方归因与许可声明

## github/spec-kit — Lean preset 与 Spec Kit

本项目参考了官方 Lean preset 的精简阶段流程与自包含命令设计，并据此设计独立的工程提示词政策。为明确来源，即便只是受到设计启发，仍在此保留上游完整 MIT 声明。

- 上游项目：[github/spec-kit](<https://github.com/github/spec-kit>)。
- 核验基线：Spec Kit `v1.2.1`，提交 `9dca9bf42a031765ee5e5f2371137f2f26967e11`。
- 参考内容：[该提交的 Lean preset](<https://github.com/github/spec-kit/tree/9dca9bf42a031765ee5e5f2371137f2f26967e11/presets/lean>)。
- 许可来源：[该提交的 LICENSE](<https://github.com/github/spec-kit/blob/9dca9bf42a031765ee5e5f2371137f2f26967e11/LICENSE>)；上游版权行为 `Copyright GitHub, Inc.`，未自行补写年份。

本包不内置或重新分发 Spec Kit CLI、其依赖或完整上游仓库，也不要求先安装 Lean。CLI 只列入本源码仓库的开发依赖，用于固定版本的确定性安装核验；使用者另行安装官方 CLI。上游及其依赖各自适用的许可不被本项目许可证取代。

下列为上述固定提交根目录许可证的完整声明：

```text
MIT License

Copyright GitHub, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

本项目新增内容由 AB 按 [MIT 许可证](<LICENSE>)提供。分发时应保留本文件及其对应许可声明；第三方项目名称用于归因，不表示其对本包作出背书或兼容性保证。
