package engineconfig

import (
	"errors"
	"fmt"
	"strings"
)

// MaximumEngineCheckTimeoutSeconds bounds engine readiness checks.
const MaximumEngineCheckTimeoutSeconds = 300

// DefaultEngineCheckTimeoutSeconds is the default readiness timeout for an engine.
const DefaultEngineCheckTimeoutSeconds = 30

// Runtime stores resolved or inherited values that are not part of YAML configuration.
type Runtime struct {
	BinaryPath     string
	PluginsDir     string
	ProxyInherited bool
}

// EngineFieldError identifies the common engine setting that failed validation.
type EngineFieldError struct {
	Field string
	Err   error
}

// Error returns the field and validation message.
func (validationError *EngineFieldError) Error() string {
	return fmt.Sprintf("%s: %v", validationError.Field, validationError.Err)
}

// Unwrap returns the underlying validation error.
func (validationError *EngineFieldError) Unwrap() error {
	return validationError.Err
}

// PrefixEngineFieldErrors adds a configuration path to every structured field error.
func PrefixEngineFieldErrors(err error, prefix string) error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		prefixed := make([]error, 0, len(causes))
		for _, cause := range causes {
			prefixed = append(prefixed, PrefixEngineFieldErrors(cause, prefix))
		}
		return errors.Join(prefixed...)
	}
	var fieldError *EngineFieldError
	if errors.As(err, &fieldError) {
		field := strings.Trim(strings.Join([]string{prefix, fieldError.Field}, "."), ".")
		return &EngineFieldError{Field: field, Err: fieldError.Err}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// EngineConfig contains common settings for any internal or external tool registered as an engine.
type EngineConfig struct {
	Enabled             *bool             `yaml:"enabled"`
	CheckTimeoutSeconds int               `yaml:"check_timeout_seconds"`
	BinaryPaths         BinaryPaths       `yaml:"binary_path"`
	ConfigPath          string            `yaml:"config_path"`
	CAFile              string            `yaml:"ca_file"`
	PluginDirs          PathEntries       `yaml:"plugins_dir"`
	Environment         map[string]string `yaml:"environment" json:"-"`
	Others              map[string]any    `yaml:"others"`
	WorkingDirectory    string            `yaml:"working_directory"`
	ProxyURL            string            `yaml:"proxy_url"`
	Runtime             `yaml:"-" json:"-"`
}

// Normalize applies shared defaults and canonicalizes common engine settings.
func (config EngineConfig) Normalize() EngineConfig {
	if config.CheckTimeoutSeconds == 0 {
		config.CheckTimeoutSeconds = DefaultEngineCheckTimeoutSeconds
	}
	config.BinaryPaths = config.BinaryPaths.Normalize()
	config.ConfigPath = strings.TrimSpace(config.ConfigPath)
	config.CAFile = strings.TrimSpace(config.CAFile)
	config.Runtime.BinaryPath = strings.TrimSpace(config.Runtime.BinaryPath)
	config.Runtime.PluginsDir = strings.TrimSpace(config.Runtime.PluginsDir)
	config.PluginDirs = NormalizePathEntries(config.PluginDirs, config.Runtime.PluginsDir)
	config.Runtime.PluginsDir = firstPluginPath(config.PluginDirs)
	config.WorkingDirectory = strings.TrimSpace(config.WorkingDirectory)
	config.ProxyURL = strings.TrimSpace(config.ProxyURL)
	return config
}

func firstPluginPath(entries PathEntries) string {
	if len(entries) == 0 {
		return ""
	}
	return entries[0].Path
}

// EnabledByDefault reports the configured startup state, defaulting omitted values to enabled.
func (config EngineConfig) EnabledByDefault() bool {
	return config.Enabled == nil || *config.Enabled
}

// Validate checks the common configuration structure without accessing the filesystem.
func (config EngineConfig) Validate() error {
	var validationErrors []error
	if config.CheckTimeoutSeconds != 0 && (config.CheckTimeoutSeconds < 1 || config.CheckTimeoutSeconds > MaximumEngineCheckTimeoutSeconds) {
		validationErrors = append(validationErrors, &EngineFieldError{
			Field: "check_timeout_seconds",
			Err:   fmt.Errorf("must be between 1 and %d", MaximumEngineCheckTimeoutSeconds),
		})
	}
	if err := config.BinaryPaths.Validate(); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if err := config.PluginDirs.ValidateField("plugins_dir"); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if proxyURL := strings.TrimSpace(config.ProxyURL); proxyURL != "" {
		if err := validateURL(proxyURL); err != nil {
			validationErrors = append(validationErrors, &EngineFieldError{
				Field: "proxy_url",
				Err:   errors.New("must be an absolute http, https, or socks5 URL"),
			})
		}
	}
	return errors.Join(validationErrors...)
}
