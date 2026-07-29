# Go 编译包打包策略

Go 能产出自包含二进制，两平台策略差异是「Linux 走 musl 全静态、macOS 减动态依赖」：

- Linux 用 `pkgsStatic`（musl 全静态）。
- macOS 用 native `pkgs` + `CGO_ENABLED=0` 摆脱 nix dylib。

容器栈 `podman`（Go）与 `crun/conmon/catatonit/gpgme`（C）虽同属一栈，但 Go/C 分别处理——C 组件见
[c-autotools.md](file:///workspace/standalone-binaries/docs/package-strategies/c-autotools.md) 的容器栈 C 组件一节。Rust 编译包见
[rust.md](file:///workspace/standalone-binaries/docs/package-strategies/rust.md)。

## 容器栈：podman（Go）

`packages/podman`，base=`pkgsStatic`（Linux musl 全静态）。用 `podman.override` 把
`conmon/catatonit/crun/runc` 换成本仓库静态版，再 `overrideAttrs`。

**runc wrapper workaround（核心）：** runc 经上游 `extraRuntimes` 进入 `helpersBin`。pkgsStatic 下真实
runc 已全静态，但上游 installPhase 对它跑 `wrapProgram`，把静态二进制改名 `.runc-wrapped` 并装一个小的
**动态** launcher `runc`（引用 /nix musl interpreter + rpath）。podman 的 helpersBin 会 ship 这个
launcher，导致复制过来的 `runc` 变动态并依赖 /nix，触发可移植性检查失败。（独立 `.#runc` 输出没这问题，
因 normalize.sh 把 `.runc-wrapped` 改名盖回 launcher。）故 `runcStatic = runc.overrideAttrs` 重写
installPhase，直接 `install -Dm755 runc` 装静态二进制、去掉 wrapper。

路径硬编码：patch `hardcode-paths.patch`（`bin_path = /opt/podmanx/libexec/podman`）与
`rm-podman-mac-helper-msg.patch`；`env.HELPER_BINARIES_DIR2`、LDFLAGS 注入 `-X
...adminOverrideConfigPath=/opt/podmanx/conf/`。

## macOS 上的 Go 工具（`CGO_ENABLED=0`）

无 per-package 目录，经 [local.nix](file:///workspace/standalone-binaries/packages/local.nix#L19-L44) 的 `goWithoutCgo`：

```
goWithoutCgo = lib.genAttrs
  [ "gdu" "gh" "bazelisk" "croc" "go-task" "git-lfs" "shfmt" "fzf"
    "dive" "scc" "buildifier" "lefthook" "oras" "lark-cli" ]
  (name: pkgs2511.${name}.overrideAttrs (oldAttrs: { env.CGO_ENABLED = "0"; }));
```

base=pinned native `pkgs2511`（非 pkgsStatic）。纯 Go（无 cgo）二进制在 macOS 上自然摆脱 nix dylib
依赖——这是 macOS 部分静态 ladder 的一环，而非 musl 全静态。该集合经 `darwin = goWithoutCgo // rec
{ ... }` 合入 darwin 包集。

> 对比：Linux 上这些工具多数直接从 manifest 取 `pkgsStatic` 版；macOS 无 musl 全静态工具链，故退一步用
> `CGO_ENABLED=0`。
