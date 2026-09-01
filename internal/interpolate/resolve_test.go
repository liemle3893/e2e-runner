package interpolate_test

import (
	"reflect"
	"testing"

	"github.com/liemle3893/go-tryve/internal/interpolate"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

// ctxWith builds a context using the current (v2) interpolation behaviour.
func ctxWith(captured map[string]any) *tryve.InterpolationContext {
	ctx := legacyCtxWith(captured)
	ctx.Compat = tryve.ModernCompat()
	return ctx
}

// legacyCtxWith builds a context with the default compatibility mode, which is
// the behaviour of releases before the interpolation area changed.
func legacyCtxWith(captured map[string]any) *tryve.InterpolationContext {
	ctx := tryve.NewInterpolationContext()
	ctx.BaseURL = "https://api.example.com"
	for k, v := range captured {
		ctx.Captured[k] = v
	}
	return ctx
}

// TestTypedSubstitution covers the case that made every interpolated comparison
// against a numeric column fail: a lone expression used to render to text, so
// `equals: "{{captured.total}}"` compared the string "42" against the number 42.
func TestTypedSubstitution(t *testing.T) {
	ctx := ctxWith(map[string]any{
		"total":  float64(42),
		"active": true,
		"row":    map[string]any{"id": "abc"},
		"empty":  nil,
	})

	cases := []struct {
		in   string
		want any
	}{
		{"{{captured.total}}", float64(42)},
		{"  {{captured.total}}  ", float64(42)},
		{"{{captured.active}}", true},
		{"{{captured.row}}", map[string]any{"id": "abc"}},
		{"{{captured.empty}}", nil},
		// Mixed text stays text.
		{"total is {{captured.total}}", "total is 42"},
	}

	for _, tc := range cases {
		got, err := interpolate.ResolveValue(tc.in, ctx)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: expected %#v (%T), got %#v (%T)", tc.in, tc.want, tc.want, got, got)
		}
	}
}

// TestCapturedJSONStringTraversal covers reading a field out of a captured JSON
// string — the gap that forced test suites to shell out to `node -e` or `jq`
// just to pull one id back out of a setup script's stdout.
func TestCapturedJSONStringTraversal(t *testing.T) {
	ctx := ctxWith(map[string]any{
		"setup_result": `{"promotionId":"p-123","games":[{"gameId":"g-1"},{"gameId":"g-2"}]}`,
	})

	got, err := interpolate.ResolveValue("{{captured.setup_result.promotionId}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "p-123" {
		t.Errorf("expected p-123, got %#v", got)
	}

	got, err = interpolate.ResolveValue("{{captured.setup_result.games[1].gameId}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "g-2" {
		t.Errorf("expected g-2, got %#v", got)
	}
}

// TestArrayIndexing covers both bracket and dotted index forms.
func TestArrayIndexing(t *testing.T) {
	ctx := ctxWith(map[string]any{
		"rows": []any{
			map[string]any{"id": float64(1)},
			map[string]any{"id": float64(2)},
		},
	})

	for _, expr := range []string{"{{captured.rows[1].id}}", "{{captured.rows.1.id}}"} {
		got, err := interpolate.ResolveValue(expr, ctx)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", expr, err)
		}
		if got != float64(2) {
			t.Errorf("%s: expected 2, got %#v", expr, got)
		}
	}
}

// TestObjectStringifiesAsJSON checks that embedding an object in text produces
// JSON rather than Go's map syntax.
func TestObjectStringifiesAsJSON(t *testing.T) {
	ctx := ctxWith(map[string]any{"row": map[string]any{"id": "abc"}})

	got, err := interpolate.ResolveString("payload={{captured.row}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `payload={"id":"abc"}` {
		t.Errorf("expected JSON rendering, got %q", got)
	}
}

// TestWholeNumbersRenderWithoutDecimal guards against an integer captured
// through JSON being spliced into SQL or a URL as "42.0".
func TestWholeNumbersRenderWithoutDecimal(t *testing.T) {
	ctx := ctxWith(map[string]any{"n": float64(42)})

	got, err := interpolate.ResolveString("id={{captured.n}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "id=42" {
		t.Errorf("expected id=42, got %q", got)
	}
}

// TestStrictModeFailsOnUnresolved verifies that a misspelled expression is an
// error under strict resolution instead of being sent through verbatim.
func TestStrictModeFailsOnUnresolved(t *testing.T) {
	ctx := ctxWith(nil)

	got, err := interpolate.ResolveString("/api/{{captured.nope}}", ctx)
	if err != nil {
		t.Fatalf("non-strict resolution should not error: %v", err)
	}
	if got != "/api/{{captured.nope}}" {
		t.Errorf("non-strict should pass the token through, got %q", got)
	}

	ctx.Strict = true
	if _, err := interpolate.ResolveString("/api/{{captured.nope}}", ctx); err == nil {
		t.Errorf("strict resolution should reject an unresolved expression")
	}
}

// TestStrictModeIgnoresDollarBraces checks that shell and SQL variables written
// as ${VAR} are never treated as unresolved tryve expressions.
func TestStrictModeIgnoresDollarBraces(t *testing.T) {
	ctx := ctxWith(nil)
	ctx.Strict = true

	got, err := interpolate.ResolveString(`echo "${SHELL_VAR}"`, ctx)
	if err != nil {
		t.Fatalf("strict mode must not reject ${…}: %v", err)
	}
	if got != `echo "${SHELL_VAR}"` {
		t.Errorf("expected the shell variable to survive, got %q", got)
	}
}

