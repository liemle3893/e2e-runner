package interpolate

import (
	"crypto/md5" //nolint:gosec // md5 used for non-security hashing
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// BuiltinFunc is the signature for all built-in interpolation functions.
// It accepts zero or more string arguments and returns a string result or an error.
type BuiltinFunc func(args ...string) (string, error)

// alphanumChars is the character set used by randomString.
const alphanumChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// builtins is the registry of all supported built-in function names.
var builtins = map[string]BuiltinFunc{
	"uuid":          builtinUUID,
	"timestamp":     builtinTimestamp,
	"isoDate":       builtinISODate,
	"random":        builtinRandom,
	"randomString":  builtinRandomString,
	"env":           builtinEnv,
	"file":          builtinFile,
	"base64":        builtinBase64,
	"base64Decode":  builtinBase64Decode,
	"md5":           builtinMD5,
	"sha256":        builtinSHA256,
	"now":           builtinNow,
	"dateAdd":       builtinDateAdd,
	"dateSub":       builtinDateSub,
	"totp":          builtinTOTP,
	"jsonStringify": builtinJSONStringify,
	"lower":         builtinLower,
	"upper":         builtinUpper,
	"trim":          builtinTrim,
}

// CallBuiltin calls a named built-in function with the given arguments.
// Returns an error if the function name is unknown or execution fails.
func CallBuiltin(name string, args ...string) (string, error) {
	fn, ok := builtins[name]
	if !ok {
		return "", fmt.Errorf("unknown built-in function: %q", name)
	}
	return fn(args...)
}

// ContextualFunc is a built-in that needs the interpolation context, and that may
// return a value of any type rather than text.
type ContextualFunc func(ctx *tryve.InterpolationContext, args ...string) (any, error)

// contextualBuiltins holds built-ins that read the surrounding context or return
// structured values.
//
// It is populated in init rather than as a composite literal because these
// functions resolve their arguments through the interpolator, which dispatches
// back through this map — a cycle the compiler rejects at package scope.
var contextualBuiltins map[string]ContextualFunc

func init() {
	contextualBuiltins = map[string]ContextualFunc{
		"json":      builtinJSON,
		"jsonPath":  builtinJSONPath,
		"jsonFile":  builtinJSONFile,
		"int":       builtinInt,
		"number":    builtinNumber,
		"bool":      builtinBool,
		"default":   builtinDefault,
		"jwt":       builtinJWT,
		"hmac":      builtinHMAC,
		"base64url": builtinBase64URL,
	}
}

// CallBuiltinValue calls a named built-in, preferring a contextual implementation
// when one exists. The returned value keeps its natural type so that, for example,
// {{$int(captured.n)}} yields a number rather than text.
func CallBuiltinValue(name string, ctx *tryve.InterpolationContext, args ...string) (any, error) {
	if fn, ok := contextualBuiltins[name]; ok {
		return fn(ctx, args...)
	}
	fn, ok := builtins[name]
	if !ok {
		return nil, fmt.Errorf("unknown built-in function %q; run `tryve doc built-in-functions` for the list", name)
	}
	return fn(args...)
}

// builtinUUID returns a new UUID v4 string.
func builtinUUID(args ...string) (string, error) {
	return uuid.NewString(), nil
}

// builtinTimestamp returns the current time as Unix milliseconds.
func builtinTimestamp(args ...string) (string, error) {
	return strconv.FormatInt(time.Now().UnixMilli(), 10), nil
}

// builtinISODate returns the current UTC time in RFC3339 format.
func builtinISODate(args ...string) (string, error) {
	return time.Now().UTC().Format(time.RFC3339), nil
}

// builtinRandom returns a random integer in the inclusive range [min, max].
// Requires exactly two arguments: min and max.
func builtinRandom(args ...string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("random: expected 2 arguments (min, max), got %d", len(args))
	}
	minVal, err := strconv.Atoi(args[0])
	if err != nil {
		return "", fmt.Errorf("random: invalid min value %q: %w", args[0], err)
	}
	maxVal, err := strconv.Atoi(args[1])
	if err != nil {
		return "", fmt.Errorf("random: invalid max value %q: %w", args[1], err)
	}
	if minVal > maxVal {
		return "", fmt.Errorf("random: min (%d) must not exceed max (%d)", minVal, maxVal)
	}
	n := minVal + rand.Intn(maxVal-minVal+1) //nolint:gosec // non-cryptographic use
	return strconv.Itoa(n), nil
}

// builtinRandomString returns a random alphanumeric string of the specified length.
// Defaults to length 8 when no argument is provided.
func builtinRandomString(args ...string) (string, error) {
	length := 8
	if len(args) >= 1 && args[0] != "" {
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return "", fmt.Errorf("randomString: invalid length %q: %w", args[0], err)
		}
		length = n
	}
	if length <= 0 {
		return "", fmt.Errorf("randomString: length must be positive, got %d", length)
	}
	b := make([]byte, length)
	for i := range b {
		b[i] = alphanumChars[rand.Intn(len(alphanumChars))] //nolint:gosec // non-cryptographic use
	}
	return string(b), nil
}

// builtinEnv returns the value of the named environment variable.
// Returns an error if the variable is not set.
func builtinEnv(args ...string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("env: expected 1 argument (variable name)")
	}
	val, ok := os.LookupEnv(args[0])
	if !ok {
		return "", fmt.Errorf("env: variable %q is not set", args[0])
	}
	return val, nil
}

// builtinFile reads and returns the contents of the file at the given path.
func builtinFile(args ...string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("file: expected 1 argument (file path)")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return "", fmt.Errorf("file: could not read %q: %w", args[0], err)
	}
	return string(data), nil
}

