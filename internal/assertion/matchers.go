package assertion

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MatchResult holds the outcome of a single matcher evaluation.
type MatchResult struct {
	Pass    bool
	Message string
}

// operatorAliases maps shorthand operator spellings to their canonical names.
// Aliases exist because these spellings appear widely in hand-written test
// suites; before they were recognised, an assertion using one was silently
// dropped instead of evaluated.
var operatorAliases = map[string]string{
	"eq":           "equals",
	"ne":           "notEquals",
	"neq":          "notEquals",
	"gt":           "greaterThan",
	"gte":          "greaterThanOrEqual",
	"lt":           "lessThan",
	"lte":          "lessThanOrEqual",
	"lengthEquals": "length",
	"in":           "oneOf",
	"notIn":        "notOneOf",
	"isEmpty":      "isEmpty",
	"empty":        "isEmpty",
}

// canonicalOperators is the set of operator names Match understands directly.
var canonicalOperators = map[string]bool{
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
	"minLength":          true,
	"maxLength":          true,
	"isEmpty":            true,
	"notEmpty":           true,
	"hasProperty":        true,
	"notHasProperty":     true,
	"startsWith":         true,
	"endsWith":           true,
	"oneOf":              true,
	"notOneOf":           true,
}

// CanonicalOperator resolves an operator name or alias to its canonical form.
// The second return value reports whether the name is a recognised operator.
func CanonicalOperator(name string) (string, bool) {
	if canonicalOperators[name] {
		return name, true
	}
	if canon, ok := operatorAliases[name]; ok {
		return canon, true
	}
	return name, false
}

// KnownOperators returns a sorted list of every operator name and alias, for
// use in error messages that need to tell the author what is available.
func KnownOperators() []string {
	names := make([]string, 0, len(canonicalOperators)+len(operatorAliases))
	for n := range canonicalOperators {
		names = append(names, n)
	}
	for n := range operatorAliases {
		if !canonicalOperators[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// Match dispatches to the appropriate matcher by operator name.
// It returns a MatchResult indicating whether the assertion passed and a human-readable message.
// Operator aliases (gte, lt, in, …) are resolved to their canonical names first.
func Match(operator string, actual, expected any) MatchResult {
	operator, _ = CanonicalOperator(operator)

	switch operator {
	case "equals":
		return matchEquals(actual, expected)
	case "notEquals":
		return invert(matchEquals(actual, expected), "notEquals")
	case "contains":
		return matchContains(actual, expected)
	case "notContains":
		return invert(matchContains(actual, expected), "notContains")
	case "matches":
		return matchRegex(actual, expected)
	case "type":
		return matchType(actual, expected)
	case "exists":
		return matchExists(actual, expected)
	case "notExists":
		return matchNotExists(actual)
	case "isNull":
		return matchIsNull(actual)
	case "isNotNull":
		return invert(matchIsNull(actual), "isNotNull")
	case "greaterThan":
		return matchGreaterThan(actual, expected)
	case "lessThan":
		return matchLessThan(actual, expected)
	case "greaterThanOrEqual":
		return matchGreaterThanOrEqual(actual, expected)
	case "lessThanOrEqual":
		return matchLessThanOrEqual(actual, expected)
	case "length":
		return matchLength(actual, expected)
	case "minLength":
		return matchMinLength(actual, expected)
	case "maxLength":
		return matchMaxLength(actual, expected)
	case "startsWith":
		return matchStartsWith(actual, expected)
	case "endsWith":
		return matchEndsWith(actual, expected)
	case "oneOf":
		return matchOneOf(actual, expected)
	case "notOneOf":
		return invert(matchOneOf(actual, expected), "notOneOf")
	case "isEmpty":
		return matchIsEmpty(actual)
	case "notEmpty":
		return invert(matchIsEmpty(actual), "notEmpty")
	case "hasProperty":
		return matchHasProperty(actual, expected)
	case "notHasProperty":
		return invert(matchHasProperty(actual, expected), "notHasProperty")
	default:
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("unknown operator %q", operator),
		}
	}
}

// invert negates a MatchResult for use with not-variants.
func invert(r MatchResult, op string) MatchResult {
	if r.Pass {
		return MatchResult{Pass: false, Message: fmt.Sprintf("%s: condition was unexpectedly true", op)}
	}
	return MatchResult{Pass: true, Message: ""}
}

// normalizeNumeric converts int, int64, float32 to float64 for uniform numeric comparison.
func normalizeNumeric(v any) any {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	default:
		return v
	}
}

// toFloat64 converts any numeric type or numeric string to float64.
// Returns 0 and logs if conversion is not possible.
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// typeOf returns the canonical type name for a value used by the "type" operator.
func typeOf(v any) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		// Use reflect for other numeric kinds.
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return "number"
		case reflect.Float32, reflect.Float64:
			return "number"
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return "number"
		case reflect.Map:
			return "object"
		case reflect.Slice, reflect.Array:
			return "array"
		}
		return rv.Type().String()
	}
}

