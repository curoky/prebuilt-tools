# 非默认产物

本页只记录偏离“直接发布完整 `pkgsStatic` 工具”的稳定产品边界。具体 patch、禁用 feature 和
回归状态以实现及[回归清单](../regression/AGENTS.md)为准。

## 裁剪产物

### ffmpeg

macOS 发布 headless、feature-reduced 的 ffmpeg。启用的依赖必须能静态链接，最终 Mach-O
只能依赖系统 dylib 和 framework。恢复 codec、filter 或 network feature 时，必须重新检查完整
依赖闭包，不能只验证 ffmpeg configure 成功。

### PostgreSQL

Linux 只发布静态链接 `libpq` 的 `psql` 客户端，不发布 server、扩展或开发工具集合。
构建不得重新引入需要动态加载模块的 server 路径。

### 单工具输出

版本化 LLVM 包只提取所需工具；类似裁剪属于产品边界。不要为了贴近上游输出而恢复无关二进制、
库或开发文件。

## 构建期依赖

构建工具可以使用 native 包，只要它不会进入目标产物的运行期闭包。例如 macOS 的静态 wget
使用 native Perl 作为生成工具，但 wget 的所有非系统运行期依赖仍需静态链接。

build-time override 必须只替换构建工具，不得改变 target 包的链接边界。

## Linux 动态例外

当前只允许两个动态 ELF 例外：

- `music-decrypto`：依赖宿主 glibc ABI 的 .NET AOT 可执行文件；
- `nsight-systems`：NVIDIA 提供的预编译 glibc 分发物。

这些包仍必须满足：

- 不引用 `/nix/store`；
- ELF interpreter 和 rpath 只指向明确的宿主 ABI 路径；
- 例外同时登记在 `lib/make-artifacts.nix`、根 `AGENTS.md` 和[回归清单](../regression/AGENTS.md)；
- 新增例外必须先确认，不能通过放宽全局校验实现。

除这两个包外，Linux ELF 必须通过 musl 全静态校验。
