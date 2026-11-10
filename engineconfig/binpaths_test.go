package engineconfig

import (
	"runtime"
	"strings"
	"testing"
)

func TestForSelectsConfiguredOSArchitecture(t *testing.T) {
	paths := BinaryPaths{
		"windows_amd64": "win",
		"linux_amd64":   "linux",
		"darwin_amd64":  "darwin",
		"freebsd_arm64": "future",
	}
	tests := []struct {
		goos, goarch, wantPath, wantKey string
	}{
		{"windows", "amd64", "win", "windows_amd64"},
		{" Windows ", "AMD64", "win", "windows_amd64"},
		{"linux", "amd64", "linux", "linux_amd64"},
		{"darwin", "amd64", "darwin", "darwin_amd64"},
		{"freebsd", "arm64", "future", "freebsd_arm64"},
		{"linux", "arm64", "", "linux_arm64"},
	}
	for _, test := range tests {
		gotPath, gotKey := paths.For(test.goos, test.goarch)
		if gotPath != test.wantPath || gotKey != test.wantKey {
			t.Errorf("For(%q, %q) = (%q, %q), want (%q, %q)", test.goos, test.goarch, gotPath, gotKey, test.wantPath, test.wantKey)
		}
	}
}

func TestCurrentSelectsRuntimePlatform(t *testing.T) {
	key := runtime.GOOS + "_" + runtime.GOARCH
	path, gotKey := BinaryPaths{key: "current-binary"}.Current()
	if path != "current-binary" || gotKey != key {
		t.Fatalf("Current() = (%q, %q), want (%q, %q)", path, gotKey, "current-binary", key)
	}
}

func TestPathsNormalizeTrimsValuesAndPreservesFutureKeys(t *testing.T) {
	paths := BinaryPaths{"windows_amd64": "  ./nuclei.exe  ", "freebsd_arm64": " ./future "}
	normalized := paths.Normalize()
	if normalized["windows_amd64"] != "./nuclei.exe" || normalized["freebsd_arm64"] != "./future" {
		t.Fatalf("Normalize() = %#v", normalized)
	}
	if paths["windows_amd64"] != "  ./nuclei.exe  " {
		t.Fatalf("Normalize() mutated original paths: %#v", paths)
	}
}

func TestBinaryPathsValidateGenericPlatformKeys(t *testing.T) {
	if err := (BinaryPaths{"windows_amd64": "win", "freebsd_arm64": "future"}).Validate(); err != nil {
		t.Fatalf("Validate(valid platforms) error = %v", err)
	}
	err := (BinaryPaths{"Windows_AMD64": "win", "linux-amd64": "linux", "_arm64": "bad", "a__c": "bad"}).Validate()
	if err == nil {
		t.Fatal("Validate(invalid platforms) error = nil")
	}
	for _, key := range []string{"binary_path.Windows_AMD64", "binary_path.linux-amd64", "binary_path._arm64", "binary_path.a__c"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("Validate() error %q does not identify %q", err, key)
		}
	}
}

func TestBinaryPathsValidateRejectsEmptyConfiguredPath(t *testing.T) {
	err := (BinaryPaths{"linux_amd64": "  "}).Validate()
	if err == nil || !strings.Contains(err.Error(), "binary_path.linux_amd64") || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("Validate() error = %v, want empty linux_amd64 path error", err)
	}
}
