package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Discover walks dir recursively and returns absolute paths to all files whose
// names end in ".test.yaml" or ".test.yml".
//
// Directories whose names start with "." (hidden) or equal "node_modules" are
// skipped entirely so that vendor trees and dot-folders are never scanned.
func Discover(dir string) ([]string, error) {
	var paths []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			// Skip the root dir itself; only prune children.
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}

		name := info.Name()
		if strings.HasSuffix(name, ".test.yaml") || strings.HasSuffix(name, ".test.yml") {
			abs, absErr := filepath.Abs(path)
			if absErr != nil {
				return absErr
			}
			paths = append(paths, abs)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// DiscoverAll resolves a list of targets — each a test file, a directory to
// search recursively, or a glob pattern — into test file paths.
//
// A target that matches nothing is an error rather than a silent skip: a
// mistyped path should say so, not quietly run a different set of tests than
// the one asked for.
func DiscoverAll(targets []string) ([]string, error) {
	var paths []string
	seen := make(map[string]struct{})

	add := func(found []string) {
		for _, p := range found {
			if _, dup := seen[p]; dup {
				continue
			}
			seen[p] = struct{}{}
			paths = append(paths, p)
		}
	}

	for _, target := range targets {
		found, err := discoverTarget(target)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no test files matched %q", target)
		}
		add(found)
	}

	return paths, nil
}

// discoverTarget resolves a single target to the test files it names.
func discoverTarget(target string) ([]string, error) {
	info, statErr := os.Stat(target)
	switch {
	case statErr == nil && info.IsDir():
		return Discover(target)

	case statErr == nil:
		// A file named explicitly is run whether or not it follows the
		// ".test.yaml" convention — the author has already been specific.
		abs, err := filepath.Abs(target)
		if err != nil {
			return nil, err
		}
		return []string{abs}, nil
	}

	// Not an existing path: treat it as a glob. "**" is expanded by walking,
	// since filepath.Glob does not support it.
	matches, err := expandGlob(target)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", target, err)
	}
	return matches, nil
}

// expandGlob resolves a shell-style pattern, including a "**" segment matching
// any number of directories.
func expandGlob(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return absPaths(filepath.Glob(pattern))
	}

	root, rest, _ := strings.Cut(pattern, "**")
	root = filepath.Dir(filepath.Join(root, "x"))
	if root == "" {
		root = "."
	}
	suffix := strings.TrimPrefix(rest, string(filepath.Separator))

	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		ok, matchErr := filepath.Match(suffix, filepath.Base(path))
		if matchErr != nil {
			return matchErr
		}
		if ok {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return absPaths(matches, nil)
}

// absPaths converts a glob result to absolute paths, passing through any error.
func absPaths(matches []string, err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		abs, absErr := filepath.Abs(m)
		if absErr != nil {
			return nil, absErr
		}
		out = append(out, abs)
	}
	return out, nil
}
