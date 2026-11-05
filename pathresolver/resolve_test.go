package pathresolver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAbsolutePathDirectly(t *testing.T) {
	target := writeFile(t, t.TempDir(), "asset.yaml")
	result, err := Resolve(target, Options{ProcessDir: filepath.Join(t.TempDir(), "missing"), Kind: KindFile})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceAbsolute || result.Base != "" {
		t.Fatalf("Resolve() = %#v", result)
	}
	if !result.Exists {
		t.Fatal("Resolve() Exists = false, want true")
	}
}

func TestResolveExpandsEnvironmentVariablesWhenEnabled(t *testing.T) {
	t.Setenv("PROCESSKIT_PATHRESOLVER_DIR", t.TempDir())
	result, err := Resolve("${PROCESSKIT_PATHRESOLVER_DIR}", Options{
		Kind:      KindDirectory,
		ExpandEnv: true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !result.Exists {
		t.Fatalf("Resolve() Exists = false, want true")
	}
}

func TestResolveDoesNotExpandEnvironmentVariablesByDefault(t *testing.T) {
	t.Setenv("PROCESSKIT_PATHRESOLVER_DIR", t.TempDir())
	_, err := Resolve("${PROCESSKIT_PATHRESOLVER_DIR}", Options{Kind: KindDirectory})
	if err == nil || !strings.Contains(err.Error(), `unknown variable "PROCESSKIT_PATHRESOLVER_DIR"`) {
		t.Fatalf("Resolve() error = %v, want unknown variable error", err)
	}
}

func TestResolveExpandsHomeRelativePath(t *testing.T) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	result, err := Resolve("~/", Options{Kind: KindDirectory})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != filepath.Clean(homeDirectory) || !result.Exists {
		t.Fatalf("Resolve() = %#v, want existing home directory %q", result, homeDirectory)
	}
}

func TestResolveAbsoluteDirectoryDirectly(t *testing.T) {
	target := t.TempDir()
	result, err := Resolve(target, Options{Kind: KindDirectory})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceAbsolute || result.Base != "" {
		t.Fatalf("Resolve() = %#v", result)
	}
}

func TestResolveAbsoluteMissingPathDoesNotUseFallbacks(t *testing.T) {
	base := t.TempDir()
	writeFile(t, base, "missing.yaml")
	target := filepath.Join(t.TempDir(), "missing.yaml")
	result, err := Resolve(target, Options{ProcessDir: base, Kind: KindFile})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceAbsolute || result.Base != "" || result.Exists {
		t.Fatalf("Resolve() = %#v, want missing absolute path without fallback", result)
	}
}

func TestResolveUsesProcessDirBeforeWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	writeFile(t, home, "asset.yaml")
	writeFile(t, workingDirectory, "asset.yaml")
	writeFile(t, executableDirectory, "asset.yaml")

	result, err := Resolve("asset.yaml", Options{
		ProcessDir:       home,
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceProcessDir || result.Path != filepath.Join(home, "asset.yaml") {
		t.Fatalf("Resolve() = %#v, want process directory candidate", result)
	}
}

func TestResolveRelativeProcessDirUsesWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	home := filepath.Join("runtime", "home")
	target := filepath.Join(workingDirectory, home, "asset.yaml")
	writeFile(t, filepath.Join(workingDirectory, home), "asset.yaml")

	result, err := Resolve("asset.yaml", Options{
		ProcessDir:       home,
		WorkingDirectory: workingDirectory,
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceProcessDir || result.Base != filepath.Dir(target) {
		t.Fatalf("Resolve() = %#v, want relative process directory %q", result, filepath.Dir(target))
	}
}

func TestResolveFallsBackFromInvalidProcessDirToWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	writeFile(t, workingDirectory, "asset.yaml")
	homeFile := writeFile(t, t.TempDir(), "home-is-a-file")

	result, err := Resolve("asset.yaml", Options{
		ProcessDir:       homeFile,
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(t.TempDir(), "bin", "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceWorkingDirectory {
		t.Fatalf("Resolve() source = %q, want %q", result.Source, SourceWorkingDirectory)
	}
}

func TestResolveFallsBackToExecutableDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	writeFile(t, executableDirectory, "asset.yaml")
	executablePath := filepath.Join(executableDirectory, "app")

	result, err := Resolve("asset.yaml", Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   executablePath,
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceExecutableDirectory || result.Path != filepath.Join(executableDirectory, "asset.yaml") {
		t.Fatalf("Resolve() = %#v, want executable directory", result)
	}
}

func TestResolveWorkingDirectoryPrecedesExecutableDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	writeFile(t, workingDirectory, "asset.yaml")
	writeFile(t, executableDirectory, "asset.yaml")

	result, err := Resolve("asset.yaml", Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceWorkingDirectory {
		t.Fatalf("Resolve() source = %q, want %q", result.Source, SourceWorkingDirectory)
	}
}

func TestResolveChecksAdditionalBasesInOrder(t *testing.T) {
	firstBase := t.TempDir()
	secondBase := t.TempDir()
	writeFile(t, firstBase, "asset.yaml")
	writeFile(t, secondBase, "asset.yaml")

	result, err := Resolve("asset.yaml", Options{
		WorkingDirectory: t.TempDir(),
		AdditionalBases: []Candidate{
			{Path: firstBase, Source: "custom-one"},
			{Path: secondBase, Source: "custom-two"},
		},
		Kind: KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != "custom-one" || result.Base != firstBase {
		t.Fatalf("Resolve() = %#v, want first custom base", result)
	}
}

func TestResolveExecutableDirectoryPrecedesAdditionalBases(t *testing.T) {
	executableDirectory := t.TempDir()
	additionalBase := t.TempDir()
	writeFile(t, executableDirectory, "asset.yaml")
	writeFile(t, additionalBase, "asset.yaml")

	result, err := Resolve("asset.yaml", Options{
		WorkingDirectory: t.TempDir(),
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		AdditionalBases:  []Candidate{{Path: additionalBase}},
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceExecutableDirectory {
		t.Fatalf("Resolve() source = %q, want %q", result.Source, SourceExecutableDirectory)
	}
}

func TestResolveSkipsInvalidAdditionalBases(t *testing.T) {
	tests := []struct {
		name     string
		makeBase func(*testing.T) string
	}{
		{
			name: "missing directory",
			makeBase: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
		},
		{
			name: "regular file",
			makeBase: func(t *testing.T) string {
				return writeFile(t, t.TempDir(), "base-file")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validBase := t.TempDir()
			writeFile(t, validBase, "asset.yaml")
			result, err := Resolve("asset.yaml", Options{
				WorkingDirectory: t.TempDir(),
				ExecutablePath:   filepath.Join(t.TempDir(), "bin", "app"),
				AdditionalBases: []Candidate{
					{Path: test.makeBase(t), Source: "invalid"},
					{Path: validBase},
				},
				Kind: KindFile,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if result.Source != SourceFallback || result.Base != validBase {
				t.Fatalf("Resolve() = %#v, want second fallback base", result)
			}
		})
	}
}

func TestResolveWorkingDirectoryAcceptsRelativeOverride(t *testing.T) {
	workingDirectory := t.TempDir()
	currentDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(currentDirectory, workingDirectory)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveWorkingDirectory(relative)
	if err != nil {
		t.Fatalf("resolveWorkingDirectory() error = %v", err)
	}
	if resolved != workingDirectory {
		t.Fatalf("resolveWorkingDirectory() = %q, want %q", resolved, workingDirectory)
	}
}

func TestResolveChecksCurrentPathsOnEveryCall(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	writeFile(t, workingDirectory, "asset.yaml")
	options := Options{ProcessDir: home, WorkingDirectory: workingDirectory, Kind: KindFile}

	first, err := Resolve("asset.yaml", options)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if first.Source != SourceWorkingDirectory {
		t.Fatalf("first Resolve() source = %q", first.Source)
	}
	writeFile(t, home, "asset.yaml")

	second, err := Resolve("asset.yaml", options)
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if second.Source != SourceProcessDir {
		t.Fatalf("second Resolve() source = %q, want %q", second.Source, SourceProcessDir)
	}
}

func TestResolveFallsBackWhenTargetKindDoesNotMatch(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		homeKind    Kind
		workingKind Kind
	}{
		{name: "directory required but home has file", kind: KindDirectory, homeKind: KindFile, workingKind: KindDirectory},
		{name: "file required but home has directory", kind: KindFile, homeKind: KindDirectory, workingKind: KindFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			workingDirectory := t.TempDir()
			createTarget(t, home, "asset", test.homeKind)
			createTarget(t, workingDirectory, "asset", test.workingKind)

			result, err := Resolve("asset", Options{
				ProcessDir:       home,
				WorkingDirectory: workingDirectory,
				Kind:             test.kind,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if result.Source != SourceWorkingDirectory {
				t.Fatalf("Resolve() source = %q, want %q", result.Source, SourceWorkingDirectory)
			}
		})
	}
}

func TestResolveAcceptsAnyExistingPath(t *testing.T) {
	workingDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(workingDirectory, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := Resolve("assets", Options{WorkingDirectory: workingDirectory})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != filepath.Join(workingDirectory, "assets") {
		t.Fatalf("Resolve() path = %q", result.Path)
	}
}

func TestResolveDoesNotSearchAboveExecutableDirectory(t *testing.T) {
	installationRoot := t.TempDir()
	executableDirectory := filepath.Join(installationRoot, "bin")
	if err := os.Mkdir(executableDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, installationRoot, filepath.Join("share", "asset.yaml"))
	workingDirectory := t.TempDir()

	result, err := Resolve(filepath.Join("share", "asset.yaml"), Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Source != SourceWorkingDirectory || result.Path != filepath.Join(workingDirectory, "share", "asset.yaml") || result.Exists {
		t.Fatalf("Resolve() = %#v, want missing path under working directory", result)
	}
}

func TestResolveErrorReportsRejectedCandidates(t *testing.T) {
	processDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(processDirectory, "missing.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve("missing.yaml", Options{
		ProcessDir:       processDirectory,
		WorkingDirectory: t.TempDir(),
		ExecutablePath:   filepath.Join(t.TempDir(), "bin", "app"),
		AdditionalBases:  []Candidate{{Path: filepath.Join(t.TempDir(), "missing")}},
		Kind:             KindFile,
	})
	var resolveError *ResolveError
	if !errors.As(err, &resolveError) {
		t.Fatalf("Resolve() error = %v, want *ResolveError", err)
	}
	if len(resolveError.Failures) != 4 {
		t.Fatalf("failure count = %d, want 4", len(resolveError.Failures))
	}
	if !strings.Contains(err.Error(), "path is not a regular file") {
		t.Fatalf("Resolve() error = %q, want candidate reasons", err)
	}
	if resolveError.Failures[3].Source != SourceFallback {
		t.Fatalf("last failure source = %q, want %q", resolveError.Failures[3].Source, SourceFallback)
	}
}

func TestResolveRejectsInvalidArguments(t *testing.T) {
	if _, err := Resolve("", Options{}); err == nil {
		t.Fatal("Resolve() accepted an empty path")
	}
	if _, err := Resolve("asset", Options{Kind: Kind(99)}); err == nil {
		t.Fatal("Resolve() accepted an invalid path kind")
	}
}

func TestResolveReturnsFirstValidBaseWhenNoTargetExists(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()

	result, err := Resolve("new/config.yaml", Options{
		ProcessDir:       home,
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != filepath.Join(home, "new", "config.yaml") || result.Base != home || result.Source != SourceProcessDir {
		t.Fatalf("Resolve() = %#v, want missing path under process directory", result)
	}
	if result.Exists {
		t.Fatal("Resolve() Exists = true, want false")
	}
}

func TestResolveFindsExistingLaterCandidateBeforeMissingFallback(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	target := writeFile(t, workingDirectory, "config.yaml")

	result, err := Resolve("config.yaml", Options{
		ProcessDir:       home,
		WorkingDirectory: workingDirectory,
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceWorkingDirectory || !result.Exists {
		t.Fatalf("Resolve() = %#v, want existing working-directory target", result)
	}
}

func TestResolveAbsoluteMissingPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "not-created.yaml")
	result, err := Resolve(target, Options{Kind: KindFile})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceAbsolute || result.Base != "" || result.Exists {
		t.Fatalf("Resolve() = %#v, want absent absolute path", result)
	}
}

func TestResolveDoesNotHideExistingWrongKind(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve("config.yaml", Options{
		ProcessDir:       home,
		WorkingDirectory: workingDirectory,
		Kind:             KindFile,
	})
	var resolveError *ResolveError
	if !errors.As(err, &resolveError) {
		t.Fatalf("Resolve() error = %v, want *ResolveError", err)
	}
}

func TestResolveErrorsWhenEveryBaseIsInvalid(t *testing.T) {
	baseFile := writeFile(t, t.TempDir(), "base-file")
	workingDirectory := filepath.Join(t.TempDir(), "missing-working-directory")
	executablePath := filepath.Join(t.TempDir(), "missing-bin", "app")

	_, err := Resolve("config.yaml", Options{
		ProcessDir:       baseFile,
		WorkingDirectory: workingDirectory,
		ExecutablePath:   executablePath,
		Kind:             KindFile,
	})
	var resolveError *ResolveError
	if !errors.As(err, &resolveError) {
		t.Fatalf("Resolve() error = %v, want *ResolveError", err)
	}
}

func TestResolveErrorNilReceiver(t *testing.T) {
	var resolveError *ResolveError
	if got := resolveError.Error(); got != "path resolution failed" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestAbsolutePathRequiresWorkingDirectoryForRelativeInput(t *testing.T) {
	if _, err := absolutePath("relative/path", ""); err == nil {
		t.Fatal("absolutePath() accepted a relative path without a working directory")
	}
}

func TestResolveExecutableDirectoryWithRelativeOverride(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	executableDirectory := t.TempDir()
	relativeExecutable, err := filepath.Rel(workingDirectory, filepath.Join(executableDirectory, "app"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveExecutableDirectory(relativeExecutable, workingDirectory)
	if err != nil {
		t.Fatalf("resolveExecutableDirectory() error = %v", err)
	}
	if got != executableDirectory {
		t.Fatalf("resolveExecutableDirectory() = %q, want %q", got, executableDirectory)
	}
}

func TestFileSystemReasonPreservesNonNotExistErrors(t *testing.T) {
	err := &os.PathError{Op: "stat", Path: "asset", Err: os.ErrPermission}
	if got := fileSystemReason(err); got != err.Error() {
		t.Fatalf("fileSystemReason() = %q, want %q", got, err.Error())
	}
}

func createTarget(t *testing.T, base, name string, kind Kind) {
	t.Helper()
	path := filepath.Join(base, name)
	if kind == KindFile {
		writeFile(t, base, name)
		return
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
