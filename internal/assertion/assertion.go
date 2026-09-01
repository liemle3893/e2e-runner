package assertion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// knownHTTPKeys lists top-level keys that receive special handling in map-format assertions.
// Any other key at the top level is treated as a direct operator.
var knownHTTPKeys = map[string]bool{
	"status":      true,
	"statusRange": true,
	"headers":     true,
	"json":        true,
	"body":        true,
	"duration":    true,
}

// structuralKeys are slice-item keys that select what to assert against rather
// than naming an operator.
var structuralKeys = map[string]bool{
	"path":   true,
	"row":    true,
	"column": true,
}

// legacyOperators is the operator set recognised before the assertions area
// changed. Names outside it — the aliases and the operators added since — were
// silently ignored, so a suite on v1 compatibility must keep ignoring them
// rather than start failing on an operator it never evaluated.
var legacyOperators = map[string]bool{
	"equals":             true,
	"notEquals":          true,
	"contains":           true,
	"notContains":        true,
	"matches":            true,
	"type":               true,
	"exists":             true,
	"notExists":          true,
	"isNull":             true,
	"isNotNull":          true,
	"greaterThan":        true,
	"lessThan":           true,
	"greaterThanOrEqual": true,
	"lessThanOrEqual":    true,
	"length":             true,
	"isEmpty":            true,
	"notEmpty":           true,
	"hasProperty":        true,
	"notHasProperty":     true,
}

// isOperator reports whether key names an assertion operator or one of its aliases.
func isOperator(key string) bool {
	_, ok := CanonicalOperator(key)
	return ok
}

// RunAssertions evaluates assertDef against data and returns one AssertionOutcome per check.
//
// assertDef may be:
//   - nil — returns empty outcomes
//   - map[string]any — HTTP-style with keys: status, statusRange, headers, json, body, duration,
//     or a direct {path, operator: value} block
//   - []any — generic slice of {path, operator: value} items
func RunAssertions(data map[string]any, assertDef any, mode tryve.CompatMode) ([]tryve.AssertionOutcome, error) {
	if assertDef == nil {
		return nil, nil
	}

	switch def := assertDef.(type) {
	case map[string]any:
		return runMapAssertions(data, def, mode)
	case []any:
		return runSliceAssertions(data, def, mode)
	default:
		return nil, fmt.Errorf("unsupported assertDef type %T", assertDef)
	}
}

