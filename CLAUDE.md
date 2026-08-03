# standalone-binaries Agent Guide

先阅读：

- 修改具体生态时阅读[包构建策略](docs/package-strategies/README.md)
- 修改 `cmd/artifact/`、`cmd/binman/` 或 `cmd/nixcache/` 时阅读对应目录的
  `CLAUDE.md`

本仓库将 nixpkgs 包和必要的本地定制转换为可搬运目录，再生成确定性 tar.gz。
包级 workaround 和回归状态以[回归清单](docs/regression/CLAUDE.md)为准。

## 产物不变量

1. 所有平台都不得运行时引用 `/nix/store`。
2. Linux 常规 ELF 必须使用 musl 全静态链接。
3. macOS 不得动态链接 Nix store 中的库；允许系统动态库和随包提供的相对路径 dylib。
4. 资源和 helper 使用相对路径；脚本工具通过同级 runtime wrapper 绑定解释器。
5. 纯脚本、字体和数据包没有静态链接要求，但仍不得保留 Nix store 路径。

Linux 当前只有 `music-decrypto` 和 `nsight-systems` 两个动态 ELF 例外。路径含
`openssl` 的文件保留既有 binary validation 豁免。改变这些例外前必须先确认。

Darwin 的 `git-filter-repo` 和 `netron` 使用宿主 `python3`，`eza-ls` 可回退
`/bin/ls`。这些是明确的产品边界，不应扩展为新包的默认策略。

macOS 系统动态库只允许来自 `/usr/lib` 和 `/System/Library/Frameworks`。随包携带
非系统 dylib 时，install name 和 load command 必须使用 `@loader_path` 或
`@rpath`，且 rpath 必须位于 `@loader_path` 下。

## 包集合

支持的平台是 `x86_64-linux`、`aarch64-linux` 和 `aarch64-darwin`。`flake.nix`
为每个 nixpkgs input 创建普通和静态 package set：

- Linux 使用 arch 对应的 musl cross static set：`x86_64-linux` 用
  `pkgsCross.musl64.pkgsStatic`，`aarch64-linux` 用
  `pkgsCross.aarch64-multiplatform-musl.pkgsStatic`。Target 是 musl-static，
  build platform 仍可复用 glibc Rust/LLVM 工具链。
- Darwin 使用原生 `pkgsStatic`。

包集合按 manifest、common local、platform local 的顺序合并，后者覆盖前者。
`manifests/default.nix` 选择可直接使用的 nixpkgs 包，完整 schema 见该文件及
`lib/make-manifest-packages.nix`。`packages/local.nix` 显式聚合本地包，不自动扫描
目录。

## 包接入

默认从 unstable 的 `pkgsStatic` 和 manifest 开始；失败后依次选择最小 override、
静态依赖替换、关闭非必要 feature、相对资源或同级 runtime wrapper。动态例外必须
单独确认。

- Manifest 不填写默认字段；包清单和 schema 以实现为准。
- Manifest schema 在 eval 时 fail-closed：未知字段、平台、nixpkgs version、空或重复
  output 和错误字段类型必须直接报错。
- 本地包只修 root cause，不复制 upstream derivation。
- 优先最小 override，避免 target overlay 污染 `buildPackages`。只针对静态 target
  的 override 必须检查 `stdenv.hostPlatform.isStatic`。
- 新增或改变 pin、patch、override、禁用检查、结构性 packaging、宿主依赖或动态
  例外时同步[回归清单](docs/regression/CLAUDE.md)。
- Wrapper、资源、证书和同级 runtime 要做行为 smoke test，不能只验证 derivation
  构建成功。

具体生态和特殊案例见[包构建策略](docs/package-strategies/README.md)。Artifact
规范化、校验和归档契约见
[`cmd/artifact/CLAUDE.md`](cmd/artifact/CLAUDE.md)。

## 发布边界

普通 workflow 构建同一 derivation 的 `out` 与 `archive` output。Standalone closure
进入 `ghcr.io/curoky/standalone-binaries-cache`，tar.gz 进入
`ghcr.io/curoky/standalone-binaries:<package>-<architecture>`。

`bm` 消费 tar.gz artifact，不解析包依赖；包与 runtime 必须分别安装。发布触发、
cache segment 和 retention 规则见[发布与 cache 模型](docs/release-model.md)。组件协议见
[`cmd/binman/CLAUDE.md`](cmd/binman/CLAUDE.md) 和
[`cmd/nixcache/CLAUDE.md`](cmd/nixcache/CLAUDE.md)。

修改 tag、layer、归档布局、cache schema 或 client 状态格式时，同步相关 workflow
和组件 `CLAUDE.md`。

## 改动入口

| 需求 | 修改位置 |
| --- | --- |
| 接入可直接使用的 nixpkgs 包 | `manifests/default.nix` |
| 添加 patch、wrapper 或平台拆分 | `packages/<name>/`、`packages/local/` |
| 登记或回归本地定制 | `docs/regression/`、manifest、`packages/local/` |
| 修改产物后处理与校验 | `cmd/artifact/CLAUDE.md`、`lib/make-artifacts.nix` |
| 修改包选择或 flake outputs | `lib/`、`flake.nix` |
| 修改 `bm` | `cmd/binman/CLAUDE.md` |
| 修改 Nix cache | `cmd/nixcache/CLAUDE.md`、`docs/release-model.md` |

处理静态构建失败时使用
[`patch-nixpkgs-standalone`](.trae/skills/patch-nixpkgs-standalone/SKILL.md)。
回归本地 patch 或版本 pin 时使用
[`regress-patched-package-to-upstream`](.trae/skills/regress-patched-package-to-upstream/SKILL.md)，
并以[回归清单](docs/regression/CLAUDE.md)为候选清单。

## 验证

先验证目标，再扩大范围：

```bash
nix build .#<name>
nix build .#tarballs.<system>.<name>
file result/bin/*
nix flake check
nix build .#all-fast
```

Go 组件的专属验证命令见各自 `CLAUDE.md`。Wrapper、证书和同级 runtime 不能只靠
build 成功判断；补充 `--version` 或代表性 smoke test。

不要把 eval、dry-run、lint 或代码审查表述为实际构建通过。完整构建成本过高时，明确
报告已执行和未执行的验证。

## 文档规则

- 跨模块设计保存在根 `CLAUDE.md`；独立组件的设计与协议放在组件目录的
  `CLAUDE.md`。
- `docs/` 只保留需要独立展开的发布模型、跨包构建策略和回归清单。
- 仓库内文档使用相对链接；实现参数和包清单以代码为准。
