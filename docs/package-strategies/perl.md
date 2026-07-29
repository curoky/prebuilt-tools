# Perl 生态打包策略

Perl 生态是**两平台策略分歧最大**的一类，因为 `packages/perl` 的解释器在两平台构建方式完全不同，
直接决定了下游 Perl 工具（尤其含 XS 编译扩展的）怎么打包：

- 解释器：`packages/perl/{linux,darwin}.nix`（platform-split）。
- 工具：`packages/{cloc,parallel}`（跨平台 common，纯 Perl）、`packages/exiftool/{linux,darwin}.nix`
  （platform-split，含 XS 压缩模块）。

所有 Perl 工具都用与 Python 相同的 shared-sibling wrapper：`mv $out/bin/<tool> $out/bin/_<tool>`，
wrapper 设 `PERL5LIB` 后 `exec $store/perl/bin/perl "$root/bin/_<tool>"`——运行期绑定同级 `perl` 包，
不依赖宿主 perl。

## Perl 解释器（platform-split）

两平台最终都用同一个 sibling-wrapper 收尾：`mv $out/bin/perl $out/bin/_perl`，装 wrapper 作 `perl`，
wrapper 设 `PERL5LIB`（含 `site_perl/*`）后 `exec $root/bin/_perl`。差异在解释器本身怎么构建。

### Linux：musl 纯静态（`-Uusedl`，XS 编进解释器）

**base：** `pkgsStatic.callPackage ./perl/linux.nix`，即 musl-cross 全静态 perl。

关键约束（注释）：静态 perl 用 `-Uusedl` 构建，**运行期无法 dlopen 任何 XS `.so`**；每个扩展必须是
`static_ext`（编译进解释器）。静态 build 没有 dylib，因此无需 install-name 改写。

上游 `pkgsStatic.perl` 已内建 `Compress::Raw::{Zlib,Bzip2}` 和 `IO::Compress::*`；但 exiftool 还需要
`Compress::Raw::Lzma` 和 `IO::Compress::Brotli`，不在 perl 源码树里。机制（`preConfigure`）：把这两个
CPAN 源 `tar` 解到 `cpan/Compress-Raw-Lzma`、`cpan/IO-Compress-Brotli`，让 perl 构建 harness 把它们
当 static extension 编译（静态 `liblzma`/`libbrotli` 归档直接链进 perl 二进制）：

- **Lzma**：写 `config.in` 指向 `${xz.dev}/include` 与 `${xz.out}/lib`。
- **Brotli**：替换上游 `Makefile.PL`。根因：上游 `Makefile.PL` 通过 `Alien::cmake3` 构建 vendored
  brotli，在 perl 静态 build harness 里跑不通。换成最小 `Makefile.PL`，`LIBS` 静态链接
  `-lbrotlienc -lbrotlidec -lbrotlicommon`（顺序：enc/dec 依赖 common）。
- 把注入文件 **append** 进顶层 `MANIFEST`（注释警告：只能 append，不能 reorder，perl 的
  pod/buildtoc 步骤依赖既有顺序）。

`postInstall` 加校验：对两个模块各跑 `perl -e "require $m; 1"`，编不进去即 `exit 1`。

### macOS：native + 单依赖静态替换（ladder 2）

**base：** native `pkgs.callPackage ./perl/darwin.nix`，传入 `libxcryptStatic = pkgsStatic.libxcrypt`。

根因：macOS 无静态 libc，在 darwin pkgsStatic 集里构建 perl 会拉进一整套独立静态 toolchain（且构建
失败）。故用 native perl（走上游 cache），**只把唯一的非系统动态依赖 libxcrypt 换成静态归档**
（`perl.override { libxcrypt = libxcryptStatic; }`），使 perl 静态链接 libcrypt，只剩 `/usr/lib`
系统库动态。

**install_name_tool 重写**（`libperl.dylib` 保持动态、解释器按绝对 `/nix/store` install path 加载它，
而 normalize.sh 不改 Mach-O install name）：

- `install_name_tool -id "@loader_path/libperl.dylib"` 改 dylib 自身 id。
- 把 `$out/bin/perl`、`perl5*` 的 load command 从旧 `/nix` id 改为
  `@loader_path/../<coreDir>/libperl.dylib`。
