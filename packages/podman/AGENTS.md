# Podman 包设计

本目录把 Podman 拆成两个并行版本，共享同一套 bin/conf/patch 资源，但每个版本用
一个完全自包含的 derivation 文件，不抽象公共 nix 逻辑：

- [`podman5.nix`](podman5.nix)：跟随上游 nixpkgs pin 的 podman 5.x（当前
  5.8.4），不 override `version`/`src`/`vendorHash`，复用 nixpkgs 拉取的源码与
  module 集。
- [`podman6.nix`](podman6.nix)：把 podman 6.x pin 到具体 release（当前 6.1.0），
  自行 override `version`/`src` 并把 `vendorHash` 设为 `null`（6.1.0 源码自带
  committed vendor/）。

两个 `.nix` 文件的 `runcStatic`、`podman.override`、patch、`postInstall` 与
`installCheckPhase` 目前内容一致，但刻意各自完整维护，改动其一时需手动同步另一个。

两个版本各自将 Podman 发布为可整体移动的目录。所有运行时依赖都应尽可能打包到
`libexec/podman`，而不是从宿主系统获取。以下运行时查找契约与打包约定对两者同时
适用。

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
系统路径列表。v5.8.x 与 v6.1.0 的 vendor 路径都已是 `go.podman.io/common`，两个版本
复用同一份 patch。

`policy.json` 的定位只用 `bin/podman`/`bin/podman-server` 设置的
`CONTAINERS_POLICY_JSON=$root/../conf/policy.json`。podman 6.x 上游已原生读取该 env，
podman 5.x 未支持，故 `podman5.nix` 额外应用 `policy-json-env.patch`，给
`vendor/go.podman.io/image/v5/signature/policy_config.go` 的 `defaultPolicyPathWithHomeDir`
backport 同样的 env 覆盖（优先级低于 `sys.SignaturePolicyPath`、高于用户与系统默认路径，
值原样使用、缺失即报原始 ENOENT 而不回退）。`podman6.nix` 不需要该 patch。

## 打包约定

两个 `.nix` 文件都将上游 helper bundle 复制到 `libexec/podman`，并把字面量
`$BINDIR/../libexec/podman` 作为查找模板链接进 Podman。新增运行时依赖时，应尽可能
把它加入该 bundle，并复用已有共享 resolver，不要增加面向宿主系统的包级查找逻辑。

helper bundle 里的 `aardvark-dns` 由两个 `.nix` 文件通过
`podman.override { aardvark-dns = ...; }` 换成本地 patch 版
（[`packages/aardvark-dns/`](../aardvark-dns/)）。上游 aardvark-dns 2.1.0 在
`src/main.rs` 无条件调用 `libc::close_range`，musl 的 libc 绑定只导出
`SYS_close_range` 常量而没有该 wrapper 函数，musl-static 构建会失败；patch 改用
`libc::syscall(libc::SYS_close_range, ...)`，仅动 crate 自身源码、不动 vendor，故不
需要 cargoHash override。改动其一时需同步另一个。

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
