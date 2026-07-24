// Package lmodjson provides a JSON module for the Lua VM.
// It registers a "json" global with parse and encode functions.
package lmodjson

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/milochristiansen/lua"
)

// Open registers the "json" module as a global table with parse/encode functions.
func Open(l *lua.State) int {
	l.NewTable(0, 2)
	tidx := l.AbsIndex(-1)

	l.SetTableFunctions(tidx, functions)

	l.Push("json")
	l.PushIndex(tidx)
	l.SetTableRaw(lua.GlobalsIndex)

	if l.AbsIndex(-1) != tidx {
		panic("lmodjson: stack misaligned")
	}
	return 1
}

var functions = map[string]lua.NativeFunction{
	"parse": func(l *lua.State) int {
		s := l.ToString(1)
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		var raw interface{}
		if err := dec.Decode(&raw); err != nil {
			l.Push(fmt.Sprintf("json.parse: %v", err))
			l.Error()
			return 0
		}
		pushJSONValue(l, raw)
		return 1
	},
	"encode": func(l *lua.State) int {
		v := luaEncodeToGo(l, 1)
		b, err := json.Marshal(v)
		if err != nil {
			l.Push(fmt.Sprintf("json.encode: %v", err))
			l.Error()
			return 0
		}
		l.Push(string(b))
		return 1
	},
}

// pushJSONValue walks a Go value (from json.Decode) and pushes the
// equivalent Lua value onto the stack.
func pushJSONValue(l *lua.State, v interface{}) {
	switch val := v.(type) {
	case nil:
		l.Push(nil)
	case bool:
		l.Push(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			l.Push(i)
		} else if f, err := val.Float64(); err == nil {
			l.Push(f)
		} else {
			l.Push(val.String())
		}
	case string:
		l.Push(val)
	case []interface{}:
		l.NewTable(len(val), 0)
		tab := l.AbsIndex(-1)
		for i, elem := range val {
			l.Push(int64(i + 1))
			pushJSONValue(l, elem)
			l.SetTableRaw(tab)
		}
	case map[string]interface{}:
		l.NewTable(0, len(val))
		tab := l.AbsIndex(-1)
		for k, elem := range val {
			l.Push(k)
			pushJSONValue(l, elem)
			l.SetTableRaw(tab)
		}
	}
}

// luaEncodeToGo converts the Lua value at idx to a Go value suitable for
// json.Marshal. Tables with sequential integer keys 1..N are encoded as
// []interface{}; all other tables become map[string]interface{}.
func luaEncodeToGo(l *lua.State, idx int) interface{} {
	idx = l.AbsIndex(idx)

	switch l.TypeOf(idx) {
	case lua.TypNil:
		return nil
	case lua.TypBool:
		return l.ToBool(idx)
	case lua.TypNumber:
		if i, ok := l.TryInt(idx); ok {
			return i
		}
		f, _ := l.TryFloat(idx)
		return f
	case lua.TypString:
		return l.ToString(idx)
	case lua.TypTable:
		return luaTableToGo(l, idx)
	default:
		return nil
	}
}

// luaTableToGo converts a Lua table to either []interface{} (if array-like)
// or map[string]interface{}.
func luaTableToGo(l *lua.State, idx int) interface{} {
	idx = l.AbsIndex(idx)

	// Collect all string keys and check array-ness.
	// A table is array-like when all keys are integers 1..N with no gaps.
	stringKeys := map[string]interface{}{}
	arrayKeys := map[int64]interface{}{}
	maxIdx := int64(0)

	l.ForEachRaw(idx, func() bool {
		k := l.GetRaw(-2)
		switch kk := k.(type) {
		case int64:
			arrayKeys[kk] = luaEncodeToGo(l, -1)
			if kk > maxIdx {
				maxIdx = kk
			}
		case float64:
			ik := int64(kk)
			arrayKeys[ik] = luaEncodeToGo(l, -1)
			if ik > maxIdx {
				maxIdx = ik
			}
		case string:
			stringKeys[kk] = luaEncodeToGo(l, -1)
		default:
			// Non-string, non-number keys -> force object encoding.
			stringKeys[fmt.Sprint(kk)] = luaEncodeToGo(l, -1)
		}
		return true
	})

	// Check if purely array-like: keys 1..maxIdx all present, no string keys.
	isArray := len(stringKeys) == 0 && int64(len(arrayKeys)) == maxIdx
	if isArray {
		arr := make([]interface{}, maxIdx)
		for i := int64(1); i <= maxIdx; i++ {
			arr[i-1] = arrayKeys[i]
		}
		return arr
	}

	// Otherwise, encode as object — merge all keys.
	obj := stringKeys
	for k, v := range arrayKeys {
		obj[fmt.Sprint(k)] = v
	}
	return obj
}
