# Design

本文档描述本仓库的设计意图：它构建什么、构建流程如何串起来、以及在哪里做改动。请持续保持本文档准确：任何影响设计的改动都应在同一次提交里更新本文件。

## Purpose

本仓库产出一组精选的 **standalone、可移植** 工具二进制，经由 Nix 构建，并以「每个工具一个归档」的方式发布。

核心目标：

- 为最小化或陌生环境（容器、initramfs、类 scratch 镜像等）提供开箱即用的工具二进制。
- 让构建可复现、集中声明（单一 flake）。
- 降低运行期耦合：normalize 产物（strip、移除 Nix store 引用、删掉 docs/manpages、inline 外部 symlink），使二进制不依赖构建主机或 Nix store 布局。

### 最关键的预期：musl 纯静态、无 glibc、无 so 依赖

这是本仓库**最高优先级、不可妥协**的产物预期：

- **最终产物必须是 musl 纯静态编译的二进制或文本文件。**
- Linux 上的可执行文件**不能有 glibc 依赖**，也**不能有任何动态库（`.so`）依赖**——包括但不限于 `/nix` 下的动态库。理想结果是一个 `file` 显示 "statically linked"、`ldd` 显示 "not a dynamic executable" 的 musl 静态 ELF。
- 纯文本产物（脚本、字体、数据、纯 Perl/Python 源码等无编译对象的文件）豁免静态链接要求，它们只经过 `normalize.sh` 的 shebang/path 改写。凡是含 ELF/Mach-O 二进制的包都必须满足静态目标。

macOS 无法做到完全静态（没有静态 libSystem/libc），因此 darwin 的目标退一步为：**每个 nix 依赖都静态链接，只有 macOS 系统库（如** **`/usr/lib/libSystem.B.dylib`）可以保持动态**——详见下文 portability 硬规则。但 Linux 侧的 musl 纯静态是硬底线。

### 版本策略：一律用 unstable channel 最新版

**长期目标是所有包都使用官方维护的、`unstable`** **channel 的最新版本。**

- manifest 里 `version` 字段默认就是 `unstable`；新增包不写 `version` 即取 unstable。
- 任何被 pin 在老版本（如 `version = "25.11"` / `"24.11"`）的包都视为**临时状态**，应定期用 `regress-patched-package-to-upstream` skill 复查，尝试切回 unstable 最新版。
- 任何 `packages/` 下的本地 patch 包，只要是临时的编译修复（而非刻意的 repackaging），一旦上游修好就应切回上游、在 `manifests/default.nix` 里维护。

### Standalone strategy（preference order）

"Standalone" 意为可移植、自包含——**不**要求一定是单个完全静态的 ELF。决定如何构建一个工具时，按以下顺序优先：

1. **Static compilation**（`pkgsStatic`），凡是能静态编译就静态。这是绝大多数包的默认路线。
2. **手动 patch + bundle**，当完全静态链接不现实时：改写硬编码路径、vendor 配置、bundle 所需资源（见 `packages/`）。
3. **`nix bundle`** 仅作为最后手段，用于确实无法静态编译的工具（如基于 Node.js 的工具）。经由 `lib/make-bundle.nix`（matthewbauer/nix-bundle）实现；产出单个自解压可执行文件，**仅限 Linux**（依赖 user namespaces）。它的主要缺点是把整个 closure 塞进一个大文件、只暴露单一入口（`meta.mainProgram`），因此无法 ship 多个二进制。

#### Portability requirement（the hard rule）

Ship 出去的二进制必须可移植，且 **绝不能依赖任何** **`/nix`** **下的动态库**。

