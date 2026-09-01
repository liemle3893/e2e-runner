package tryve_test

import (
	"testing"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// TestAbsentAPIVersionIsV1 pins the contract that makes an upgrade safe: a file
// or config that says nothing runs exactly as it always has.
func TestAbsentAPIVersionIsV1(t *testing.T) {
	mode, err := tryve.ResolveLevel(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, area := range tryve.CompatAreas() {
		if mode.Modern(area) {
			t.Errorf("area %s should be at tryve/v1 when apiVersion is absent", area)
		}
	}
	if got := mode.APIVersion(); got != tryve.APIVersionV1 {
		t.Errorf("expected %q, got %q", tryve.APIVersionV1, got)
	}
}

// TestAPIVersionSpellings covers the forms accepted for apiVersion.
func TestAPIVersionSpellings(t *testing.T) {
	cases := []struct {
		in     any
		modern bool
	}{
		{"tryve/v1", false},
		{"tryve/v2", true},
		{"v1", false},
		{"v2", true},
		{"TRYVE/V2", true},
		{"  tryve/v2  ", true},
		{"tryve.dev/v2", true},
		{"", false},
		{nil, false},
	}

	for _, tc := range cases {
		mode, err := tryve.ParseAPIVersion(tc.in)
		if err != nil {
			t.Errorf("%v: unexpected error: %v", tc.in, err)
			continue
		}
		if got := mode.Modern(tryve.CompatAssertions); got != tc.modern {
			t.Errorf("%v: expected modern=%v, got %v", tc.in, tc.modern, got)
		}
	}
}

// TestAPIVersionRejectsUnknown checks that a typo is an error naming the valid
// values, rather than silently selecting a level.
func TestAPIVersionRejectsUnknown(t *testing.T) {
	for _, in := range []any{"v3", "tryve/v9", "k8s/v1", "latest", 2} {
		if _, err := tryve.ParseAPIVersion(in); err == nil {
			t.Errorf("%v: expected an error", in)
		}
	}
}

// TestCompatibilityRefinesAPIVersion covers adopting one area at a time.
func TestCompatibilityRefinesAPIVersion(t *testing.T) {
	mode, err := tryve.ResolveLevel("tryve/v1", map[string]any{"assertions": "v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mode.Modern(tryve.CompatAssertions) {
		t.Error("assertions should be at tryve/v2")
	}
	for _, area := range []tryve.CompatArea{
		tryve.CompatInterpolation, tryve.CompatExecution, tryve.CompatAdapters,
	} {
		if mode.Modern(area) {
			t.Errorf("area %s should stay at tryve/v1", area)
		}
	}

	// A refinement can also hold an area back from a v2 baseline.
	mode, err = tryve.ResolveLevel("tryve/v2", map[string]any{"adapters": "tryve/v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode.Modern(tryve.CompatAdapters) {
		t.Error("adapters should have been held at tryve/v1")
	}
	if !mode.Modern(tryve.CompatAssertions) {
		t.Error("assertions should follow the v2 baseline")
	}
}

// TestCompatibilityRejectsAScalar checks that the level belongs in apiVersion:
// `compatibility` is the per-area refinement, and a scalar there says the author
// meant something else.
func TestCompatibilityRejectsAScalar(t *testing.T) {
	_, err := tryve.ResolveLevel(nil, "v2")
	if err == nil {
		t.Fatal("expected an error for a scalar compatibility")
	}
	if got := err.Error(); got == "" {
		t.Error("the error should explain what to use instead")
	}
}

// TestCompatibilityRejectsUnknownArea checks a misspelled area is reported.
func TestCompatibilityRejectsUnknownArea(t *testing.T) {
	if _, err := tryve.ResolveLevel(nil, map[string]any{"assertion": "v2"}); err == nil {
		t.Error("expected an error naming the valid areas")
	}
}

// TestAPIVersionRoundTrip checks the value written back into a file.
func TestAPIVersionRoundTrip(t *testing.T) {
	if got := tryve.ModernCompat().APIVersion(); got != tryve.APIVersionV2 {
		t.Errorf("expected %q, got %q", tryve.APIVersionV2, got)
	}
	if got := tryve.LegacyCompat().APIVersion(); got != tryve.APIVersionV1 {
		t.Errorf("expected %q, got %q", tryve.APIVersionV1, got)
	}
	// A partially-adopted suite reports its base version; the detail lives in
	// `compatibility`, so writing it back must not claim the whole suite is v2.
	partial := tryve.LegacyCompat().With(tryve.CompatAssertions, true)
	if got := partial.APIVersion(); got != tryve.APIVersionV1 {
		t.Errorf("a partial adoption should report %q, got %q", tryve.APIVersionV1, got)
	}
}
