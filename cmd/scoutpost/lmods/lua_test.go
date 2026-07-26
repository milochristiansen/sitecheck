package lmods

import (
	"testing"

	"github.com/milochristiansen/lua"
)

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

func TestLuaNewState(t *testing.T) {
	l, err := NewState(30)
	if err != nil {
		t.Fatalf("NewState returned error: %v", err)
	}
	if l == nil {
		t.Fatal("NewState returned nil state")
	}

	// Verify PASS global = 2
	l.Push("PASS")
	typ := l.GetTableRaw(lua.GlobalsIndex)
	if typ != lua.TypNumber {
		t.Fatalf("PASS global type = %v, want TypNumber", typ)
	}
	if got := l.ToInt(-1); got != 2 {
		t.Errorf("PASS = %d, want 2", got)
	}
	l.Pop(1)

	// Verify FAIL global = 0
	l.Push("FAIL")
	typ = l.GetTableRaw(lua.GlobalsIndex)
	if typ != lua.TypNumber {
		t.Fatalf("FAIL global type = %v, want TypNumber", typ)
	}
	if got := l.ToInt(-1); got != 0 {
		t.Errorf("FAIL = %d, want 0", got)
	}
	l.Pop(1)

	// Verify DEGRADED global = 1
	l.Push("DEGRADED")
	typ = l.GetTableRaw(lua.GlobalsIndex)
	if typ != lua.TypNumber {
		t.Fatalf("DEGRADED global type = %v, want TypNumber", typ)
	}
	if got := l.ToInt(-1); got != 1 {
		t.Errorf("DEGRADED = %d, want 1", got)
	}
	l.Pop(1)
}

func TestLuaPushStrSlice(t *testing.T) {
	l := lua.NewState()

	strs := []string{"alpha", "beta", "gamma"}
	pushStrSlice(l, strs)

	if typ := l.TypeOf(-1); typ != lua.TypTable {
		t.Fatalf("pushStrSlice result type = %v, want TypTable", typ)
	}

	// Verify table length
	if n := l.Length(-1); n != len(strs) {
		t.Errorf("table length = %d, want %d", n, len(strs))
	}

	// Verify each element (1-indexed)
	for i, want := range strs {
		idx := int64(i + 1)
		l.Push(idx)
		typ := l.GetTableRaw(-2) // table at -2, key popped, value pushed
		if typ != lua.TypString {
			t.Errorf("element %d type = %v, want TypString", i+1, typ)
		}
		if got := l.ToString(-1); got != want {
			t.Errorf("element %d = %q, want %q", i+1, got, want)
		}
		l.Pop(1) // pop the string value
	}

	l.Pop(1) // pop the table
}

func TestLuaPushStrSliceEmpty(t *testing.T) {
	l := lua.NewState()

	pushStrSlice(l, []string{})

	if typ := l.TypeOf(-1); typ != lua.TypTable {
		t.Fatalf("pushStrSlice empty result type = %v, want TypTable", typ)
	}
	if n := l.Length(-1); n != 0 {
		t.Errorf("empty table length = %d, want 0", n)
	}
	l.Pop(1)
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
		name     string
		key      string
		def      string
		want     string
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
		"enabled": true,
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
