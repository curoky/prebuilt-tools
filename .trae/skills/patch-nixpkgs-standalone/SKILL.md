---
name: "patch-nixpkgs-standalone"
description: "把一个 nixpkgs 包 patch 成可移植的 standalone 构建，不依赖任何 /nix 下的动态库。当在 packages/ 或 manifests/ 下新增/修复一个包、或某个构建仍残留 /nix/store 动态库时调用。按平台拆分指引：Linux 强制 musl 纯静态；macOS 只允许系统库保持动态。"
---

# Patch a nixpkgs Package for Standalone Portability

在本仓库新增或修复一个包、使其 ship 出的 artifact 可移植时用本 skill。先读
`CLAUDE.md`——它是 source of truth；本 skill 是从中派生出的可操作流程。

## Prime directive：只在 stock 静态构建失败时才 patch

**绝不 patch 一个已经能正确构建并链接的包。** `packages/` 下的本地 patched derivation
**只**在纯上游静态构建（`nixpkgs.pkgsStatic.<x>`）无法编译/链接时才有理由存在。若
`pkgsStatic.<x>` 能干净构建并满足平台目标，就经 manifest 直接用它（`isStatic = true`）——
不要加本地包。下面每条 patch 路线都是针对 stock 静态构建*某个具体*失败的 fallback；
优先选侵入性最小的那条，并优先「根本不 patch」。

> 与此对偶：一旦上游修好、patch 变成 no-op，就用 `regress-patched-package-to-upstream`
> skill 把它切回 unstable 上游。本仓库的长期目标是所有包都用 unstable channel 最新版。

## The Hard Rule（both platforms）

Ship 出的 payload **绝不能依赖任何 `/nix` 下的动态库**。

- **Linux（硬底线）：** 最终产物必须是 musl 纯静态——无 glibc、无任何 `.so` 依赖。
- **macOS：** 见下文 ladder，只允许 macOS 系统库保持动态。

纯文本包（脚本、字体、数据、无编译对象的纯 Perl/Python 源码）豁免静态链接目标——它们
只走 `normalize.sh` 的 shebang/path 改写。凡是含 ELF/Mach-O 二进制的包都必须满足下面的平台目标。

## Step 0 — Decide the split

一个包通常各平台需要不同处理。经 `manifests/default.nix` 的 per-platform key、或经
`packages/local.nix`（`linux` / `darwin` 集合）接好平台特定的 derivation。同一份构建
到处都能用时留一个共享文件；不行时拆成 `default.nix`（Linux）+ `darwin.nix`。

## Step 1 — Is it text-only?

若包不 ship **任何**编译二进制（只有脚本/数据/源码）：
- 无需静态链接。
- 确保 `normalize.sh` 能处理它（shebang 改写、剥 `/nix` path 片段）。
- 若是需要 runtime 的脚本（perl/python/node），用 **sibling-wrapper** 模式：把 `bin/tool`
  rename 成 `bin/_tool`，放一个调用同级静态 runtime sibling（`$store/perl/bin/perl`、
  `$store/python311/...`、`$store/nodejs-slim26/bin/node`）的 wrapper。见 `packages/exiftool`、
  `packages/cloc`。
- 完成，跳过其余。

---

## LINUX — Goal：force musl full static

目标：一个完全静态的 ELF（musl）。**这不要求字面上在 manifest 里写 `pkgsStatic.<x>`**——
它指*结果*是静态链接的。按顺序尝试路线；**若更早的路线已能构建就不要 patch**：

1. **`isStatic = true`（默认）in the manifest**——直接从 `pkgsStatic` env 构建、无本地包。
   **若 `pkgsStatic.<x>` 能编译并链接，就到此为止——不要 patch。** 这是默认路线、对大多数
   工具有效。见 `packages/wget/default.nix`（`pkgsStatic.wget` 直通）。
2. **Selective static override**——仅当 `pkgsStatic.<x>` 构建/链接失败时。从更轻的 base 构建、
   经 `.override { <lib> = pkgsStatic.<lib>; }` 或把某模块构建指向 `pkgsStatic.<lib>` lib 目录
   （只 ship `.a`）只注入所需静态归档。见 `packages/exiftool`（XS 压缩模块重指向
   `pkgsStatic.{zlib,bzip2,xz,brotli}`）。
3. **Feature reduction**——仅当静态构建卡在可选特性上：从 `pkgsStatic.<tool>` 起步、
   `.override` 关掉惹事特性，保留每个 `.a` 能干净链接的库。模式见 `packages/ffmpeg/darwin.nix`
   （同一思路在 Linux 上也适用）。
4. **Go/CGO 工具：** 设 `CGO_ENABLED=0` 彻底去掉 libc。
5. **仅限最后手段：** 若一个工具确实无法静态编译（如 Node.js runtime），用 sibling-wrapper
   runtime 模式；仅当没有可复用的 sibling runtime 时，才用 `bundle = true`（仅 Linux）。

Verify（Linux）：构建后，二进制必须是静态且无 `/nix` 引用（见下文 Verification）。

---

## macOS — Goal：only macOS system libs may stay dynamic

darwin 上完全静态不可能（没有静态 libSystem/libc）。目标：**每个 nix 内部依赖都静态链接，
只有 macOS 系统库可以保持动态**——`/usr/lib/libSystem.B.dylib`、其他 `/usr/lib/*`、
`/System/Library/Frameworks/*`。**不允许任何 `/nix/store` dylib 存活。**

