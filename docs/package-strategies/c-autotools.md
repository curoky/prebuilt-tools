# C / autotools 构建案例

绝大多数 C/autotools 工具走最简路径：**`pkgsStatic.callPackage` 直接吃默认静态**（Linux musl 纯静态），
`.nix` 文件本身不加 `-static`，静态归属交给包集 + `normalize.sh` 校验。大部分额外工作不是静态编译，而是
两类可移植性收尾：**rename + relative-path wrapper**（处理 baked 数据路径）和**去掉 baked 进二进制的
`/nix/store` 绝对路径**。

本文覆盖：跨平台 common 的 C 工具、Linux-only C 工具（cmake/git/openssh 等）、s6 stack、clang-tools、
以及容器栈的 C 组件。真正需要 linker/工具链级修复的重案例（gnutar/postgresql）见
[特殊案例](special-cases.md)。

## 跨平台 common 的 C 工具

均 `pkgsStatic.callPackage`，Linux musl 纯静态、macOS 分依赖静态。

**只加 rename + relative-path wrapper（处理 baked 数据/exec 路径）：**

- **file**：wrapper `file`→`_file`，运行期用相对 `$root/share/misc/magic.mgc` 指定 magic 数据库。
- **vim**：wrapper `vim`→`_vim`，若无 `VIMRUNTIME` 则相对设 `$root/share/vim/vim92`（静态 vim 找不到
  runtime 目录）。
- **makeself**：wrapper `makeself`→`_makeself`，相对指定 `--header .../makeself-header.sh`。
- **libtool**：`postInstall` 用 `sed` 把 `libtoolize` 里硬编码的 `prefix=`/`datadir=`/`pkgauxdir=`
  改为运行期经 `$root`（`readlink -f "$0"`）相对解析，去掉 store 绝对路径。
- **autoconf / automake**：`postInstall` 把入口二进制/脚本 rename 为 `_<name>`，从 `./scripts/` 覆盖
  同名 relative-path wrapper。autoconf/automake 本质是脚本集，无静态链接特殊处理。

**自带 CA bundle（静态网络工具的可移植性）：**

- **curl**：用 `stdenv.mkDerivation` 重组：拷 `curl.bin`/`curl.dev`，把官方 `cacert` 的 CA bundle 装到
  `etc/ssl/certs/ca-bundle.crt`，rename `curl`→`_curl`，wrapper 相对解析并强制 `--cacert`。
  静态 curl 没有系统 CA 路径，自带证书使其可移植。（`wget/linux.nix` 同模式，见
  [特殊案例](special-cases.md)。）

**feature / 依赖内联：**

- **gettext**：加 `--with-included-gettext`、`--with-included-libintl`，用内置 gettext/libintl 而非
  外部库，减动态依赖。
- **zsh**（三处 root-cause 处理）：
  1. `hardeningDisable += ["fortify"]`——musl 静态 build 下 `_FORTIFY_SOURCE` 编 `Src/sort.o` 时
     触发 GCC 的 object-size pass ICE。
  2. 过滤 `--enable-zshenv=$out/etc/zshenv` 改为 `/etc/zsh/zshenv`——nixpkgs 把全局 zshenv pin 进
     只读 store，改指系统路径。
  3. `patchPhase` 把 system/regex/mathfunc 三模块 `link=either`、`termcap.c` 的
     `#ifndef HAVE_BOOLCODES` 改 `#if 0`——静态构建下模块链接方式与 boolcodes 声明冲突。
     外加 rename `_zsh` + 相对设 `FPATH` 的 wrapper。

**其它：**

- **diffutils**：`overrideAttrs` 唯一改动 `doCheck = false`（跳过测试）。属纯编译修复 workaround，
  **需按根 `CLAUDE.md` 的回归流程评估切回上游**。
