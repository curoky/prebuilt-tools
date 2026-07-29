# 基础工具特例（feature reduction / 工具链修复 / prebuilt）

本文收录那些**无法靠默认 `pkgsStatic` 直接静态化**、需要 feature reduction、linker/工具链级修复、或
本身是 prebuilt 二进制重打包的重案例。共同点是修复的 root cause 都很具体，且大多是需要跟踪回归的
**临时编译修复**（见 [regress skill](file:///workspace/standalone-binaries/.trae/skills/regress-patched-package-to-upstream/SKILL.md)）——除非明确标注是结构性需要。

## ffmpeg（macOS，ladder 2b：feature reduction）

`packages/ffmpeg/darwin.nix`。在 `pkgsStatic.ffmpeg-headless` 之上关掉无法静态构建/链接的可选特性。
完整 `pkgsStatic.ffmpeg` 在 aarch64-darwin 无法构建，逐条 root cause：

- dav1d/opus 等静态构建撞上 meson `arm64` cross-file bug；
- zimg/vid-stab/OpenCL 拖进 `openmp` → `llvm-static`，后者在 CheckAtomic/libatomic 处失败；
- `libopenmpt` 拖进一个会被 SIGKILL 的 autogen 步骤。

所以关掉 `withDav1d`/`withOpus`/`withZimg`/…。关掉所有 `openmp` 路径也让 `nix eval` 保持干净、无需
`config.problems` handler（openmp 是唯一会把 darwin 静态 `python3` 标记 broken 的东西）。`withOpenapv`
关掉是因为 `liboapv` 即便在 `pkgsStatic` 下也只 ship 一个 `.dylib`；network/TLS 库（gnutls/ssh/srt/
rist）关掉是因为过不了 ffmpeg 的静态 configure 链接测试。

`x265` 保留，但需两处包修复：

- 去掉它 `postInstall` 里的 `rm -f $out/lib/*.a`——在 `pkgsStatic`/`ENABLE_SHARED=false` 下它会删掉
  唯一产物 `libx265.a`；
- `multibitdepthSupport = false` 以避免静态归档里未定义的 `x265_1{0,2}bit::` 符号（代价：只支持 8-bit
  HEVC 编码）。

结果只依赖 `/usr/lib/*` 和 `/System/Library/Frameworks/*`。

## postgresql（Linux，只出 psql 客户端）

`packages/postgresql/default.nix`，base=`pkgsStatic`，**只出 psql 客户端 + libpq 静态归档**。
`pkgsStatic.postgresql` 直接失败，root cause：

- postgres 的 `generic.nix` 为启用 `-flto` 把编译器从 gcc 切成 clang，而本仓库 musl64-cross 静态集里
  clang 是坏的（找不到 `-lgcc_eh`、缺 `libunwind.a`、无可用 `lld`）。解法：构造
  `gccAsClang = stdenv // { cc = stdenv.cc // { isClang = true; }; }` 伪装成 clang stdenv，让
  `generic.nix` 保留可用的 gcc。
- gcc 的 `-flto` 破坏 postgres 的 partial-link（`ld -r`），故 `env.CFLAGS="-fdata-sections
  -ffunction-sections"`（去 `-flto`，保 section-GC）。
- 全静态 gcc 无法链接 `.so`，故跳过 server backend（charset `.so` 模块），只建 libpq 静态归档 + psql：
  `substituteInPlace src/Makefile.shlib` 把 `all-lib: all-shared-lib`→`all-static-lib`，自定义
  `buildPhase`/`installPhase` 只 make `libpq all-lib` 和 `psql`。
- 关掉 `jitSupport/perlSupport/pythonSupport/tclSupport/curlSupport/gssSupport`（各自静态 configure
  link 失败，如 curl "does not provide curl_multi_init"、gss "could not find function
  gss_store_cred_into"）。
- 清 `meta.broken=false`（上游对 `isStatic` 标 broken 是因 server 不能 dlopen，此处不建 server）；单
  output，把 dev/doc/man/lib 全指向 `$out` 破除引用环。

## krb5（macOS，ladder 1：小上游 patch）

`packages/krb5/darwin.nix`。构建完全静态的 `pkgsStatic.krb5`，但禁用 macOS CCAPI ccache 后端
（`USE_CCAPI_MACOS`）、并移动一处 DES const 定义（`mit_des_zeroblock`），使 `libkrb5.a`/
`libk5crypto.a` 的静态归档链接能 resolve。结果只依赖 `/usr/lib/libSystem`。Linux krb5 直接从 manifest
取上游。

## wget（两平台完全静态）

两平台都经 `pkgsStatic` 完全静态：

- **Linux**（`wget/linux.nix`）：`wget_static = wget.overrideAttrs { doCheck=false; }`，再
  `stdenv.mkDerivation` 拷 bin/etc/share、注入自带 CA bundle（`cacert-*.pem` 装到
  `etc/ssl/certs/ca-certificates.crt`）、rename `wget`→`_wget`、wrapper 相对 `--ca-certificate`
  （与 curl 同模式）。
- **macOS**（`wget/darwin-static.nix`）：取同一 `pkgsStatic.wget`，**只把它 build-time 的
  `perlPackages` override 成 native 集**。root cause：darwin 的 `pkgsStatic.perl` 构建失败——最后的
  `mktables` 步骤跑刚编出的静态 miniperl 生成 Unicode 表，而该静态 miniperl 在 darwin 崩溃（静态 build
  禁掉了大部分 locale 支持：`-DNO_THREAD_SAFE_QUERYLOCALE` 等），在 "Updating 'mktables.lst'" 后
  exit code 2。wget 只把 perl 当 build tool，指向 native（cache-prebuilt）perl 即可绕过，wget 二进制
  本身仍每个 nix dep 静态链接，只剩 `/usr/lib` 动态。
- 备选（保留未启用）：`wget/darwin.nix` 走 native `pkgs.wget` + 逐依赖换 `pkgsStatic` 归档的变体。

## gnutar（Linux，linker workaround）

`packages/gnutar/default.nix`，base=`pkgsStatic`。root cause：`pkgsStatic.gnutar (1.35)` 静态链接报
`libtar.a(xattr-at.o): multiple definition of 'setxattrat'`（及 `getxattrat`/`listxattrat`），
`acl-static/libacl.a(xattrat.o): first defined here`。原因：gnutar bundle 了旧的 gnulib `xattr-at`
模块，而较新的 libacl（2.4.0）现在也 ship 真正的 `*xattrat` 符号，静态链 `tar` 会同时拉进两者，GCC 15
默认 `-fno-common` 暴露冲突。

解法：`makeFlags += ["LDFLAGS=-Wl,--allow-multiple-definition"]`——保留第一个定义、丢重复，从而保住
ACL/xattr 支持而非关掉它们。注释强调只加到 `make`（不加到 configure 的编译器检查，否则 cross 下会坏），
因 `LDFLAGS` 是 automake 用户变量，仅作用于最终链接。属**临时编译修复**，libacl/gnutar 上游协调后应回归。

## music-decrypto（.NET AOT）

`packages/music-decrypto/default.nix`，`buildDotnetModule`（.NET 8，两平台）。用 `PublishAot=true` +
`PublishTrimmed`（partial）+ `InvariantGlobalization` 产 AOT native 二进制，`selfContainedBuild`。
`buildInputs` 用 `zlib.static`。收尾 platform-split：

- **Linux**：`patchelf --set-interpreter /lib64/ld-linux-x86-64.so.2`、`--set-rpath
  "/lib64:/usr/lib64"`——**注意这是 glibc 动态可执行文件**（.NET AOT 未走 musl 静态），指向宿主 glibc。
  这是该包的现状特例，不满足 musl 纯静态硬底线（`StaticExecutable=true` / `linux-musl-x64` 在注释里未
  启用）。
- **macOS**：`install_name_tool -change` 把 `libicucore` 引用重指到 `/usr/lib/libicucore.A.dylib`
  系统库。

## nsight-systems（Linux，prebuilt 重打包）

`packages/nsight-systems/default.nix`。重打包 NVIDIA 预编译 native 二进制（非静态编译）：`fetchurl` 下
`.run` 自解包安装器，`sed` 把 `/dev/tty`→`/dev/null` 以便非交互，`perl ./install-linux.pl
-targetpath=$out -noprompt` 安装。二进制是 NVIDIA 提供的 glibc 动态可执行文件，本包不做静态化，属
**"repackaged prebuilt" 例外**，musl 纯静态硬校验对其不适用。
