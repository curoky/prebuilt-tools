---
name: "regress-patched-package-to-upstream"
description: "定期 review 本仓库中被 patch 或被 pin 老版本的包，把「临时的编译修复」尽可能切回 unstable channel 的上游包，去掉本地 patch/老版本 pin，改在 manifests/default.nix 里维护。当 review/更新 packages/ 或 manifests/ 条目、bump nixpkgs、或审计本地 patch/版本 pin 是否仍必要时调用。"
---

# Regress a Patched / Pinned Package Back to Upstream

本仓库的**长期目标是所有包都用官方维护的 `unstable` channel 最新版**（见 `CLAUDE.md`
的「版本策略」）。凡是偏离这个目标的东西——`packages/` 下的本地 patch 包、或
`manifests/default.nix` 里被 pin 在老 `version` 的条目——都应视为**临时状态**，
定期复查，一旦上游已修好就切回上游。

本 skill 就是干这件事的：判定某个 patch/pin 是否已过时，并干净地执行回归。

先读 `CLAUDE.md`——patch 策略、版本策略、package-selection 模型都在那里。
本 skill 是 `patch-nixpkgs-standalone` 的逆操作：那个在 stock 静态构建失败时**加**
workaround；这个在 stock 构建重新可用时**去掉**它。

## 两类回归对象

本 skill 覆盖两种「偏离 unstable 上游」的情况，判定与回归方式不同：

### A. 本地 patch 包（`packages/<pkg>/`）

`packages/` 下的本地 derivation 只应因两种原因存在：

