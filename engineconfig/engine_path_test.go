package engineconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPathEntriesUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		document string
		want     PathEntries
		wantErr  string
	}{
		{name: "empty string", document: "plugins_dir: \"\"\n", want: nil},
		{name: "string shorthand", document: "plugins_dir: \" ./templates \"\n", want: PathEntries{{Path: "./templates", Required: true}}},
		{name: "list defaults required", document: "plugins_dir:\n  - path: ./templates\n", want: PathEntries{{Path: "./templates", Required: true}}},
		{name: "named list defaults required", document: "plugins_dir:\n  - name: templates\n    path: ./templates\n", want: PathEntries{{Name: "templates", Path: "./templates", Required: true}}},
		{name: "list overrides required", document: "plugins_dir:\n  - path: ./templates\n    required: false\n", want: PathEntries{{Path: "./templates", Required: false}}},
		{name: "list mixes defaults", document: "plugins_dir:\n  - path: ./templates\n  - path: ./custom\n    required: false\n", want: PathEntries{{Path: "./templates", Required: true}, {Path: "./custom", Required: false}}},
		{name: "named list mixes required settings", document: "plugins_dir:\n  - name: templates\n    path: ./templates\n  - name: custom-pocs\n    path: ./custom\n    required: false\n", want: PathEntries{{Name: "templates", Path: "./templates", Required: true}, {Name: "custom-pocs", Path: "./custom", Required: false}}},
		{name: "list item needs path", document: "plugins_dir:\n  - required: false\n", wantErr: "path is required"},
		{name: "list item needs object", document: "plugins_dir:\n  - ./templates\n", wantErr: "expected object"},
		{name: "empty name rejected", document: "plugins_dir:\n  - name: \" \"\n    path: ./templates\n", wantErr: "must not be empty"},
		{name: "duplicate names rejected", document: "plugins_dir:\n  - name: templates\n    path: ./templates\n  - name: templates\n    path: ./custom\n", wantErr: `plugins_dir["templates"].name`},
		{name: "unsupported map", document: "plugins_dir:\n  path: ./templates\n", wantErr: "expected a path string or list"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config struct {
				Paths PathEntries `yaml:"plugins_dir"`
			}
			err := yaml.Unmarshal([]byte(test.document), &config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Unmarshal() error = %v, want substring %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if len(config.Paths) != len(test.want) {
				t.Fatalf("PathEntries = %#v, want %#v", config.Paths, test.want)
			}
			for index, want := range test.want {
				if config.Paths[index] != want {
					t.Errorf("PathEntries[%d] = %#v, want %#v", index, config.Paths[index], want)
				}
			}
		})
	}
}

func TestPathEntriesUnmarshalYAMLFieldUsesCallerField(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("- required: true\n"), &document); err != nil {
		t.Fatal(err)
	}
	var entries PathEntries
	err := entries.UnmarshalYAMLField(document.Content[0], "ca_file")
	if err == nil || !strings.Contains(err.Error(), "ca_file[0].path") {
		t.Fatalf("UnmarshalYAMLField() error = %v, want ca_file[0].path", err)
	}
}

func TestPathEntriesCheckPaths(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "templates")
	file := filepath.Join(root, "ca.crt")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := (PathEntries{{Path: filepath.Join(root, "optional-missing"), Required: false}}).CheckPaths(PathKindDirectory, "test", "plugins_dir"); err != nil {
		t.Fatalf("CheckPaths(optional missing) error = %v", err)
	}
	if err := (PathEntries{{Path: directory, Required: true}}).CheckPaths(PathKindDirectory, "test", "plugins_dir"); err != nil {
		t.Fatalf("CheckPaths(existing directory) error = %v", err)
	}
	if err := (PathEntries{{Path: filepath.Join(root, "required-missing"), Required: true}}).CheckPaths(PathKindDirectory, "test", "plugins_dir"); err == nil {
		t.Fatal("CheckPaths(required missing) error = nil")
	}
	if err := (PathEntries{{Path: file, Required: true}}).CheckPaths(PathKindDirectory, "test", "plugins_dir"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("CheckPaths(wrong type) error = %v, want not a directory", err)
	}
	if err := (PathEntries{{Path: file, Required: true}}).CheckPaths(PathKindFile, "test", "ca_file"); err != nil {
		t.Fatalf("CheckPaths(existing file) error = %v", err)
	}
}

func TestPathEntriesCheckPathsReportsEveryInvalidEntry(t *testing.T) {
	root := t.TempDir()
	entries := PathEntries{
		{Name: "custom-pocs", Path: filepath.Join(root, "missing-required-a"), Required: true},
		{Name: "exploit-rules", Path: filepath.Join(root, "missing-required-b"), Required: true},
	}
	err := entries.CheckPaths(PathKindDirectory, "nuclei plugins", "plugins_dir")
	if err == nil {
		t.Fatal("CheckPaths() error = nil")
	}
	for _, field := range []string{`plugins_dir["custom-pocs"].path`, `plugins_dir["exploit-rules"].path`, "missing-required-a", "missing-required-b"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("CheckPaths() error %q does not identify %s", err, field)
		}
	}
}

func TestPathEntriesCheckPathsUsesCallerField(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-ca.crt")
	err := (PathEntries{{Name: "custom-ca", Path: missing, Required: true}}).CheckPaths(PathKindFile, "xray CA", "ca_file")
	if err == nil || !strings.Contains(err.Error(), `ca_file["custom-ca"].path`) {
		t.Fatalf("CheckPaths() error = %v, want named ca_file path", err)
	}
}

func TestPathEntriesValidateRejectsDuplicateNames(t *testing.T) {
	entries := PathEntries{
		{Name: "templates", Path: "./official", Required: true},
		{Name: "templates", Path: "./custom", Required: false},
	}
	err := entries.ValidateField("plugins_dir")
	if err == nil || !strings.Contains(err.Error(), `plugins_dir["templates"].name`) {
		t.Fatalf("ValidateField() error = %v, want duplicate named entry error", err)
	}
}
