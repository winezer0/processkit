// Package expand expands caller-approved ${NAME} variables and optional environment variables.
package expand

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	homeDirName       = "HOME_DIR"
	tempDirName       = "TEMP_DIR"
	userConfigDirName = "USER_CONFIG_DIR"
	userCacheDirName  = "USER_CACHE_DIR"
)

var builtInNames = map[string]struct{}{
	homeDirName:       {},
	tempDirName:       {},
	userConfigDirName: {},
	userCacheDirName:  {},
}

// Expand replaces built-in directory variables and variables explicitly supplied by the caller.
// When expandEnv is true, ${NAME} expressions can also resolve from the process environment.
// Variable values are inserted literally and are not recursively expanded.
func Expand(input string, variables map[string]string, expandEnv bool) (string, error) {
	if err := validateVariables(variables); err != nil {
		return "", err
	}
	return expandStrict(input, variables, expandEnv)
}

// ExpandPath expands a leading ~/ or ~\ current-user prefix and then expands variables.
// Tilde prefixes elsewhere in the input are left unchanged.
func ExpandPath(input string, variables map[string]string, expandEnv bool) (string, error) {
	if len(input) >= 2 && input[0] == '~' && (input[1] == '/' || input[1] == '\\') {
		homeDirectory, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		if homeDirectory == "" {
			return "", fmt.Errorf("user home directory is empty")
		}
		input = filepath.Join(homeDirectory, input[2:])
	}
	return Expand(input, variables, expandEnv)
}

func expandStrict(input string, variables map[string]string, expandEnv bool) (string, error) {
	var output strings.Builder
	for offset := 0; offset < len(input); {
		if offset+1 >= len(input) || input[offset] != '$' || input[offset+1] != '{' {
			output.WriteByte(input[offset])
			offset++
			continue
		}

		closeOffset := strings.IndexByte(input[offset+2:], '}')
		if closeOffset < 0 {
			return "", fmt.Errorf("unterminated variable expression at byte %d", offset)
		}
		closeOffset += offset + 2
		name := input[offset+2 : closeOffset]
		if !validName(name) {
			return "", fmt.Errorf("invalid variable name %q", name)
		}

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
		offset = closeOffset + 1
	}

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
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func builtInValue(name string) (string, bool, error) {
	var value string
	var err error
	switch name {
	case homeDirName:
		value, err = os.UserHomeDir()
	case tempDirName:
		value = os.TempDir()
	case userConfigDirName:
		value, err = os.UserConfigDir()
	case userCacheDirName:
		value, err = os.UserCacheDir()
	default:
		return "", false, nil
	}
	if err != nil {
		return "", true, fmt.Errorf("resolve built-in variable %q: %w", name, err)
	}
	if value == "" {
		return "", true, fmt.Errorf("built-in variable %q resolved to an empty value", name)
	}
	return value, true, nil
}
