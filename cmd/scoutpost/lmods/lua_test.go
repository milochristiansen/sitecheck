package lmods

import (
	"testing"

	"github.com/milochristiansen/lua"
)

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
