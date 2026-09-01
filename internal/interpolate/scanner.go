package interpolate

import (
	"fmt"
	"strings"
)

// token is one piece of a parsed template: either literal text or an expression.
type token struct {
	text   string // literal text, when isExpr is false
	expr   string // expression source without the delimiters, when isExpr is true
	raw    string // the original token including delimiters, for passthrough
	isExpr bool
	dollar bool // true when the delimiter was ${…} rather than {{…}}
}

// tokenize splits a template string into literal and expression tokens.
//
// Unlike a regular expression, this scanner tracks nesting depth, so an
// expression may itself contain a `{{…}}` expression or any number of braces:
//
//	{{$upper({{captured.name}})}}
//	{{$jsonStringify({"a": 1})}}
//
// An unterminated delimiter is not an error; the remaining text is emitted as a
// literal so that a stray brace in a shell script or SQL string passes through
// untouched.
func tokenize(s string) []token {
	var tokens []token
	var lit strings.Builder

	flushLiteral := func() {
		if lit.Len() > 0 {
			tokens = append(tokens, token{text: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], "{{"):
			inner, end, ok := scanDelimited(s, i+2, "{{", "}}")
			if !ok {
				lit.WriteByte(s[i])
				i++
				continue
			}
			flushLiteral()
			tokens = append(tokens, token{expr: inner, raw: s[i:end], isExpr: true})
			i = end

		case strings.HasPrefix(s[i:], "${"):
			inner, end, ok := scanDelimited(s, i+2, "${", "}")
			if !ok {
				lit.WriteByte(s[i])
				i++
				continue
			}
			flushLiteral()
			tokens = append(tokens, token{expr: inner, raw: s[i:end], isExpr: true, dollar: true})
			i = end

		default:
			lit.WriteByte(s[i])
			i++
		}
	}
	flushLiteral()

	return tokens
}

// scanDelimited reads from start until the matching close delimiter, honouring
// nested open/close pairs. It returns the inner text, the index just past the
// close delimiter, and whether a match was found.
func scanDelimited(s string, start int, open, close string) (string, int, bool) {
	depth := 1
	for i := start; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], open):
			depth++
			i += len(open)
		case strings.HasPrefix(s[i:], close):
			depth--
			i += len(close)
			if depth == 0 {
				return s[start : i-len(close)], i, true
			}
		default:
			i++
		}
	}
	return "", 0, false
}

// parseBuiltinCall parses `$name` or `$name(arg, arg, …)` into a function name
// and its arguments.
//
// Arguments are split on commas that sit outside quotes and outside nested
// parentheses, and surrounding single or double quotes are stripped. That is
// what allows an argument to contain the separator characters that a naive
// split would break on:
//
//	{{$hmac("a,b", key)}}      a comma inside a quoted argument
//	{{$jsonPath(captured.r, $.items[0].id)}}   brackets and dots
//
// unquoteArgs selects whether surrounding quotes are stripped from arguments.
// Legacy argument parsing passed the quotes through to the builtin, so
// {{$upper("abc")}} produced `"ABC"` including them.
func parseBuiltinCall(expr string, unquoteArgs bool) (string, []string, bool) {
	if !strings.HasPrefix(expr, "$") {
		return "", nil, false
	}
	body := expr[1:]

	open := strings.IndexByte(body, '(')
	if open < 0 {
		// Bare `$name` form — must be a plain identifier.
		if body == "" || !isIdentifier(body) {
			return "", nil, false
		}
		return body, nil, true
	}

	name := body[:open]
	if !isIdentifier(name) {
		return "", nil, false
	}
	if !strings.HasSuffix(strings.TrimSpace(body), ")") {
		return "", nil, false
	}
	argsSrc := strings.TrimSpace(body)[open+1 : len(strings.TrimSpace(body))-1]

	return name, splitArgs(argsSrc, unquoteArgs), true
}

// splitArgs divides an argument list on top-level commas, respecting quotes and
// nested parentheses, then trims and unquotes each argument.
func splitArgs(src string, unquoteArgs bool) []string {
	trim := func(s string) string {
		s = strings.TrimSpace(s)
		if unquoteArgs {
			return unquote(s)
		}
		return s
	}
	if strings.TrimSpace(src) == "" {
		return nil
	}

	var args []string
	var cur strings.Builder
	depth := 0
	var quote byte

	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == '(' || c == '[':
			depth++
			cur.WriteByte(c)
		case c == ')' || c == ']':
			depth--
			cur.WriteByte(c)
		case c == ',' && depth == 0:
			args = append(args, trim(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	args = append(args, trim(cur.String()))
	return args
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// isIdentifier reports whether s is a bare function name.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// formatUnresolved builds the error text for expressions left unresolved.
func formatUnresolved(exprs []string) string {
	return fmt.Sprintf("unresolved expression(s): %s", strings.Join(exprs, ", "))
}