// runMapAssertions handles the HTTP-style map format.
func runMapAssertions(data map[string]any, def map[string]any, mode tryve.CompatMode) ([]tryve.AssertionOutcome, error) {
	modern := mode.Modern(tryve.CompatAssertions)
	var outcomes []tryve.AssertionOutcome

	// status — single number or []any oneOf check.
	if statusDef, ok := def["status"]; ok {
		actual := data["status"]
		switch sv := statusDef.(type) {
		case []any:
			// oneOf check — actual must equal one of the values in the array.
			o := assertOneOf("status", actual, sv)
			outcomes = append(outcomes, o)
		default:
			// Single-value equals check.
			r := Match("equals", actual, statusDef)
			outcomes = append(outcomes, tryve.AssertionOutcome{
				Path:     "status",
				Operator: "equals",
				Expected: statusDef,
				Actual:   actual,
				Passed:   r.Pass,
				Message:  r.Message,
			})
		}
	}

	// statusRange — [min, max] inclusive.
	if rangeDef, ok := def["statusRange"]; ok {
		outcomes = append(outcomes, assertStatusRange(data["status"], rangeDef))
	}

	// headers — map of name→expected value with case-insensitive lookup.
	if headersDef, ok := def["headers"]; ok {
		if hm, ok := headersDef.(map[string]any); ok {
			actualHeaders, _ := data["headers"].(map[string]any)
			for wantName, wantVal := range hm {
				actual := headerLookup(actualHeaders, wantName)
				r := Match("equals", actual, wantVal)
				outcomes = append(outcomes, tryve.AssertionOutcome{
					Path:     "headers." + wantName,
					Operator: "equals",
					Expected: wantVal,
					Actual:   actual,
					Passed:   r.Pass,
					Message:  r.Message,
				})
			}
		}
	}

	// json — []any of {path, operator: value} items.
	if jsonDef, ok := def["json"]; ok {
		if items, ok := jsonDef.([]any); ok {
			// JSON path assertions evaluate against the response body, not the
			// full adapter data, so "$.data.id" means the body's data.id.
			//
			// The body may be an array as legitimately as an object; treating
			// only objects as the root made "$" and "$[0]" silently address the
			// enclosing result instead, so those assertions checked the wrong
			// value rather than failing.
			outs, err := runSliceAssertions(jsonRoot(data, modern), items, mode)
			if err != nil {
				return outcomes, err
			}
			outcomes = append(outcomes, outs...)
		}
	}

	// body — map of {contains/matches/equals: value}.
	if bodyDef, ok := def["body"]; ok {
		if bm, ok := bodyDef.(map[string]any); ok {
			bodyStr := fmt.Sprintf("%v", data["body"])
			for op, val := range bm {
				r := Match(op, bodyStr, val)
				outcomes = append(outcomes, tryve.AssertionOutcome{
					Path:     "body",
					Operator: op,
					Expected: val,
					Actual:   bodyStr,
					Passed:   r.Pass,
					Message:  r.Message,
				})
			}
		}
	}

	// duration — map of {lessThan/greaterThan: value}.
	if durationDef, ok := def["duration"]; ok {
		if dm, ok := durationDef.(map[string]any); ok {
			actual := data["duration"]
			for op, val := range dm {
				r := Match(op, actual, val)
				outcomes = append(outcomes, tryve.AssertionOutcome{
					Path:     "duration",
					Operator: op,
					Expected: val,
					Actual:   actual,
					Passed:   r.Pass,
					Message:  r.Message,
				})
			}
		}
	}

	// Direct operator format — top-level map has a "path" key and one or more operator keys.
	if path, hasPath := def["path"]; hasPath {
		pathStr, _ := path.(string)
		actual, _ := EvalJSONPath(data, pathStr)
		outcomes = append(outcomes, applyOperators(pathStr, actual, def, structuralKeys, modern)...)
		return outcomes, nil
	}

	// Remaining top-level keys are either operators applied to the whole result
	// (rare) or the name of a field in the adapter's result data.
	//
	// The field form is what makes `exitCode: 0`, `stdout: {contains: …}` and
	// `rowCount: {gte: 1}` work. Before it existed those keys matched nothing and
	// were dropped without a word, so the assertion never ran and the step passed.
	for _, key := range sortedKeys(def) {
		if knownHTTPKeys[key] {
			continue
		}
		val := def[key]

		if isOperator(key) && (modern || legacyOperators[key]) {
			r := Match(key, data, val)
			outcomes = append(outcomes, tryve.AssertionOutcome{
				Path:     "$",
				Operator: key,
				Expected: val,
				Actual:   data,
				Passed:   r.Pass,
				Message:  r.Message,
			})
			continue
		}

		// Field assertions did not exist before the assertions area changed: an
		// unrecognised key was dropped, and the step passed without the check.
		if !modern {
			continue
		}

		// Field assertion: look the key up in the result data.
		actual, _ := EvalJSONPath(data, key)
		if opMap, ok := val.(map[string]any); ok && looksLikeOperatorMap(opMap) {
			outcomes = append(outcomes, applyOperators(key, actual, opMap, nil, modern)...)
			continue
		}
		r := Match("equals", actual, val)
		outcomes = append(outcomes, tryve.AssertionOutcome{
			Path:     key,
			Operator: "equals",
			Expected: val,
			Actual:   actual,
			Passed:   r.Pass,
			Message:  r.Message,
		})
	}

	return outcomes, nil
}

// looksLikeOperatorMap reports whether every key in m names an operator, which
// distinguishes `stdout: {contains: "x"}` from an equality check against a
// literal object value.
func looksLikeOperatorMap(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}
	for k := range m {
		if !isOperator(k) {
			return false
		}
	}
	return true
}

