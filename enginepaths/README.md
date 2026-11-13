# enginepaths

`enginepaths` 集中处理外部引擎相关路径：展开变量、解析为绝对路径，并在单独调用校验函数时检查文件是否存在。它不读取应用配置文件，也不下载或安装引擎。

## 解析引擎路径

```go
import "github.com/winezer0/processkit/enginepaths"

func resolveNucleiPaths() (enginepaths.ResolvedPaths, error) {
	paths, err := enginepaths.ResolveEnginePaths(enginepaths.EnginePathsConfig{
		EngineName:       "nuclei",
		BinaryPath:       "${APP_DIR}/third/nuclei/nuclei",
		ConfigPath:       "${APP_DIR}/configs/nuclei.yaml",
		WorkingDirectory: "${APP_DIR}",
		Environment: map[string]string{
			"NUCLEI_TEMPLATES": "${APP_DIR}/data/nuclei-templates",
		},
	})
	if err != nil {
		return enginepaths.ResolvedPaths{}, err
	}
	if err := enginepaths.ValidatePaths(paths, "nuclei"); err != nil {
		return enginepaths.ResolvedPaths{}, err
	}
	return paths, nil
}
```

`ResolveEnginePaths` 返回 `ResolvedPaths`，其中路径已展开并解析。它要求提供 `BinaryPath`；裸命令名（例如 `nuclei`）会通过 `PATH` 查找，显式路径按路径规则解析。配置文件、CA 文件、工作目录和插件目录可省略。路径是否存在及类型是否正确由 `ValidatePaths` 检查。

`EnginePathsConfig` 和返回值 `ResolvedPaths` 包含引擎名、二进制、配置文件、CA 文件、插件目录、工作目录和子进程环境。插件目录通过 `engineconfig.PathEntries` 表示，每个条目可以指定 `Required`；校验时缺失的可选目录会被允许。

## 路径变量

| 变量 | 含义 |
| --- | --- |
| `${APP_DIR}` | 当前宿主程序可执行文件所在目录。 |
| `${ENGINE_DIR}` | 已解析的引擎可执行文件所在目录；可用于其配置、工作目录、插件目录和子进程环境。 |
| `${HOME_DIR}`、`${TEMP_DIR}` 等 | `expand` 提供的内置目录变量。 |

路径支持 `~/` 和 `~\` 主目录前缀。`BinaryPath` 在解析时可使用 `${APP_DIR}` 和内置变量；引擎路径解析出二进制后，其他路径和环境变量还可使用 `${ENGINE_DIR}`。相对路径由 `pathresolver` 按工作目录和宿主程序目录等候选位置解析。若路径需要读取环境变量，请先由调用方显式展开并校验值；`enginepaths` 不会读取任意环境变量。

## 独立路径与插件目录

不属于某个引擎配置的路径可用 `ResolveStandalonePath` 解析，并指定 `pathresolver.KindFile`、`KindDirectory` 或 `KindAny`。返回的 `ResolvedPath` 包含绝对路径及 `Exists` 状态。

`ResolvePluginsDirs(entries, legacyPath)` 会解析插件目录列表并返回可用目录：必需条目缺失时返回错误，可选条目缺失时跳过。单个插件目录可用 `ResolvePluginsDir` 解析。

## 子进程环境

在将配置的环境变量传给外部进程前，调用 `ExpandProcessEnvironment`：

```go
environment, err := enginepaths.ExpandProcessEnvironment(paths.BinaryPath, paths.Environment)
if err != nil {
	return err
}
```

它会在环境变量值中展开 `${APP_DIR}`、`${ENGINE_DIR}` 和内置变量，并返回一份新 map，不修改输入 map。未定义变量会报错。这个函数只处理给定的子进程环境映射，不会继承或改写宿主环境；进程启动和环境合并由调用方或 `processrunner` 管理。

## 与其他包配合

典型的引擎启动顺序是：用 `engineconfig.EngineConfig.Normalize` 和 `Validate` 处理配置；由调用方按当前平台选择 `BinaryPaths.Current()`；传入 `ResolveEnginePaths`；用 `ValidatePaths` 检查磁盘状态；调用 `ExpandProcessEnvironment` 准备子进程环境；最后用 `processrunner` 启动进程。完整示例见 [项目 README](../README.md#综合调用示例)。

## 导入路径

```text
github.com/winezer0/processkit/enginepaths
```
