package engineconfig

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEngineConfigNormalizeSharedFields(t *testing.T) {
	config := EngineConfig{
		BinaryPaths:      BinaryPaths{"linux_amd64": " ./nuclei "},
		ConfigPath:       " ./config.yaml ",
		CAFile:           " ./ca.crt ",
		PluginDirs:       PathEntries{{Path: " ./templates ", Required: true}},
		WorkingDirectory: " ./work ",
		ProxyURL:         " http://127.0.0.1:8080 ",
	}

	normalized := config.Normalize()
	if normalized.CheckTimeoutSeconds != DefaultEngineCheckTimeoutSeconds {
		t.Errorf("CheckTimeoutSeconds = %d, want %d", normalized.CheckTimeoutSeconds, DefaultEngineCheckTimeoutSeconds)
	}
	if normalized.BinaryPaths["linux_amd64"] != "./nuclei" || normalized.ConfigPath != "./config.yaml" || normalized.CAFile != "./ca.crt" || normalized.WorkingDirectory != "./work" || normalized.ProxyURL != "http://127.0.0.1:8080" {
		t.Errorf("Normalize() did not trim shared settings: %#v", normalized)
	}
	if len(normalized.PluginDirs) != 1 || normalized.PluginDirs[0].Path != "./templates" || normalized.PluginsDir != "./templates" {
		t.Errorf("Normalize() plugin paths = %#v (legacy %q)", normalized.PluginDirs, normalized.PluginsDir)
	}
	if config.BinaryPaths["linux_amd64"] != " ./nuclei " {
		t.Errorf("Normalize() mutated the source config: %#v", config.BinaryPaths)
	}
}

func TestEngineRuntimeIsExcludedFromYAML(t *testing.T) {
	encoded, err := yaml.Marshal(EngineConfig{Runtime: Runtime{BinaryPath: "resolved-nuclei", PluginsDir: "resolved-templates", ProxyInherited: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"resolved-nuclei", "resolved-templates", "ProxyInherited"} {
		if strings.Contains(string(encoded), field) {
			t.Errorf("YAML contains runtime-only value %q: %s", field, encoded)
		}
	}
}

func TestEngineConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  EngineConfig
		wantErr string
	}{
		{name: "empty config"},
		{
			name: "valid settings and paths are not checked for existence",
			config: EngineConfig{
				CheckTimeoutSeconds: 300,
				PluginDirs: PathEntries{
					{Path: "./missing-required", Required: true},
					{Path: "./missing-optional", Required: false},
				},
				ProxyURL: "socks5://127.0.0.1:1080",
			},
		},
		{name: "negative timeout", config: EngineConfig{CheckTimeoutSeconds: -1}, wantErr: "check_timeout_seconds"},
		{name: "timeout over maximum", config: EngineConfig{CheckTimeoutSeconds: 301}, wantErr: "check_timeout_seconds"},
		{name: "invalid proxy", config: EngineConfig{ProxyURL: "proxy.example:8080"}, wantErr: "proxy_url"},
		{name: "empty plugin entry", config: EngineConfig{PluginDirs: PathEntries{{Required: false}}}, wantErr: "plugins_dir"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestEngineConfigValidateAggregatesAndPrefixesFields(t *testing.T) {
	config := EngineConfig{
		CheckTimeoutSeconds: 301,
		PluginDirs:          PathEntries{{}, {}},
		ProxyURL:            "proxy.example:8080",
	}
	err := config.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	for _, field := range []string{"check_timeout_seconds", "plugins_dir[0].path", "plugins_dir[1].path", "proxy_url"} {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("Validate() error %q does not identify %s", err, field)
		}
	}

	prefixed := PrefixEngineFieldErrors(err, "services.scanner.engines.nuclei-pocs")
	for _, field := range []string{
		"services.scanner.engines.nuclei-pocs.check_timeout_seconds",
		"services.scanner.engines.nuclei-pocs.plugins_dir[0].path",
		"services.scanner.engines.nuclei-pocs.plugins_dir[1].path",
		"services.scanner.engines.nuclei-pocs.proxy_url",
	} {
		if !strings.Contains(prefixed.Error(), field) {
			t.Errorf("prefixed error %q does not identify %s", prefixed, field)
		}
	}
	var fieldError *EngineFieldError
	if !errors.As(prefixed, &fieldError) || !strings.HasPrefix(fieldError.Field, "services.scanner.engines.nuclei-pocs.") {
		t.Fatalf("errors.As() field error = %#v, want a fully-qualified field", fieldError)
	}
}