- 遍历 `CORE/` 下每个 Mach-O，把对 libperl 的引用改成 `@loader_path/libperl.dylib`。
- **zlib 系统库重指**：`Compress::Raw::Zlib` 的 `Zlib.bundle` 动态链 `/nix` 的 libz（唯一的 `/nix`
  Mach-O 依赖）；因 zlib 是 macOS 系统库，用 `install_name_tool -change` 指向
  `/usr/lib/libz.1.dylib`，使只剩 `/usr/lib` 依赖。

## Perl 脚本工具

### cloc / parallel（跨平台 common，纯 Perl）

**base：** native `pkgs.callPackage`。根因（[local.nix](file:///workspace/standalone-binaries/packages/local.nix#L87-L92)）：
cloc 不需静态链接，且它会拉进的全静态 perl/perlPackages 在 darwin 构建失败，故两平台都从 native pkgs
构建。这属于刻意的 **packaging decision（wrapper）**，不是临时编译修复，**不切回上游**。

- **cloc**：`overrideAttrs`，`mv cloc _cloc` + wrapper 直接 `exec $store/perl/bin/perl "$root/bin/_cloc"`。
  用 rsync 把纯 Perl 依赖模块树复制进 `$out/lib/perl5`（`Moo`、`AlgorithmDiff`、`RoleTiny`、
  `SubQuote`、`ParallelForkManager`、`RegexpCommon`）。无 XS。
- **parallel**：多命令 sibling-wrapper。去掉 nixpkgs 的 bash PATH-wrapper（`mv .parallel-wrapped
  parallel`、`rm sem`），对 `parallel sem niceload parcat parsort sql` 逐个 `mv` 成 `_<name>` 铺
  wrapper。wrapper 把脚本自身目录前置 `PATH`（让 shell out 到裸 `parallel` 的脚本找到同级 wrapped），
  再 `exec $store/perl/bin/perl "$bindir/_$name"`，用 `basename "$0"` 派生脚本名。纯 Perl。

### exiftool（platform-split，XS 压缩模块）

**base：** 两平台都 native `pkgs.callPackage`（darwin 额外 `inherit pkgsStatic`）。共同点：
`ImageExifTool.overrideAttrs`，`mv exiftool _exiftool` + sibling-wrapper，rsync 铺模块树进
`$out/lib/perl5`。差异全在 XS 压缩模块怎么处理，直接对应上面解释器的 platform-split。

**Linux（`exiftool/linux.nix`）——只 ship 纯 Perl：** sibling 静态 perl（`-Uusedl`）无法 dlopen 任何
XS `.so`，压缩 XS 模块已编进 perl 解释器（见上），故本包**不发单独的编译压缩模块**，只发纯 Perl 部分：
exiftool 脚本、`Image::ExifTool`、`Archive::Zip`（纯 Perl，用 perl 内建的 `Compress::Raw::Zlib`）。
rsync 复制 `ArchiveZip`、`FileSlurper`、`GetoptLong`。

**darwin（`exiftool/darwin.nix`）——XS 保留为 `.bundle` + 静态链压缩库：** native perl 能 dlopen
`.bundle`。XS 压缩模块默认动态链 `/nix/store` 压缩 dylib（libz/liblzma/libbz2/libbrotli），而
normalize.sh 不改 Mach-O load command 会残留 `/nix` 引用违反可移植规则。修法（ladder 2）：逐个
override XS 模块，把其构建指向只带静态 `.a` 的 `pkgsStatic.<lib>`：

- `CompressRawZlib`：`preConfigure` 写 `config.in`，`INCLUDE/LIB` 指 `pkgsStatic.zlib`
  （`BUILD_ZLIB=False`）。
- `CompressRawBzip2`：`env` 设 `BZIP2_LIB`/`BZIP2_INCLUDE` 指 `pkgsStatic.bzip2`。
- `CompressRawLzma`：`config.in` 指 `pkgsStatic.xz`。
- `IOCompressBrotli`：`postPatch` 把 `Makefile.PL` 的 `@LIBS@` 替成
  `-L${pkgsStatic.brotli.lib}/lib -lbrotlienc -lbrotlidec -lbrotlicommon`。
- `IOCompress`（纯 Perl）：过滤掉原 `propagatedBuildInputs` 里动态的 `Compress-Raw-{Zlib,Bzip2}`，
  换成上面的静态版，使闭包携带静态变体。

结果 `.bundle` 只依赖 `/usr/lib/libSystem.B.dylib`。
