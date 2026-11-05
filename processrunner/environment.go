package processrunner

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
)

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copy := make(map[string]string, len(values))
	for name, value := range values {
		copy[name] = value
	}
	return copy
}

func validateEnvironment(environment map[string]string) error {
	for name, value := range environment {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("invalid process environment variable %q", name)
		}
	}
	return nil
}

func environmentEntries(environment map[string]string) []string {
	names := sortedEnvironmentNames(environment)
	entries := make([]string, 0, len(names))
	for _, name := range names {
		entries = append(entries, name+"="+environment[name])
	}
	return entries
}

func sortedEnvironmentNames(environment map[string]string) []string {
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	entries := environmentEntries(overrides)
	merged := make([]string, 0, len(base)+len(entries))
	indices := make(map[string]int, len(base)+len(entries))
	appendEntry := func(entry string) {
		name, ok := environmentEntryName(entry)
		if !ok {
			merged = append(merged, entry)
			return
		}
		key := name
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		if index, exists := indices[key]; exists {
			merged[index] = entry
			return
		}
		indices[key] = len(merged)
		merged = append(merged, entry)
	}
	for _, entry := range base {
		appendEntry(entry)
	}
	for _, entry := range entries {
		appendEntry(entry)
	}
	return merged
}

func environmentEntryName(entry string) (string, bool) {
	start := 0
	if strings.HasPrefix(entry, "=") {
		start = 1
	}
	separator := strings.IndexByte(entry[start:], '=')
	if separator < 0 {
		return "", false
	}
	return entry[:start+separator], true
}
