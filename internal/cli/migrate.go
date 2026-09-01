package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/liemle3893/go-tryve/internal/config"
	"github.com/liemle3893/go-tryve/internal/loader"
	"github.com/liemle3893/go-tryve/internal/migrate"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// newMigrateCmd constructs the `migrate` sub-command, which moves a suite between
// compatibility levels without requiring every file to be reviewed at once.
func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate [path...]",
		Short: "Move a suite to a new compatibility level",
		Long: `Move a suite to a new compatibility level.

Reports what would behave differently, and — with --apply — raises the suite's
level while pinning the affected files to their current one, so the suite keeps
passing exactly as it does today. Each pinned file can then be worked through
and unpinned individually, which is the only workable order for a suite of more
than a handful of files.

Nothing is written without --apply.

Examples:
  tryve migrate                              what would change, everywhere
  tryve migrate --area assertions            just that area
  tryve migrate --explain tests/e2e/x.yaml   every difference in one file
  tryve migrate --apply                      raise the suite, pin what breaks
  tryve migrate --status                     how many files remain pinned
  tryve migrate --unpin tests/e2e/x.yaml     clear a pin once the file is fixed`,
		RunE: migrateCmdHandler,
	}

	cmd.Flags().StringP("test-dir", "d", "", "directory to search for test files")
	cmd.Flags().String("to", tryve.APIVersionV2, "target apiVersion")
	cmd.Flags().StringSlice("area", nil,
		"limit to these areas: assertions, interpolation, execution, adapters (default: all)")
	cmd.Flags().Bool("apply", false, "write the changes: raise the config and pin affected files")
	cmd.Flags().Bool("status", false, "report how many files are pinned below the suite's level")
	cmd.Flags().Bool("explain", false, "list every difference in the named files, one per line")
	cmd.Flags().Bool("unpin", false, "remove the pin from the named files")
	cmd.Flags().Bool("only-certain", false,
		"pin only files that provably change, leaving the uncertain ones to run at the new level")

	return cmd
}

// migrateCmdHandler implements the `migrate` command.
func migrateCmdHandler(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	cfgPath, _ := cmd.Root().PersistentFlags().GetString("config")
	envName, _ := cmd.Root().PersistentFlags().GetString("env")
	testDir, _ := cmd.Flags().GetString("test-dir")
	toLevel, _ := cmd.Flags().GetString("to")
	areaNames, _ := cmd.Flags().GetStringSlice("area")
	apply, _ := cmd.Flags().GetBool("apply")
	status, _ := cmd.Flags().GetBool("status")
	explain, _ := cmd.Flags().GetBool("explain")
	unpin, _ := cmd.Flags().GetBool("unpin")
	// Pinning is conservative by default: a difference that only *may* materialise
	// still breaks a suite when it does, and the point of migrating is to end up
	// green. --only-certain narrows it for anyone who wants a smaller pin set.
	onlyCertain, _ := cmd.Flags().GetBool("only-certain")
	includeMay := !onlyCertain

	cfg, err := config.Load(cfgPath, envName)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	paths, err := resolveMigrateTargets(args, testDir, cfgPath, cfg)
	if err != nil {
		return err
	}

	if unpin {
		return runUnpin(out, paths)
	}
	if status {
		return runStatus(out, paths, cfg.Compat)
	}

	target, err := targetMode(toLevel, areaNames)
	if err != nil {
		return err
	}

	report, err := migrate.Analyze(paths, cfg.Compat, target)
	if err != nil {
		return err
	}

	if explain {
		return runExplain(out, report)
	}
	if apply {
		return runApply(out, report, cfgPath, toLevel, areaNames, cfg.Compat, includeMay)
	}

	printReport(out, report, cfg.Compat, target, includeMay)
	return nil
}

// resolveMigrateTargets finds the test files to operate on.
func resolveMigrateTargets(args []string, testDir, cfgPath string, cfg *config.LoadedConfig) ([]string, error) {
	targets := args
	if len(targets) == 0 {
		dir := testDir
		if dir == "" {
			dir = cfg.TestDir
			if dir == "" {
				dir = "tests"
			}
			if !filepath.IsAbs(dir) && cfgPath != "" {
				dir = filepath.Join(filepath.Dir(cfgPath), dir)
			}
		}
		targets = []string{dir}
	}
	return loader.DiscoverAll(targets)
}

// targetMode builds the mode being migrated to.
func targetMode(level string, areaNames []string) (tryve.CompatMode, error) {
	full, err := tryve.ParseAPIVersion(level)
	if err != nil {
		return tryve.LegacyCompat(), fmt.Errorf("--to: %w", err)
	}
	if len(areaNames) == 0 {
		return full, nil
	}

	// Limiting to areas means every other area stays where it is.
	mode := tryve.LegacyCompat()
	for _, name := range areaNames {
		area, err := tryve.ParseCompatArea(name)
		if err != nil {
			return tryve.LegacyCompat(), fmt.Errorf("--area: %w", err)
		}
		mode = mode.With(area, full.Modern(area))
	}
	return mode, nil
}

