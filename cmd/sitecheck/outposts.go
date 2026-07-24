package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/milochristiansen/lua"
)

// OutpostDef holds the configuration for a single outpost.
type OutpostDef struct {
	Slug       string
	Name       string
	URL        string
	Token      string
	Skip       bool
	NotifyDown bool
}

// scanOutposts reads outpost definitions from outposts/*.lua. The local outpost is NOT included here — it is added
// unconditionally by the caller.
func scanOutposts() ([]OutpostDef, error) {
	entries, err := os.ReadDir("outposts")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read outposts dir: %w", err)
	}

	var outposts []OutpostDef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		// local.lua overrides the implicit local outpost, handled by caller.
		if entry.Name() == "local.lua" {
			continue
		}
		slug := entry.Name()[:len(entry.Name())-4] // strip .lua

		def, err := loadOutpostDef(slug, filepath.Join("outposts", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("outpost %s: %w", slug, err)
		}
		outposts = append(outposts, def)
	}

	return outposts, nil
}

// loadOutpostDef loads a single outpost definition script and calls meta().
func loadOutpostDef(slug, path string) (OutpostDef, error) {
	l := lua.NewState()
	l.Output = io.Discard

	src, err := os.ReadFile(path)
	if err != nil {
		return OutpostDef{}, fmt.Errorf("read %s: %w", path, err)
	}
	if err := l.LoadText(strings.NewReader(string(src)), path, 0); err != nil {
		return OutpostDef{}, fmt.Errorf("compile %s: %w", path, err)
	}
	if err := l.PCall(0, 0); err != nil {
		return OutpostDef{}, fmt.Errorf("execute %s: %w", path, err)
	}

	def := OutpostDef{
		Slug:       slug,
		Name:       slug,
		NotifyDown: true,
	}

	// Call meta() if it exists.
	l.Push("meta")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return def, nil
	}

	if err := l.Protect(func() { l.Call(0, 1) }); err != nil {
		return OutpostDef{}, fmt.Errorf("call meta() for %s: %w", slug, err)
	}

	if l.TypeOf(-1) == lua.TypTable {
		// name
		l.Push("name")
		if l.GetTableRaw(-2) == lua.TypString {
			def.Name = l.ToString(-1)
		}
		l.Pop(1)
		// url
		l.Push("url")
		if l.GetTableRaw(-2) == lua.TypString {
			def.URL = l.ToString(-1)
		}
		l.Pop(1)
		// token
		l.Push("token")
		if l.GetTableRaw(-2) == lua.TypString {
			def.Token = l.ToString(-1)
		}
		l.Pop(1)
		// skip
		l.Push("skip")
		if l.GetTableRaw(-2) == lua.TypBool {
			def.Skip = l.ToBool(-1)
		}
		l.Pop(1)
		// notify_down
		l.Push("notify_down")
		if l.GetTableRaw(-2) == lua.TypBool {
			def.NotifyDown = l.ToBool(-1)
		}
		l.Pop(1)
	}
	l.Pop(1)

	return def, nil
}
