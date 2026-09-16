package main

import (
	"fmt"
	"os"
)

// runCGI executes the outpost in CGI mode. It reads the bearer token from HTTP_AUTHORIZATION, validates it, scans
// resources, runs the worker pool, and streams JSON-lines results to stdout with CGI headers.
func runCGI(cfg *Config) {
	debugf("cgi: validating bearer token")
	if cfg.Token != "" && bearerToken(os.Getenv("HTTP_AUTHORIZATION")) != cfg.Token {
		debugf("cgi: unauthorized bearer token")
		fmt.Print("Status: 401 Unauthorized\r\nContent-Type: text/plain\r\n\r\n")
		fmt.Println("unauthorized: invalid bearer token")
		return
	}
	debugf("cgi: bearer token accepted")

	// Scan resources directory.
	resources, err := ScanResources(cfg.ResourcesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
		debugf("cgi: scan error: %v", err)
		fmt.Printf("Status: 500 Internal Server Error\r\nContent-Type: text/plain\r\n\r\nscan error: %v\n", err)
		return
	}
	debugf("cgi: scanned %d resource(s) in %q", len(resources), cfg.ResourcesDir)
	for _, res := range resources {
		debugf("cgi: found resource slug=%q script=%q", res.Slug, res.ScriptPath)
	}

	if len(resources) == 0 {
		// Empty resources is fine — return 200 with empty body.
		debugf("cgi: no resources, writing empty 200 response")
		fmt.Print("Status: 200 OK\r\nContent-Type: application/json\r\n\r\n")
		return
	}

	// Write CGI headers, then stream results as JSON-lines.
	fmt.Print("Status: 200 OK\r\nContent-Type: application/json\r\n\r\n")
	debugf("cgi: headers written; streaming results to stdout")
	runChecks(cfg, resources, os.Stdout, nil)
	debugf("cgi: result stream finished")
}
