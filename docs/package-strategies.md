# Package Strategies（分生态 case study 索引）

本文档是**按语言生态/runtime 分类**的 per-package 构建策略索引。设计总览与策略抽象见根目录
[CLAUDE.md](file:///workspace/standalone-binaries/CLAUDE.md)；这里只讲「某一类包为什么这么打、在
Linux 与 macOS 两平台分别怎么做」。

这些是 **case study**，不是穷举清单。source of truth 永远是 `packages/<pkg>/` 下的代码与注释；
本文档过时或与代码冲突时以代码为准。

## 怎么用这份文档

先按包的「构建来源」定位到对应生态文档，再看具体包的两平台策略：

| 生态 / 构建来源 | 文档 | 覆盖的典型包 |
| --- | --- | --- |
| Python 解释器 + Python 脚本工具 | [python.md](file:///workspace/standalone-binaries/docs/package-strategies/python.md) | `python311..315`、`git-filter-repo`、`netron`、`dool` |
| Node.js runtime + Node CLI 工具 | [nodejs.md](file:///workspace/standalone-binaries/docs/package-strategies/nodejs.md) | `nodejs-slim24/26`、`pnpm`、`prettier`、`markdownlint-cli2`、`opencommit` |
| Perl 解释器（platform-split）+ Perl 脚本工具 | [perl.md](file:///workspace/standalone-binaries/docs/package-strategies/perl.md) | `perl`、`cloc`、`parallel`、`exiftool` |
| Go 编译（含 CGO 关闭） | [go.md](file:///workspace/standalone-binaries/docs/package-strategies/go.md) | `podman`、macOS 上一批 Go 工具 |
| Rust 编译 | [rust.md](file:///workspace/standalone-binaries/docs/package-strategies/rust.md) | `zellij` |
| C / autotools（stdenv）+ s6 stack + clang-tools | [c-autotools.md](file:///workspace/standalone-binaries/docs/package-strategies/c-autotools.md) | `curl`、`git`、`vim`、`zsh`、`cmake`、`s6*`、`clang-tools-*` 等 |
| 基础工具特例（feature reduction / 工具链修复 / prebuilt） | [special-cases.md](file:///workspace/standalone-binaries/docs/package-strategies/special-cases.md) | `ffmpeg`、`postgresql`、`krb5`、`gnutar`、`nsight-systems`、`music-decrypto` |

包分类的骨架 source of truth 是 [packages/local.nix](file:///workspace/standalone-binaries/packages/local.nix)（注释已按生态分好类），
manifest 里的上游包见 [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix)。

## 两平台通用策略（回顾）

具体分级与硬底线的完整定义在 [CLAUDE.md](file:///workspace/standalone-binaries/CLAUDE.md) 的 Prime Directives /
Standalone Strategy 节，这里只做一页速查，供各生态文档引用。

### Standalone strategy 分级

按侵入性从低到高，**能停在早一级就不要用后一级**：

1. **Static compilation**（`pkgsStatic`）——默认。
2. **手动 patch + bundle**——改写硬编码路径、vendor 配置、bundle 资源。
   - 2a. **Selective static override**：从更轻的 base 起，只注入所需静态归档。
   - 2b. **Feature reduction**：从 `pkgsStatic.<tool>` 起，只关掉惹事的可选特性。
   - 2c. **Shared-sibling wrapper**：重 runtime 独立成包，工具用薄 wrapper 运行期解析同级 sibling。
3. **`nix bundle`**——最后手段，仅 Linux。

### Linux 硬底线

含二进制的包最终必须是 **musl 纯静态 ELF**：`file` 显示 "statically linked"、`ldd` 显示 "not a
dynamic executable"，无 glibc 依赖、无任何 `.so` 依赖（尤其不能依赖 `/nix` 下动态库）。Linux 上
`pkgsStatic` 实为 **musl64 cross 静态集**（见 [flake.nix](file:///workspace/standalone-binaries/flake.nix) 的 `mkEnv`）。

### macOS static ladder

darwin 无法完全静态（没有静态 libSystem/libc）。目标：每个 nix 内部依赖静态链接，只有 `/usr/lib/*`、
`/System/Library/Frameworks/*` 可动态；**零** `/nix/store` dylib。ladder：

1. 先修 `pkgsStatic.<x>`（小上游 patch 让静态归档在 darwin 链接成功）。
2. 无解时 fall back 到 native `pkgs.<x>`，把每个非系统动态依赖换成 `pkgsStatic.<dep>` 静态归档。
3. 仍残留 `/nix/store` dylib：**先与用户确认**，再用 `install_name_tool` 改写成 `@loader_path` 相对。
4. 复制 dylib 进包（dylib-bundle）——绝对最后手段，**需用户显式确认**。

> macOS Mach-O 提醒：`normalize.sh` 处理 ELF（`nuke-refs`/strip）但**不**动 Mach-O load command。
> 任何在包里留下非系统 dylib 的 darwin 路线，必须在 `postInstall` 里用 `install_name_tool`
> 把 id / consumer 的 load command 改写成 `@loader_path` 相对。

### 两种反复出现的可移植性手法

跨生态高频复用、各文档会反复引用的两个 pattern：

- **Shared-sibling wrapper**：无法静态链接、但能被多工具复用的 runtime（Python/Perl 解释器、Node.js
  runtime），独立成包 ship（如 `packages/python/314`、`packages/nodejs/26`），工具用薄 bash wrapper
  经 `readlink -f "$0"` 定位自身、求出同级 sibling 目录（`$store/<runtime>`），运行期显式 `exec` 那个
  runtime 跑主脚本，并显式设 `PYTHONHOME`/`PYTHONPATH`/`PERL5LIB`——从而绕开被 normalize 改写的
  nix-store shebang，实现可移植。
- **Rename + relative-path wrapper**：静态二进制运行期不知道自己被部署到哪，凡是需要 baked 数据路径的
  工具（`file` 的 magic.mgc、`vim` 的 VIMRUNTIME、`curl`/`wget` 的 CA bundle、`git` 的 exec-path）都把
  真实二进制 rename 成 `_<name>`、放一个 bash wrapper 当 `<name>`，wrapper 用 `$root` 相对解析这些数据
  再 exec 真身。
