# Standalone Binaries

本仓库使用 Nix 构建可搬运的命令行工具，并将每个工具发布为 standalone 目录和
tar.gz OCI artifact。支持的平台是：

- Linux x86_64
- macOS arm64

Linux 二进制默认使用 musl 全静态链接；macOS 二进制只允许动态链接系统库或随包提供的
相对路径 dylib。所有产物都不得在运行时依赖 `/nix/store`。

## 安装

推荐先安装包管理器 `bm`：

```bash
base='https://raw.githubusercontent.com/curoky/standalone-binaries'
curl -fsSL "$base/master/cmd/binman/install.sh" | bash
```

然后将工具安装到可写目录：

```bash
bm --prefix "$HOME/.local/share/binman" install ripgrep fd jq
export PATH="$HOME/.local/share/binman:$PATH"
```

`bm` 支持 `install`、`remove`、`upgrade`、`list`、`info` 和 `outdated`。完整参数以
`bm --help` 为准。

`bm sync binman.yaml` 可声明式安装 package 和 profile；加上 `--prune` 删除
manifest 未引用的包。格式和状态语义见 [`bm` 设计](docs/binman.md)。

## 使用 Nix

从源码构建单个工具或其归档：

```bash
nix build .#ripgrep
nix build .#tarballs.x86_64-linux.ripgrep
```

flake 支持 `x86_64-linux` 和 `aarch64-darwin`。可用包以对应平台的 flake outputs
为准。

## 文档

- [架构与产物约束](docs/architecture.md)
- [贡献与验证流程](docs/contributing.md)
- [包构建策略](docs/package-strategies/README.md)
- [`bm` 设计与协议](docs/binman.md)
- [发布与 cache 模型](docs/release-model.md)

## License

见 [LICENSE](LICENSE)。
