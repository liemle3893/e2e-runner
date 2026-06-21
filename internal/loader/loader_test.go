package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/liemle3893/go-tryve/internal/loader"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// --- helpers ---

// writeFile writes content to a file inside dir and returns the path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
	return path
}

// --- discovery tests ---

// TestDiscover_FindsTestFiles verifies that Discover returns .test.yaml and
// .test.yml files, recurses into sub-directories, and skips unrelated files,
// hidden directories, and node_modules.
func TestDiscover_FindsTestFiles(t *testing.T) {
	root := t.TempDir()

	// Expected: plain .test.yaml at root level.
	writeFile(t, root, "alpha.test.yaml", "name: alpha")

	// Expected: .test.yml variant.
	writeFile(t, root, "beta.test.yml", "name: beta")

	// Not expected: plain .yaml file.
	writeFile(t, root, "config.yaml", "version: 1.0")

	// Expected: .test.yaml in a sub-directory.
	subDir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	writeFile(t, subDir, "gamma.test.yaml", "name: gamma")

	// Not expected: file inside a hidden directory.
	hiddenDir := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatalf("mkdir .hidden: %v", err)
	}
	writeFile(t, hiddenDir, "hidden.test.yaml", "name: hidden")

	// Not expected: file inside node_modules.
	nmDir := filepath.Join(root, "node_modules")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	writeFile(t, nmDir, "dep.test.yaml", "name: dep")

	paths, err := loader.Discover(root)
	if err != nil {
		t.Fatalf("Discover() unexpected error: %v", err)
	}

	// Build a set of basenames for easy lookup.
	found := make(map[string]bool, len(paths))
	for _, p := range paths {
		// Verify that every returned path is absolute.
		if !filepath.IsAbs(p) {
			t.Errorf("Discover() returned non-absolute path: %s", p)
		}
		found[filepath.Base(p)] = true
	}

	for _, want := range []string{"alpha.test.yaml", "beta.test.yml", "gamma.test.yaml"} {
		if !found[want] {
			t.Errorf("Discover() missing expected file: %s", want)
		}
	}
	for _, unwanted := range []string{"config.yaml", "hidden.test.yaml", "dep.test.yaml"} {
		if found[unwanted] {
			t.Errorf("Discover() should not have included: %s", unwanted)
		}
	}
}

// --- parser tests ---

// TestParse_MinimalTest verifies that a file with only the required fields is
// parsed correctly and that adapter-specific fields (url, method) are collected
// into Params rather than being silently dropped.
func TestParse_MinimalTest(t *testing.T) {
	dir := t.TempDir()
	content := `
name: TC-MINIMAL-001
tags: [smoke, minimal]
priority: P1

execute:
  - adapter: http
    action: request
    url: "https://example.com/health"
    method: GET
`
	path := writeFile(t, dir, "minimal.test.yaml", content)

	td, err := loader.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() unexpected error: %v", err)
	}

	if td.Name != "TC-MINIMAL-001" {
		t.Errorf("Name = %q, want %q", td.Name, "TC-MINIMAL-001")
	}
	if td.Priority != "P1" {
		t.Errorf("Priority = %q, want P1", td.Priority)
	}
	if len(td.Tags) != 2 {
		t.Fatalf("len(Tags) = %d, want 2", len(td.Tags))
	}
	if td.Tags[0] != "smoke" || td.Tags[1] != "minimal" {
		t.Errorf("Tags = %v, want [smoke minimal]", td.Tags)
	}
	if td.SourceFile != path {
		t.Errorf("SourceFile = %q, want %q", td.SourceFile, path)
	}

	if len(td.Execute) != 1 {
		t.Fatalf("len(Execute) = %d, want 1", len(td.Execute))
	}
	step := td.Execute[0]
	if step.Adapter != "http" {
		t.Errorf("step.Adapter = %q, want %q", step.Adapter, "http")
	}
	if step.Action != "request" {
		t.Errorf("step.Action = %q, want %q", step.Action, "request")
	}

	// Adapter-specific params must be collected into Params.
	if step.Params == nil {
		t.Fatal("step.Params is nil, want map with url and method")
	}
	if url, _ := step.Params["url"].(string); url != "https://example.com/health" {
		t.Errorf("Params[url] = %q, want %q", step.Params["url"], "https://example.com/health")
	}
	if method, _ := step.Params["method"].(string); method != "GET" {
		t.Errorf("Params[method] = %q, want %q", step.Params["method"], "GET")
	}
}

