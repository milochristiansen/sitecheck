package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/milochristiansen/lua"

	"sitecheck/db"
	"sitecheck/lmods"
)

// Resource represents a single check script discovered in the resources directory.
type Resource struct {
	Slug           string
	ScriptPath     string
	Name           string
	Desc           string
	NotifyPass     string
	NotifyDegraded string
	NotifyFail     string
}

// Result holds the outcome of a single check execution.
// Raw carries the typed result struct (e.g. *lmods.HTTPResult) on success;
// Err is non-empty when check() itself failed before producing a typed result.
type Result struct {
	Slug           string
	Name           string
	Desc           string
	Raw            interface{}
	Err            string
	Elapsed        time.Duration
	History        interface{} // typed DB check slice, populated by collector
	NotifyPass     string
	NotifyDegraded string
	NotifyFail     string
}

// ScanResources finds all .lua files in dir and returns Resources with slugs
// derived from filenames. Names and descriptions are not yet populated.
func ScanResources(dir string) ([]Resource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read resources dir %s: %w", dir, err)
	}

	var resources []Resource
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".lua") {
			continue
		}
		slug := strings.TrimSuffix(entry.Name(), ".lua")
		resources = append(resources, Resource{
			Slug:       slug,
			ScriptPath: filepath.Join(dir, entry.Name()),
			Name:       titleCase(slug),
			Desc:       "",
		})
	}
	return resources, nil
}

// PopulateMeta executes the script's meta() function if present, filling in
// Name and Description on the resource.
// The script must already be loaded by the caller (via ExecuteFile).
func PopulateMeta(l *lua.State, res *Resource) error {
	// Check if meta() exists as a global function.
	l.Push("meta")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return nil
	}

	err := l.Protect(func() {
		l.Call(0, 1)
	})
	if err != nil {
		l.Pop(1)
		return fmt.Errorf("call meta() for %s: %w", res.Slug, err)
	}

	// Result is a table at TOS. Read name, description, and notify topics.
	if l.TypeOf(-1) == lua.TypTable {
		name := lmods.ReadStringField(l, -1, "name", "")
		desc := lmods.ReadStringField(l, -1, "description", "")
		if name != "" {
			res.Name = name
		}
		if desc != "" {
			res.Desc = desc
		}

		// Read notify sub-table if present.
		l.Push("notify")
		if l.GetTableRaw(-2) == lua.TypTable {
			res.NotifyPass = lmods.ReadStringField(l, -1, "pass", "")
			res.NotifyDegraded = lmods.ReadStringField(l, -1, "degraded", "")
			res.NotifyFail = lmods.ReadStringField(l, -1, "fail", "")
		}
		l.Pop(1) // pop notify table or nil
	}
	l.Pop(1)
	return nil
}

// RunCheck executes a script's check() function and returns a Result.
// The state must already have the script loaded.
func RunCheck(l *lua.State, res Resource) (Result, error) {
	l.Push("check")
	t := l.GetTableRaw(lua.GlobalsIndex)
	if t == lua.TypNil || t != lua.TypFunction {
		l.Pop(1)
		return Result{}, fmt.Errorf("resource %s: check() function not found", res.Slug)
	}

	start := time.Now()
	err := l.Protect(func() {
		l.Call(0, 1)
	})
	elapsed := time.Since(start)

	if err != nil {
		l.Pop(1)
		return Result{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			NotifyFail:     res.NotifyFail,
			Elapsed:        elapsed,
			Err:            err.Error(),
		}, nil
	}

	if l.TypeOf(-1) != lua.TypUserData {
		l.Pop(1)
		return Result{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			NotifyFail:     res.NotifyFail,
			Elapsed:        elapsed,
			Err:            "check() did not return userdata",
		}, nil
	}


	raw := l.ToUser(-1)
	l.Pop(1)

	// Validate: the type-switch itself confirms we have a known result type.
	switch raw.(type) {
	case *lmods.HTTPResult, *lmods.PingResult, *lmods.TCPResult,
		*lmods.DNSResult, *lmods.SSLResult:
	default:
		return Result{
			Slug:           res.Slug,
			Name:           res.Name,
			Desc:           res.Desc,
			NotifyPass:     res.NotifyPass,
			NotifyDegraded: res.NotifyDegraded,
			NotifyFail:     res.NotifyFail,
			Elapsed:        elapsed,
			Err:            fmt.Sprintf("unknown result type %T", raw),
		}, nil
	}

	return Result{
		Slug:           res.Slug,
		Name:           res.Name,
		Desc:           res.Desc,
		NotifyPass:     res.NotifyPass,
		NotifyDegraded: res.NotifyDegraded,
		NotifyFail:     res.NotifyFail,
		Raw:            raw,
		Elapsed:        elapsed,
	}, nil
}

