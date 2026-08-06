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
	"sitecheck/core"
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

		l.Push(int64(core.PASS))
		l.SetGlobal("PASS")
		l.Push(int64(core.DEGRADED))
		l.SetGlobal("DEGRADED")
		l.Push(int64(core.FAIL))
		l.SetGlobal("FAIL")

		for _, p := range core.All() {
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
