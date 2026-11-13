// Package enginepaths resolves and validates common paths used by external engines.
//
// The package depends only on processkit and the Go standard library so it can be
// moved into processkit without depending on Tanscan internals.
package enginepaths

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/winezer0/processkit/engineconfig"
	"github.com/winezer0/processkit/expand"
	"github.com/winezer0/processkit/pathresolver"
)

// EnginePathsConfig contains common path settings for one engine process.
type EnginePathsConfig struct {
	EngineName       string
	BinaryPath       string
	ConfigPath       string
	CAFile           string
	PluginsDir       string
	PluginDirs       engineconfig.PathEntries
	WorkingDirectory string
	Environment      map[string]string
}

// ResolvedPaths contains absolute paths used to configure an engine process.
type ResolvedPaths struct {
	BinaryPath       string
	ConfigPath       string
	CAFile           string
	PluginsDir       string
	PluginDirs       engineconfig.PathEntries
	WorkingDirectory string
	Environment      map[string]string
}

// ResolvedPath contains the resolved path and whether it currently exists.
type ResolvedPath struct {
	Path   string
	Exists bool
}

// ResolveEnginePaths expands supported variables and resolves configured engine paths.
func ResolveEnginePaths(config EnginePathsConfig) (ResolvedPaths, error) {
	if strings.TrimSpace(config.BinaryPath) == "" {
		return ResolvedPaths{}, fmt.Errorf("%s binary path must be configured", config.EngineName)
	}

	appExecutable, _ := os.Executable()
	resolveOptions := pathResolutionOptions{ExecutablePath: appExecutable, Variables: processVariables("")}
	binary, err := resolveBinaryPath(config.EngineName, config.BinaryPath, resolveOptions)
	if err != nil {
		return ResolvedPaths{}, err
	}
	resolveOptions.Variables = processVariables(binary)

	workingDirectory := ""
	if strings.TrimSpace(config.WorkingDirectory) != "" {
		workingDirectory, err = resolveEnginePath(config.EngineName, "working directory", config.WorkingDirectory, resolveOptions)
		if err != nil {
			return ResolvedPaths{}, err
		}
	}
	paths := ResolvedPaths{
		BinaryPath:       binary,
		WorkingDirectory: workingDirectory,
		Environment:      cloneEnvironment(config.Environment),
	}
	if config.ConfigPath != "" {
		paths.ConfigPath, err = resolveEnginePath(config.EngineName, "config path", config.ConfigPath, resolveOptions)
		if err != nil {
			return ResolvedPaths{}, err
		}
	}
	if config.CAFile != "" {
		paths.CAFile, err = resolveEnginePath(config.EngineName, "CA file", config.CAFile, resolveOptions)
		if err != nil {
			return ResolvedPaths{}, err
		}
	}
	for _, entry := range engineconfig.NormalizePathEntries(config.PluginDirs, config.PluginsDir) {
		resolved, resolveErr := resolveEnginePath(config.EngineName, "plugins directory", entry.Path, resolveOptions)
		if resolveErr != nil {
			if entry.Name != "" {
				resolveErr = fmt.Errorf("plugins_dir[%q].path: %w", entry.Name, resolveErr)
			}
			return ResolvedPaths{}, resolveErr
		}
		paths.PluginDirs = append(paths.PluginDirs, engineconfig.PathEntry{Name: entry.Name, Path: resolved, Required: entry.Required})
		if paths.PluginsDir == "" {
			paths.PluginsDir = resolved
		}
	}
	return paths, nil
}

// ResolveStandalonePath resolves a path using APP_DIR and the standard path bases.
func ResolveStandalonePath(path string, kind pathresolver.Kind) (ResolvedPath, error) {
	expanded, err := expand.Expand(path, processVariables(""), false, true)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("expand configured path: %w", err)
	}
	result, err := pathresolver.Resolve(expanded, pathresolver.Options{Kind: kind})
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("resolve configured path: %w", err)
	}
	return ResolvedPath{Path: result.Path, Exists: result.Exists}, nil
}

// ValidatePaths checks the required engine executable and configured file paths.
func ValidatePaths(paths ResolvedPaths, engineName string) error {
	var validationErrors []error
	if err := requirePath(paths.BinaryPath, pathresolver.KindFile); err != nil {
		validationErrors = append(validationErrors, &engineconfig.EngineFieldError{
			Field: "binary_path",
			Err:   fmt.Errorf("%s binary must be a regular file: %w", engineName, err),
		})
	}
	if paths.ConfigPath != "" {
		if err := requirePath(paths.ConfigPath, pathresolver.KindFile); err != nil {
			validationErrors = append(validationErrors, &engineconfig.EngineFieldError{
				Field: "config_path",
				Err:   fmt.Errorf("%s config must be a regular file: %w", engineName, err),
			})
		}
	}
	if paths.CAFile != "" {
		if err := requirePath(paths.CAFile, pathresolver.KindFile); err != nil {
			validationErrors = append(validationErrors, &engineconfig.EngineFieldError{
				Field: "ca_file",
				Err:   fmt.Errorf("%s CA file must be a regular file: %w", engineName, err),
			})
		}
	}
	pluginDirs := engineconfig.NormalizePathEntries(paths.PluginDirs, paths.PluginsDir)
	if err := pluginDirs.CheckPaths(engineconfig.PathKindDirectory, engineName+" plugins", "plugins_dir"); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if strings.TrimSpace(paths.WorkingDirectory) != "" {
		if err := requirePath(paths.WorkingDirectory, pathresolver.KindDirectory); err != nil {
			validationErrors = append(validationErrors, &engineconfig.EngineFieldError{
				Field: "working_directory",
				Err:   fmt.Errorf("%s working directory must be a directory: %w", engineName, err),
			})
		}
	}
	return errors.Join(validationErrors...)
}

