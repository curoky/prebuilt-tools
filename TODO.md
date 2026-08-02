# 上游回归清单

本表是人和 agent 共用的 pin、patch 与本地 packaging 总账，也是批量回归的唯一输入。

状态含义：

- ✅ **整项候选**：优先尝试直接回到 unstable 上游；标记只表示值得验证，不表示已经构建成功。
- 🟡 **部分候选**：只回归临时 workaround，必须保留表中写明的 packaging 或产品行为。
- ❌ **结构性保留**：当前不是上游回归目标；用于说明本地包为何存在，避免误删。
- ⏳ **长期审计**：动态或预编译例外；只有出现满足仓库产物不变量的替代方案时才处理。

按平台拆成 [Linux](#linux) 和 [macOS](#macos) 两张表，每张表只列该平台有定制的包；跨平台包在两张表都出现。
`定制` 列含义：📌 表示旧 nixpkgs pin，🩹 表示编译或 portability patch，📦 表示结构性 packaging，
⚠️ 表示动态例外，⏸️ 表示临时停用。

批量回归按表格顺序遍历 `✅` 和 `🟡` 行：

```bash
rg '^\| .+ \| (✅|🟡)' TODO.md
```

`commit` 记录最后一次在该平台做回归测试时 `flake.lock` 里 `nixpkgs-unstable` 的 rev（短 hash），
未测过填 `—`。审计时若该 commit 与当前 `flake.lock` 的 unstable rev 相同，说明该平台在当前 channel
已测过、可跳过；rev 变化后需重新验证。`原因与保留边界` 记录该平台的失败原因或保留边界。

回归成功后，整项回归删除该行；部分回归更新原因与判据，只保留尚未解决的部分，并刷新 commit。
跨平台包在某个平台回归成功后只删该平台表中的行，另一平台的行保留。新增或改变非 unstable pin、
本地 derivation、override、禁用检查或动态例外时，必须同步维护本表。

## Linux

| 包 | 定制 | 回归 | 原因与保留边界 | 回归判据 | commit | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `autoconf` | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | — | `packages/autoconf/` |
| `automake` | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | — | `packages/automake/` |
| `catatonit` | 🩹 本地 | ✅ | 清空 stock `installCheckPhase`：上游 check 跑 `readelf` 但未把 binutils 加入 `nativeBuildInputs`，musl64 cross `strictDeps` 构建下报 `readelf: command not found` | 上游把 binutils 加入 `nativeBuildInputs`（或改用可用 check）后，unstable install check 与最终静态验证通过 | 624af665418d | `packages/catatonit/` |
| `clang-tools-18` | 📦 本地 | ❌ | 固定 LLVM 18，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | — | `packages/clang-tools/` |
| `clang-tools-19` | 📦 本地 | ❌ | 固定 LLVM 19，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | — | `packages/clang-tools/` |
| `clang-tools-20` | 📦 本地 | ❌ | 固定 LLVM 20，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | — | `packages/clang-tools/` |
| `clang-tools-21` | 📦 本地 | ❌ | 固定 LLVM 21，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | — | `packages/clang-tools/` |
| `clang-tools-22` | 📦 本地 | ❌ | 固定 LLVM 22，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | — | `packages/clang-tools/` |
| `cloc` | 📦 本地 | 🟡 | 实测 `doInstallCheck=false` 不可回归：上游 installCheck 跑 `$out/bin/cloc`（sibling wrapper），沙箱无 sibling perl 报 `perl: No such file or directory`；sibling Perl wrapper 与模块 bundling packaging 保留 | 只恢复可运行的 install check | 624af665418d | `packages/cloc/` |
| `cmake_3_27_9` | 📌 源码版本 + 🩹 | 🟡 | 已回归掉冗余 flag（删除 `CXXFLAGS=-Wno-elaborated-enum-base`、`CMAKE_EXE_LINKER_FLAGS=-static`、`NIX_CFLAGS_COMPILE=-Wno-unused-command-line-argument`）；仍保留 `postPatch`（补 `#include <cstdint>`）、`BUILD_TESTING=false`（否则 shared-module test 报 `R_X86_64_32 against __TMC_END__`）与 openssl/curses 关闭 | 上游修复老源码 header 与静态 shared-module test 后删除剩余 workaround，保留版本化 output | 624af665418d | `packages/cmake/3_27_9/` |
| `cmake_4_1_2` | 📌 源码版本 + 🩹 | 🟡 | 已回归掉 `CMAKE_EXE_LINKER_FLAGS=-static`；仍保留 `--no-system-libs`、openssl/curses 关闭、`BUILD_TESTING=false`（同 3.27.9 的 shared-module test 链接失败） | 上游支持静态 shared-module test 后删除剩余 workaround，保留版本化 output | 624af665418d | `packages/cmake/4_1_2/` |
| `conmon` | 🩹 本地 | ✅ | 收窄 build inputs 并清空 propagated inputs：stock unstable 的 propagatedBuildInputs 拉入 `systemd-minimal`，其 `badPlatforms` 含 `isStatic`，musl-static 下 eval 即被拒 | stock unstable 无需清空 propagated inputs 即可构建为 musl-static | 624af665418d | `packages/conmon/` |
| `crun` | 🩹 本地 | 🟡 | 实测均不可回归：stock features 拉入 `elfutils`（`badPlatforms` 含 isStatic）eval 即被拒，故 feature 禁用必须保留；恢复 checks 后 348 项 37 failed（rootless/namespace 用例在 musl 静态沙箱失败），`doCheck=false` 保留 | 逐项恢复 stock features/checks，保持 musl-static | 624af665418d | `packages/crun/` |
| `curl` | 📦 本地 | ❌ | 内置 CA bundle 与相对路径 wrapper | 自包含证书定位是 packaging | — | `packages/curl/` |
| `diffutils` | 🩹 本地 | ✅ | 仅禁用 stock checks：unstable diffutils 3.12 的 gnulib checkPhase 在 musl-static 下 9 个多线程/setlocale 测试失败（`test-setlocale_null-mt`、`test-thread_create` 等 SIGABRT） | Linux 用 unstable 全量 checks 与 musl-static portability 验证通过 | 624af665418d | `packages/diffutils/` |
| `dive` | 📌 `25.11` | ✅ | 去 pin 实测失败：unstable 静态依赖链 dive→gpgme-static→gnupg-static→openldap-static 在 openldap 配置阶段报 `Could not locate Cyrus SASL`（`sasl.h`/`-lsasl2` 缺失），构建中断；macOS 已回归到 unstable native | Linux 用 unstable 并满足 musl-static portability | 624af665418d | `manifests/default.nix` |
| `dool` | 📦 本地 | ❌ | Python sibling runtime wrapper，并默认追加 `--bytes` | runtime 与产品默认行为必须保留 | — | `packages/dool/` |
| `execline` | 🩹 本地 | 🟡 | 实测不可回归：stock unstable execline 2.9.9.2 用 `--enable-absolute-paths`，产物二进制文本 baked 自身 `/nix/store/.../bin` 路径（`EXECLINE_BINPREFIX`/`EXECLINE_SHEBANGPREFIX`），仍需 patch 去掉 baked prefix | stock 输出不再写入 Nix store 路径 | 624af665418d | `packages/execline/` |
| `exiftool` | 📦 本地 | 🟡 | 已删 `propagatedBuildInputs = [ ArchiveZip ]` override（stock `perlPackages.ImageExifTool` 已自带 Archive-Zip 等压缩模块，模块 bundling 靠 postInstall rsync 独立于该 override）；仍保留 `doInstallCheck=false`（versionCheckPhase 跑 sibling wrapper，沙箱无 sibling perl）与 sibling Perl/模块 bundling packaging | install check 与 wrapper packaging 保留，仅在上游可运行 install check 时恢复 | 624af665418d | `packages/exiftool/` |
| `eza-ls` | 📦 本地 | ❌ | 自定义 `ls` 兼容层与 bundled eza | 这是独立产品行为，不是上游 bug | — | `packages/eza-ls/` |
| `file` | 📦 本地 | ❌ | wrapper 相对定位 `magic.mgc` | 可搬运资源定位必须保留 | — | `packages/file/` |
| `gdb` | 📌 `25.11` | ❌ | 历史 pin；已验证：unstable gdb 17.2 的构建依赖 `dejagnu → expect` 在 musl-static 下链接失败（`undefined reference to tclStubsPtr`），连带 gdb 无法构建 | 已确认两平台都必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `git` | 🩹 本地 | 🟡 | 实测无可回归项：`error`→`git_error` 符号冲突 patch 删除后仍 `multiple definition of 'error'`（libgit.a vs libidn2 gnulib）；恢复 install check 报 `git-prompt.sh: File exists`；静态传递依赖与 `-static -lnghttp2` 必需；相对资源 wrapper 保留 | 逐项删除构建 workaround，保留 wrapper | 624af665418d | `packages/git/` |
| `git-filter-repo` | 📦 本地 | ❌ | Python sibling runtime | runtime packaging 不会因上游构建修复消失 | — | `packages/git-filter-repo/` |
| `glibcLocales` | 📦 override | ❌ | 只发布裁剪后的 locale 数据 | 输出裁剪是产品决策 | — | `packages/local/linux.nix` |
| `gnupg` | 📦 override | ❌ | 明确启用 minimal 并关闭 GUI | feature selection 是产品决策 | — | `packages/local/common.nix` |
| `gnutar` | 🩹 本地 | ✅ | gnutar gnulib `xattr-at` 与静态 libacl 都定义 `*xattrat`，GCC 15 `-fno-common` 下链接冲突，需 `-Wl,--allow-multiple-definition` | stock unstable 无 flag 也能静态链接并保留 ACL/xattr | 624af665418d | `packages/gnutar/` |
| `gpgme` | 🩹 本地 | 🟡 | 实测均不可回归：stock 完整 gnupg 依赖树拖入 openldap 报 `Could not locate Cyrus SASL`，minimalGnuPG 必须保留；保留 minimal 后恢复 checks 又因静态 gpg-agent 无法启动报 `gpg: failed to start gpg-agent`，`--disable-gpg-test`+`doCheck=false` 保留 | 逐项恢复依赖与 checks，保持 musl-static | 624af665418d | `packages/gpgme/` |
| `libtool` | 📦 本地 | ❌ | 改写 `libtoolize` 的 baked data paths | 相对资源定位必须保留 | — | `packages/libtool/` |
| `makeself` | 📦 本地 | ❌ | wrapper 相对定位 header 资源 | 可搬运资源定位必须保留 | — | `packages/makeself/` |
| `markdownlint-cli2` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/markdownlint-cli2/` |
| `miniserve` | 📦 本地 | ❌ | wrapper 设置仓库要求的默认功能开关 | 产品行为必须保留 | — | `packages/miniserve/` |
| `music-decrypto` | ⚠️ glibc 动态 | 🟡 | Linux 仅长期审计 .NET AOT | Linux 出现 musl-static AOT | — | `packages/music-decrypto/` |
| `netron` | 📦 本地 | ❌ | wheel 重打包并绑定 sibling/宿主 Python | runtime packaging 必须保留 | — | `packages/netron/` |
| `nodejs-slim24` | 🩹 本地 | 🟡 | 已删 `uvwasi` override（stock 已传 `UVWASI_BUILD_SHARED=FALSE`，删后构建成功、musl 静态、`node --version` v24.18.0）；仍保留 `ada`/`libuv`/`hdrhistogram_c` 的 doCheck/SHARED-off 与 node 级 configureFlags | 逐 patch 验证删除，保留 Node 24 runtime 产品 | 624af665418d | `packages/nodejs/24/` |
| `nodejs-slim26` | 🩹 本地 | 🟡 | 已删 `uvwasi` override（同 node24，删后构建成功、musl 静态、v26.5.0）；仍保留 `ada`/`libuv`/`hdrhistogram_c`/`lief`（maturin musl cdylib 失败）/`temporal_capi`（pkg-config 缺失）与 node 级 configureFlags | 逐 patch 删除，最终满足各平台动态依赖规则 | 624af665418d | `packages/nodejs/26/` |
| `nsight-systems` | ⚠️ 预编译 glibc | ⏳ | NVIDIA 只提供 glibc 动态发行物 | 上游提供可用的 musl-static 发行物 | — | `packages/nsight-systems/` |
| `opencommit` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/opencommit/` |
| `openssh_gssapi` | 🩹 + 📦 本地 | ❌ | wrappers 相对定位 ssh 与 sshd helpers；服务端关闭预认证 sandbox，因为 QEMU user-mode 明确拒绝 guest seccomp，rlimit sandbox 也会让跨架构容器在 SSH 握手前断连；消费者必须把监听面限制在可信边界 | 可搬运 helper 定位与跨架构 SSH 必须保留 | — | `packages/openssh_gssapi/` |
| `p7zip` | 🩹 本地 | 🟡 | 实测不可回归：去掉 `buildFlags=default`（回退上游 `all3`）后构建 `7z.so` 共享库，musl 全静态工具链报 `R_X86_64_32 against __TMC_END__ ... making a shared object`；default 只构建 `7za`/`7zr` 静态可执行；output 布局 packaging 保留 | stock build 可用时删 build workaround，保留所需 outputs | 624af665418d | `packages/p7zip/` |
| `parallel` | 📦 本地 | ❌ | 多入口 sibling Perl wrappers | runtime packaging 必须保留 | — | `packages/parallel/` |
| `patchelf` | 📌 `25.05` | ✅ | 历史 pin；已验证：unstable patchelf 0.15.2 `make check` 编译测试用 `.so` 时报 `R_X86_64_32 against hidden symbol __TMC_END__`（musl-static crt 与 PIC 冲突），构建失败 | Linux 用 unstable 构建并满足 musl-static portability | 624af665418d | `manifests/default.nix` |
| `perl` | 🩹 + 📦 本地 | 🟡 | 实测无可回归项：注入 Compress::Raw::Lzma + IO::Compress::Brotli 的静态 XS 必需，stock perl `require` 直接 `Can't locate Compress/Raw/Lzma.pm`（unstable 未 vendor 这两个模块）；wrapper 保留 | 只删除 stock 已覆盖的依赖/link patch | 624af665418d | `packages/perl/` |
| `pnpm` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/pnpm/` |
| `podman` | 🩹 + 📦 本地 | 🟡 | 实测不可回归：改用 stock runc 后 helpersBin 复制来的 `runc` 是上游 `wrapProgram` 的动态 launcher（`interpreter /nix/.../ld-musl`+`/nix` rpath），normalize 报 dynamically linked，`runcStatic` 必须保留；helper bundling 保留。可搬运化：helper、conmon/conmonrs 和 OCI runtime 路径使用 Podman 原生 `$BINDIR` token，并把 executable 查找限制在 sibling `libexec/podman`；`bin/podman` 和 `bin/podman-server` 按自身位置设置配置路径、`PODMAN_DATA_DIR` 和 `TMPDIR`，storage.conf 通过环境变量展开 sibling `data` 下的 graphroot/runroot；已删除固定 `/opt/podmanx` 和 `adminOverrideConfigPath`。service 仅用 `@PODMANX_ROOT@` 模板，install.sh 原地注册当前包，不支持 prefix，也不复制包目录。新初始化的数据跟随包路径；Podman state 会记录绝对路径，已有 data 初始化后不支持整体移动 | 上游 helpers 复制真实静态 runc 后只删 runcStatic workaround；可搬运 packaging 必须保留 | 624af665418d | `packages/podman/` |
| `postgresql` | 🩹 + 📦 本地 | 🟡 | 实测无可回归项：`gccAsClang`（否则 generic.nix 切 clang 报 `C compiler cannot create executables`）与其绑定的去 `-flto` 必需；`curlSupport=false`（打开报 library 'curl' does not provide curl_multi_init）、`gssSupport=false`（gss_store_cred_into 缺失）必需；psql-only 产品边界保留 | 逐项删 workaround，保留 psql-only 输出 | 624af665418d | `packages/postgresql/` |
| `prettier` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/prettier/` |
| `protobuf3_20` | 📌 `24.05` | ❌ | unstable 已删除该版本：属性不存在，去 pin 后 `base.${name} or null` 静默产出空包，非有效回归 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf3_21` | 📌 `24.05` | ❌ | unstable 已把该属性改为 throwing alias（renamed to `protobuf_21`），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf_23` | 📌 `24.05` | ❌ | unstable 已删除该版本：属性不存在，去 pin 后静默产出空包，非有效回归 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf_24` | 📌 `25.05` | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf_26` | 📌 `25.05` | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf_28` | 📌 `25.05` | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | 624af665418d | `manifests/default.nix` |
| `protobuf_3_8_0` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.8.0 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_8_0/` |
| `protobuf_3_9_2` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.9.2 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_9_2/` |
| `python311` | 📦 本地 | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | — | `packages/python/` |
| `python312` | 📦 本地 | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | — | `packages/python/` |
| `python313` | 📦 本地 | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | — | `packages/python/` |
| `python314` | 📦 本地 | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | — | `packages/python/` |
| `python315` | 🩹 + 📦 本地 | 🟡 | 实测不可回归：去掉 `patchMuslStatx` 后 `nix build` 报 `struct statx has no member named stx_dio_offset_align; did you mean stx_dio_offet_align`，unstable 未修复 musl 头拼写与 CPython 3.15 的冲突；静态 modules packaging 保留 | 只删除字段替换，保留静态 modules | 624af665418d | `packages/python/` |
| `rime-plugins` | 📦 本地 | ❌ | 聚合多个 Rime 词库与转换结果 | 数据 bundle 是产品 | — | `packages/rime-plugins/` |
| `runc` | 📦 native selection | ❌ | Linux 容器运行时，依赖 namespaces/cgroups，无 macOS 构建目标 | 无 macOS 端可回归空间（平台固有） | — | `manifests/default.nix` |
| `s6` | 🩹 本地 | 🟡 | 实测不可回归：stock unstable s6 2.15.1.0 产物多数二进制文本 baked 自身 `/nix/store/.../bin`（如 s6-svscan 引用 s6-supervise），仍需 patch 去 baked prefix | stock 输出不再写入 Nix store 路径 | 624af665418d | `packages/s6/` |
| `s6-linux-init` | 🩹 本地 | 🟡 | 实测不可回归：stock unstable `s6-linux-init-maker` baked `#!/nix/store/...execlineb` 及 execline/s6 helper 绝对路径，会写进生成的 init 脚本，仍需 patch；额外的 out+bin symlinkJoin 属结构性 packaging | stock 产物与生成脚本无 Nix store 路径 | 624af665418d | `packages/s6-linux-init/` |
| `s6-rc` | 🩹 本地 | 🟡 | 实测不可回归：stock unstable `s6-rc-compile` baked `#!/nix/store/...execlineb -S0` 及 fdmove/s6-fdholder-retrieve 等绝对路径，会写进 compile 生成的 service scripts，仍需 patch | stock 产物与生成服务无 Nix store 路径 | 624af665418d | `packages/s6-rc/` |
| `tmux-plugins` | 📦 本地 | ❌ | 独立发布 `.tmux.conf` 数据 | 数据 bundle 是产品 | — | `packages/tmux-plugins/` |
| `vim` | 📦 本地 | ❌ | wrapper 相对设置 `VIMRUNTIME` | 可搬运 runtime 定位必须保留 | — | `packages/vim/` |
| `vim-plugins` | 📦 本地 | ❌ | 聚合固定 Vim plugins | plugin bundle 是产品 | — | `packages/vim-plugins/` |
| `wget` | 🩹 + 📦 本地 | 🟡 | 实测无可回归项：恢复 checks 后 `wget_options_fuzzer` 段错误（exit 139）且缺 fuzzer corpus，`doCheck=false` 必需；CA wrapper packaging 保留 | 恢复 checks/build tool 后保留 CA packaging | 624af665418d | `packages/wget/` |
| `zellij` | 🩹 checks | 🟡 | 已去掉 `26.05` pin，改用 unstable `zellij-unwrapped`（0.44.3，static-pie musl 达标）；仍保留 `doCheck=false`/`doInstallCheck=false`（test target 静态链 libcurl 时 libssh2 符号未解析：`undefined reference to libssh2_crypto_engine` 等） | 上游 test target 静态链接修复后恢复 checks | 624af665418d | `packages/zellij/`, `packages/local/linux.nix` |
| `zsh` | 🩹 + 📦 本地 | 🟡 | GCC fortify ICE 与静态 module patches；FPATH wrapper 和 zshenv policy 必须保留 | 逐项删编译 patch，保留 relocation packaging | — | `packages/zsh/` |
| `zsh-plugins` | 📦 本地 | ❌ | 聚合 oh-my-zsh 与 plugins | plugin bundle 是产品 | — | `packages/zsh-plugins/` |

## macOS

| 包 | 定制 | 回归 | 原因与保留边界 | 回归判据 | commit | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `aria2` | 📌 `24.11` | ❌ | 已验证：unstable aria2 1.37.0 静态 darwin 构建链接 `libxml2.a` 时缺 `iconv`/`iconv_open`/`libiconv` 符号，链接失败 | 已确认必要，两平台都无可回归空间 | 624af665418d | `manifests/default.nix` |
| `autoconf` | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | — | `packages/autoconf/` |
| `automake` | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | — | `packages/automake/` |
| `cloc` | 📦 本地 | 🟡 | sibling Perl wrapper 与模块 bundling 必须保留；install check 被禁用，darwin 未验证 | 只恢复可运行的 install check | — | `packages/cloc/` |
| `colima` | 🩹 本地 | 🟡 | darwin-only；只打 colima 本体，运行时依赖（lima/qemu/docker）按本仓库模型单独安装，故删掉 stock `wrapProgram`（会把 lima-full/qemu/docker 的 `/nix/store` 路径 baked 进 PATH，违反不变量 #1），改由用户 PATH 解析；CGO net resolver 拉入 nix-store libresolv stub，postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`；shell completion 保留 | 上游提供不 baked store 路径的运行时依赖定位、且 CGO 构建只链系统 libresolv 后删除 override | 624af665418d | `packages/colima/` |
| `curl` | 📦 本地 | ❌ | 内置 CA bundle 与相对路径 wrapper | 自包含证书定位是 packaging | — | `packages/curl/` |
| `docker-buildx` | 🩹 本地 | ✅ | 仅 darwin 有定制（Linux 走零定制 manifest pkgsStatic 全静态）；darwin 上 stock `pkgsStatic` 构建 Go toolchain 时因缺静态 libresolv 失败，native stock binary 的唯一非系统动态依赖是 Nix libresolv，本地 override 改指 macOS 系统库 | stock native 只链接系统 dylib，或 `pkgsStatic` 可直接构建后删除 override | 624af665418d | `packages/docker-buildx/` |
| `docker-compose` | 📦 native selection | ✅ | 仅 darwin 有定制（Linux 走零定制 manifest pkgsStatic 全静态）；darwin 上 stock `pkgsStatic` 构建 Go toolchain 时因缺静态 libresolv 失败，native stock binary 已只链接 macOS 系统 dylib，故 manifest 选择 `isStatic = false` | `pkgsStatic` 可直接构建并满足 macOS portability 后恢复默认选择 | 624af665418d | `manifests/default.nix` |
| `exiftool` | 📦 本地 | 🟡 | sibling Perl 与压缩模块 bundling 必须保留；install checks 被禁用，darwin 未验证 | install check 与 wrapper packaging 保留，仅在上游可运行 install check 时恢复 | — | `packages/exiftool/` |
| `eza-ls` | 📦 本地 | ❌ | 自定义 `ls` 兼容层与 bundled eza | 这是独立产品行为，不是上游 bug | — | `packages/eza-ls/` |
| `ffmpeg` | 🩹 本地 | 🟡 | 关闭无法静态化的 codec/network 链，修 x265 静态归档 | 逐 feature 恢复，最终只依赖系统 dylib | — | `packages/ffmpeg/` |
| `file` | 📦 本地 | ❌ | wrapper 相对定位 `magic.mgc` | 可搬运资源定位必须保留 | — | `packages/file/` |
| `gdb` | 📌 `25.11` | ❌ | 已验证：unstable gdb 17.2 的 `dejagnu → expect` 静态 darwin 构建同样缺 `tclStubsPtr`/`tclIntStubsPtr`/`tclStubsPtr`（arm64 symbol not found），gdb 无法构建 | 已确认两平台都必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `git-filter-repo` | 📦 本地 | ❌ | Python sibling runtime；macOS 暂用宿主 Python | runtime packaging 不会因上游构建修复消失 | — | `packages/git-filter-repo/` |
| `gnupg` | 📦 override | ❌ | 明确启用 minimal 并关闭 GUI | feature selection 是产品决策 | — | `packages/local/common.nix` |
| `golangci-lint` | 🩹 本地 | 🟡 | native Go 构建的 CGO net resolver 拉入 nix-store libresolv stub；postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`，standalone 产物只链系统 dylib | 上游 CGO 构建直接链接系统 libresolv 后删除 override | 624af665418d | `packages/golangci-lint/` |
| `gost` | 📦 native + CI-only | ❌ | macOS 作为受支持平台使用 native `pkgs`；本机受 EDR 实时防护影响，构建在 `go tool buildid -w` 报 `operation not permitted`，只在 GitHub Actions 构建和验证 | CI 构建约束是当前环境边界，不视为包不支持 macOS | 624af665418d | `manifests/default.nix` |
| `krb5` | 🩹 本地 | ❌ | 实测不可回归：去掉 override 后 stock unstable krb5 1.22.2 静态 darwin 构建时 `krb5kdc`/consumer 链接报 `_cc_initialize`（CCAPI 仅 `-framework Kerberos` 提供）与 `_krb5int_c_mit_des_zeroblock`（f_aead.o 未被静态 ld 拉入）两处 undefined symbol；禁用 CCAPI 与移动 DES const 的 patch 必须保留 | 上游修复 CCAPI 依赖与 DES const 静态可见性后删除 patch | 624af665418d | `packages/krb5/` |
| `lark-cli` | 📦 native selection | ❌ | manifest 在 macOS 选择 unstable native pkgs；关闭 CGO 反而产生 disallowed reference | 当前没有 pin 或 patch 可回归 | — | `manifests/default.nix` |
| `libtool` | 📦 本地 | ❌ | 改写 `libtoolize` 的 baked data paths | 相对资源定位必须保留 | — | `packages/libtool/` |
| `lima` | 🩹 本地 | 🟡 | darwin-only；只打宿主 limactl + `*.lima` helper + 随包 guest agents/templates，运行时依赖按本仓库模型单独安装，故删掉 stock `wrapProgram`（会把 qemu 的 `/nix/store` 路径 baked 进 PATH，违反不变量 #1，darwin 默认走 VZ 无需 qemu），改由用户 PATH 解析；三个 Mach-O（`limactl`/`limactl-mcp`/`lima-driver-krunkit`）的 CGO net resolver 拉入 nix-store libresolv stub，postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`；`limactl` 带 `com.apple.security.virtualization` entitlement（VZ 后端必需，也是上游 darwin `dontStrip` 的原因），rewrite 会失效签名，故用源码 `vz.entitlements` adhoc 重签，helper 用普通 adhoc 重签 | 上游提供不 baked store 路径的 qemu 定位、且 CGO 构建只链系统 libresolv 后删除 override | 624af665418d | `packages/lima/` |
| `makeself` | 📦 本地 | ❌ | wrapper 相对定位 header 资源 | 可搬运资源定位必须保留 | — | `packages/makeself/` |
| `markdownlint-cli2` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/markdownlint-cli2/` |
| `music-decrypto` | 🩹 ICU 路径 | 🟡 | macOS 可回归系统 ICU patch | macOS stock 仅用系统 dylib | — | `packages/music-decrypto/` |
| `netron` | 📦 本地 | ❌ | wheel 重打包并绑定 sibling/宿主 Python | runtime packaging 必须保留 | — | `packages/netron/` |
| `nodejs-slim26` | 🩹 本地 | 🟡 | 修 static deps、LIEF/Temporal、system libs 和 checks；macOS 注入 build tools，darwin 未验证 | 逐 patch 删除，最终满足各平台动态依赖规则 | — | `packages/nodejs/26/` |
| `opencommit` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/opencommit/` |
| `p7zip` | 🩹 本地 | 🟡 | 强制 default build flags；output 布局为 packaging，darwin 未验证 | stock build 可用时删 build workaround，保留所需 outputs | — | `packages/p7zip/` |
| `parallel` | 📦 本地 | ❌ | 多入口 sibling Perl wrappers | runtime packaging 必须保留 | — | `packages/parallel/` |
| `perl` | 🩹 + 📦 本地 | 🟡 | macOS 静态替换与 install-name relocation；wrapper 必须保留，darwin 未验证 | 只删除 stock 已覆盖的依赖/link patch | — | `packages/perl/` |
| `pnpm` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/pnpm/` |
| `prettier` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/prettier/` |
| `protobuf_3_8_0` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.8.0 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_8_0/` |
| `protobuf_3_9_2` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.9.2 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_9_2/` |
| `rime-plugins` | 📦 本地 | ❌ | 聚合多个 Rime 词库与转换结果 | 数据 bundle 是产品 | — | `packages/rime-plugins/` |
| `shellcheck` | 📌 `25.11` | ❌ | 已验证：unstable ShellCheck 0.11.0 静态 darwin 构建时 GHC 报 `External interpreter terminated (1)`，构建失败 | 已确认必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `silver-searcher` | 📌 `26.05` | ❌ | 已验证：unstable 已 removed `silver-searcher`（自 2020 停止开发、卡在 obsolete pcre），eval 直接 throw | 上游已移除，只能保持老 channel pin | 624af665418d | `manifests/default.nix` |
| `tmux-plugins` | 📦 本地 | ❌ | 独立发布 `.tmux.conf` 数据 | 数据 bundle 是产品 | — | `packages/tmux-plugins/` |
| `uv` | 📌 `25.11` | ❌ | 已验证：unstable uv 0.11.32 静态 darwin 构建时 `aws-lc-sys` 的 `memcmp_invalid_stripped_check` 用 `--target arm64-apple-macosx` 触发 cc-wrapper 多 target 缺陷（`posix_spawn failed`），构建失败 | 已确认必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `vim` | 📦 本地 | ❌ | wrapper 相对设置 `VIMRUNTIME` | 可搬运 runtime 定位必须保留 | — | `packages/vim/` |
| `vim-plugins` | 📦 本地 | ❌ | 聚合固定 Vim plugins | plugin bundle 是产品 | — | `packages/vim-plugins/` |
| `wget` | 🩹 + 📦 本地 | 🟡 | macOS 绕过 static Perl；CA wrapper 必须保留，darwin 未验证 | 恢复 checks/build tool 后保留 CA packaging | — | `packages/wget/` |
| `zsh` | 🩹 + 📦 本地 | 🟡 | 静态 module patches；FPATH wrapper 和 zshenv policy 必须保留 | 逐项删编译 patch，保留 relocation packaging | — | `packages/zsh/` |
| `zsh-plugins` | 📦 本地 | ❌ | 聚合 oh-my-zsh 与 plugins | plugin bundle 是产品 | — | `packages/zsh-plugins/` |
