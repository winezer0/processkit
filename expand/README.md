# expand

`expand` 是一个 Go 字符串和路径展开包。`Expand` 一次处理 `${NAME}` 变量和可选的 `~/`、`~\` 主目录前缀；环境变量展开默认关闭。

要求 Go 1.25.5 或更高版本。

## 快速开始

```go
package main

import (
	"fmt"
	"log"

	"github.com/winezer0/processkit/expand"
)

func main() {
	value, err := expand.Expand("${APP_DIR}/third/katana", map[string]string{
		"APP_DIR": "/opt/tanscan",
	}, false, false)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value) // /opt/tanscan/third/katana
}
```

变量 map 的键只写变量名，不带 `${}`。传入 `nil` 或空 map 也可以使用内置变量或展开不含变量的字符串。

默认关闭环境变量展开。设置第三个参数 `expandEnv` 为 `true` 时，变量先从 map 和内置变量中解析，再从当前进程环境中解析。未设置的环境变量展开为空字符串。`$NAME` 始终保持原样：

```go
value, err := expand.Expand("${PATH}", nil, true, false)
```

## 内置变量

| 变量 | 来源 |
| --- | --- |
| `${HOME_DIR}` | `os.UserHomeDir()`：当前运行账号的主目录 |
| `${HOME_CONFIG_DIR}` | 绝对主目录下的 `.config` |
| `${TEMP_DIR}` | `os.TempDir()`：系统临时目录 |
| `${USER_CONFIG_DIR}` | `os.UserConfigDir()`：当前用户的配置目录 |
| `${USER_CACHE_DIR}` | `os.UserCacheDir()`：当前用户的缓存目录 |
| `${NOW_WORK_DIR}` | `os.Getwd()`：调用时进程的当前工作目录 |
| `${NOW_EXE_PATH}` | `os.Executable()`：启动当前进程的可执行文件路径 |
| `${NOW_EXE_DIR}` | `filepath.Dir(os.Executable())`：当前可执行文件所在目录 |

这些值由 Go 在当前操作系统上解析，因此具体路径随操作系统和运行账号而异。`${HOME_CONFIG_DIR}` 固定指主目录下的 `.config`，与平台默认的 `${USER_CONFIG_DIR}` 不同。`NOW_*` 的值表示调用展开函数时的宿主进程状态；`NOW_EXE_PATH` 可能受符号链接启动方式影响。内置变量不能通过 map 覆盖。

内置目录变量始终返回绝对路径。如果 `os.UserHomeDir()` 或 `os.TempDir()` 返回相对路径，`expand` 会在展开时以进程当前工作目录为基准转换为绝对路径；转换失败会返回错误。

将内置变量放入输入字符串即可使用；无需传入自定义变量 map，也无需开启环境变量展开：

```go
directories, err := expand.Expand(
	"home=${HOME_DIR}; homeConfig=${HOME_CONFIG_DIR}; temp=${TEMP_DIR}; config=${USER_CONFIG_DIR}; cache=${USER_CACHE_DIR}; work=${NOW_WORK_DIR}; exe=${NOW_EXE_PATH}; exeDir=${NOW_EXE_DIR}",
	nil,
	false,
	false,
)
if err != nil {
	log.Fatal(err)
}
fmt.Println(directories)
```

展开结果是当前操作系统和运行账号对应的目录路径。

### 开启环境变量展开时使用内置变量

设置 `expandEnv` 为 `true`、`expandHome` 为 `false` 后，固定内置目录变量仍按原方式解析；其他 `${NAME}` 变量则可从当前进程环境中读取，例如 `${PATH}`：

```go
value, err := expand.Expand(
	"home=${HOME_DIR}; temp=${TEMP_DIR}; path=${PATH}",
	nil,
	true,
	false,
)
if err != nil {
	log.Fatal(err)
}
fmt.Println(value)
```

## 自定义变量

变量名使用 `\w` 字符（ASCII 字母、数字或下划线），两端空白会在查找前去除；中间空格不属于变量名。自定义 map 的键不带 `${}`，例如：

```go
value, err := expand.Expand(
	"${ENGINE_DIR}/config.yaml",
	map[string]string{"ENGINE_DIR": "/opt/tanscan/third/katana"},
	false,
	false,
)
```

变量值会按字面值插入，不会继续解析其中的 `${...}`。例如，若 `A` 的值为 `${B}`，展开 `${A}` 的结果仍是 `${B}`。

## 用户主目录路径

设置第四个参数 `expandHome` 为 `true`，即可把输入开头的 `~/` 或 `~\` 替换为当前用户主目录，并在同一次调用中展开变量：

```go
configPath, err := expand.Expand("~/configs/${CONFIG_NAME}.yaml", map[string]string{
	"CONFIG_NAME": "app",
}, false, true)
if err != nil {
	log.Fatal(err)
}
fmt.Println(configPath)
```

只有输入开头的 `~/` 和 `~\` 会展开；单独的 `~`、`~other/` 和字符串中间的 `~` 保持原样。其余分隔符按当前操作系统解释；例如 Unix 上 `~\a\b` 中后续的反斜杠是普通字符。

## 解析规则与错误

- 仅提取 `${\w+}`，变量名两端可带空白并会自动去除；`$NAME`、`%NAME%`、内部含空格的表达式和未闭合表达式保持原文。
- 未知的完整变量会返回错误；开启环境展开后，未设置的环境变量会替换为空字符串。
- 自定义变量名非法、值为空或试图覆盖内置变量时会返回错误；即使该变量没有出现在输入字符串中也一样。
- 内置目录无法由操作系统解析时会返回错误。
- 不含变量表达式的普通文本会原样返回。变量内容不会经过 shell 解释。

## 导入路径

```text
github.com/winezer0/processkit/expand
```

## 许可证

[Apache License 2.0](../LICENSE)。
