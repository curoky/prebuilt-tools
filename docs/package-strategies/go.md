# Go 构建案例

Linux 上普通 Go 包直接使用 manifest 的 `pkgsStatic`，无需在本文列出。这里仅记录 podman 容器栈和
macOS native Go 选择。容器栈的 C 组件见 [C / autotools](c-autotools.md)。

## podman

`packages/podman`，base=`pkgsStatic`（Linux musl 全静态）。用 `podman.override` 把
`conmon/catatonit/crun/runc` 换成本仓库静态版，再 `overrideAttrs`。

上游 `extraRuntimes` 把 runc 加入 `helpersBin`。虽然 `pkgsStatic.runc` 的真实二进制是静态的，
其 install phase 会运行 `wrapProgram`，产生引用 Nix interpreter 和 rpath 的动态 launcher。
独立 runc output 会在 normalization 时用 `.runc-wrapped` 覆盖 launcher，但 podman 提前复制的是
launcher。

本地 `runcStatic` 因此重写 install phase，直接安装真实静态 runc。不要删除这层 override，除非确认
podman 的 `helpersBin` 已复制静态入口。

路径硬编码：patch `hardcode-paths.patch`（`bin_path = /opt/podmanx/libexec/podman`）与
`rm-podman-mac-helper-msg.patch`；`env.HELPER_BINARIES_DIR2`、LDFLAGS 注入 `-X
...adminOverrideConfigPath=/opt/podmanx/conf/`。

## macOS native Go

一组跨平台 Go 工具在 `manifests/default.nix` 的 `aarch64-darwin` 配置中 pin 到 `25.11`，并设置
`isStatic = false`。这会选择 native pkgs、保留 CGO；它们的上游 CGO build 已只链接 `/usr/lib`
和系统 frameworks，不包含 Nix dylib。Linux 仍使用 unstable `pkgsStatic` 的 musl-static 产物。

不要恢复旧的 `CGO_ENABLED=0` override：纯 Go 二进制会保留 Go compiler store path，并触发
`buildGoModule` 的 `disallowedReferences`。`lark-cli` 使用同一 native CGO 原则，但来自 unstable，
因此只有 `isStatic = false`，没有版本 pin。

这些 `25.11` pin 都需要按根 `TODO.md` 对 unstable native 构建定期回归。若 unstable 产物仍只依赖
系统 dylib，就删除对应平台的 version pin；`isStatic = false` 只有在 unstable `pkgsStatic` 也能满足
macOS portability 时才一起删除。
