# standalone-binaries Agent Guide

本仓库用 Nix 构建并发布一组可移植工具。每个工具独立构建为目录树、压缩为一个 tarball，再以
OCI artifact 发布。本文是构建端设计、约束和改动流程的 agent source of truth。

修改具体生态前，再阅读[包构建策略](docs/package-strategies.md)及对应 case study。修改
`client/` 时同时遵守 [`client/CLAUDE.md`](client/CLAUDE.md)。

## 产物不变量

按优先级执行：

1. **所有平台都不得运行期引用 `/nix/store`。** 文本路径、ELF rpath、Mach-O load command
   均不得残留 Nix store 依赖。
2. **Linux 常规二进制必须是 musl 全静态。** `file` 应显示 statically linked，`ldd`
   应报告 not a dynamic executable；不得依赖 glibc 或任何 `.so`。
3. **macOS 静态链接所有 Nix 依赖。** 只允许 `/usr/lib` 和
   `/System/Library/Frameworks` 中的系统动态库，不允许 `/nix/store` dylib。
4. **默认使用 unstable 上游最新版。** manifest 的 `version` pin 和本地编译 patch
   都是待定期回归的临时状态；所有 pin、patch 和本地 packaging 必须登记到
   [`TODO.md`](TODO.md) 总表。
5. **每个包必须可搬运。** 不依赖宿主包管理器；需要 runtime 的脚本工具优先通过同级
   runtime wrapper 解决。`sb` 不做包依赖解析。

纯脚本、字体和数据包没有静态链接要求，但仍需移除 Nix store 路径。当前有两个明确记录的
Linux 动态例外：

- `music-decrypto`：glibc 动态 .NET AOT 产物；
- `nsight-systems`：NVIDIA 预编译 glibc 动态产物。

这两个现状例外不得作为新增动态包的先例。新增或改变例外必须先明确告知用户，并同步修改本文和
[特殊案例](docs/package-strategies/special-cases.md)。

已知宿主工具依赖也必须显式记录：Darwin 的 `git-filter-repo`、`netron` 使用宿主 `python3`，
`eza-ls` 对不支持的参数回退 `/bin/ls`。这些是现状缺口，不得类推给新包。

## 支持平台

- `x86_64-linux`
- `aarch64-darwin`

`flake.nix` 为每个 nixpkgs input 创建普通 `pkgs` 和 `pkgsStatic` 环境。Linux 的静态环境使用
`pkgsCross.musl64.pkgsStatic`：target 是 musl-static，build 平台仍可复用缓存的 glibc
Rust/LLVM 工具链。Darwin 使用原生 `pkgsStatic`。

可选环境为 `unstable`、`26.05`、`25.11`、`25.05`、`24.11`、`24.05`。只在
unstable 当前无法构建时才选择旧环境，并在代码附近记录具体原因。

## 构建模型

构建流水线：

```text
manifests/default.nix ─┐
                       ├─> platform package set ─> makeStandalone ─> flake outputs
packages/local.nix ────┘                                  │
                                                         └─> normalize.sh
```

1. `manifests/default.nix` 声明可直接选择的 nixpkgs 包。
2. `packages/local.nix` 显式聚合 patched、wrapped、pinned 和平台特定包。
3. `flake.nix` 按 `upstream // common // platform-specific` 合并，后者覆盖前者。
4. `lib/make-standalone.nix` 复制 derivation output，并运行 `scripts/normalize.sh`。
5. flake 暴露每个独立包，以及聚合输出 `all` 和跳过慢速 LLVM 包的 `all-fast`。

manifest 中 `bundle = true` 的包改走 `lib/make-bundle.nix`，使用 `nix-bundle` 生成
Linux-only 自解压可执行文件并跳过 normalization。它依赖 user namespace，是最后手段。

## Manifest Schema

`manifests/default.nix` 的一级 key 是 nixpkgs attribute path。支持：

| 字段 | 默认值 | 含义 |
| --- | --- | --- |
| `platforms` | 两个平台 | 启用的平台 |
| `version` | `"unstable"` | 选择的 nixpkgs 环境 |
| `isStatic` | `true` | 使用 `pkgsStatic`，否则使用普通 `pkgs` |
| `output` | `[ "out" ]` | 合并并公开的 derivation outputs |
| `alias` | 一级 key | flake output 名称 |
| `bundle` | `false` | 使用 Linux `nix-bundle` |
| `"<system>"` | `{ }` | 当前平台的字段覆盖 |

平台配置覆盖包级配置。解析实现以 `lib/make-manifest-packages.nix` 为准。

## Local Packages

`packages/local.nix` 返回：

```nix
{
  common = { ... };
  linux = { ... };
  darwin = { ... };
}
```

本地包通过 `callPackage` 显式接入，不自动扫描目录。通常一个工具一个目录；同一应用的多版本放在
一个父目录下，如 `packages/python/{311,312,313,314,315}`。

只有以下情况应创建本地包：

- 修复上游静态编译或链接；
- 移除编译期写入的 Nix store 路径；
- 为证书、数据、插件或 helper 增加相对路径 wrapper；
- 组合脚本与同级 runtime；
- 对上游分发做必要的结构性 repackaging。

## Standalone 策略

按侵入性从低到高选择，能停在前一级就不要继续：

