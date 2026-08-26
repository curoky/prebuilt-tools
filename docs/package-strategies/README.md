# 包构建策略

默认路径是在 `manifests/default.nix` 选择 unstable `pkgsStatic`，再由
`cmd/artifact/` 完成 assembly、校验和归档。专题文档只解释不能从代码表面看出的
设计约束；具体包清单和回归状态分别以实现和[回归清单](../regression/AGENTS.md)为准。

| 生态 | 文档 | 主要案例 |
| --- | --- | --- |
| C / autotools | [c-autotools.md](c-autotools.md) | 静态链接、资源路径、s6 |
| Go | [go.md](go.md) | podman、macOS CGO |
| Node.js | [nodejs.md](nodejs.md) | 静态 runtime、同级 Node wrapper |
| `pkgsStatic.extend` | [pkgsstatic-extend.md](pkgsstatic-extend.md) | target 与 build platform 边界 |
| Perl | [perl.md](perl.md) | 平台拆分解释器、纯 Perl 工具、XS 模块 |
| Python | [python.md](python.md) | 静态解释器、同级 Python wrapper |
| Rust | [rust.md](rust.md) | miniserve、zellij |
| 特殊案例 | [special-cases.md](special-cases.md) | 非默认产物与动态例外 |

## 共用模式

### 相对资源 wrapper

把真实入口重命名为 `_<name>`，由 wrapper 根据自身路径定位资源。不要把 build-time
store path 写入 wrapper。

### 同级 runtime wrapper

wrapper 从自身路径求出共同 `store/` 目录，再显式执行同级 runtime。依赖包必须一起安装；
`bm` 不做依赖解析。

### 平台拆分

平台构建策略明显不同时，在 `packages/local/linux/` 和
`packages/local/darwin.nix` 接入独立 derivation，不在单个文件堆叠条件。
`packages/local/linux/` 再按架构拆成 `common.nix` 与 `x86_64.nix` / `aarch64.nix`。

### 状态管理

pin、patch、禁用检查、linker workaround、结构性 packaging 和动态例外只在
[回归清单](../regression/AGENTS.md)维护状态。专题文档不复制回归队列或测试历史。
