package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/liemle3893/go-tryve/internal/tryve"
)

const (
	postgresqlAdapterName   = "postgresql"
	defaultPostgresPoolSize = 5
)

// PostgreSQLAdapter executes SQL statements against a PostgreSQL database using
// a pgxpool connection pool. It supports the "execute", "query", "queryOne",
// and "count" actions.
type PostgreSQLAdapter struct {
	connectionString string
	schema           string
	poolSize         int
	pool             *pgxpool.Pool
	compat           tryve.CompatMode
}

// NewPostgreSQLAdapter constructs a PostgreSQLAdapter from a configuration map.
//
// Recognised keys:
//   - "connectionString" (string, required) — libpq-compatible DSN or URL.
//   - "schema"           (string, optional) — default search_path schema.
//   - "poolSize"         (int or float64, optional, default 5) — maximum pool connections.
func NewPostgreSQLAdapter(cfg map[string]any) *PostgreSQLAdapter {
	return NewPostgreSQLAdapterWithCompat(cfg, tryve.LegacyCompat())
}

// NewPostgreSQLAdapterWithCompat is NewPostgreSQLAdapter with an explicit
// compatibility mode selecting how column values and the count action behave.
func NewPostgreSQLAdapterWithCompat(cfg map[string]any, mode tryve.CompatMode) *PostgreSQLAdapter {
	connStr, _ := cfg["connectionString"].(string)

	schema, _ := cfg["schema"].(string)

	poolSize := defaultPostgresPoolSize
	switch v := cfg["poolSize"].(type) {
	case int:
		if v > 0 {
			poolSize = v
		}
	case float64:
		if int(v) > 0 {
			poolSize = int(v)
		}
	}

	return &PostgreSQLAdapter{
		connectionString: connStr,
		schema:           schema,
		poolSize:         poolSize,
		compat:           mode,
	}
}

// Name returns the adapter's registered identifier.
func (a *PostgreSQLAdapter) Name() string { return postgresqlAdapterName }

// Connect creates the pgxpool connection pool. It applies the configured
// poolSize as the maximum number of connections. If schema is non-empty,
// it is set as the default search_path for all connections in the pool.
func (a *PostgreSQLAdapter) Connect(ctx context.Context) error {
	if a.connectionString == "" {
		return tryve.ConnectionError(postgresqlAdapterName, "connectionString must not be empty", nil)
	}
	if err := CheckUnresolvedEnvVars(postgresqlAdapterName, "connectionString", a.connectionString); err != nil {
		return err
	}

	cfg, err := pgxpool.ParseConfig(a.connectionString)
	if err != nil {
		return tryve.ConnectionError(postgresqlAdapterName, "failed to parse connection string", err)
	}

	cfg.MaxConns = int32(a.poolSize) //nolint:gosec

	if a.schema != "" {
		// Prepend the configured schema to the search_path for every acquired connection.
		origAfterConnect := cfg.AfterConnect
		cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			if origAfterConnect != nil {
				if err := origAfterConnect(ctx, conn); err != nil {
					return err
				}
			}
			_, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", a.schema))
			return err
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return tryve.ConnectionError(postgresqlAdapterName, "failed to create connection pool", err)
	}

	a.pool = pool
	return nil
}

// Close shuts down the connection pool, releasing all held connections.
func (a *PostgreSQLAdapter) Close(_ context.Context) error {
	if a.pool != nil {
		a.pool.Close()
		a.pool = nil
	}
	return nil
}

// Health performs a lightweight ping against the database to verify connectivity.
func (a *PostgreSQLAdapter) Health(ctx context.Context) error {
	if a.pool == nil {
		return tryve.ConnectionError(postgresqlAdapterName, "pool is not initialised; call Connect first", nil)
	}
	if err := a.pool.Ping(ctx); err != nil {
		return tryve.ConnectionError(postgresqlAdapterName, "ping failed", err)
	}
	return nil
}

