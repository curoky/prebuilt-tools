# Node.js 生态打包策略

Node.js 生态分两层：**静态 Node.js runtime**（独立成包，Linux musl 全静态 / macOS 部分静态）和
**Node CLI 工具**（复用上游 JS 分发 + 薄 wrapper 运行期绑定同级静态 node）。

- runtime：`packages/nodejs/{24,26}`（`nodejs-slim24` Linux-only；`nodejs-slim26` 两平台各一份）。
- 工具：`packages/{pnpm,prettier,markdownlint-cli2,opencommit}`（两平台复用同一 wrapper derivation）。

## Node.js runtime

三个 runtime 都不额外 `mkDerivation` 包裹，直接用 override 后的 `nodejs-slim` derivation 当产物
（`$out` 是完整 node 安装，消费方引用 `$store/nodejs-slimNN/bin/node`）。

**共同手法：** ada/libuv/uvwasi 等是内层 `nodejs.nix` 的 callPackage 依赖、无法经 `.override` 触达，
所以用 `pkgsStatic.extend` 加本地 overlay 给这些依赖打补丁，再取其 `nodejs-slim`，最后 `.overrideAttrs`
调 node 本身的 configureFlags/buildInputs/doCheck。

**build vs runtime python：** python 只作 `nativeBuildInput`（跑 `configure.py`），从不链入 node。
必须注入 **native** python3——pkgsStatic 的静态 musl python 其 ctypes 无法 dlopen
（`OSError: Dynamic loading not supported`），gyp 的 ninja generator import ctypes 会中止。

### nodejs-slim24（Linux，musl 全静态）

`packages/nodejs/24`，base=`pkgsStatic`，pin 到 `nodejs-slim_24`。

overlay 补丁（root cause）：

- `ada` / `libuv`：`doCheck = false`。ada 的 `basic_fuzzer` 在静态工具链未构建；libuv 的
  `udp_try_send` 在沙箱受限网络报 `-98/EADDRINUSE`。
- `uvwasi`：CMakeLists 硬编码 `add_library(uvwasi SHARED)` 忽略 `BUILD_SHARED_LIBS`，静态工具链链
  `libuvwasi.so` 失败（`R_X86_64_32 against crtbeginT.o`）。改 `STATIC` 并重命名 `OUTPUT_NAME
  "uvwasi_noshared"` 避免与既有 `uvwasi_a` 冲突。
- `hdrhistogram_c`：默认建 SHARED 同样链接失败；用 `-DHDR_HISTOGRAM_BUILD_SHARED=OFF` 只留静态归档，
  但 CMake 命名为 `libhdr_histogram_static.a`，node gyp 链接行用 `-lhdr_histogram`，故 `postInstall`
  加 `libhdr_histogram.a` symlink。

node 自身 overrideAttrs：

- 过滤静态 stdenv 追加的 `--enable-static`/`--disable-shared`（node 的 `configure.py` 拒绝
  `--disable-shared`）。
- 过滤 `--shared-brotli`/`--shared-simdutf` 改用 node 自带 bundle：系统 brotli 静态库是分裂归档
  （`libbrotli{common,enc,dec}.a`），单遍 musl GNU ld 链接顺序错误留下未定义符号
  （`_kBrotliPrefixCodeRanges`）；系统 simdutf 6.5.0 缺 node24 需要的 `base64_to_binary_safe`。
- `doCheck = false`：check 阶段建 `.node` test addon，静态 musl gcc 报 `R_X86_64_32 against hidden
  symbol __TMC_END__`（静态 CRT crtbeginT.o 非 PIC）。

> 放弃的方案（注释）：Node SEA 单文件方案——翻 SEA fuse 会在任何 JS 运行前于 V8 init 段错误，与
> 静态依赖无关。

### nodejs-slim26（Linux，musl 全静态经 cross set）

`packages/nodejs/26/linux.nix`，base=`pkgsStatic`（实为 **musl64 cross 静态集**）。node26 链
`temporal_capi`（Rust 依赖）：native-static 集会从源码重建整个 musl LLVM+rustc，而 cross 集走 rust 的
`fastCross` path 复用缓存的 glibc rustc/LLVM，产物仍是全静态 musl。

相比 24 的关键改进 —— **`onlyStatic` guard**：

```
onlyStatic = pkg: overrides:
  if pkg.stdenv.hostPlatform.isStatic then pkg.overrideAttrs overrides else pkg;
```

