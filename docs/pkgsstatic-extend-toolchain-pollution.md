# `pkgsStatic.extend` 覆写依赖污染构建工具链，导致 LLVM/rustc 从源码重编

本文记录一次排查经验：`nix build .#all-fast`（以及 `.#nodejs-slim26`）在本地会从源码编译 `llvm-21.1.8`，即便这颗 LLVM 本应能从 `cache.nixos.org` 直接拉取。根因是 `pkgsStatic.extend` 覆写了一个依赖（`libuv`），而该覆写连带改到了**构建工具链**里的同名包，使工具链闭包 hash 全部偏离官方缓存。

## 现象

- `nix build .#all-fast --dry-run` 里出现 `llvm-21.1.8` / `rustc-1.97.0`（以及一整套 musl Haskell/rust 依赖）被列入「will be built」。
- 期望：`all-fast` 已经用 `isSlowLLVM` 谓词排除了 `clang-tools-*` / `clang<N>` / `lld_*` 这些按名字命中的 LLVM 工具链包（见 [flake.nix](../flake.nix) 的 `isSlowLLVM`），本地应当很快。

## 排查过程（可复现）

### 1. 定位是谁把 LLVM 拖进来的

`isSlowLLVM` 只按**包名**排除，而这颗 LLVM 不叫 `clang*`/`lld*`，所以过滤器管不到它。用 `why-depends` 找真正的引入链：

```console
$ nix why-depends --derivation .#all-fast /nix/store/…-llvm-21.1.8.drv
all-standalone-tools-fast.drv
└── nodejs-slim26-standalone.drv
    └── nodejs-slim26-static-…-musl-26.5.0.drv
        └── temporal_capi-static-…-musl-0.2.4.drv
            └── …-rustc-wrapper-1.97.0.drv
                └── …-rustc-1.97.0.drv
                    └── llvm-21.1.8.drv
```

