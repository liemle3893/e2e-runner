package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAutoflowJiraSubtree(t *testing.T) {
	want := []string{
		"config", "download", "fetch", "search", "transition",
		"transitions", "upload",
	}
	sort.Strings(want)

	root := NewRoot("test")
	d := findChild(root, "deliver")
	if d == nil {
		t.Fatal("deliver subcommand not found under root")
	}
	j := findChild(d, "jira")
	if j == nil {
		t.Fatal("jira subcommand not found under deliver")
	}
	got := commandNames(j.Commands())
	assertEqualNames(t, "jira", got, want)
}

func TestParseCSV(t *testing.T) {
	cases := map[string][]string{
		"":                 nil,
		"  ":               nil,
		"a":                {"a"},
		"a,b,c":            {"a", "b", "c"},
		" a , b , c ":      {"a", "b", "c"},
		"a,,b,":            {"a", "b"},
	}
	for in, want := range cases {
		got := parseCSV(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseCSV(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWriteJSON_Stdout(t *testing.T) {
	cmd := newAutoflowJiraFetchCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := writeJSON(cmd, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, `"k": "v"`) {
		t.Errorf("stdout JSON missing key: %q", out)
	}
}

func TestWriteJSON_OutFile(t *testing.T) {
	cmd := newAutoflowJiraFetchCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	dir := t.TempDir()
	target := filepath.Join(dir, "out.json")
	_ = cmd.Flags().Set("out", target)
	if err := writeJSON(cmd, map[string]int{"n": 3}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"n": 3`) {
		t.Errorf("file missing expected JSON: %q", string(data))
	}
	if !strings.Contains(buf.String(), target) {
		t.Errorf("stdout should mention output path, got %q", buf.String())
	}
}

// runRootCmd executes the autoflow root command with the given args and
// returns the combined stderr/stdout plus any execution error.
func runRootCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRoot("test")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestJiraTransitionRequiresFlag(t *testing.T) {
	// Ensure we don't require Jira env to evaluate argument validation —
	// but both flags missing should error before any credential lookup.
	t.Setenv("JIRA_API_TOKEN", "") // unset; validation must fire first

	// Because RunE runs in order (flags parsed -> RunE body), the
	// name/id check is the first effect.
	_, err := runRootCmd(t, "deliver", "jira", "transition", "PROJ-1")
	if err == nil {
		t.Fatal("expected error when --name/--id both omitted")
	}
	if !strings.Contains(err.Error(), "--name") && !strings.Contains(err.Error(), "--id") {
		t.Errorf("error should mention --name/--id, got: %v", err)
	}
}

func TestJiraSearchRequiresJQL(t *testing.T) {
	_, err := runRootCmd(t, "deliver", "jira", "search")
	if err == nil {
		t.Fatal("expected error when --jql omitted")
	}
	if !strings.Contains(err.Error(), "jql") {
		t.Errorf("error should mention jql, got: %v", err)
	}
}

func TestJiraFetchHelp(t *testing.T) {
	out, err := runRootCmd(t, "deliver", "jira", "fetch", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fields", "expand", "out"} {
		if !strings.Contains(out, want) {
			t.Errorf("fetch --help missing flag %q; output:\n%s", want, out)
		}
	}
}

// assertCommandExists is a small helper used below.
func assertCommandExists(t *testing.T, parent *cobra.Command, name string) {
	t.Helper()
	if findChild(parent, name) == nil {
		t.Errorf("missing subcommand %q under %q", name, parent.Name())
	}
}

func TestJiraSubcommandsExist(t *testing.T) {
	root := NewRoot("test")
	d := findChild(root, "deliver")
	if d == nil {
		t.Fatal("deliver subcommand not found under root")
	}
	j := findChild(d, "jira")
	if j == nil {
		t.Fatal("jira subcommand not found under deliver")
	}
	for _, n := range []string{"fetch", "search", "transitions", "transition"} {
		assertCommandExists(t, j, n)
	}
}
