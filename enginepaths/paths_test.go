package enginepaths

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/winezer0/processkit/engineconfig"
	"github.com/winezer0/processkit/pathresolver"
)

func TestResolveEnginePathsExpandsVariablesAndCopiesEnvironment(t *testing.T) {
	engineDir := filepath.Join(t.TempDir(), "engine")
	binary := filepath.Join(engineDir, "nuclei")
	environment := map[string]string{"ENGINE_CACHE": "${ENGINE_DIR}/cache", "APP_CACHE": "${APP_DIR}/cache"}
	workingDirectory := filepath.Join(t.TempDir(), "work")
	if err := os.Mkdir(workingDirectory, 0o700); err != nil {
		t.Fatal(err)
	}

	paths, err := ResolveEnginePaths(EnginePathsConfig{
		EngineName:       "nuclei",
		BinaryPath:       binary,
		ConfigPath:       "${ENGINE_DIR}/config.yaml",
		CAFile:           "${ENGINE_DIR}/ca.crt",
		PluginDirs:       engineconfig.PathEntries{{Path: "${ENGINE_DIR}/templates", Required: true}},
		WorkingDirectory: workingDirectory,
		Environment:      environment,
	})
	if err != nil {
		t.Fatalf("ResolveEnginePaths() error = %v", err)
	}
	if paths.BinaryPath != binary || paths.ConfigPath != filepath.Join(engineDir, "config.yaml") || paths.CAFile != filepath.Join(engineDir, "ca.crt") {
		t.Fatalf("resolved paths = %#v", paths)
	}
	if paths.PluginDirs[0].Path != filepath.Join(engineDir, "templates") || paths.PluginsDir != paths.PluginDirs[0].Path {
		t.Fatalf("resolved plugin paths = %#v", paths.PluginDirs)
	}
	if paths.WorkingDirectory != workingDirectory {
		t.Fatalf("WorkingDirectory = %q, want %q", paths.WorkingDirectory, workingDirectory)
	}
	if !reflect.DeepEqual(paths.Environment, environment) {
		t.Fatalf("ResolveEnginePaths() changed the environment: got %#v, want a raw copy of %#v", paths.Environment, environment)
	}
	if environment["ENGINE_CACHE"] != "${ENGINE_DIR}/cache" {
		t.Fatal("ResolveEnginePaths() mutated caller-owned environment")
	}
}

func TestResolveEnginePathsFindsBareBinaryOnPATH(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	workDir := filepath.Join(root, "work")
	for _, directory := range []string{binDir, workDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	name := "enginepaths-test-bin"
	mode := os.FileMode(0o700)
	if runtime.GOOS == "windows" {
		name += ".exe"
		mode = 0o600
	}
	want := filepath.Join(binDir, name)
	if err := os.WriteFile(want, []byte("test executable"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(want, mode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	paths, err := ResolveEnginePaths(EnginePathsConfig{EngineName: "test", BinaryPath: name, WorkingDirectory: workDir})
	if err != nil {
		t.Fatalf("ResolveEnginePaths() error = %v", err)
	}
	if paths.BinaryPath != want {
		t.Fatalf("BinaryPath = %q, want %q", paths.BinaryPath, want)
	}
}

func TestResolveEnginePathsKeepsExplicitBinaryPathOutOfPATHLookup(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "enginepaths-explicit-test-bin"
	mode := os.FileMode(0o700)
	if runtime.GOOS == "windows" {
		name += ".exe"
		mode = 0o600
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("test executable"), mode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	explicitPath := filepath.Join(root, "missing", name)

	paths, err := ResolveEnginePaths(EnginePathsConfig{
		EngineName: "test", BinaryPath: explicitPath, WorkingDirectory: root,
	})
	if err != nil {
		t.Fatalf("ResolveEnginePaths() error = %v", err)
	}
	if paths.BinaryPath != explicitPath {
		t.Fatalf("BinaryPath = %q, want explicit path %q", paths.BinaryPath, explicitPath)
	}
	if err := ValidatePaths(paths, "test"); err == nil || !strings.Contains(err.Error(), "binary_path") {
		t.Fatalf("ValidatePaths() error = %v, want missing explicit binary path error", err)
	}
}

func TestResolveStandalonePathExpandsApplicationDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	want := filepath.Dir(executable)
	resolved, err := ResolveStandalonePath("${APP_DIR}", pathresolver.KindDirectory)
	if err != nil {
		t.Fatalf("ResolveStandalonePath() error = %v", err)
	}
	if resolved.Path != want || !resolved.Exists {
		t.Fatalf("ResolveStandalonePath() = %#v, want existing path %q", resolved, want)
	}
}

func TestExpandProcessEnvironmentExpandsBuiltinsWithoutMutation(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test executable: %v", err)
	}
	engineBinary := filepath.Join(t.TempDir(), "engine", "scanner")
	environment := map[string]string{
		"APP_CACHE":    "${APP_DIR}/cache",
		"ENGINE_CACHE": "${ENGINE_DIR}/cache",
	}
	got, err := ExpandProcessEnvironment(engineBinary, environment)
	if err != nil {
		t.Fatalf("ExpandProcessEnvironment() error = %v", err)
	}
	want := map[string]string{
		"APP_CACHE":    filepath.Join(filepath.Dir(executable), "cache"),
		"ENGINE_CACHE": filepath.Join(filepath.Dir(engineBinary), "cache"),
	}
	for key := range want {
		got[key] = filepath.Clean(got[key])
		want[key] = filepath.Clean(want[key])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandProcessEnvironment() = %#v, want %#v", got, want)
	}
	if environment["APP_CACHE"] != "${APP_DIR}/cache" {
		t.Fatal("ExpandProcessEnvironment() mutated caller-owned environment")
	}
}

func TestValidatePathsReportsEveryInvalidFieldAndAllowsMissingOptionalPlugin(t *testing.T) {
	root := t.TempDir()
	validWorkDirectory := filepath.Join(root, "work")
	if err := os.Mkdir(validWorkDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	err := ValidatePaths(ResolvedPaths{
		BinaryPath:       filepath.Join(root, "missing-binary"),
		ConfigPath:       filepath.Join(root, "missing-config"),
		CAFile:           filepath.Join(root, "missing-ca"),
		PluginDirs:       engineconfig.PathEntries{{Path: filepath.Join(root, "optional-plugins"), Required: false}},
		WorkingDirectory: validWorkDirectory,
	}, "nuclei")
	if err == nil {
		t.Fatal("ValidatePaths() error = nil")
	}
	for _, field := range []string{"binary_path", "config_path", "ca_file"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("ValidatePaths() error %q does not identify %s", err, field)
		}
	}
	if strings.Contains(err.Error(), "plugins_dir") {
		t.Errorf("ValidatePaths() error %q reports a missing optional plugin directory", err)
	}
}

func TestResolvePluginsDirsHonorsRequiredEntries(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	if err := os.Mkdir(plugins, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ResolvePluginsDirs(engineconfig.PathEntries{
		{Path: plugins, Required: true},
		{Path: filepath.Join(root, "optional-missing"), Required: false},
	}, "")
	if err != nil {
		t.Fatalf("ResolvePluginsDirs() error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{plugins}) {
		t.Fatalf("ResolvePluginsDirs() = %#v, want only %q", got, plugins)
	}
	if _, err := ResolvePluginsDirs(engineconfig.PathEntries{{Path: filepath.Join(root, "required-missing"), Required: true}}, ""); err == nil {
		t.Fatal("ResolvePluginsDirs() error = nil for a missing required directory")
	}
}
