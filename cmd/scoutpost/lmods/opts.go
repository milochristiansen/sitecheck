package lmods

import "github.com/milochristiansen/lua"

// readOptional reads a key from a Lua table, returning the raw Go value or nil.
func readOptional(l *lua.State, tableIdx int, key string) interface{} {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil {
		return nil
	}
	v := l.GetRaw(-1)
	l.Pop(1)
	return v
}

func readStringOpt(l *lua.State, tableIdx int, key string, def string) string {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

func readIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

func readBoolOpt(l *lua.State, tableIdx int, key string, def bool) bool {
	v := readOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

func readStringMapOpt(l *lua.State, tableIdx int, key string) map[string]string {
	l.Push(key)
	t := l.GetTableRaw(tableIdx)
	if t == lua.TypNil || t != lua.TypTable {
		l.Pop(1)
		return nil
	}

	result := make(map[string]string)
	subIdx := l.AbsIndex(-1)

	l.ForEachRaw(subIdx, func() bool {
		k := l.GetRaw(-2)
		if ks, ok := k.(string); ok {
			v := l.GetRaw(-1)
			if vs, ok := v.(string); ok {
				result[ks] = vs
			}
		}
		return true
	})

	l.Pop(1)
	if len(result) == 0 {
		return nil
	}
	return result
}