链路是：`nodejs-slim26` → `temporal_capi`（Node 26 新引入的 Rust 依赖，Temporal API 的 Rust 实现，自 [nodejs/node#61806](https://github.com/nodejs/node/pull/61806) 起默认启用）→ `rustc` → `llvm`。`temporal_capi` 是 Node 26 的**编译期硬依赖**，无法去掉。

### 2. 确认这颗 LLVM 是「普通 glibc」的，本该能命中缓存

关键排查点：如果它是被 `pkgsStatic` 编成 musl 静态的 LLVM，官方缓存没有、需要本地编是正常的。但检查 drv 发现它其实是普通 glibc 构建：

```console
$ nix show-derivation /nix/store/…-llvm-21.1.8.drv | grep -oE '"system":"[^"]*"|LLVM_HOST_TRIPLE[^ ]*|stdenv'
"system":"x86_64-linux"
LLVM_HOST_TRIPLE:STRING=x86_64-unknown-linux-gnu    # 不是 musl
stdenv-linux                                        # 普通 glibc stdenv，不是 musl
```

> 说明：Linux 上本仓库的 `pkgsStatic` 是 **musl64 的 cross 集**（`base.pkgsCross.musl64.pkgsStatic`，见 [flake.nix](../flake.nix) 的 `mkEnv`）。build 平台是 glibc、host==target 是 musl-static。Rust 走 `fastCross` 路径，**复用 build 平台（glibc）缓存里的 rustc/LLVM** 来交叉编译出 musl 静态产物，而**不是**重编一套 musl 静态的 rustc/LLVM。所以这颗 LLVM 是「普通 glibc、target=gnu」的，官方缓存里本应有。

### 3. 确认官方缓存里没有这颗，但有另一颗同名的

```console
$ nix path-info --store https://cache.nixos.org /nix/store/…(我们的 llvm out path)
error: path '…' is not valid          # 缓存里没有

# 对比官方 nixpkgs-unstable 里的同版本 llvm：
$ nix eval --raw --impure --expr \
  'let p = import (builtins.getFlake "github:NixOS/nixpkgs/nixos-unstable") { system = "x86_64-linux"; }; \
   in p.llvmPackages_21.llvm.drvPath'
/nix/store/s79h…-llvm-21.1.8.drv       # 官方那颗，hash 与我们那颗（q074…）不同
```

两颗同版本 LLVM 的 drv hash 不同 → 是两个不同的构建 → 官方那颗可 substitute，我们这颗不行。

### 4. 逐层 diff 输入，定位差异源头

`llvm` 自身的 `doCheck` / `patches` / `cmakeBuildType` 完全一致，差异在它的**输入依赖 hash**。逐层向下 diff（`nix show-derivation … | jq -r '.derivations[].inputs.drvs|keys[]'`）：

- `llvm` 的输入里 `cmake-4.3.4.drv` 的 hash 与官方不同；
- `cmake` 的输入里 `libuv-1.52.1.drv` 的 hash 与官方不同；
- 对比这两颗 `libuv` 的 `doCheck`：我们的为空（false），官方为 `1`（true）。

```bash
nix show-derivation /nix/store/…-our-libuv.drv |
  jq -r '.derivations[].env.doCheck // "unset"'
nix show-derivation /nix/store/…-nixpkgs-libuv.drv |
  jq -r '.derivations[].env.doCheck // "unset"'
```

这正是 [packages/nodejs/26/linux.nix](../packages/nodejs/26/linux.nix) overlay 里的 `libuv = prev.libuv.overrideAttrs { doCheck = false; };`。

## 根因

`nodejs-slim26` 用 `pkgsStatic.extend` 建了一个局部 overlay，覆写 node 静态构建所需的几个依赖（`ada` / `libuv` / `hdrhistogram_c` / `lief` / `temporal_capi` / `uvwasi`）。这些覆写本意只针对 **musl-static target** 的那份（关掉在 sandbox 里跑不了的测试、把 SHARED-only 的 CMake target 改成 STATIC 等）。

但 `pkgsStatic.extend` 得到的整个包集，其 `buildPackages`（build 平台工具链）也从**同一个 overlay** 派生。于是 build 平台（glibc）的 `libuv` 也被 `doCheck = false` 改了 hash。而 `libuv` 是 `cmake` 的 `nativeBuildInput`，`cmake` 又是 `llvm` 的 `nativeBuildInput`，连锁污染：

```text
libuv（glibc，doCheck=false 改了 hash）
  → cmake（nativeBuildInputs 含 libuv → hash 变）
    → llvm-21.1.8（nativeBuildInputs 含 cmake → hash 变）
      → rustc → temporal_capi → nodejs-slim26
```

结果：rustc 用的这颗 glibc LLVM 偏离官方缓存、必然 miss，本地从源码重编。而对 glibc build 平台的 `libuv` 来说，`doCheck=false` 根本不必要（它测试本来就能过、缓存里就有）。

## 修复

给 overlay 里所有依赖覆写加一个「仅 static target 生效」的守卫，build 平台（glibc）的那份原样透传：

```nix
# 判据：target（node 链接的）isStatic=true / isMusl=true；
#       build（cmake 用的）  isStatic=false / isMusl=false。
onlyStatic =
  pkg: overrides:
  if pkg.stdenv.hostPlatform.isStatic then pkg.overrideAttrs overrides else pkg;

pkgsStaticNode = pkgsStatic.extend (
  _: prev: {
    ada = onlyStatic prev.ada { doCheck = false; };
    libuv = onlyStatic prev.libuv { doCheck = false; };
    hdrhistogram_c = onlyStatic prev.hdrhistogram_c (old: { … });
    lief = onlyStatic prev.lief (old: { … });
    temporal_capi = onlyStatic prev.temporal_capi { doInstallCheck = false; };
    uvwasi = onlyStatic prev.uvwasi (old: { … });
  }
);
```

这样 target（musl-static）的依赖仍被正确修补，而 build 平台的 `libuv` → `cmake` → `llvm` 回到官方缓存、不再本地重编。

判据来源（验证 static 与 build 平台的区分是可靠的）：

```console
$ nix eval --impure --expr 'let p = (import (builtins.getFlake \
    "github:NixOS/nixpkgs/nixos-unstable") { system = "x86_64-linux"; }).pkgsCross.musl64.pkgsStatic; \
  in { t = p.libuv.stdenv.hostPlatform.isStatic; b = p.buildPackages.libuv.stdenv.hostPlatform.isStatic; }'
{ b = false; t = true; }
```

## 验证

- `pkgsStatic.buildPackages.rustc.llvmPackages.llvm.drvPath` 修复后变回官方可 substitute 的 `s79h…-llvm-21.1.8.drv`（与 nixpkgs-unstable 一致），不再是被污染的 `q074…`。
- `nix build .#nodejs-slim26 --dry-run`：待编从 159 项降到个位数，且全部是 `-static-…-musl-` 的 target 依赖（`ada`/`lief`/`merve`/`simdjson`/`simdutf`/`temporal_capi`/node 自身），**无 `llvm` / `rustc` / `cmake` / `libuv` 工具链包**。
- `nix build .#all-fast --dry-run`：待编列表里 `llvm-2` / `rustc-1` 零命中。

`temporal_capi`（Rust target 静态库）本身仍需编译（Node 26 硬依赖，无法去除），但它现在复用官方缓存的 glibc rustc/LLVM，不再触发从源码编 LLVM。

## 经验教训

- **`pkgsStatic.extend`（以及任何 `overlay`）会同时改写 `buildPackages` 里的同名包。** 只想改「target 链接的那份依赖」时，覆写会顺着 `nativeBuildInputs` 泄漏进构建工具链（如 `libuv → cmake → llvm`），把本可 substitute 的工具链包变成本地重编。
- **只针对 target 的覆写要加 `stdenv.hostPlatform.isStatic`（或 `isMusl`）守卫**，让 build 平台的副本保持不变、继续命中缓存。
- **排查「为什么某个包本地重编」的通法**：`why-depends` 找引入链 → `show-derivation` 看它是不是本该 substitutable 的构建（platform/stdenv）→ 与官方 nixpkgs 的同名 drv 比 `drvPath` → 逐层 diff `inputs.drvs` 直到定位 hash 偏离的源头。
- 不要被 drv/store 路径里的三元组名字（如 `…-rustc-1.97.0` 带 `x86_64-unknown-linux-musl` 前缀）误导：那是它的 **target** 三元组，编译器本身仍可能是跑在 glibc build 平台、走 `fastCross` 复用缓存的普通 rustc/LLVM。
