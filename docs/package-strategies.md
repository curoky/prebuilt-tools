# 包构建策略

默认构建路径是：在 `manifests/default.nix` 选择 unstable 的 `pkgsStatic` 包，然后由
`scripts/normalize.sh` 后处理。本文索引只记录偏离默认路径的本地实现；通用约束和改动流程见
[根 CLAUDE.md](../CLAUDE.md)。

| 生态 | 文档 | 主要案例 |
| --- | --- | --- |
| C / autotools | [c-autotools.md](package-strategies/c-autotools.md) | 资源 wrapper、s6 路径、容器组件、clang-format |
| Go | [go.md](package-strategies/go.md) | podman、macOS `CGO_ENABLED=0` |
| Node.js | [nodejs.md](package-strategies/nodejs.md) | 静态 runtime、同级 Node wrapper |
| Perl | [perl.md](package-strategies/perl.md) | 平台拆分解释器、纯 Perl 工具、XS 模块 |
| Python | [python.md](package-strategies/python.md) | 静态解释器、同级 Python wrapper |
| Rust | [rust.md](package-strategies/rust.md) | miniserve、zellij |
| 特殊案例 | [special-cases.md](package-strategies/special-cases.md) | feature reduction、linker 修复、动态例外 |

包集合以 `packages/local.nix` 和 `manifests/default.nix` 为准。case study 不复制完整 derivation，
只解释为什么需要本地实现、关键约束是什么、何时可以删除。

## 共用模式

### 相对资源 wrapper

静态二进制仍可能把数据目录、证书、插件或 helper 路径写入构建结果。常见做法是把真实入口重命名为
`_<name>`，由公开 wrapper 根据自身真实路径计算 package root，再传入相对资源路径。`file`、`vim`、
`curl`、`wget` 和 `git` 使用这一模式。

### 同级 runtime wrapper

Python、Perl 和 Node.js 工具把脚本与 runtime 分成独立 artifact。wrapper 从自身位置求出共同
`store/` 目录，再显式执行同级 runtime。这样工具不依赖宿主解释器，也不依赖构建期 shebang。

这类工具在单独下载时需要同时安装对应 runtime；`sb` 不做依赖解析。

### 平台拆分

同名输出可在 `packages/local.nix` 的 `linux` 与 `darwin` 集合中使用不同 derivation。平台拆分优于在
单个文件中堆叠大量条件，尤其适合解释器和 macOS 部分静态构建。

### 临时修复

下列内容应视为待回归状态：

- 非 unstable 版本；
- 为上游编译错误增加的 source patch；
- `doCheck = false` 或 `doInstallCheck = false`；
- linker 容错参数；
- 只为旧版 nixpkgs 复制的 builder。

上游修复后删除本地路径和对应说明，不保留兼容层。
