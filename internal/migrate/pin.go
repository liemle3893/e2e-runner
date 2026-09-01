package migrate

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// pinComment introduces a pin so the next reader knows it is temporary and how
// to clear it.
const pinComment = "# Pinned by `tryve migrate`: this file relies on behaviour that changed.\n" +
	"# Run `tryve migrate --explain <this file>` for what differs, fix what it\n" +
	"# reports, then delete these three lines to move it to the suite's level.\n"

// existingPin matches an apiVersion already declared at the top level of a file.
// A `compatibility` block alone also counts: it means the file has been given a
// level of its own, whether or not it names an apiVersion.
var existingPin = regexp.MustCompile(`(?m)^(apiVersion|compatibility)\s*:.*$`)

// Pin writes a top-level `apiVersion` into a test file, so it keeps its current
// behaviour while the rest of the suite moves on.
//
// The rest of the file is left byte-for-byte intact: the key is inserted as
// text rather than by re-serialising the YAML, which would drop comments,
// reflow block scalars, and produce a diff nobody can review.
func Pin(path, level string) (changed bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(raw)

	if existingPin.MatchString(content) {
		// Already pinned; leave whatever level it names alone.
		return false, nil
	}

	insertAt := insertionPoint(content)
	updated := content[:insertAt] + pinComment +
		fmt.Sprintf("apiVersion: %s\n\n", level) + content[insertAt:]

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

// Unpin removes a pin previously written by Pin, along with its comment.
func Unpin(path string) (changed bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(raw)
	if !existingPin.MatchString(content) {
		return false, nil
	}

	lines := strings.Split(content, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "# Pinned by `tryve migrate`") {
			// Drop the comment block, the key, and one blank line after it.
			for i < len(lines) && strings.HasPrefix(lines[i], "#") {
				i++
			}
			if i < len(lines) && existingPin.MatchString(lines[i]) {
				i++
			}
			if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				continue
			}
			i--
			continue
		}
		if existingPin.MatchString(line) {
			// A hand-written declaration: remove the key, keep the comments.
			continue
		}
		out = append(out, line)
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")), info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

// IsPinned reports whether a file already declares a level of its own.
func IsPinned(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return existingPin.MatchString(string(raw))
}

// insertionPoint finds the offset to insert at: after any leading comment block
// and blank lines, so a schema directive or a file header stays at the top.
func insertionPoint(content string) int {
	offset := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += len(line)
			continue
		}
		break
	}
	return offset
}
