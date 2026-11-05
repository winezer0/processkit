# processkit

`processkit` 是一个 Go module，提供路径变量展开、路径解析和外部进程执行能力。

要求 Go 1.25.5 或更高版本。

| 包 | 作用 | 文档 |
| --- | --- | --- |
| `expand` | 展开内置变量、调用方提供的变量，以及可选的环境变量。 | [expand README](expand/README.md) |
| `pathresolver` | 按候选目录优先级解析路径，并检查目标类型和存在状态。 | [pathresolver README](pathresolver/README.md) |
| `processrunner` | 执行外部程序，管理输出捕获、超时、异步状态和日志。 | [processrunner README](processrunner/README.md) |

## 环境变量展开注意事项

`expand.Expand`、`pathresolver` 和 `processrunner` 开启 `ExpandEnv` 后，未设置或值为空的环境变量都会展开为空字符串；

当前没有缺失变量报错模式。例如 `${MISSING}/etc` 在 Unix 上会变成 `/etc`，可能意外指向根目录下的位置。

将环境变量用于路径、可执行文件或参数前，请由调用方确认所需变量已设置且非空。
