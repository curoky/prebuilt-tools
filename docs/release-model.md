# 包发布模型

Linux 和 Darwin workflow 共享同一发布契约：按当前 `outPath` 发现缺失包，构建 standalone
目录及归档，再分别发布 Nix cache 和工具 OCI artifact。

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

cache segment 使用 matrix package 名作为稳定的 `NIXCACHE_PACKAGE_KEY`。prune 据此在增量 run
中按 package 保留最新 segment；不得改用 run ID 或 snapshot 内的 tag 数量表示包 identity。

## Cache 命中判定

命中判定只使用当前 `flake.lock` 和包定义产生的确切 `outPath`。探测成功则跳过，探测失败则
构建。以下情形跳过 cache 探测并强制构建候选范围：

- `schedule` 触发（每周定时全量刷新）；
- `workflow_dispatch` 且 `skip_discover=true`。

## 触发矩阵

`discover` 与 `build` job 按下表联动。`skip_discover` 是 `workflow_dispatch` 的布尔输入；
`name` 是包名字符串输入（`*` 或空表示全部包）。

| 触发 | `discover` job | 候选范围 | cache 命中过滤 | build matrix 来源 |
| --- | --- | --- | --- | --- |
| `push` | 运行 | 全部包 | 做 | `discover` 输出 |
| `schedule` | 运行 | 全部包 | 跳过（强制重建） | `discover` 输出 |
| dispatch 全部，普通 | 运行 | 全部包 | 做 | `discover` 输出 |
| dispatch 全部，强制 | 运行 | 全部包 | 跳过 | `discover` 输出 |
| dispatch 具体包 | 跳过 | 仅该包 | 不适用 | `inputs.name` |

具体包 dispatch 直接构造单包 matrix，不做 cache 探测。新增触发方式或改变选择语义时，必须
同步 Linux、Darwin workflow 和本表。

LLVM 工具 workflow 按版本矩阵直接构建，不经过普通包的 `discover`，但使用相同的 artifact
和 cache 发布契约。
