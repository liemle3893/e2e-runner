package interpolate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// ResolveString interpolates a template string and returns the result as text.
//
// Substitution is single-pass: text that a resolved value contributes is never
// re-scanned for further expressions. A value that arrives from an API response
// or a database column may legitimately contain "{{" and must not be treated as
// a template. Expressions nested inside another expression are still handled —
// they are parsed as part of the expression, not by re-scanning output.
func ResolveString(s string, ctx *tryve.InterpolationContext) (string, error) {
	if ctx != nil && ctx.Compat.Modern(tryve.CompatInterpolation) {
		return resolveStringDepth(s, ctx, 0)
	}

	// Legacy resolution re-scans its own output until it stabilises, so text
	// contributed by a resolved value is itself treated as a template. That is
	// how earlier releases behaved; the single-pass rule is opt-in because it
	// changes what a response containing "{{" does.
	cur := s
	for i := 0; i < maxVarDepth; i++ {
		next, err := resolveStringDepth(cur, ctx, 0)
		if err != nil {
			return "", err
		}
		if next == cur {
			break
		}
		cur = next
	}
	return cur, nil
}

// maxVarDepth bounds how far a variable whose value is itself a template may be
// expanded, so a pair of variables referring to each other cannot loop forever.
const maxVarDepth = 10

func resolveStringDepth(s string, ctx *tryve.InterpolationContext, depth int) (string, error) {
	if !strings.ContainsAny(s, "{$") {
		return s, nil
	}

	var out strings.Builder
	var unresolved []string

	for _, tok := range tokenize(s) {
		if !tok.isExpr {
			out.WriteString(tok.text)
			continue
		}
		val, err := evalExpressionDepth(tok.expr, ctx, depth)
		if err != nil {
			if !isUnresolved(err) {
				return "", err
			}
			// Leave the original token in place so shell and SQL text that merely
			// looks like a template survives untouched.
			out.WriteString(tok.raw)
			if !tok.dollar {
				unresolved = append(unresolved, tok.raw)
			}
			continue
		}
		out.WriteString(stringifyFor(ctx, val))
	}

	if ctx.Strict && len(unresolved) > 0 {
		return "", tryve.InterpolationError(s, formatUnresolved(unresolved))
	}
	return out.String(), nil
}

// ResolveValue interpolates a value of any type, preserving the resolved type
// where it can.
//
// When a string consists of exactly one expression — "{{captured.total}}" — the
// resolved value is returned as-is: a number stays a number, an object stays an
// object, null stays null. Only a string that mixes literal text with
// expressions is rendered to text.
//
// This is what makes `equals: "{{captured.total}}"` compare against a numeric
// column, and what lets a captured object be passed straight into a request body
// or bound as a typed SQL parameter.
func ResolveValue(v any, ctx *tryve.InterpolationContext) (any, error) {
	switch typed := v.(type) {
	case string:
		// Under legacy interpolation every resolved value renders to text, so a
		// captured number reaches a request body or a SQL parameter as a string
		// exactly as it did before typed substitution existed.
		if !ctx.Compat.Modern(tryve.CompatInterpolation) {
			return ResolveString(typed, ctx)
		}
		if expr, ok := soleExpression(typed); ok {
			val, err := evalExpressionDepth(expr, ctx, 0)
			if err == nil {
				return val, nil
			}
			if !isUnresolved(err) {
				return nil, err
			}
			if ctx.Strict && !strings.HasPrefix(strings.TrimSpace(typed), "${") {
				return nil, tryve.InterpolationError(typed, formatUnresolved([]string{typed}))
			}
			return typed, nil
		}
		return ResolveString(typed, ctx)
	case map[string]any:
		return ResolveMap(typed, ctx)
	case []any:
		return ResolveSlice(typed, ctx)
	default:
		return v, nil
	}
}

// soleExpression reports whether s is a single expression with no surrounding
// text, returning the expression source when it is.
func soleExpression(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	tokens := tokenize(trimmed)
	if len(tokens) != 1 || !tokens[0].isExpr {
		return "", false
	}
	return tokens[0].expr, true
}

