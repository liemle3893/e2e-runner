// Package runner provides the public Go API for programmatic use of the tryve
// test runner. It mirrors the CLI command surface (run, validate, list, health)
// and returns structured results that callers can inspect without parsing text output.
package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/config"
	"github.com/liemle3893/go-tryve/internal/executor"
	"github.com/liemle3893/go-tryve/internal/loader"
	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// Options configures a test run. Zero values are intentional defaults:
// unset string fields are resolved from e2e.config.yaml, and zero int fields
// cause the config-level defaults to be used.
type Options struct {
	// ConfigPath is the path to e2e.config.yaml. Required.
	ConfigPath string
	// Environment selects which environment block to load from the config. Required.
	Environment string
	// TestDir is the directory to search for *.test.yaml files. Defaults to "tests".
	TestDir string
	// Tags filters tests to those with at least one matching tag.
	Tags []string
	// Grep filters tests by name (regexp or substring).
	Grep string
	// Priority filters tests by exact priority value (P0–P3).
	Priority string
	// Parallel overrides the config-level parallel setting. 0 uses the config default.
	Parallel int
	// Timeout overrides the per-test timeout in milliseconds. 0 uses the config default.
	Timeout int
	// Retries overrides the retry count. -1 uses the config default.
	Retries int
	// Bail stops scheduling new tests after the first failure.
	Bail bool
	// DryRun discovers and filters tests without executing them.
	// RunTests returns a SuiteResult with zero counts when DryRun is true.
	DryRun bool
	// Verbose enables per-step output on the console reporter.
	Verbose bool
	// Reporters lists additional reporter types to activate: "console", "junit", "html", "json".
	// A console reporter is always added unless Reporters contains exactly "none".
	Reporters []string
	// OutputPath is the file path for file-based reporters (junit, html, json).
	OutputPath string
}

// ValidationResult carries the parse and validation outcome for a single test file.
type ValidationResult struct {
	// File is the absolute path of the test file.
	File string
	// Valid is true when the file was parsed and passed all validation checks.
	Valid bool
	// Errors lists every parse or validation error found in the file.
	Errors []string
}

// HealthResult carries the connectivity check outcome for a single adapter.
type HealthResult struct {
	// Adapter is the registered adapter name (e.g. "http", "redis").
	Adapter string
	// OK is true when the adapter connected and passed its health check.
	OK bool
	// Error is the human-readable error message when OK is false; empty otherwise.
	Error string
}

// testDirOrDefault returns opts.TestDir when non-empty, otherwise "tests".
func testDirOrDefault(opts Options) string {
	if opts.TestDir != "" {
		return opts.TestDir
	}
	return "tests"
}

// buildRegistry constructs an adapter Registry from the loaded configuration.
// It registers the HTTP adapter when a baseURL is present, always registers the
// shell adapter, and registers any additional adapters declared in the config.
func buildRegistry(cfg *config.LoadedConfig) *adapter.Registry {
	return adapter.BuildRegistry(adapter.RegistryOptions{
		BaseURL:   cfg.Environment.BaseURL,
		Adapters:  cfg.Environment.Adapters,
		ConfigDir: ".",
		Compat:    cfg.Compat,
		Warn:      os.Stderr,
	})
}

// buildReporter constructs a Multi reporter from the options. A console reporter
// (writing to os.Stdout) is included by default. Additional reporters are appended
// when named in opts.Reporters; unknown names are silently skipped.
func buildReporter(opts Options) reporter.Reporter {
	reporters := []reporter.Reporter{reporter.NewConsoleFromEnv(opts.Verbose)}

	for _, rType := range opts.Reporters {
		switch rType {
		case "junit":
			reporters = append(reporters, reporter.NewJUnit(opts.OutputPath))
		case "html":
			reporters = append(reporters, reporter.NewHTML(opts.OutputPath))
		case "json":
			reporters = append(reporters, reporter.NewJSON(opts.OutputPath))
		}
	}

	return reporter.NewMulti(reporters...)
}

// discoverAndLoad discovers test files under dir, parses every file, skips files
// with parse or validation errors (printing a warning to stderr is handled by the
// caller if needed), and returns the successfully loaded definitions.
func discoverAndLoad(dir string) ([]*tryve.TestDefinition, map[string][]string, error) {
	paths, err := loader.Discover(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("discovering tests in %q: %w", dir, err)
	}

	var tests []*tryve.TestDefinition
	fileErrors := make(map[string][]string)

	for _, p := range paths {
		td, parseErr := loader.ParseFile(p)
		if parseErr != nil {
			fileErrors[p] = []string{parseErr.Error()}
			continue
		}
		if errs := loader.Validate(td); len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, e := range errs {
				msgs[i] = e.Error()
			}
			fileErrors[p] = msgs
			continue
		}
		tests = append(tests, td)
	}

	return tests, fileErrors, nil
}

