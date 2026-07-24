package main

import (
	"fmt"
	"os"
	"strings"

	"sitecheck/protocol"
)

// runCGI executes the outpost in CGI mode. It reads the bearer token from HTTP_AUTHORIZATION, validates it, scans
// resources, runs the worker pool, and streams JSON-lines results to stdout with CGI headers.
func runCGI(cfg *Config) {
	auth := os.Getenv("HTTP_AUTHORIZATION")
	token := ""
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimPrefix(auth, "Bearer ")
	}

	if cfg.Token != "" && token != cfg.Token {
		fmt.Print("Status: 401 Unauthorized\r\n")
		fmt.Print("Content-Type: text/plain\r\n")
		fmt.Print("\r\n")
		fmt.Println("unauthorized: invalid bearer token")
		return
	}

	// Scan resources directory.
	resources, err := ScanResources(cfg.ResourcesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
		fmt.Print("Status: 500 Internal Server Error\r\n")
		fmt.Print("Content-Type: text/plain\r\n")
		fmt.Print("\r\n")
		fmt.Printf("scan error: %v\n", err)
		return
	}

	if len(resources) == 0 {
		// Empty resources is fine — return 200 with empty body.
		fmt.Print("Status: 200 OK\r\n")
		fmt.Print("Content-Type: application/json\r\n")
		fmt.Print("\r\n")
		return
	}

	// Create pool and submit jobs.
	pool := NewPool(cfg.Workers, cfg.DefaultTimeout)

	fmt.Fprintf(os.Stderr, "Running %d check(s) with %d worker(s)...\n", len(resources), cfg.Workers)

	go func() {
		for _, res := range resources {
			pool.Submit(Job{Resource: res})
		}
		pool.Wait()
	}()

	// Write CGI headers, then stream results as JSON-lines.
	fmt.Print("Status: 200 OK\r\n")
	fmt.Print("Content-Type: application/json\r\n")
	fmt.Print("\r\n")

	for wr := range pool.Results() {
		if err := protocol.WriteResult(os.Stdout, wr); err != nil {
			fmt.Fprintf(os.Stderr, "write result error: %v\n", err)
		}
	}
}
