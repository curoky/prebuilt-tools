# Nix Cache 设计与协议

`cmd/nixcache/` 是本仓库专用的 GHCR-backed Nix binary cache，提供 `push`、
`serve`、`prune` 和 `size`。Nix 负责生成 closure、NAR 和 narinfo；Go 负责校验、
OCI 编排和 HTTP serving。命令参数以 `nixcache --help` 为准。

## 设计

- Segment immutable；每次 push 发布独立 tag，matrix job 可以并发执行。
- Snapshot 是原始 `flake.lock` 的 SHA-256。
- 每个 segment 引用完整 closure，NAR blob 由 registry 按 digest 去重。
- `packageKey` 是 retention 使用的稳定 package identity；`runId` 只用于诊断。
- 二进制使用 `CGO_ENABLED=0` 构建。`push` 依赖宿主 Nix，其他命令不调用外部程序。

## OCI Schema

Cache repository 是 `ghcr.io/curoky/standalone-binaries-cache`；segment tag 是
`v1-<snapshot-prefix>-<system>-<run-id>-<random-id>`。

每个 segment 是一个 OCI image manifest：

1. 第一层是 `application/vnd.curoky.nixcache.segment.v1+json` metadata。
2. 其余层是 `application/vnd.curoky.nixcache.nar.v1` NAR blob。
3. 每个 NAR descriptor 的 `org.nixos.store.hash` annotation 对应一个 metadata
   entry。

读取 segment 时 fail-closed 校验 tag、metadata、store hash、narinfo、NAR URL、
digest、size、media type 和 annotation 一致。

## Push

```text
nixcache push --key <package-key> <store-path>...
```

`--key` 或 `NIXCACHE_PACKAGE_KEY` 必填。命令创建临时 file cache，有限并发上传 NAR，
最后发布 metadata 和按 store hash 排序的 layer。`NIX_SIGNING_KEY_FILE` 传给 Nix；
registry auth 优先使用 GitHub Actions 环境，否则使用 Docker credential store。

## Serve

服务监听 `127.0.0.1:37515`，按当前 snapshot 和 system 的 tag prefix 并发加载
index，每 5 分钟刷新，提供 Nix cache info、narinfo 和 NAR endpoint。

Index 首次加载完成前 `/nix-cache-info` 返回 `503`。Repository 不存在视为空 cache；
并发消失的 tag 跳过；其他首次加载错误终止服务。周期刷新错误保留上一份 index。NAR
直接从 GHCR 流式返回，并由 request context 取消上游请求。

## Prune 与 Size

Retention 按 system 独立计算：

1. 按最新 segment 时间保留最近 `N` 个 snapshot，默认 `N=2`。
2. 每个保留 snapshot 按 `packageKey` 保留最新 segment。
3. 保留 snapshot 内空 `packageKey` 的 segment 全部保留。
4. 删除其余 segment。

删除前必须为全部目标 tag 找到 version ID，否则不发送删除请求。`--dry-run` 不发删除
请求；实际删除需要 package admin 权限。

`size` 只读取 manifest，按 digest 去重统计 metadata 和 NAR layer payload bytes，
不包含 manifest JSON 和 registry 内部开销。

## Binary Distribution

`.github/workflows/build-nixcache.yaml` 发布 `nixcache-linux-x86_64` 和
`nixcache-darwin-arm64`。Artifact 只有一个 tar.gz layer，归档布局为
`nixcache/nixcache`；installer 只依赖 `curl` 和 `tar`。

修改 repository、tag、media type、segment schema 或归档布局时，必须同步相关
workflow、installer 和[发布模型](release-model.md)。
