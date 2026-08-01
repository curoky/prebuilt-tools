# Podman 包设计

本包将 Podman 发布为可整体移动的目录。所有运行时依赖都应尽可能打包到
`libexec/podman`，而不是从宿主系统获取。

## 运行时查找契约

运行中的 `_podman` 根据自身位置确定 `BINDIR`，并且只从以下目录解析随包提供的
二进制：

```text
$BINDIR/../libexec/podman
```

该规则覆盖 OCI runtime、`conmon`、`conmonrs`、容器 init、网络 helper、Quadlet，
以及所有通过 Podman helper 查找 API 解析的二进制。

- 整体移动包目录后，查找行为必须保持不变。
- 用户配置、`CONTAINERS_HELPER_BINARY_DIR`、系统目录和 `PATH` 都不得改变随包
  二进制的查找位置。
- 随包二进制缺失时，要求它的调用点必须报错；Podman 不得回退到宿主系统中的同名
  二进制。
- 未设置 link-time helper 目录的非本包构建仍应保持上游查找行为。

`strict-helper-search.patch` 在共享 resolver 边界实现该契约。包级 patch 不得重新引入
系统路径列表。

## 打包约定

`default.nix` 将上游 helper bundle 复制到 `libexec/podman`，并把字面量
`$BINDIR/../libexec/podman` 作为查找模板链接进 Podman。新增运行时依赖时，应尽可能
把它加入该 bundle，并复用已有共享 resolver，不要增加面向宿主系统的包级查找逻辑。

`bin/podman` 和 `bin/podman-server` 必须根据自身位置设置配置文件路径、
`PODMAN_DATA_DIR` 及 `data/tmpdir` 下的 `TMPDIR`。`storage.conf` 通过
`PODMAN_DATA_DIR` 把 `graphroot` 和 `runroot` 定位到 sibling `data` 目录；
配置文件不得写死安装前缀。

`bin/install.sh` 原地安装当前包目录，不得复制或移动 `bin`、`conf`、`libexec` 和
`data`。它只创建 sibling `data`，并从 `conf/podmanxd.service` 模板渲染当前包根目录
的真实路径后注册 systemd unit。

`installCheckPhase` 应聚焦 resolver 行为，验证 relocation、sibling 目录查找成功，以及
无法逃逸到外部二进制。至少覆盖 conmon 专用 resolver、OCI runtime resolver 和通用
helper resolver。检查应直接调用 resolver，不得通过启动完整 Podman runtime 间接触发；
build sandbox 的 kernel、namespace、storage 或 cgroup 状态不属于该契约。
