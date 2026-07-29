# sb Client Agent Guide

`client/` 是本仓库 OCI artifact 的消费端。它实现单个静态 Go 二进制 `sb`，面向没有 Nix、
Homebrew 或 `oras` 的最小环境。构建端协议与全局约束见根 [`CLAUDE.md`](../CLAUDE.md)。

本文是 client 的公开契约、状态模型和实现约束 source of truth。

## 不变量

1. **无宿主运行时依赖。** `sb` 必须保持 `CGO_ENABLED=0` 可构建，不调用外部
   `curl`、`tar`、`oras`、`jq` 或 Nix。
2. **安装可整体移动。** package root 和 profile 中的链接必须是相对 symlink；移动 prefix
   后无需修复。
3. **包彼此独立。** client 不做依赖解析。脚本工具需要的 Python、Perl 或 Node.js runtime
   由发布包的 sibling wrapper 约定解决。
4. **digest 是版本标识。** 远端 OCI layer digest 与本地 `.sb-meta` 相同就跳过安装，
   `--force` 除外。
5. **多包 resolve 具有写入前原子性。** 必须先 resolve 全部请求；任一 tag 不存在时报告完整
   缺失列表，不安装任何包。
6. **平台协议固定。** 自动探测只支持 `linux-x86_64` 和 `darwin-arm64`，可由
   `--arch` 覆盖。

## 远端协议

registry 固定为：

```text
ghcr.io/curoky/standalone-binaries:<package>-<arch>
```

发布 workflow 为每个 artifact 写入一个 tar.gz layer，归档顶层目录为包名。client 当前取
manifest 的最后一个 layer，并用其 digest 和内容。增加额外 layer 会改变 client 选层语义。

`sb` 自身使用 `sb-<arch>` tag，由 `.github/workflows/build-sb.yaml` 发布。bootstrap
installer 通过 GHCR HTTP API 获取匿名 token、读取 manifest、下载 layer，再从归档中的
`sb/sb` 安装二进制。

改变 registry、tag、layer 数量、media type 或归档布局属于跨构建端协议变更，必须同时修改：

- 普通包发布 workflow；
- `.github/workflows/build-sb.yaml`；
- `main.go`；
- `install.sh`；
- 根和本目录 `CLAUDE.md`。

## 代码布局

- `main.go`：OCI 访问、cache、解压、metadata、store、link、manifest sync、日志和 CLI。
- `main_test.go`：本地 registry 与临时 prefix 上的行为测试。
- `install.sh`：仅用于首次安装 `sb` 的 bootstrap。
- `go.mod`、`go.sum`：Go 1.26 module 与依赖锁定。

保持单文件实现，除非拆分能解决已经出现的职责或测试问题；不要为可能的未来 backend、registry
或 package graph 预造抽象。

## 本地状态

默认 prefix 是 `/opt/sb`：

```text
<prefix>/
├── bin/                       # 指向 store 的相对 symlink
├── lib/
├── share/
├── profile/<profile>/         # profile 聚合树
├── store/<package>/
│   └── .sb-meta
└── sb.log
```

每个包完整解压到 `store/<name>/`。`.sb-meta` 位于包目录内部，随包一起创建和删除，格式为
缩进 JSON，字段包括：

- `name`
- `arch`
- `digest`
- `linked`
- `installed_at`

根目录 `bin/`、`lib/`、`share/` 等按包内容建立相对 symlink；`.sb-meta` 永不参与 link。
profile 使用相同算法，但 link root 是 `<prefix>/profile/<name>/`。

下载 cache 位于 `${XDG_CACHE_HOME:-$HOME/.cache}/sb/<arch>/<name>.tar.gz`。cache 不是安装状态，
删除后不影响已装包。

## 安装状态机

`install <package>...` 分三阶段：

1. 并行 resolve 所有 tag 和 digest；任一缺失则在写入前终止。
2. 对需要更新的包并行下载 layer 到 cache。
3. 串行解压、写 metadata、移除旧链接并建立目标链接。

串行提交避免多个包并发修改相同 prefix 路径。digest 相同的包不重新下载，但 link 状态仍可调和。

`remove` 删除 metadata 标记的根链接，再删除 store 目录。`upgrade` 复用 install：

- 指定包名时升级指定包；
- 无参数时扫描所有 `.sb-meta`；
- 保留每包记录的 `arch` 与 `linked`。

