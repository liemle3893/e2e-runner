package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// chdirTemp creates a git-initialised temp dir and chdirs into it so
// state.RepoRoot() resolves there. Returns cleanup path.
func chdirTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// `git init` so RepoRoot resolves here.
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skipf("git init unavailable: %v", err)
	}
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

func TestAutoflowConfig_SetGetShow(t *testing.T) {
	dir := chdirTempRepo(t)

	root := NewRoot("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	root.SetArgs([]string{"deliver", "config", "set", "coding_agent", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("set: %v", err)
	}
	// File created.
	if _, err := os.Stat(filepath.Join(dir, ".autoflow", "config.json")); err != nil {
		t.Fatalf("config not written: %v", err)
	}

	out.Reset()
	root.SetArgs([]string{"deliver", "config", "get", "coding_agent"})
	if err := root.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "claude" {
		t.Errorf("get coding_agent = %q, want claude", got)
	}

	out.Reset()
	root.SetArgs([]string{"deliver", "config", "show"})
	if err := root.Execute(); err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(out.String(), `"coding_agent": "claude"`) {
		t.Errorf("show output missing coding_agent: %s", out.String())
	}
}

func TestAutoflowConfig_SetInvalidAgent(t *testing.T) {
	chdirTempRepo(t)
	root := NewRoot("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"deliver", "config", "set", "coding_agent", "gpt"})
	if err := root.Execute(); err == nil {
		t.Error("want error for invalid agent")
	}
}
