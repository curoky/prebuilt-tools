# 发布与 Cache 模型

`.github/workflows/build.yaml` 是 Linux 和 Darwin 的统一发布入口。它按平台调用
`.github/workflows/build-platform.yaml`，各平台独立按当前 `outPath` 发现缺失包，构建
standalone 目录及归档，再分别发布 Nix cache 和工具 OCI artifact。

平台与 runner 映射固定为：

| Nix system | Runner | Artifact suffix |
| --- | --- | --- |
| `x86_64-linux` | `ubuntu-latest` | `linux-x86_64` |
| `aarch64-linux` | `ubuntu-24.04-arm` | `linux-arm64` |
| `aarch64-darwin` | `macos-26` | `darwin-arm64` |

每个平台使用独立的动态 package matrix，不能把多个平台的包展开到同一个 matrix。这样既让
平台失败相互独立，也避免触及 GitHub Actions 单个 matrix 的 256 job 上限。

Nix cache 的 segment schema、serving 和 retention 细节见
[`cmd/nixcache/AGENTS.md`](../cmd/nixcache/AGENTS.md)。

## 发布流程

1. `discover` eval 当前平台的包名和 `outPath`，排除聚合输出。
2. 本地 `nixcache serve` 作为 GHCR-backed substituter，按 `outPath` 检查 cache。
3. 未命中的包进入 build matrix。
4. 每个包构建 `packages.<system>.<name>` 和
   `tarballs.<system>.<name>`。两者是同一 multi-output derivation 的 `out` 与
   `archive` output。
5. standalone closure 推送到 cache repository；归档发布为
   `ghcr.io/curoky/standalone-binaries:<name>-<arch>`。
6. `summary` 汇总发现数量和各 matrix leg 的最终状态。

归档必须由 `lib/make-artifacts.nix` 在 Nix build 内生成。workflow 不得自行重新打包或修改
归档内容。

Cache segment 使用 matrix package 名作为稳定的 `NIXCACHE_PACKAGE_KEY`。Prune 据此在增量
run 中按 package 保留最新 segment；不得改用 run ID 或 snapshot 内的 tag 数量表示 package
identity。

## Cache 命中判定

命中判定只使用当前 `flake.lock` 和包定义产生的确切 `outPath`。探测成功则跳过，探测失败则
构建。以下情形跳过 cache 探测并强制构建候选范围：

- `schedule` 触发（每周定时全量刷新）；
- `workflow_dispatch` 且 `skip_discover=true`。

本地 cache 不签名。Workflow 仅将固定的 loopback URL
`http://127.0.0.1:37515?trusted=true` 作为 trusted substituter；不得通过
`require-sigs=false` 全局接受其他未签名 cache。`NIXCACHE_STORE` 保留无 query 的 URL，
只用于 readiness HTTP 请求；Nix 命令使用 `NIXCACHE_SUBSTITUTER`。

## 触发矩阵

`push` 和 `schedule` 始终运行全部三个平台。`workflow_dispatch` 的 `platform` 输入可选择
`all`、`x86_64-linux`、`aarch64-linux` 或 `aarch64-darwin`，默认 `all`。
`skip_discover` 是 `workflow_dispatch` 的布尔输入；`name` 是包名字符串输入（`*` 或空
表示全部包）。

| 触发 | `discover` job | 候选范围 | cache 命中过滤 | build matrix 来源 |
| --- | --- | --- | --- | --- |
| `push` | 运行 | 全部包 | 做 | `discover` 输出 |
| `schedule` | 运行 | 全部包 | 跳过（强制重建） | `discover` 输出 |
| dispatch 全部，普通 | 运行 | 全部包 | 做 | `discover` 输出 |
| dispatch 全部，强制 | 运行 | 全部包 | 跳过 | `discover` 输出 |
| dispatch 具体包 | 跳过 | 仅该包 | 不适用 | 入口预检后的 `inputs.name` |

具体包 dispatch 先在入口 workflow 验证所选平台是否暴露该包，再为可用平台直接构造
单包 matrix，不做 cache 探测。不存在该包的平台显示为 skipped，其他平台继续；如果所有
所选平台都不可用，入口 job 失败，避免错误包名静默成功。新增触发方式或改变选择语义时，
必须同步两个 build workflow 和本表。

Node.js 运行时和同级 runtime 工具（`nodejs-slim*`、`markdownlint-cli2`、`opencommit`、
`pnpm`、`prettier`）以及 `nil`、`nixfmt`、`shellcheck`、`gdb`、`clang-tools-{18..22}`
编译慢，仅在 `push` 触发时通过平台配置的 `push_exclude_pkgs` 从候选中排除，避免拖慢普通
代码 push。其中 `nil`、`nixfmt`、`clang-tools-*` 只在 Linux 暴露，Darwin 的排除列表相应
更短。`schedule` 和 `workflow_dispatch` 不受此排除影响，仍会构建并发布它们。改动
`push_exclude_pkgs` 时须同步入口 workflow 和本说明。

`clang-tools-{18..22}`（原 `build-llvm-tools.yaml`）已并入 Linux workflow 的普通
`discover` / build matrix，与其他包共用 cache 命中过滤和 artifact 发布契约。它们是
LLVM/clang 大型构建，build job 仅对 `clang-tools-*` 的 matrix leg 额外执行释放磁盘和
配置 swap 的准备步骤，且已随上文列入 `PUSH_EXCLUDE_PKGS`（push 时跳过）。`lld_*` 与
`clang*` 仍通过 `EXCLUDE_PKGS` 排除（只作为构建输入暴露，不单独发布）。
`.github/workflows/prune-nix-cache.yaml` 只允许手动触发。
