package lmods

import (
	"testing"

	"github.com/milochristiansen/lua"
)

// pushValue pushes a Go value onto the Lua stack. Supports string, int64,
// float64, bool, and map[string]interface{} (converted to a Lua table).
func pushValue(l *lua.State, v interface{}) {
	switch val := v.(type) {
	case string:
		l.Push(val)
	case int64:
		l.Push(val)
	case float64:
		l.Push(val)
	case bool:
		l.Push(val)
	case map[string]interface{}:
		l.NewTable(0, len(val))
		subIdx := l.AbsIndex(-1)
		for sk, sv := range val {
			l.Push(sk) // key
			pushValue(l, sv)
			l.SetTableRaw(subIdx)
		}
	default:
		panic("unsupported test value type")
	}
}

// setGlobalTable creates a global table named "cfg" with key-value pairs.
// The table remains on the stack at the top; caller should Pop when done or
// use getCfgIdx to retrieve it from globals.
func setGlobalTable(l *lua.State, entries map[string]interface{}) int {
	l.NewTable(0, len(entries))
	idx := l.AbsIndex(-1)
	for k, v := range entries {
		l.Push(k) // key
		pushValue(l, v)
		l.SetTableRaw(idx) // table[key] = value; pops key and value
	}
	l.Push("cfg")
	l.PushIndex(idx)
	l.SetTableRaw(lua.GlobalsIndex)
	return idx
}

// getCfgIdx retrieves the "cfg" global table and returns its absolute index.
// The value is left on the stack; caller should Pop(1) when done.
func getCfgIdx(l *lua.State) int {
	l.Push("cfg")
	l.GetTableRaw(lua.GlobalsIndex)
	return l.AbsIndex(-1)
}

// --- readOptional tests ---

func TestReadOptional(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"strkey":  "hello",
		"intkey":  int64(42),
		"boolkey": true,
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// key exists — string
	if v := readOptional(l, idx, "strkey"); v != "hello" {
		t.Errorf("readOptional(strkey): expected %q, got %v", "hello", v)
	}
	// key exists — int64
	if v := readOptional(l, idx, "intkey"); v != int64(42) {
		t.Errorf("readOptional(intkey): expected int64(42), got %T(%v)", v, v)
	}
	// key exists — bool
	if v := readOptional(l, idx, "boolkey"); v != true {
		t.Errorf("readOptional(boolkey): expected true, got %v", v)
	}
	// key missing
	if v := readOptional(l, idx, "missing"); v != nil {
		t.Errorf("readOptional(missing): expected nil, got %v", v)
	}
	// key is nil — in Lua, setting a key to nil removes it, so semantically
	// equivalent to "key missing"; both paths return nil from readOptional.
	if v := readOptional(l, idx, "also_missing"); v != nil {
		t.Errorf("readOptional(also_missing): expected nil, got %v", v)
	}
}

// --- readStringOpt tests ---

func TestReadStringOpt(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"name":   "hello",
		"number": int64(42),
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// string key exists
	if v := readStringOpt(l, idx, "name", "default"); v != "hello" {
		t.Errorf("readStringOpt(name): expected %q, got %q", "hello", v)
	}
	// key missing — returns default
	if v := readStringOpt(l, idx, "missing", "default"); v != "default" {
		t.Errorf("readStringOpt(missing): expected %q, got %q", "default", v)
	}
	// key is non-string type (int64) — returns default
	if v := readStringOpt(l, idx, "number", "default"); v != "default" {
		t.Errorf("readStringOpt(number): expected %q, got %q", "default", v)
	}
}

// --- readIntOpt tests ---

func TestReadIntOpt(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"z":   int64(0),
		"n":   int64(42),
		"neg": int64(-7),
		"f":   float64(3.14),
		"str": "oops",
		"b":   true,
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// int64 key exists
	if v := readIntOpt(l, idx, "n", -1); v != 42 {
		t.Errorf("readIntOpt(n): expected 42, got %d", v)
	}
	// zero int64
	if v := readIntOpt(l, idx, "z", -1); v != 0 {
		t.Errorf("readIntOpt(z): expected 0, got %d", v)
	}
	// negative int64
	if v := readIntOpt(l, idx, "neg", -1); v != -7 {
		t.Errorf("readIntOpt(neg): expected -7, got %d", v)
	}
	// float64 key exists — truncates to int
	if v := readIntOpt(l, idx, "f", -1); v != 3 {
		t.Errorf("readIntOpt(f): expected 3 (truncated), got %d", v)
	}
	// key missing — returns default
	if v := readIntOpt(l, idx, "missing", 99); v != 99 {
		t.Errorf("readIntOpt(missing): expected 99, got %d", v)
	}
	// key is non-numeric — string
	if v := readIntOpt(l, idx, "str", 99); v != 99 {
		t.Errorf("readIntOpt(str): expected 99, got %d", v)
	}
	// key is non-numeric — bool
	if v := readIntOpt(l, idx, "b", 99); v != 99 {
		t.Errorf("readIntOpt(b): expected 99, got %d", v)
	}
}

