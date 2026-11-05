// Package expand expands caller-approved ${NAME} variables and optional environment variables.
package expand

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	homeDirName       = "HOME_DIR"
	homeConfigDirName = "HOME_CONFIG_DIR"
	tempDirName       = "TEMP_DIR"
	userConfigDirName = "USER_CONFIG_DIR"
	userCacheDirName  = "USER_CACHE_DIR"
	nowWorkDirName    = "NOW_WORK_DIR"
	nowExePathName    = "NOW_EXE_PATH"
	nowExeDirName     = "NOW_EXE_DIR"
)

var builtInNames = map[string]struct{}{
	homeDirName:       {},
	homeConfigDirName: {},
	tempDirName:       {},
	userConfigDirName: {},
	userCacheDirName:  {},
	nowWorkDirName:    {},
	nowExePathName:    {},
	nowExeDirName:     {},
}

var variableExpression = regexp.MustCompile(`\$\{\s*([\w_]+)\s*\}`)
var variableNameExpression = regexp.MustCompile(`^[\w_]+$`)

// Expand replaces built-in variables and variables explicitly supplied by the caller.
// When expandEnv is true, ${NAME} expressions can also resolve from the process environment.
// When expandHome is true, a leading ~/ or ~\ prefix is replaced with the current user's home directory.
// Variable values are inserted literally and are not recursively expanded.
func Expand(input string, variables map[string]string, expandEnv, expandHome bool) (string, error) {
	if err := validateVariables(variables); err != nil {
		return "", err
	}
	if expandHome && len(input) >= 2 && input[0] == '~' && (input[1] == '/' || input[1] == '\\') {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		if homeDirectory == "" {
			return "", fmt.Errorf("user home directory is empty")
		}
		homeDirectory, err = filepath.Abs(homeDirectory)
		if err != nil {
			return "", fmt.Errorf("make user home directory absolute: %w", err)
		}
		input = filepath.Join(homeDirectory, input[2:])
	}
	return expandStrict(input, variables, expandEnv)
}

func expandStrict(input string, variables map[string]string, expandEnv bool) (string, error) {
	var output strings.Builder
	lastOffset := 0
	for _, match := range variableExpression.FindAllStringSubmatchIndex(input, -1) {
		output.WriteString(input[lastOffset:match[0]])
		name := strings.TrimSpace(input[match[2]:match[3]])

		value, exists := variables[name]
		if !exists {
			var err error
			value, exists, err = builtInValue(name)
			if err != nil {
				return "", err
			}
		}
		environmentLookup := !exists && expandEnv
		if environmentLookup {
			value, _ = os.LookupEnv(name)
			exists = true
		}
		if !exists {
			return "", fmt.Errorf("unknown variable %q", name)
		}
		if value == "" && !environmentLookup {
			return "", fmt.Errorf("variable %q has an empty value", name)
		}

		output.WriteString(value)
		lastOffset = match[1]
	}
	output.WriteString(input[lastOffset:])
	return output.String(), nil
}

func validateVariables(variables map[string]string) error {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !validName(name) {
			return fmt.Errorf("invalid variable name %q", name)
		}
		if _, reserved := builtInNames[name]; reserved {
			return fmt.Errorf("built-in variable %q cannot be overridden", name)
		}
		if variables[name] == "" {
			return fmt.Errorf("variable %q has an empty value", name)
		}
	}
	return nil
}

func validName(name string) bool {
	return variableNameExpression.MatchString(name)
}

func builtInValue(name string) (string, bool, error) {
	var value string
	var err error
	switch name {
	case homeDirName:
		value, err = os.UserHomeDir()
	case homeConfigDirName:
		var homeDirectory string
		homeDirectory, err = os.UserHomeDir()
		if err == nil {
			if homeDirectory == "" {
				return "", true, fmt.Errorf("built-in variable %q resolved to an empty home directory", name)
			}
			value = filepath.Join(homeDirectory, ".config")
		}
	case tempDirName:
		value = os.TempDir()
	case userConfigDirName:
		value, err = os.UserConfigDir()
	case userCacheDirName:
		value, err = os.UserCacheDir()
	case nowWorkDirName:
		value, err = os.Getwd()
	case nowExePathName:
		value, err = os.Executable()
	case nowExeDirName:
		var executablePath string
		executablePath, err = os.Executable()
		if err == nil {
			value = filepath.Dir(executablePath)
		}
	default:
		return "", false, nil
	}
	if err != nil {
		return "", true, fmt.Errorf("resolve built-in variable %q: %w", name, err)
	}
	if value == "" {
		return "", true, fmt.Errorf("built-in variable %q resolved to an empty value", name)
	}
	if !filepath.IsAbs(value) {
		value, err = filepath.Abs(value)
		if err != nil {
			return "", true, fmt.Errorf("resolve built-in variable %q to an absolute path: %w", name, err)
		}
	}
	return value, true, nil
}
