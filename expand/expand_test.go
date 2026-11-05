package expand

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	homeDirAbs, err := filepath.Abs(homeDir)
	if err != nil {
		t.Fatal(err)
	}
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	exePath, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := os.TempDir()
	tempDirAbs, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		input     string
		variables map[string]string
		expandHome bool
		want      string
	}{
		{
			name:  "leaves strings without expressions unchanged",
			input: "literal $HOME and %TEMP%",
			want:  "literal $HOME and %TEMP%",
		},
		{
			name:      "expands custom variables",
			input:     "${ENGINE_DIR}/katana",
			variables: map[string]string{"ENGINE_DIR": "/opt/scanners"},
			want:      "/opt/scanners/katana",
		},
		{
			name:      "trims whitespace around variable names",
			input:     "${ ENGINE_DIR }/katana",
			variables: map[string]string{"ENGINE_DIR": "/opt/scanners"},
			want:      "/opt/scanners/katana",
		},
		{
			name:      "expands home prefix and variables in one call",
			input:     "~/${ENGINE_DIR}/katana",
			variables: map[string]string{"ENGINE_DIR": "third"},
			expandHome: true,
			want:      filepath.Join(homeDirAbs, "third", "katana"),
		},
		{
			name:      "supports digits and underscores in variable names",
			input:     "${ENGINE_DIR_2}/katana",
			variables: map[string]string{"ENGINE_DIR_2": "/opt/scanners"},
			want:      "/opt/scanners/katana",
		},
		{
			name:      "expands adjacent and repeated variables",
			input:     "${A}${B}/${A}",
			variables: map[string]string{"A": "left", "B": "right"},
			want:      "leftright/left",
		},
		{
			name:      "expands built-in and custom variables",
			input:     "${HOME_DIR}|${HOME_CONFIG_DIR}|${TEMP_DIR}|${USER_CONFIG_DIR}|${USER_CACHE_DIR}|${NOW_WORK_DIR}|${NOW_EXE_PATH}|${NOW_EXE_DIR}|${APP_DIR}",
			variables: map[string]string{"APP_DIR": "/srv/tanscan"},
			want:      strings.Join([]string{homeDirAbs, filepath.Join(homeDirAbs, ".config"), tempDirAbs, userConfigDir, userCacheDir, workDir, exePath, filepath.Dir(exePath), "/srv/tanscan"}, "|"),
		},
		{
			name:      "inserts custom values literally without recursive expansion",
			input:     "${A}",
			variables: map[string]string{"A": "${B}", "B": "expanded"},
			want:      "${B}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Expand(test.input, test.variables, false, test.expandHome)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Expand() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExpandRelativeHomeDirectoryBecomesAbsolute(t *testing.T) {
	homeEnvName := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeEnvName = "USERPROFILE"
	case "plan9":
		homeEnvName = "home"
	}
	relativeHome := filepath.Join("relative", "home")
	t.Setenv(homeEnvName, relativeHome)
	wantHome, err := filepath.Abs(relativeHome)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Expand("${HOME_DIR}|${HOME_CONFIG_DIR}", nil, false, false)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	want := wantHome + "|" + filepath.Join(wantHome, ".config")
	if got != want {
		t.Errorf("Expand() = %q, want %q", got, want)
	}
}

func TestExpandRelativeTempDirectoryBecomesAbsolute(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("os.TempDir does not use TMPDIR on this platform")
	}
	relativeTemp := filepath.Join("relative", "temp")
	t.Setenv("TMPDIR", relativeTemp)
	want, err := filepath.Abs(relativeTemp)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Expand("${TEMP_DIR}", nil, false, false)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got != want {
		t.Errorf("Expand() = %q, want %q", got, want)
	}
}

