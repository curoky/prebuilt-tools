# standalone-binaries Agent Guide

先阅读：

- [架构与产物约束](docs/architecture.md)
- [贡献与验证流程](docs/contributing.md)
- 修改具体生态时阅读[包构建策略](docs/package-strategies/README.md)
- 修改 `cmd/binman/` 或 `cmd/nixcache/` 时阅读对应目录的 `CLAUDE.md`

## 工作边界

- 保持 Linux musl 全静态；macOS 仅动态链接系统库或随包相对路径 dylib；所有平台无
  运行时 `/nix/store` 引用。
- 默认从 unstable `pkgsStatic` 和 `manifests/default.nix` 开始；本地包只修
  root cause。
- 新增或改变 pin、patch、override、禁用检查、结构性 packaging、宿主依赖或动态
  例外时同步 [`TODO.md`](TODO.md)。
- 保留 `music-decrypto`、`nsight-systems` 的动态 ELF 例外和 `openssl` binary
  validation 豁免，除非用户明确要求改变。
- 不自动扫描 `packages/`；本地包通过 `packages/local/*.nix` 显式接入。
- `cmd/artifact/` 只负责规范化和 fail-closed 校验，不用后处理掩盖错误链接或硬编码
  资源路径。

## 修改原则

- Manifest 不填写默认字段；包清单、schema 和 CLI 参数以实现为准。
- 优先最小 override，避免 target overlay 污染 `buildPackages`。只针对静态 target
  的 override 必须检查 `stdenv.hostPlatform.isStatic`。
- Wrapper、资源、证书和同级 runtime 要做行为 smoke test，不能只验证 derivation
  构建成功。
- 修改 tag、layer、归档布局、cache schema 或 client 状态格式时，同步相关 workflow
  和 `docs/` 协议文档。
- 仓库内文档使用相对链接；设计说明放入 `docs/`，`CLAUDE.md` 只保留 agent 约束与
  导航。

按[贡献与验证流程](docs/contributing.md#验证)从目标范围开始验证。不要把 eval、
dry-run、lint 或代码审查表述为实际构建通过。
