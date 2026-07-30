# `bm` 设计与协议

`bm` 是 `cmd/binman/` 实现的 OCI artifact client，也可根据 YAML manifest 调和完整
安装状态。命令和参数以 `bm --help` 为准。

## 不变量

- 二进制使用 `CGO_ENABLED=0` 构建，运行时不调用外部下载或归档工具。
- package 和 profile link 都是相对 symlink，整个 prefix 可以直接移动。
- package 彼此独立，client 不解析依赖。
- OCI layer digest 是版本标识；digest 相同则只调和 link 状态，除非指定 `--force`。
- 多包操作先 resolve 全部 tag；任一失败时不写入安装状态。
- 支持的平台只有 `linux-x86_64` 和 `darwin-arm64`。

## Artifact 与状态

包发布为 `ghcr.io/curoky/standalone-binaries:<package>-<architecture>`。Image 的最后
一个 layer 是 tar.gz，归档顶层目录是 package 名；client 使用 anonymous auth。

`bm` 自身使用 `binman-<architecture>` tag，归档路径是 `binman/bm`。首次安装脚本
`cmd/binman/install.sh` 是唯一允许依赖宿主 `curl` 和 `tar` 的路径。

安装状态位于 `<prefix>/store/<package>/`，`.binman-meta` 记录版本和 link 状态。Prefix
根目录与 `<prefix>/profile/<profile>/` 只包含指向 store 的聚合 link。安装事务：

1. 有限并发 resolve，并保留该次 resolve 得到的 layer。
2. 有限并发下载到临时 cache 后原子替换。
3. 串行解压到 staged store，写 metadata，再交换 store 和调和 link。

Store 交换或 link 失败时恢复旧 store 与旧 link。不同 package 提供同一路径时直接
报错，不覆盖已有 owner。

## Manifest

```yaml
prefix: /opt/binman
arch: linux-x86_64
packages:
  link:
    - ripgrep
  unlink:
    - python314
profiles:
  go:
    - gopls
    - delve
```

- YAML 只允许一个 document，未知字段直接报错。
- 显式 `--prefix` 和 `--arch` 优先于 manifest。
- 重复 package 合并为一个 install target，root link 优先。
- Profile tree 每次完整 staged rebuild 后原子替换。
- `remove` 清理关联 profile link；`sync --prune` 删除未引用的 package。

## 安全边界

- Tar entry、link target 及既有 symlink parent 都必须留在 staged store 内。
- `.binman-meta` 只能由 client 创建，不能继承归档内容。
- Link 前完成全包冲突预检；unlink 只删除仍指向该 package 的 symlink。
- Link 和 unlink 拒绝聚合根目录内已有 symlink parent，避免借其访问 prefix 或
  profile 之外的路径。

## 实现导航

`registry.go` 负责 OCI 和下载，`store.go` 负责安全解压与 link ownership，
`install.go` 负责事务，`manifest.go` 负责声明式调和。代码保持同一个
`package main`，不预造 backend、registry 或 package graph 抽象。
