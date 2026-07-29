# Rust 构建案例

普通 Rust 包使用 manifest 的 `pkgsStatic`。本地 Rust 包目前有 `miniserve` 和 `zellij`。

## miniserve

`packages/miniserve` 从当前 `pkgsStatic.miniserve` 构建，保留静态真实二进制并增加 wrapper。
wrapper 默认打开 tar、tar.gz、zip、目录优先、symlink 信息和 wget footer 等环境开关，再执行
`_miniserve`。

这是产品行为 wrapper，不是编译 workaround。回归上游时不能只比较构建成功，还要确认这些默认行为
是否仍然需要。

## zellij

`packages/zellij` 使用 pinned 静态集 `pkgs2605Static`，不是默认 unstable。
直接构建 `zellij-unwrapped`（nixpkgs 的 `zellij` 只是 symlinkJoin wrapper，无 `extraPackages` 时不增值），
少一层 derivation。

- Linux musl64 cross 静态集；`build==host==x86-64` 导致 checkPhase 不自动跳过。
- 临时 workaround：`doCheck = false` / `doInstallCheck = false`。根因：cargoCheckHook 会构建 `zellij` 测试
  target，其静态链 libcurl against libssh2，libssh2 1.11 符号因链接顺序未解析而失败。

回归条件：unstable 的 `zellij-unwrapped` 在当前 musl cross static 环境可构建，且 test target
不再因 libcurl/libssh2 链接顺序失败。回归时删除 `pkgs2605Static` 参数和禁用检查的 override。

## Rust runtime 依赖

一些非 Rust 主体的包会链入 Rust 编译的 runtime 依赖，其静态构建坑归在对应生态文档：node26 的
`temporal_capi`、pydantic-core（lief 的 Python bindings 拖进）等见
[Node.js](nodejs.md)。