// Execute dispatches the named action against the database.
//
// Supported actions:
//   - "execute"  — run a non-SELECT statement; returns {"rowsAffected": float64}.
//   - "query"    — run a SELECT; returns {"rows": []map[string]any, "rowCount": float64}.
//   - "queryOne" — run a SELECT, return the first row; returns {"row": map[string]any}.
//   - "count"    — run a SELECT, return only the row count; returns {"count": float64}.
func (a *PostgreSQLAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	switch action {
	case "execute":
		return a.executeAction(ctx, params)
	case "query":
		return a.queryAction(ctx, params)
	case "queryOne":
		return a.queryOneAction(ctx, params)
	case "count":
		return a.countAction(ctx, params)
	default:
		return nil, tryve.AdapterError(postgresqlAdapterName, action,
			fmt.Sprintf("unsupported action %q; valid actions are: execute, query, queryOne, count", action), nil)
	}
}

// executeAction runs a non-SELECT SQL statement and returns the number of rows affected.
func (a *PostgreSQLAdapter) executeAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	sql, queryParams, err := extractSQLParams(params)
	if err != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "execute", err.Error(), err)
	}

	var tag interface{ RowsAffected() int64 }
	duration, execErr := MeasureDuration(func() error {
		ct, e := a.pool.Exec(ctx, sql, queryParams...)
		if e == nil {
			tag = ct
		}
		return e
	})
	if execErr != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "execute", "statement execution failed", execErr)
	}

	data := map[string]any{
		"rowsAffected": float64(tag.RowsAffected()),
	}
	return SuccessResult(data, duration, nil), nil
}

// queryAction runs a SELECT statement and returns all rows as a slice of maps.
func (a *PostgreSQLAdapter) queryAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	sql, queryParams, err := extractSQLParams(params)
	if err != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "query", err.Error(), err)
	}

	var rows []map[string]any
	duration, execErr := MeasureDuration(func() error {
		var e error
		rows, e = a.fetchRows(ctx, sql, queryParams)
		return e
	})
	if execErr != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "query", "query execution failed", execErr)
	}

	data := map[string]any{
		"rows":     rows,
		"rowCount": float64(len(rows)),
	}
	return SuccessResult(data, duration, nil), nil
}

// queryOneAction runs a SELECT and returns the first row. Returns an error when
// the result set is empty.
func (a *PostgreSQLAdapter) queryOneAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	sql, queryParams, err := extractSQLParams(params)
	if err != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "queryOne", err.Error(), err)
	}

	var rows []map[string]any
	duration, execErr := MeasureDuration(func() error {
		var e error
		rows, e = a.fetchRows(ctx, sql, queryParams)
		return e
	})
	if execErr != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "queryOne", "query execution failed", execErr)
	}
	if len(rows) == 0 {
		// With allowEmpty the absence of a row is a fact to assert on, not a step
		// error — this is how a test says "no such row exists" without having to
		// fall back to continueOnError and lose every other check in the step.
		if boolParam(params, "allowEmpty") {
			return SuccessResult(map[string]any{"found": false}, duration, nil), nil
		}
		return nil, tryve.AdapterError(postgresqlAdapterName, "queryOne",
			"query returned no rows; set allowEmpty: true to treat an empty result as a value to assert on", nil)
	}

	// Return the row columns at top level (matching TS behavior).
	// This allows capture paths like "id" to work as $.id.
	data := rows[0]
	if tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters) {
		if _, taken := data["found"]; !taken {
			data["found"] = true
		}
	}
	return SuccessResult(data, duration, nil), nil
}

// boolParam reads an optional boolean parameter, defaulting to false.
func boolParam(params map[string]any, key string) bool {
	b, _ := params[key].(bool)
	return b
}