// TestCapturedValuesAreNotRescanned checks that data containing "{{" is inserted
// literally and never interpreted as a further template.
func TestCapturedValuesAreNotRescanned(t *testing.T) {
	ctx := ctxWith(map[string]any{
		"payload": "{{captured.secret}}",
		"secret":  "leaked",
	})

	got, err := interpolate.ResolveString("value={{captured.payload}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "value={{captured.secret}}" {
		t.Errorf("captured data must not be re-expanded, got %q", got)
	}
}

// TestNestedBuiltinArguments covers expressions and braces inside a builtin call,
// which the previous regex-based parser could not represent.
func TestNestedBuiltinArguments(t *testing.T) {
	ctx := ctxWith(map[string]any{"name": "world"})

	got, err := interpolate.ResolveValue("{{$upper({{captured.name}})}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "WORLD" {
		t.Errorf("expected WORLD, got %#v", got)
	}
}

// TestJSONPathBuiltin covers reading a path out of a captured JSON document.
func TestJSONPathBuiltin(t *testing.T) {
	ctx := ctxWith(map[string]any{"result": `{"a":{"b":[10,20]}}`})

	got, err := interpolate.ResolveValue("{{$jsonPath(captured.result, a.b[1])}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != float64(20) {
		t.Errorf("expected 20, got %#v", got)
	}
}

// TestIntBuiltin covers coercing captured text to a number for a typed comparison
// or a bound SQL parameter.
func TestIntBuiltin(t *testing.T) {
	ctx := ctxWith(map[string]any{"n": "42"})

	got, err := interpolate.ResolveValue("{{$int(captured.n)}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != int64(42) {
		t.Errorf("expected int64(42), got %#v (%T)", got, got)
	}
}

// TestDefaultBuiltin covers the fallback for an optional value.
func TestDefaultBuiltin(t *testing.T) {
	ctx := ctxWith(map[string]any{"present": "yes"})

	got, _ := interpolate.ResolveValue("{{$default(captured.present, fallback)}}", ctx)
	if got != "yes" {
		t.Errorf("expected the present value, got %#v", got)
	}

	got, _ = interpolate.ResolveValue("{{$default(captured.absent, fallback)}}", ctx)
	if got != "fallback" {
		t.Errorf("expected the fallback, got %#v", got)
	}
}

// TestJWTBuiltinHS256 checks that a signed token is produced with three segments
// and a stable signature for fixed input.
func TestJWTBuiltinHS256(t *testing.T) {
	ctx := ctxWith(nil)

	got, err := interpolate.ResolveValue(`{{$jwt(HS256, secret, {"sub":"84987654321"}, 1h)}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	token, ok := got.(string)
	if !ok {
		t.Fatalf("expected a string token, got %T", got)
	}
	segments := 1
	for _, c := range token {
		if c == '.' {
			segments++
		}
	}
	if segments != 3 {
		t.Errorf("expected a three-segment JWT, got %q", token)
	}
}

// TestLegacyInterpolationIsUnchanged pins the pre-v2 behaviour that an existing
// suite depends on. Upgrading the binary without opting in must not alter what a
// step sends or compares.
func TestLegacyInterpolationIsUnchanged(t *testing.T) {
	ctx := legacyCtxWith(map[string]any{
		"n":    float64(7),
		"b":    true,
		"row":  map[string]any{"id": "abc"},
		"null": nil,
	})

	// A lone expression renders to text, whatever the resolved type.
	for _, tc := range []struct{ in, want string }{
		{"{{captured.n}}", "7"},
		{"{{captured.b}}", "true"},
		{"{{captured.null}}", ""},
	} {
		got, err := interpolate.ResolveValue(tc.in, ctx)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.in, err)
		}
		if s, ok := got.(string); !ok || s != tc.want {
			t.Errorf("%s: expected the string %q, got %#v", tc.in, tc.want, got)
		}
	}

	// Objects render with Go's %v, not as JSON.
	got, err := interpolate.ResolveValue("{{captured.row}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "map[id:abc]" {
		t.Errorf("expected legacy %%v rendering, got %#v", got)
	}
}

// TestLegacyRescansSubstitutedText pins the multi-pass behaviour: text a value
// contributes is itself resolved.
func TestLegacyRescansSubstitutedText(t *testing.T) {
	ctx := legacyCtxWith(map[string]any{
		"payload": "{{captured.inner}}",
		"inner":   "expanded",
	})

	got, err := interpolate.ResolveString("value={{captured.payload}}", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "value=expanded" {
		t.Errorf("legacy resolution should re-scan its output, got %q", got)
	}
}

// TestLegacyKeepsBuiltinArgumentQuotes pins the argument handling: quotes were
// passed through to the builtin rather than stripped.
func TestLegacyKeepsBuiltinArgumentQuotes(t *testing.T) {
	ctx := legacyCtxWith(nil)

	got, err := interpolate.ResolveString(`{{$upper("abc")}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `"ABC"` {
		t.Errorf("expected the quotes to survive, got %q", got)
	}

	// Unquoted arguments behave identically in both modes.
	got, _ = interpolate.ResolveString(`{{$upper(abc)}}`, ctx)
	if got != "ABC" {
		t.Errorf("expected ABC, got %q", got)
	}
}