- **Linux：** 目标是 musl 完全静态（`pkgsStatic`），无 glibc、无 `.so`。
- **macOS：** 完全静态不可能（没有静态 libSystem/libc），目标是：**每个 nix 依赖都静态链接，只有 macOS 系统库（如** **`/usr/lib/libSystem.B.dylib`）可以保持动态。** 按以下阶梯处理：
  1. 能完全静态就完全静态（`pkgsStatic`）——有时需要小的上游 patch 才能让 darwin 上的静态链接成功（例如 `packages/krb5/darwin.nix` 构建完全静态的 `pkgsStatic.krb5`，但禁用 macOS CCAPI ccache 后端、并移动一处 DES const 定义，使 `libkrb5.a`/`libk5crypto.a` 的静态归档链接能 resolve；结果只依赖 `/usr/lib/libSystem`）。
  2. 否则，把其余每个依赖都静态链接，只让系统库保持动态。
  3. 复制 dylib（下文 dylib-bundle 变体）只在某个依赖无法静态链接时才用——**需要显式确认**。

如果构建后仍残留 `/nix/store` dylib，就修掉它（`CGO_ENABLED=0`、patch install names / rpaths，或——经确认后——复制 dylib）。

对于 strategy 2：当 darwin `pkgsStatic` 集合只会拖进一套独立的静态 toolchain（并没有真正的 static-libc 收益），而工具只有一两个非系统的动态依赖时，有一个更轻量的变体：从 **native** **`pkgs`** derivation 构建（上游缓存里已预编译，无需本地编 toolchain），只注入那个依赖的静态归档（如 `pkgs.perl.override { libxcrypt = pkgsStatic.libxcrypt; }`），再在包的 `postInstall` 里把残留的 `/nix/store` Mach-O install name 改写成 `@loader_path` 相对路径（`normalize.sh` 不处理 Mach-O load command）。这样结果只依赖 `/usr/lib` 系统库且可 relocate。`packages/perl` 在 darwin 上用这套，同时在 Linux 上经 `pkgsStatic` 保持完全静态。`packages/wget` 在两个平台都经 `pkgsStatic` 完全静态：Linux 直接从集合取（`./wget/linux.nix`），darwin 从 `pkgsStatic.wget` 取、仅把它 *build-time* 的 `perlPackages` override 成 native 集（`./wget/darwin-static.nix`）——darwin 的 `pkgsStatic.perl` 本身构建失败（它最后的 `mktables` 步骤会让刚编出来的静态 miniperl 崩溃，后者的 locale 支持大多被编译掉了），而 wget 只把 perl 当构建工具用。一个逐依赖替换的变体（改为构建 native `pkgs.wget`、再把每个非系统依赖换成其 `pkgsStatic` 归档）作为备选保留在 `./wget/darwin.nix`。

对于一个功能丰富、darwin `pkgsStatic` 构建仅因 *某些* 可选特性库无法静态构建/链接而失败的工具，另一个 strategy-2 变体是 **feature reduction**：从 `pkgsStatic.<tool>` derivation 起步，仅通过 `.override` 关掉那些惹事的特性，保留每个 `.a` 能干净链接的 codec/库。`packages/ffmpeg/darwin.nix` 在 `pkgsStatic.ffmpeg-headless` 之上就是这么做的：完整的 `pkgsStatic.ffmpeg` 在 aarch64-darwin 上无法构建（dav1d/opus 等的静态构建撞上 meson `arm64` cross-file bug；zimg/vid-stab/OpenCL 拖进 `openmp` -> `llvm-static`，后者在 CheckAtomic/libatomic 处失败；`libopenmpt` 拖进一个会被 SIGKILL 的 autogen 步骤），所以这些特性被关掉（`withDav1d`/`withOpus`/`withZimg`/... = false）。关掉所有 `openmp` 路径也让 `nix eval` 保持干净、无需 `config.problems` handler（openmp 是唯一会把 darwin 静态 `python3` 标记为 broken 的东西）。`withOpenapv` 关掉是因为 `liboapv` 即便在 `pkgsStatic` 下也只 ship 一个 `.dylib`（会留下 `/nix/store` load command）；network/TLS 库（gnutls/ssh/srt/rist）关掉是因为它们过不了 ffmpeg 的静态 configure 链接测试。`x265` 保留，但需要两处包修复（去掉它 `postInstall` 里的 `rm -f $out/lib/*.a`，在 `pkgsStatic`/`ENABLE_SHARED=false` 下它会删掉唯一产物 `libx265.a`；以及 `multibitdepthSupport = false` 以避免静态归档里出现未定义的 `x265_1{0,2}bit::` 符号——代价：只支持 8-bit HEVC 编码）。结果只依赖 `/usr/lib/*` 和 `/System/Library/Frameworks/*`。

