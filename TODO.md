# 上游回归清单

本表是人和 agent 共用的 pin、patch 与本地 packaging 总账，也是批量回归的唯一输入。

状态含义：

- ✅ **整项候选**：优先尝试直接回到 unstable 上游；标记只表示值得验证，不表示已经构建成功。
- 🟡 **部分候选**：只回归临时 workaround，必须保留表中写明的 packaging 或产品行为。
- ❌ **结构性保留**：当前不是上游回归目标；用于说明本地包为何存在，避免误删。
- ⏳ **长期审计**：动态或预编译例外；只有出现满足仓库产物不变量的替代方案时才处理。

平台列含义：📌 表示旧 nixpkgs pin，🩹 表示编译或 portability patch，📦 表示结构性 packaging，
⚠️ 表示动态例外，⏸️ 表示临时停用，— 表示该平台无此定制。

批量回归按表格顺序遍历 `✅` 和 `🟡` 行：

```bash
rg '^\| .+ \| (✅|🟡)' TODO.md
```

回归成功后，整项回归删除该行；部分回归更新原因与判据，只保留尚未解决的部分。新增或改变非
unstable pin、本地 derivation、override、禁用检查或动态例外时，必须同步维护本表。

| 包 | Linux | macOS | 回归 | 原因与保留边界 | 回归判据 | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `aria2` | — | 📌 `24.11` | ✅ | 历史 pin，原始失败未记录 | unstable 产物只依赖系统 dylib | `manifests/default.nix` |
| `autoconf` | 📦 本地 | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | `packages/autoconf/` |
| `automake` | 📦 本地 | 📦 本地 | ❌ | 相对路径 wrappers 定位配套脚本 | 上游入口无需 Nix store 路径时再评估 | `packages/automake/` |
| `cacert` | 📦 本地 | 📦 本地 | ❌ | 独立发布 curl CA bundle | 这是仓库产品，不随 nixpkgs 构建修复消失 | `packages/cacert/` |
| `catatonit` | 🩹 本地 | — | ✅ | 清空 stock `installCheckPhase`：上游 check 跑 `readelf` 但未把 binutils 加入 `nativeBuildInputs`，musl64 cross `strictDeps` 构建下报 `readelf: command not found` | 上游把 binutils 加入 `nativeBuildInputs`（或改用可用 check）后，unstable install check 与最终静态验证通过 | `packages/catatonit/` |
| `clang-tools-18` | 📦 本地 | — | ❌ | 固定 LLVM 18，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | `packages/clang-tools/` |
| `clang-tools-19` | 📦 本地 | — | ❌ | 固定 LLVM 19，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | `packages/clang-tools/` |
| `clang-tools-20` | 📦 本地 | — | ❌ | 固定 LLVM 20，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | `packages/clang-tools/` |
| `clang-tools-21` | 📦 本地 | — | ❌ | 固定 LLVM 21，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | `packages/clang-tools/` |
| `clang-tools-22` | 📦 本地 | — | ❌ | 固定 LLVM 22，只提取并瘦身 `clang-format` | 多版本单工具发布是产品决策 | `packages/clang-tools/` |
| `cloc` | 📦 本地 | 📦 本地 | 🟡 | sibling Perl wrapper 与模块 bundling 必须保留；install check 被禁用 | 只恢复可运行的 install check | `packages/cloc/` |
| `cmake` | 🩹 本地 | — | 🟡 | 强制静态 bootstrap、内置依赖并禁用 checks | 逐项移除 stock 已不需要的 flags；保持 musl-static | `packages/cmake/default/` |
| `cmake_3_27_9` | 📌 源码版本 + 🩹 | — | 🟡 | 固定 3.27.9；补 C++ header、cross bootstrap 并禁用 checks | 只回归构建补丁与 checks，保留版本化 output | `packages/cmake/3_27_9/` |
| `cmake_4_1_2` | 📌 源码版本 + 🩹 | — | 🟡 | 固定 4.1.2；自定义 cross bootstrap 并禁用 checks | 只回归构建补丁与 checks，保留版本化 output | `packages/cmake/4_1_2/` |
| `conmon` | 🩹 本地 | — | ✅ | 收窄 build inputs 并清空 propagated inputs：stock unstable 的 propagatedBuildInputs 拉入 `systemd-minimal`，其 `badPlatforms` 含 `isStatic`，musl-static 下 eval 即被拒 | stock unstable 无需清空 propagated inputs 即可构建为 musl-static | `packages/conmon/` |
| `crun` | 🩹 本地 | — | 🟡 | 强制全静态、关闭不可用 features 并禁用 checks | 逐项恢复 stock features/checks，保持 musl-static | `packages/crun/` |
| `curl` | 📦 本地 | 📦 本地 | ❌ | 内置 CA bundle 与相对路径 wrapper | 自包含证书定位是 packaging | `packages/curl/` |
| `diffutils` | 🩹 本地 | 🩹 本地 | ✅ | 仅禁用 stock checks：unstable diffutils 3.12 的 gnulib checkPhase 在 musl-static 下 9 个多线程/setlocale 测试失败（`test-setlocale_null-mt`、`test-thread_create` 等 SIGABRT） | unstable 全量 checks 与 portability 验证通过 | `packages/diffutils/` |
| `dive` | 📌 `25.11` | — | ✅ | Linux 仍 pin `25.11`；macOS 已回归到 unstable native | Linux 用 unstable 并满足 musl-static portability | `manifests/default.nix` |
| `dool` | 📦 本地 | — | ❌ | Python sibling runtime wrapper，并默认追加 `--bytes` | runtime 与产品默认行为必须保留 | `packages/dool/` |
| `execline` | 🩹 本地 | — | 🟡 | 去掉 baked Nix prefixes，改用 PATH 与 portable shebang | stock 输出不再写入 Nix store 路径 | `packages/execline/` |
| `exiftool` | 📦 本地 | 📦 本地 | 🟡 | sibling Perl 与压缩模块 bundling 必须保留；install checks 被禁用 | 只恢复 checks 或删除已不需的依赖 patch | `packages/exiftool/` |
| `eza-ls` | 📦 本地 | 📦 本地 | ❌ | 自定义 `ls` 兼容层与 bundled eza | 这是独立产品行为，不是上游 bug | `packages/eza-ls/` |
| `ffmpeg` | — | 🩹 本地 | 🟡 | 关闭无法静态化的 codec/network 链，修 x265 静态归档 | 逐 feature 恢复，最终只依赖系统 dylib | `packages/ffmpeg/` |
| `file` | 📦 本地 | 📦 本地 | ❌ | wrapper 相对定位 `magic.mgc` | 可搬运资源定位必须保留 | `packages/file/` |
| `gdb` | 📌 `25.11` | 📌 `25.11` | ✅ | 历史 pin；Linux 已验证：unstable gdb 17.2 的构建依赖 `dejagnu → expect` 在 musl-static 下链接失败（`undefined reference to tclStubsPtr`），连带 gdb 无法构建 | unstable 在两平台构建并满足 portability | `manifests/default.nix` |
| `gettext` | 🩹 本地 | 🩹 本地 | ✅ | 强制使用 bundled gettext/libintl；Linux 已验证 stock unstable 静态可搬运（可删 override），darwin 未验证 | 两平台 stock unstable 静态构建可搬运时删除 override | `packages/gettext/` |
| `git` | 🩹 本地 | — | 🟡 | 静态传递依赖、`error` 符号冲突和 install check workaround；相对资源 wrapper 必须保留 | 逐项删除构建 workaround，保留 wrapper | `packages/git/` |
| `git-filter-repo` | 📦 本地 | 📦 本地 | ❌ | Python sibling runtime；macOS 暂用宿主 Python | runtime packaging 不会因上游构建修复消失 | `packages/git-filter-repo/` |
| `git-lfs` | 📌 `25.11` | — | ✅ | Linux 仍 pin `25.11`；macOS 已回归到 unstable native | Linux 用 unstable 并满足 musl-static portability | `manifests/default.nix` |
| `glibcLocales` | 📦 override | — | ❌ | 只发布裁剪后的 locale 数据 | 输出裁剪是产品决策 | `packages/local/linux.nix` |
| `gnupg` | 📦 override | 📦 override | ❌ | 明确启用 minimal 并关闭 GUI | feature selection 是产品决策 | `packages/local/common.nix` |
| `gnutar` | 🩹 本地 | — | ✅ | gnutar gnulib `xattr-at` 与静态 libacl 都定义 `*xattrat`，GCC 15 `-fno-common` 下链接冲突，需 `-Wl,--allow-multiple-definition` | stock unstable 无 flag 也能静态链接并保留 ACL/xattr | `packages/gnutar/` |
| `gpgme` | 🩹 本地 | — | 🟡 | minimal GnuPG，禁用 gpg test 与 checks | 逐项恢复依赖与 checks，保持 musl-static | `packages/gpgme/` |
| `krb5` | — | 🩹 本地 | ✅ | 禁用 CCAPI 并移动 DES const，使静态归档可链接 | stock unstable 静态归档可链接且仅依赖系统 dylib | `packages/krb5/` |
| `lark-cli` | — | 📦 native selection | ❌ | manifest 在 macOS 选择 unstable native pkgs；关闭 CGO 反而产生 disallowed reference | 当前没有 pin 或 patch 可回归 | `manifests/default.nix` |
| `libtool` | 📦 本地 | 📦 本地 | ❌ | 改写 `libtoolize` 的 baked data paths | 相对资源定位必须保留 | `packages/libtool/` |
| `makeself` | 📦 本地 | 📦 本地 | ❌ | wrapper 相对定位 header 资源 | 可搬运资源定位必须保留 | `packages/makeself/` |
| `markdownlint-cli2` | 📦 本地 | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | `packages/markdownlint-cli2/` |
| `miniserve` | 📦 本地 | — | ❌ | wrapper 设置仓库要求的默认功能开关 | 产品行为必须保留 | `packages/miniserve/` |
| `music-decrypto` | ⚠️ glibc 动态 | 🩹 ICU 路径 | 🟡 | Linux 仅长期审计 .NET AOT；macOS 可回归系统 ICU patch | Linux 出现 musl-static AOT；macOS stock 仅用系统 dylib | `packages/music-decrypto/` |
| `netron` | 📦 本地 | 📦 本地 | ❌ | wheel 重打包并绑定 sibling/宿主 Python | runtime packaging 必须保留 | `packages/netron/` |
| `nodejs-slim24` | 🩹 本地 | — | 🟡 | 修 ada/libuv checks、shared-only deps、system libs 与 addon checks | 逐 patch 验证删除，保留 Node 24 runtime 产品 | `packages/nodejs/24/` |
| `nodejs-slim26` | 🩹 本地 | 🩹 本地 | 🟡 | 修 static deps、LIEF/Temporal、system libs 和 checks；macOS 注入 build tools | 逐 patch 删除，最终满足各平台动态依赖规则 | `packages/nodejs/26/` |
| `nsight-systems` | ⚠️ 预编译 glibc | — | ⏳ | NVIDIA 只提供 glibc 动态发行物 | 上游提供可用的 musl-static 发行物 | `packages/nsight-systems/` |
| `opencommit` | 📦 本地 | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | `packages/opencommit/` |
| `openssh_gssapi` | 📦 本地 | — | ❌ | wrappers 相对定位 ssh 与 sshd helpers | 可搬运 helper 定位必须保留 | `packages/openssh_gssapi/` |
| `p7zip` | 🩹 本地 | 🩹 本地 | 🟡 | 强制 default build flags；output 布局为 packaging | stock build 可用时删 build workaround，保留所需 outputs | `packages/p7zip/` |
| `parallel` | 📦 本地 | 📦 本地 | ❌ | 多入口 sibling Perl wrappers | runtime packaging 必须保留 | `packages/parallel/` |
| `patchelf` | 📌 `25.05` | 📌 `25.05` | ✅ | 历史 pin；Linux 已验证：unstable patchelf 0.15.2 `make check` 编译测试用 `.so` 时报 `R_X86_64_32 against hidden symbol __TMC_END__`（musl-static crt 与 PIC 冲突），构建失败 | unstable 在两平台构建并满足 portability | `manifests/default.nix` |
| `perl` | 🩹 + 📦 本地 | 🩹 + 📦 本地 | 🟡 | Linux 静态内建 XS；macOS 静态替换与 install-name relocation；wrapper 必须保留 | 只删除 stock 已覆盖的依赖/link patch | `packages/perl/` |
| `pnpm` | 📦 本地 | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | `packages/pnpm/` |
| `podman` | 🩹 + 📦 本地 | — | 🟡 | `runcStatic` 绕过动态 launcher；路径与 helper bundling 必须保留 | 上游 helpers 复制真实静态 runc 后只删该 workaround | `packages/podman/` |
| `postgresql` | 🩹 + 📦 本地 | — | 🟡 | gcc/clang/LTO 与静态 feature workarounds；psql-only 是产品边界 | 逐项删 workaround，保留 psql-only 输出 | `packages/postgresql/` |
| `prettier` | 📦 本地 | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | `packages/prettier/` |
| `protobuf3_20` | 📌 `24.05` | — | ❌ | unstable 已删除该版本：属性不存在，去 pin 后 `base.${name} or null` 静默产出空包，非有效回归 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf3_21` | 📌 `24.05` | — | ❌ | unstable 已把该属性改为 throwing alias（renamed to `protobuf_21`），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf_23` | 📌 `24.05` | — | ❌ | unstable 已删除该版本：属性不存在，去 pin 后静默产出空包，非有效回归 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf_24` | 📌 `25.05` | — | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf_26` | 📌 `25.05` | — | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf_28` | 📌 `25.05` | — | ❌ | unstable 已 removed 该版本（throwing alias），去 pin 后 eval 报错 | 只能改指 unstable 现存版本别名（改变版本语义），不属去 pin 回归 | `manifests/default.nix` |
| `protobuf_3_8_0` | 📌 源码版本 | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.8.0 | 版本化产品，不回到最新 upstream | `packages/protobuf/3_8_0/` |
| `protobuf_3_9_2` | 📌 源码版本 | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.9.2 | 版本化产品，不回到最新 upstream | `packages/protobuf/3_9_2/` |
| `python311` | 📦 本地 | — | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | `packages/python/` |
| `python312` | 📦 本地 | — | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | `packages/python/` |
| `python313` | 📦 本地 | — | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | `packages/python/` |
| `python314` | 📦 本地 | — | ❌ | 静态 CPython 与内建扩展模块 | 多版本静态 runtime 是产品决策 | `packages/python/` |
| `python315` | 🩹 + 📦 本地 | — | 🟡 | 保留静态 runtime；临时修正 musl `statx` 字段拼写 | 只删除字段替换，保留静态 modules | `packages/python/` |
| `rime-plugins` | 📦 本地 | 📦 本地 | ❌ | 聚合多个 Rime 词库与转换结果 | 数据 bundle 是产品 | `packages/rime-plugins/` |
| `s6` | 🩹 本地 | — | 🟡 | 去掉 baked Nix prefixes 并接入 patched execline | stock 输出不再写入 Nix store 路径 | `packages/s6/` |
| `s6-linux-init` | 🩹 本地 | — | 🟡 | 去掉 baked prefixes 并合并 outputs | stock 产物与生成脚本无 Nix store 路径 | `packages/s6-linux-init/` |
| `s6-rc` | 🩹 本地 | — | 🟡 | 去掉自身和依赖 baked prefixes | stock 产物与生成服务无 Nix store 路径 | `packages/s6-rc/` |
| `shellcheck` | — | 📌 `25.11` | ✅ | 历史 pin，原始失败未记录 | unstable 产物只依赖系统 dylib | `manifests/default.nix` |
| `silver-searcher` | — | 📌 `26.05` | ✅ | 历史 pin，原始失败未记录 | unstable 产物只依赖系统 dylib | `manifests/default.nix` |
| `tmux-plugins` | 📦 本地 | 📦 本地 | ❌ | 独立发布 `.tmux.conf` 数据 | 数据 bundle 是产品 | `packages/tmux-plugins/` |
| `uv` | — | 📌 `25.11` | ✅ | 历史 pin，原始失败未记录 | unstable 产物只依赖系统 dylib | `manifests/default.nix` |
| `vim` | 📦 本地 | 📦 本地 | ❌ | wrapper 相对设置 `VIMRUNTIME` | 可搬运 runtime 定位必须保留 | `packages/vim/` |
| `vim-plugins` | 📦 本地 | 📦 本地 | ❌ | 聚合固定 Vim plugins | plugin bundle 是产品 | `packages/vim-plugins/` |
| `wget` | 🩹 + 📦 本地 | 🩹 + 📦 本地 | 🟡 | Linux 禁用 checks；macOS 绕过 static Perl；CA wrapper 必须保留 | 恢复 checks/build tool 后保留 CA packaging | `packages/wget/` |
| `zellij` | 📌 `26.05` + 🩹 checks | — | ✅ | test target 静态链接 libcurl/libssh2 失败 | unstable `zellij-unwrapped` 构建、checks 与 portability 通过 | `packages/zellij/`, `packages/local/linux.nix` |
| `zsh` | 🩹 + 📦 本地 | 🩹 + 📦 本地 | 🟡 | GCC fortify ICE 与静态 module patches；FPATH wrapper 和 zshenv policy 必须保留 | 逐项删编译 patch，保留 relocation packaging | `packages/zsh/` |
| `zsh-plugins` | 📦 本地 | 📦 本地 | ❌ | 聚合 oh-my-zsh 与 plugins | plugin bundle 是产品 | `packages/zsh-plugins/` |