// printReport renders the analysis.
func printReport(out io.Writer, report *migrate.Report, from, to tryve.CompatMode, includeMay bool) {
	fmt.Fprintf(out, "Scanned %d test file(s).\n", report.FilesScanned)
	fmt.Fprintf(out, "Currently: %s   Target: %s\n\n", from, to)

	if len(report.Findings) == 0 {
		fmt.Fprintln(out, "Nothing behaves differently at the target level.")
		fmt.Fprintln(out, "Raise `compatibility` in your config, or run `tryve migrate --apply`.")
		return
	}

	byArea := map[tryve.CompatArea][]migrate.Finding{}
	for _, f := range report.Findings {
		byArea[f.Area] = append(byArea[f.Area], f)
	}

	for _, area := range []tryve.CompatArea{
		tryve.CompatAssertions, tryve.CompatInterpolation,
		tryve.CompatExecution, tryve.CompatAdapters,
	} {
		findings := byArea[area]
		if len(findings) == 0 {
			continue
		}
		fmt.Fprintf(out, "%s — %d site(s) in %d file(s)\n",
			strings.ToUpper(string(area)), len(findings), len(report.FilesFor(area)))

		// Group by rule so the report is a short list of kinds, not a wall.
		type ruleStat struct {
			rule      string
			certainty migrate.Certainty
			count     int
			example   migrate.Finding
		}
		stats := map[string]*ruleStat{}
		for _, f := range findings {
			st, ok := stats[f.Rule]
			if !ok {
				st = &ruleStat{rule: f.Rule, certainty: f.Certainty, example: f}
				stats[f.Rule] = st
			}
			st.count++
		}
		ordered := make([]*ruleStat, 0, len(stats))
		for _, st := range stats {
			ordered = append(ordered, st)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].count > ordered[j].count })

		for _, st := range ordered {
			fmt.Fprintf(out, "  %-5d %-12s %s\n", st.count, st.certainty, st.rule)
			fmt.Fprintf(out, "        e.g. %s:%d\n", shortPath(st.example.File), st.example.Line)
		}
		fmt.Fprintln(out)
	}

	willFiles := affected(report, false)
	mayFiles := affected(report, true)

	// Separate work still to do from work already deferred, so the report reads
	// as remaining effort once a migration is under way.
	var pinnedAlready, notYetPinned int
	for _, f := range mayFiles {
		if migrate.IsPinned(f) {
			pinnedAlready++
		} else {
			notYetPinned++
		}
	}

	fmt.Fprintf(out, "%d file(s) will behave differently; %d more may, depending on runtime values.\n",
		len(willFiles), len(mayFiles)-len(willFiles))
	if pinnedAlready > 0 {
		fmt.Fprintf(out, "%d of those are already pinned and awaiting work; %d are not yet pinned.\n",
			pinnedAlready, notYetPinned)
	}

	if len(report.Unparsable) > 0 {
		fmt.Fprintf(out, "%d file(s) could not be parsed and were skipped.\n", len(report.Unparsable))
	}

	pinCount := 0
	for _, f := range affected(report, includeMay) {
		if !migrate.IsPinned(f) {
			pinCount++
		}
	}

	fmt.Fprintf(out, "\nNext:\n")
	if pinCount == 0 {
		fmt.Fprintf(out, "  Every affected file is pinned. Work through them:\n")
		fmt.Fprintf(out, "    tryve migrate --explain <file>   what to fix\n")
		fmt.Fprintf(out, "    tryve migrate --unpin <file>     once it passes at the new level\n")
		return
	}
	fmt.Fprintf(out, "  tryve migrate --explain <file>   every difference in one file\n")
	fmt.Fprintf(out, "  tryve migrate --apply            raise the suite and pin %d more file(s)\n", pinCount)
	fmt.Fprintf(out, "                                   (%d certain + %d uncertain overall; --only-certain pins just the certain ones)\n",
		len(willFiles), len(mayFiles)-len(willFiles))
}

