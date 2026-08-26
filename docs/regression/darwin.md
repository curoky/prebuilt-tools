# macOS 回归表

适用于 aarch64-darwin，只列该平台有定制的包。表格约定、状态/定制图例与批量回归命令见
[`AGENTS.md`](AGENTS.md)。

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
| `libarchive` | 🩹 本地 + ⏸️ 停用 darwin | 🟡 | 本地 override 在 macOS 上编译失败，暂时仅接入 Linux；Linux 仍需关闭 XAR/libxml2，并把静态 OpenSSL 的配置、engine 与 module 默认目录改为系统路径 | macOS 构建修复，且四个 CLI 均不内嵌 Nix store 路径、Mach-O 只依赖系统库并通过 TAR/ZIP/AES smoke test | — | `packages/local/linux/common.nix`, `packages/libarchive/` |
| `libtool` | 📦 本地 | ❌ | 改写 `libtoolize` 的 baked data paths | 相对资源定位必须保留 | — | `packages/libtool/` |
| `lima` | 🩹 本地 | 🟡 | darwin-only；只打宿主 limactl + `*.lima` helper + 随包 guest agents/templates，运行时依赖按本仓库模型单独安装，故删掉 stock `wrapProgram`（会把 qemu 的 `/nix/store` 路径 baked 进 PATH，违反不变量 #1，darwin 默认走 VZ 无需 qemu），改由用户 PATH 解析；三个 Mach-O（`limactl`/`limactl-mcp`/`lima-driver-krunkit`）的 CGO net resolver 拉入 nix-store libresolv stub，postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`；`limactl` 带 `com.apple.security.virtualization` entitlement（VZ 后端必需，也是上游 darwin `dontStrip` 的原因），rewrite 会失效签名，故用源码 `vz.entitlements` adhoc 重签，helper 用普通 adhoc 重签 | 上游提供不 baked store 路径的 qemu 定位、且 CGO 构建只链系统 libresolv 后删除 override | 624af665418d | `packages/lima/` |
| `makeself` | 📦 本地 | ❌ | wrapper 相对定位 header 资源 | 可搬运资源定位必须保留 | — | `packages/makeself/` |
| `markdownlint-cli2` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/markdownlint-cli2/` |
| `music-decrypto` | 🩹 ICU 路径 | 🟡 | macOS 可回归系统 ICU patch | macOS stock 仅用系统 dylib | — | `packages/music-decrypto/` |
| `netron` | 📦 本地 | ❌ | wheel 重打包并绑定 sibling/宿主 Python | runtime packaging 必须保留 | — | `packages/netron/` |
| `nixfmt` | ⏸️ 停用 darwin | 🟡 | stock `pkgsStatic` 构建在 macOS 上编译失败，暂时仅接入 Linux（两平台均走零定制 manifest pkgsStatic）| macOS 构建修复后恢复 `aarch64-darwin` | — | `manifests/default.nix` |
| `nodejs-slim26` | 🩹 本地 | 🟡 | 修 static deps、LIEF/Temporal、system libs 和 checks；macOS 注入 build tools，darwin 未验证 | 逐 patch 删除，最终满足各平台动态依赖规则 | — | `packages/nodejs/26/` |
| `opencommit` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/opencommit/` |
| `p7zip` | 🩹 本地 | 🟡 | 强制 default build flags；output 布局为 packaging，darwin 未验证 | stock build 可用时删 build workaround，保留所需 outputs | — | `packages/p7zip/` |
| `pkgconf` | 🩹 本地 | ✅ | stock `pkgconf-unwrapped` 把自身 Nix output 的 `.pc`、system lib/include 与 personality 路径编译进二进制；本地 override 改用标准 `/usr` 与 `/usr/local` 路径，避免 standalone 产物残留 `/nix/store` | stock 二进制不再编译进 Nix store 路径且只依赖系统 dylib | — | `packages/pkgconf/` |
| `parallel` | 📦 本地 | ❌ | 多入口 sibling Perl wrappers | runtime packaging 必须保留 | — | `packages/parallel/` |
| `perl` | 🩹 + 📦 本地 | 🟡 | macOS 静态替换与 install-name relocation；wrapper 必须保留，darwin 未验证 | 只删除 stock 已覆盖的依赖/link patch | — | `packages/perl/` |
| `pnpm` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/pnpm/` |
| `prettier` | 📦 本地 | ❌ | JS 分发绑定 sibling Node runtime | sibling runtime packaging 必须保留 | — | `packages/prettier/` |
| `protobuf_3_8_0` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.8.0 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_8_0/` |
| `protobuf_3_9_2` | 📌 源码版本 | ❌ | 明确发布 legacy protobuf 3.9.2 | 版本化产品，不回到最新 upstream | — | `packages/protobuf/3_9_2/` |
| `rclone` | 🩹 本地 | 🟡 | 仅 darwin 有定制（Linux 走零定制 manifest pkgsStatic 全静态）；native Go 构建的 CGO net resolver 拉入 nix-store libresolv stub，postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`，并用 `nuke-refs` 清理 Go toolchain 内嵌的 tzdata/mailcap/iana-etc fallback store hash；standalone 产物只链系统 dylib且无真实 store 路径 | stock native 只链接系统 dylib且不内嵌真实 store 路径，或 `pkgsStatic` 可直接构建并满足 macOS portability 后删除 override | b7c2ada94fe9 | `packages/rclone/` |
| `rime-plugins` | 📦 本地 | ❌ | 聚合多个 Rime 词库与转换结果 | 数据 bundle 是产品 | — | `packages/rime-plugins/` |
| `shellcheck` | 📌 `25.11` | ❌ | 已验证：unstable ShellCheck 0.11.0 静态 darwin 构建时 GHC 报 `External interpreter terminated (1)`，构建失败 | 已确认必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `supercronic` | 🩹 本地 | 🟡 | 仅 darwin 有定制（Linux 走零定制 manifest pkgsStatic 全静态）；native Go 构建的 CGO net resolver 拉入 nix-store libresolv stub，postInstall 用 `install_name_tool` 改指 `/usr/lib/libresolv.9.dylib`，standalone 产物只链系统 dylib | 上游 CGO 构建直接链接系统 libresolv，或 `pkgsStatic` 可直接构建后删除 override | — | `packages/supercronic/` |
| `tmux-plugins` | 📦 本地 | ❌ | 独立发布 `.tmux.conf` 数据 | 数据 bundle 是产品 | — | `packages/tmux-plugins/` |
| `uv` | 📌 `25.11` | ❌ | 已验证：unstable uv 0.11.32 静态 darwin 构建时 `aws-lc-sys` 的 `memcmp_invalid_stripped_check` 用 `--target arm64-apple-macosx` 触发 cc-wrapper 多 target 缺陷（`posix_spawn failed`），构建失败 | 已确认必要，无可回归空间 | 624af665418d | `manifests/default.nix` |
| `vim` | 📦 本地 | ❌ | wrapper 相对设置 `VIMRUNTIME` | 可搬运 runtime 定位必须保留 | — | `packages/vim/` |
| `vim-plugins` | 📦 本地 | ❌ | 聚合固定 Vim plugins | plugin bundle 是产品 | — | `packages/vim-plugins/` |
| `watchexec` | 🩹 本地 | ✅ | stock 在 workspace 根执行裸 `cargo build`，install hook 会把测试 crate `test-socketfd` 一并发布；本地 override 用 `--package=watchexec-cli` 只构建产品 CLI；darwin 仅完成 eval/dry-run | stock 输出不再包含 `test-socketfd`，且最终 Mach-O 只依赖系统库 | — | `packages/watchexec/` |
| `wget` | 🩹 + 📦 本地 | 🟡 | macOS 绕过 static Perl；CA wrapper 必须保留，darwin 未验证 | 恢复 checks/build tool 后保留 CA packaging | — | `packages/wget/` |
| `zsh` | 🩹 + 📦 本地 | 🟡 | 静态 module patches；FPATH wrapper 和 zshenv policy 必须保留 | 逐项删编译 patch，保留 relocation packaging | — | `packages/zsh/` |
| `zsh-plugins` | 📦 本地 | ❌ | 聚合 oh-my-zsh 与 plugins | plugin bundle 是产品 | — | `packages/zsh-plugins/` |
