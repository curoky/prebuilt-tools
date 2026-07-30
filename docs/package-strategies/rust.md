# Rust 包

普通 Rust CLI 直接使用 unstable `pkgsStatic`。本地 derivation 只保留产品 wrapper 或当前
静态构建所需的最小 override；状态见 [`TODO.md`](../../TODO.md)。

## 产品 Wrapper

`miniserve` 的 wrapper 设置仓库定义的默认功能开关，再执行包内真实二进制。这些默认值属于
发布产品行为，不应在上游包可直接构建时一并删除。

## 直接使用 Unwrapped 输出

`zellij` 直接构建 unstable `zellij-unwrapped`。上游 `zellij` 仅在注入额外 PATH 内容时提供
价值，本仓库不需要该 wrapper 层。

当前构建关闭 checks；具体原因和回归条件只在 `TODO.md` 维护。最终可执行文件仍必须通过 Linux
musl 全静态校验。

## 构建工具链

Rust 依赖在 Linux musl cross 环境中应复用 build 平台可 substitute 的 glibc rustc/LLVM。
局部 overlay 不得修改 `buildPackages` 中的同名依赖；只针对 target 的 override 必须以
`stdenv.hostPlatform.isStatic` 限定。Node.js 中的 Rust 依赖遵守同一规则，见
[Node.js](nodejs.md)。