对于无法静态链接、但能被多个工具复用的 runtime（如 Python 解释器），优先用 strategy 2 的 **shared-sibling wrapper** 变体而非 `nix bundle`：把这个重 runtime 作为独立包 ship（如 `packages/python/311`），再给每个工具一个薄 wrapper，运行期从共享 `$store` 父目录下同级的 sibling 解析出 runtime（如 `netron` -> `$store/python311`）。这让工具仍是普通的多文件包（需要时可暴露多个二进制），一份 runtime 被多个工具共享而非重复，并且照常走 standalone normalization。

Perl 工具遵循这套 sibling-wrapper 约定、对接共享的 `packages/perl`：工具的 bin 被 rename（如 `exiftool` -> `_exiftool`）并替换为一个 wrapper，用 `$store/perl/bin/perl` 运行它、`PERL5LIB` 指向工具自带的 `lib/perl5` 模块（`cloc`、`exiftool`）。处理 XS（编译型）Perl 依赖是 **platform-split** 的，因为两个 `packages/perl` 构建不同：Linux 的 `perl` 是完全静态（`pkgsStatic`，以 `-Uusedl` 构建），因此**运行期无法** **`dlopen`** **任何 XS** **`.so`**；而 darwin 的 `perl` 是 native、可以。所以 `packages/exiftool` 拆成 `linux.nix`（Linux）和 `darwin.nix`：

- **Linux（`packages/exiftool/linux.nix`）：** 可选的压缩 XS 模块必须作为静态扩展编译**进**静态解释器。上游 `pkgsStatic.perl` 已 vendor 了 `Compress::Raw::{Zlib,Bzip2}` 和 `IO::Compress::*` 系列；`packages/perl/linux.nix` 额外在 `cpan/` 下注入缺的两个 XS dist（`Compress::Raw::Lzma`、`IO::Compress::Brotli`）（配一个把 Lzma 指向静态 `xz` 的 `config.in`、以及一个链接静态 `brotli` 归档的最小 `Makefile.PL`，加上 MANIFEST 条目），让 perl 自己的构建 harness 把 `liblzma`/`libbrotli` 直接链进 `perl` 二进制。exiftool 随后只 ship 纯 Perl 部分（它的脚本、`Image::ExifTool`、纯 Perl 的 `Archive::Zip`）——完全没有 `.so`。
- **darwin（`packages/exiftool/darwin.nix`）：** native perl 能加载 `.bundle`，所以对每个 XS 模块套用 strategy 2——把模块的 nixpkgs 构建重新指向对应的 `pkgsStatic.<lib>` lib 目录（只 ship `.a`），使压缩库静态链接、产出的 `.bundle` 只依赖 `/usr/lib` 系统库（`Compress::Raw::{Zlib,Bzip2,Lzma}`、`IO::Compress::Brotli` -> 静态 `zlib`/`bzip2`/`xz`/`brotli`），从而避免任何 dylib 复制。

Non-goals：

- 不保证每个工具在每个平台上都是单个完全静态二进制。有些包故意非静态（如字体），"static" 是尽力而为、取决于上游。
- 不提供超出本仓库需要的通用打包框架。

## High-Level Architecture

构建 pipeline：

1. 通过 per-platform manifest 声明式选择上游包 derivation。
2. 合入本地定义的包（patched/wrapped/pinned 构建）与平台特定的补充。
3. 用一个 "standalone normalization" 步骤包裹每个 derivation，为其瘦身并移除 Nix 特定引用。
4. 把每个最终 derivation 暴露为 flake package 输出。
5. 在 CI 中，构建每个包、归档成 tar.gz，并用 `oras` 发布到 OCI registry。