// countAction runs a SELECT and returns only the number of rows produced.
func (a *PostgreSQLAdapter) countAction(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	sql, queryParams, err := extractSQLParams(params)
	if err != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "count", err.Error(), err)
	}

	var rows []map[string]any
	duration, execErr := MeasureDuration(func() error {
		var e error
		rows, e = a.fetchRows(ctx, sql, queryParams)
		return e
	})
	if execErr != nil {
		return nil, tryve.AdapterError(postgresqlAdapterName, "count", "query execution failed", execErr)
	}

	// Before the adapters area changed, count reported the number of rows the
	// query returned, so SELECT COUNT(*) always answered 1.
	countValue := float64(len(rows))
	if tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters) {
		countValue = scalarCount(rows)
	}

	data := map[string]any{
		"count":    countValue,
		"rowCount": float64(len(rows)),
		"rows":     rows,
	}
	return SuccessResult(data, duration, nil), nil
}

// scalarCount decides what "count" means for a result set.
//
// A query that aggregates — SELECT COUNT(*) — returns one row holding one
// numeric column, and the count the author wants is that value, not the number
// of rows the aggregate happened to occupy (always 1). Any other shape falls
// back to the number of rows returned.
func scalarCount(rows []map[string]any) float64 {
	if len(rows) == 1 {
		for _, v := range rows[0] {
			if len(rows[0]) != 1 {
				break
			}
			switch n := v.(type) {
			case int64:
				return float64(n)
			case int32:
				return float64(n)
			case int:
				return float64(n)
			case float64:
				return n
			}
		}
	}
	return float64(len(rows))
}

// fetchRows executes sql with args and collects all result rows into a slice of
// maps keyed by column name.
func (a *PostgreSQLAdapter) fetchRows(ctx context.Context, sql string, args []any) ([]map[string]any, error) {
	pgRows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer pgRows.Close()

	fieldDescs := pgRows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = string(fd.Name)
	}

	colOIDs := make([]uint32, len(fieldDescs))
	for i, fd := range fieldDescs {
		colOIDs[i] = fd.DataTypeOID
	}

	var result []map[string]any
	for pgRows.Next() {
		values, err := pgRows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(colNames))
		for i, name := range colNames {
			row[name] = normalizeColumn(values[i], colOIDs[i], tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters))
		}
		result = append(result, row)
	}
	if err := pgRows.Err(); err != nil {
		return nil, err
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// extractSQLParams reads the "sql" (required) and "params" (optional) keys from
// a params map. Returns an error when "sql" is absent or not a string.
func extractSQLParams(params map[string]any) (string, []any, error) {
	sqlVal, ok := params["sql"]
	if !ok {
		return "", nil, fmt.Errorf("required parameter \"sql\" is missing")
	}
	sql, ok := sqlVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("parameter \"sql\" must be a string, got %T", sqlVal)
	}
	if sql == "" {
		return "", nil, fmt.Errorf("parameter \"sql\" must not be empty")
	}

	var queryParams []any
	if p, ok := params["params"]; ok && p != nil {
		if slice, ok := p.([]any); ok {
			queryParams = slice
		}
	}

	return sql, queryParams, nil
}

// normalizeColumn converts one decoded column value into a JSON-friendly
// representation, using the column's PostgreSQL type OID for the cases where
// the Go type alone is ambiguous.
//
// The OID matters for DATE: pgx decodes both DATE and TIMESTAMP to time.Time, so
// without it a DATE column renders as "2026-08-31T00:00:00Z" and never compares
// equal to the "2026-08-31" a test author writes.
func normalizeColumn(v any, oid uint32, modern bool) any {
	if v == nil {
		return nil
	}
	if !modern {
		return legacyNormalizeValue(v)
	}
	if t, ok := v.(time.Time); ok && oid == pgtype.DateOID {
		return t.Format(dateOnlyLayout)
	}
	return normalizeValue(v)
}

