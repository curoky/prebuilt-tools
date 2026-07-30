# Nixcache Agent Guide

先阅读根 [`CLAUDE.md`](../../CLAUDE.md)、[Nix Cache 设计](../../docs/nix-cache.md)
和[发布模型](../../docs/release-model.md)。本文件只记录修改 `cmd/nixcache/` 时的 agent
约束。

- Nix 通过 `nix copy --to file://...` 生成 closure、NAR 和 narinfo；Go 负责校验、
  OCI 编排和 HTTP serving。
- Cache repository、listen address、refresh interval 和 media type 固定在代码中。
- Segment immutable；每次 push 发布独立 tag，matrix job 可并发执行。
- Snapshot 是原始 `flake.lock` 的 SHA-256。每个 snapshot segment 引用完整 closure，
  NAR blob 由 registry 按 digest 去重。
- `packageKey` 是稳定 package identity；`runId` 只用于诊断。
- 二进制以 `CGO_ENABLED=0` 构建。`push` 依赖宿主 `nix`，其他命令不调用外部程序。
- 读取 segment 时 fail-closed 校验 tag、metadata、store hash、narinfo、NAR URL、
  digest、size、media type 和 annotation 一致。
- `serve` 首次加载失败与周期刷新失败的语义不同；不要让 refresh 错误清空已有 index。
- `prune` 必须先解析全部目标 version ID，再进行任何删除；`--dry-run` 不发请求。
- `size` 只统计 metadata 和 NAR layer payload，并按 digest 去重。
- 修改 repository、tag、media type、segment metadata、retention identity 或归档布局时，
  同步 workflow、installer、`docs/nix-cache.md` 和 `docs/release-model.md`。
- 保持 repository-specific 实现，不为假设中的 registry 或 cache backend 预造抽象。

验证命令见 [`docs/contributing.md`](../../docs/contributing.md#验证)。