// builtinBase64 encodes the input string using standard base64 encoding.
func builtinBase64(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("base64: expected 1 argument")
	}
	return base64.StdEncoding.EncodeToString([]byte(args[0])), nil
}

// builtinBase64Decode decodes a standard base64-encoded string.
func builtinBase64Decode(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("base64Decode: expected 1 argument")
	}
	decoded, err := base64.StdEncoding.DecodeString(args[0])
	if err != nil {
		return "", fmt.Errorf("base64Decode: invalid base64 input: %w", err)
	}
	return string(decoded), nil
}

// builtinMD5 returns the MD5 hex digest of the input string.
func builtinMD5(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("md5: expected 1 argument")
	}
	sum := md5.Sum([]byte(args[0])) //nolint:gosec // md5 used for non-security hashing
	return fmt.Sprintf("%x", sum), nil
}

// builtinSHA256 returns the SHA-256 hex digest of the input string.
func builtinSHA256(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("sha256: expected 1 argument")
	}
	sum := sha256.Sum256([]byte(args[0]))
	return fmt.Sprintf("%x", sum), nil
}

// nowFormats maps symbolic format names to Go time layout strings or special tokens.
var nowFormats = map[string]string{
	"iso":      time.RFC3339,
	"date":     "2006-01-02",
	"time":     "15:04:05",
	"datetime": "2006-01-02 15:04:05",
	"unix":     "_unix_",
	"unixMs":   "_unixMs_",
}

// builtinNow returns the current time in the specified format.
// Accepts "iso", "date", "time", "datetime", "unix", "unixMs", or any Go time layout string.
// Defaults to RFC3339 when no argument is provided.
func builtinNow(args ...string) (string, error) {
	now := time.Now().UTC()
	format := time.RFC3339
	if len(args) >= 1 && args[0] != "" {
		format = args[0]
	}
	if mapped, ok := nowFormats[format]; ok {
		format = mapped
	}
	switch format {
	case "_unix_":
		return strconv.FormatInt(now.Unix(), 10), nil
	case "_unixMs_":
		return strconv.FormatInt(now.UnixMilli(), 10), nil
	default:
		return now.Format(format), nil
	}
}

// parseTimeUnit converts a unit string to a time.Duration multiplier.
// Supported units: s/second, m/minute, h/hour, d/day, w/week, month, y/year.
func parseTimeUnit(amount int, unit string) (time.Duration, error) {
	switch strings.ToLower(unit) {
	case "s", "second", "seconds":
		return time.Duration(amount) * time.Second, nil
	case "m", "minute", "minutes":
		return time.Duration(amount) * time.Minute, nil
	case "h", "hour", "hours":
		return time.Duration(amount) * time.Hour, nil
	case "d", "day", "days":
		return time.Duration(amount) * 24 * time.Hour, nil
	case "w", "week", "weeks":
		return time.Duration(amount) * 7 * 24 * time.Hour, nil
	case "month", "months":
		// Approximate: 30 days
		return time.Duration(amount) * 30 * 24 * time.Hour, nil
	case "y", "year", "years":
		// Approximate: 365 days
		return time.Duration(amount) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown time unit: %q", unit)
	}
}

// builtinDateAdd returns the current UTC time plus the given amount and unit, in RFC3339.
// Requires exactly two arguments: amount (integer) and unit string.
func builtinDateAdd(args ...string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("dateAdd: expected 2 arguments (amount, unit), got %d", len(args))
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil {
		return "", fmt.Errorf("dateAdd: invalid amount %q: %w", args[0], err)
	}
	dur, err := parseTimeUnit(amount, args[1])
	if err != nil {
		return "", fmt.Errorf("dateAdd: %w", err)
	}
	return time.Now().UTC().Add(dur).Format(time.RFC3339), nil
}

// builtinDateSub returns the current UTC time minus the given amount and unit, in RFC3339.
// Requires exactly two arguments: amount (integer) and unit string.
func builtinDateSub(args ...string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("dateSub: expected 2 arguments (amount, unit), got %d", len(args))
	}
	amount, err := strconv.Atoi(args[0])
	if err != nil {
		return "", fmt.Errorf("dateSub: invalid amount %q: %w", args[0], err)
	}
	dur, err := parseTimeUnit(amount, args[1])
	if err != nil {
		return "", fmt.Errorf("dateSub: %w", err)
	}
	return time.Now().UTC().Add(-dur).Format(time.RFC3339), nil
}

// builtinTOTP generates a current 6-digit TOTP code from the given base32 secret.
func builtinTOTP(args ...string) (string, error) {
	if len(args) != 1 || args[0] == "" {
		return "", fmt.Errorf("totp: expected 1 argument (base32 secret)")
	}
	code, err := totp.GenerateCode(args[0], time.Now())
	if err != nil {
		return "", fmt.Errorf("totp: failed to generate code: %w", err)
	}
	return code, nil
}

// builtinJSONStringify escapes a string value for safe embedding in a JSON string.
// Escapes backslashes, double quotes, newlines, and tabs.
func builtinJSONStringify(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("jsonStringify: expected 1 argument")
	}
	s := args[0]
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s, nil
}

// builtinLower converts the input string to lowercase.
func builtinLower(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("lower: expected 1 argument")
	}
	return strings.ToLower(args[0]), nil
}

// builtinUpper converts the input string to uppercase.
func builtinUpper(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("upper: expected 1 argument")
	}
	return strings.ToUpper(args[0]), nil
}

// builtinTrim removes leading and trailing whitespace from the input string.
func builtinTrim(args ...string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("trim: expected 1 argument")
	}
	return strings.TrimSpace(args[0]), nil
}
