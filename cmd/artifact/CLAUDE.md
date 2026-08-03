# Artifact Agent Guide

`cmd/artifact/` 是所有包共享的后处理与归档工具，由
`lib/make-artifacts.nix` 调用。全局产物约束见根
[`CLAUDE.md`](../../CLAUDE.md)；本文件定义该组件的行为边界。

## 输入与输出

工具接收 source package tree、standalone output、tar.gz output、artifact name 和
目标平台。`lib/make-artifacts.nix` 为每个包创建同一个 multi-output derivation：

- `out`：规范化后的 standalone 目录；
- `archive`：以 artifact name 为顶层目录的确定性 tar.gz。

Artifact name 必须是安全的相对路径。Archive 固定 uid、gid、时间戳和 header metadata，
保证相同输入生成相同归档。

## 规范化

- 删除 `nix-support`、man page、文档、shell completion、`.a` 和 `.pyc`。
- 保留内部 symlink，物化外部 symlink，删除 dangling symlink。
- 恢复 `.*-wrapped` 入口。
- 文本 shebang 改为 `/usr/bin/env`，删除文本中的 Nix store binary path。
- Linux ELF 执行 strip；二进制中的 store hash 被稳定替换。
- 统一目录和文件权限，再生成归档。

Artifact 只做规范化与约束校验，不会把动态程序变成静态程序，也不会推断任意硬编码资源
路径。此类问题必须在 package derivation 中修复。

## Binary Validation

- 只检查当前构建平台的原生格式：Linux 检查 ELF，Darwin 检查 Mach-O。打包的跨平台
  binary 不得触发另一平台工具链。
- Linux ELF 默认必须静态链接；`music-decrypto` 和 `nsight-systems` 由
  `lib/make-artifacts.nix` 显式传入动态例外。
- Darwin dependency 只允许 `/usr/lib`、`/System/Library/Frameworks`、
  `@loader_path` 和 `@rpath`。
- Darwin rpath 必须是 `@loader_path` 或其子路径；`/nix` load command 一律失败。
- 原生 ELF 或 Mach-O 无法解析时 fail-closed。
- 路径含 `openssl` 的文件保留既有 validation 豁免，但仍执行格式相关清理。

不要通过放宽全局校验修复单包问题。新增动态例外或改变 `openssl` 豁免前先确认，并同步
`lib/make-artifacts.nix`、[回归清单](../../docs/regression/CLAUDE.md) 和
根 [`CLAUDE.md`](../../CLAUDE.md)。

## 实现边界

- `main.go`：参数、平台和调用顺序。
- `normalize.go`：tree 复制、裁剪、symlink、文本和 binary 规范化。
- `binary.go`：ELF/Mach-O inspection 与 portability validation。
- `archive.go`：确定性 tar.gz。
- `main_test.go`：规范化、格式识别、校验与归档行为。

保持标准库实现，不增加外部运行时依赖。修改归档顶层目录、metadata 或 binary policy
时，同步 `lib/make-artifacts.nix`、[`cmd/binman/CLAUDE.md`](../binman/CLAUDE.md)
和发布 workflow。

## 验证

```bash
CGO_ENABLED=0 go test ./cmd/artifact
CGO_ENABLED=0 go vet ./cmd/artifact
CGO_ENABLED=0 go build ./cmd/artifact
```
