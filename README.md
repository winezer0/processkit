# processkit

`processkit` 是一个 Go module，提供路径变量展开、路径解析和外部进程执行三个独立子包。

要求 Go 1.25.5 或更高版本。

| 子包 | 职责 |
| --- | --- |
| `expand` | 展开 `${NAME}`；提供常用系统目录变量，其他变量由调用方通过 map 显式传入 |
| `pathresolver` | 将路径变量展开后，按候选根目录解析并检查目标 |
| `processrunner` | 展开调用配置中的变量，直接执行程序并管理进程生命周期 |

## 路径变量

`expand.Expand` 支持以下内置变量：`${HOME_DIR}`、`${TEMP_DIR}`、`${USER_CONFIG_DIR}` 和 `${USER_CACHE_DIR}`。其他变量可显式传入；`pathresolver.Options.ExpandEnv` 默认为 `false`，设为 `true` 后可展开当前进程的环境变量：

```go
resolved, err := pathresolver.Resolve("${APP_DIR}/configs/app.yaml", pathresolver.Options{
	Kind:      pathresolver.KindFile,
	Variables: map[string]string{"APP_DIR": appDirectory},
	ExpandEnv: true,
})
```

未找到的环境变量展开为空字符串。自定义变量值按字面值插入，不递归展开；变量展开不调用 shell。

## 子进程环境值

`processrunner.Invocation.Variables` 用于展开可执行文件、参数、工作目录和环境变量的值；`Environment` 则是要传给子进程的环境变量覆盖项：

```go
result, err := processrunner.Run(ctx, processrunner.Invocation{
	Binary: "katana",
	Environment: map[string]string{
		"XDG_CONFIG_HOME": "${APP_DIR}/third",
	},
	Variables: map[string]string{
		"APP_DIR": appDirectory,
	},
	ExpandEnv: true,
})
```

`Invocation.ExpandEnv` 默认为 `false`；设为 `true` 后，命令字段和 `Environment` 的值也会从当前进程环境展开。`Environment` 会覆盖继承环境中的同名变量，环境变量名本身不会展开。命令参数直接传给可执行文件，不经过 shell。

## 开发

在仓库根目录运行：

```sh
go test ./...
go vet ./...
```

module 导入路径为 `github.com/winezer0/processkit`。
