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
	"flag"
	"log"

	"github.com/winezer0/processkit/pathresolver"
)

func main() {
	processDir := flag.String("process-dir", "", "process directory")
	flag.Parse()

	result, err := pathresolver.Resolve("plugins/templates", pathresolver.Options{
		ProcessDir: *processDir,
		Kind:       pathresolver.KindDirectory,
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

`ProcessDir` 是调用方指定的资源根目录，与当前工作目录和用户主目录是不同概念。包不读取配置文件，也不负责注册命令行参数。

## 路径变量

目标路径和候选目录字段支持 `${NAME}` 变量。内置变量为 `${HOME_DIR}`、`${TEMP_DIR}`、`${USER_CONFIG_DIR}` 和 `${USER_CACHE_DIR}`。其他变量可通过 `Options.Variables` 显式传入：

```go
result, err := pathresolver.Resolve("${ENGINE_DIR}/config/katana.yaml", pathresolver.Options{
	Kind: pathresolver.KindFile,
	Variables: map[string]string{
		"ENGINE_DIR": engineDirectory,
	},
	ExpandEnv: true,
})
```

`Options.ExpandEnv` 默认为 `false`；设为 `true` 后，`${NAME}` 也会从当前进程环境展开。未设置的环境变量替换为空字符串；`$NAME` 始终保持原样。默认模式下未知变量、非法名称或空自定义值会返回错误。变量值按字面值插入，不递归展开。`Variables` 和 `ExpandEnv` 同样用于 `ProcessDir`、`WorkingDirectory`、`ExecutablePath` 和 `AdditionalBases` 中的路径。

所有路径字段也支持以 `~/` 或 `~\` 开头的相对路径，并会展开到当前用户的主目录；单独的 `~` 和 `~other/` 不展开。

## 解析顺序

相对路径按以下顺序检查：

1. `Options.ProcessDir`（为空时跳过）。
2. `Options.WorkingDirectory`；为空时使用调用时的当前工作目录。
3. `Options.ExecutablePath` 所在目录；为空时使用当前进程可执行文件所在目录。
4. `Options.AdditionalBases`，按切片顺序检查。

每个根目录必须存在且是目录。找到首个存在并符合 `Kind` 的目标后立即返回。绝对路径只检查自身，不尝试其他根目录。路径会在每次调用时重新检查；默认不会向可执行文件目录的父目录搜索。

解析结果始终是绝对路径。相对路径返回命中的根目录作为 `Base`；绝对路径的 `Base` 为空。`ProcessDir`、`ExecutablePath` 和 `AdditionalBases` 中的相对根路径相对于解析后的工作目录解析；相对 `WorkingDirectory` 则相对于进程当前工作目录解析。通常建议 `ProcessDir` 使用绝对路径。

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
	ProcessDir: "/opt/myapp",
	Kind:       pathresolver.KindFile,
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

`Candidate.Source` 用于标记候选来源；省略时默认为 `SourceFallback`。标准来源值还有 `SourceAbsolute`、`SourceProcessDir`、`SourceWorkingDirectory` 和 `SourceExecutableDirectory`。

## 返回值与错误

`Result` 包含以下字段：

| 字段 | 含义 |
| --- | --- |
| `Path` | 解析后的绝对路径 |
| `Base` | 命中的绝对根目录；绝对输入路径时为空 |
| `Source` | 命中的候选来源 |
| `Exists` | 目标存在且符合 `Kind` 时为 `true`；目标缺失时为 `false` |

例如，假设 `ProcessDir` 是 `/opt/myapp`，目标文件存在：

```go
result, err := pathresolver.Resolve("config/app.yaml", pathresolver.Options{
	ProcessDir: "/opt/myapp",
	Kind:       pathresolver.KindFile,
})
if err != nil {
	log.Fatal(err)
}
log.Printf("path=%s base=%s source=%s exists=%t",
	result.Path, result.Base, result.Source, result.Exists)
```

输出类似：

```text
path=/opt/myapp/config/app.yaml base=/opt/myapp source=process-dir exists=true
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
source=process-dir path=/opt/myapp/settings.yaml reason=path is not a regular file
source=working-directory path=/work/settings.yaml reason=path does not exist
source=executable-directory path=/opt/myapp/bin/settings.yaml reason=path does not exist
```

不要依赖错误字符串来判断程序逻辑；需要诊断时读取 `ResolveError.Failures`。

## 路径边界

相对路径中的 `..` 不会被限制在候选根目录内。如果调用方需要将路径限制在某个目录之下，应额外进行边界校验。文件系统检查使用 `os.Stat`，会跟随符号链接。

## 开发与测试

在仓库根目录运行：

```sh
go test ./...
go vet ./...
```

模块导入路径为 `github.com/winezer0/processkit/pathresolver`。
