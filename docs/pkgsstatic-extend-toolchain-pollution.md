# `pkgsStatic.extend` 的 Target 边界

Linux 的 `pkgsStatic` 是 musl cross package set：target 产物使用 musl-static，build 平台仍使用
glibc 工具链。`pkgsStatic.extend` 会同时影响 target 包和 `buildPackages` 中的同名包。

只为静态 target 准备的 override 必须限定在
`pkg.stdenv.hostPlatform.isStatic`：

```nix
onlyStatic =
  pkg: overrides:
  if pkg.stdenv.hostPlatform.isStatic then pkg.overrideAttrs overrides else pkg;
```

否则对 `libuv` 等依赖的修改会进入 build 平台闭包，改变 `cmake`、LLVM 和 rustc 的
`drvPath`，导致本可 substitute 的工具链重新编译。

## 验证

修改局部 overlay 后检查：

- target 包仍应用静态构建 override；
- `buildPackages` 中对应包保持 upstream `drvPath`；
- `nix build .#<package> --dry-run` 不包含意外的 LLVM、rustc 或 CMake 源码构建；
- 最终 target 产物仍通过静态链接校验。

Node.js 的具体应用见
[`packages/nodejs/26/linux.nix`](../packages/nodejs/26/linux.nix)。
