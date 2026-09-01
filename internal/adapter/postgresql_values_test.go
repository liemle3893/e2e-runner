package adapter

import (
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestNumericNormalisation covers NUMERIC and DECIMAL columns. pgx decodes them
// to a struct with no String method, so before normalisation they reached
// assertions opaque: `equals: 100` failed against a struct, and greaterThan
// silently compared against zero.
func TestNumericNormalisation(t *testing.T) {
	cases := []struct {
		name string
		in   pgtype.Numeric
		want any
	}{
		{
			name: "whole number",
			in:   pgtype.Numeric{Int: big.NewInt(100), Exp: 0, Valid: true},
			want: float64(100),
		},
		{
			name: "two decimal places",
			in:   pgtype.Numeric{Int: big.NewInt(12345), Exp: -2, Valid: true},
			want: float64(123.45),
		},
		{
			name: "null",
			in:   pgtype.Numeric{Valid: false},
			want: nil,
		},
		{
			name: "not a number",
			in:   pgtype.Numeric{NaN: true, Valid: true},
			want: "NaN",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeValue(tc.in)
			if got != tc.want {
				t.Errorf("expected %#v (%T), got %#v (%T)", tc.want, tc.want, got, got)
			}
		})
	}
}

// TestNumericPrecisionIsNotLost checks that a value too large for a float64 is
// returned as exact decimal text rather than silently rounded.
func TestNumericPrecisionIsNotLost(t *testing.T) {
	huge, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	got := normalizeValue(pgtype.Numeric{Int: huge, Exp: 0, Valid: true})

	str, ok := got.(string)
	if !ok {
		t.Fatalf("expected exact decimal text for an oversized NUMERIC, got %#v (%T)", got, got)
	}
	if str != "123456789012345678901234567890" {
		t.Errorf("expected the exact value, got %q", str)
	}
}

// TestDateColumnDropsTimeComponent covers DATE columns. pgx decodes DATE and
// TIMESTAMP alike to time.Time, so without the column's type OID a DATE renders
// as "2026-08-31T00:00:00Z" and never equals the "2026-08-31" an author writes.
func TestDateColumnDropsTimeComponent(t *testing.T) {
	ts := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

	if got := normalizeColumn(ts, pgtype.DateOID, true); got != "2026-08-31" {
		t.Errorf("DATE column: expected 2026-08-31, got %#v", got)
	}
	if got := normalizeColumn(ts, pgtype.TimestamptzOID, true); got != "2026-08-31T00:00:00Z" {
		t.Errorf("TIMESTAMPTZ column: expected the full timestamp, got %#v", got)
	}
}

// TestUUIDNormalisation covers both representations pgx may hand back.
func TestUUIDNormalisation(t *testing.T) {
	raw := [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
	want := "01234567-89ab-cdef-0123-456789abcdef"

	if got := normalizeValue(raw); got != want {
		t.Errorf("expected %q, got %#v", want, got)
	}
	if got := normalizeValue(pgtype.UUID{Bytes: raw, Valid: true}); got != want {
		t.Errorf("pgtype.UUID: expected %q, got %#v", want, got)
	}
	if got := normalizeValue(pgtype.UUID{Valid: false}); got != nil {
		t.Errorf("a null uuid should normalise to nil, got %#v", got)
	}
}

// TestArrayElementsAreNormalised checks that an array column's elements get the
// same treatment as scalar columns.
func TestArrayElementsAreNormalised(t *testing.T) {
	raw := [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}

	got, ok := normalizeValue([]any{raw, pgtype.Numeric{Int: big.NewInt(5), Valid: true}}).([]any)
	if !ok {
		t.Fatalf("expected a slice back")
	}
	if got[0] != "01234567-89ab-cdef-0123-456789abcdef" {
		t.Errorf("uuid element was not normalised: %#v", got[0])
	}
	if got[1] != float64(5) {
		t.Errorf("numeric element was not normalised: %#v", got[1])
	}
}

// TestScalarCount covers the `count` action. A SELECT COUNT(*) returns one row
// holding the count, so counting rows always answered 1 regardless of the value.
func TestScalarCount(t *testing.T) {
	aggregate := []map[string]any{{"count": int64(37)}}
	if got := scalarCount(aggregate); got != 37 {
		t.Errorf("SELECT COUNT(*) returning 37: expected 37, got %v", got)
	}

	multiRow := []map[string]any{{"id": 1}, {"id": 2}, {"id": 3}}
	if got := scalarCount(multiRow); got != 3 {
		t.Errorf("a three-row result should count 3, got %v", got)
	}

	// More than one column is not an aggregate; fall back to counting rows.
	wide := []map[string]any{{"id": 1, "name": "a"}}
	if got := scalarCount(wide); got != 1 {
		t.Errorf("expected the row count, got %v", got)
	}

	if got := scalarCount(nil); got != 0 {
		t.Errorf("an empty result should count 0, got %v", got)
	}
}

// TestIntervalFormatting checks that INTERVAL columns become readable text
// rather than an opaque struct.
func TestIntervalFormatting(t *testing.T) {
	got := normalizeValue(pgtype.Interval{Days: 2, Microseconds: 3_600_000_000, Valid: true})
	if got != "P2DT3600S" {
		t.Errorf("expected P2DT3600S, got %#v", got)
	}
	if got := normalizeValue(pgtype.Interval{Valid: false}); got != nil {
		t.Errorf("a null interval should normalise to nil, got %#v", got)
	}
}

// TestLegacyColumnConversionIsUnchanged pins the pre-v2 conversion: a DATE kept
// its time component, and driver types with no case fell through untouched.
func TestLegacyColumnConversionIsUnchanged(t *testing.T) {
	ts := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if got := normalizeColumn(ts, pgtype.DateOID, false); got != "2026-08-31T00:00:00Z" {
		t.Errorf("legacy DATE should keep the full timestamp, got %#v", got)
	}

	// A NUMERIC reached assertions as the driver's own struct.
	n := pgtype.Numeric{Int: big.NewInt(100), Valid: true}
	if got := normalizeColumn(n, pgtype.NumericOID, false); got != any(n) {
		t.Errorf("legacy NUMERIC should pass through untouched, got %#v", got)
	}
}
