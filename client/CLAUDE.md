# Client Install / Upgrade Model（`client/`）

本文档描述 `sb` client 的设计。它是本仓库产物的消费端；构建/发布模型见根目录
[CLAUDE.md](file:///workspace/standalone-binaries/CLAUDE.md)。

[client/](file:///workspace/standalone-binaries/client) 是一个小型包管理器（brew/apt 风格的 client），面向**没有 Nix/Homebrew/包管理器**的最小化环境。它用 **Go** 写成单个静态链接二进制（`sb`），交叉编译到 `linux-x86_64` 和 `darwin-arm64`。它直接从 `ghcr.io/curoky/standalone-binaries` OCI registry 拉取已发布的 tarball（复用根 CLAUDE.md 里描述的 `<name>-<arch>` tag -> layer blob digest 流程）并本地安装。

## Design principles

- **无宿主运行期依赖。** sb 是一个以 `CGO_ENABLED=0` 构建的静态二进制；目标主机上**什么都不需要**（`curl`/`tar`/`oras`/`jq`/Nix 都不要）。它依赖维护良好的 Go 库而非手写管道：[go-containerregistry](https://github.com/google/go-containerregistry)（`crane`）负责全部 OCI 访问（auth、manifest、layer pull、digest），[cobra](https://github.com/spf13/cobra) 做 CLI，[`x/sync/errgroup`](https://pkg.go.dev/golang.org/x/sync/errgroup) 做有界并行 fan-out，[mpb](https://github.com/vbauerster/mpb) 做并发下载进度条。tarball 解压用标准库（`archive/tar` + `compress/gzip`）。同一份源码交叉编译到两个平台。
- **可 relocate 的安装。** 一个包在 prefix 下暴露的一切都是**相对** symlink。因为链接是相对的，整个 prefix 可以移动到任何地方而**零修复**。
- **包彼此独立。** 每个包都被当作完全自包含；sb **不做依赖解析**。每个包独立安装、移除、relocate。运行期重的包（node/python/perl 工具）自带相对路径 wrapper，所以单个 `store/<name>/` 目录也是自包含的。
- **Platforms。** 自动探测的 arch tag 是 `linux-x86_64`（Linux/x86_64）或 `darwin-arm64`（macOS/arm64），与已发布的 OCI tag 一致；可用 `--arch` 覆盖。
- **Self-publishing。** sb 自身也由 [build-sb.yaml](file:///workspace/standalone-binaries/.github/workflows/build-sb.yaml) 作为 `sb-<arch>` 发布到 registry，因此可用单条 `curl` bootstrap，之后像任何其他包一样升级。

## Source layout

[client/](file:///workspace/standalone-binaries/client) 是一个 Go module：

- [main.go](file:///workspace/standalone-binaries/client/main.go)：整个 client（OCI 访问、store/meta/link、命令、CLI）。
- [main_test.go](file:///workspace/standalone-binaries/client/main_test.go)：离线单测（tar 解压 + 相对链接 relocate + arg parsing + metadata round-trip）。
- [install.sh](file:///workspace/standalone-binaries/client/install.sh)：`sb` 自身的 **bootstrap installer**。全新主机上没有 `oras`/Go/Nix，只有 `curl` + `tar`，所以不能用 `sb` 装 `sb`。它直接经 ghcr registry HTTP API 拉 `sb-<arch>` artifact（匿名 pull token -> manifest -> 单个 layer blob），解出 `sb/sb`，把二进制放进安装目录（默认 `~/.local/bin`；可用 `SB_INSTALL_DIR`/`--prefix` 和 `SB_ARCH`/`--arch` 覆盖）。设计为 `curl -fsSL <raw-url>/install.sh | bash` 运行。这一次性 bootstrap 之后，`sb` 像任何其他包一样自升级。

## Local layout

包安装在一个 prefix 下（默认 `/opt/sb`）：

- `store/<name>/`：解出的包内容。
- `store/<name>/.sb-meta`：每包元数据，放在**包目录内部**，从而与包原子地一同创建/删除。它是一个纯 `key=value` 文件（`name`、`arch`、`digest`、`linked`、`installed_at`）。
- `bin/`、`lib/`、`share/`...：以 `--link`（默认）安装时，是指向 `store/<name>/` 的**相对** symlink（`.sb-meta` 不参与 link）。`--nolink` 只装进 store。

## Upgrade semantics（digest comparison）

OCI tag（`<name>-<arch>`）里没有人类可读的版本号，所以「是否需要更新」由 **OCI blob digest 比较** 决定：client resolve 远端 manifest 的 layer digest，与本地 `.sb-meta` 里记录的 `digest` 比较。不同则重新下载+解压；否则跳过（除非 `--force`，`install` 是幂等的）。

## Subcommands

- `install <pkg>...`：安装/刷新**一个或多个**包；本地 digest 已匹配远端的会被跳过（`--force` 覆盖）。`--link`/`--nolink` 控制 symlink 暴露。多包安装分三阶段：(1) **并行** resolve 每个包的远端 digest——任一缺失则 sb 报出完整列表并中止、什么都不装；(2) **并行**把需要的 blob 下载进 cache；(3) **串行** 解压 + link。
- `remove <pkg>`：移除包的 symlink（若已 link）并删除其 `store/<name>/` 目录。
- `upgrade [pkg...]`：升级给定的包，或不给参数时升级所有已装包（复用 install 的 digest-skip 逻辑，保留各包记录的 arch/linked）。
- `info <pkg>`：显示一个包记录的元数据（未装则显示其 registry 坐标）以及是否相对远端 digest 最新。
- `list`：读取每个 `store/*/.sb-meta`，列出已装包及其记录的 digest。
- `outdated`：报告远端 digest 已变的已装包。
- `sync [file]`：安装/刷新一个 **YAML manifest**（默认文件名 `sb.yaml`，或显式路径参数）里声明的每个包——sb 版的 Brewfile / pyproject。包在 `packages:` 下声明，分为 `link:`（装进 store 并 link 到 prefix 根）和 `unlink:`（只装进 store），另有可选的 `arch:` / `prefix:` 默认值（显式 `--arch` 总是优先；`prefix:` 仅在命令行未传 `--prefix` 时使用——log 文件仍在 flag 的 prefix 下）。它复用 install 的 digest-skip 逻辑，所以重跑是幂等的。`sync` 期间，`link`、`unlink`、`profiles` 引用的包先合并成一个去重的安装计划，于是 digest resolve 与下载在一个共享 batch 里完成；之后 sb 调和每个包最终的 root-link 状态，再把 profile 包 link 到 `<prefix>/profile/<name>/` 下。若一个包出现在多个 manifest 段里，root linking 优先于 store-only/profile-only 引用。加 `--prune` 时它还会移除 manifest **未**引用的已装包（走正常 `remove` 路径），使已装集合与 manifest 完全一致。manifest 也可声明 **profiles**（`profiles: { <name>: [<pkg>...] }`）：每个 profile 把其包装进 store，并经相对 symlink 把它们的文件聚合到 `<prefix>/profile/<name>/` 下。link 拆分与 profiles 是 sync 专属概念——client 其余部分（`install`/`remove`/`list`）不感知。

通用选项：`--prefix PATH|--prefix=PATH` 和 `--arch ARCH|--arch=ARCH`（`--opt value` 和 `--opt=value` 两种形式都接受；选项可出现在包名前或后）。`--verbose` 额外把详细日志镜像到 stderr。

## Logging

每次调用都把一份详细的结构化（slog text）日志写到 `<prefix>/sb.log`（prefix 不存在则创建）。终端显示简化的关键步骤输出（`> ...` 行，含安装进度与运行结束时带耗时的 summary），外加下载阶段的 per-package 字节级进度条（由 [mpb](https://github.com/vbauerster/mpb) 渲染）；完整的 per-package resolve/download/extract/link 事件与阶段计时进日志文件。`--verbose` 会把该日志也 stream 到 stderr（走 stderr，不与 stdout 上的进度条冲突）。

这是 client 侧的事：根 CLAUDE.md 的 CI/publishing 模型不变，因为比较依赖的是 `ghcr.io` 在 `oras push` 时已算好的 layer digest。
