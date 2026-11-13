# engineconfig

`engineconfig` 定义外部引擎共用的配置结构和校验规则。它只校验配置格式与字段值，不读取配置文件，也不检查文件系统中的路径；路径解析和文件存在性检查由 [`enginepaths`](../enginepaths/README.md) 负责。

## EngineConfig

`EngineConfig` 可通过 YAML 标签嵌入应用自己的引擎配置。字段如下：

| YAML 字段 | Go 字段 | 说明 |
| --- | --- | --- |
| `enabled` | `Enabled` | 可选启用状态；`nil` 时 `EnabledByDefault()` 返回 `true`。 |
| `check_timeout_seconds` | `CheckTimeoutSeconds` | 引擎检查超时秒数。`Normalize()` 将 `0` 设为 30；`Validate()` 接受 1–300。 |
| `binary_path` | `BinaryPaths` | 按 `GOOS_GOARCH` 键配置可执行文件路径。 |
| `config_path` | `ConfigPath` | 引擎配置文件路径。 |
| `ca_file` | `CAFile` | CA 证书文件路径。 |
| `plugins_dir` | `PluginDirs` | 插件目录；可配置单个路径或带 `path`、`required` 字段的列表。 |
| `environment` | `Environment` | 子进程环境变量映射；不序列化到 JSON。 |
| `others` | `Others` | 引擎专用的扩展设置。 |
| `working_directory` | `WorkingDirectory` | 引擎工作目录。 |
| `proxy_url` | `ProxyURL` | 可选的绝对 `http`、`https` 或 `socks5` URL。 |

运行时字段 `Runtime` 不属于 YAML 或 JSON 配置。它用于调用方保存解析后的二进制和插件目录，以及代理继承状态。

## YAML 示例

```yaml
nuclei:
  enabled: true
  check_timeout_seconds: 30
  binary_path:
    linux_amd64: "${APP_DIR}/third/nuclei/nuclei"
    windows_amd64: "${APP_DIR}/third/nuclei/nuclei.exe"
  config_path: "${APP_DIR}/configs/nuclei.yaml"
  working_directory: "${APP_DIR}"
  plugins_dir:
    - path: "${APP_DIR}/data/nuclei-templates"
      required: true
    - path: "${APP_DIR}/data/local-templates"
      required: false
  environment:
    NUCLEI_CACHE: "${ENGINE_DIR}/.cache"
  proxy_url: "http://127.0.0.1:8080"
  others:
    rate_limit: 150
```

表达式在配置解析阶段会作为普通字符串保留。由调用方使用 [`enginepaths`](../enginepaths/README.md) 解析路径和进程环境变量。

## 规范化、选择平台路径和校验

```go
import (
	"fmt"

	"github.com/winezer0/processkit/engineconfig"
)

func currentEngineBinary(config engineconfig.EngineConfig) (string, error) {
	config = config.Normalize()
	if err := config.Validate(); err != nil {
		return "", err
	}

	binary, platformKey := config.BinaryPaths.Current()
	if binary == "" {
		return "", fmt.Errorf("binary_path.%s is not configured", platformKey)
	}
	return binary, nil
}
```

`BinaryPaths.For(goos, goarch)` 可查询指定平台，`Current()` 查询当前运行平台。校验接受当前尚未发布的 Go 平台组合，但键必须由小写字母和数字组成，并以 `_` 分隔；每个配置的二进制路径必须非空。

`EngineConfig.Validate()` 会聚合超时、二进制键和值、插件目录项以及代理 URL 的格式错误。它不验证二进制或目录是否存在。`EngineFieldError` 保留出错字段名并支持 `errors.Is` / `errors.As`；`PrefixEngineFieldErrors` 可为嵌套配置错误添加字段前缀。

`PathEntries` 的 `NormalizePathEntries` 会清理路径并兼容单个旧式插件目录值。`PathEntries.CheckPaths` 可在解析路径后按文件、目录或任意目标类型检查条目，并遵循 `required` 标记。

## 导入路径

```text
github.com/winezer0/processkit/engineconfig
```
