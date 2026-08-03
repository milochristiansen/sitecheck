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
	"github.com/milochristiansen/lua/lmodutf8"

	"sitecheck/cmd/scoutpost/lmods/lmodjson"
	"sitecheck/checktypes/registry"
	"sitecheck/protocol"
)


// NewState creates a Lua state with standard libraries, pass constants, and all check modules registered. The Lua
// stdout is discarded.
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
		l.Push(lmodutf8.Open)
		l.Call(0, 0)
		l.Push(lmodjson.Open)
		l.Call(0, 0)

		l.Push(int64(protocol.PASS))
		l.SetGlobal("PASS")
		l.Push(int64(protocol.DEGRADED))
		l.SetGlobal("DEGRADED")
		l.Push(int64(protocol.FAIL))
		l.SetGlobal("FAIL")

		for _, p := range registry.All() {
			p.RegisterLua(l, defaultTimeout)
		}
	})

	return l, err
}

// ExecuteFile loads and runs a Lua script file. After this call, any global functions defined by the script (meta,
// check) are available.
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

// ReadStringField reads a string field from a Lua table, returning the default if the field is absent or nil.
func ReadStringField(l *lua.State, tableIdx int, key string, def string) string {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		l.Pop(1)
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

// ReadBoolField reads a boolean field from a Lua table at tableIdx, returning the default if the field is absent or not
// a boolean.
func ReadBoolField(l *lua.State, tableIdx int, key string, def bool) bool {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		l.Pop(1)
		return def
	}
	if t == lua.TypBool {
		v := l.ToBool(-1)
		l.Pop(1)
		return v
	}
	l.Pop(1)
	return def
}

// ReadStringMap reads a string→string table field from a Lua table at tableIdx, returning an
// error if the field is present but is not a table, or contains any non-string key or value.
// The field's value is left on the stack in all cases; the caller pops it.
func ReadStringMap(l *lua.State, tableIdx int, key string) (map[string]string, error) {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		l.Pop(1)
		return nil, nil
	}
	if t != lua.TypTable {
		l.Pop(1)
		return nil, fmt.Errorf("%s: %q must be a table", key, key)
	}

	result := make(map[string]string)
	subIdx := l.AbsIndex(-1)
	var mapErr error
	l.ForEachRaw(subIdx, func() bool {
		k := l.GetRaw(-2)
		ks, ok := k.(string)
		if !ok {
			mapErr = fmt.Errorf("%s: table keys must be strings", key)
			return false
		}
		v := l.GetRaw(-1)
		vs, ok := v.(string)
		if !ok {
			mapErr = fmt.Errorf("%s: value for %q must be a string", key, ks)
			return false
		}
		result[ks] = vs
		return true
	})
	if mapErr != nil {
		return nil, mapErr
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
