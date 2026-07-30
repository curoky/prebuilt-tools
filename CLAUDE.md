# standalone-binaries Agent Guide

本仓库用 Nix 构建并发布可移植工具。每个工具同时生成 standalone 目录和 tar.gz，
并以 OCI artifact 发布。本文只记录稳定约束、架构和改动入口。

修改具体生态前，再阅读[包构建策略](docs/package-strategies.md)。修改
`cmd/binman/` 时同时遵守 [`cmd/binman/CLAUDE.md`](cmd/binman/CLAUDE.md)。修改
`cmd/nixcache/` 时同时遵守 [`cmd/nixcache/CLAUDE.md`](cmd/nixcache/CLAUDE.md)。

## 产物不变量

1. **所有平台都不得运行期引用 `/nix/store`。** 文本路径、ELF rpath、Mach-O load command
   均不得残留 Nix store 依赖。
2. **Linux 常规 ELF 必须是 musl 全静态。**
3. **macOS 静态链接所有 Nix 依赖。** 只允许 `/usr/lib` 和
   `/System/Library/Frameworks` 中的系统动态库，不允许 `/nix/store` dylib。
4. **默认使用 unstable。** pin、patch、本地 packaging 和例外统一登记到
   [`TODO.md`](TODO.md)。
5. **每个包必须可搬运。** 资源使用相对路径；脚本工具通过同级 runtime wrapper
   绑定解释器。`bm` 不解析包依赖。

纯脚本、字体和数据包没有静态链接要求，但仍需移除 Nix store 路径。当前有两个明确记录的
Linux 动态例外：

- `music-decrypto`
- `nsight-systems`

新增或改变动态例外必须先确认，并同步本文、`TODO.md` 和
[特殊案例](docs/package-strategies/special-cases.md)。

已知宿主依赖：Darwin 的 `git-filter-repo`、`netron` 使用 `python3`，`eza-ls`
可回退 `/bin/ls`。不得为新包增加隐式宿主依赖。

## 支持平台

- `x86_64-linux`
- `aarch64-darwin`

`flake.nix` 为每个 nixpkgs input 创建普通 `pkgs` 和 `pkgsStatic` 环境。Linux 的静态环境使用
`pkgsCross.musl64.pkgsStatic`：target 是 musl-static，build 平台仍可复用缓存的 glibc
Rust/LLVM 工具链。Darwin 使用原生 `pkgsStatic`。

可选环境以 `flake.nix` 为准。非 unstable 选择必须登记到 `TODO.md`。

## 构建模型

构建流水线：

```text
manifests/default.nix ─┐
                       ├─> platform package set ─> makeArtifacts ─┬─> out
packages/local/ ───────┘                                         └─> archive
```

1. `manifests/default.nix` 声明可直接选择的 nixpkgs 包。
2. `packages/local.nix` 聚合 `packages/local/{common,linux,darwin}.nix` 中显式接入的
   patched、wrapped、pinned 和平台特定包。
3. `flake.nix` 按 `upstream // common // platform-specific` 合并，后者覆盖前者。
4. `lib/make-artifacts.nix` 为每个包创建一个 multi-output derivation；Go artifact tool
   在同一次构建中生成规范化目录 `out` 和确定性 tar.gz 文件 `archive`。
5. flake 在 `packages` 暴露各 derivation 的默认 `out` 及聚合输出 `all`、`all-fast`；
   在 `tarballs` 暴露同一 derivation 的 `archive` output。`tarballs` 与 `packages`
   平级，不参与 `discover` 的包枚举。

## 包接入

优先把可直接使用的 nixpkgs 包加入 `manifests/default.nix`。一级 key 是 nixpkgs
attribute path，支持：

| 字段 | 默认值 | 含义 |
| --- | --- | --- |
| `platforms` | 两个平台 | 启用的平台 |
| `version` | `"unstable"` | 选择的 nixpkgs 环境 |
| `isStatic` | `true` | 使用 `pkgsStatic`，否则使用普通 `pkgs` |
| `output` | `[ "out" ]` | 合并并公开的 derivation outputs |
| `alias` | 一级 key | flake output 名称 |
| `"<system>"` | `{ }` | 当前平台的字段覆盖 |

平台配置覆盖包级配置。解析实现以 `lib/make-manifest-packages.nix` 为准。

只有以下情况创建本地包：

- 修复上游静态编译或链接；
- 移除编译期写入的 Nix store 路径；
- 为证书、数据、插件或 helper 增加相对路径 wrapper；
- 组合脚本与同级 runtime；
- 做必要的结构性 repackaging。

