# Nixcache Agent Guide

`cmd/nixcache/` 实现本仓库专用的 GHCR-backed Nix binary cache。它构建为单个静态
Go 二进制 `nixcache`，只提供 `push` 和 `serve`。整体设计和过期策略见
[`docs/nix-cache-proxy-plan.md`](../../docs/nix-cache-proxy-plan.md)，全局产物约束见根
[`CLAUDE.md`](../../CLAUDE.md)。

本文是 `nixcache` 的 CLI、OCI schema、发布协议和验证要求 source of truth。

## 不变量

1. **只服务本仓库。** cache repository、listen address、refresh interval 和 media type
   直接固定在代码中，不增加通用配置层或 backend abstraction。
2. **Nix 负责 Nix store 协议。** `push` 必须通过 `nix copy --to file://...` 生成完整
   closure、NAR 和 narinfo；Go 不自行实现 NAR 编码、closure 查询或 narinfo 签名。
3. **标准格式交给开源库。** narinfo 和 store path 使用
   `github.com/nix-community/go-nix` 解析与校验；OCI descriptor、manifest 和 registry
   操作使用 `oras.land/oras-go/v2`。`bm` 独立保留 `go-containerregistry`。
4. **segment immutable。** 每次 `push` 只追加一个随机 tag，不更新共享 index tag。
   同一 GitHub Actions run 的 matrix job 必须可以并发发布且互不覆盖。
5. **snapshot 是完整 `flake.lock`。** cache identity 是原始 `flake.lock` 内容的
   SHA-256；所有 `nixpkgs-*` revision 只作为 metadata，不单独决定过期。
6. **新 snapshot 重新引用完整 closure。** 共享 NAR 可由 registry digest 去重，但当前
   snapshot 的 manifest 必须包含 closure 中所有 NAR layers，保证旧 snapshot 删除后仍可用。
7. **无宿主运行时依赖。** 发布的两个平台二进制都必须以 `CGO_ENABLED=0` 构建。
   `push` 运行时明确需要宿主 `nix`；`serve` 不调用外部程序。

## Cache OCI 协议

Nix cache 内容固定存放在：

```text
ghcr.io/curoky/standalone-binaries-cache
```

segment tag：

```text
v1-<flake-lock-sha256前16位>-<system>-<github-run-id>-<random-id>
```

每个 segment 是一个 OCI image manifest：

1. 第一层是 JSON metadata，media type 为
   `application/vnd.curoky.nixcache.segment.v1+json`；
2. 其余层是该次 closure 的 NAR blobs，media type 为
   `application/vnd.curoky.nixcache.nar.v1`；
3. NAR descriptor 使用 `org.nixos.store.hash` annotation 标识 store hash。

metadata schema 包含 `version`、`snapshot`、`repositoryCommit`、`system`、
`createdAt`、`channels` 和 `entries`。`entries` 保存原始 narinfo、NAR URL、digest
和 size。`NARPath` 仅供上传本地文件使用，使用 `json:"-"`，不得进入远端 metadata。

不保留旧 schema、旧 tag 或共享可变 index 的兼容读取。

## Push

```text
nixcache push <store-path>...
```

流程固定为：

1. 读取当前工作目录的 `flake.lock`；
2. 创建临时 file binary cache；
3. 执行带 zstd compression 的 `nix copy`；
4. 用 `go-nix` 读取并校验所有 narinfo；
5. 把 metadata 和全部 NAR layers 作为一个 immutable segment 发布；
6. 删除临时 cache。

NAR descriptor 的 digest 和 size 直接取已校验 narinfo 的 `FileHash` 和 `FileSize`，不得为
OCI 上传再次完整读取并 hash NAR 文件。

设置 `NIX_SIGNING_KEY_FILE` 时，把路径作为 URL encoded `secret-key` 参数交给 Nix。
设置 `GITHUB_TOKEN` 时，registry auth 使用 `GITHUB_ACTOR` 和 token；否则使用 Docker
default keychain。

## Serve

```text
nixcache serve
```

固定监听 `127.0.0.1:37515`，启动时加载所有 `v1-*` segments，之后每 5 分钟刷新。
只支持 `GET` 和 `HEAD`：

- `/nix-cache-info`
- `/<store-hash>.narinfo`
- `/nar/<file>`

miss 返回 `404`，不实现 upstream fallback、管理 API 或磁盘 NAR cache。NAR 从 GHCR
直接流式返回。刷新失败保留上一份可用 index；首次加载失败直接退出。

## 二进制发布协议

`.github/workflows/build-nixcache.yaml` 构建：

- `linux/amd64` → `linux-x86_64`
- `darwin/arm64` → `darwin-arm64`

二进制不是发布到 cache repository，而是作为普通工具发布到：

```text
ghcr.io/curoky/standalone-binaries:nixcache-<arch>
```

每个 artifact 只有一个 tar.gz layer，归档布局固定为：

```text
nixcache/nixcache
```

该布局使 `bm install nixcache` 能按普通 package 协议安装。改变 repository、tag、layer
数量、media type 或归档布局时，必须同时修改：

- `.github/workflows/build-nixcache.yaml`
- 根 `CLAUDE.md`
- 本文
- `bm` 的远端协议（如果变化不再兼容普通 package）

## 代码布局

- `main.go`：Cobra CLI 与固定 repository 接线。
- `model.go`：snapshot、segment 和 entry schema。
- `push.go`：临时 file binary cache 与 `go-nix` 解析。
- `registry.go`：GHCR auth、segment 读写和 NAR blob。
- `serve.go`：Nix HTTP substituter 与 refresh。
- `*_test.go`：本地 OCI registry、HTTP endpoint 和 opt-in Nix round-trip。

不要重新引入自建 HTTP writer、手写 narinfo parser、closure traversal、segment 分片或
共享 mutable index。

## 验证

在仓库根目录运行：

```bash
CGO_ENABLED=0 go test ./cmd/nixcache
CGO_ENABLED=0 go vet ./cmd/nixcache
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/nixcache
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/nixcache
```

真实 Nix round-trip：

```bash
NIXCACHE_TEST_STORE_PATH=/nix/store/<path> \
  CGO_ENABLED=0 go test -run '^TestNixRoundTrip$' -v ./cmd/nixcache
```

该测试必须覆盖 `nix copy -> file cache -> OCI -> serve -> nix copy --from`。修改 workflow
时还要验证 YAML、两个平台的归档布局和 tag 与本文一致。
