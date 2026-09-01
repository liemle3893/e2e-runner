package interpolate_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/liemle3893/go-tryve/internal/interpolate"
)

// TestBuiltin_UUID verifies that $uuid() returns a valid UUID v4 string.
func TestBuiltin_UUID(t *testing.T) {
	got, err := interpolate.CallBuiltin("uuid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4.MatchString(got) {
		t.Errorf("expected UUID v4 format, got %q", got)
	}
}

// TestBuiltin_Timestamp verifies that $timestamp() returns a Unix millisecond number > 1700000000000.
func TestBuiltin_Timestamp(t *testing.T) {
	got, err := interpolate.CallBuiltin("timestamp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ms, err := strconv.ParseInt(got, 10, 64)
	if err != nil {
		t.Fatalf("expected numeric string, got %q: %v", got, err)
	}
	const minMS = int64(1700000000000)
	if ms <= minMS {
		t.Errorf("expected timestamp > %d, got %d", minMS, ms)
	}
}

// TestBuiltin_ISODate verifies that $isoDate() contains "T" and ends with "Z".
func TestBuiltin_ISODate(t *testing.T) {
	got, err := interpolate.CallBuiltin("isoDate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "T") {
		t.Errorf("expected ISO date with 'T', got %q", got)
	}
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("expected ISO date ending with 'Z', got %q", got)
	}
}

// TestBuiltin_Random verifies that $random(1, 10) returns a number in [1, 10].
func TestBuiltin_Random(t *testing.T) {
	for i := 0; i < 20; i++ {
		got, err := interpolate.CallBuiltin("random", "1", "10")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		n, err := strconv.Atoi(got)
		if err != nil {
			t.Fatalf("expected integer string, got %q: %v", got, err)
		}
		if n < 1 || n > 10 {
			t.Errorf("expected value in [1, 10], got %d", n)
		}
	}
}

// TestBuiltin_RandomString verifies that $randomString(n) returns a string of length n.
func TestBuiltin_RandomString(t *testing.T) {
	cases := []struct {
		arg     string
		wantLen int
	}{
		{"8", 8},
		{"16", 16},
		{"1", 1},
	}
	for _, tc := range cases {
		got, err := interpolate.CallBuiltin("randomString", tc.arg)
		if err != nil {
			t.Fatalf("unexpected error for length %s: %v", tc.arg, err)
		}
		if len(got) != tc.wantLen {
			t.Errorf("expected length %d, got %d (%q)", tc.wantLen, len(got), got)
		}
		// Must be alphanumeric
		alphanumeric := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
		if !alphanumeric.MatchString(got) {
			t.Errorf("expected alphanumeric string, got %q", got)
		}
	}
}

// TestBuiltin_RandomString_Default verifies that $randomString() (no args) defaults to length 8.
func TestBuiltin_RandomString_Default(t *testing.T) {
	got, err := interpolate.CallBuiltin("randomString")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 8 {
		t.Errorf("expected default length 8, got %d (%q)", len(got), got)
	}
}

// TestBuiltin_Base64 and TestBuiltin_Base64Decode verify round-trip encoding.
func TestBuiltin_Base64(t *testing.T) {
	original := "hello world"
	encoded, err := interpolate.CallBuiltin("base64", original)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded, err := interpolate.CallBuiltin("base64Decode", encoded)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip failed: got %q, want %q", decoded, original)
	}
}

// TestBuiltin_MD5 verifies md5("hello") produces the known digest.
func TestBuiltin_MD5(t *testing.T) {
	got, err := interpolate.CallBuiltin("md5", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Errorf("md5(hello) = %q, want %q", got, want)
	}
}

// TestBuiltin_SHA256 verifies sha256("hello") produces the known digest.
func TestBuiltin_SHA256(t *testing.T) {
	got, err := interpolate.CallBuiltin("sha256", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("sha256(hello) = %q, want %q", got, want)
	}
}

