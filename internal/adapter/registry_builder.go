package adapter

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// RegistryOptions configures how BuildRegistry constructs adapters.
type RegistryOptions struct {
	// BaseURL enables the http adapter when non-empty.
	BaseURL string
	// Adapters is the environment's per-adapter configuration block.
	Adapters map[string]map[string]any
	// ConfigDir anchors relative paths in adapter configuration.
	ConfigDir string
	// Compat selects adapter behaviour that changed between releases.
	Compat tryve.CompatMode
	// Warn receives diagnostics about unusable configuration. Optional.
	Warn io.Writer
}

// BuildRegistry constructs and registers every adapter the environment declares.
//
// This is the single place adapter names are mapped to constructors. It used to
// be duplicated across the run command, the health command, and the library API,
// so an adapter registered in one worked there and reported "not registered" in
// the others.
func BuildRegistry(opts RegistryOptions) *Registry {
	reg := NewRegistry()

	warn := func(format string, args ...any) {
		if opts.Warn != nil {
			fmt.Fprintf(opts.Warn, format, args...)
		}
	}

	// The http adapter is available whenever a base URL is configured.
	if opts.BaseURL != "" {
		reg.Register("http", NewHTTPAdapterWithCompat(opts.BaseURL, opts.Compat))
	}

	// The shell adapter is always available, configured from its block if present.
	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = "."
	}
	reg.Register("shell", NewShellAdapterFromConfig(opts.Adapters["shell"], configDir, opts.Compat))

	for name, cfg := range opts.Adapters {
		switch name {
		case "http", "shell":
			// Registered above; the block is already applied.
		case "postgresql":
			reg.Register(name, NewPostgreSQLAdapterWithCompat(cfg, opts.Compat))
		case "mongodb":
			reg.Register(name, NewMongoDBAdapterWithCompat(cfg, opts.Compat))
		case "redis":
			reg.Register(name, NewRedisAdapter(cfg))
		case "kafka":
			reg.Register(name, NewKafkaAdapterWithCompat(cfg, opts.Compat))
		case "eventhub":
			reg.Register(name, NewEventHubAdapter(cfg))
		default:
			warn("WARN  unknown adapter %q in config — skipping\n", name)
		}
	}

	return reg
}

// ConfigDirOf returns the directory holding the given config file, for anchoring
// relative adapter paths.
func ConfigDirOf(configPath string) string {
	if configPath == "" {
		return "."
	}
	return filepath.Dir(configPath)
}
