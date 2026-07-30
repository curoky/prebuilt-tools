# C / autotools 构建策略

默认直接使用 `pkgsStatic.callPackage`。不要机械追加 `-static`；静态工具链和最终
artifact validation 共同保证产物约束。

## 资源与 helper

程序依赖数据、证书、插件或 helper 时，使用相对资源 wrapper：

- 真实入口改名为 `_<name>`；
- wrapper 从自身路径计算 package root；
- 资源和 helper 路径相对 package root；
- 静态网络工具随包携带 CA bundle。

不要依赖 artifact assembly 猜测并修复二进制内的任意硬编码路径。

## 静态链接

静态链接不会自动解决传递依赖和 feature 选择：

- 只补齐实际需要的静态传递库；
- 关闭会强制引入 shared library 的可选 feature；
- build tool 必须来自 build platform，不能误用 target 静态包；
- checks 只能在确有平台限制时禁用，并登记到 `TODO.md`。

复杂的 linker 或 feature-reduction 方案见
[特殊案例](special-cases.md)。

## s6

`execline`、`s6`、`s6-linux-init` 和 `s6-rc` 会把依赖前缀写入二进制或生成脚本。
这些包必须：

- 禁用 absolute-path 配置；
- 清除相关 `*PREFIX` 宏；
- 在依赖链中继续使用本仓库修正后的 `execline` 和 `s6`；
- 保证生成的 shebang 和 helper 路径不引用 Nix store。

## 工具裁剪

只发布产品需要的 output。例如 clang-tools 只提取 `clang-format`，PostgreSQL 只发布
psql client。裁剪边界是产品契约，具体实现和版本以 `packages/` 与 `TODO.md` 为准。
