# 架构与产物约束

本仓库将 nixpkgs 包和必要的本地定制统一转换为可搬运目录，再生成确定性 tar.gz
归档。本文记录跨实现文件的稳定设计；包级 workaround 和回归状态以
[`TODO.md`](../TODO.md) 为准。

## 产物不变量

1. 所有平台都不得运行时引用 `/nix/store`。
2. Linux 常规 ELF 必须使用 musl 全静态链接。
3. macOS 不得动态链接 Nix store 中的库；允许系统动态库和随包提供的相对路径 dylib。
4. 资源和 helper 使用相对路径；脚本工具通过同级 runtime wrapper 绑定解释器。
5. 纯脚本、字体和数据包没有静态链接要求，但仍不得保留 Nix store 路径。

Linux 当前只有 `music-decrypto` 和 `nsight-systems` 两个动态 ELF 例外。

Darwin 的 `git-filter-repo` 和 `netron` 使用宿主 `python3`，`eza-ls` 可回退
`/bin/ls`。这些是明确的产品边界，不应扩展为新包的默认策略。

## 包集合

支持的平台是 `x86_64-linux` 和 `aarch64-darwin`。`flake.nix` 为每个 nixpkgs
input 创建普通和静态 package set：

- Linux 使用 `pkgsCross.musl64.pkgsStatic`。target 是 musl-static，build
  platform 仍可复用 glibc Rust/LLVM 工具链。
- Darwin 使用原生 `pkgsStatic`。

包集合按 manifest、common local、platform local 的顺序合并，后者覆盖前者。
`manifests/default.nix` 选择可直接使用的 nixpkgs 包，完整 schema 见该文件及
`lib/make-manifest-packages.nix`。`packages/local.nix` 显式聚合本地包，不自动扫描
目录。

## Artifact Assembly

`lib/make-artifacts.nix` 调用 `cmd/artifact/`，在同一个 multi-output derivation
中生成 standalone 目录和归档。它裁剪无关内容，规范化 symlink、wrapper、权限与
metadata，清除 store reference，校验宿主原生 ELF 或 Mach-O，再生成确定性归档。

校验 fail-closed：无法解析原生 ELF 或 Mach-O 时构建失败。路径含 `openssl`
的文件保留既有 binary validation 豁免，但仍执行格式相关清理。

Artifact assembly 只做规范化与约束校验，不会把动态程序变成静态程序，也不会推断
任意硬编码资源路径。此类问题必须在包 derivation 中解决。

## 包接入原则

默认从 unstable 的 `pkgsStatic` 和 manifest 开始；失败后依次选择最小 override、
静态依赖替换、关闭非必要 feature、相对资源或同级 runtime wrapper。动态例外必须
单独确认。

macOS 系统动态库只允许来自 `/usr/lib` 和 `/System/Library/Frameworks`。随包携带
非系统 dylib 时，install name 和 load command 必须使用 `@loader_path` 或
`@rpath`，且 rpath 必须位于 `@loader_path` 下。生态和特殊案例见
[包构建策略](package-strategies/README.md)。

## 发布边界

普通 workflow 构建同一 derivation 的 `out` 与 `archive` output。Standalone closure
进入 `ghcr.io/curoky/standalone-binaries-cache`，tar.gz 进入
`ghcr.io/curoky/standalone-binaries:<package>-<architecture>`。

`bm` 消费 tar.gz artifact，不解析包依赖；包与 runtime 必须分别安装。发布触发、
cache segment 和 retention 规则见[发布与 cache 模型](release-model.md)，client
状态和事务规则见 [`bm` 设计](binman.md)。