// affected returns the files with findings, optionally including uncertain ones.
func affected(report *migrate.Report, includeMay bool) []string {
	seen := map[string]struct{}{}
	for _, f := range report.Findings {
		if !includeMay && f.Certainty != migrate.WillChange {
			continue
		}
		seen[f.File] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// runExplain lists every finding, grouped by file.
func runExplain(out io.Writer, report *migrate.Report) error {
	if len(report.Findings) == 0 {
		fmt.Fprintln(out, "Nothing behaves differently at the target level.")
		return nil
	}

	byFile := map[string][]migrate.Finding{}
	for _, f := range report.Findings {
		byFile[f.File] = append(byFile[f.File], f)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, file := range files {
		findings := byFile[file]
		sort.Slice(findings, func(i, j int) bool { return findings[i].Line < findings[j].Line })
		fmt.Fprintf(out, "%s\n", shortPath(file))
		for _, f := range findings {
			fmt.Fprintf(out, "  line %-5d %-12s [%s] %s\n", f.Line, f.Certainty, f.Area, f.Detail)
		}
		fmt.Fprintln(out)
	}
	return nil
}

// runApply raises the config and pins the files that would behave differently.
func runApply(out io.Writer, report *migrate.Report, cfgPath, level string,
	areaNames []string, from tryve.CompatMode, includeMay bool) error {

	files := affected(report, includeMay)

	// A partial migration pins to the level files are on today, so the areas not
	// being migrated are unaffected either way.
	pinLevel := from.APIVersion()

	pinned := 0
	for _, file := range files {
		changed, err := migrate.Pin(file, pinLevel)
		if err != nil {
			return fmt.Errorf("pinning %s: %w", file, err)
		}
		if changed {
			pinned++
		}
	}

	if err := setConfigCompat(cfgPath, level, areaNames); err != nil {
		return err
	}

	if len(areaNames) == 0 {
		fmt.Fprintf(out, "Set apiVersion to %s in %s.\n", level, shortPath(cfgPath))
	} else {
		fmt.Fprintf(out, "Set compatibility.%s to %s in %s.\n",
			strings.Join(areaNames, ", compatibility."), level, shortPath(cfgPath))
	}
	fmt.Fprintf(out, "Pinned %d file(s) to %s.\n\n", pinned, pinLevel)
	fmt.Fprintln(out, "The suite behaves exactly as it did — run it now to confirm.")
	fmt.Fprintln(out, "Then work through the pinned files:")
	fmt.Fprintln(out, "  tryve migrate --status                 how many remain")
	fmt.Fprintln(out, "  tryve migrate --explain <file>         what to fix in one")
	fmt.Fprintln(out, "  tryve migrate --unpin <file>           once it passes at the new level")
	return nil
}

// runStatus reports how many files are pinned below the suite's level.
func runStatus(out io.Writer, paths []string, suite tryve.CompatMode) error {
	var pinned []string
	for _, p := range paths {
		if migrate.IsPinned(p) {
			pinned = append(pinned, p)
		}
	}
	sort.Strings(pinned)

	fmt.Fprintf(out, "Suite level: %s\n", suite)
	fmt.Fprintf(out, "%d of %d file(s) pinned to an older level.\n", len(pinned), len(paths))
	if len(pinned) == 0 {
		fmt.Fprintln(out, "\nMigration complete.")
		return nil
	}
	fmt.Fprintln(out)
	limit := 20
	for i, p := range pinned {
		if i == limit {
			fmt.Fprintf(out, "  … and %d more\n", len(pinned)-limit)
			break
		}
		fmt.Fprintf(out, "  %s\n", shortPath(p))
	}
	return nil
}

// runUnpin removes pins from the named files.
func runUnpin(out io.Writer, paths []string) error {
	count := 0
	for _, p := range paths {
		changed, err := migrate.Unpin(p)
		if err != nil {
			return fmt.Errorf("unpinning %s: %w", p, err)
		}
		if changed {
			count++
			fmt.Fprintf(out, "unpinned %s\n", shortPath(p))
		}
	}
	fmt.Fprintf(out, "\n%d file(s) unpinned. Run them to confirm they pass at the suite's level.\n", count)
	return nil
}

// setConfigCompat writes the target level into the config file.
func setConfigCompat(cfgPath, level string, areaNames []string) error {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", cfgPath, err)
	}
	content := string(raw)

	// Adopting every area sets apiVersion; adopting some sets the per-area
	// refinement beneath the level the suite is already on.
	var block string
	if len(areaNames) == 0 {
		block = fmt.Sprintf("apiVersion: %s\n", level)
	} else {
		var b strings.Builder
		b.WriteString("compatibility:\n")
		for _, name := range areaNames {
			fmt.Fprintf(&b, "  %s: %s\n", strings.TrimSpace(name), level)
		}
		block = b.String()
	}

	key := apiVersionKeyRe
	if len(areaNames) > 0 {
		key = compatKeyRe
	}
	if existing := key.FindStringIndex(content); existing != nil {
		end := compatBlockEnd(content, existing[0])
		content = content[:existing[0]] + block + content[end:]
	} else {
		content = block + "\n" + content
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, []byte(content), info.Mode().Perm())
}

// shortPath renders a path relative to the working directory when possible.
func shortPath(p string) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return p
}

// compatKeyRe matches a top-level compatibility block in a config file.
var compatKeyRe = regexp.MustCompile(`(?m)^compatibility\s*:.*$`)

// apiVersionKeyRe matches a top-level apiVersion in a config file.
var apiVersionKeyRe = regexp.MustCompile(`(?m)^apiVersion\s*:.*$`)

// compatBlockEnd finds where a compatibility block ends, so replacing it also
// removes any indented per-area entries beneath it.
func compatBlockEnd(content string, start int) int {
	rest := content[start:]
	lines := strings.SplitAfter(rest, "\n")
	offset := 0
	for i, line := range lines {
		if i == 0 {
			offset += len(line)
			continue
		}
		// Indented lines belong to the block; anything else ends it.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			offset += len(line)
			continue
		}
		break
	}
	return start + offset
}
