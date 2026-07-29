# Go 构建案例

Linux 上普通 Go 包直接使用 manifest 的 `pkgsStatic`，无需在本文列出。这里仅记录 podman 容器栈和
macOS 无 CGO 重建。容器栈的 C 组件见 [C / autotools](c-autotools.md)。

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

## macOS `CGO_ENABLED=0`

`packages/local.nix` 的 `goWithoutCgo` 从 native `pkgs2511` 选择一组 Go 工具，并设置
`CGO_ENABLED=0`。这样产物不再链接 Nix dylib。当前列表以 `local.nix` 为准，不在文档复制。

`lark-cli` 是例外：它来自 unstable，且上游 CGO build 已只链接系统库。强制关闭 CGO 会让纯 Go
二进制保留 Go compiler store path，并触发 `disallowedReferences`，因此不加入 `goWithoutCgo`。

`pkgs2511` pin 和列表中的每个 CGO override 都需要定期对 unstable 重新验证。若上游默认产物已无
`/nix` dylib，应回归 manifest 或普通包。