func TestExpandEnvironmentOption(t *testing.T) {
	t.Setenv("PROCESSKIT_EXPAND_ENV_TEST", "environment")
	t.Setenv("PROCESSKIT_EXPAND_ENV_EMPTY_TEST", "")
	t.Setenv("PROCESSKIT_EXPAND_ENV_MISSING_TEST", "temporary")
	if err := os.Unsetenv("PROCESSKIT_EXPAND_ENV_MISSING_TEST"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}

	tests := []struct {
		name      string
		input     string
		variables map[string]string
		expandEnv bool
		want      string
		wantError string
	}{
		{
			name:  "environment expansion is disabled by default",
			input: "$PROCESSKIT_EXPAND_ENV_TEST",
			want:  "$PROCESSKIT_EXPAND_ENV_TEST",
		},
		{
			name:      "disabled environment expansion keeps strict unknown variable errors",
			input:     "${PROCESSKIT_EXPAND_ENV_TEST}",
			wantError: `unknown variable "PROCESSKIT_EXPAND_ENV_TEST"`,
		},
		{
			name:      "enabled environment expansion only supports braced names",
			input:     "${PROCESSKIT_EXPAND_ENV_TEST}|$PROCESSKIT_EXPAND_ENV_TEST",
			expandEnv: true,
			want:      "environment|$PROCESSKIT_EXPAND_ENV_TEST",
		},
		{
			name:      "caller variables take precedence over environment",
			input:     "${PROCESSKIT_EXPAND_ENV_TEST}",
			variables: map[string]string{"PROCESSKIT_EXPAND_ENV_TEST": "caller"},
			expandEnv: true,
			want:      "caller",
		},
		{
			name:      "unset or empty environment values expand to empty strings",
			input:     "before${PROCESSKIT_EXPAND_ENV_EMPTY_TEST}|${PROCESSKIT_EXPAND_ENV_MISSING_TEST}after",
			expandEnv: true,
			want:      "before|after",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Expand(test.input, test.variables, test.expandEnv, false)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Expand() error = %v, want substring %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Expand() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExpandHomePrefix(t *testing.T) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "slash prefix", input: "~/config", want: filepath.Join(homeDirectory, "config")},
		{name: "backslash prefix", input: "~\\config", want: filepath.Join(homeDirectory, "config")},
		{name: "bare tilde remains unchanged", input: "~", want: "~"},
		{name: "named user remains unchanged", input: "~other/config", want: "~other/config"},
		{name: "interior tilde remains unchanged", input: "prefix~/config", want: "prefix~/config"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Expand(test.input, nil, false, true)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Expand() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExpandPreservesUnmatchedExpressions(t *testing.T) {
	for _, input := range []string{"${NAME WITH SPACE}", "${NAME"} {
		got, err := Expand(input, nil, false, false)
		if err != nil {
			t.Fatalf("Expand(%q) error = %v", input, err)
		}
		if got != input {
			t.Errorf("Expand(%q) = %q, want original text", input, got)
		}
	}
}

func TestExpandHomeOptionDisabled(t *testing.T) {
	got, err := Expand("~/configs/app.yaml", nil, false, false)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	if got != "~/configs/app.yaml" {
		t.Fatalf("Expand() = %q, want unchanged tilde path", got)
	}
}

func TestExpandErrors(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		variables map[string]string
		wantError string
	}{
		{
			name:      "unknown variable",
			input:     "${UNKNOWN}",
			wantError: `unknown variable "UNKNOWN"`,
		},
		{
			name:      "used custom variable has empty value",
			input:     "${APP_DIR}",
			variables: map[string]string{"APP_DIR": ""},
			wantError: `variable "APP_DIR" has an empty value`,
		},
		{
			name:      "rejects invalid unused custom variable",
			input:     "plain text",
			variables: map[string]string{"APP-DIR": "/srv/tanscan"},
			wantError: `invalid variable name "APP-DIR"`,
		},
		{
			name:      "rejects empty unused custom variable",
			input:     "plain text",
			variables: map[string]string{"APP_DIR": ""},
			wantError: `variable "APP_DIR" has an empty value`,
		},
		{
			name:      "rejects overriding built-in variable",
			input:     "${HOME_DIR}",
			variables: map[string]string{"HOME_DIR": "/tmp/override"},
			wantError: `built-in variable "HOME_DIR" cannot be overridden`,
		},
		{
			name:      "rejects overriding new built-in variables",
			input:     "${HOME_CONFIG_DIR}",
			variables: map[string]string{"HOME_CONFIG_DIR": "/tmp/override"},
			wantError: `built-in variable "HOME_CONFIG_DIR" cannot be overridden`,
		},
		{
			name:      "rejects overriding runtime built-in variables",
			input:     "${NOW_WORK_DIR}",
			variables: map[string]string{"NOW_WORK_DIR": "/tmp/override"},
			wantError: `built-in variable "NOW_WORK_DIR" cannot be overridden`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Expand(test.input, test.variables, false, false)
			if err == nil {
				t.Fatalf("Expand() error = nil, want error containing %q", test.wantError)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("Expand() error = %q, want substring %q", err, test.wantError)
			}
		})
	}
}
