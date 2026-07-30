# 贡献与验证

先从最小改动入口开始，并保持[产物不变量](architecture.md#产物不变量)。默认使用
unstable；所有 pin、patch、override、禁用检查、结构性 packaging 和动态例外都登记到
[`TODO.md`](../TODO.md)。

## 修改入口

| 需求 | 修改位置 |
| --- | --- |
| 接入可直接使用的 nixpkgs 包 | `manifests/default.nix` |
| 添加 patch、wrapper 或平台拆分 | `packages/<name>/`、`packages/local/<platform>.nix` |
| 登记或回归本地定制 | `TODO.md`、manifest、`packages/local/` |
| 修改产物后处理与校验 | `cmd/artifact/`、`lib/make-artifacts.nix` |
| 修改包选择或 flake outputs | `lib/`、`flake.nix` |
| 修改 `bm` | `cmd/binman/`，并阅读 [`binman.md`](binman.md) |
| 修改 Nix cache | `cmd/nixcache/`，并阅读 [`release-model.md`](release-model.md) |

## 新增或修复包

1. 先尝试在 `manifests/default.nix` 使用 unstable `pkgsStatic`。
2. 构建并检查可执行文件、wrapper、资源、证书和同级 runtime。
3. 默认路径失败时才创建本地 derivation，并在 `TODO.md` 记录回归条件。
4. 动态 ELF 例外或新宿主依赖必须先确认，不能放宽全局校验。

处理静态构建失败时使用
[`patch-nixpkgs-standalone`](../.trae/skills/patch-nixpkgs-standalone/SKILL.md)。
回归本地 patch 或版本 pin 时使用
[`regress-patched-package-to-upstream`](../.trae/skills/regress-patched-package-to-upstream/SKILL.md)，
并以 `TODO.md` 为候选清单。

## 验证

先验证目标，再扩大范围：

```bash
nix build .#<name>
nix build .#tarballs.<system>.<name>
file result/bin/*
nix flake check
nix build .#all-fast
```

修改 Go 组件时将 `<component>` 替换为 `artifact`、`binman` 或 `nixcache`：

```bash
CGO_ENABLED=0 go test ./cmd/<component>
CGO_ENABLED=0 go vet ./cmd/<component>
CGO_ENABLED=0 go build ./cmd/<component>
```

`binman` 另跑 race test；`binman` 和 `nixcache` 还需检查 `install.sh`。修改
`nixcache` 时构建 Linux 与 Darwin 目标：

```bash
CGO_ENABLED=1 go test -race ./cmd/binman
bash -n cmd/<component>/install.sh
shellcheck cmd/<component>/install.sh
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> go build ./cmd/nixcache
NIXCACHE_TEST_STORE_PATH=/nix/store/<path> \
  CGO_ENABLED=0 go test -run '^TestNixRoundTrip$' -v ./cmd/nixcache
```

Wrapper、证书和同级 runtime 不能只靠 build 成功判断；补充 `--version` 或代表性
smoke test。不要把 eval、dry-run、lint 或代码审查表述为实际构建通过。

文档按职责维护：`README.md` 面向用户，`architecture.md` 记录设计，
`package-strategies/` 解释非显然策略，`CLAUDE.md` 只放 agent 约束，`TODO.md`
记录定制与回归状态。仓库内使用相对链接；实现参数和包清单以代码为准。
