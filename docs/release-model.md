# 包发布模型

普通 Linux（`build-linux.yaml`）和 Darwin（`build-darwin.yaml`）workflow 的发布流程、
cache 命中判定和各 CI 触发条件下的排列组合。LLVM 工具的专用 workflow（`build-llvm-tools.yaml`）
按 version 矩阵直接构建、无 `discover` job，但复用同一套 cache 流程。

## 发布流程

每个 workflow 分 `discover` 和 `build` 两个 job：

1. `discover` 一次 `nix eval` 当前平台全部包名与 `outPath`（排除 `all`、`all-fast`
   聚合），产出 `<name>\t<outPath>` 的 TSV。
2. 通过 `cmd/nixcache/install.sh` 下载已发布的 `nixcache`，启动本地 GHCR-backed
   substituter（`127.0.0.1:37515`）。
3. `discover` 按[触发矩阵](#触发矩阵)决定候选范围与是否做 cache 命中过滤，把需要重建的
   包名写入 build matrix；命中 cache 的包不进 matrix。
4. `build` 对 matrix 中每个包，以本地 substituter 执行 `nix build .#<name>`（standalone
   产物）与 `nix build .#tarballs.<system>.<name>`（发布归档），unsigned cache 仅在 workflow
   命令上显式设置 `require-sigs=false`。
5. 归档由 `lib/make-tarball.nix` 在 Nix 内生成，workflow 只 `cp -L` 出
   `<name>.linux-x86_64.tar.gz` 或 `<name>.darwin-arm64.tar.gz`，不依赖宿主 `rsync`/`tar`。
6. 用 `nixcache push` 把 standalone closure 发布到 cache repository，再把工具 tarball 发布到
   `ghcr.io/curoky/standalone-binaries:<name>-<arch>`。

## Cache 命中判定

`discover` 对每个候选包用 `nix path-info --store <本地 substituter> --option require-sigs false <outPath>`
探测：

- 探测成功 = cache 命中：该包已发布，跳过，不进 build matrix。
- 探测失败 = cache 未命中：该包需要重建，进 build matrix。

命中判定基于 `outPath`，即当前 `flake.lock` 与包定义 eval 出的确切 store path；上游或本地
包定义变化会改变 `outPath`，从而自然表现为未命中。

**强制重建**指跳过上述探测、把候选范围内每个包都当作未命中直接进 matrix。以下情形强制重建：

- `schedule` 触发（每周定时全量刷新）；
- `workflow_dispatch` 且 `skip_discover=true`。

## 触发矩阵

`discover` 与 `build` job 按下表联动。`skip_discover` 是 `workflow_dispatch` 的布尔输入；
`name` 是包名字符串输入（`*` 或空表示全部包）。

| 触发 | `discover` job | 候选范围 | cache 命中过滤 | build matrix 来源 |
| --- | --- | --- | --- | --- |
| `push` | 运行 | 全部包 | 做 | `discover` 输出 |
| `schedule` | 运行 | 全部包 | 跳过（强制重建） | `discover` 输出 |
| `workflow_dispatch`，`name` 为空或 `*`，`skip_discover=false` | 运行 | 全部包 | 做 | `discover` 输出 |
| `workflow_dispatch`，`name` 为空或 `*`，`skip_discover=true` | 运行 | 全部包 | 跳过（强制重建） | `discover` 输出 |
| `workflow_dispatch`，`name` 为具体包 | 整个跳过 | 仅该包 | 不适用 | 由 `inputs.name` 直接构造单包 matrix |

要点：

- `name` 为具体包时 `discover` 整个 job 被 skip，`skip_discover` 无意义；该包由 `build`
  直接构建，不做 cache 探测。
- `schedule` 恒等于强制重建，与 `skip_discover` 无关。
- `build` job 用 `always()` + `needs.discover.result` 区分 `discover` 是运行选包（用其
  输出的 matrix）还是被跳过（用 `inputs.name` 构造单包 matrix）。

## 实现锚点

- `discover` job 的 `if`：`workflow_dispatch` 且 `name` 为具体包时整个跳过。
- `discover` 的 shell 分支：`skip_discover=true` 或 `schedule` 时 `cut -f1 pkgs.tsv`
  取全部候选，否则并行 `nix path-info` 过滤。
- `build` job 的 `if` 与 `matrix`：`always()` + `needs.discover.result` 判断。

新增触发方式或调整跳过逻辑时以本文触发矩阵为准，并同步 `build-linux.yaml` 与
`build-darwin.yaml` 两个 workflow 的对应表达式。
