# Go 构建策略

Linux Go 工具默认使用 manifest 的 `pkgsStatic`。macOS 可选择 native CGO build，只要
最终产物仅依赖系统 dylib；不要为了“更静态”机械设置 `CGO_ENABLED=0`，因为这可能把
Go compiler store path 写入产物并触发 `disallowedReferences`。

## Podman

Podman 必须绑定本仓库的静态 container helpers。尤其是 runc：上游 wrapper 是动态
launcher，而 Podman 的 helper collection 会复制该 launcher。`packages/podman` 因此直接
安装真实静态 runc，并显式绑定 `conmon`、`catatonit` 和 `crun`。

删除这层替换前，必须确认 Podman 收集到的是静态入口，而不是 `wrapProgram` launcher。
相关 helper 路径必须保持可搬运。

容器栈的 C 组件见 [C / autotools](c-autotools.md)。pin 和 patch 状态见
[`TODO.md`](../../TODO.md)。
