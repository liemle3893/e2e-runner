package tryve

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// CompatArea names a group of behaviours that changed together between API
// versions. Adopting a version moves every area; `compatibility` refines that
// per area, so a large suite can take one at a time.
type CompatArea string

const (
	// CompatAssertions covers which assertion forms are evaluated: field-name
	// assertions (exitCode, stdout, rowCount), row/column addressing of SQL
	// results, failing on an unrecognised operator, the extra operators and
	// aliases, and treating an array response body as the JSONPath root.
	CompatAssertions CompatArea = "assertions"

	// CompatInterpolation covers how {{…}} expressions resolve: whether a lone
	// expression keeps its type, how objects render as text, whether substituted
	// text is re-scanned, and whether quoted builtin arguments are unquoted.
	CompatInterpolation CompatArea = "interpolation"

	// CompatExecution covers step lifecycle: whether a step's own `timeout` and
	// `skip` fields take effect.
	CompatExecution CompatArea = "execution"

	// CompatAdapters covers adapter result shapes and defaults: SQL value
	// conversion, the postgresql count action, mongodb findOne's shape, the
	// shell command timeout fallback, and the HTTP request timeout model.
	CompatAdapters CompatArea = "adapters"
)

// compatAreas lists every area, in the order they are reported.
var compatAreas = []CompatArea{
	CompatAssertions, CompatInterpolation, CompatExecution, CompatAdapters,
}

// API version identifiers, as written in `apiVersion`.
const (
	// APIVersionV1 is the behaviour of releases before the areas above changed.
	// It is the default, so a file or config without an apiVersion behaves
	// exactly as it always has.
	APIVersionV1 = "tryve/v1"
	// APIVersionV2 is the current behaviour.
	APIVersionV2 = "tryve/v2"
)

// apiGroup is the group prefix in a qualified apiVersion.
const apiGroup = "tryve"

// CompatMode records, per area, whether the current behaviour applies.
//
// A zero CompatMode is fully tryve/v1, so any code path that has not been given
// a mode behaves the way it did before these areas changed. That is deliberate:
// forgetting to thread the mode through degrades to the old behaviour rather
// than silently changing what a test does.
type CompatMode struct {
	modern map[CompatArea]bool
}

// LegacyCompat returns a mode in which every area is at tryve/v1.
func LegacyCompat() CompatMode {
	return CompatMode{modern: map[CompatArea]bool{}}
}

// ModernCompat returns a mode in which every area is at tryve/v2.
func ModernCompat() CompatMode {
	m := map[CompatArea]bool{}
	for _, a := range compatAreas {
		m[a] = true
	}
	return CompatMode{modern: m}
}

// Modern reports whether the named area uses tryve/v2 behaviour.
func (c CompatMode) Modern(area CompatArea) bool {
	if c.modern == nil {
		return false
	}
	return c.modern[area]
}

// With returns a copy of c with the named area set.
func (c CompatMode) With(area CompatArea, modern bool) CompatMode {
	out := map[CompatArea]bool{}
	for k, v := range c.modern {
		out[k] = v
	}
	out[area] = modern
	return CompatMode{modern: out}
}

// APIVersion returns the version this mode corresponds to, for writing back into
// a file. A mode with a mix of areas reports tryve/v1 as its base — the per-area
// detail belongs in `compatibility`, not in `apiVersion`.
func (c CompatMode) APIVersion() string {
	for _, a := range compatAreas {
		if !c.Modern(a) {
			return APIVersionV1
		}
	}
	return APIVersionV2
}

// String renders the level for diagnostics.
func (c CompatMode) String() string {
	var modern, legacy []string
	for _, a := range compatAreas {
		if c.Modern(a) {
			modern = append(modern, string(a))
		} else {
			legacy = append(legacy, string(a))
		}
	}
	switch {
	case len(legacy) == 0:
		return APIVersionV2
	case len(modern) == 0:
		return APIVersionV1
	default:
		sort.Strings(modern)
		return fmt.Sprintf("%s, with %s at %s", APIVersionV1, strings.Join(modern, "+"), APIVersionV2)
	}
}

// ResolveLevel determines the behaviour level from an `apiVersion` and an
// optional `compatibility` refinement.
//
// apiVersion sets every area; compatibility then overrides individual ones. An
// absent apiVersion means tryve/v1, so pointing a new binary at an existing
// project changes nothing until the field is added.
func ResolveLevel(apiVersion, compatibility any) (CompatMode, error) {
	mode, err := ParseAPIVersion(apiVersion)
	if err != nil {
		return LegacyCompat(), err
	}
	return ApplyCompatOverrides(mode, compatibility)
}

