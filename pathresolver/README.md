# Go Path Resolver

`pathresolver` 是 Go 通用路径解析包。它按优先级检查候选根目录，并在每次调用时验证目标，返回绝对路径、来源和目标存在状态。

要求 Go 1.25.5 或更高版本。

## 安装

仓库发布版本并创建版本标签后，在项目中运行：

```sh
go get github.com/winezer0/processkit/pathresolver@latest
```

## 快速开始

```go
package main

import (
	"log"

	"github.com/winezer0/processkit/pathresolver"
)

func main() {
	result, err := pathresolver.Resolve("plugins/templates", pathresolver.Options{
		Kind: pathresolver.KindDirectory,
	})
	if err != nil {
		log.Fatal(err)
	}
	if !result.Exists {
		log.Fatalf("template directory does not exist: %s", result.Path)
	}
	log.Printf("path=%s base=%s source=%s", result.Path, result.Base, result.Source)
}
```

包不读取配置文件，也不负责注册命令行参数；相对路径默认从当前工作目录开始查找，再查找当前进程可执行文件所在目录。

## 路径变量

`pathresolver` 不负责展开路径。调用方应先用 `expand` 处理 `~` 和变量，再将展开后的路径传给 `Resolve`。`expand` 提供 `${HOME_DIR}`、`${HOME_CONFIG_DIR}`、`${TEMP_DIR}`、`${USER_CONFIG_DIR}`、`${USER_CACHE_DIR}`、`${NOW_WORK_DIR}`、`${NOW_EXE_PATH}` 和 `${NOW_EXE_DIR}` 等内置变量，也支持调用方提供变量。环境变量展开默认关闭，只有调用方显式传入 `true` 时才启用。

```go
package main

import (
	"fmt"

	"github.com/winezer0/processkit/expand"
	"github.com/winezer0/processkit/pathresolver"
)

func resolveEngineConfig(engineDirectory string) (string, error) {
path, err := expand.Expand("${ENGINE_DIR}/config/katana.yaml", map[string]string{
		"ENGINE_DIR": engineDirectory,
}, false, true) // Keep ambient environment variable expansion disabled; expand a leading ~/ when present.
	if err != nil {
		return "", err
	}
	result, err := pathresolver.Resolve(path, pathresolver.Options{Kind: pathresolver.KindFile})
	if err != nil {
		return "", err
	}
	if !result.Exists {
		return "", fmt.Errorf("config file does not exist: %s", result.Path)
	}
	return result.Path, nil
}
```

先展开所有需要变量处理的路径字段，包括 `WorkingDirectory`、`ExecutablePath` 和 `AdditionalBases`；然后交给解析器进行候选目录查找和文件系统检查。解析器不会改写 `${NAME}`、`~` 或环境变量表达式。其余展开规则见 [expand README](../expand/README.md)。

## 解析顺序

相对路径按以下顺序检查：

1. `Options.WorkingDirectory`；为空时使用调用时的当前工作目录。
2. `Options.ExecutablePath` 所在目录；为空时使用当前进程可执行文件所在目录。
3. `Options.AdditionalBases`，按切片顺序检查。

每个根目录必须存在且是目录。找到首个存在并符合 `Kind` 的目标后立即返回。绝对路径只检查自身，不尝试其他根目录。路径会在每次调用时重新检查；默认不会向可执行文件目录的父目录搜索。

解析结果始终是绝对路径。相对路径返回命中的根目录作为 `Base`；绝对路径的 `Base` 为空。`ExecutablePath` 和 `AdditionalBases` 中的相对根路径相对于解析后的工作目录解析；相对 `WorkingDirectory` 则相对于进程当前工作目录解析。

## 目标类型

`Options.Kind` 控制现有目标所需类型：

| 值 | 要求 |
| --- | --- |
| `KindAny` | 任意已存在的文件系统项；零值，也是默认值 |
| `KindFile` | 已存在的普通文件 |
| `KindDirectory` | 已存在的目录 |

目标缺失时，`Kind` 仍用于检查已存在候选是否符合预期类型。

## 缺失目标与 `Exists`

包只有一种解析方式：先按优先级寻找已存在且类型符合要求的目标；如果没找到，且存在可安全回退的缺失目标候选，就返回优先级最高的此类路径，并令 `Result.Exists` 为 `false`。目标在某个候选中已存在但类型不符、或因其他文件系统错误无法检查时，不会把它当成缺失路径回退。解析不会创建文件或目录。绝对路径不存在时也会直接返回该路径并设 `Exists: false`。

