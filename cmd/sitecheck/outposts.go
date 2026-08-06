package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sitecheck/core"
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
	Sites      map[string]string // site name → detail level (level-only; membership is derived)
}

// scanOutposts reads outpost definitions from .lua files in dir.
func scanOutposts(dir string) ([]OutpostDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read outposts dir %q: %w", dir, err)
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

		def, err := loadOutpostDef(slug, filepath.Join(dir, entry.Name()))
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
	ok, err := core.CallMeta(l)
	if err != nil {
		return OutpostDef{}, fmt.Errorf("call meta() for %s: %w", slug, err)
	}
	if !ok {
		return def, nil
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
		// sites — site name → detail level. Level-only: it never adds the outpost to a site.
		l.Push("sites")
		if l.GetTableRaw(-2) == lua.TypTable {
			sites, err := readStringMap(l, -1)
			if err != nil {
				l.Pop(1)
				return OutpostDef{}, fmt.Errorf("meta() sites for %s: %w", slug, err)
			}
			def.Sites = sites
		}
		l.Pop(1)
	}
	l.Pop(1)

	return def, nil
}

// readStringMap reads a string→string table from the Lua value at tableIdx, erroring on any
// non-string key or value. Mirrors scoutpost's lmods.ReadStringMap for the core's own Lua use.
func readStringMap(l *lua.State, tableIdx int) (map[string]string, error) {
	abs := l.AbsIndex(tableIdx)
	result := make(map[string]string)
	var mapErr error
	l.ForEachRaw(abs, func() bool {
		k := l.GetRaw(-2)
		ks, ok := k.(string)
		if !ok {
			mapErr = fmt.Errorf("sites: table keys must be strings")
			return false
		}
		v := l.GetRaw(-1)
		vs, ok := v.(string)
		if !ok {
			mapErr = fmt.Errorf("sites: value for %q must be a string", ks)
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
