package core

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"reflect"
	"time"
)

// ContentHash returns a short, stable digest of a stored text value. An empty
// string hashes like any other value, so empty content compares equal to other
// empty content. The digest is only used for equality (elision); it is not a
// security boundary. SHA-256 truncated to 128 bits is cheap and makes deliberate
// collisions impractical for attacker-influenced bodies.
//
// A nil *string (NULL hash column) is the only "unknown" marker: it means the row
// predates hashing and must never be treated as similar to anything.
func ContentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16]) // 32 hex chars
}

// QueryPoints reads the narrow numeric history (pass, response time, run
// duration, timestamp) for a slug from table. No large text columns are touched.
// Results are chronological (oldest first). A zero since means "no lower bound".
// A positive limit returns the most recent limit rows (still chronological).
//
// Every check table shares this column shape, which is what lets the whole fleet
// share one implementation.
func QueryPoints(db *sql.DB, table, slug, outpostSlug string, since time.Time, limit int) ([]CheckPoint, error) {
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")
	const cols = "pass, response_time_ms, duration_ms, timestamp"

	var (
		q    string
		args []interface{}
	)
	if limit > 0 {
		q = fmt.Sprintf("SELECT %s FROM %s WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp DESC LIMIT ?", cols, table)
		args = []interface{}{slug, outpostSlug, sinceStr, limit}
	} else {
		q = fmt.Sprintf("SELECT %s FROM %s WHERE slug = ? AND outpost_slug = ? AND timestamp >= ? ORDER BY timestamp", cols, table)
		args = []interface{}{slug, outpostSlug, sinceStr}
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query points from %s: %w", table, err)
	}
	defer rows.Close()

	var pts []CheckPoint
	for rows.Next() {
		var (
			pass int
			resp sql.NullFloat64
			dur  sql.NullInt64
			ts   string
		)
		if err := rows.Scan(&pass, &resp, &dur, &ts); err != nil {
			return nil, fmt.Errorf("scan point from %s: %w", table, err)
		}
		pts = append(pts, CheckPoint{Pass: pass, Resp: resp.Float64, Duration: float64(dur.Int64), TS: ts})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows points from %s: %w", table, err)
	}

	if limit > 0 {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	return pts, nil
}

// similarIgnoreFields are check-row fields that don't affect whether two checks
// are the same event: row identity and the run/response timings. Every other
// field (pass status, fail reason, URL, status code, ...) must match for checks
// to be elidable.
var similarIgnoreFields = map[string]bool{
	"ID":             true,
	"Timestamp":      true,
	"DurationMS":     true,
	"ResponseTimeMS": true,
	"MinMS":          true, // ping RTT stats — timing
	"MaxMS":          true, // ping RTT stats — timing
}

// Similar reports whether two typed check rows are the same event apart from
// identity/run timing. Large text fields are compared through their hash field
// (struct tag `hash:"..."`), so callers can pass "light" rows that omit the
// large columns. A nil hash is unknown and never matches, not even another nil.
func Similar(a, b interface{}) bool {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)
	if !av.IsValid() || !bv.IsValid() || av.Type() != bv.Type() {
		return false
	}
	// Callers normally pass row values, but tolerate matching pointers too.
	if av.Kind() == reflect.Ptr {
		av, bv = av.Elem(), bv.Elem()
		if !av.IsValid() || !bv.IsValid() {
			return false
		}
	}
	t := av.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if similarIgnoreFields[f.Name] {
			continue
		}
		if hashField, ok := f.Tag.Lookup("hash"); ok {
			ah := av.FieldByName(hashField)
			bh := bv.FieldByName(hashField)
			if !ah.IsValid() || !bh.IsValid() || !hashEqual(ah.Interface(), bh.Interface()) {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(av.Field(i).Interface(), bv.Field(i).Interface()) {
			return false
		}
	}
	return true
}

// hashEqual compares two hash fields. A nil hash means "unknown" and never
// matches anything, not even another nil. A non-nil digest is a real value
// (including ContentHash("") for empty content) and compares by value.
func hashEqual(a, b interface{}) bool {
	ap, aok := a.(*string)
	bp, bok := b.(*string)
	return aok && bok && ap != nil && bp != nil && *ap == *bp
}
