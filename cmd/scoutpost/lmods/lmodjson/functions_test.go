package lmodjson

import (
	"strings"
	"testing"

	"github.com/milochristiansen/lua"
)

func newState() *lua.State {
	l := lua.NewState()
	Open(l)  // registers json global, returns 1 (the module table)
	l.Pop(1) // discard module table
	return l
}

// execLua compiles and executes the given Lua source in a protected call.
// Returns the error from PCall, if any.
func execLua(l *lua.State, src string) error {
	if err := l.LoadText(strings.NewReader(src), "test", 0); err != nil {
		return err
	}
	return l.PCall(0, 0)
}

// execLua1 is like execLua but expects 1 return value left on the stack.
func execLua1(l *lua.State, src string) error {
	if err := l.LoadText(strings.NewReader(src), "test", 0); err != nil {
		return err
	}
	return l.PCall(0, 1)
}

func TestParseScalars(t *testing.T) {
	l := newState()

	tests := []struct {
		name, json string
		check      func(l *lua.State)
	}{
		{"null", "null", func(l *lua.State) {
			if !l.IsNil(-1) {
				t.Error("null should be nil")
			}
		}},
		{"true", "true", func(l *lua.State) {
			if !l.ToBool(-1) {
				t.Error("true should be true")
			}
		}},
		{"false", "false", func(l *lua.State) {
			if l.ToBool(-1) {
				t.Error("false should be false")
			}
		}},
		{"string", `"hello"`, func(l *lua.State) {
			if l.ToString(-1) != "hello" {
				t.Errorf("expected hello, got %s", l.ToString(-1))
			}
		}},
		{"integer", "42", func(l *lua.State) {
			if i, ok := l.TryInt(-1); !ok || i != 42 {
				t.Errorf("expected 42, got %v", l.GetRaw(-1))
			}
		}},
		{"float", "3.14", func(l *lua.State) {
			if f, ok := l.TryFloat(-1); !ok || f != 3.14 {
				t.Errorf("expected 3.14, got %v", l.GetRaw(-1))
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := execLua1(l, `return json.parse([[`+tt.json+`]])`); err != nil {
				t.Fatalf("execute: %v", err)
			}
			tt.check(l)
			l.Pop(1)
		})
	}
}

func TestParseArray(t *testing.T) {
	l := newState()

	if err := execLua1(l, `return json.parse('[1, "two", true, null]')`); err != nil {
		t.Fatal(err)
	}

	if l.TypeOf(-1) != lua.TypTable {
		t.Fatal("expected table")
	}

	if l.LengthRaw(-1) != 3 {
		t.Errorf("expected length 3 (null entry removed per Lua semantics), got %d", l.LengthRaw(-1))
	}

	l.Push(int64(1))
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 1 {
		t.Errorf("arr[1]: expected 1, got %v", l.GetRaw(-1))
	}
	l.Pop(1)

	l.Push(int64(2))
	l.GetTableRaw(-2)
	if l.ToString(-1) != "two" {
		t.Errorf("arr[2]: expected 'two', got %q", l.ToString(-1))
	}
	l.Pop(1)

	l.Push(int64(3))
	l.GetTableRaw(-2)
	if !l.ToBool(-1) {
		t.Error("arr[3]: expected true")
	}
	l.Pop(1)

	// Element 4 was null → entry removed per Lua semantics. Access returns nil.
	l.Push(int64(4))
	l.GetTableRaw(-2)
	if !l.IsNil(-1) {
		t.Error("arr[4]: expected nil (key deleted)")
	}
	l.Pop(1)
}

func TestParseObject(t *testing.T) {
	l := newState()

	if err := execLua1(l, `return json.parse('{"name":"test","count":5,"active":true}')`); err != nil {
		t.Fatal(err)
	}

	if l.TypeOf(-1) != lua.TypTable {
		t.Fatal("expected table")
	}

	l.Push("name")
	l.GetTableRaw(-2)
	if l.ToString(-1) != "test" {
		t.Errorf("obj.name: expected 'test', got %q", l.ToString(-1))
	}
	l.Pop(1)

	l.Push("count")
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 5 {
		t.Errorf("obj.count: expected 5, got %v", l.GetRaw(-1))
	}
	l.Pop(1)

	l.Push("active")
	l.GetTableRaw(-2)
	if !l.ToBool(-1) {
		t.Error("obj.active: expected true")
	}
	l.Pop(1)
}

func TestParseNested(t *testing.T) {
	l := newState()

	if err := execLua1(l, `return json.parse('{"items":[{"id":1},{"id":2}],"meta":{"version":"1.0"}}')`); err != nil {
		t.Fatal(err)
	}

	l.Push("items")
	l.GetTableRaw(-2)
	l.Push(int64(2))
	l.GetTableRaw(-2)
	l.Push("id")
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 2 {
		t.Errorf("items[2].id: expected 2, got %v", l.GetRaw(-1))
	}

	l.Pop(3)
	l.Push("meta")
	l.GetTableRaw(-2)
	l.Push("version")
	l.GetTableRaw(-2)
	if l.ToString(-1) != "1.0" {
		t.Errorf("meta.version: expected '1.0', got %q", l.ToString(-1))
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	l := newState()

	input := `{"a":1,"b":[2,3],"c":{"d":"hello"}}`

	if err := execLua1(l, `return json.encode(json.parse([[`+input+`]]))`); err != nil {
		t.Fatal(err)
	}

	encoded := l.ToString(-1)
	t.Logf("encoded: %s", encoded)

	l.Pop(1)
	if err := execLua1(l, `return json.parse([[`+encoded+`]])`); err != nil {
		t.Fatal(err)
	}

	l.Push("a")
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 1 {
		t.Errorf("roundtrip a: expected 1, got %v", l.GetRaw(-1))
	}
	l.Pop(1)

	l.Push("b")
	l.GetTableRaw(-2)
	l.Push(int64(1))
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 2 {
		t.Errorf("roundtrip b[1]: expected 2, got %v", l.GetRaw(-1))
	}
	l.Pop(2)

	l.Push("c")
	l.GetTableRaw(-2)
	l.Push("d")
	l.GetTableRaw(-2)
	if l.ToString(-1) != "hello" {
		t.Errorf("roundtrip c.d: expected 'hello', got %q", l.ToString(-1))
	}
}

func TestEncodeArray(t *testing.T) {
	l := newState()

	if err := execLua1(l, `return json.encode({10, 20, 30})`); err != nil {
		t.Fatal(err)
	}

	if l.ToString(-1) != "[10,20,30]" {
		t.Errorf("expected [10,20,30], got %q", l.ToString(-1))
	}
}

func TestEncodeMixedObject(t *testing.T) {
	l := newState()

	if err := execLua1(l, `local t = {a=1, b="x"}; return json.encode(t)`); err != nil {
		t.Fatal(err)
	}

	encoded := l.ToString(-1)
	l.Pop(1)

	if err := execLua1(l, `return json.parse([[`+encoded+`]])`); err != nil {
		t.Fatal(err)
	}

	l.Push("a")
	l.GetTableRaw(-2)
	if i, ok := l.TryInt(-1); !ok || i != 1 {
		t.Errorf("obj.a: expected 1, got %v", l.GetRaw(-1))
	}
	l.Pop(1)

	l.Push("b")
	l.GetTableRaw(-2)
	if l.ToString(-1) != "x" {
		t.Errorf("obj.b: expected 'x', got %q", l.ToString(-1))
	}
}

func TestParseError(t *testing.T) {
	l := newState()

	err := execLua(l, `return json.parse("not json")`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