// InsertTypedCheck dispatches a Result to the correct typed DB insert function.
func InsertTypedCheck(database *db.DB, result Result) error {
	switch v := result.Raw.(type) {
	case *lmods.HTTPResult:
		return insertHTTP(database, result.Slug, result.Elapsed, v)
	case *lmods.PingResult:
		return insertPing(database, result.Slug, result.Elapsed, v)
	case *lmods.TCPResult:
		return insertTCP(database, result.Slug, result.Elapsed, v)
	case *lmods.DNSResult:
		return insertDNS(database, result.Slug, result.Elapsed, v)
	case *lmods.SSLResult:
		return insertSSL(database, result.Slug, result.Elapsed, v)
	default:
		if result.Err != "" {
			return fmt.Errorf("%s: %s", result.Slug, result.Err)
		}
		return fmt.Errorf("unknown result type %T for slug %s", result.Raw, result.Slug)
	}
}

func insertHTTP(database *db.DB, slug string, elapsed time.Duration, r *lmods.HTTPResult) error {
	c := db.HTTPCheck{
		Slug:           slug,
		DurationMS:     elapsed.Milliseconds(),
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		StatusCode:     r.StatusCode,
		URL:            r.URL,
		BodySize:       r.BodySize,
		TLSVersion:     r.TLSVersion,
		RemoteIP:       r.RemoteIP,
		RedirectCount:  r.RedirectCount,
		Error:          r.Error,
	}
	if r.Body != "" {
		c.Body = &r.Body
	}
	_, err := db.InsertHTTPCheck(database, c)
	return err
}

func insertPing(database *db.DB, slug string, elapsed time.Duration, r *lmods.PingResult) error {
	c := db.PingCheck{
		Slug:            slug,
		DurationMS:      elapsed.Milliseconds(),
		Pass:            r.Pass,
		ResponseTimeMS:  r.ResponseTimeMS,
		PacketsSent:     r.PacketsSent,
		PacketsReceived: r.PacketsReceived,
		PacketLossPct:   r.PacketLossPct,
		MinMS:           r.MinMS,
		MaxMS:           r.MaxMS,
		Host:            r.Host,
		Error:           r.Error,
	}
	_, err := db.InsertPingCheck(database, c)
	return err
}

func insertTCP(database *db.DB, slug string, elapsed time.Duration, r *lmods.TCPResult) error {
	c := db.TCPCheck{
		Slug:           slug,
		DurationMS:     elapsed.Milliseconds(),
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		Port:           r.Port,
		RemoteIP:       r.RemoteIP,
		Error:          r.Error,
	}
	_, err := db.InsertTCPCheck(database, c)
	return err
}

func insertDNS(database *db.DB, slug string, elapsed time.Duration, r *lmods.DNSResult) error {
	ipsJSON := "["
	for i, ip := range r.IPs {
		if i > 0 {
			ipsJSON += ","
		}
		ipsJSON += `"` + ip + `"`
	}
	ipsJSON += "]"

	c := db.DNSCheck{
		Slug:           slug,
		DurationMS:     elapsed.Milliseconds(),
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		IPs:            ipsJSON,
		Error:          r.Error,
	}
	_, err := db.InsertDNSCheck(database, c)
	return err
}

func insertSSL(database *db.DB, slug string, elapsed time.Duration, r *lmods.SSLResult) error {
	c := db.SSLCheck{
		Slug:           slug,
		DurationMS:     elapsed.Milliseconds(),
		Pass:           r.Pass,
		ResponseTimeMS: r.ResponseTimeMS,
		Host:           r.Host,
		Port:           r.Port,
		Issuer:         r.Issuer,
		Subject:        r.Subject,
		NotBefore:      r.NotBefore,
		NotAfter:       r.NotAfter,
		DaysRemaining:  r.DaysRemaining,
		Error:          r.Error,
	}
	_, err := db.InsertSSLCheck(database, c)
	return err
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
