# Python 构建案例

Python 生态分两层：**静态 CPython 解释器**（作为共享 runtime 独立成包）和**纯 Python 脚本工具**
（用 shared-sibling wrapper 运行期绑定那个解释器）。工具本身不做任何静态链接。

- 解释器：`packages/python/{311,312,313,314,315}`（Linux-only）。
- 工具：`packages/{git-filter-repo,netron}`（跨平台 common）、`packages/dool`（Linux-only）。

## Python 解释器（311 / 312 / 313 / 314 / 315）

`packages/local.nix` 用 `pkgsStatic.callPackage ./python/<ver>` 接入五个 Linux-only 解释器，
即对上游 musl-static `pythonNNN` 做 `overrideAttrs`。

### Linux：musl 纯静态

核心机制：把仓库自带的 `Setup.local` 复制进 CPython 源码树的 `Modules/Setup.local`
（`postPatch` 里 `cp ${Modules_Setup_local} Modules/Setup.local`），让 `makesetup` 把可选扩展模块
**静态编译进解释器**而非做成 `.so`——因为 musl 全静态解释器运行期无法 dlopen `.so`。

`Setup.local` 关键点：

- stdlib C 扩展列为 static（无 `*shared*` 标签即静态）：`_socket`、`_ctypes`、`_bz2`、`_lzma`、
  `zlib`、`_curses`、`readline` 等。
- **OpenSSL 显式静态归档链接**：`_ssl` / `_hashlib` 用 `-l:libssl.a -l:libcrypto.a` 强制静态归档、
  配 `-Wl,--exclude-libs,libssl.a`（隐藏归档符号）。
- `configureFlags` 追加 `LDFLAGS=-L${termcap}/lib`，为 `readline`/`_curses` 的 `-ltermcap` 提供
  静态库路径。

瘦身开关：`stripIdlelib = true; stripTests = true; stripTkinter = true;`；`_tkinter`/`_scproxy`/
`nis`/`_uuid` 保持禁用。

### 临时编译修复（待回归）

**python315 的 musl `statx` 拼写 patch**（仅 `stdenv.hostPlatform.isMusl` 时）：

```bash
substituteInPlace Modules/posixmodule.c --replace-fail stx_dio_offset_align stx_dio_offet_align
```

根因：musl 的 `<bits/statx.h>` 把 `struct statx` 成员误拼为 `stx_dio_offet_align`（少一个 `s`），
而 CPython 3.15 的 `posixmodule.c` 用正确的内核名 `stx_dio_offset_align`；`configure` 只探测到拼写
正确的 `stx_dio_mem_align`，导致同一 `#ifdef` 块里的 offset-align 引用编译失败。这是典型的**临时
编译修复**。musl 或 CPython 修好后应删除；回归流程见根 `CLAUDE.md`。

## Python 脚本工具（sibling-wrapper：git-filter-repo / netron / dool）

**base：** 一律 native `pkgs.callPackage`（`git-filter-repo`/`netron` 在 common、`dool` 在 linux）。
它们不含需静态化的编译产物，只打包纯 Python 脚本 + 一个 bash wrapper。

### Sibling wrapper

三者共享同一套 wrapper 模式：

- wrapper 解析自身真实路径 → 计算 `root`（包根）与 `store`（上级 store 目录）。
- **Linux 分支**：显式设 `PYTHONHOME=$store/python314`，逐段拼 `PYTHONPATH`（`lib/python3.14`、
  `site-packages`、`lib-dynload`，以及本包 `$root/lib/python3.14/site-packages`），再 `exec` 同级
  `python314` 包里的 `$PYTHONHOME/bin/python3.14` 运行主脚本——**运行期解析 sibling 静态解释器**。
- **darwin 分支**：`exec -a "$0" python3 "$root/bin/_<name>_main.py"`，使用宿主 `python3`；
  当前 Darwin 不发布 Python runtime。

因为 wrapper 显式指定相对解释器，上游写死的 nix-store shebang 全部失效。

注意：Darwin 分支依赖宿主 `python3`，与根 `CLAUDE.md` 的“包独立”长期目标存在差距。修改这些
工具时不要把这一现状误写成完全自包含；若新增 Darwin Python runtime，应同步改三个 wrapper。

### 各自的打包细节

- **git-filter-repo**：`version` 取自 `python3Packages.git-filter-repo`；从上游包复制 site-packages，
  把上游 `.git-filter-repo-wrapped` 拷成 `_git_filter_repo_main.py` 作真实入口。
- **netron**：从 PyPI `.whl`（`fetchurl` + `unzip`）解包成 site-packages；`writeText` 生成
  `_netron_main.py`（`import netron; main()`，含 `sys.argv[0]` 清洗）。
- **dool**：dool 是单文件、纯 stdlib 的 Python 脚本，直接把 `${dool}/bin/dool` 复制成
  `_dool_main.py` 当入口，wrapper 默认追加 `--bytes`。
