package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/liemle3893/go-tryve/internal/adapter"
	"github.com/liemle3893/go-tryve/internal/config"
	"github.com/liemle3893/go-tryve/internal/executor"
	"github.com/liemle3893/go-tryve/internal/loader"
	"github.com/liemle3893/go-tryve/internal/reporter"
	"github.com/liemle3893/go-tryve/internal/tryve"
	"github.com/liemle3893/go-tryve/internal/watcher"
)

// newRunCmd constructs the `run` sub-command which discovers, filters, and
// executes YAML test files with the configured adapters.
func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Discover and run YAML test files",
		RunE:  runCmdHandler,
	}

	cmd.Flags().StringP("test-dir", "d", "tests", "directory to search for test files")
	cmd.Flags().IntP("parallel", "p", 0, "number of tests to run in parallel (0 = use config default)")
	cmd.Flags().IntP("timeout", "t", 0, "per-test timeout in milliseconds (0 = use config default)")
	cmd.Flags().IntP("retries", "r", -1, "number of retries on failure (-1 = use config default)")
	cmd.Flags().Bool("bail", false, "stop after the first test failure")
	cmd.Flags().StringP("grep", "g", "", "filter tests by name (regexp or substring)")
	cmd.Flags().StringSlice("tag", nil, "filter tests by tag (can be repeated)")
	cmd.Flags().String("priority", "", "filter tests by priority (P0, P1, P2, P3)")
	cmd.Flags().Bool("dry-run", false, "print matching tests without executing them")
	cmd.Flags().Bool("skip-setup", false, "skip the setup phase for every test")
	cmd.Flags().Bool("skip-teardown", false, "skip the teardown phase for every test")
	cmd.Flags().StringSlice("reporter", nil, "additional reporter names (can be repeated)")
	cmd.Flags().StringP("output", "o", "", "output file path for file-based reporters")
	cmd.Flags().Bool("verbose", false, "enable verbose per-step output")
	cmd.Flags().Bool("debug", false, "show full request/response data for every step")
	cmd.Flags().Bool("watch", false, "re-run tests on file changes")
	cmd.Flags().Bool("failed-only", false, "re-run only tests that failed in the previous run")

	return cmd
}

