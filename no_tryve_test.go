package assets_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTryveSurvives is a brand regression guard. It walks the repo and
// fails if the string "tryve" (case-insensitive) appears in any tracked
// source, config, or documentation file. Historical planning docs under
// docs/superpowers/ are excluded — those are frozen records that
// deliberately reference the pre-rebrand name.
func TestNoTryveSurvives(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs cwd: %v", err)
	}

	var offenders []string
	skipDirs := map[string]struct{}{
		".git":                 {},
		"bin":                  {},
		"vendor":               {},
		"node_modules":         {},
		".claude":              {},
		".idea":                {},
		".planning":            {},
	}
	skipFiles := map[string]struct{}{
		"no_tryve_test.go": {},
		".goreleaser.yaml": {}, // repo name is go-tryve on GitHub
	}
	skipExts := map[string]struct{}{
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".pdf": {},
		".ico": {}, ".woff": {}, ".woff2": {}, ".ttf": {},
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip {
				return fs.SkipDir
			}
			// Skip historical planning trees.
			if strings.HasPrefix(rel, "docs/superpowers") ||
				strings.HasPrefix(rel, "docs/plans") {
				return fs.SkipDir
			}
			return nil
		}
		if _, skip := skipFiles[d.Name()]; skip {
			return nil
		}
		if _, skip := skipExts[strings.ToLower(filepath.Ext(path))]; skip {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(data)), "tryve") {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("rebrand regression: %d files still reference \"tryve\":\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