- **Workaround（regression 候选）**——绕过一个 `nixpkgs`（通常是 `pkgsStatic.<x>`）
  的构建/链接失败或 portability 问题。这类是临时的编译修复，上游修好后就该切回。
  例如 [packages/diffutils/default.nix](file:///workspace/standalone-binaries/packages/diffutils/default.nix)
  只是 `doCheck = false` 关掉一个失败的 check——**属于此类，需要评估能否切回上游**。
- **Packaging decision（不是候选）**——刻意的 repackaging，改变了「这个包是什么/如何被消费」。
  例如 [packages/cloc/default.nix](file:///workspace/standalone-binaries/packages/cloc/default.nix)
  把 `cloc` rename 成 `_cloc` 并放一个调用同级 `perl` 的 wrapper、vendor 它的 Perl 依赖——
  这种 **wrapper 类的包不需要切回上游**，无论上游怎么修都不会消失。

### B. 被 pin 老版本的 manifest 条目（`manifests/default.nix`）

manifest 里任何写了非 `unstable` `version` 的条目（无论包级还是 per-platform key），
都是因为「当时 unstable 最新版构建失败/不 portable」才临时 pin 的老 channel。例如：

```nix
shellcheck = {
  "aarch64-darwin" = {
    version = "25.11";
  };
};
```

这类要**试着去掉 `version`（即回到 unstable）**，只要 unstable 最新版现在能构建且 portable，
就删掉这个 pin。

## Principle

**只要上游能构建且满足 portability 目标，就优先用 unstable 上游包。**
本地包和老版本 pin 都是负债（维护成本、偏离上游、diff 更大）。若
`nixpkgs.pkgsStatic.<x>`（unstable）现在能编译、链接、并通过 portability 检查，
就删掉本地 patch / 老版本 pin，改用 manifest 里的 unstable 上游。

## First：这个东西是不是 regression 候选？

判定的关键问题是**假如上游完全没有 bug，这个 override / pin 会变成什么样。**

- **A 类本地 patch**——若一个无 bug 的上游会让 override *变空*（你没东西可写了），
  它是纯 **workaround**，是 regression 候选。它存在的全部理由就是一个上游缺陷；
  缺陷没了，override 就没内容了。
  - 反之，若无 bug 的上游*仍然*给 override 留着活干，它编码的是一个**刻意的
    packaging 决策**（wrapper、rename 二进制、bundle sibling 依赖），永远不是候选。
  - litmus test：假想上游完美，问「override 现在空了吗？」空了 ⇒ 候选；还有活干 ⇒ 别动。
    看不出意图（代码和注释都不清楚）时，先问再动。
  - 一个 override 可能**两者兼有**（一部分 workaround + 一部分 packaging）。只有
    workaround 部分可回归；保留 packaging 部分。见 step 4 的「partial regression」。

- **B 类版本 pin**——所有非 `unstable` 的 `version` pin 天生就是 workaround（当时上游最新版有问题），
  一律是候选。目标是去掉 pin、回到 unstable。

## When to run this

- 定期 review / 审计 `packages/` 与 `manifests/default.nix` 的条目。
- bump 了某个 `nixpkgs` input（`flake.nix` 里的 `unstable`/`26.05`/...）之后。
- 某个包的 `default.nix`/`darwin.nix` 注释说它在绕过某个具体上游 bug——查那个 bug 是否已消失。
- 看到 manifest 里有非 `unstable` 的 `version` pin。

## Procedure

1. **搞清 patch/pin 为何存在。**
   - A 类：读本地 `packages/<pkg>/*.nix` 及其注释。先用上面「regression 候选？」分类——
     若是刻意 repackaging（wrapper、rename 二进制、bundle sibling 依赖，如
     `packages/cloc`），**停手**，不可回归。否则注释几乎总会点名确切的失败
     （如 "darwin `pkgsStatic.perl` fails at `mktables`"、"`liboapv` ships only a `.dylib`"、
     或 diffutils 的 check 失败）。那个失败就是你的 regression test。
   - B 类：找出这个包/平台当初为何被 pin 到老 `version`（看注释或 git 历史）。它的
     regression test 就是「unstable 最新版能否构建 + portable」。

2. **测 stock upstream 构建。** 为目标平台构建 patch/pin 要替换掉的那个纯上游 derivation：
   - 全静态路线：`nix build nixpkgs#pkgsStatic.<x>`（对应 manifest 会用的 unstable input）。
   - A 类：若这正是 patch 绕过的东西、现在**能构建且能链接**，patch 是移除候选。
   - B 类：直接改 manifest 去掉 `version` pin（或临时本地测 unstable 的 `pkgsStatic.<x>`），
     看能否构建。
   - 在包**目标的每个平台**（Linux + darwin）上复现。一个平台修好不代表另一个也修好。

3. **验证上游构建仍满足 portability 目标**（与 `patch-nixpkgs-standalone` 同一套检查）：
   - **Linux（硬底线）：** `file`/`ldd` → musl 纯静态、无 glibc、无 `.so`、无 `/nix` 运行期引用。
   - **macOS：** `otool -L` 检查二进制及每个 ship 的 `.dylib`/`.bundle`/`.so` → 只有
     `/usr/lib/*`、`/System/Library/Frameworks/*` 或 `@loader_path` 条目；**零** `/nix/store`。
   - 若上游能构建但仍不 portable，则 patch/pin **尚未**过时——保留它。只有在上游
     **既可构建又 portable** 时才回归。

4. **执行回归**（验证通过后）：
   - **A 类，整包可用上游：** **删除** `packages/<pkg>/`，从
     [packages/local.nix](file:///workspace/standalone-binaries/packages/local.nix) 的
     `common`/`linux`/`darwin` 集合里移除它的 `callPackage ./<pkg>` 行，然后在
     [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix)
     里加/调条目（`isStatic = true`，正确的 `version`（省略 => unstable）/`platforms`/`output`/`alias`）。
   - **A 类，只有一个平台修好：** 只删那个平台的本地文件（如删 `darwin.nix`、留 `default.nix`），
     把该平台切到 manifest/上游；仍需 patch 的平台保留。
   - **A 类，partial regression：** 若 patch 只*部分*过时（如 feature-reduction override 里
     某个被禁的特性已能用），把 override 缩到仍必需的最小集，而不是删整个包。
   - **B 类，去掉版本 pin：** 在 `manifests/default.nix` 里删掉那条 `version = "..."`（回到 unstable）。
     若这个 pin 只在某个 per-platform key 下、且去掉后该 key 变空，就把整个 per-platform key
     也删掉；若删掉后整个包条目变成 `{ }`，保留 `<pkg> = { };` 即可。

5. **清理你的改动产生的 orphan：** 不再被用到的 patch 文件、wrapper 脚本、
   `packages/<pkg>/` 下的 vendor 配置、以及失效的 `callPackage` wiring。
   不要删与本次改动无关的既有代码。

6. **重建受影响包的最终 standalone 产物**，重跑 step 3 的验证，确认回归后的构建仍 portable。

7. **同步更新 `CLAUDE.md`**（同一次改动），如果回归改变了 package selection 模型
   （如某个包从 `packages/` 本地 override 变成 manifest 条目、或某个 per-platform 策略不再适用）。

## Guardrails

- 只有 bug/portability 的 workaround 和版本 pin 可回归。绝不删刻意的 repackaging override
  （wrapper 脚本、rename 二进制、sibling/bundled 依赖）——见「这个东西是不是 regression 候选？」。
  上游修构建 bug 不会让 repackaging 过时。
- 不要凭假设回归——总是先在**所有**目标平台上构建并验证上游。
- 只有上游**既可构建又 portable** 时才回归。可构建但不 portable 不是去掉 patch/pin 的理由。
- diff 要外科式：只移除过时 patch/pin 拥有的东西。
- 拿不准某个平台是否真修好（如本地无法测 darwin），就说明并先问，再删那个平台的 patch/pin。