// applyOperators evaluates every operator key in def against actual, producing
// one outcome per operator. Keys listed in skip are ignored; any remaining key
// that is not a recognised operator yields a FAILING outcome rather than being
// dropped, so a typo surfaces as a red test instead of a silent pass.
func applyOperators(path string, actual any, def map[string]any, skip map[string]bool, modern bool) []tryve.AssertionOutcome {
	var outcomes []tryve.AssertionOutcome
	for _, key := range sortedKeys(def) {
		if skip[key] {
			continue
		}
		val := def[key]

		// Legacy behaviour recognised a fixed operator set and silently ignored
		// everything else, including the aliases and the operators added since.
		if !modern {
			if legacyOperators[key] {
				r := Match(key, actual, val)
				outcomes = append(outcomes, tryve.AssertionOutcome{
					Path: path, Operator: key, Expected: val, Actual: actual,
					Passed: r.Pass, Message: r.Message,
				})
			}
			continue
		}

		if !isOperator(key) {
			outcomes = append(outcomes, tryve.AssertionOutcome{
				Path:     path,
				Operator: key,
				Expected: val,
				Actual:   actual,
				Passed:   false,
				Message: fmt.Sprintf("unknown assertion operator %q; valid operators are: %s",
					key, strings.Join(KnownOperators(), ", ")),
			})
			continue
		}
		r := Match(key, actual, val)
		outcomes = append(outcomes, tryve.AssertionOutcome{
			Path:     path,
			Operator: key,
			Expected: val,
			Actual:   actual,
			Passed:   r.Pass,
			Message:  r.Message,
		})
	}
	return outcomes
}

// sortedKeys returns the keys of m in a stable order so assertion outcomes are
// reported deterministically across runs.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// jsonRoot returns the value that "$" refers to for an HTTP-style assertion:
// the parsed response body when there is one, and otherwise the whole result.
func jsonRoot(data map[string]any, modern bool) any {
	body, ok := data["body"]
	if !ok || body == nil {
		return data
	}
	if m, isMap := body.(map[string]any); isMap {
		return m
	}
	// An array body only became the root when the assertions area changed;
	// before that "$" addressed the whole result for anything but an object.
	if _, isSlice := body.([]any); isSlice && modern {
		return body
	}
	// A scalar or unparsed string body is addressed as "body", not as the root.
	return data
}

// runSliceAssertions handles the generic []any format. Each item selects a value
// to assert against — either with "path" (a JSONPath expression) or with
// "row"/"column" (a cell in a SQL result set) — plus one or more operator keys.
func runSliceAssertions(data any, items []any, mode tryve.CompatMode) ([]tryve.AssertionOutcome, error) {
	modern := mode.Modern(tryve.CompatAssertions)

	var outcomes []tryve.AssertionOutcome
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Before the assertions area changed, a slice item without "path" matched
		// no handler and every operator on it was dropped.
		if _, hasPath := m["path"]; !hasPath && !modern {
			continue
		}

		// row/column form: address a cell in a query result.
		if _, hasColumn := m["column"]; hasColumn && modern {
			path, actual, err := resolveCell(data, m)
			if err != nil {
				outcomes = append(outcomes, tryve.AssertionOutcome{
					Path:     path,
					Operator: "row/column",
					Actual:   nil,
					Passed:   false,
					Message:  err.Error(),
				})
				continue
			}
			outcomes = append(outcomes, applyOperators(path, actual, m, structuralKeys, modern)...)
			continue
		}

		pathStr, _ := m["path"].(string)
		actual, _ := EvalJSONPath(data, pathStr)
		outcomes = append(outcomes, applyOperators(pathStr, actual, m, structuralKeys, modern)...)
	}
	return outcomes, nil
}

