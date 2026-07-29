# Rust 编译包打包策略

Rust 能产出自包含二进制。Linux 用 pinned 静态集 `pkgs2605Static`（musl64 cross）。Go 编译包见
[go.md](file:///workspace/standalone-binaries/docs/package-strategies/go.md)。

## zellij

`packages/zellij`，base=**pinned 静态集 `pkgs2605Static`**（26.05 pinned static env，非默认 unstable）。
直接构建 `zellij-unwrapped`（nixpkgs 的 `zellij` 只是 symlinkJoin wrapper，无 `extraPackages` 时不增值），
少一层 derivation。

- Linux musl64 cross 静态集；`build==host==x86-64` 导致 checkPhase 不自动跳过。
- workaround：`doCheck = false` / `doInstallCheck = false`。根因：cargoCheckHook 会构建 `zellij` 测试
  target，其静态链 libcurl against libssh2，libssh2 1.11 符号因链接顺序未解析而失败。

> `miniserve` 曾计划纳入（`local.nix` 该行已注释、目录不存在），当前无 Rust 目录级 case 需覆盖。

## Rust runtime 依赖

一些非 Rust 主体的包会链入 Rust 编译的 runtime 依赖，其静态构建坑归在对应生态文档：node26 的
`temporal_capi`、pydantic-core（lief 的 Python bindings 拖进）等见
[nodejs.md](file:///workspace/standalone-binaries/docs/package-strategies/nodejs.md)。
