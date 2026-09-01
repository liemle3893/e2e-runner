package interpolate

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Lookup walks root along a dotted path and returns the value found there.
//
// The path syntax covers what test authors actually write against captured data:
//
//	user.name          map key
//	rows[0].id         array index in bracket form
//	rows.0.id          array index in dotted form
//
// When a segment must descend into a value that is a JSON *string*, the string
// is parsed and traversal continues inside it. That is what lets a step capture
// a script's stdout — a JSON document as far as the shell is concerned — and
// then address a field inside it directly, instead of shelling out to `node -e`
// or `jq` to dig the value back out.
func Lookup(root any, path string) (any, bool) {
	segments, ok := splitPath(path)
	if !ok {
		return nil, false
	}

	cur := root
	for _, seg := range segments {
		next, found := descend(cur, seg)
		if !found {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// descend resolves a single path segment against the current value, parsing a
// JSON string first when the segment cannot otherwise be applied.
func descend(cur any, seg pathSegment) (any, bool) {
	if v, ok := descendDirect(cur, seg); ok {
		return v, true
	}
	// The value may be a JSON document carried as a string (a captured stdout, a
	// text column holding JSON). Parse it once and retry the same segment.
	if parsed, ok := parseJSONString(cur); ok {
		return descendDirect(parsed, seg)
	}
	return nil, false
}

// descendDirect applies one segment to a value without any JSON parsing.
func descendDirect(cur any, seg pathSegment) (any, bool) {
	if seg.isIndex {
		arr, ok := cur.([]any)
		if !ok {
			return nil, false
		}
		idx := seg.index
		if idx < 0 {
			idx += len(arr) // negative indices count from the end
		}
		if idx < 0 || idx >= len(arr) {
			return nil, false
		}
		return arr[idx], true
	}

	switch typed := cur.(type) {
	case map[string]any:
		v, ok := typed[seg.key]
		return v, ok
	case []any:
		// A numeric key written in dotted form still addresses an array element.
		if n, err := strconv.Atoi(seg.key); err == nil && n >= 0 && n < len(typed) {
			return typed[n], true
		}
	}
	return nil, false
}

// parseJSONString reports whether v is a string holding a JSON object or array,
// returning the decoded value when it is.
func parseJSONString(v any) (any, bool) {
	s, ok := v.(string)
	if !ok {
		return nil, false
	}
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return nil, false
	}
	// Only objects and arrays are worth parsing; treating a bare number or a
	// quoted word as JSON would turn ordinary captured text into a surprise.
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

// pathSegment is one step of a lookup path: either a map key or an array index.
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

// splitPath parses a dotted path with optional bracket indices into segments.
// It reports false for syntactically malformed paths (an unclosed or non-numeric
// bracket), so a typo surfaces as an unresolved expression rather than silently
// looking up a key that happens to contain brackets.
func splitPath(path string) ([]pathSegment, bool) {
	var segments []pathSegment
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			segments = append(segments, pathSegment{key: cur.String()})
			cur.Reset()
		}
	}

	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			flush()
		case '[':
			flush()
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, false
			}
			inner := path[i+1 : i+end]
			n, err := strconv.Atoi(strings.TrimSpace(inner))
			if err != nil {
				return nil, false
			}
			segments = append(segments, pathSegment{index: n, isIndex: true})
			i += end
		default:
			cur.WriteByte(path[i])
		}
	}
	flush()

	if len(segments) == 0 {
		return nil, false
	}
	return segments, true
}