根因：`pkgsStatic.extend` 会同时改写 build-platform（glibc）副本；而 libuv 是 cmake 的
nativeBuildInput、cmake 又是 llvm 的 nativeBuildInput，改写 glibc libuv 会变更 cmake/llvm hash，
导致 rustc 的 llvm 无法从 cache.nixos.org 替换而全量源码重建。故每个 override 只作用于 musl-static
target 副本。详见 [docs/pkgsstatic-extend-toolchain-pollution.md](file:///workspace/standalone-binaries/docs/pkgsstatic-extend-toolchain-pollution.md)。

新增 overlay 补丁：

- `lief`（node ≥25.6 经 `useSharedLief` 链入）：nixpkgs 硬编码 `LIEF_PYTHON_API true` 建 Python
  bindings，拉入 pydantic-core（Rust/maturin cdylib），pkgsStatic 下 maturin 无法产 cdylib。node 只
  需 lief 的 C/C++ 库，故禁用 `LIEF_PYTHON_API`、丢 `py` output。注意 python deps 经
  `propagatedBuildInputs`（非 `buildInputs`）进闭包，故 `buildInputs` 与 `propagatedBuildInputs`
  都要清。
- `temporal_capi`：`doInstallCheck = false`。其 installCheck 编译并运行 C/C++ 程序，静态 musl cross
  下裸 `pkg-config` 报 command not found（只有 target 前缀的 `x86_64-...-pkg-config`）。
- node overrideAttrs 额外过滤 `--shared-merve`（node26 新增 cjs-module-lexer 的 C++ 库）：系统
  `libmerve.a` 引用 node 自带 simdutf 缺失的 `simdutf::detail::find`，bundle merve 使其用 node 自带
  simdutf。

### nodejs-slim26（macOS，部分静态）

`packages/nodejs/26/darwin.nix`，base=`pkgsStatic`，注入 native `python3` 与 `cctools`。

macOS 部分静态 ladder：全静态 Mach-O 不可能（无静态 libSystem），经 `pkgsStatic` 使每个 nix 依赖链为
`.a`，仅剩 `/usr/lib`（libSystem、libc++）与系统 frameworks（CoreFoundation、Security）动态。
验证 `otool -L $out/bin/node` 只剩 `/usr/lib` + 系统 framework。

darwin 专属 build-tool 注入：

- `python3`（native）：darwin pkgsStatic 的 python3 被标记 broken，会阻塞 eval。
- `cctools`（native）：node darwin 构建的 `gyp-mac-tool ExecFilterLibtool` 调裸 `libtool`（Apple
  Mach-O 静态归档工具，非 GNU libtool）；pkgsStatic 只暴露 target 前缀的
  `arm64-apple-darwin-libtool`，裸调用报 `FileNotFoundError: ... 'libtool'`。

其它：`lief` 因 darwin pkgsStatic python broken，先 `.override { python3 = <native> }` 再从零重建
cmakeFlags；ICU 去 `--with-intl=system-icu` 改 `--with-intl=small-icu`（英文 ICU 数据嵌入二进制，
非英文 locale 经 `NODE_ICU_DATA` 提供）；`doCheck = false`（SEA 测试在 macOS 沙箱 flaky）。

> 放弃的方案（注释）：native `pkgs` + 仅去 `--shared-<dep>`——node 只 bundle 少数依赖，二进制仍留
> 约 18 个 `/nix/store/*.dylib` load command，违反 no-/nix-dylib 规则。必须 `pkgsStatic` 才让依赖出
> `.a`。

## Node CLI 工具（sibling static node wrapper）

四者共享同一机制：`stdenvNoCC.mkDerivation` + `dontUnpack`，把上游 nixpkgs 的 JS 分发拷进
`$out/libexec/<tool>`，`writeText` 生成 relative-path bash wrapper 放 `$out/bin`。wrapper 经
`readlink -f "$0"` 定位自身 → 求 `root`（本包 deploy 目录）与 `store`（其父目录），**显式调用兄弟包
`$store/nodejs-slim26/bin/node <entry> "$@"`**。这样 node 显式传入、bundle JS 的 shebang 无关紧要，
解决了 normalize 后 `.mjs`/`.cjs` shebang 被改写为 `/usr/bin/env node` 会依赖 host PATH node 的问题。
wrapper 是 shell 脚本无静态链接需求，平台无关，Linux/darwin 复用同一 derivation（各自对接本平台
`nodejs-slim26`）。这套取代了此前的 `bundle = true` manifest 条目。

**build-vs-runtime node 是两类工具的关键差异：**

| 工具 | build 用什么 node | 说明 |
| --- | --- | --- |
| `pnpm` | **静态 node**（`pnpm.override { nodejs-slim = nodejs-slim26; }`） | pnpm 只解包 JS；端到端锻炼静态 runtime。入口 v11+ 是 `.mjs`、旧版 `.cjs`；生成 `pnpm`/`pnpx` 两个 wrapper |
| `prettier` | **静态 node**（`prettier.override { nodejs = nodejs-slim26; }`） | prettier 用 pnpm 拉依赖；入口 `prettier.cjs` |
| `markdownlint-cli2` | **regular node**（`inherit (pkgs) markdownlint-cli2`） | 是 `buildNpmPackage` 工具，构建需 `npm`（`nodejs-slim` 无 npm），仅运行期切静态 node |
| `opencommit` | **regular node**（`inherit (pkgs) opencommit`） | 同上，需 npm；生成 `opencommit`/`oco` 双 wrapper |

`markdownlint-cli2` / `opencommit` 的 wrapper derivation 额外加 `installCheck`：用 `nodejs-slim26`
实际跑一遍 shipped JS（markdownlint-cli2 lint 一个临时干净 markdown、opencommit 跑 `--version`），
确认 JS 确实能在静态 runtime 上运行。
