package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/liemle3893/go-tryve/internal/tryve"
	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR_NAME} placeholders used throughout e2e.config.yaml.
var envVarPattern = regexp.MustCompile(`\$\{(\w+)\}`)

// Load reads the YAML file at path, resolves env vars, applies defaults,
// and returns the fully populated LoadedConfig for the requested environment.
//
// Resolution rules:
//   - baseUrl: STRICT — error if a referenced env var is not set.
//   - adapter configs and variables: NON-STRICT — unresolved placeholders are preserved.
func Load(path, envName string) (*LoadedConfig, error) {
	// Load .env file from the config file's directory (if it exists).
	// Only sets vars that aren't already in the environment.
	configDir := filepath.Dir(path)
	loadDotEnv(filepath.Join(configDir, ".env"))

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot read config file %q", path),
			"verify the file exists and is readable",
			err,
		)
	}

	var raw RawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot parse config file %q", path),
			"verify the YAML syntax is valid",
			err,
		)
	}

	if raw.Version != "1.0" {
		return nil, tryve.ConfigError(
			fmt.Sprintf("unsupported config version %q", raw.Version),
			`only version "1.0" is supported`,
			nil,
		)
	}

	env, ok := raw.Environments[envName]
	if !ok {
		hint := buildMissingEnvHint(envName, raw.Environments)
		return nil, tryve.ConfigError(
			fmt.Sprintf("environment %q not found in config", envName),
			hint,
			nil,
		)
	}

	resolvedBaseURL, err := resolveStrict(env.BaseURL)
	if err != nil {
		return nil, tryve.ConfigError(
			fmt.Sprintf("cannot resolve baseUrl for environment %q: %s", envName, err.Error()),
			"set the required environment variable before running e2e-runner",
			nil,
		)
	}
	env.BaseURL = resolvedBaseURL

	env.Adapters = resolveAdapterConfigs(env.Adapters)

	resolvedVars := resolveVariables(raw.Variables)

	defaults := applyDefaults(raw.Defaults)

	reporters := raw.Reporters
	if len(reporters) == 0 {
		reporters = []ReporterConfig{{Type: "console"}}
	}

	testDir := raw.TestDir
	if testDir == "" {
		testDir = "."
	}

	compat, err := tryve.ResolveLevel(raw.APIVersion, raw.Compatibility)
	if err != nil {
		return nil, tryve.ConfigError(
			err.Error(),
			"set apiVersion to \"tryve/v1\" or \"tryve/v2\" — see `tryve doc config`",
			nil,
		)
	}

	return &LoadedConfig{
		Raw:             raw,
		Environment:     env,
		EnvironmentName: envName,
		TestDir:         testDir,
		Compat:          compat,
		Defaults:        defaults,
		Variables:       resolvedVars,
		Hooks:           raw.Hooks,
		Reporters:       reporters,
	}, nil
}

// resolveStrict replaces all ${VAR_NAME} tokens in s with OS env var values.
// It returns an error if any referenced variable is not set.
func resolveStrict(s string) (string, error) {
	var missingVars []string

	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envVarPattern.FindStringSubmatch(match)[1]
		val, set := os.LookupEnv(name)
		if !set {
			missingVars = append(missingVars, name)
			return match
		}
		return val
	})

	if len(missingVars) > 0 {
		return "", fmt.Errorf("undefined environment variable(s): %s", strings.Join(missingVars, ", "))
	}
	return result, nil
}

// resolveNonStrict replaces ${VAR_NAME} tokens in s with OS env var values.
// Variables that are not set are left as the original placeholder.
func resolveNonStrict(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envVarPattern.FindStringSubmatch(match)[1]
		val, set := os.LookupEnv(name)
		if !set {
			return match
		}
		return val
	})
}

// resolveAdapterConfigs walks each adapter's config map and applies non-strict
// env var resolution to any string values found.
func resolveAdapterConfigs(adapters map[string]map[string]any) map[string]map[string]any {
	if adapters == nil {
		return nil
	}
	resolved := make(map[string]map[string]any, len(adapters))
	for adapterName, adapterCfg := range adapters {
		resolvedCfg := make(map[string]any, len(adapterCfg))
		for k, v := range adapterCfg {
			if str, ok := v.(string); ok {
				resolvedCfg[k] = resolveNonStrict(str)
			} else {
				resolvedCfg[k] = v
			}
		}
		resolved[adapterName] = resolvedCfg
	}
	return resolved
}

// resolveVariables applies non-strict env var resolution to all string values
// in the variables map.
func resolveVariables(vars map[string]any) map[string]any {
	if vars == nil {
		return nil
	}
	resolved := make(map[string]any, len(vars))
	for k, v := range vars {
		if str, ok := v.(string); ok {
			resolved[k] = resolveNonStrict(str)
		} else {
			resolved[k] = v
		}
	}
	return resolved
}

// applyDefaults merges user-supplied defaults with the builtin fallback values.
// Zero values in the user config are replaced by the builtin default.
func applyDefaults(d DefaultsConfig) DefaultsConfig {
	const (
		defaultTimeout    = 30000
		defaultRetryDelay = 1000
		defaultParallel   = 1
		defaultRetries    = 0
	)

	if d.Timeout == 0 {
		d.Timeout = defaultTimeout
	}
	if d.RetryDelay == 0 {
		d.RetryDelay = defaultRetryDelay
	}
	if d.Parallel == 0 {
		d.Parallel = defaultParallel
	}
	// Retries defaults to 0, so only set if the field was not explicitly supplied.
	// Since zero is the builtin default, no adjustment is needed.
	_ = defaultRetries
	return d
}

// loadDotEnv reads a .env file and sets any variables not already in the
// environment. Lines with # comments, empty lines, and export prefixes are handled.
// If the file doesn't exist, this is a no-op.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // file not found or not readable — silently skip
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional "export " prefix
		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Strip surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Only set if not already in environment (env takes precedence over .env)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

// buildMissingEnvHint constructs a human-readable hint listing the available
// environment names when the requested environment is not found.
func buildMissingEnvHint(requested string, envs map[string]EnvironmentConfig) string {
	names := make([]string, 0, len(envs))
	for name := range envs {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Sprintf(
		"environment %q does not exist; available environments: %s",
		requested,
		strings.Join(names, ", "),
	)
}
