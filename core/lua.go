package core

import (
	"fmt"

	"github.com/milochristiansen/lua"
)

// --- Lua table helpers ------------------------------------------------------

// ReadOptional reads a key from a Lua table, returning the raw Go value or nil.
func ReadOptional(l *lua.State, tableIdx int, key string) interface{} {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
	if t == lua.TypNil {
		return nil
	}
	v := l.GetRaw(-1)
	l.Pop(1)
	return v
}

// ReadStringOpt reads a string option from a Lua table, returning the default
// when the field is absent or not a string.
func ReadStringOpt(l *lua.State, tableIdx int, key string, def string) string {
	v := ReadOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return def
}

// ReadIntOpt reads an integer option from a Lua table, returning the default
// when the field is absent or not a number.
func ReadIntOpt(l *lua.State, tableIdx int, key string, def int) int {
	v := ReadOptional(l, tableIdx, key)
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

// ReadBoolOpt reads a boolean option from a Lua table, returning the default
// when the field is absent or not a boolean.
func ReadBoolOpt(l *lua.State, tableIdx int, key string, def bool) bool {
	v := ReadOptional(l, tableIdx, key)
	if v == nil {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}

// ReadStringMapOpt reads a string→string table option, silently skipping
// non-string keys or values. Returns nil when the field is absent, not a
// table, or empty.
func ReadStringMapOpt(l *lua.State, tableIdx int, key string) map[string]string {
	abs := l.AbsIndex(tableIdx)
	l.Push(key)
	t := l.GetTableRaw(abs)
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

// ReadStringField reads a string field from a Lua table, returning the default
// if the field is absent, nil, or not a string or number.
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

// ReadBoolField reads a boolean field from a Lua table at tableIdx, returning
// the default if the field is absent or not a boolean.
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

// ReadStringMap reads a string→string table field from a Lua table at tableIdx,
// returning an error if the field is present but is not a table or contains any
// non-string key or value. The field's value is left on the stack in all cases;
// the caller pops it.
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

// ReadStrSlice reads the array table at tableIdx as a []string, skipping
// non-string elements.
func ReadStrSlice(l *lua.State, tableIdx int) []string {
	idx := l.AbsIndex(tableIdx)
	var out []string
	l.ForEachRaw(idx, func() bool {
		v := l.GetRaw(-1)
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
		return true
	})
	return out
}

// CallMeta invokes the script's meta() function if one is defined. It returns
// false when the script defines no meta function (the stack is unchanged). On
// success the meta() result is left on the stack; on call error the stack is
// restored to its pre-call state and the error is returned.
func CallMeta(l *lua.State) (bool, error) {
	l.Push("meta")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return false, nil
	}
	if err := l.Protect(func() { l.Call(0, 1) }); err != nil {
		// Protect restores the stack to its pre-call state; drop the function slot.
		l.Pop(1)
		return true, err
	}
	return true, nil
}

// --- Check-result userdata --------------------------------------------------

// LuaField describes one field exposed on a check-result userdata's metatable.
// Get must push exactly one value. Set assigns the value at stack index 3 and
// is nil for read-only fields.
type LuaField struct {
	Get func(l *lua.State)
	Set func(l *lua.State)
}

// PushResultUserData pushes r as userdata with a metatable whose __index and
// __newindex dispatch to fields by name. Unknown fields read as nil and
// ignore writes.
func PushResultUserData(l *lua.State, r interface{}, fields map[string]LuaField) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		if f, ok := fields[l.ToString(2)]; ok {
			f.Get(l)
			return 1
		}
		l.Push(nil)
		return 1
	})
	l.SetTableRaw(-3)

	l.Push("__newindex")
	l.Push(func(l *lua.State) int {
		if f, ok := fields[l.ToString(2)]; ok && f.Set != nil {
			f.Set(l)
			return 0
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}
