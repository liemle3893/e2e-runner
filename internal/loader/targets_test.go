package loader_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liemle3893/go-tryve/internal/loader"
)

// writeTestTree creates a small tree of test files and returns its root.
func writeTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := []string{
		"users/TC-USER-001.test.yaml",
		"users/TC-USER-002.test.yaml",
		"auth/TC-AUTH-001.test.yaml",
		"auth/notes.md",
	}
	for _, rel := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("name: placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestDiscoverAllSingleFile covers running exactly one test file, the case that
// previously required knowing that -d happened to accept a file path.
func TestDiscoverAllSingleFile(t *testing.T) {
	root := writeTestTree(t)
	target := filepath.Join(root, "users/TC-USER-001.test.yaml")

	got, err := loader.DiscoverAll([]string{target})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || !strings.HasSuffix(got[0], "TC-USER-001.test.yaml") {
		t.Errorf("expected exactly the named file, got %v", got)
	}
}

// TestDiscoverAllDirectory covers the directory form.
func TestDiscoverAllDirectory(t *testing.T) {
	root := writeTestTree(t)

	got, err := loader.DiscoverAll([]string{filepath.Join(root, "users")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 test files, got %d: %v", len(got), got)
	}
}

// TestDiscoverAllMultipleTargetsDeduplicates checks that overlapping targets do
// not run the same test twice.
func TestDiscoverAllMultipleTargetsDeduplicates(t *testing.T) {
	root := writeTestTree(t)

	got, err := loader.DiscoverAll([]string{
		filepath.Join(root, "users"),
		filepath.Join(root, "users/TC-USER-001.test.yaml"),
		filepath.Join(root, "auth"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 unique test files, got %d: %v", len(got), got)
	}
}

// TestDiscoverAllGlob covers shell-style patterns, including "**".
func TestDiscoverAllGlob(t *testing.T) {
	root := writeTestTree(t)

	got, err := loader.DiscoverAll([]string{filepath.Join(root, "**", "TC-USER-*.test.yaml")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected the two user tests, got %d: %v", len(got), got)
	}

	got, err = loader.DiscoverAll([]string{filepath.Join(root, "auth", "*.test.yaml")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected one auth test, got %d: %v", len(got), got)
	}
}

// TestDiscoverAllMissingTargetErrors checks that a mistyped path is reported
// rather than quietly running a different set of tests.
func TestDiscoverAllMissingTargetErrors(t *testing.T) {
	root := writeTestTree(t)

	_, err := loader.DiscoverAll([]string{filepath.Join(root, "users/TC-NOPE-999.test.yaml")})
	if err == nil {
		t.Fatalf("a path matching nothing must be an error")
	}
	if !strings.Contains(err.Error(), "TC-NOPE-999") {
		t.Errorf("the error should name the target that matched nothing, got: %v", err)
	}
}