按顺序套用这个 ladder——**在第一个可行处停下，若 `pkgsStatic.<x>` 已能构建就不要 patch**：

1. **先修 `pkgsStatic.<x>`。** 永远先尝试让 stock 完全静态的 `pkgsStatic.<x>` 构建/链接。
   若它能干净构建，直接用——不 patch。若它*几乎*能构建，优先用一个小而精准的上游 patch
   让静态归档在 darwin 上链接成功，而非放弃静态路线。见 `packages/krb5/darwin.nix`（禁用
   CCAPI ccache 后端 + 移动一处 DES const，使 `libkrb5.a`/`libk5crypto.a` 能 resolve；结果只
   依赖 `/usr/lib/libSystem`）。有时只需把一个 *build-time* 工具换成 native 集、而二进制本身
   仍完全静态——见 `packages/wget/darwin-static.nix`（darwin `pkgsStatic.perl` 坏了，所以
   `perlPackages` 被 override 成 native，但 wget 仍把每个依赖静态链接）。也可把 **feature
   reduction** 作为修静态构建的一部分：从 `pkgsStatic.<tool>` 起步、`.override` 只关掉那些静态
   构建/链接失败的可选特性，保留其余。见 `packages/ffmpeg/darwin.nix`。

2. **仅当 `pkgsStatic.<x>` 的问题明显无解时：** fall back 到 **native `pkgs.<x>`** derivation
   （上游缓存里已预编译、无需本地编 toolchain），把它每个非系统的动态依赖换成对应静态归档——
   即 `pkgs.<x>.override { <dep> = pkgsStatic.<dep>; }`（或把构建指向只 ship `.a` 的
   `pkgsStatic.<dep>` lib 目录）——使每个 nix 依赖静态链接、只有 `/usr/lib`/framework 库保持
   动态。这不动 Mach-O load command 就达成最终目标。见 `packages/perl/darwin.nix`（native perl +
   `libxcrypt = libxcryptStatic`）和 `packages/wget/darwin.nix`（逐依赖静态替换变体）。

3. **若 step 2 仍留下你无法静态替换的 `/nix/store` dylib：停手，先与用户确认。** 只有在显式
   确认后，才用 `install_name_tool` 路线，在 `postInstall` 里把残留的 `/nix/store` Mach-O
   install name 改写成 `@loader_path` 相对路径（`normalize.sh` **不**动 Mach-O load command）。
   见 `packages/perl/darwin.nix` 里 repoint `libperl.dylib` 的 `install_name_tool -id`/`-change` 循环。

4. **把 dylib 复制进包**（dylib-bundle）——绝对最后手段，仅当上述任何路线都无法静态链接某依赖时。
   **这需要用户显式确认后才能实施。** 不要静默做。到这一步时，停手并向用户询问（附上具体依赖及
   为何无法静态链接）再继续。确认后：把 dylib 复制到二进制旁，把它的 install name / 消费方的
   load command 改写成 `@loader_path` 相对，使 payload 保持可 relocate。

### macOS Mach-O relocation reminder

`normalize.sh` 处理 ELF（`nuke-refs`、strip）但**不**处理 Mach-O load command。任何在包里
留下非系统 dylib 的 darwin 路线，必须在 `postInstall` 里：
- `install_name_tool -id "@loader_path/<name>.dylib" <dylib>`
- `install_name_tool -change "<old /nix or abs id>" "@loader_path/..." <consumer>`
- 用 `otool -D` 读当前 id，用 `otool -L`/`file` 枚举。
完整循环见 `packages/perl/darwin.nix`。

---

## Verification（宣布完成前必做）

构建 standalone 产物，确认没有残留的禁止动态依赖。

- **Linux** — 必须完全静态、无 glibc、无 `/nix` interpreter/引用：
  - `file ./result/bin/<tool>` → 期望 "statically linked"。
  - `ldd ./result/bin/<tool>` → "not a dynamic executable"（或无 `/nix` 条目）。
  - 没有作为真实运行期依赖的 `/nix/store` 字符串（其余由 normalize nuke-refs 掉）。
- **macOS** — 每条 load command 必须指向 `/usr/lib/*`、`/System/Library/Frameworks/*`、
  或包内 `@loader_path` 相对路径；**零** `/nix/store` 条目：
  - `otool -L ./result/bin/<tool>`（以及任何 ship 的 `.dylib`/`.bundle`/`.so`）。
  - grep 整个树找残留的 `/nix/store` Mach-O 引用。

若仍有 `/nix/store` 动态库存活：回到 ladder 上一层（`CGO_ENABLED=0`、patch install names/rpaths、
静态替换该依赖，或——经确认后——复制 dylib）。

## Guardrails

- 最小 diff：只 patch 能改善 portability / 移除动态依赖 / 修运行期路径的部分。别重构无关代码。
- 匹配既有包风格（`overrideAttrs`、`postInstall`、wrapper 脚本）。
- 若你的改动影响 architecture、build/package selection、或 artifact format，在同一次改动里更新
  `CLAUDE.md`（仓库规则）。
- 在 macOS 上**绝不**未经用户显式确认就复制 `/nix` dylib。