// ResolveMap interpolates every value in a map recursively.
func ResolveMap(m map[string]any, ctx *tryve.InterpolationContext) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for k, v := range m {
		resolved, err := ResolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		result[k] = resolved
	}
	return result, nil
}

// ResolveSlice interpolates every element in a slice recursively.
func ResolveSlice(s []any, ctx *tryve.InterpolationContext) ([]any, error) {
	result := make([]any, len(s))
	for i, v := range s {
		resolved, err := ResolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		result[i] = resolved
	}
	return result, nil
}

// evalExpression resolves one expression to its value, using the priority:
//
//  1. Built-in function call (expression starts with $)
//  2. The literal "baseUrl"
//  3. "captured.<path>" — a value captured by an earlier step
//  4. A variable, by dotted path
//  5. An environment value
//
// A nested "{{…}}" inside the expression is resolved first, so builtin arguments
// may themselves be expressions.
func evalExpression(expr string, ctx *tryve.InterpolationContext) (any, error) {
	return evalExpressionDepth(expr, ctx, 0)
}

func evalExpressionDepth(expr string, ctx *tryve.InterpolationContext, depth int) (any, error) {
	expr = strings.TrimSpace(expr)

	// Resolve any nested expressions before interpreting this one.
	if strings.Contains(expr, "{{") {
		inner, err := resolveStringDepth(expr, ctx, depth)
		if err != nil {
			return nil, err
		}
		expr = strings.TrimSpace(inner)
	}

	// 1. Built-in function.
	if strings.HasPrefix(expr, "$") {
		unquote := ctx != nil && ctx.Compat.Modern(tryve.CompatInterpolation)
		name, args, ok := parseBuiltinCall(expr, unquote)
		if !ok {
			return nil, errUnresolved(expr)
		}
		return CallBuiltinValue(name, ctx, args...)
	}

	// 2. Base URL.
	if expr == "baseUrl" {
		return ctx.BaseURL, nil
	}

	// 3. Captured values.
	if rest, ok := strings.CutPrefix(expr, "captured."); ok {
		val, found := Lookup(ctx.Captured, rest)
		if !found {
			return nil, errUnresolved(expr)
		}
		return val, nil
	}

	// 4. Variables.
	//
	// A variable's value may itself be a template ("greeting: hello {{name}}"),
	// so it is expanded further. This applies to variables only: values captured
	// from a response or read from the environment are substituted verbatim, so
	// data that happens to contain "{{" is never interpreted as a template.
	if ctx.Variables != nil {
		if val, found := Lookup(ctx.Variables, expr); found {
			if tmpl, isString := val.(string); isString && depth < maxVarDepth && strings.Contains(tmpl, "{{") {
				return resolveStringDepth(tmpl, ctx, depth+1)
			}
			return val, nil
		}
	}

	// 5. Environment.
	if ctx.Env != nil {
		if val, ok := ctx.Env[expr]; ok {
			return val, nil
		}
	}

	return nil, errUnresolved(expr)
}

// unresolvedError signals that an expression named nothing the context knows.
type unresolvedError struct{ expr string }

func (e unresolvedError) Error() string { return "unresolved: " + e.expr }

// errUnresolved constructs an unresolvedError for the given expression.
func errUnresolved(expr string) error { return unresolvedError{expr: expr} }

// isUnresolved reports whether err is an unresolvedError.
func isUnresolved(err error) bool {
	_, ok := err.(unresolvedError)
	return ok
}

// stringifyFor renders a value as text using the context's compatibility mode.
//
// Legacy rendering is Go's %v, which prints an object as `map[id:5]`. That is
// neither valid JSON nor valid SQL, but it is what earlier releases produced, so
// it is preserved for suites that have not opted in.
func stringifyFor(ctx *tryve.InterpolationContext, v any) string {
	if ctx != nil && ctx.Compat.Modern(tryve.CompatInterpolation) {
		return Stringify(v)
	}
	return legacyStringify(v)
}