// ExpandProcessEnvironment expands APP_DIR and ENGINE_DIR in a copied environment map.
func ExpandProcessEnvironment(engineBinary string, environment map[string]string) (map[string]string, error) {
	expandedEnvironment := cloneEnvironment(environment)
	if len(expandedEnvironment) == 0 {
		return expandedEnvironment, nil
	}
	variables := processVariables(engineBinary)
	names := make([]string, 0, len(expandedEnvironment))
	for name := range expandedEnvironment {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		expanded, err := expand.Expand(expandedEnvironment[name], variables, false, false)
		if err != nil {
			return nil, fmt.Errorf("expand process environment variable %q: %w", name, err)
		}
		expandedEnvironment[name] = expanded
	}
	return expandedEnvironment, nil
}

// ResolvePluginsDir validates a configured plugin directory and returns its absolute path.
func ResolvePluginsDir(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", nil
	}
	path, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve plugins_dir: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect plugins_dir %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("plugins_dir %q is not a directory", path)
	}
	return path, nil
}

// ResolvePluginsDirs resolves configured paths and skips missing optional entries.
func ResolvePluginsDirs(entries engineconfig.PathEntries, legacyPath string) ([]string, error) {
	entries = engineconfig.NormalizePathEntries(entries, legacyPath)
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path, err := ResolvePluginsDir(entry.Path)
		if err != nil {
			if !entry.Required && errors.Is(err, os.ErrNotExist) {
				continue
			}
			if entry.Name != "" {
				return nil, fmt.Errorf("plugins_dir[%q].path: %w", entry.Name, err)
			}
			return nil, err
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func resolveBinaryPath(engineName, binaryPath string, options pathResolutionOptions) (string, error) {
	expanded, err := expand.Expand(strings.TrimSpace(binaryPath), options.Variables, false, true)
	if err != nil {
		return "", fmt.Errorf("expand %s binary path: %w", engineName, err)
	}
	if isBareCommandName(expanded) {
		if resolved, err := exec.LookPath(expanded); err == nil {
			return resolved, nil
		}
	}
	result, err := pathresolver.Resolve(expanded, pathresolver.Options{
		Kind: pathresolver.KindAny, ExecutablePath: options.ExecutablePath,
	})
	if err != nil {
		return "", fmt.Errorf("resolve %s binary path: %w", engineName, err)
	}
	return result.Path, nil
}

func isBareCommandName(value string) bool {
	return value != "" && !filepath.IsAbs(value) && filepath.VolumeName(value) == "" && !strings.ContainsAny(value, `/\\`)
}

func processVariables(engineBinary string) map[string]string {
	variables := make(map[string]string, 2)
	if executable, err := os.Executable(); err == nil {
		if absolute, absErr := filepath.Abs(executable); absErr == nil {
			variables["APP_DIR"] = filepath.Dir(absolute)
		}
	}
	if engineBinary != "" {
		if absolute, err := filepath.Abs(engineBinary); err == nil {
			variables["ENGINE_DIR"] = filepath.Dir(absolute)
		}
	}
	return variables
}

type pathResolutionOptions struct {
	ExecutablePath string
	Variables      map[string]string
}

func resolveEnginePath(engineName, pathName, path string, options pathResolutionOptions) (string, error) {
	expanded, err := expand.Expand(path, options.Variables, false, true)
	if err != nil {
		return "", fmt.Errorf("expand %s %s: %w", engineName, pathName, err)
	}
	result, err := pathresolver.Resolve(expanded, pathresolver.Options{
		Kind: pathresolver.KindAny, ExecutablePath: options.ExecutablePath,
	})
	if err != nil {
		return "", fmt.Errorf("resolve %s %s: %w", engineName, pathName, err)
	}
	return result.Path, nil
}

func requirePath(path string, kind pathresolver.Kind) error {
	result, err := pathresolver.Resolve(path, pathresolver.Options{Kind: kind})
	if err != nil {
		return err
	}
	if !result.Exists {
		return fmt.Errorf("path %q does not exist", path)
	}
	return nil
}

func cloneEnvironment(environment map[string]string) map[string]string {
	if len(environment) == 0 {
		return nil
	}
	copy := make(map[string]string, len(environment))
	for name, value := range environment {
		copy[name] = value
	}
	return copy
}
