package core

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

// --- ReadOptional tests ---

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
	if v := ReadOptional(l, idx, "strkey"); v != "hello" {
		t.Errorf("ReadOptional(strkey): expected %q, got %v", "hello", v)
	}
	// key exists — int64
	if v := ReadOptional(l, idx, "intkey"); v != int64(42) {
		t.Errorf("ReadOptional(intkey): expected int64(42), got %T(%v)", v, v)
	}
	// key exists — bool
	if v := ReadOptional(l, idx, "boolkey"); v != true {
		t.Errorf("ReadOptional(boolkey): expected true, got %v", v)
	}
	// key missing
	if v := ReadOptional(l, idx, "missing"); v != nil {
		t.Errorf("ReadOptional(missing): expected nil, got %v", v)
	}
	// key is nil — in Lua, setting a key to nil removes it, so semantically
	// equivalent to "key missing"; both paths return nil from ReadOptional.
	if v := ReadOptional(l, idx, "also_missing"); v != nil {
		t.Errorf("ReadOptional(also_missing): expected nil, got %v", v)
	}
}

// --- ReadStringOpt tests ---

func TestReadStringOpt(t *testing.T) {
	l := lua.NewState()
	setGlobalTable(l, map[string]interface{}{
		"name":   "hello",
		"number": int64(42),
	})
	idx := getCfgIdx(l)
	defer l.Pop(1)

	// string key exists
	if v := ReadStringOpt(l, idx, "name", "default"); v != "hello" {
		t.Errorf("ReadStringOpt(name): expected %q, got %q", "hello", v)
	}
	// key missing — returns default
	if v := ReadStringOpt(l, idx, "missing", "default"); v != "default" {
		t.Errorf("ReadStringOpt(missing): expected %q, got %q", "default", v)
	}
	// key is non-string type (int64) — returns default
	if v := ReadStringOpt(l, idx, "number", "default"); v != "default" {
		t.Errorf("ReadStringOpt(number): expected %q, got %q", "default", v)
	}
}

// --- ReadIntOpt tests ---

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
	if v := ReadIntOpt(l, idx, "n", -1); v != 42 {
		t.Errorf("ReadIntOpt(n): expected 42, got %d", v)
	}
	// zero int64
	if v := ReadIntOpt(l, idx, "z", -1); v != 0 {
		t.Errorf("ReadIntOpt(z): expected 0, got %d", v)
	}
	// negative int64
	if v := ReadIntOpt(l, idx, "neg", -1); v != -7 {
		t.Errorf("ReadIntOpt(neg): expected -7, got %d", v)
	}
	// float64 key exists — truncates to int
	if v := ReadIntOpt(l, idx, "f", -1); v != 3 {
		t.Errorf("ReadIntOpt(f): expected 3 (truncated), got %d", v)
	}
	// key missing — returns default
	if v := ReadIntOpt(l, idx, "missing", 99); v != 99 {
		t.Errorf("ReadIntOpt(missing): expected 99, got %d", v)
	}
	// key is non-numeric — string
	if v := ReadIntOpt(l, idx, "str", 99); v != 99 {
		t.Errorf("ReadIntOpt(str): expected 99, got %d", v)
	}
	// key is non-numeric — bool
	if v := ReadIntOpt(l, idx, "b", 99); v != 99 {
		t.Errorf("ReadIntOpt(b): expected 99, got %d", v)
	}
}

// --- ReadBoolOpt tests ---

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
	if v := ReadBoolOpt(l, idx, "yes", false); v != true {
		t.Errorf("ReadBoolOpt(yes): expected true, got %v", v)
	}
	// bool key exists — false
	if v := ReadBoolOpt(l, idx, "no", true); v != false {
		t.Errorf("ReadBoolOpt(no): expected false, got %v", v)
	}
	// key missing — returns default (true)
	if v := ReadBoolOpt(l, idx, "missing", true); v != true {
		t.Errorf("ReadBoolOpt(missing): expected true, got %v", v)
	}
	// key missing — returns default (false)
	if v := ReadBoolOpt(l, idx, "also_missing", false); v != false {
		t.Errorf("ReadBoolOpt(also_missing): expected false, got %v", v)
	}
	// key is non-bool — integer
	if v := ReadBoolOpt(l, idx, "num", false); v != false {
		t.Errorf("ReadBoolOpt(num): expected false, got %v", v)
	}
	// key is non-bool — string
	if v := ReadBoolOpt(l, idx, "str", true); v != true {
		t.Errorf("ReadBoolOpt(str): expected true, got %v", v)
	}
}