// legacyStringify reproduces the pre-v2 rendering: nil becomes empty, everything
// else goes through %v.
func legacyStringify(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

// Stringify renders a resolved value as text for embedding in a template.
//
// Maps and slices are rendered as JSON rather than with Go's %v formatting,
// which would produce `map[id:5]` — text that is neither valid JSON nor valid
// SQL, and that silently corrupts whatever it is spliced into.
func Stringify(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	case map[string]any, []any:
		if encoded, err := json.Marshal(typed); err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("%v", typed)
	case float64:
		// Render whole numbers without a trailing ".0" so an integer captured
		// through JSON splices into SQL and URLs as an integer.
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%v", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// ResolveVariables resolves a map of variables in dependency order so that a
// variable referencing another resolves correctly.
// Returns an error when a circular dependency is detected.
func ResolveVariables(vars map[string]any, ctx *tryve.InterpolationContext) (map[string]any, error) {
	// Build dependency graph: varName → the variable names it depends on.
	deps := make(map[string][]string, len(vars))
	for name, val := range vars {
		strVal, ok := val.(string)
		if !ok {
			deps[name] = nil
			continue
		}
		deps[name] = findVarDeps(strVal, vars)
	}

	order, err := topoSort(deps)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]any, len(vars))

	// Work on a copy so the caller's Variables map is not mutated.
	workCtx := &tryve.InterpolationContext{
		Variables: shallowCopyMap(ctx.Variables),
		Captured:  ctx.Captured,
		BaseURL:   ctx.BaseURL,
		Env:       ctx.Env,
		Strict:    ctx.Strict,
	}

	for _, name := range order {
		val := vars[name]
		strVal, ok := val.(string)
		if !ok {
			resolved[name] = val
			workCtx.Variables[name] = val
			continue
		}
		r, err := ResolveValue(strVal, workCtx)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", name, err)
		}
		resolved[name] = r
		workCtx.Variables[name] = r
	}

	return resolved, nil
}

// findVarDeps extracts variable names referenced inside s that match keys in vars.
// Built-in ($…), baseUrl, and captured.* references are not variable dependencies.
func findVarDeps(s string, vars map[string]any) []string {
	seen := make(map[string]struct{})
	var deps []string

	for _, tok := range tokenize(s) {
		// Only {{…}} references cross-reference other variables; ${…} is an
		// environment reference resolved by the config loader.
		if !tok.isExpr || tok.dollar {
			continue
		}
		expr := strings.TrimSpace(tok.expr)
		if strings.HasPrefix(expr, "$") || expr == "baseUrl" || strings.HasPrefix(expr, "captured.") {
			continue
		}
		root := strings.SplitN(strings.SplitN(expr, ".", 2)[0], "[", 2)[0]
		if _, isVar := vars[root]; !isVar {
			continue
		}
		if _, already := seen[root]; already {
			continue
		}
		seen[root] = struct{}{}
		deps = append(deps, root)
	}

	return deps
}

// topoSort performs Kahn's algorithm on the dependency graph.
// deps[node] lists the nodes that node depends on (its prerequisites).
// Returns keys in dependency-first order, or an error on cycle detection.
func topoSort(deps map[string][]string) ([]string, error) {
	adj := make(map[string][]string, len(deps))
	nodeInDegree := make(map[string]int, len(deps))
	for node := range deps {
		nodeInDegree[node] = 0 // ensure every node appears
	}
	for node, nodeDeps := range deps {
		for _, dep := range nodeDeps {
			adj[dep] = append(adj[dep], node)
			nodeInDegree[node]++
		}
	}

	var queue []string
	for node, deg := range nodeInDegree {
		if deg == 0 {
			queue = append(queue, node)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, dependent := range adj[node] {
			nodeInDegree[dependent]--
			if nodeInDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(deps) {
		return nil, fmt.Errorf("circular dependency detected in variables")
	}

	return order, nil
}

// shallowCopyMap returns a shallow copy of a map[string]any.
func shallowCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return make(map[string]any)
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
