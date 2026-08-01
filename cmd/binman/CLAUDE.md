# Binman Agent Guide

`cmd/binman/` 实现 OCI artifact client `bm`。全局产物约束见根
[`CLAUDE.md`](../../CLAUDE.md)；本文件是该组件的设计与修改契约。

## 不变量

- `bm` 使用 `CGO_ENABLED=0` 构建，运行时不调用 `curl`、`tar`、`oras`、`jq` 或 Nix。
- package 和 profile 的叶子 link 都是相对 symlink，整个 prefix 可以直接移动。
  指向 package 内目录的 symlink 展开为聚合目录，使多个 package 可以共享 `sbin`
  等目录。
- package 彼此独立，client 不解析依赖。
- OCI layer digest 是版本标识；digest 相同只调和 link 状态，除非指定 `--force`。
- 多包操作先 resolve 全部 tag；任一失败时不写入安装状态。
- 支持的平台只有 `linux-x86_64`、`linux-arm64` 和 `darwin-arm64`。

## OCI Artifact

包发布为：

```text
ghcr.io/curoky/standalone-binaries:<package>-<architecture>
```

Image 的最后一个 layer 是 tar.gz，归档顶层目录是 package 名。Client 使用 anonymous
auth，不读取 Docker credential config。

`bm` 自身使用 `binman-<architecture>` tag，归档路径是 `binman/bm`。
`install.sh` 是唯一允许依赖宿主 `curl` 和 `tar` 的路径。

## 状态与事务

安装状态位于 `<prefix>/store/<package>/`，`.binman-meta` 记录 package、architecture、
digest、link 状态和安装时间。Prefix 根目录与 `<prefix>/profile/<profile>/` 只包含
指向 store 的聚合 link。

安装事务：

1. 有限并发 resolve，并保留该次 resolve 得到的 layer。
2. 有限并发下载到临时 cache 后原子替换。
3. 串行解压到 staged store，写 metadata，再交换 store 和调和 link。

Store 交换、link 或 profile rebuild 失败时恢复旧状态。不同 package 提供同一路径时
直接报错，不覆盖已有 owner。`upgrade` 复用同一状态机；`outdated` 并发 resolve，
但按 package 名稳定输出。

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
- 显式 `--prefix` 和 `--arch` 优先于 manifest。
- 重复 package 合并为一个 install target，root link 优先。
- Profile tree 每次完整 staged rebuild 后原子替换。
- `remove` 清理关联 profile link；`sync --prune` 删除未引用的 package。

## 安全边界

- Tar entry、symlink target、hardlink target 及既有 symlink parent 必须留在
  staged store 内；未支持的 entry type 直接报错。
- `.binman-meta` 只能由 client 创建，不能继承归档内容。
- Link 前完成全包冲突预检；聚合目录可以合并，叶子路径冲突直接报错；unlink 只删除
  仍由该 package 拥有的 symlink。
- Link 和 unlink 拒绝聚合根目录内已有的外部 symlink parent；旧版 `bm` 创建且仍
  指向当前 prefix store 的目录 symlink 会安全迁移为聚合目录。
- 修改解压、link 或事务逻辑时，先添加能复现边界的测试。

## 实现边界

- `registry.go`：OCI resolve、digest 和原子 cache 下载。
- `store.go`：metadata、安全解压、store 交换和 link ownership。
- `install.go`：安装状态机及 package 命令。
- `manifest.go`：strict YAML、install plan、profile 和 prune。
- `install.sh`：首次安装 bootstrap。

保持同一个 `package main`，不为假设中的 backend、registry 或 package graph 预造
interface。修改 registry、tag、layer、归档布局或 metadata 格式时，同步
`registry.go`、`install.sh` 和发布 workflow。

## 验证

```bash
CGO_ENABLED=0 go test ./cmd/binman
CGO_ENABLED=1 go test -race ./cmd/binman
CGO_ENABLED=0 go vet ./cmd/binman
CGO_ENABLED=0 go build ./cmd/binman
bash -n cmd/binman/install.sh
shellcheck cmd/binman/install.sh
```
