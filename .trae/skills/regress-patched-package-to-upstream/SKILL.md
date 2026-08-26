---
name: "regress-patched-package-to-upstream"
description: "定期 review 本仓库中被 patch 或被 pin 老版本的包，把「临时的编译修复」尽可能切回 unstable channel 的上游包，去掉本地 patch/老版本 pin，改在 manifests/default.nix 里维护。当 review/更新 packages/ 或 manifests/ 条目、bump nixpkgs、或审计本地 patch/版本 pin 是否仍必要时调用。"
---

# 回归 patched / pinned 包到上游

本仓库长期目标是优先使用官方维护的 unstable 最新版。根 `AGENTS.md` 是版本策略、
平台约束和现有例外的 source of truth；`docs/regression/` 是 pin、patch 与本地 packaging 的总账，
也是待审计候选的唯一队列。本 skill 只定义如何消费其中的回归候选并移除已经失效的 workaround。

本 skill 是 `patch-nixpkgs-standalone` 的逆操作：那个在 stock 静态构建失败时**加**
workaround；这个在 stock 构建重新可用时**去掉**它。

## 两类回归对象

本 skill 覆盖两种「偏离 unstable 上游」的情况，判定与回归方式不同：

### A. 本地 patch 包（`packages/<pkg>/`）

`packages/` 下的本地 derivation 只应因两种原因存在：

- **Workaround（regression 候选）**——绕过一个 `nixpkgs`（通常是 `pkgsStatic.<x>`）
  的构建/链接失败或 portability 问题。这类是临时的编译修复，上游修好后就该切回。
  例如 `packages/diffutils/default.nix` 只是 `doCheck = false` 关掉一个失败的
  check，属于此类。
- **Packaging decision（不是候选）**——刻意的 repackaging，改变了「这个包是什么/如何被消费」。
  例如 `packages/cloc/default.nix` 把 `cloc` rename 成 `_cloc` 并放一个调用同级
  `perl` 的 wrapper、vendor 它的 Perl 依赖——
  这种 **wrapper 类的包不需要切回上游**，无论上游怎么修都不会消失。

### B. 被 pin 老版本的 manifest 条目（`manifests/default.nix`）

manifest 里任何写了非 `unstable` `version` 的条目（无论包级还是 per-platform key），
默认都按临时 pin 审计，但先通过附近注释和 git history 确认原因。例如：

```nix
shellcheck = {
  "aarch64-darwin" = {
    version = "25.11";
  };
};
```

这类要试着去掉 `version`。只要 unstable 最新版在目标平台能构建且满足根 `AGENTS.md`
的约束，就删除 pin。

## Principle

**只要上游能构建且满足当前产物目标，就优先用 unstable 上游包。**
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
    workaround 部分可回归；保留 packaging 部分。见 step 5 的「partial regression」。

- **B 类版本 pin**——所有非 `unstable` 的 `version` pin 都进入审计候选。确认它不是刻意的
  长期兼容选择后，目标是去掉 pin、回到 unstable。

## Queue Contract

- `docs/regression/` 按平台/架构拆成多张 Markdown 表：`linux.md`（跨架构共享）、
  `linux-aarch64.md`（aarch64 特有差异）、`darwin.md`（macOS），约定见 `README.md`。批量回归只遍历
  「回归」列为 `✅` 或 `🟡` 的行，默认按表格顺序处理：

  ```bash
  rg '^\| .+ \| (✅|🟡)' docs/regression/*.md
  ```

- 不在批量执行时重新扫描 `packages/` 或 `manifests/default.nix` 猜测候选。仓库扫描只用于维护
  `docs/regression/`、核对队列完整性，或用户明确要求全量审计时。
- 每次只消费一行。先读包名、Linux、macOS、回归、`Linux 原因与保留边界`、`macOS 原因与保留边界`、
  回归判据、`Linux commit`、`macOS commit`、来源，再读取实现。表内判据是最低验收条件，不替代根
  `AGENTS.md` 的通用 portability 检查。原因与保留边界按平台拆成两列，只读目标平台那一列。
- `Linux commit` / `macOS commit` 记录该平台最后一次回归测试时 `flake.lock` 里 `nixpkgs-unstable`
  的 rev（短 hash），未测过填 `—`。消费前先对比该列与当前 `flake.lock` 的 unstable rev：若相同，说明
  该平台在当前 channel 已验证过、本轮可跳过；rev 不同（含 `—`）才需要重新构建验证。
- `✅` 可以尝试整项回到 unstable；`🟡` 只回归对应平台原因栏所述 workaround，保留 wrapper、资源打包、
  产品行为与多版本发布；`❌` 不进入批量回归；`⏳` 只做长期例外审计。
- `✅` 和 `🟡` 只表示值得验证，不表示当前已经可以安全回归。仍需实际构建和 portability 验证。
- 回归失败时保留状态；若得到更准确的失败证据，更新原因和判据。不要用 checkbox 表示“已测试”。

新增或改变 pin、本地 derivation、override、禁用检查或例外，由 `patch-nixpkgs-standalone` skill
负责同步维护表格。bump nixpkgs 后，应再次消费现有候选，而不是生成另一份临时清单。

## Procedure

1. **从队列选择一个条目。**
   - 批量模式取 `docs/regression/` 中下一条 `✅` 或 `🟡` 行；用户指定包时，从对应表格定位该行。
   - 若指定的 pin、patch 或本地 packaging 不在表格，先补齐该行和正确状态再继续。
   - 按「来源」读取实现，确认当前代码仍与「原因与保留边界」一致。表格陈旧时先修正表格。