// runCmdHandler implements the `run` command execution logic.
func runCmdHandler(cmd *cobra.Command, _ []string) error {
	// Set up cancellable context tied to OS interrupts.
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Read persistent flags (defined on root, inherited by all sub-commands).
	cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
	envName, _ := cmd.Root().PersistentFlags().GetString("env")

	// Read run-specific flags.
	testDir, _ := cmd.Flags().GetString("test-dir")
	parallel, _ := cmd.Flags().GetInt("parallel")
	timeout, _ := cmd.Flags().GetInt("timeout")
	retries, _ := cmd.Flags().GetInt("retries")
	bail, _ := cmd.Flags().GetBool("bail")
	grep, _ := cmd.Flags().GetString("grep")
	tags, _ := cmd.Flags().GetStringSlice("tag")
	priority, _ := cmd.Flags().GetString("priority")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipSetup, _ := cmd.Flags().GetBool("skip-setup")
	skipTeardown, _ := cmd.Flags().GetBool("skip-teardown")
	verbose, _ := cmd.Flags().GetBool("verbose")
	debug, _ := cmd.Flags().GetBool("debug")
	reporterTypes, _ := cmd.Flags().GetStringSlice("reporter")
	outputPath, _ := cmd.Flags().GetString("output")
	watchMode, _ := cmd.Flags().GetBool("watch")
	failedOnly, _ := cmd.Flags().GetBool("failed-only")

	// Load configuration.
	cfg, err := config.Load(cfgPath, envName)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply CLI overrides on top of config defaults.
	if parallel > 0 {
		cfg.Defaults.Parallel = parallel
	}
	if timeout > 0 {
		cfg.Defaults.Timeout = timeout
	}
	if retries >= 0 {
		cfg.Defaults.Retries = retries
	}

	// Use config testDir if CLI flag wasn't explicitly set.
	// testDir in config is relative to the config file, not CWD.
	if !cmd.Flags().Changed("test-dir") && cfg.TestDir != "" {
		testDir = filepath.Join(filepath.Dir(cfgPath), cfg.TestDir)
	}

	// Discover test files once; used by both dry-run and watch/run paths.
	paths, err := loader.Discover(testDir)
	if err != nil {
		return fmt.Errorf("discovering tests in %q: %w", testDir, err)
	}

	filterOpts := executor.FilterOptions{Tags: tags, Grep: grep, Priority: priority}
	hasFilter := len(tags) > 0 || grep != "" || priority != ""

	// --failed-only: load the names of tests that failed in the previous run.
	if failedOnly {
		names, err := loadFailedNames()
		if err != nil {
			return fmt.Errorf("reading failed-test list: %w", err)
		}
		if len(names) == 0 {
			fmt.Fprintln(os.Stderr, "No failed tests recorded from a previous run.")
			return nil
		}
		filterOpts.Names = names
		hasFilter = true
	}

	// loadAndFilter parses all discovered paths, filters FIRST (cheap name/tag
	// check), then validates only matching tests. This avoids expensive validation
	// of tests that won't be run.
	loadAndFilter := func() []*tryve.TestDefinition {
		// Phase 1: parse all files (fast — just YAML unmarshal).
		var parsed []*tryve.TestDefinition
		for _, p := range paths {
			td, parseErr := loader.ParseFile(p)
			if parseErr != nil {
				if !hasFilter {
					fmt.Fprintf(os.Stderr, "WARN  parse error %s: %v\n", p, parseErr)
				}
				continue
			}
			parsed = append(parsed, td)
		}

		// Phase 2: filter by tag/grep/priority BEFORE validation.
		candidates := parsed
		if hasFilter {
			candidates = executor.FilterTests(parsed, filterOpts)
		}

		// Phase 3: validate only the tests that will actually run.
		var valid []*tryve.TestDefinition
		for _, td := range candidates {
			if errs := loader.Validate(td); len(errs) > 0 {
				for _, ve := range errs {
					fmt.Fprintf(os.Stderr, "WARN  validation error %s: %v\n", td.SourceFile, ve)
				}
				continue
			}
			valid = append(valid, td)
		}
		return valid
	}

	// Dry-run: print matching tests and exit without running them.
	if dryRun {
		filtered := loadAndFilter()
		for _, td := range filtered {
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s (%s)\n", td.Priority, td.Name, td.SourceFile)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\n%d test(s) matched\n", len(filtered))
		return nil
	}

	// Build reporter pipeline: console is always included; additional reporters
	// are appended based on the --reporter flag values.
	reporters := []reporter.Reporter{reporter.NewConsoleFromEnvWithDebug(verbose, debug)}
	for _, rType := range reporterTypes {
		switch rType {
		case "junit":
			reporters = append(reporters, reporter.NewJUnit(outputPath))
		case "html":
			reporters = append(reporters, reporter.NewHTML(outputPath))
		case "json":
			reporters = append(reporters, reporter.NewJSON(outputPath))
		default:
			fmt.Fprintf(os.Stderr, "WARN  unknown reporter %q — skipping\n", rType)
		}
	}
	rep := reporter.NewMulti(reporters...)

	// Build adapter registry.
	reg := adapter.NewRegistry()

	// HTTP adapter: available when baseURL is configured.
	if cfg.Environment.BaseURL != "" {
		reg.Register("http", adapter.NewHTTPAdapter(cfg.Environment.BaseURL))
	}

	// Shell adapter is always available.
	reg.Register("shell", adapter.NewShellAdapter(&adapter.ShellConfig{}))

	// Process adapter is always available.
	reg.Register("process", adapter.NewProcessAdapter())

	// Register adapters from the environment config block.
	for name, adapterCfg := range cfg.Environment.Adapters {
		switch name {
		case "http", "shell":
			// Already registered above; skip duplicate.
		case "postgresql":
			reg.Register("postgresql", adapter.NewPostgreSQLAdapter(adapterCfg))
		case "mongodb":
			reg.Register("mongodb", adapter.NewMongoDBAdapter(adapterCfg))
		case "redis":
			reg.Register("redis", adapter.NewRedisAdapter(adapterCfg))
		case "kafka":
			reg.Register("kafka", adapter.NewKafkaAdapter(adapterCfg))
		case "eventhub":
			reg.Register("eventhub", adapter.NewEventHubAdapter(adapterCfg))
		default:
			fmt.Fprintf(os.Stderr, "WARN  unknown adapter %q in config — skipping\n", name)
		}
	}
	defer reg.CloseAll(ctx)

	// Pre-warm: connect required adapters in parallel before the first test.
	// This avoids paying connection latency during step execution.
	// Connection failures are reported as warnings so the user sees them up front.
	if !dryRun {
		filtered := loadAndFilter()
		needed := map[string]bool{}
		for _, td := range filtered {
			for _, phases := range [][]tryve.StepDefinition{td.Setup, td.Execute, td.Verify, td.Teardown} {
				for _, s := range phases {
					needed[s.Adapter] = true
				}
			}
		}
		type connResult struct {
			name string
			err  error
		}
		results := make(chan connResult, len(needed))
		var wg sync.WaitGroup
		for name := range needed {
			if reg.Has(name) {
				wg.Add(1)
				go func(n string) {
					defer wg.Done()
					_, err := reg.Get(ctx, n)
					if err != nil {
						results <- connResult{name: n, err: err}
					}
				}(name)
			}
		}
		wg.Wait()
		close(results)
		for r := range results {
			fmt.Fprintf(os.Stderr, "WARN  adapter %q: %v\n", r.name, r.err)
		}
	}

	// runOnce executes the full filtered test suite and returns the suite result.
	runOnce := func() *tryve.SuiteResult {
		runFiltered := loadAndFilter()
		if len(runFiltered) == 0 {
			fmt.Fprintln(os.Stderr, "No tests matched the current filters.")
			return &tryve.SuiteResult{}
		}

		if skipSetup {
			for _, td := range runFiltered {
				td.Setup = nil
			}
		}
		if skipTeardown {
			for _, td := range runFiltered {
				td.Teardown = nil
			}
		}

		orch := executor.NewOrchestrator(reg, rep, cfg)
		orch.SetBail(bail)
		return orch.Run(ctx, runFiltered)
	}

	if !watchMode {
		result := runOnce()
		_ = saveFailedNames(result)
		if result.Failed > 0 {
			os.Exit(1)
		}
		return nil
	}

	// Watch mode: run tests initially, then re-run on file changes.
	fmt.Fprintln(cmd.OutOrStdout(), "Watch mode enabled — press Ctrl+C to stop.")
	result := runOnce()
	_ = saveFailedNames(result)

	w, err := watcher.New([]string{testDir}, func() {
		fmt.Fprintf(cmd.OutOrStdout(), "\nFile change detected, re-running tests...\n\n")
		r := runOnce()
		_ = saveFailedNames(r)
	})
	if err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}
	// Start blocks until ctx is cancelled (Ctrl+C).
	return w.Start(ctx)
}

// failedNamesFile is the path where failed test names are persisted between runs.
const failedNamesFile = ".tryve-failed"

// saveFailedNames writes the names of failed tests in result to failedNamesFile,
// one name per line. If no tests failed, the file is removed so --failed-only
// does not accidentally rerun stale results.
func saveFailedNames(result *tryve.SuiteResult) error {
	if result == nil || result.Failed == 0 {
		_ = os.Remove(failedNamesFile)
		return nil
	}
	f, err := os.Create(failedNamesFile)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, r := range result.Tests {
		if r.Status == tryve.StatusFailed && r.Test != nil {
			fmt.Fprintln(w, r.Test.Name)
		}
	}
	return w.Flush()
}

// loadFailedNames reads failedNamesFile and returns a set of test names.
// It returns an error only if the file exists but cannot be read.
// A missing file is treated as "no prior failures".
func loadFailedNames() (map[string]struct{}, error) {
	f, err := os.Open(failedNamesFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if name := strings.TrimSpace(scanner.Text()); name != "" {
			names[name] = struct{}{}
		}
	}
	return names, scanner.Err()
}
