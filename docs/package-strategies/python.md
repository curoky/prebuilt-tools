# Python Runtime 与脚本工具

Linux 发布多个 musl 全静态 CPython runtime；纯 Python 工具由 wrapper 绑定明确的运行时。
版本、补丁和包状态见 [`TODO.md`](../../TODO.md)。

## 静态 CPython

每个版本使用独立 `Modules/Setup.local`，把所需 C 扩展静态编入解释器。全静态解释器不能依赖
运行期加载的 `.so`，因此新增模块时必须：

- 在对应 `Setup.local` 中声明静态扩展；
- 对外部库使用静态归档；
- 保证解释器和扩展均不残留 `/nix/store` 运行期引用。

共享 factory 只承载所有版本一致的构建规则；版本专属差异留在对应资源文件或显式参数中，
不在 factory 中按版本号分支。

## 同级 Runtime Wrapper

Linux 的 Python 工具把入口和 `site-packages` 放在工具自身包内。wrapper 根据自身路径定位
同级 `python314`，设置 `PYTHONHOME` 和 `PYTHONPATH`，再显式执行该解释器。上游 shebang 不参与
运行期解析。

Darwin 当前不发布 Python runtime，`git-filter-repo` 和 `netron` 使用宿主 `python3`。这是明确的
宿主依赖；新增 Darwin Python 工具不得静默采用同一做法。

工具应保持纯脚本分发。若加入 native extension，必须改为平台拆分，并分别满足 Linux 静态链接
和 macOS 系统 dylib 边界。
