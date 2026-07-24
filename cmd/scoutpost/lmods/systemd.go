package lmods

import (
	"context"
	"sync"
	"time"

	systemdDbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/milochristiansen/lua"
	"sitecheck/protocol"
)

var (
	systemdConn     *systemdDbus.Conn
	systemdConnOnce sync.Once
	systemdConnErr  error
)

func getSystemdConn() (*systemdDbus.Conn, error) {
	systemdConnOnce.Do(func() {
		systemdConn, systemdConnErr = systemdDbus.New()
	})
	return systemdConn, systemdConnErr
}

func registerSystemd(l *lua.State, defaultTimeout int) {
	l.Push(func(l *lua.State) int {
		serviceName := l.ToString(1)

		timeout := defaultTimeout
		if !l.IsNil(2) && l.TypeOf(2) == lua.TypTable {
			timeout = readIntOpt(l, 2, "timeout", defaultTimeout)
		}

		r := &protocol.SystemdResult{
			Pass:        protocol.FAIL,
			ServiceName: serviceName,
		}

		conn, err := getSystemdConn()
		if err != nil {
			r.Error = "systemd dbus connection failed: " + err.Error()
			pushSystemdResult(l, r)
			return 1
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		start := time.Now()

		// Query unit properties via D-Bus.
		props, err := conn.GetUnitPropertiesContext(ctx, serviceName)
		elapsed := time.Since(start)
		r.ResponseTimeMS = float64(elapsed.Microseconds()) / 1000.0

		if err != nil {
			r.Error = err.Error()
			pushSystemdResult(l, r)
			return 1
		}

		if v, ok := props["ActiveState"]; ok {
			r.ActiveState = dbusValueString(v)
		}
		if v, ok := props["SubState"]; ok {
			r.SubState = dbusValueString(v)
		}
		if v, ok := props["LoadState"]; ok {
			r.LoadState = dbusValueString(v)
		}
		if v, ok := props["MainPID"]; ok {
			r.MainPID = dbusValueInt(v)
		}

		pushSystemdResult(l, r)
		return 1
	})
	l.SetGlobal("systemd_check")
}

func dbusValueString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func dbusValueInt(v interface{}) int {
	switch n := v.(type) {
	case uint32:
		return int(n)
	case int32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func pushSystemdResult(l *lua.State, r *protocol.SystemdResult) {
	l.Push(r)
	l.NewTable(0, 2)

	l.Push("__index")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*protocol.SystemdResult)
		switch l.ToString(2) {
		case "Pass":
			l.Push(int64(r.Pass))
		case "FailReason":
			pushStr(l, r.FailReason)
		case "ServiceName":
			l.Push(r.ServiceName)
		case "ActiveState":
			pushStr(l, r.ActiveState)
		case "SubState":
			pushStr(l, r.SubState)
		case "LoadState":
			pushStr(l, r.LoadState)
		case "MainPID":
			l.Push(int64(r.MainPID))
		case "ResponseTimeMS":
			l.Push(r.ResponseTimeMS)
		case "Error":
			pushStr(l, r.Error)
		default:
			l.Push(nil)
		}
		return 1
	})
	l.SetTableRaw(-3)

	l.Push("__newindex")
	l.Push(func(l *lua.State) int {
		r := l.ToUser(1).(*protocol.SystemdResult)
		switch l.ToString(2) {
		case "Pass":
			r.Pass = int(l.ToInt(3))
		case "FailReason":
			r.FailReason = l.ToString(3)
		case "ServiceName":
			r.ServiceName = l.ToString(3)
		case "ActiveState":
			r.ActiveState = l.ToString(3)
		case "SubState":
			r.SubState = l.ToString(3)
		case "LoadState":
			r.LoadState = l.ToString(3)
		case "MainPID":
			r.MainPID = int(l.ToInt(3))
		case "ResponseTimeMS":
			r.ResponseTimeMS = l.ToFloat(3)
		case "Error":
			r.Error = l.ToString(3)
		}
		return 0
	})
	l.SetTableRaw(-3)

	l.SetMetaTable(-2)
}
