package engineconfig

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// PathEntry configures one required or optional engine file or directory.
type PathEntry struct {
	Path     string `yaml:"path"`
	Required bool   `yaml:"required"`
}

// PathEntries accepts a single path or a list of path objects in YAML.
type PathEntries []PathEntry

// PathKind describes the filesystem object expected at a configured path.
type PathKind string

const (
	PathKindFile      PathKind = "file"
	PathKindDirectory PathKind = "directory"
)

// UnmarshalYAML accepts a scalar shorthand or a list of objects.
// Non-empty shorthand paths and entries default to required.
func (entries *PathEntries) UnmarshalYAML(node *yaml.Node) error {
	return entries.UnmarshalYAMLField(node, "plugins_dir")
}

// UnmarshalYAMLField decodes path entries and prefixes parse errors with their config field.
// Field-specific wrappers can call this when adding other list-valued file settings.
func (entries *PathEntries) UnmarshalYAMLField(node *yaml.Node, field string) error {
	prefix := strings.Trim(strings.TrimSpace(field), ".")
	if prefix == "" {
		prefix = "path_entries"
	}
	if node == nil || node.Tag == "!!null" {
		*entries = nil
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		if node.Tag != "!!str" {
			return &EngineFieldError{Field: prefix, Err: fmt.Errorf("expected a path string or list of path objects, got %s", node.Tag)}
		}
		path := strings.TrimSpace(node.Value)
		if path == "" {
			*entries = nil
			return nil
		}
		*entries = PathEntries{{Path: path, Required: true}}
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return &EngineFieldError{Field: prefix, Err: fmt.Errorf("expected a path string or list of path objects, got %s", node.Tag)}
	}

	parsed := make(PathEntries, 0, len(node.Content))
	for index, item := range node.Content {
		entry, err := parsePathEntry(item)
		if err != nil {
			var fieldError *EngineFieldError
			if errors.As(err, &fieldError) {
				return &EngineFieldError{Field: fmt.Sprintf("%s[%d].%s", prefix, index, fieldError.Field), Err: fieldError.Err}
			}
			return &EngineFieldError{Field: fmt.Sprintf("%s[%d]", prefix, index), Err: err}
		}
		parsed = append(parsed, entry)
	}
	*entries = parsed
	return nil
}

func parsePathEntry(node *yaml.Node) (PathEntry, error) {
	if node.Kind != yaml.MappingNode {
		return PathEntry{}, fmt.Errorf("expected object, got %s", node.Tag)
	}
	entry := PathEntry{Required: true}
	seen := make(map[string]bool, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index].Value, node.Content[index+1]
		if seen[key] {
			return PathEntry{}, &EngineFieldError{Field: key, Err: fmt.Errorf("duplicate field %q", key)}
		}
		seen[key] = true
		switch key {
		case "path":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return PathEntry{}, &EngineFieldError{Field: "path", Err: fmt.Errorf("must be a string")}
			}
			entry.Path = strings.TrimSpace(value.Value)
		case "required":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!bool" {
				return PathEntry{}, &EngineFieldError{Field: "required", Err: fmt.Errorf("must be a boolean")}
			}
			if err := value.Decode(&entry.Required); err != nil {
				return PathEntry{}, &EngineFieldError{Field: "required", Err: fmt.Errorf("must be a boolean: %w", err)}
			}
		default:
			return PathEntry{}, &EngineFieldError{Field: key, Err: fmt.Errorf("unknown field %q", key)}
		}
	}
	if entry.Path == "" {
		return PathEntry{}, &EngineFieldError{Field: "path", Err: fmt.Errorf("path is required and must not be empty")}
	}
	return entry, nil
}

// NormalizePathEntries preserves a legacy scalar field while providing a canonical list.
func NormalizePathEntries(entries PathEntries, legacyPath string) PathEntries {
	if len(entries) == 0 {
		legacyPath = strings.TrimSpace(legacyPath)
		if legacyPath == "" {
			return nil
		}
		return PathEntries{{Path: legacyPath, Required: true}}
	}
	normalized := make(PathEntries, 0, len(entries))
	for _, entry := range entries {
		entry.Path = strings.TrimSpace(entry.Path)
		if entry.Path != "" {
			normalized = append(normalized, entry)
		}
	}
	return normalized
}

// Validate checks that every configured path entry has a non-empty path.
func (entries PathEntries) Validate() error {
	return entries.ValidateField("path_entries")
}

// ValidateField validates entries and prefixes errors with the owning config field.
func (entries PathEntries) ValidateField(field string) error {
	prefix := strings.Trim(strings.TrimSpace(field), ".")
	if prefix == "" {
		prefix = "path_entries"
	}
	var validationErrors []error
	for index, entry := range entries {
		if strings.TrimSpace(entry.Path) == "" {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: fmt.Sprintf("%s[%d].path", prefix, index),
				Err:   fmt.Errorf("must not be empty"),
			})
		}
	}
	return errors.Join(validationErrors...)
}

// CheckPaths verifies required paths and the type of every path that exists.
// Missing optional paths are ignored; other filesystem errors are returned.
func (entries PathEntries) CheckPaths(kind PathKind, owner, field string) error {
	prefix := strings.Trim(strings.TrimSpace(field), ".")
	if prefix == "" {
		prefix = "path_entries"
	}
	var validationErrors []error
	for index, entry := range entries {
		path := strings.TrimSpace(entry.Path)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if !entry.Required && os.IsNotExist(err) {
				continue
			}
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: fmt.Sprintf("%s[%d].path", prefix, index),
				Err:   fmt.Errorf("%s %s %q: %w", owner, kind, path, err),
			})
			continue
		}
		if kind == PathKindFile && !info.Mode().IsRegular() {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: fmt.Sprintf("%s[%d].path", prefix, index),
				Err:   fmt.Errorf("%s %s %q is not a regular file", owner, kind, path),
			})
		}
		if kind == PathKindDirectory && !info.IsDir() {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: fmt.Sprintf("%s[%d].path", prefix, index),
				Err:   fmt.Errorf("%s %s %q is not a directory", owner, kind, path),
			})
		}
	}
	return errors.Join(validationErrors...)
}
