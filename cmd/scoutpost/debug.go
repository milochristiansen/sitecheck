package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// debugOn is set by enableDebug() once .env has been loaded. When true, debugf
// writes verbose trace lines to stderr.
var (
	debugOn    bool
	debugStart = time.Now()
	debugMu    sync.Mutex
)

// enableDebug turns on verbose tracing when SITECHECK_DEBUG is set to a truthy
// value. It is called from main after godotenv has loaded .env, so the setting
// may live in either the environment or the .env file.
func enableDebug() {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SITECHECK_DEBUG"))) {
	case "", "0", "false", "no", "off":
		debugOn = false
	default:
		debugOn = true
	}
}

// debugf writes a timestamped trace line to stderr when tracing is enabled.
// stderr is used deliberately: stdout carries the CGI JSON-lines wire protocol.
// The mutex keeps lines from interleaving when workers trace concurrently.
func debugf(format string, args ...any) {
	if !debugOn {
		return
	}
	debugMu.Lock()
	defer debugMu.Unlock()
	fmt.Fprintf(os.Stderr, "[scoutpost %8s] %s\n",
		time.Since(debugStart).Round(time.Millisecond),
		fmt.Sprintf(format, args...))
}
