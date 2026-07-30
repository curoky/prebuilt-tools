# Binman Agent Guide

先阅读根 [`CLAUDE.md`](../../CLAUDE.md) 和 [`bm` 设计与协议](../../docs/binman.md)。
本文件只记录修改 `cmd/binman/` 时的 agent 约束。

- `bm` 必须可用 `CGO_ENABLED=0` 构建，运行时不调用 `curl`、`tar`、`oras`、`jq` 或 Nix。
- package 和 profile link 必须是相对 symlink，整个 prefix 可直接移动。
- package 彼此独立，client 不解析依赖。
- 多包操作先 resolve 全部 tag；任一失败时不得写入安装状态。
- Store 交换、link 或 profile rebuild 失败时必须恢复旧状态。
- Tar entry、symlink target、hardlink target 及其既有 symlink parent 都必须留在
  staged store 内；未支持的 entry type 直接报错。
- `.binman-meta` 只能由 client 创建，不能继承归档内容。
- Link 前完成全包冲突预检；unlink 只删除仍由该 package 拥有的 symlink。
- 修改解压、link 或事务逻辑时，先添加能复现边界的测试。
- 保持同一 `package main`，不要为假设中的 backend、registry 或 package graph 预造
  interface。
- 修改 registry、tag、layer、归档布局或 metadata 格式时，同步 `registry.go`、
  `install.sh`、发布 workflow 和 `docs/binman.md`。

验证命令见 [`docs/contributing.md`](../../docs/contributing.md#验证)。