1. 直接使用 unstable `pkgsStatic`。
2. 对上游 derivation 做最小 override 或 patch。
3. 只替换造成问题的依赖为静态版本。
4. 关闭阻止静态链接的非必要 feature。
5. 增加相对资源 wrapper 或同级 runtime wrapper。
6. Linux 使用 `nix bundle`。
7. 经用户确认建立显式动态例外。

macOS 不得静默复制 `/nix/store` dylib。确需 bundle 非系统 dylib 时，先确认设计变化，并把
Mach-O load command 改为 `@loader_path` 或 `@rpath` 相对路径。

处理新包或静态构建失败时，使用
`.trae/skills/patch-nixpkgs-standalone/SKILL.md`。回归本地 patch 或版本 pin 时，使用
`.trae/skills/regress-patched-package-to-upstream/SKILL.md`；批量回归只遍历
[`TODO.md`](TODO.md)，不重新扫描仓库猜测候选。

## Normalization

`scripts/normalize.sh` 是所有非 bundle 包共享的后处理层。它：

- 删除 `nix-support`、man page、文档和 bash completion；
- 保留 output 内部 symlink，展开外部 symlink，删除 dangling symlink；
- 改写文本 shebang 和可识别的 Nix store path；
- 对 ELF strip 并运行 `nuke-refs`；
- 恢复 `.*-wrapped` 的公开文件名；
- 删除 `.a` 和 `.pyc`；
- 检查 ELF rpath 与 Mach-O 动态依赖，发现 `/nix` 引用就失败。

Normalization 不会把动态程序变成静态程序，也不能可靠修复任意二进制硬编码路径。此类问题必须在
derivation 或 wrapper 中解决。修改 normalization 会影响几乎全部包，应优先考虑包级修复。

## 发布模型

普通 Linux 和 Darwin workflow：

1. 一次 eval 当前平台的包名与 `outPath`；
2. push 时只选择 Cachix 缺失包，定时和手工全量模式选择全部包；
3. 执行 `nix build .#<name>`；
4. 复制 output 并生成 `<name>.linux-x86_64.tar.gz` 或
   `<name>.darwin-arm64.tar.gz`；
5. 发布到 `ghcr.io/curoky/standalone-binaries:<name>-<arch>`。

LLVM 工具由专用 workflow 构建。`sb` client 以 `sb-<arch>` 发布。每个 artifact 当前只有一个
tar.gz layer，client 使用 layer digest 判断升级。修改 tag、layer 数量或归档布局时，必须同步修改
所有 build workflow、`client/main.go`、`client/install.sh` 和 `client/CLAUDE.md`。

## 改动入口

| 需求 | 修改位置 |
| --- | --- |
| 添加可直接使用的 nixpkgs 包 | `manifests/default.nix` |
| 添加 patch、wrapper 或平台拆分 | `packages/<name>/`、`packages/local.nix` |
| 登记 pin、patch、packaging 与回归状态 | `TODO.md` |
| 回归版本 pin | `manifests/default.nix` |
| 回归本地 patch | manifest、`packages/local.nix`，并删除孤儿目录 |
| 修改通用后处理 | `scripts/normalize.sh` |
| 修改包选择或 outputs | `lib/`、`flake.nix` |
| 修改 client | `client/`，并遵守 `client/CLAUDE.md` |

添加上游包时不要显式填写默认字段。添加本地包前先区分编译、链接、硬编码路径和资源定位问题，
只修 root cause。上游修复可用后，一次性删除本地 patch、pin、聚合条目、无用资源和过时文档，
不保留兼容路径。新增或改变非 unstable pin、本地 derivation、override、禁用检查或动态例外时，
同步维护 `TODO.md`；结构性 wrapper、资源打包和产品行为进入总表，但标记为不可整包回归。

## 验证

先验证目标包，再扩大范围：

```bash
nix flake check
nix build .#<name>
file result/bin/*
ldd result/bin/<binary>       # Linux
otool -L result/bin/<binary>  # macOS
nix build .#all-fast
```

根据包补充 `--version` 或代表性命令 smoke test。wrapper、证书和同级 runtime 不能只靠构建成功判断。
修改 `client/` 时运行其 `CLAUDE.md` 指定的 Go 与 shell 检查。

不把 eval、dry-run、lint 或代码审查表述为实际构建通过。完整构建成本过高时，明确报告已执行和
未执行的验证。

## Agent 文档规则

- 本仓库不维护面向人的 README；agent 入口只有根和目录级 `CLAUDE.md`。
- 根 `CLAUDE.md` 保存稳定架构、全局约束、改动入口和验证要求。
- `client/CLAUDE.md` 保存 client 的公开契约、状态模型和专属约束。
- `TODO.md` 是人和 agent 共用的 pin、patch 与本地 packaging 总表；批量回归只消费其中标为
  `✅` 或 `🟡` 的行。
- `docs/package-strategies*.md` 只记录偏离默认构建路径的技术案例和诊断记录。
- 代码参数与当前包清单以实现为准；文档解释决策和不变量，不复制完整 derivation。
- 仓库内链接使用相对路径，不使用 `file://` URL、工作区绝对路径或易漂移的行号链接。
- 设计、协议、schema、例外或工作流发生变化时，在同一次改动中更新相应 `CLAUDE.md`。
