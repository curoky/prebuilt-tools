# Binman Agent Guide

`cmd/binman/` 实现 OCI artifact 消费端 `bm`。构建端协议见根
[`CLAUDE.md`](../../CLAUDE.md)；本文件只记录 client 中不能从 CLI help 直接得出的约束。

## 不变量

- `bm` 必须可用 `CGO_ENABLED=0` 构建，运行时不调用 `curl`、`tar`、`oras`、`jq` 或 Nix。
- package 和 profile link 必须是相对 symlink，整个 prefix 可直接移动。
- package 彼此独立，client 不解析依赖。
- OCI layer digest 是版本标识；digest 相同则只调和 link 状态，除非指定 `--force`。
- 多包操作先 resolve 全部 tag；任一失败时不得写入安装状态。
- 支持的平台只有 `linux-x86_64` 和 `darwin-arm64`。
- package/profile 名必须匹配 `^[A-Za-z0-9][A-Za-z0-9._-]*$`。

## OCI 协议

package reference 为：

```text
ghcr.io/curoky/standalone-binaries:<package>-<arch>
```

image 包含 tar.gz layer，归档顶层目录是 package 名；client 使用最后一个 layer 的 digest 和内容。
registry 是公开只读源，client 只使用 anonymous auth，不读取 Docker credential config。

`bm` 使用 `binman-<arch>` tag，归档内路径是 `binman/bm`。修改 registry、tag、layer 或归档布局时，
必须同步发布 workflow、`registry.go`、`install.sh` 和根设计文档。

## 状态与事务

安装状态位于 `<prefix>/store/<package>/`，其中 `.binman-meta` 记录 `name`、`arch`、
`digest`、`linked` 和 `installed_at`。prefix root 与 `<prefix>/profile/<profile>/`
只包含指向 store 的聚合 link。下载 cache 不属于安装状态。

安装分三步：

1. bounded parallel resolve，并保留同一次 resolve 得到的 layer；
2. bounded parallel 下载到临时 cache 后原子替换；
3. 串行解压到 staged store，写 metadata，再交换 store 和调和 link。

store 交换或 link 失败时必须恢复旧 store 与旧 link。不同 package 提供同一路径时直接报错，不覆盖
已有 owner。`upgrade` 复用同一批量状态机；`outdated` 并发 resolve，但按 package 名稳定输出。

## Manifest

```yaml
prefix: /opt/binman
arch: linux-x86_64
packages:
  link: [ripgrep]
  unlink: [python314]
profiles:
  go: [gopls, delve]
```

- YAML 只允许一个 document，未知字段直接报错。
- 显式 `--prefix`、`--arch` 优先于 manifest。
- 重复 package 合并为一个 install target，root link 优先。
- profile tree 每次完整 staged rebuild 后原子替换；`remove` 同时清理指向 package 的 profile link。
- `--prune` 通过正常 remove 路径删除 manifest 未引用的 package。

## 安全边界

- tar entry、symlink target、hardlink target 及其既有 symlink parent 都必须留在
  staged store 内；未支持的 entry type 直接报错。
- `.binman-meta` 只能由 client 创建，不能继承归档内容。
- link 前必须完成全包冲突预检；unlink 只删除仍指向该 package 的 symlink。
- link/unlink 拒绝聚合根目录内已有 symlink parent，不能借其访问 prefix/profile 之外的路径。
- 修改解压、link 或事务逻辑时，先添加可复现边界的测试。

## 代码布局

- `main.go`：输入校验、日志和 Cobra wiring。
- `registry.go`：OCI resolve、digest 和原子 cache 下载。
- `store.go`：metadata、安全解压、store 交换和 link ownership。
- `install.go`：安装状态机及 package 命令。
- `manifest.go`：strict YAML、install plan、profile 与 prune。
- `main_test.go`：本地 registry 和临时 prefix 上的行为测试。
- `install.sh`：首次安装 `bm` 的 bootstrap；这是唯一允许依赖宿主 `curl` 和 `tar` 的路径。

保持同一 `package main`，不要为假设中的 backend、registry 或 package graph 预造 interface。

## 验证

```bash
CGO_ENABLED=0 go test ./cmd/binman
CGO_ENABLED=1 go test -race ./cmd/binman
CGO_ENABLED=0 go vet ./cmd/binman
CGO_ENABLED=0 go build ./cmd/binman
bash -n cmd/binman/install.sh
shellcheck cmd/binman/install.sh
```