// lengthOf returns the length of a string, []any, or map[string]any.
// Returns -1 when the type is not supported.
func lengthOf(v any) int {
	if v == nil {
		return 0
	}
	switch c := v.(type) {
	case string:
		return len(c)
	case []any:
		return len(c)
	case map[string]any:
		return len(c)
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
			return rv.Len()
		}
		return -1
	}
}

// matchEquals performs deep-equal comparison with numeric normalisation.
func matchEquals(actual, expected any) MatchResult {
	a := normalizeNumeric(actual)
	e := normalizeNumeric(expected)
	if reflect.DeepEqual(a, e) {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected %v (type %T), got %v (type %T)", expected, expected, actual, actual),
	}
}

// matchContains checks whether actual contains expected (string substring or array element).
func matchContains(actual, expected any) MatchResult {
	switch a := actual.(type) {
	case string:
		exp, ok := expected.(string)
		if !ok {
			exp = fmt.Sprintf("%v", expected)
		}
		if strings.Contains(a, exp) {
			return MatchResult{Pass: true}
		}
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("string %q does not contain %q", a, exp),
		}
	case []any:
		for _, item := range a {
			if reflect.DeepEqual(normalizeNumeric(item), normalizeNumeric(expected)) {
				return MatchResult{Pass: true}
			}
		}
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("array does not contain %v", expected),
		}
	default:
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("contains requires a string or array, got %T", actual),
		}
	}
}

// matchRegex checks whether the string representation of actual matches the regex in expected.
func matchRegex(actual, expected any) MatchResult {
	pattern, ok := expected.(string)
	if !ok {
		return MatchResult{Pass: false, Message: "matches: expected value must be a string regex pattern"}
	}
	str, ok := actual.(string)
	if !ok {
		str = fmt.Sprintf("%v", actual)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return MatchResult{Pass: false, Message: fmt.Sprintf("matches: invalid regex %q: %v", pattern, err)}
	}
	if re.MatchString(str) {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("%q does not match pattern %q", str, pattern),
	}
}

// matchType checks that actual has the type named by expected.
func matchType(actual, expected any) MatchResult {
	wantType, ok := expected.(string)
	if !ok {
		return MatchResult{Pass: false, Message: "type: expected value must be a string type name"}
	}
	got := typeOf(actual)
	if got == wantType {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected type %q, got %q", wantType, got),
	}
}