// applyConfigOverrides merges CLI-level options into the loaded configuration,
// following the same precedence rules as the CLI run command.
func applyConfigOverrides(cfg *config.LoadedConfig, opts Options) {
	if opts.Parallel > 0 {
		cfg.Defaults.Parallel = opts.Parallel
	}
	if opts.Timeout > 0 {
		cfg.Defaults.Timeout = opts.Timeout
	}
	if opts.Retries >= 0 {
		cfg.Defaults.Retries = opts.Retries
	}
}

// RunTests executes the test suite described by opts and returns the aggregated
// SuiteResult. It follows the same flow as the CLI `run` command:
//
//  1. Load configuration from opts.ConfigPath / opts.Environment.
//  2. Discover and parse test files under opts.TestDir.
//  3. Apply filter options (tags, grep, priority).
//  4. Build adapter registry and reporter pipeline.
//  5. Run the orchestrator and return the result.
//
// When opts.DryRun is true the tests are discovered and filtered but not
// executed; a SuiteResult with zero counts is returned.
func RunTests(ctx context.Context, opts Options) (*tryve.SuiteResult, error) {
	cfg, err := config.Load(opts.ConfigPath, opts.Environment)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	applyConfigOverrides(cfg, opts)

	testDir := testDirOrDefault(opts)
	tests, _, err := discoverAndLoad(testDir)
	if err != nil {
		return nil, err
	}

	filtered := executor.FilterTests(tests, executor.FilterOptions{
		Tags:     opts.Tags,
		Grep:     opts.Grep,
		Priority: opts.Priority,
	})

	if opts.DryRun {
		return &tryve.SuiteResult{Total: len(filtered)}, nil
	}

	reg := buildRegistry(cfg)
	defer reg.CloseAll(ctx)

	rep := buildReporter(opts)

	orch := executor.NewOrchestrator(reg, rep, cfg)
	orch.SetBail(opts.Bail)

	return orch.Run(ctx, filtered), nil
}

// ValidateTests discovers test files under opts.TestDir, parses each file, and
// runs structural validation. It returns one ValidationResult per file regardless
// of whether the file is valid or not.
func ValidateTests(opts Options) ([]ValidationResult, error) {
	testDir := testDirOrDefault(opts)

	paths, err := loader.Discover(testDir)
	if err != nil {
		return nil, fmt.Errorf("discovering tests in %q: %w", testDir, err)
	}

	results := make([]ValidationResult, 0, len(paths))

	for _, p := range paths {
		vr := ValidationResult{File: p}

		td, parseErr := loader.ParseFile(p)
		if parseErr != nil {
			vr.Valid = false
			vr.Errors = []string{parseErr.Error()}
			results = append(results, vr)
			continue
		}

		errs := loader.Validate(td)
		if len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, e := range errs {
				msgs[i] = e.Error()
			}
			vr.Valid = false
			vr.Errors = msgs
		} else {
			vr.Valid = true
		}

		results = append(results, vr)
	}

	return results, nil
}

// ListTests discovers, parses, and filters test files under opts.TestDir.
// It returns the definitions that survived all filter criteria. Files with
// parse or validation errors are silently skipped.
func ListTests(opts Options) ([]*tryve.TestDefinition, error) {
	testDir := testDirOrDefault(opts)

	tests, _, err := discoverAndLoad(testDir)
	if err != nil {
		return nil, err
	}

	filtered := executor.FilterTests(tests, executor.FilterOptions{
		Tags:     opts.Tags,
		Grep:     opts.Grep,
		Priority: opts.Priority,
	})

	return filtered, nil
}

// CheckHealth loads the configuration, builds the adapter registry, and attempts
// to connect to and health-check every registered adapter. It returns one
// HealthResult per adapter; errors do not abort the loop so that all adapters are
// checked even when some fail.
func CheckHealth(ctx context.Context, opts Options) ([]HealthResult, error) {
	cfg, err := config.Load(opts.ConfigPath, opts.Environment)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	reg := buildRegistry(cfg)
	defer reg.CloseAll(ctx)

	names := reg.Names()
	results := make([]HealthResult, 0, len(names))

	for _, name := range names {
		hr := HealthResult{Adapter: name}

		a, getErr := reg.Get(ctx, name)
		if getErr != nil {
			hr.OK = false
			hr.Error = getErr.Error()
			results = append(results, hr)
			continue
		}

		if hErr := a.Health(ctx); hErr != nil {
			hr.OK = false
			hr.Error = hErr.Error()
		} else {
			hr.OK = true
		}

		results = append(results, hr)
	}

	return results, nil
}
