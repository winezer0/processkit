package engineconfig

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
)

// BinaryPaths maps GOOS_GOARCH platform keys to executable paths.
// It accepts platforms beyond the combinations currently shipped by Tanscan.
type BinaryPaths map[string]string

// Current returns the configured executable path for the current OS and architecture.
func (paths BinaryPaths) Current() (path, key string) {
	return paths.For(runtime.GOOS, runtime.GOARCH)
}

// For returns the configured path and normalized key for an OS/architecture pair.
func (paths BinaryPaths) For(goos, goarch string) (path, key string) {
	key = strings.ToLower(strings.TrimSpace(goos)) + "_" + strings.ToLower(strings.TrimSpace(goarch))
	return paths[key], key
}

// Normalize trims configured executable paths without restricting platform keys.
func (paths BinaryPaths) Normalize() BinaryPaths {
	if paths == nil {
		return nil
	}
	normalized := make(BinaryPaths, len(paths))
	for key, path := range paths {
		normalized[key] = strings.TrimSpace(path)
	}
	return normalized
}

// Validate checks that each platform key uses lowercase alphanumeric components.
func (paths BinaryPaths) Validate() error {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var validationErrors []error
	for _, key := range keys {
		if !validPlatformKey(key) {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: "binary_path." + key,
				Err:   fmt.Errorf("must use lowercase GOOS_GOARCH identifiers"),
			})
		}
		if strings.TrimSpace(paths[key]) == "" {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: "binary_path." + key,
				Err:   fmt.Errorf("must not be empty"),
			})
		}
	}
	return errors.Join(validationErrors...)
}

func validPlatformKey(key string) bool {
	if !strings.Contains(key, "_") {
		return false
	}
	for _, part := range strings.Split(key, "_") {
		if !validPlatformIdentifier(part) {
			return false
		}
	}
	return true
}

func validPlatformIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}
