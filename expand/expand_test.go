package expand

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpand(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := os.TempDir()
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
			name:      "expands adjacent and repeated variables",
			input:     "${A}${B}/${A}",
			variables: map[string]string{"A": "left", "B": "right"},
			want:      "leftright/left",
		},
		{
			name:      "expands built-in and custom variables",
			input:     "${HOME_DIR}|${TEMP_DIR}|${USER_CONFIG_DIR}|${USER_CACHE_DIR}|${APP_DIR}",
			variables: map[string]string{"APP_DIR": "/srv/tanscan"},
			want:      strings.Join([]string{homeDir, tempDir, userConfigDir, userCacheDir, "/srv/tanscan"}, "|"),
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
			got, err := Expand(test.input, test.variables, false)
			if err != nil {
				t.Fatalf("Expand() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Expand() = %q, want %q", got, test.want)
			}
		})
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
			got, err := Expand(test.input, test.variables, test.expandEnv)
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

func TestExpandPathHomePrefix(t *testing.T) {
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
			got, err := ExpandPath(test.input, nil, false)
			if err != nil {
				t.Fatalf("ExpandPath() error = %v", err)
			}
			if got != test.want {
				t.Errorf("ExpandPath() = %q, want %q", got, test.want)
			}
		})
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
			name:      "unterminated expression",
			input:     "${APP_DIR",
			wantError: "unterminated variable expression",
		},
		{
			name:      "empty variable name",
			input:     "${}",
			wantError: "invalid variable name",
		},
		{
			name:      "lowercase variable name",
			input:     "${app_dir}",
			wantError: "invalid variable name",
		},
		{
			name:      "invalid character in variable name",
			input:     "${APP-DIR}",
			wantError: "invalid variable name",
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
			variables: map[string]string{"app_dir": "/srv/tanscan"},
			wantError: `invalid variable name "app_dir"`,
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Expand(test.input, test.variables, false)
			if err == nil {
				t.Fatalf("Expand() error = nil, want error containing %q", test.wantError)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Errorf("Expand() error = %q, want substring %q", err, test.wantError)
			}
		})
	}
}