flake 刻意做得很薄；逻辑在 `lib/`，包定义在 `manifests/` 和 `packages/`。

## Repository Layout

- [flake.nix](file:///workspace/standalone-binaries/flake.nix)：薄入口。声明 inputs、构建 per-system env、接好 helper、暴露 outputs。
- [lib/](file:///workspace/standalone-binaries/lib)：可复用构建 helper。
  - [make-manifest-packages.nix](file:///workspace/standalone-binaries/lib/make-manifest-packages.nix)：把 manifest attrset 转成一组上游 nixpkgs derivation。
  - [make-standalone.nix](file:///workspace/standalone-binaries/lib/make-standalone.nix)：用 normalization 步骤（运行 `scripts/normalize.sh`）包裹一个 derivation。
  - [make-bundle.nix](file:///workspace/standalone-binaries/lib/make-bundle.nix)：经 `nix bundle`（matthewbauer/nix-bundle）把一个 derivation 打成单个自解压可执行文件，用于无法静态编译的工具。仅限 Linux。bundle 产物跳过 standalone normalization。
- [manifests/](file:///workspace/standalone-binaries/manifests)：声明式选择上游 nixpkgs 包。
  - [default.nix](file:///workspace/standalone-binaries/manifests/default.nix)：以包名为 key 的单一 manifest；每个条目声明其目标 `platforms` 及可选的 per-platform 配置 override。
- [packages/](file:///workspace/standalone-binaries/packages)：本地定义的 derivation 与 override，**一个目录一个包**。
  - [local.nix](file:///workspace/standalone-binaries/packages/local.nix)：显式 manifest，经 `callPackage ./<pkg>` 把本地包聚合为 `{ common; linux; darwin; }`。
  - `packages/<pkg>/default.nix`：一个目录一个本地包；该目录也放这个包自己的资源（patch、wrapper 脚本、vendor 配置，如 `packages/podman/{bin,conf,*.patch}`、`packages/python/311/Setup.local`）。
  - 多版本应用归到单一应用目录下、一个版本一个子目录：`packages/cmake/{default,3_27_9,4_1_2}`、`packages/python/{311,312,313}`、`packages/clang-tools/{18,19,20,21,22}`、`packages/protobuf/{3_8_0,3_9_2}`。应用的默认/当前版本放在 `default/`。
  - `packages/protobuf/generic-v3.nix`：被各 protobuf 版本目录复用的共享 builder；共享 builder 不单独给版本子目录。
- [scripts/normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)：standalone wrapper 用的产物 normalization。
- CI workflows：
  - [build-linux.yaml](file:///workspace/standalone-binaries/.github/workflows/build-linux.yaml)
  - [build-darwin.yaml](file:///workspace/standalone-binaries/.github/workflows/build-darwin.yaml)
  - [build-llvm-tools.yaml](file:///workspace/standalone-binaries/.github/workflows/build-llvm-tools.yaml)：clang-tools / lld 的专用 builder（从主 Linux matrix 里排除）。
  - [build-sb.yaml](file:///workspace/standalone-binaries/.github/workflows/build-sb.yaml)：交叉编译并发布 Go 写的 `sb` client 自身（`sb-<arch>`）。

## Flake Outputs and Package Selection

### Systems

flake 目前为以下系统定义 output：

- `x86_64-linux`
- `aarch64-darwin`

### Per-system environments

`flake.nix` 为每个 pin 的 nixpkgs input（`unstable`、`26.05`、`25.11`、`25.05`、`24.11`、`24.05`）构建一个 "env"。每个 env 同时暴露 `pkgs` 和 `pkgsStatic`。manifest 决定一个包从哪个 env + 变体取。

### Package Sources

最终包集合合并三个来源：

1. **Upstream packages（manifest-driven）**——`lib/make-manifest-packages.nix` 应用到当前系统的 `manifests/default.nix`。每个 manifest 条目把包名映射到一个配置 attrset：
   - `platforms`：该包构建的目标系统列表（省略 => 所有系统）。
   - `version`：从哪个 nixpkgs env 导入（默认 `unstable`，也是长期目标值）。
   - `isStatic`：用 `pkgsStatic`（`true`，默认）还是普通 `pkgs`（`false`）。
   - `output`：要暴露的 derivation output 列表（默认 `[ "out" ]`，有时 `[ "bin" ]`），用 `symlinkJoin` 合并。
   - `alias`：重命名导出的 flake package。
   - `bundle`：用 `nix bundle` 把包打成单个自解压可执行文件、而非 normalize（仅 Linux，用于无法静态编译的工具）。bundle 包总是用普通 `pkgs`。
   - `"<system>"`：per-platform key，可 override 上面任意字段（有效配置 = 包级共享配置 `//` 平台 key，平台优先）。
2. **Local packages**——`packages/local.nix` 返回 `{ common; linux; darwin; }`：
   - `common`：跨平台的 pinned/patched/wrapped 构建（如特定 protobuf 版本、`coreutils`、vim/zsh/curl wrapper、`eza-ls`）。
   - `linux`：Linux-only 的 patched 工具（podman 栈、多版本 clang-tools、静态 Python 变体等）。
   - `darwin`：一小撮以 `CGO_ENABLED=0` 重建的 Go 工具（降低动态库依赖），外加 `nodejs-slim26`（一个基本静态的 Node.js 26——见下文）。
3. **Platform merge**——flake 按目标系统合并 `upstreamPackages // local.common // (local.linux | local.darwin)`。

### "all" Aggregation Output

除 per-package output 外，flake 还提供一个 `all` output，用 `linkFarm` 覆盖所有 standalone derivation。便于本地使用与检视。

## Standalone Normalization Pipeline

最终包集合里的每个 derivation 都被 `make-standalone.nix` 包裹，它：

- 把 derivation output 复制进一个全新的、可写的 `$out`。
- 在 output 树上运行 [scripts/normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)。

bundle 包（manifest 里 `bundle = true`）是例外：它们由 `make-bundle.nix` 产出为单个自解压可执行文件、完全跳过 normalization（strip / nuke-refs / shebang 改写会损坏归档）。

这个 wrapper 纯粹是后处理；它不改变包如何编译（static vs dynamic）。`normalize.sh` 里的 normalization 包括：

- 移除非必要目录（`share/man`、`share/doc`、`share/bash-completion`、`nix-support`）。
- 解析 symlink：保留指向 `$out` 内部的链接；对指向外部（如 `/nix/store`）的链接，inline（复制）其真实文件，使 payload 保持自包含；丢弃 dangling 链接。
- 文本文件：把硬编码 Nix store 路径的 shebang 改写为 `/usr/bin/env ...`；剥掉 Nix store 路径片段。
- ELF 二进制：`strip --strip-unneeded`（尽力而为）与 `nuke-refs`。
- rename `.*-wrapped` 可执行文件，去掉 `-wrapped` 后缀与前导 `.`。
- 移除 `.a` 与 `.pyc` 文件。
- **最终 portability check（the hard rule）：** 遍历每个 ELF（Linux）/ Mach-O（Darwin）文件，打印其动态依赖（Linux 用 `patchelf --print-needed` + `--print-rpath`，Darwin 用 `otool -L`——这些工具按平台加进 standalone wrapper 的 `nativeBuildInputs`，见 `make-standalone.nix`），若任一依赖（或 ELF rpath）resolve 到 `/nix` 下就**让构建失败**。这在构建期强制执行「ship 出去的二进制不得依赖任何 `/nix` 下的动态库」。`otool -L` 的第一行是文件自身路径（在 `/nix/store` output 目录下），匹配前会跳过。

设计意图：让运行期 payload 保持小巧、并移除对 Nix store 布局的隐式依赖。

## CI / Publishing Model

CI 模型是「独立构建每个工具、每个工具发布一个 artifact」。

### Build selection（Linux）

Linux workflow 用两阶段模型，避免每次改动都为每个包起一个 runner：

1. 一个 `discover` job 经 `nix eval .#packages.x86_64-linux` 枚举所有包名（排除 `all` 聚合），resolve 每个包的 `outPath`，并用 `nix path-info --store <cachix>` 查询 Cachix 二进制缓存。缓存里缺失的包组成一个 GitHub Actions matrix、作为 job output 发出。`EXCLUDE_PKGS`（由专用 workflow 构建）里的包被过滤掉。
2. `build` job 经 `fromJSON` 消费该 matrix、仅对选中的包运行。无需构建时 `build` job 被跳过（`if: needs.discover.outputs.count != '0'`）。

`workflow_dispatch` 带具体 `name` 只构建那个包；带 `*`（或空）强制构建所有包。`schedule` 总是强制构建所有包。

### Artifacts

Linux 与 Darwin workflow 都：

- CI job 运行 `nix build .#<name>`。
- `./result` output 经 `rsync --copy-unsafe-links` 复制进一个以包名命名的目录。
- 该目录归档为：
  - Linux 上 `<name>.linux-x86_64.tar.gz`
  - macOS 上 `<name>.darwin-arm64.tar.gz`

### Publishing

workflow 用 `oras push` 把 tarball 发布到 `ghcr.io`，tag 为：

- `ghcr.io/curoky/standalone-binaries:<name>-linux-x86_64`
- `ghcr.io/curoky/standalone-binaries:<name>-darwin-arm64`

flake 也配了一个 Cachix substituter；CI 把构建 closure push 到 Cachix 以加速后续构建。

## Client Install / Upgrade Model（`client/`）

[client/](file:///workspace/standalone-binaries/client) 是消费本仓库产物的 `sb` client（一个 brew/apt 风格的小型包管理器，Go 写的单个静态二进制，从 `ghcr.io` OCI registry 拉取已发布的 tarball 并安装）。它的设计是独立主题，详见 [client/CLAUDE.md](file:///workspace/standalone-binaries/client/CLAUDE.md)。

这是 client 侧的事，与上文的 CI/publishing 模型解耦——client 依赖的只是 `ghcr.io` 在 `oras push` 时已算好的 `<name>-<arch>` tag 与 layer digest。

## How to Make Changes

### Add a new upstream tool from nixpkgs

1. 在 [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix) 加一个条目：
   - 全平台包省略 `platforms`，或设 `platforms = [ "x86_64-linux" ]` / `[ "aarch64-darwin" ]` 限定。
   - 一个到处都有、但需要 per-system 不同配置的包，加一个 per-platform key，如 `aria2 = { "aarch64-darwin" = { version = "24.11"; }; };`。
   - **默认不写** **`version`（即取 unstable 最新版）**；只在上游最新版确实构建失败时才临时 pin 老版本，并按 `regress-patched-package-to-upstream` skill 定期复查、尽快切回 unstable。
2. 决定它该用 `pkgsStatic`（`isStatic = true`，默认）还是普通 `pkgs`（`isStatic = false`）。
3. 多 output 的包，用 `output = [ "bin" ]`（或列多个合并）挑对的 output。
4. nixpkgs attribute 名不好时，用 `alias` 导出更好的公开名。
5. 无法静态编译的工具，优先用 shared-sibling wrapper 而非 `nix bundle`：作为本地包 ship、其 JS/runtime 复用上游、wrap 成调用同级的静态 `nodejs-slim26` sibling（见下文 "local override" 里的 `pnpm`、`prettier`、`markdownlint-cli2`、`opencommit`）。仅当没有可复用的 sibling runtime 时，才把 `bundle = true`（配 `isStatic = false`、`platforms = [ "x86_64-linux" ]`）当真正的最后手段。

### Add a local override / patched build

1. 建目录 `packages/<pkg>/`，放一个 `default.nix`，外加该包所需的资源（patch、wrapper 脚本、vendor 配置）。
2. 经 `callPackage ./<pkg> { }` 接进 `packages/local.nix` 里合适的集合（`common`、`linux` 或 `darwin`）。`local.nix` 仍是本地包的显式 manifest（无自动发现）。
3. 遵循 standalone strategy：优先 static；否则 patch + bundle；仅当静态不可能时用 `nix bundle`。
4. 优先最小 diff：只 patch 为改善 portability、降低动态依赖、或修运行期路径所必需的部分。
5. **本地 patch 包只应是临时的编译修复。** 一旦上游修好，就按 `regress-patched-package-to-upstream` skill 切回上游、在 `manifests/default.nix` 里维护。刻意的 repackaging（wrapper、rename 二进制、bundle sibling 依赖，如 `packages/cloc`）不在此列。

Example——Node.js 工具（`packages/pnpm`、`packages/prettier`、`packages/markdownlint-cli2`、`packages/opencommit`）：不用 `nix bundle`，而是各自复用工具的 JS 分发（`lib/node_modules/<pkg>`）、把上游 bin wrapper 换成一个显式调用同级静态 node（`$store/nodejs-slim26/bin/node`）的相对路径脚本，使静态 node 随部署的工具一起走、而非在 standalone normalize 后依赖宿主 PATH 上的 node。`pnpm`/`prettier` 还额外通过 override 上游解释器**用静态 node 构建**（`pnpm` 只解 JS；`prettier` 用 pnpm 拉依赖），端到端地锻炼静态 runtime。`markdownlint-cli2`/`opencommit` 是基于 npm 的 `buildNpmPackage` 工具、构建需要 `npm`（`nodejs-slim` 里没有），所以用普通 node 构建、仅运行期切到同级静态 node。这套取代了 manifest 里这些工具之前的 `bundle = true` 条目。

Example——`eza-ls`（`packages/eza-ls`）：一个 `ls` 兼容前端，后端是 `eza`。上游 `eza` manifest 包保持不动；`eza-ls` 是一个单独的 `common` 包，ship 静态 `eza` 二进制外加一个 `bin/ls` bash wrapper（`packages/eza-ls/ls-wrapper.sh`）。wrapper 改编自 eggbean eza gist（与 `/opt/devspace/tools/eza-wrapper.sh` 同一脚本），改为调用同级的 `./eza`（使 `ls` 自包含）、并扩展映射了大多数常见 GNU `ls` 选项——短**和**长形式（`-la`、`--all`、`--long`、`--reverse`、`--recursive`、`--inode`、`--classify`、`--human-readable`、`--color[=WHEN]`、`--sort=WORD`、`--time-style=STYLE`、`--group-directories-first`、`--ignore=GLOB`、`-I`、`--opt=value` 和 `--opt value` 两种形式）——映射到带 ls 风格默认值（human sizes、git status、`--color-scale`）的 eza 等价项。eza 无法忠实表达的选项（如 `-Q`/`-C`/`-m`/`-w`、`--sort=version`、任何未知长选项）会透明地 `exec /bin/ls "$@"`，使任何受支持的真 `ls` 调用都不会报错；当自带 eza 不可用或 stdout 被 pipe 时也回退到 `/bin/ls`。兼容性由 `packages/eza-ls/tests/compat.bats`（经 `nix shell nixpkgs#bats -c bats ...` 运行）刻画，断言被接受的短/长选项、不可映射选项的 `/bin/ls` 回退、以及与 GNU `ls` 的 entry-set 一致性。

### Change normalization behavior

编辑 [scripts/normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)。把这个脚本当作兼容性表面对待：

- 改动 removal/rewrite 可能以微妙方式弄坏工具。
- 优先增量改动，并在有代表性的一批包上验证。

## Design Documentation Rules

- 本文件是本仓库的设计 source of truth。
- 任何影响 architecture、build flow、package selection model、artifact format 或 publishing model 的改动，必须在同一次改动里更新本文档。