// --- readBoolOpt tests ---

func TestReadBoolOpt(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"yes": true,
		"no":  false,
		"num": int64(1),
		"str": "true",
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// bool key exists — true
	if v := readBoolOpt(l, idx, "yes", false); v != true {
		t.Errorf("readBoolOpt(yes): expected true, got %v", v)
	}
	// bool key exists — false
	if v := readBoolOpt(l, idx, "no", true); v != false {
		t.Errorf("readBoolOpt(no): expected false, got %v", v)
	}
	// key missing — returns default (true)
	if v := readBoolOpt(l, idx, "missing", true); v != true {
		t.Errorf("readBoolOpt(missing): expected true, got %v", v)
	}
	// key missing — returns default (false)
	if v := readBoolOpt(l, idx, "also_missing", false); v != false {
		t.Errorf("readBoolOpt(also_missing): expected false, got %v", v)
	}
	// key is non-bool — integer
	if v := readBoolOpt(l, idx, "num", false); v != false {
		t.Errorf("readBoolOpt(num): expected false, got %v", v)
	}
	// key is non-bool — string
	if v := readBoolOpt(l, idx, "str", true); v != true {
		t.Errorf("readBoolOpt(str): expected true, got %v", v)
	}
}

// --- readStringMapOpt tests ---

func TestReadStringMapOpt(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"valid": map[string]interface{}{
			"a": "x",
			"b": "y",
		},
		"empty":   map[string]interface{}{},
		"notable": "just a string",
		"mixed": map[string]interface{}{
			"k1": "v1",
			"k2": int64(42),
		},
		"nonstringkey": map[string]interface{}{
			"42": "v",
		},
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// valid string→string table
	m := readStringMapOpt(l, idx, "valid")
	if m == nil {
		t.Fatal("readStringMapOpt(valid): expected non-nil map")
	}
	if len(m) != 2 {
		t.Fatalf("readStringMapOpt(valid): expected 2 entries, got %d", len(m))
	}
	if m["a"] != "x" {
		t.Errorf(`readStringMapOpt(valid)["a"]: expected "x", got %q`, m["a"])
	}
	if m["b"] != "y" {
		t.Errorf(`readStringMapOpt(valid)["b"]: expected "y", got %q`, m["b"])
	}

	// empty table — no string→string entries → returns nil
	if m := readStringMapOpt(l, idx, "empty"); m != nil {
		t.Errorf("readStringMapOpt(empty): expected nil, got %v", m)
	}

	// key missing — returns nil
	if m := readStringMapOpt(l, idx, "missing"); m != nil {
		t.Errorf("readStringMapOpt(missing): expected nil, got %v", m)
	}

	// key is non-table — returns nil
	if m := readStringMapOpt(l, idx, "notable"); m != nil {
		t.Errorf("readStringMapOpt(notable): expected nil, got %v", m)
	}

	// key is table with a non-string value — k2=42 is int64, not string.
	// k1="v1" is valid string→string, so result has one entry.
	m = readStringMapOpt(l, idx, "mixed")
	if m == nil {
		t.Fatal("readStringMapOpt(mixed): expected non-nil map (k1=v1 is valid)")
	}
	if m["k1"] != "v1" {
		t.Errorf(`readStringMapOpt(mixed)["k1"]: expected "v1", got %q`, m["k1"])
	}
	if _, ok := m["k2"]; ok {
		t.Error("readStringMapOpt(mixed): k2 should not be present (non-string value)")
	}

	// table with string key "42" and string value "v" — this IS a valid
	// string→string pair, so we expect a non-nil result.
	m = readStringMapOpt(l, idx, "nonstringkey")
	if m == nil {
		t.Fatal("readStringMapOpt(nonstringkey): expected non-nil map (key \"42\" is a string)")
	}
	if m["42"] != "v" {
		t.Errorf(`readStringMapOpt(nonstringkey)["42"]: expected "v", got %q`, m["42"])
	}
}