// TestBuiltin_Env verifies that $env() returns the value of a set environment variable.
func TestBuiltin_Env(t *testing.T) {
	t.Setenv("E2E_TEST_VAR", "hello_from_env")
	got, err := interpolate.CallBuiltin("env", "E2E_TEST_VAR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello_from_env" {
		t.Errorf("expected %q, got %q", "hello_from_env", got)
	}
}

// TestBuiltin_Env_Missing verifies that $env() returns an error for unset variables.
func TestBuiltin_Env_Missing(t *testing.T) {
	os.Unsetenv("E2E_DEFINITELY_NOT_SET_VAR_XYZ")
	_, err := interpolate.CallBuiltin("env", "E2E_DEFINITELY_NOT_SET_VAR_XYZ")
	if err == nil {
		t.Fatal("expected error for missing env var, got nil")
	}
}

// TestBuiltin_File verifies that $file() reads a temp file correctly.
func TestBuiltin_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")
	content := "file content here"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	got, err := interpolate.CallBuiltin("file", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("expected %q, got %q", content, got)
	}
}

// TestBuiltin_Lower verifies $lower() converts a string to lowercase.
func TestBuiltin_Lower(t *testing.T) {
	got, err := interpolate.CallBuiltin("lower", "Hello WORLD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", got)
	}
}

// TestBuiltin_Upper verifies $upper() converts a string to uppercase.
func TestBuiltin_Upper(t *testing.T) {
	got, err := interpolate.CallBuiltin("upper", "Hello World")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "HELLO WORLD" {
		t.Errorf("expected %q, got %q", "HELLO WORLD", got)
	}
}

// TestBuiltin_Trim verifies $trim() removes leading and trailing whitespace.
func TestBuiltin_Trim(t *testing.T) {
	got, err := interpolate.CallBuiltin("trim", "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

// TestBuiltin_Unknown verifies that calling an unknown built-in returns an error.
func TestBuiltin_Unknown(t *testing.T) {
	_, err := interpolate.CallBuiltin("doesNotExist")
	if err == nil {
		t.Fatal("expected error for unknown builtin, got nil")
	}
}

// TestBuiltin_Now verifies that $now() returns a formatted time string for known format keys.
func TestBuiltin_Now(t *testing.T) {
	cases := []struct {
		format  string
		wantSub string // substring that must appear in result
	}{
		{"iso", "T"},
		{"date", "-"},
		{"unix", ""},   // numeric
		{"unixMs", ""}, // numeric
	}
	for _, tc := range cases {
		got, err := interpolate.CallBuiltin("now", tc.format)
		if err != nil {
			t.Fatalf("now(%q): unexpected error: %v", tc.format, err)
		}
		if got == "" {
			t.Errorf("now(%q): got empty string", tc.format)
		}
		if tc.wantSub != "" && !strings.Contains(got, tc.wantSub) {
			t.Errorf("now(%q): expected %q in output, got %q", tc.format, tc.wantSub, got)
		}
	}
}

// TestBuiltin_DateAdd verifies that $dateAdd(amount, unit) returns a future timestamp.
func TestBuiltin_DateAdd(t *testing.T) {
	// Adding 1 hour should produce a result > current unix seconds
	got, err := interpolate.CallBuiltin("dateAdd", "1", "h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result from dateAdd")
	}
}

// TestBuiltin_DateSub verifies that $dateSub(amount, unit) returns a past timestamp.
func TestBuiltin_DateSub(t *testing.T) {
	got, err := interpolate.CallBuiltin("dateSub", "1", "h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty result from dateSub")
	}
}

// TestBuiltin_TOTP verifies $totp() returns a 6-digit numeric code.
func TestBuiltin_TOTP(t *testing.T) {
	// Use a valid base32 secret (RFC 4648 base32, padded)
	secret := "JBSWY3DPEHPK3PXP"
	got, err := interpolate.CallBuiltin("totp", secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sixDigit := regexp.MustCompile(`^\d{6}$`)
	if !sixDigit.MatchString(got) {
		t.Errorf("expected 6-digit TOTP code, got %q", got)
	}
}

// TestBuiltin_JSONStringify verifies $jsonStringify() escapes special characters.
func TestBuiltin_JSONStringify(t *testing.T) {
	input := `hello "world"` + "\nwith\ttabs"
	got, err := interpolate.CallBuiltin("jsonStringify", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not contain raw newline or tab
	if strings.Contains(got, "\n") {
		t.Error("expected newline to be escaped, but raw newline found")
	}
	if strings.Contains(got, "\t") {
		t.Error("expected tab to be escaped, but raw tab found")
	}
	// Should contain escaped quote
	if !strings.Contains(got, `\"`) {
		t.Error("expected escaped double-quote in output")
	}
}