2. **搞清 patch/pin 为何存在。**
   - A 类：读本地 `packages/<pkg>/*.nix` 及其注释。先用上面「regression 候选？」分类——
     若是刻意 repackaging（wrapper、rename 二进制、bundle sibling 依赖，如
     `packages/cloc`），**停手**，不可回归。否则注释几乎总会点名确切的失败
     （如 "darwin `pkgsStatic.perl` fails at `mktables`"、"`liboapv` ships only a `.dylib`"、
     或 diffutils 的 check 失败）。那个失败就是你的 regression test。
   - B 类：找出这个包/平台当初为何被 pin 到老 `version`（看注释或 git 历史）。它的
     regression test 就是「unstable 最新版能否构建 + portable」。

3. **测 stock upstream 构建。** 为目标平台构建 patch/pin 要替换掉的纯上游 derivation：
   - 优先临时修改本仓库 manifest/local wiring 后构建 `nix build .#<name>`，这样使用的正是
     flake lock 和 Linux musl cross static 环境。
   - 不要把 `nix build nixpkgs#pkgsStatic.<x>` 当成等价验证；它可能使用不同 nixpkgs revision
     或不同 static package set。
   - A 类：若这正是 patch 绕过的东西、现在**能构建且能链接**，patch 是移除候选。
   - B 类：直接改 manifest 去掉 `version` pin，再构建最终 flake output。
   - 在包**目标的每个平台**（Linux + darwin）上复现。一个平台修好不代表另一个也修好。

4. **验证上游构建仍满足 portability 目标**（与 `patch-nixpkgs-standalone` 同一套检查）：
   - **Linux 常规包：** `file`/`ldd` → musl 纯静态、无 glibc、无 `.so`、无 `/nix`
     运行期引用。
   - **macOS：** `otool -L` 检查二进制及每个 ship 的 `.dylib`/`.bundle`/`.so` → 只有
     `/usr/lib/*`、`/System/Library/Frameworks/*` 或 `@loader_path` 条目；**零** `/nix/store`。
   - 根 `AGENTS.md` 记录的例外按对应 case study 验证，不把动态例外误判成上游回归成功。
   - 若上游能构建但仍不 portable，则 patch/pin **尚未**过时——保留它。只有在上游
     **既可构建又 portable** 时才回归。

5. **执行回归**（验证通过后）：
   - **A 类，整包可用上游：** **删除** `packages/<pkg>/`，从
     `packages/local.nix` 的
     `common`/`linux`/`darwin` 集合里移除它的 `callPackage ./<pkg>` 行，然后在
     `manifests/default.nix`
     里加/调条目（`isStatic = true`，正确的 `version`（省略 => unstable）/`platforms`/`output`/`alias`）。
   - **A 类，只有一个平台修好：** 只删那个平台的本地文件（如删 `darwin.nix`、留 `default.nix`），
     把该平台切到 manifest/上游；仍需 patch 的平台保留。
   - **A 类，partial regression：** 若 patch 只*部分*过时（如 feature-reduction override 里
     某个被禁的特性已能用），把 override 缩到仍必需的最小集，而不是删整个包。
   - **B 类，去掉版本 pin：** 在 `manifests/default.nix` 里删掉那条 `version = "..."`（回到 unstable）。
     若这个 pin 只在某个 per-platform key 下、且去掉后该 key 变空，就把整个 per-platform key
     也删掉；若删掉后整个包条目变成 `{ }`，保留 `<pkg> = { };` 即可。

6. **清理你的改动产生的 orphan：** 不再被用到的 patch 文件、wrapper 脚本、
   `packages/<pkg>/` 下的 vendor 配置、以及失效的 `callPackage` wiring。
   不要删与本次改动无关的既有代码。

7. **重建受影响包的最终 standalone 产物**，重跑 step 4 的验证，确认回归后的构建仍 portable。

8. **更新队列与 agent 文档。**
   - 纯 pin 或纯 workaround 完整回归成功：删除表格行。
   - mixed 包回归掉全部 workaround、但仍有 packaging：保留行，平台列只写 `📦`，回归状态改成
     `❌`，原因和判据改为结构性保留说明。
   - 只回归部分 workaround：保留 `✅` 或 `🟡`，原因和判据只留下仍未解决的部分。
   - 验证失败：保留状态；若本次得到比原记录更具体的错误，更新对应平台的原因和判据。
   - 无论成功或失败，只要在某平台实际构建验证过，就把该平台的 `Linux commit` / `macOS commit`
     刷新为本次 `flake.lock` 的 `nixpkgs-unstable` 短 rev，标记该 channel 已测过。只有整项回归删行时
     无需再填 commit。
   - 若回归改变稳定 package selection、例外或协议，更新根
   `AGENTS.md`；若只删除包级非默认策略，更新或删除对应 case study。不要在根文档维护易漂移的
   当前包枚举。

## Guardrails

- 只有 bug/portability 的 workaround 和版本 pin 可回归。绝不删刻意的 repackaging override
  （wrapper 脚本、rename 二进制、sibling/bundled 依赖）——见「这个东西是不是 regression 候选？」。
  上游修构建 bug 不会让 repackaging 过时。
- 不要凭假设回归——总是先在**所有**目标平台上构建并验证上游。
- 只有上游**既可构建又 portable** 时才回归。可构建但不 portable 不是去掉 patch/pin 的理由。
- 批量回归不得绕过 `docs/regression/` 自建候选清单；代码与队列不一致时，先维护队列。
- diff 要外科式：只移除过时 patch/pin 拥有的东西。
- 拿不准某个平台是否真修好（如本地无法测 darwin），就说明并先问，再删那个平台的 patch/pin。
