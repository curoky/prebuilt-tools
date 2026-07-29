# Design

本仓库产出一组精选的 **standalone、可移植** 工具二进制，经 Nix 构建、以「每个工具一个归档」
发布。本文件是设计 source of truth，也是 AI/贡献者的索引：先读它把握全局与「去哪儿改」，
细节靠链接下钻。任何影响设计的改动都应在同一次提交里更新本文件。

- 具体包的构建 case study（按 Python / Node.js / Perl / Go-Rust / C-autotools / 特例分生态拆分）：[docs/package-strategies.md](file:///workspace/standalone-binaries/docs/package-strategies.md)
- `sb` client（消费端）设计：[client/CLAUDE.md](file:///workspace/standalone-binaries/client/CLAUDE.md)

## Prime Directives（不可妥协的产物预期）

按优先级排列，一切设计取舍服从于此：

1. **musl 纯静态、无 glibc、无 so 依赖（Linux 硬底线）。** Linux 上含二进制的包，最终产物必须是
   musl 纯静态 ELF——`file` 显示 "statically linked"、`ldd` 显示 "not a dynamic executable"，
   **不能有 glibc 依赖，也不能有任何 `.so` 依赖**（尤其不能依赖 `/nix` 下的动态库）。
2. **macOS 尽可能静态。** darwin 无法完全静态（没有静态 libSystem/libc），退一步为：**每个 nix
   依赖都静态链接，只有 macOS 系统库（`/usr/lib/*`、`/System/Library/Frameworks/*`）可保持动态；
   零 `/nix/store` dylib。**
3. **一律用 unstable channel 最新版。** 长期目标是所有包都用官方 `unstable` channel 最新版。任何
   老版本 `version` pin 或本地 patch 都视为**临时状态**，定期用
   [regress-patched-package-to-upstream](file:///workspace/standalone-binaries/.trae/skills/regress-patched-package-to-upstream/SKILL.md)
   skill 复查、尽快切回上游。

> 例外：纯文本产物（脚本、字体、数据、无编译对象的纯 Perl/Python 源码）豁免静态链接，只走
> `normalize.sh` 的 shebang/path 改写。凡含 ELF/Mach-O 二进制的包都必须满足 1/2。

Non-goals：不保证每个工具在每个平台都是单个完全静态二进制（有些包故意非静态，如字体）；
不提供超出本仓库需要的通用打包框架。

## Standalone Strategy（preference order）

"Standalone" 意为可移植、自包含，**不**要求一定是单个完全静态 ELF。选构建方式时按侵入性从低到高，
**能停在早一级就不要用后一级**（各级的 per-package 例子见 [package-strategies.md](file:///workspace/standalone-binaries/docs/package-strategies.md)）：

1. **Static compilation**（`pkgsStatic`）——默认，对多数工具有效。
2. **手动 patch + bundle**——静态不现实时：selective static override（只注入所需静态归档）、
   feature reduction（关掉惹事的可选特性）、shared-sibling wrapper（重 runtime 独立成包、运行期
   解析同级 sibling）。
3. **`nix bundle`**（`lib/make-bundle.nix`，matthewbauer/nix-bundle）——最后手段，仅 Linux（依赖
   user namespaces），产单个自解压可执行文件、只能暴露单一入口，跳过 normalization。

macOS 的静态化 ladder（先修 `pkgsStatic` → native + 单依赖静态替换 → 经确认改写/复制 dylib）
详见 [package-strategies.md](file:///workspace/standalone-binaries/docs/package-strategies.md)。**在 macOS 上绝不未经用户显式确认就复制 `/nix` dylib。**

## Architecture

构建 pipeline：

1. 经 per-platform manifest 声明式选择上游包 derivation。
2. 合入本地定义的包（patched/wrapped/pinned 构建）与平台特定补充。
3. 用 standalone normalization 步骤包裹每个 derivation，瘦身并移除 Nix 引用。
4. 把每个最终 derivation 暴露为 flake package output。
5. CI 中构建每个包、归档成 tar.gz、用 `oras` 发布到 OCI registry。

flake 刻意做得很薄；逻辑在 `lib/`，包定义在 `manifests/` 和 `packages/`。

Systems：`x86_64-linux`、`aarch64-darwin`。`flake.nix` 为每个 pin 的 nixpkgs input
（`unstable`/`26.05`/`25.11`/`25.05`/`24.11`/`24.05`）构建一个 env，各暴露 `pkgs` 与 `pkgsStatic`；
manifest 决定一个包从哪个 env + 变体取。

## Repository Layout

- [flake.nix](file:///workspace/standalone-binaries/flake.nix)：薄入口。声明 inputs、构建 per-system env、接 helper、暴露 outputs。
- [lib/](file:///workspace/standalone-binaries/lib)：可复用构建 helper。
  - [make-manifest-packages.nix](file:///workspace/standalone-binaries/lib/make-manifest-packages.nix)：把 manifest attrset 转成上游 nixpkgs derivation。
  - [make-standalone.nix](file:///workspace/standalone-binaries/lib/make-standalone.nix)：用 normalization（运行 `normalize.sh`）包裹 derivation。
  - [make-bundle.nix](file:///workspace/standalone-binaries/lib/make-bundle.nix)：经 `nix bundle` 打成单个自解压可执行文件（仅 Linux，无法静态编译的工具用）。
- [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix)：以包名为 key 的单一 manifest；每条声明目标 `platforms` 与可选 per-platform override。
- [packages/](file:///workspace/standalone-binaries/packages)：本地 derivation 与 override，**一个目录一个包**（patch/wrapper/vendor 配置就近放）。
  - [local.nix](file:///workspace/standalone-binaries/packages/local.nix)：显式 manifest，经 `callPackage ./<pkg>` 聚合为 `{ common; linux; darwin; }`（无自动发现）。
  - 多版本应用归到单一应用目录、一版本一子目录（如 `packages/python/{311,312,313}`、`packages/clang-tools/{18..22}`），默认版放 `default/`；共享 builder（如 `protobuf/generic-v3.nix`）不单独给版本子目录。
- [scripts/normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)：standalone wrapper 的产物 normalization。
- CI workflows：[build-linux.yaml](file:///workspace/standalone-binaries/.github/workflows/build-linux.yaml)、[build-darwin.yaml](file:///workspace/standalone-binaries/.github/workflows/build-darwin.yaml)、[build-llvm-tools.yaml](file:///workspace/standalone-binaries/.github/workflows/build-llvm-tools.yaml)（clang-tools/lld 专用）、[build-sb.yaml](file:///workspace/standalone-binaries/.github/workflows/build-sb.yaml)（发布 `sb` client）。

## Package Selection Model

最终包集合合并三个来源：`upstreamPackages // local.common // (local.linux | local.darwin)`（按目标系统）。

**1. Upstream（manifest-driven）**——`make-manifest-packages.nix` 应用到当前系统的 `manifests/default.nix`。
每条把包名映射到配置 attrset：

- `platforms`：目标系统列表（省略 => 所有系统）。
- `version`：从哪个 nixpkgs env 导入（**默认 `unstable`，也是长期目标值**）。
- `isStatic`：用 `pkgsStatic`（`true`，默认）还是普通 `pkgs`（`false`）。
- `output`：暴露的 derivation output（默认 `[ "out" ]`，有时 `[ "bin" ]`），`symlinkJoin` 合并。
- `alias`：重命名导出的 flake package。
- `bundle`：用 `nix bundle` 而非 normalize（仅 Linux，总是用普通 `pkgs`）。
- `"<system>"`：per-platform key，override 上面任意字段（有效配置 = 包级 `//` 平台 key，平台优先）。

**2. Local**——`packages/local.nix` 返回 `{ common; linux; darwin; }`：`common` 跨平台
patched/wrapped 构建；`linux` Linux-only patched 工具；`darwin` 一撮 `CGO_ENABLED=0` 重建的 Go 工具
外加 `nodejs-slim26`。

**3. `all` output**——用 `linkFarm` 覆盖所有 standalone derivation，便于本地检视。

## Standalone Normalization

每个最终 derivation 被 [make-standalone.nix](file:///workspace/standalone-binaries/lib/make-standalone.nix) 包裹：把 output 复制进全新可写 `$out`、在其上运行
[normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)。这纯是后处理，不改变包如何编译（static vs dynamic）。bundle 包（`bundle = true`）是例外，
完全跳过 normalization。

`normalize.sh` 做：移除非必要目录（`share/man`、`share/doc`、`share/bash-completion`、`nix-support`）；
解析 symlink（保留指向 `$out` 内的，inline 指向外部/`/nix/store` 的真实文件，丢弃 dangling）；
文本文件改写 shebang 为 `/usr/bin/env`、剥 Nix store path 片段；ELF `strip` + `nuke-refs`；
rename `.*-wrapped` 可执行文件；移除 `.a`/`.pyc`。

**最终 portability check（构建期强制 hard rule）：** 遍历每个 ELF（Linux，`patchelf --print-needed`
+ `--print-rpath`）/ Mach-O（Darwin，`otool -L`），若任一依赖或 rpath resolve 到 `/nix` 下就
**让构建失败**。这在构建期强制执行 Prime Directive 1/2。

## CI / Publishing

模型是「独立构建每个工具、每个工具发布一个 artifact」。

- **Linux build selection（两阶段）：** `discover` job 经 `nix eval` 枚举包名、resolve `outPath`、
  用 `nix path-info --store <cachix>` 查 Cachix；缓存缺失的包组成 matrix。`build` job 经 `fromJSON`
  消费、仅构建选中包（无需构建则跳过）。`workflow_dispatch` 带具体 `name` 只构建该包、带 `*`/空
  强制全部；`schedule` 总是全部。`EXCLUDE_PKGS`（专用 workflow 构建）被过滤。
- **Artifacts：** `nix build .#<name>` → `./result` 经 `rsync --copy-unsafe-links` 复制进以包名命名的
  目录 → 归档为 `<name>.linux-x86_64.tar.gz` / `<name>.darwin-arm64.tar.gz`。
- **Publishing：** `oras push` 到 `ghcr.io/curoky/standalone-binaries:<name>-<arch>`。flake 也配 Cachix
  substituter，CI 把构建 closure push 到 Cachix 加速后续构建。

## How to Make Changes

本节是**入口级导航**（改哪个文件、填哪些字段、遵循什么原则）。涉及具体执行流程（构建/验证/
静态化 ladder 决策）时下钻到对应 Skill——见每项末尾的指引。

| 我要做什么 | 改哪里 | 详细流程 |
| --- | --- | --- |
| 加一个上游工具 | [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix) 加条目 | — |
| 加/改本地 patch 或 wrapper 包 | `packages/<pkg>/` + [local.nix](file:///workspace/standalone-binaries/packages/local.nix) | patch-nixpkgs-standalone skill |
| 把过时 patch / 老版本 pin 切回上游 | manifest / 删 `packages/<pkg>/` | regress-patched-package-to-upstream skill |
| 改 normalization 行为 | [normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh) | — |

### Add a new upstream tool from nixpkgs

在 [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix) 加条目，按 [Package Selection Model](#package-selection-model) 的字段填写。要点：

- 全平台省略 `platforms`，否则设 `platforms = [ ... ]`；需要 per-system 不同配置时加 per-platform key
  （如 `aria2 = { "aarch64-darwin" = { version = "24.11"; }; };`）。
- **默认不写 `version`（取 unstable 最新版，见 Prime Directive 3）**；只在 unstable 最新版确实构建失败
  时才临时 pin 老版本。
- 选 `isStatic`（默认 `true`）、`output`、`alias`。若上游最新版无法静态编译，先走
  [patch-nixpkgs-standalone](file:///workspace/standalone-binaries/.trae/skills/patch-nixpkgs-standalone/SKILL.md) 判断该用哪级 strategy（含 shared-sibling wrapper / `bundle` 的适用条件），
  往往结果会是一个本地包（见下）。

### Add a local override / patched build

建 `packages/<pkg>/`（`default.nix` + 所需资源），经 `callPackage ./<pkg> { }` 接进
[local.nix](file:///workspace/standalone-binaries/packages/local.nix) 的 `common`/`linux`/`darwin`。**本地 patch 包只应是临时编译修复**（刻意的
repackaging——wrapper、rename 二进制、bundle sibling 依赖，如 `packages/cloc`——不在此列）。

- 怎么 patch 成可移植静态构建（strategy 分级、macOS ladder、Mach-O 改写、验证）：走
  [patch-nixpkgs-standalone](file:///workspace/standalone-binaries/.trae/skills/patch-nixpkgs-standalone/SKILL.md)。同类包的 case study 见 [package-strategies.md](file:///workspace/standalone-binaries/docs/package-strategies.md)。
- 上游修好后怎么切回上游、去掉本地 patch：走
  [regress-patched-package-to-upstream](file:///workspace/standalone-binaries/.trae/skills/regress-patched-package-to-upstream/SKILL.md)。

### Change normalization behavior

编辑 [normalize.sh](file:///workspace/standalone-binaries/scripts/normalize.sh)。把它当兼容性表面：改动 removal/rewrite 可能以微妙方式弄坏工具，优先增量改动、
在有代表性的一批包上验证。

## Design Documentation Rules

- 本文件是仓库设计 source of truth。任何影响 architecture、build flow、package selection model、
  artifact format 或 publishing model 的改动，必须在同一次改动里更新本文档。
- per-package 实现细节放 [package-strategies.md](file:///workspace/standalone-binaries/docs/package-strategies.md)；client 设计放 [client/CLAUDE.md](file:///workspace/standalone-binaries/client/CLAUDE.md)。