// ParseAPIVersion interprets an `apiVersion` value.
//
// Accepted spellings, case-insensitively:
//
//	tryve/v2        the canonical form
//	v2              the group may be omitted
//	tryve.dev/v2    a fully-qualified group is accepted
//
// nil means tryve/v1.
func ParseAPIVersion(raw any) (CompatMode, error) {
	if raw == nil {
		return LegacyCompat(), nil
	}

	s, ok := raw.(string)
	if !ok {
		return LegacyCompat(), fmt.Errorf(
			"apiVersion must be a string such as %q, got %T", APIVersionV2, raw)
	}

	value := strings.ToLower(strings.TrimSpace(s))
	if value == "" {
		return LegacyCompat(), nil
	}

	// Split an optional group prefix, and check it names this tool.
	version := value
	if slash := strings.LastIndex(value, "/"); slash >= 0 {
		group, rest := value[:slash], value[slash+1:]
		if group != apiGroup && group != apiGroup+".dev" {
			return LegacyCompat(), fmt.Errorf(
				"apiVersion %q names the group %q; expected %q (as in %q)",
				s, group, apiGroup, APIVersionV2)
		}
		version = rest
	}

	switch version {
	case "v1", "1":
		return LegacyCompat(), nil
	case "v2", "2":
		return ModernCompat(), nil
	default:
		return LegacyCompat(), fmt.Errorf(
			"apiVersion %q is not a known version; use %q or %q", s, APIVersionV1, APIVersionV2)
	}
}

// ApplyCompatOverrides layers a `compatibility` block over a base level.
//
// The block names areas whose behaviour differs from the file's apiVersion,
// which is what lets a suite adopt one area at a time:
//
//	apiVersion: tryve/v1
//	compatibility:
//	  assertions: v2
func ApplyCompatOverrides(base CompatMode, raw any) (CompatMode, error) {
	if raw == nil {
		return base, nil
	}

	overrides, ok := raw.(map[string]any)
	if !ok {
		return base, fmt.Errorf(
			"compatibility must be a map of areas to versions (%s); "+
				"to set the level for the whole file, use apiVersion instead", compatAreaNames())
	}

	mode := base
	for key, val := range overrides {
		area, err := ParseCompatArea(key)
		if err != nil {
			return base, fmt.Errorf("compatibility: %w", err)
		}
		parsed, err := ParseAPIVersion(val)
		if err != nil {
			return base, fmt.Errorf("compatibility.%s: %w", key, err)
		}
		mode = mode.With(area, parsed.Modern(area))
	}
	return mode, nil
}

// ParseCompatArea resolves an area name, reporting the valid names on failure.
func ParseCompatArea(name string) (CompatArea, error) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, a := range compatAreas {
		if string(a) == want {
			return a, nil
		}
	}
	return "", fmt.Errorf("unknown area %q; valid areas are: %s", name, compatAreaNames())
}

// CompatAreas returns every area, for callers that enumerate them.
func CompatAreas() []CompatArea {
	out := make([]CompatArea, len(compatAreas))
	copy(out, compatAreas)
	return out
}

// compatAreaNames lists the valid area names for error messages.
func compatAreaNames() string {
	names := make([]string, 0, len(compatAreas))
	for _, a := range compatAreas {
		names = append(names, string(a))
	}
	return strings.Join(names, ", ")
}

// compatContextKey is the private key under which a CompatMode travels in a
// context.Context.
type compatContextKey struct{}

// ContextWithCompat attaches a behaviour level to a context.
//
// Adapters are built once for the whole suite, but a single test file may
// declare a different apiVersion. Carrying the level on the request context is
// what lets an adapter honour that, rather than applying the suite's level to
// every step it serves.
func ContextWithCompat(ctx context.Context, mode CompatMode) context.Context {
	return context.WithValue(ctx, compatContextKey{}, mode)
}

// CompatFromContext returns the level attached to ctx, and whether there was one.
func CompatFromContext(ctx context.Context) (CompatMode, bool) {
	if ctx == nil {
		return LegacyCompat(), false
	}
	mode, ok := ctx.Value(compatContextKey{}).(CompatMode)
	return mode, ok
}

// CompatOrDefault returns the level attached to ctx, falling back to fallback.
func CompatOrDefault(ctx context.Context, fallback CompatMode) CompatMode {
	if mode, ok := CompatFromContext(ctx); ok {
		return mode
	}
	return fallback
}
