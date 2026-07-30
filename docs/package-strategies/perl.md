# Perl Runtime 与 XS

Perl 包按平台构建独立 runtime，脚本工具在运行期显式使用同级 `perl` 包。精确模块清单和
构建参数以 `packages/perl/`、`packages/exiftool/` 为准；定制状态见
[`TODO.md`](../../TODO.md)。

## 平台 Runtime

### Linux

Linux 发布 musl 全静态 Perl。解释器以 `-Uusedl` 构建，不能在运行期加载 XS `.so`，因此
所需 XS 扩展必须作为 static extension 编入解释器。

`exiftool` 需要的压缩扩展也遵守该约束：Linux 的 `perl` runtime 内建这些 XS 扩展，
`exiftool` 包只携带脚本和纯 Perl 模块。增加 XS 依赖时必须同时修改解释器构建，并增加
`require` 校验，不能把可加载模块放进工具包。

### macOS

macOS 使用 native Perl，并把非系统依赖静态链接。`libperl.dylib` 保留在包内，其 install
name 和消费者 load command 必须使用 `@loader_path` 相对路径。

macOS 可以携带 XS `.bundle`，但其中的 Nix 依赖必须静态链接；最终只允许加载系统 dylib
和包内 `libperl.dylib`。artifact assembly 不负责改写 Mach-O load command。

## 同级 Runtime

Perl 脚本工具把真实入口和模块树放在自身包内，由 wrapper：

1. 根据自身路径定位包根目录和同级 package store；
2. 设置指向包内模块树的 `PERL5LIB`；
3. 显式执行同级 `perl/bin/perl`。

工具不能依赖宿主 Perl，也不能保留 Nix 生成的绝对 shebang。多入口工具还需让入口间调用
继续解析到同一包内的 wrapper。