// TestParse_WithAllPhases verifies that all four lifecycle phases are parsed and
// that step IDs follow the "{phase}-{index}" format.
func TestParse_WithAllPhases(t *testing.T) {
	dir := t.TempDir()
	content := `
name: TC-PHASES-001

setup:
  - adapter: http
    action: request
    url: "https://example.com/setup"

execute:
  - adapter: http
    action: request
    url: "https://example.com/execute"

verify:
  - adapter: http
    action: request
    url: "https://example.com/verify"

teardown:
  - adapter: http
    action: request
    url: "https://example.com/teardown"
`
	path := writeFile(t, dir, "phases.test.yaml", content)

	td, err := loader.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() unexpected error: %v", err)
	}

	phaseChecks := []struct {
		phase string
		steps []tryve.StepDefinition
		wantID string
	}{
		{"setup", td.Setup, "setup-0"},
		{"execute", td.Execute, "execute-0"},
		{"verify", td.Verify, "verify-0"},
		{"teardown", td.Teardown, "teardown-0"},
	}

	for _, tc := range phaseChecks {
		if len(tc.steps) != 1 {
			t.Errorf("[%s] len(steps) = %d, want 1", tc.phase, len(tc.steps))
			continue
		}
		if tc.steps[0].ID != tc.wantID {
			t.Errorf("[%s] step.ID = %q, want %q", tc.phase, tc.steps[0].ID, tc.wantID)
		}
	}
}

// TestParse_WithCapture verifies that a capture map defined in a step is
// unmarshalled into the StepDefinition.Capture field.
func TestParse_WithCapture(t *testing.T) {
	dir := t.TempDir()
	content := `
name: TC-CAPTURE-001

execute:
  - adapter: http
    action: request
    url: "https://example.com/users/1"
    capture:
      user_id: "$.id"
      user_name: "$.name"
`
	path := writeFile(t, dir, "capture.test.yaml", content)

	td, err := loader.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() unexpected error: %v", err)
	}

	if len(td.Execute) != 1 {
		t.Fatalf("len(Execute) = %d, want 1", len(td.Execute))
	}
	step := td.Execute[0]

	if step.Capture == nil {
		t.Fatal("step.Capture is nil, want populated map")
	}
	if step.Capture["user_id"] != "$.id" {
		t.Errorf("Capture[user_id] = %q, want %q", step.Capture["user_id"], "$.id")
	}
	if step.Capture["user_name"] != "$.name" {
		t.Errorf("Capture[user_name] = %q, want %q", step.Capture["user_name"], "$.name")
	}
}

// --- validator tests ---

// TestValidate_MissingName verifies that a TestDefinition with an empty name
// produces a validation error.
func TestValidate_MissingName(t *testing.T) {
	td := &tryve.TestDefinition{
		Execute: []tryve.StepDefinition{
			{Adapter: "http", Action: "request", Params: map[string]any{"url": "https://example.com"}},
		},
	}

	errs := loader.Validate(td)
	if len(errs) == 0 {
		t.Fatal("Validate() expected errors for missing name, got none")
	}
	hasNameErr := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "name") {
			hasNameErr = true
			break
		}
	}
	if !hasNameErr {
		t.Errorf("Validate() errors %v do not mention missing name", errs)
	}
}

// TestValidate_InvalidAdapter verifies that a step referencing an unknown
// adapter type is rejected.
func TestValidate_InvalidAdapter(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-INVALID-ADAPTER",
		Execute: []tryve.StepDefinition{
			{Adapter: "ftp", Action: "upload"},
		},
	}

	errs := loader.Validate(td)
	if len(errs) == 0 {
		t.Fatal("Validate() expected errors for invalid adapter, got none")
	}
	hasAdapterErr := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "ftp") || containsSubstring(e.Error(), "adapter") {
			hasAdapterErr = true
			break
		}
	}
	if !hasAdapterErr {
		t.Errorf("Validate() errors %v do not mention invalid adapter", errs)
	}
}

