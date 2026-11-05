package pathresolver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/winezer0/processkit/expand"
)

func TestResolveAbsolutePathDirectly(t *testing.T) {
	target := writeFile(t, t.TempDir(), "asset.yaml")
	result, err := Resolve(target, Options{Kind: KindFile})
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

func TestResolveAcceptsCallerExpandedPath(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PROCESSKIT_PATHRESOLVER_DIR", directory)
	expanded, err := expand.Expand("${PROCESSKIT_PATHRESOLVER_DIR}", nil, true, true)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	result, err := Resolve(expanded, Options{Kind: KindDirectory})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !result.Exists {
		t.Fatalf("Resolve() Exists = false, want true")
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
	result, err := Resolve(target, Options{Kind: KindFile})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceAbsolute || result.Base != "" || result.Exists {
		t.Fatalf("Resolve() = %#v, want missing absolute path without fallback", result)
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
	workingDirectory := t.TempDir()
	options := Options{WorkingDirectory: workingDirectory, Kind: KindFile}

	first, err := Resolve("asset.yaml", options)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	if first.Exists || first.Source != SourceWorkingDirectory {
		t.Fatalf("first Resolve() = %#v, want missing working-directory candidate", first)
	}
	target := writeFile(t, workingDirectory, "asset.yaml")

	second, err := Resolve("asset.yaml", options)
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if second.Source != SourceWorkingDirectory || second.Path != target || !second.Exists {
		t.Fatalf("second Resolve() = %#v, want newly created working-directory target", second)
	}
}

func TestResolveFallsBackWhenTargetKindDoesNotMatch(t *testing.T) {
	tests := []struct {
		name           string
		kind           Kind
		workingKind    Kind
		executableKind Kind
	}{
		{name: "directory required but working directory has file", kind: KindDirectory, workingKind: KindFile, executableKind: KindDirectory},
		{name: "file required but working directory has directory", kind: KindFile, workingKind: KindDirectory, executableKind: KindFile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workingDirectory := t.TempDir()
			executableDirectory := t.TempDir()
			createTarget(t, executableDirectory, "asset", test.executableKind)
			createTarget(t, workingDirectory, "asset", test.workingKind)

			result, err := Resolve("asset", Options{
				WorkingDirectory: workingDirectory,
				ExecutablePath:   filepath.Join(executableDirectory, "app"),
				Kind:             test.kind,
			})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if result.Source != SourceExecutableDirectory {
				t.Fatalf("Resolve() source = %q, want %q", result.Source, SourceExecutableDirectory)
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
	workingDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(workingDirectory, "missing.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Resolve("missing.yaml", Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(t.TempDir(), "bin", "app"),
		AdditionalBases:  []Candidate{{Path: filepath.Join(t.TempDir(), "missing")}},
		Kind:             KindFile,
	})
	var resolveError *ResolveError
	if !errors.As(err, &resolveError) {
		t.Fatalf("Resolve() error = %v, want *ResolveError", err)
	}
	if len(resolveError.Failures) != 3 {
		t.Fatalf("failure count = %d, want 3", len(resolveError.Failures))
	}
	if !strings.Contains(err.Error(), "path is not a regular file") {
		t.Fatalf("Resolve() error = %q, want candidate reasons", err)
	}
	lastFailure := resolveError.Failures[len(resolveError.Failures)-1]
	if lastFailure.Source != SourceFallback {
		t.Fatalf("last failure source = %q, want %q", lastFailure.Source, SourceFallback)
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
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()

	result, err := Resolve("new/config.yaml", Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != filepath.Join(workingDirectory, "new", "config.yaml") || result.Base != workingDirectory || result.Source != SourceWorkingDirectory {
		t.Fatalf("Resolve() = %#v, want missing path under working directory", result)
	}
	if result.Exists {
		t.Fatal("Resolve() Exists = true, want false")
	}
}

func TestResolveFindsExistingLaterCandidateBeforeMissingFallback(t *testing.T) {
	workingDirectory := t.TempDir()
	executableDirectory := t.TempDir()
	target := writeFile(t, executableDirectory, "config.yaml")

	result, err := Resolve("config.yaml", Options{
		WorkingDirectory: workingDirectory,
		ExecutablePath:   filepath.Join(executableDirectory, "app"),
		Kind:             KindFile,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.Path != target || result.Source != SourceExecutableDirectory || !result.Exists {
		t.Fatalf("Resolve() = %#v, want existing executable-directory target", result)
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
	workingDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(workingDirectory, "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve("config.yaml", Options{
		WorkingDirectory: workingDirectory,
		Kind:             KindFile,
	})
	var resolveError *ResolveError
	if !errors.As(err, &resolveError) {
		t.Fatalf("Resolve() error = %v, want *ResolveError", err)
	}
}

func TestResolveErrorsWhenEveryBaseIsInvalid(t *testing.T) {
	workingDirectory := filepath.Join(t.TempDir(), "missing-working-directory")
	executablePath := filepath.Join(t.TempDir(), "missing-bin", "app")

	_, err := Resolve("config.yaml", Options{
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
