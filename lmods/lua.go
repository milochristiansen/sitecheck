package lmods

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/milochristiansen/lua"
	"github.com/milochristiansen/lua/lmodbase"
	"github.com/milochristiansen/lua/lmodmath"
	"github.com/milochristiansen/lua/lmodpackage"
	"github.com/milochristiansen/lua/lmodstring"
	"github.com/milochristiansen/lua/lmodtable"
)

// Pass level constants exposed to Lua scripts.
const (
	FAIL     = 0
	DEGRADED = 1
	PASS     = 2
)

// NewState creates a Lua state with standard libraries, pass constants, and all
// check modules registered. The Lua stdout is discarded.
func NewState(defaultTimeout int) (*lua.State, error) {
	l := lua.NewState()
	l.Output = io.Discard

	err := l.Protect(func() {
		l.Push(lmodbase.Open)
		l.Call(0, 0)
		l.Push(lmodpackage.Open)
		l.Call(0, 0)
		l.Push(lmodstring.Open)
		l.Call(0, 0)
		l.Push(lmodtable.Open)
		l.Call(0, 0)
		l.Push(lmodmath.Open)
		l.Call(0, 0)

		l.Push(int64(PASS))
		l.SetGlobal("PASS")
		l.Push(int64(DEGRADED))
		l.SetGlobal("DEGRADED")
		l.Push(int64(FAIL))
		l.SetGlobal("FAIL")

		registerHTTP(l, defaultTimeout)
		registerPing(l, defaultTimeout)
		registerTCP(l, defaultTimeout)
		registerDNS(l, defaultTimeout)
		registerSSL(l, defaultTimeout)
	})

	return l, err
}

// ExecuteFile loads and runs a Lua script file. After this call, any global
// functions defined by the script (meta, check) are available.
func ExecuteFile(l *lua.State, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read script %s: %w", path, err)
	}

	if err := l.LoadText(strings.NewReader(string(src)), path, 0); err != nil {
		return fmt.Errorf("compile %s: %w", path, err)
	}
	if err := l.PCall(0, 0); err != nil {
		return fmt.Errorf("execute %s: %w", path, err)
	}
	return nil
}
// pushStrSlice pushes a []string as a Lua array table (1-indexed).
func pushStrSlice(l *lua.State, strs []string) {
	l.NewTable(len(strs), 0)
	for i, s := range strs {
		l.Push(int64(i + 1))
		l.Push(s)
		l.SetTableRaw(-3)
	}
}

// ReadStringField reads a string field from a Lua table, returning the default
// if the field is absent or nil.
func ReadStringField(l *lua.State, tableIdx int, key string, def string) string {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		return def
	}
	if t == lua.TypString || t == lua.TypNumber {
		v := l.ToString(-1)
		l.Pop(1)
		return v
	}
	l.Pop(1)
	return def
}