// TestValidate_HTTPMissingURL verifies that an HTTP step without a url in Params
// produces a validation error.
func TestValidate_HTTPMissingURL(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-HTTP-NO-URL",
		Execute: []tryve.StepDefinition{
			{Adapter: "http", Action: "request", Params: map[string]any{}},
		},
	}

	errs := loader.Validate(td)
	if len(errs) == 0 {
		t.Fatal("Validate() expected errors for missing HTTP url, got none")
	}
	hasURLErr := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "url") {
			hasURLErr = true
			break
		}
	}
	if !hasURLErr {
		t.Errorf("Validate() errors %v do not mention missing url", errs)
	}
}

// TestValidate_ProcessStartValid verifies that a valid process/start step passes validation.
func TestValidate_ProcessStartValid(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-PROCESS-VALID",
		Execute: []tryve.StepDefinition{
			{Adapter: "shell", Action: "exec", Params: map[string]any{"command": "echo ok"}},
		},
		Setup: []tryve.StepDefinition{
			{
				Name:    "my-server",
				Adapter: "process",
				Action:  "start",
				Params:  map[string]any{"command": "sleep 60"},
			},
		},
	}
	errs := loader.Validate(td)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

// TestValidate_ProcessStartMissingCommand verifies that process/start without command is an error.
func TestValidate_ProcessStartMissingCommand(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-PROCESS-NO-CMD",
		Execute: []tryve.StepDefinition{
			{Adapter: "shell", Action: "exec", Params: map[string]any{"command": "echo ok"}},
		},
		Setup: []tryve.StepDefinition{
			{Adapter: "process", Action: "start", Params: map[string]any{}},
		},
	}
	errs := loader.Validate(td)
	if len(errs) == 0 {
		t.Fatal("expected error for missing command")
	}
}

// TestValidate_ProcessStopRequiresTargetOrPID verifies that process/stop without target or pid fails.
func TestValidate_ProcessStopRequiresTargetOrPID(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-PROCESS-STOP-NO-TARGET",
		Execute: []tryve.StepDefinition{
			{Adapter: "shell", Action: "exec", Params: map[string]any{"command": "echo ok"}},
		},
		Teardown: []tryve.StepDefinition{
			{Adapter: "process", Action: "stop", Params: map[string]any{}},
		},
	}
	errs := loader.Validate(td)
	found := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "target") || containsSubstring(e.Error(), "pid") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error mentioning target/pid, got: %v", errs)
	}
}

// TestValidate_ProcessBackgroundFalse verifies that background: false is rejected.
func TestValidate_ProcessBackgroundFalse(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-PROCESS-BG-FALSE",
		Execute: []tryve.StepDefinition{
			{Adapter: "shell", Action: "exec", Params: map[string]any{"command": "echo ok"}},
		},
		Setup: []tryve.StepDefinition{
			{Adapter: "process", Action: "start", Params: map[string]any{
				"command":    "sleep 60",
				"background": false,
			}},
		},
	}
	errs := loader.Validate(td)
	found := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "background") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error mentioning background, got: %v", errs)
	}
}

// TestValidate_DuplicateStepName verifies that duplicate step names are rejected.
func TestValidate_DuplicateStepName(t *testing.T) {
	td := &tryve.TestDefinition{
		Name: "TC-DUP-NAME",
		Execute: []tryve.StepDefinition{
			{Adapter: "shell", Action: "exec", Params: map[string]any{"command": "echo ok"}},
		},
		Setup: []tryve.StepDefinition{
			{Name: "server", Adapter: "process", Action: "start", Params: map[string]any{"command": "sleep 60"}},
			{Name: "server", Adapter: "process", Action: "start", Params: map[string]any{"command": "sleep 60"}},
		},
	}
	errs := loader.Validate(td)
	found := false
	for _, e := range errs {
		if containsSubstring(e.Error(), "duplicate") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error mentioning duplicate, got: %v", errs)
	}
}

// containsSubstring reports whether s contains substr (case-insensitive helper).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				match := true
				for j := 0; j < len(substr); j++ {
					cs, csub := s[i+j], substr[j]
					if cs >= 'A' && cs <= 'Z' {
						cs += 'a' - 'A'
					}
					if csub >= 'A' && csub <= 'Z' {
						csub += 'a' - 'A'
					}
					if cs != csub {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
			return false
		}())
}
