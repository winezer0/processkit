# processkit

`processkit` 是 Go 工具库，提供配置校验、路径解析、变量展开和受控的外部进程执行能力。项目要求 Go 1.25.5 或更高版本。

## 包

| 包 | 用途 | 文档 |
| --- | --- | --- |
| `engineconfig` | 规范化并校验引擎配置、平台二进制路径和插件目录项。 | [engineconfig README](engineconfig/README.md) |
| `enginepaths` | 展开并解析引擎二进制、配置、CA、插件和工作目录，校验路径并准备子进程环境。 | [enginepaths README](enginepaths/README.md) |
| `expand` | 展开内置变量、调用方变量和可选的环境变量。 | [expand README](expand/README.md) |
| `pathresolver` | 按候选目录优先级解析路径，并检查目标类型和存在状态。 | [pathresolver README](pathresolver/README.md) |
| `processrunner` | 执行外部程序，管理输出捕获、进程树、超时和运行状态。 | [processrunner README](processrunner/README.md) |

## 综合调用示例

下面的流程参考相邻 `../tanscan` 对 processkit 的实际使用：规范化并校验引擎设置，解析和检查路径，展开子进程环境变量，再用 `processrunner` 执行引擎版本探测。示例以 Nuclei 为例；Tanscan 会按引擎选择对应的探测参数并处理诊断结果。

```go
package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/winezer0/processkit/engineconfig"
	"github.com/winezer0/processkit/enginepaths"
	"github.com/winezer0/processkit/processrunner"
)

func main() {
	config := engineconfig.EngineConfig{
		CheckTimeoutSeconds: 30,
		BinaryPaths: engineconfig.BinaryPaths{
			runtime.GOOS + "_" + runtime.GOARCH: "${APP_DIR}/third/nuclei/nuclei",
		},
		ConfigPath:       "${APP_DIR}/configs/nuclei.yaml",
		WorkingDirectory: "${APP_DIR}",
		Environment: map[string]string{
			"NUCLEI_TEMPLATES": "${APP_DIR}/data/nuclei-templates",
		},
	}.Normalize()
	if err := config.Validate(); err != nil {
		log.Fatal(err)
	}

	binary, platform := config.BinaryPaths.Current()
	if binary == "" {
		log.Fatalf("binary_path.%s must be configured", platform)
	}
	paths, err := enginepaths.ResolveEnginePaths(enginepaths.EnginePathsConfig{
		EngineName:       "nuclei",
		BinaryPath:       binary,
		ConfigPath:       config.ConfigPath,
		WorkingDirectory: config.WorkingDirectory,
		PluginDirs:       config.PluginDirs,
		Environment:      config.Environment,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := enginepaths.ValidatePaths(paths, "nuclei"); err != nil {
		log.Fatal(err)
	}

	environment, err := enginepaths.ExpandProcessEnvironment(paths.BinaryPath, paths.Environment)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.CheckTimeoutSeconds)*time.Second)
	defer cancel()
	result, err := processrunner.Run(ctx, processrunner.Invocation{
		Name: "nuclei-version", Binary: paths.BinaryPath, Args: []string{"-version"},
		Directory: paths.WorkingDirectory, Environment: environment, CaptureBytes: 64 << 10,
	}, processrunner.RunOptions{})
	if err == nil && result.StopReason == processrunner.StopDeadline {
		err = context.DeadlineExceeded
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(result.Output))
}
```

路径示例假定配置文件和引擎已安装在宿主程序目录下。`enginepaths` 负责解析路径，不会安装或下载引擎；引擎专用参数仍由调用方提供。

## 变量展开

调用方需要变量展开时应显式启用。`expand.Expand` 默认不读取进程环境；未设置的环境变量在启用环境展开后会被替换为空字符串，因此将结果用于路径或参数前应检查所需变量。`pathresolver` 和 `processrunner` 不会自行展开变量。

```sh
go test ./...
```