这使同一个接口可以解析必需资源和未来创建的输出路径。必需资源由调用方检查 `Exists`；输出路径可在检查后交给创建逻辑：

```go
result, err := pathresolver.Resolve("logs/app.log", pathresolver.Options{
	WorkingDirectory: "/opt/myapp",
	Kind:             pathresolver.KindFile,
})
if err != nil {
	log.Fatal(err)
}
if !result.Exists {
	log.Printf("create the file at %s", result.Path)
}
```

若目标在某个候选中已存在但类型不符，且后续没有符合要求的目标，解析会返回错误。根目录无效或文件系统无法检查时也会报错，避免把这些情况误判为目标缺失。

## 添加备用根目录

需要添加额外搜索位置时，可通过 `AdditionalBases` 指定顺序：

```go
result, err := pathresolver.Resolve("templates", pathresolver.Options{
	Kind: pathresolver.KindDirectory,
	AdditionalBases: []pathresolver.Candidate{
		{Path: "/opt/shared-assets", Source: pathresolver.SourceFallback},
		{Path: "/srv/app/assets", Source: "application-assets"},
	},
})
```

`Candidate.Source` 用于标记候选来源；省略时默认为 `SourceFallback`。标准来源值还有 `SourceAbsolute`、`SourceWorkingDirectory` 和 `SourceExecutableDirectory`。

## 返回值与错误

`Result` 包含以下字段：

| 字段 | 含义 |
| --- | --- |
| `Path` | 解析后的绝对路径 |
| `Base` | 命中的绝对根目录；绝对输入路径时为空 |
| `Source` | 命中的候选来源 |
| `Exists` | 目标存在且符合 `Kind` 时为 `true`；目标缺失时为 `false` |

例如，假设工作目录是 `/opt/myapp`，目标文件存在：

```go
result, err := pathresolver.Resolve("config/app.yaml", pathresolver.Options{
	WorkingDirectory: "/opt/myapp",
	Kind:             pathresolver.KindFile,
})
if err != nil {
	log.Fatal(err)
}
log.Printf("path=%s base=%s source=%s exists=%t",
	result.Path, result.Base, result.Source, result.Exists)
```

输出类似：

```text
path=/opt/myapp/config/app.yaml base=/opt/myapp source=working-directory exists=true
```

绝对路径直接检查自身。例如 `Resolve("/var/lib/myapp/cache.db", ...)` 在目标不存在时返回 `Path=/var/lib/myapp/cache.db`、`Base=""`、`Source=absolute`、`Exists=false`。

如果无法返回有效路径，`Resolve` 会返回 `err`。候选诊断可通过 `errors.As` 读取 `*ResolveError`，其中的 `Failures` 按检查顺序记录来源、路径和原因：

```go
import (
	"errors"
	"fmt"

	"github.com/winezer0/processkit/pathresolver"
)

if err != nil {
	var resolveErr *pathresolver.ResolveError
	if errors.As(err, &resolveErr) {
		for _, failure := range resolveErr.Failures {
			fmt.Printf("source=%s path=%s reason=%s\n",
				failure.Source, failure.Path, failure.Reason)
		}
	}
}
```

例如，`settings.yaml` 在进程目录中是目录，但调用方要求普通文件，且其他候选没有符合要求的目标时，错误诊断可能包含：

```text
source=working-directory path=/work/settings.yaml reason=path does not exist
source=executable-directory path=/opt/myapp/bin/settings.yaml reason=path does not exist
```

不要依赖错误字符串来判断程序逻辑；需要诊断时读取 `ResolveError.Failures`。

## 路径边界

相对路径中的 `..` 不会被限制在候选根目录内；绝对路径也只检查自身，不受候选根目录约束。文件系统检查使用 `os.Stat`，会跟随符号链接。因此本包不提供目录沙箱保证：如果调用方需要将路径限制在某个目录之下，应在实际文件访问处额外进行边界校验，并考虑符号链接带来的越界情况。

## 开发与测试

在仓库根目录运行：

```sh
go test ./...
go vet ./...
```

模块导入路径为 `github.com/winezer0/processkit/pathresolver`。