## CLI 契约

全局选项：

- `--prefix PATH`：默认 `/opt/sb`；
- `--arch ARCH`：覆盖自动探测；
- `--verbose`：把详细日志同时写入 stderr。

命令：

- `install <package>...`：安装或刷新多个包；
  - `--link` 是 bool flag，默认 `true`，设为 `false` 时只进入 store；
  - `--force` 忽略 digest。
- `remove <package>`：删除一个包和其根链接。
- `upgrade [package...]`：升级指定包或全部包。
- `info <package>`：显示 metadata、registry 坐标和远端状态。
- `list`：列出已装包及 digest。
- `outdated`：列出远端 digest 已变化的包。
- `sync [file]`：应用 YAML manifest，默认 `sb.yaml`；
  - `--force` 强制重装；
  - `--prune` 删除 manifest 未引用的包。

store-only 安装的 CLI 表达固定为 `--link=false`。

## Manifest 契约

```yaml
prefix: /opt/sb
arch: linux-x86_64

packages:
  link:
    - ripgrep
  unlink:
    - python314

profiles:
  go:
    - gopls
    - delve
```

- `packages.link`：安装到 store 并链接到 prefix root。
- `packages.unlink`：只安装到 store。
- `profiles.<name>`：只安装到 store，并聚合到 `profile/<name>/`。
- manifest 的 `arch` 和 `prefix` 可省略。
- 显式 `--arch` 和 `--prefix` 优先于 manifest。
- logger 在 Cobra pre-run 初始化；manifest 改写 prefix 前产生的 log 仍位于 flag/default prefix。

sync 先把 link、unlink 和全部 profile 引用合并成一个去重 install plan。同一包被多处引用时，
root link 优先。所有包共享一次 resolve/download batch，随后建立 profile links。`--prune`
最后通过正常 remove 路径清理未引用包。

profile 和 link/unlink 拆分只属于 sync 层；其他命令不维护独立 profile 状态。

## 解压与链接安全

- tar entry path 必须留在目标 package root；`extractTarGz` 已检查这一点。
- 当前解压会原样创建归档中的 symlink，不验证 link target 是否逃出 package root。修改解压逻辑时
  必须补 symlink escape 测试，不能把现状描述为已完成完整 tar 安全校验。
- strip artifact 的第一个顶层目录，使内容落入 `store/<name>/`。
- `linkPkgInto` 当前对目标执行 `os.Remove` 后建立新链接，因此后安装的包会覆盖同路径 link，
  没有 ownership 数据。改变冲突语义时需同时设计 metadata 和 remove 行为。
- client 创建的 package/profile 聚合链接必须保持相对路径。
- `.sb-meta` 不得从远端归档继承，必须由 client 写入。

涉及解压或链接算法时，先补能复现边界的测试，再修改实现。

## Logging

每次调用把 slog text 写到 `<prefix>/sb.log`。终端显示关键步骤、下载进度和 summary；
`--verbose` 把详细日志镜像到 stderr。进度条走 stdout，避免与 verbose 日志冲突。

不要把日志文件当作状态源；命令行为只依赖 store 和 `.sb-meta`。

## Bootstrap Installer

`install.sh` 是唯一允许依赖宿主 `curl` 和 `tar` 的路径。它：

- 默认安装到 `~/.local/bin`；
- 接受 `SB_INSTALL_DIR`、`SB_ARCH`；
- 接受覆盖环境变量的 `--prefix`、`--arch`；
- 仅支持两个已发布平台；
- 安装成功后即使 help probe 失败，也只警告而不回滚已安装文件。

不要让 installer 依赖 Go、Nix、`oras`、`jq` 或非基础 shell 工具。

## 验证

在 `client/` 运行：

```bash
go test ./...
go vet ./...
go build ./...
```

涉及 installer 时追加：

```bash
bash -n install.sh
shellcheck install.sh
```

新增或改变公开行为时必须同步更新 `main_test.go` 和本文。重点覆盖：

- tar 路径与 symlink 安全；
- prefix relocate；
- metadata round-trip；
- 多包 resolve 失败不产生部分安装；
- digest 幂等与 force；
- sync 去重、root-link 优先、profile 和 prune；
- 两个平台自动探测；
- installer 参数与 artifact 布局。