- **rsync**：只改 `checkPhase`（删 `testsuite/itemize.test` + `make check EXCLUDE=itemize`）。
- **p7zip**：`preConfigure` 追加 `buildFlags=default`，`outputs = [out doc man]`。
- **protobuf 3.8.0 / 3.9.2**：都只是 `callPackage ../generic-v3.nix` 传 `version`+`sha256`，共享
  builder `protobuf/generic-v3.nix`（源自 nixpkgs release-24.05）。关键 `dontDisableStatic = true`
  （保留静态归档），从 `gtest.src` 铺 gmock/googletest；darwin 分支 `substituteInPlace ...
  googletest.cc --replace 'tmpnam(b)'`（darwin `tmpnam` 问题）。
- **eza-ls**：base=`stdenvNoCC`，**无编译**：`dontUnpack`，拷 `pkgsStatic.eza`（静态二进制）+ bash
  `ls-wrapper.sh`，把 `ls` 风格 flag 翻译成 eza 参数，不支持的选项透明回退 `/bin/ls`。静态性来自被拷贝的
  `pkgsStatic.eza`。兼容性由 `tests/compat.bats` 刻画。

## Linux-only C 工具

- **cmake（三份，均 base=`pkgsStatic`）**：共同点是 `--no-system-libs`（全内置）、
  `CMAKE_EXE_LINKER_FLAGS=-static`（关键静态开关）、显式把 `CMAKE_C/CXX_COMPILER`、`AR/RANLIB/STRIP`
  指向带 `targetPrefix` 的 musl 工具绝对路径（cross 需要），关 OpenSSL/CursesDialog/`BUILD_TESTING`。
  - `cmake/default`：override `cmakeMinimal`，`CXXFLAGS=-Wno-elaborated-enum-base`。
  - `cmake/3_27_9`：直接 `stdenv.mkDerivation` 自建（fetchurl）。额外 `postPatch` 给 `cmcppdap
    network.h` 补 `#include <cstdint>`，`preConfigure` 改 `UnixPaths.cmake` 注入 libc 路径 + 用
    `CC_FOR_BUILD` bootstrap，`NIX_CFLAGS_COMPILE=-Wno-unused-command-line-argument`（`-pie` unused
    警告会干扰 C++11 特性检测误判编译器）。
  - `cmake/4_1_2`：结构同 3.27.9，`separateDebugInfo=true`，patch 列表全注释未启用。
- **git**：base=`pkgsStatic`。先 `git.override` 关 `pythonSupport/nlsSupport/perlSupport/withManual`
  （feature reduction）。`buildInputs` 加 `nghttp2/libpsl/c-ares/brotli`。关键手动补齐静态传递依赖：
  `env.NIX_LDFLAGS += " -static -lnghttp2 -lnghttp3 -lcares -lngtcp2 -lngtcp2_crypto_ossl -lpsl
  -lssl -lcrypto -lssh2 -lidn2 -lzstd -lz -lunistring -lbrotlidec -lbrotlicommon"`（静态链接不会自动
  拉传递依赖）。`patchPhase` 用 `sed` 把源码里 `error(` 全局 rename 成 `git_error(`（规避与某静态库
  `error` 符号冲突）。wrapper 相对设 `GIT_TEMPLATE_DIR`/`GIT_EXEC_PATH`，rename `_git`。
- **openssh_gssapi**：base=`pkgsStatic`，仅 wrapper：rename `scp`→`_scp`（相对 `-S $root/bin/ssh`）、
  `sshd`→`_sshd`（相对注入 `SshdSessionPath`/`SshdAuthPath`）。静态由 `pkgsStatic` 提供。

> `gnutar`（`*xattrat` 符号冲突）、`postgresql`（psql-only + gccAsClang）、`wget/linux.nix`（CA
> bundle）、`nsight-systems`（prebuilt）见[特殊案例](special-cases.md)。

## s6 stack（execline / s6 / s6-linux-init / s6-rc）

四包均 `pkgsStatic`（Linux-only，musl 纯静态）。**难点不在静态，而在去掉编译期 baked 进二进制的
`/nix/store` 绝对路径**——统一过滤 `--enable-absolute-paths`、`sed` 置空/改写各 `*PREFIX` 宏，并用
`override` 把依赖回写成本仓库 patched 的 s6/execline（否则会 bake 默认 execline/s6 的 store 路径）。