// matchExists checks that actual is non-nil when expected is true, or nil when expected is false.
func matchExists(actual, expected any) MatchResult {
	wantExists := true
	if b, ok := expected.(bool); ok {
		wantExists = b
	}
	isNil := actual == nil
	if wantExists && !isNil {
		return MatchResult{Pass: true}
	}
	if !wantExists && isNil {
		return MatchResult{Pass: true}
	}
	if wantExists {
		return MatchResult{Pass: false, Message: "expected value to exist (non-nil), but got nil"}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected value to not exist, but got %v", actual)}
}

// matchNotExists checks that actual is nil.
func matchNotExists(actual any) MatchResult {
	if actual == nil {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected nil (not exist), but got %v", actual)}
}

// matchIsNull checks that actual is nil.
func matchIsNull(actual any) MatchResult {
	if actual == nil {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected null, got %v (type %T)", actual, actual)}
}

// matchGreaterThan checks that actual > expected numerically.
func matchGreaterThan(actual, expected any) MatchResult {
	a, e := toFloat64(actual), toFloat64(expected)
	if a > e {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected %v > %v", actual, expected),
	}
}

// matchLessThan checks that actual < expected numerically.
func matchLessThan(actual, expected any) MatchResult {
	a, e := toFloat64(actual), toFloat64(expected)
	if a < e {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected %v < %v", actual, expected),
	}
}

// matchGreaterThanOrEqual checks that actual >= expected numerically.
func matchGreaterThanOrEqual(actual, expected any) MatchResult {
	a, e := toFloat64(actual), toFloat64(expected)
	if a >= e {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected %v >= %v", actual, expected),
	}
}

// matchLessThanOrEqual checks that actual <= expected numerically.
func matchLessThanOrEqual(actual, expected any) MatchResult {
	a, e := toFloat64(actual), toFloat64(expected)
	if a <= e {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected %v <= %v", actual, expected),
	}
}

// matchLength checks that the length of actual equals the expected integer.
func matchLength(actual, expected any) MatchResult {
	l := lengthOf(actual)
	if l < 0 {
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("length: unsupported type %T", actual),
		}
	}
	wantLen := int(toFloat64(expected))
	if l == wantLen {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected length %d, got %d", wantLen, l),
	}
}

// matchMinLength checks that the length of actual is at least the expected integer.
func matchMinLength(actual, expected any) MatchResult {
	l := lengthOf(actual)
	if l < 0 {
		return MatchResult{Pass: false, Message: fmt.Sprintf("minLength: unsupported type %T", actual)}
	}
	want := int(toFloat64(expected))
	if l >= want {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected length >= %d, got %d", want, l)}
}

// matchMaxLength checks that the length of actual is at most the expected integer.
func matchMaxLength(actual, expected any) MatchResult {
	l := lengthOf(actual)
	if l < 0 {
		return MatchResult{Pass: false, Message: fmt.Sprintf("maxLength: unsupported type %T", actual)}
	}
	want := int(toFloat64(expected))
	if l <= want {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected length <= %d, got %d", want, l)}
}

// matchStartsWith checks that the string form of actual begins with expected.
func matchStartsWith(actual, expected any) MatchResult {
	s, prefix := asString(actual), asString(expected)
	if strings.HasPrefix(s, prefix) {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected %q to start with %q", s, prefix)}
}

// matchEndsWith checks that the string form of actual ends with expected.
func matchEndsWith(actual, expected any) MatchResult {
	s, suffix := asString(actual), asString(expected)
	if strings.HasSuffix(s, suffix) {
		return MatchResult{Pass: true}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected %q to end with %q", s, suffix)}
}

// matchOneOf checks that actual equals at least one member of the expected slice.
func matchOneOf(actual, expected any) MatchResult {
	allowed, ok := expected.([]any)
	if !ok {
		return MatchResult{Pass: false, Message: fmt.Sprintf("oneOf: expected value must be an array, got %T", expected)}
	}
	a := normalizeNumeric(actual)
	for _, v := range allowed {
		if reflect.DeepEqual(a, normalizeNumeric(v)) {
			return MatchResult{Pass: true}
		}
	}
	return MatchResult{Pass: false, Message: fmt.Sprintf("expected %v to be one of %v", actual, allowed)}
}

// asString renders a value as a string, leaving genuine strings untouched.
func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// matchIsEmpty checks that actual has zero length or is nil.
func matchIsEmpty(actual any) MatchResult {
	if actual == nil {
		return MatchResult{Pass: true}
	}
	l := lengthOf(actual)
	if l == 0 {
		return MatchResult{Pass: true}
	}
	if l < 0 {
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("isEmpty: unsupported type %T", actual),
		}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("expected empty, got length %d", l),
	}
}

// matchHasProperty checks that actual (map[string]any) contains the key named by expected.
func matchHasProperty(actual, expected any) MatchResult {
	key, ok := expected.(string)
	if !ok {
		return MatchResult{Pass: false, Message: "hasProperty: expected value must be a string key name"}
	}
	m, ok := actual.(map[string]any)
	if !ok {
		return MatchResult{
			Pass:    false,
			Message: fmt.Sprintf("hasProperty: actual value must be an object, got %T", actual),
		}
	}
	if _, exists := m[key]; exists {
		return MatchResult{Pass: true}
	}
	return MatchResult{
		Pass:    false,
		Message: fmt.Sprintf("object does not have property %q", key),
	}
}