// legacyNormalizeValue reproduces the conversion performed before the adapters
// area changed: uuid bytes, []byte, time.Time, net.IP, and fmt.Stringer were
// converted, and every other driver type — pgtype.Numeric among them — reached
// assertions as-is.
func legacyNormalizeValue(v any) any {
	switch val := v.(type) {
	case [16]byte:
		return formatUUIDBytes(val)
	case []byte:
		var js any
		if err := json.Unmarshal(val, &js); err == nil {
			return js
		}
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	case net.IP:
		return val.String()
	case fmt.Stringer:
		return val.String()
	default:
		return v
	}
}

// normalizeValue converts pgx-specific Go types into JSON-friendly representations
// so that captured values and assertions work as expected.
func normalizeValue(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case pgtype.Numeric:
		// NUMERIC / DECIMAL columns decode to a struct with no String method.
		// Left as-is it reaches assertions as an opaque struct, so `equals: 100`
		// fails and `greaterThan` silently compares against zero.
		return normalizeNumericValue(val)
	case pgtype.Interval:
		return formatInterval(val)
	case pgtype.UUID:
		if !val.Valid {
			return nil
		}
		return formatUUIDBytes(val.Bytes)
	case []any:
		// Array columns decode element-wise; normalise each element so a uuid[]
		// or numeric[] is as usable as its scalar counterpart.
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = normalizeValue(elem)
		}
		return out
	case [16]byte:
		return formatUUIDBytes(val)
	case []byte:
		// Try JSON first, then string
		var js any
		if err := json.Unmarshal(val, &js); err == nil {
			return js
		}
		return string(val)
	case time.Time:
		return val.Format(time.RFC3339)
	case net.IP:
		return val.String()
	case fmt.Stringer:
		return val.String()
	default:
		return v
	}
}

// formatUUIDBytes renders 16 raw bytes as a canonical UUID string.
func formatUUIDBytes(b [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// normalizeNumericValue converts a pgtype.Numeric into a plain float64 so that
// it compares and orders like any other number.
//
// Values that cannot be represented exactly as a float64 — anything outside
// ±2^53 or with more precision than a float64 holds — are returned as their
// decimal string instead, so a big NUMERIC is never silently rounded.
func normalizeNumericValue(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}
	if n.NaN {
		return "NaN"
	}
	if n.InfinityModifier != pgtype.Finite {
		return n.InfinityModifier.String()
	}

	text, err := n.MarshalJSON()
	if err != nil {
		return nil
	}
	str := strings.Trim(string(text), `"`)

	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return str
	}
	// Round-tripping proves the float64 carries the full value; when it does not,
	// hand back the exact decimal text rather than a lossy number.
	if strconv.FormatFloat(f, 'f', -1, 64) != str && !numericallyEqual(str, f) {
		return str
	}
	return f
}

// numericallyEqual reports whether the decimal text and the float64 denote the
// same value, allowing for differences in formatting (trailing zeros, exponents).
func numericallyEqual(text string, f float64) bool {
	exact, ok := new(big.Rat).SetString(text)
	if !ok {
		return false
	}
	fromFloat := new(big.Rat).SetFloat64(f)
	if fromFloat == nil {
		return false
	}
	return exact.Cmp(fromFloat) == 0
}

// formatInterval renders a pgtype.Interval as an ISO 8601 duration string,
// which is both human-readable and stable to assert against.
func formatInterval(iv pgtype.Interval) any {
	if !iv.Valid {
		return nil
	}
	var b strings.Builder
	b.WriteString("P")
	if iv.Months != 0 {
		fmt.Fprintf(&b, "%dM", iv.Months)
	}
	if iv.Days != 0 {
		fmt.Fprintf(&b, "%dD", iv.Days)
	}
	if iv.Microseconds != 0 {
		seconds := float64(iv.Microseconds) / 1e6
		fmt.Fprintf(&b, "T%gS", seconds)
	}
	if b.Len() == 1 {
		return "PT0S"
	}
	return b.String()
}

// dateOnlyLayout formats a DATE column without a time component.
const dateOnlyLayout = "2006-01-02"