- **execline**：过滤 `--enable-absolute-paths`（该 flag 使 `EXECLINE_BINPREFIX` 变绝对 store 路径）。
  `postConfigure` 把 `config.h` 的 `EXECLINE_SHEBANGPREFIX` 改为 `"/usr/bin/env -S "`——shebang 必须
  绝对路径（内核不搜 `$PATH`），用 `-S` 让 `env` 拆分 `execlineb -P`（Debian 10 的 env 不拆分）。
- **s6**：`s6.override { inherit execline; }`（必须回写 patched execline）。过滤
  `--enable-absolute-paths`；`postConfigure` 把 `S6_LIBEXECPREFIX` 置空（否则 `s6-ftrig-listen` bake
  `s6-ftrigrd` 的 store 路径），置空后走 `$PATH`。
- **s6-linux-init**：`override { inherit s6 execline; }`（编译期把 `S6_EXTBINPREFIX`/
  `EXECLINE_EXTBINPREFIX` bake 进生成的 init 脚本）。过滤 `--enable-absolute-paths`，`symlinkJoin`
  合并 `out`+`bin`。
- **s6-rc**：`override { inherit s6 execline; }`（s6-rc-compile 编译期把 `<s6/config.h>`/
  `<execline/config.h>` 前缀 bake 进 service 脚本）。过滤 `--enable-absolute-paths`；`postConfigure`
  追加把 `S6RC_EXTLIBEXECPREFIX` 置空。

## 容器栈的 C 组件（crun / conmon / catatonit / gpgme）

均 base=`pkgsStatic`（Linux musl 全静态）。与 Go 的 `podman` 同栈但按语言分开处理（podman 见
[Go 案例](go.md)）。

- **crun**：`crun.override { withLibkrun=false; withLibkrunSEV=false; }` 再 overrideAttrs 强制全静态：
  `env` 显式 `CFLAGS="-static"`、`LDFLAGS="-static"`、`CRUN_LDFLAGS="-all-static"`、`NIX_LDFLAGS=""`；
  `configureFlags` 含 `--enable-static`、`--disable-systemd`、`--enable-embedded-yajl`、
  `--without-python-bindings`。buildInputs：libcap/libseccomp/yajl/argp-standalone。`doCheck = false`。
- **conmon**：极简 `overrideAttrs`，buildInputs 收窄为 `glib`、`libseccomp`，清空
  `propagatedBuildInputs`。静态来自 pkgsStatic。
- **catatonit**：仅 `overrideAttrs { installCheckPhase = ""; }`。
- **gpgme**：`gpgme.override { gnupg = minimalGnuPG; }`（`enableMinimal`/关 `guiSupport` 缩减依赖），
  再 overrideAttrs 加 `--disable-gpg-test`、`doCheck = false`。

## clang-tools（18 / 19 / 20 / 21 / 22）

五个版本目录 `clang-tools/{18..22}` **结构完全一致，仅 `llvmPackages_NN` 与 `version` 字符串不同**
（未抽公共 builder，是逐版本复制）。base=`pkgsStatic`。重点是**产物瘦身 + 只抽 `clang-format` 单二进制**，
而非纯静态编译：

- 内层：`llvmPackages_NN.clang-unwrapped.overrideAttrs`——`env.NIX_CFLAGS_COMPILE += " -g0
  -ffunction-sections -fdata-sections"`、`env.NIX_LDFLAGS += " --gc-sections -s"`（去调试信息、按
  section GC、strip）；`cmakeFlags += ["-DCMAKE_BUILD_TYPE=MinSizeRel" "-DLLVM_USE_LINKER=mold"]`
  并设置 `nativeBuildInputs += [mold]`（LLVM 全量太大，用 mold + MinSizeRel 最小体积构建）。
- 外层：`stdenv.mkDerivation`，`unpackPhase`/`buildPhase` 皆空，`installPhase` 只 `cp
  ${clang}/bin/clang-format $out/bin/`（不发 clang-tidy）。

静态性由 `pkgsStatic` 的 musl 集提供，文件本身不加 `-static`，交给包集 + `normalize.sh` 校验。