// --- ReadStringMapOpt tests ---

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
	m := ReadStringMapOpt(l, idx, "valid")
	if m == nil {
		t.Fatal("ReadStringMapOpt(valid): expected non-nil map")
	}
	if len(m) != 2 {
		t.Fatalf("ReadStringMapOpt(valid): expected 2 entries, got %d", len(m))
	}
	if m["a"] != "x" {
		t.Errorf(`ReadStringMapOpt(valid)["a"]: expected "x", got %q`, m["a"])
	}
	if m["b"] != "y" {
		t.Errorf(`ReadStringMapOpt(valid)["b"]: expected "y", got %q`, m["b"])
	}

	// empty table — no string→string entries → returns nil
	if m := ReadStringMapOpt(l, idx, "empty"); m != nil {
		t.Errorf("ReadStringMapOpt(empty): expected nil, got %v", m)
	}

	// key missing — returns nil
	if m := ReadStringMapOpt(l, idx, "missing"); m != nil {
		t.Errorf("ReadStringMapOpt(missing): expected nil, got %v", m)
	}

	// key is non-table — returns nil
	if m := ReadStringMapOpt(l, idx, "notable"); m != nil {
		t.Errorf("ReadStringMapOpt(notable): expected nil, got %v", m)
	}

	// key is table with a non-string value — k2=42 is int64, not string.
	// k1="v1" is valid string→string, so result has one entry.
	m = ReadStringMapOpt(l, idx, "mixed")
	if m == nil {
		t.Fatal("ReadStringMapOpt(mixed): expected non-nil map (k1=v1 is valid)")
	}
	if m["k1"] != "v1" {
		t.Errorf(`ReadStringMapOpt(mixed)["k1"]: expected "v1", got %q`, m["k1"])
	}
	if _, ok := m["k2"]; ok {
		t.Error("ReadStringMapOpt(mixed): k2 should not be present (non-string value)")
	}

	// table with string key "42" and string value "v" — this IS a valid
	// string→string pair, so we expect a non-nil result.
	m = ReadStringMapOpt(l, idx, "nonstringkey")
	if m == nil {
		t.Fatal("ReadStringMapOpt(nonstringkey): expected non-nil map (key \"42\" is a string)")
	}
	if m["42"] != "v" {
		t.Errorf(`ReadStringMapOpt(nonstringkey)["42"]: expected "v", got %q`, m["42"])
	}
}

// newTestTable pushes a new table with the given string-keyed fields onto the Lua stack and returns its absolute
// index. Fields with nil values are skipped (absent from the table).
func newTestTable(l *lua.State, fields map[string]interface{}) int {
	l.NewTable(0, len(fields))
	for k, v := range fields {
		if v == nil {
			continue
		}
		l.Push(k)
		l.Push(v)
		l.SetTableRaw(-3)
	}
	return l.AbsIndex(-1)
}

func TestLuaReadStringField(t *testing.T) {
	l := lua.NewState()

	tblIdx := newTestTable(l, map[string]interface{}{
		"name":  "hello",
		"count": int64(42),
		// "missing" key intentionally absent
		"nothing": nil, // explicitly skipped
	})

	tests := []struct {
		name string
		key  string
		def  string
		want string
	}{
		{name: "existing field", key: "name", def: "fallback", want: "hello"},
		{name: "missing field", key: "missing", def: "fallback", want: "fallback"},
		{name: "nil field", key: "nothing", def: "fallback", want: "fallback"},
		{name: "number field", key: "count", def: "fallback", want: "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadStringField(l, tblIdx, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("ReadStringField(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	l.Pop(1) // pop the table
}

func TestLuaReadStringFieldNonString(t *testing.T) {
	l := lua.NewState()

	tblIdx := newTestTable(l, map[string]interface{}{
		"enabled": true,
	})

	got := ReadStringField(l, tblIdx, "enabled", "fallback")
	if got != "fallback" {
		t.Errorf("ReadStringField(bool) = %q, want %q", got, "fallback")
	}

	l.Pop(1) // pop the table
}

func TestLuaReadBoolField(t *testing.T) {
	l := lua.NewState()

	tblIdx := newTestTable(l, map[string]interface{}{
		"enabled":  true,
		"disabled": false,
		// "missing" key intentionally absent
		"nothing": nil, // explicitly skipped
	})

	tests := []struct {
		name string
		key  string
		def  bool
		want bool
	}{
		{name: "existing true", key: "enabled", def: false, want: true},
		{name: "existing false", key: "disabled", def: true, want: false},
		{name: "missing field", key: "missing", def: true, want: true},
		{name: "nil field", key: "nothing", def: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReadBoolField(l, tblIdx, tt.key, tt.def)
			if got != tt.want {
				t.Errorf("ReadBoolField(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}

	l.Pop(1) // pop the table
}

func TestLuaReadBoolFieldNonBool(t *testing.T) {
	l := lua.NewState()

	tblIdx := newTestTable(l, map[string]interface{}{
		"name":    "test",
		"counter": int64(7),
	})

	got := ReadBoolField(l, tblIdx, "name", true)
	if got != true {
		t.Errorf("ReadBoolField(string) = %v, want default true", got)
	}

	got = ReadBoolField(l, tblIdx, "counter", false)
	if got != false {
		t.Errorf("ReadBoolField(number) = %v, want default false", got)
	}

	l.Pop(1) // pop the table
}