`packages/local.nix` 聚合 `packages/local/{common,linux,darwin}.nix`。本地包必须通过
`callPackage` 显式接入，不自动扫描目录。实现选择按侵入性递增：

1. 直接使用 unstable `pkgsStatic`。
2. 对上游 derivation 做最小 override 或 patch。
3. 只替换造成问题的依赖为静态版本。
4. 关闭阻止静态链接的非必要 feature。
5. 增加相对资源 wrapper 或同级 runtime wrapper。
6. 经用户确认建立显式动态例外。

macOS 随包携带非系统 dylib 时，load command 必须使用 `@loader_path` 或 `@rpath`。
处理新包或静态构建失败时使用
`.trae/skills/patch-nixpkgs-standalone/SKILL.md`。回归本地 patch 或版本 pin 时，使用
`.trae/skills/regress-patched-package-to-upstream/SKILL.md`；批量回归只遍历
`TODO.md`。

## Artifact Assembly

`cmd/artifact/` 是所有包共享的后处理与归档实现，由 `lib/make-artifacts.nix` 调用。它：

- 删除发布无关内容、`.a` 和 `.pyc`；
- 保留内部 symlink，物化外部 symlink，删除 dangling symlink；
- 恢复 `.*-wrapped` 入口并改写文本 store path；
- strip ELF，并清除嵌入的 store hash；
- 校验 Linux ELF 静态性和 macOS Mach-O load commands；
- 只校验当前宿主平台的原生 binary format；
- 生成固定 metadata、固定顶层目录的确定性 tar.gz。

校验 fail-closed；无法解析原生 ELF/Mach-O 时构建失败。路径含 `openssl` 的文件保留既有
validation 豁免，但仍执行格式相关清理。artifact assembly 不修复链接方式或任意硬编码路径，
此类问题必须在包 derivation 中处理。

## 发布模型

普通 workflow 先通过本地 `nixcache serve` 检查当前 `outPath` 是否命中 cache，再构建
`packages` 和同一 derivation 的 `archive` output。standalone closure 发布到
`ghcr.io/curoky/standalone-binaries-cache`，tar.gz 发布到
`ghcr.io/curoky/standalone-binaries:<name>-<arch>`。

触发矩阵和 cache 判定见[包发布模型](docs/release-model.md)。Nix cache 协议见
[`cmd/nixcache/CLAUDE.md`](cmd/nixcache/CLAUDE.md)，client 归档协议见
[`cmd/binman/CLAUDE.md`](cmd/binman/CLAUDE.md)。修改 tag、layer 数量、顶层目录或
cache schema 时，必须同步相关 workflow 和对应目录级文档。

## 改动入口

| 需求 | 修改位置 |
| --- | --- |
| 添加可直接使用的 nixpkgs 包 | `manifests/default.nix` |
| 添加 patch、wrapper 或平台拆分 | `packages/<name>/`、`packages/local/<platform>.nix` |
| 登记或回归本地定制 | `TODO.md`、manifest、`packages/local/` |
| 修改 artifact assembly | `cmd/artifact/`、`lib/make-artifacts.nix`；同步下游协议 |
| 修改包选择或 outputs | `lib/`、`flake.nix` |
| 修改 client | `cmd/binman/`，并遵守 `cmd/binman/CLAUDE.md` |
| 修改 Nix cache | `cmd/nixcache/`，并遵守 `cmd/nixcache/CLAUDE.md` |

manifest 不显式填写默认字段。本地包只修 root cause。新增或改变 pin、patch、override、
禁用检查、结构性 packaging 或动态例外时同步维护 `TODO.md`。

## 验证

先验证目标包，再扩大范围：

```bash
nix flake check
CGO_ENABLED=0 go test ./cmd/artifact
CGO_ENABLED=0 go vet ./cmd/artifact
nix build .#<name>
nix build .#tarballs.<system>.<name>
file result/bin/*
nix build .#all-fast
```

根据包补充 `--version`、代表性 smoke test 和平台 binary inspection。wrapper、证书和
同级 runtime 不能只靠构建成功判断。修改 `cmd/` 时运行对应目录级文档指定的检查。

不把 eval、dry-run、lint 或代码审查表述为实际构建通过。完整构建成本过高时，明确报告已执行和
未执行的验证。

## Agent 文档规则

- 根 `CLAUDE.md` 只保存稳定架构、全局约束和验证入口。
- `TODO.md` 是 pin、patch、packaging 和例外的状态总表。
- `docs/package-strategies/` 只解释仍然有效的非显然设计，不复制 derivation 或回归历史。
- 目录级 `CLAUDE.md` 保存对应 CLI 的协议和专属约束。
- 代码参数与包清单以实现为准；仓库内链接使用相对路径。
