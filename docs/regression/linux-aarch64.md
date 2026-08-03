# Linux 回归表（aarch64 特有差异）

仅列 aarch64-linux 与共享 [`linux.md`](linux.md) 的差异。当前都是「临时停用」（⏸️）：这些包的
musl-static cross 构建目前在 aarch64-linux 失败，已在 manifest 或本地包集里去掉该架构，等上游/工具链
修复后恢复。x86_64-linux 与 darwin 的状态见各自表格。表格约定与图例见 [`CLAUDE.md`](CLAUDE.md)。

| 包 | 定制 | 回归 | 原因与保留边界 | 回归判据 | commit | 来源 |
| --- | --- | --- | --- | --- | --- | --- |
| `bash` | ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 manifest 临时去掉该架构；x86_64-linux 与 darwin 走零定制 manifest 构建 | aarch64-linux musl-static 构建修复后恢复该架构 | 624af665418d | `manifests/default.nix` |
| `coreutils` | ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 manifest 临时去掉该架构；x86_64-linux 与 darwin 走零定制 manifest 构建 | aarch64-linux musl-static 构建修复后恢复该架构 | 624af665418d | `manifests/default.nix` |
| `dive` | 📌 `25.11` + ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 manifest 临时去掉该架构。x86_64-linux 仍走 `25.11` pin（详见 `linux.md`） | aarch64-linux 构建修复后恢复该架构 | 624af665418d | `manifests/default.nix` |
| `patchelf` | 📌 `25.05` + ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 manifest 临时去掉该架构。x86_64-linux 仍走 `25.05` pin（详见 `linux.md`） | aarch64-linux 构建修复后恢复该架构 | 624af665418d | `manifests/default.nix` |
| `python311` | 📦 本地 + ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 `packages/local/linux/aarch64.nix` 去掉该架构（x86_64-linux 在 `x86_64.nix` 保留） | aarch64-linux 构建修复后恢复该架构 | 624af665418d | `packages/local/linux/aarch64.nix`, `packages/local/linux/x86_64.nix` |
| `qemu-user` | ⏸️ 停用 aarch64 | 🟡 | aarch64-linux 上 musl-static cross 构建当前失败，已在 manifest 临时去掉该架构；x86_64-linux 走零定制 manifest 构建 | aarch64-linux musl-static 构建修复后恢复该架构 | 624af665418d | `manifests/default.nix` |
