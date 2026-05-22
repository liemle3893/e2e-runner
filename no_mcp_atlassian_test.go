package assets_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoMcpAtlassianInDeliver is a regression guard for PR 2: the
// read-path migration of autoflow-deliver + its spawned agent
// (autoflow-jira-fetcher) from Atlassian MCP to REST via `autoflow deliver jira`.
//
// The autoflow-ticket skill is EXEMPT — it still uses MCP for
// edit/create/link operations and will be migrated in a follow-up PR.
func TestNoMcpAtlassianInDeliver(t *testing.T) {
	roots := []string{
		"skills/autoflow/autoflow-deliver",
		"agents/autoflow/autoflow-jira-fetcher.md",
	}
	banned := []string{"mcp__atlassian__", "mcp__claude_ai_Atlassian__"}

	var offenders []string
	check := func(rel string, data []byte) {
		for _, needle := range banned {
			if strings.Contains(string(data), needle) {
				offenders = append(offenders, rel+" contains "+needle)
				break
			}
		}
	}

	for _, r := range roots {
		info, err := os.Stat(r)
		if err != nil {
			t.Fatalf("stat %s: %v", r, err)
		}
		if !info.IsDir() {
			data, err := os.ReadFile(r)
			if err != nil {
				t.Fatalf("read %s: %v", r, err)
			}
			check(r, data)
			continue
		}
		err = filepath.WalkDir(r, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(".", path)
			check(rel, data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", r, err)
		}
	}

	if len(offenders) > 0 {
		t.Errorf("MCP Atlassian references must be purged from autoflow-deliver surface:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