// resolveCell locates the value addressed by an item's "row" (default 0) and
// "column" keys within a SQL adapter result.
//
// It handles both result shapes the postgresql adapter produces: the "query"
// shape, where rows live under data["rows"], and the "queryOne" shape, where the
// single row's columns sit at the top level of data.
func resolveCell(root any, m map[string]any) (string, any, error) {
	data, ok := root.(map[string]any)
	if !ok {
		return "column", nil, fmt.Errorf(
			"row/column assertions need an object result, got %T", root)
	}
	column, ok := m["column"].(string)
	if !ok {
		return "column", nil, fmt.Errorf("assertion \"column\" must be a string, got %T", m["column"])
	}

	rowIdx := 0
	if rv, present := m["row"]; present {
		rowIdx = int(toFloat64(rv))
	}
	path := fmt.Sprintf("rows[%d].%s", rowIdx, column)

	rowsVal, hasRows := data["rows"]
	if !hasRows {
		// queryOne shape: the row's columns are the top-level data map.
		if rowIdx != 0 {
			return path, nil, fmt.Errorf("row %d requested but the result holds a single row", rowIdx)
		}
		val, found := data[column]
		if !found {
			return path, nil, fmt.Errorf("column %q not present in result; available columns: %s",
				column, strings.Join(sortedKeys(data), ", "))
		}
		return path, val, nil
	}

	rows, ok := rowsVal.([]any)
	if !ok {
		return path, nil, fmt.Errorf("result \"rows\" is %T, not an array", rowsVal)
	}
	if rowIdx < 0 || rowIdx >= len(rows) {
		return path, nil, fmt.Errorf("row %d is out of range; the result has %d row(s)", rowIdx, len(rows))
	}
	row, ok := rows[rowIdx].(map[string]any)
	if !ok {
		return path, nil, fmt.Errorf("row %d is %T, not an object", rowIdx, rows[rowIdx])
	}
	val, found := row[column]
	if !found {
		return path, nil, fmt.Errorf("column %q not present in row %d; available columns: %s",
			column, rowIdx, strings.Join(sortedKeys(row), ", "))
	}
	return path, val, nil
}

// assertOneOf checks that actual equals one value in the allowed slice.
func assertOneOf(path string, actual any, allowed []any) tryve.AssertionOutcome {
	normalActual := normalizeNumeric(actual)
	for _, v := range allowed {
		if fmt.Sprintf("%v", normalizeNumeric(v)) == fmt.Sprintf("%v", normalActual) {
			return tryve.AssertionOutcome{
				Path:     path,
				Operator: "oneOf",
				Expected: allowed,
				Actual:   actual,
				Passed:   true,
			}
		}
	}
	return tryve.AssertionOutcome{
		Path:     path,
		Operator: "oneOf",
		Expected: allowed,
		Actual:   actual,
		Passed:   false,
		Message:  fmt.Sprintf("expected %v to be one of %v", actual, allowed),
	}
}

// assertStatusRange checks that actual status is within [min, max] inclusive.
func assertStatusRange(actual any, rangeDef any) tryve.AssertionOutcome {
	arr, ok := rangeDef.([]any)
	if !ok || len(arr) < 2 {
		return tryve.AssertionOutcome{
			Path:     "statusRange",
			Operator: "statusRange",
			Expected: rangeDef,
			Actual:   actual,
			Passed:   false,
			Message:  "statusRange must be an array with [min, max]",
		}
	}
	min := toFloat64(arr[0])
	max := toFloat64(arr[1])
	val := toFloat64(actual)
	if val >= min && val <= max {
		return tryve.AssertionOutcome{
			Path:     "statusRange",
			Operator: "statusRange",
			Expected: rangeDef,
			Actual:   actual,
			Passed:   true,
		}
	}
	return tryve.AssertionOutcome{
		Path:     "statusRange",
		Operator: "statusRange",
		Expected: rangeDef,
		Actual:   actual,
		Passed:   false,
		Message:  fmt.Sprintf("status %v is not in range [%v, %v]", actual, arr[0], arr[1]),
	}
}

// headerLookup performs a case-insensitive key lookup in a headers map.
// Returns nil when the header is not present.
func headerLookup(headers map[string]any, name string) any {
	if headers == nil {
		return nil
	}
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) == lower {
			return v
		}
	}
	return nil
}
