# Nixcache Agent Guide

`cmd/nixcache/` 是本仓库专用的 GHCR-backed Nix binary cache，提供 `push`、`serve`、
`prune` 和 `size`。本文定义其设计、OCI schema 和 CI 契约；全局约束见根
[`CLAUDE.md`](../../CLAUDE.md)。

## 设计

- Nix 通过 `nix copy --to file://...` 生成 closure、NAR 和 narinfo；Go 负责校验、
  OCI 编排和 HTTP serving。
- Cache repository、listen address、refresh interval 和 media type 固定在代码中。
- Segment immutable；每次 push 发布独立 tag，matrix job 可并发执行。
- Snapshot 是原始 `flake.lock` 的 SHA-256。每个 snapshot segment 引用完整 closure，
  NAR blob 由 registry 按 digest 去重。
- `packageKey` 是稳定 package identity；`runId` 只用于诊断。
- 二进制以 `CGO_ENABLED=0` 构建。`push` 依赖宿主 `nix`，其他命令不调用外部程序。

Narinfo 和 store path 使用 `github.com/nix-community/go-nix`；OCI 操作使用
`oras.land/oras-go/v2`；GHCR version 删除使用 `github.com/google/go-github/v75`。

## OCI Schema

Cache repository：

```text
ghcr.io/curoky/standalone-binaries-cache
```

Segment tag：

```text
v1-<snapshot前16位>-<system>-<run-id>-<random-id>
```

每个 segment 是一个 OCI image manifest：

1. 第一层是 `application/vnd.curoky.nixcache.segment.v1+json` metadata；
2. 其余层是 `application/vnd.curoky.nixcache.nar.v1` NAR blobs；
3. 每个 NAR descriptor 的 `org.nixos.store.hash` annotation 对应一个 metadata entry。

Metadata 字段为 `version`、`snapshot`、`repositoryCommit`、`system`、`packageKey`、
`runId`、`createdAt`、`channels` 和 `entries`。Entry 保存 store path、narinfo、NAR URL、
digest 和 size；本地 `NARPath` 不序列化。

读取 segment 时必须校验 tag、metadata、store hash、narinfo、NAR URL、digest、size、
layer media type 和 annotation 一致。

## Commands

### Push

```text
nixcache push --key <package-key> <store-path>...
```

`--key` 或 `NIXCACHE_PACKAGE_KEY` 必填。命令创建临时 file cache，解析 narinfo，以 8 路有限
并发检查并上传 NAR，最后发布 metadata 和按 store hash 排序的 NAR layers。

`NIX_SIGNING_KEY_FILE` 作为 file cache 的 `secret-key` 传给 Nix。Registry auth 优先使用
`GITHUB_ACTOR` 和 `GITHUB_TOKEN`，否则使用 Docker credential store。

### Serve

```text
nixcache serve
```

服务监听 `127.0.0.1:37515`，按当前 snapshot/system tag prefix 并发加载 index，每 5 分钟
刷新。支持 `GET` 和 `HEAD`：

- `/nix-cache-info`
- `/<store-hash>.narinfo`
- `/nar/<file>`

Index 首次加载完成前 `/nix-cache-info` 返回 `503`。Repository 不存在视为空 cache；并发消失的
tag 跳过；其他首次加载错误终止服务，周期刷新错误保留上一份 index。NAR 直接从 GHCR 流式返回，
并由 request context 取消上游请求。

### Prune

```text
nixcache prune [--keep N] [--dry-run]
```

Retention 按 system 独立计算：

1. 按最新 segment 时间保留最近 `N` 个 snapshot，默认 `N=2`；
2. 每个保留 snapshot 按 `packageKey` 保留最新 segment；
3. 保留 snapshot 内空 `packageKey` 的 segment 全部保留；
4. 其余 segment 通过 GitHub Packages version API 删除。

删除前必须为全部目标 tag 找到 version ID，否则不发出删除请求。`--dry-run` 不需要
`GITHUB_TOKEN`。实际删除需要 package admin 权限；Actions job 使用 `packages: write`。

`.github/workflows/prune-nix-cache.yaml` 仅允许手动触发。

### Size

```text
nixcache size
```

只读取 manifests，按 digest 去重统计 metadata 和 NAR layer payload bytes。结果不包含
manifest JSON 和 registry 内部开销。

## CI

Linux、Darwin 和 LLVM workflow：

1. 通过 `cmd/nixcache/install.sh` 安装已发布的 `nixcache`；
2. 启动 `serve` 并等待 `/nix-cache-info` ready；
3. 用本地 substituter 做 discover 和 build；
4. 以 matrix package 名设置 `NIXCACHE_PACKAGE_KEY`，push 实际 output closure。

Cache 未签名，相关 Nix 命令显式设置 `require-sigs=false`。每个 matrix job 发布一个完整
closure segment。

## Binary Distribution

`.github/workflows/build-nixcache.yaml` 发布：

- `linux/amd64` → `ghcr.io/curoky/standalone-binaries:nixcache-linux-x86_64`
- `darwin/arm64` → `ghcr.io/curoky/standalone-binaries:nixcache-darwin-arm64`

每个 artifact 只有一个 tar.gz layer，归档布局为 `nixcache/nixcache`。
`cmd/nixcache/install.sh` 只依赖 `curl` 和 `tar`。

修改 repository、tag、media type 或归档布局时，同步 workflow、installer、根
`CLAUDE.md` 和 `cmd/binman/` 的读取协议。

## Validation

```bash
CGO_ENABLED=0 go test ./cmd/nixcache
CGO_ENABLED=0 go vet ./cmd/nixcache
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/nixcache
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/nixcache
bash -n cmd/nixcache/install.sh
shellcheck cmd/nixcache/install.sh
```

真实 Nix round-trip：

```bash
NIXCACHE_TEST_STORE_PATH=/nix/store/<path> \
  CGO_ENABLED=0 go test -run '^TestNixRoundTrip$' -v ./cmd/nixcache
```
