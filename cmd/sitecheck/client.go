package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"sitecheck/protocol"
)

// Client executes checks by communicating with a scoutpost outpost.
type Client interface {
	// Run starts the outpost and returns a channel of results. The channel is closed when all results have been delivered.
	Run() (<-chan protocol.WireResult, error)
}

// LocalClient runs the scoutpost binary as a local subprocess in CGI mode.
type LocalClient struct {
	bin          string
	resourcesDir string
	timeout      int
	token        string
}

// NewLocalClient creates a Client that spawns the local scoutpost binary.
func NewLocalClient(bin, resourcesDir string, timeout int) *LocalClient {
	return &LocalClient{
		bin:          bin,
		resourcesDir: resourcesDir,
		timeout:      timeout,
		token:        "local",
	}
}

// Run spawns the scoutpost binary with CGI environment variables and returns a channel of protocol.WireResult values
// parsed from its stdout.
func (c *LocalClient) Run() (<-chan protocol.WireResult, error) {
	// Try to find the binary. First check the configured path, then $PATH.
	binPath, err := exec.LookPath(c.bin)
	if err != nil {
		return nil, fmt.Errorf("scoutpost binary %q not found: %w", c.bin, err)
	}

	cmd := exec.Command(binPath)
	cmd.Env = append(os.Environ(),
		"GATEWAY_INTERFACE=CGI/1.1",
		"REQUEST_METHOD=GET",
		"HTTP_AUTHORIZATION=Bearer "+c.token,
		"SITECHECK_TOKEN="+c.token,
		"SITECHECK_RESOURCES_DIR="+c.resourcesDir,
		"SITECHECK_DEFAULT_TIMEOUT="+fmt.Sprintf("%d", c.timeout),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start scoutpost: %w", err)
	}

	ch := make(chan protocol.WireResult)

	go func() {
		defer close(ch)

		if err := parseCGIResponse(stdout, ch); err != nil {
			fmt.Fprintf(os.Stderr, "Local outpost read error: %v\n", err)
		}

		// Wait for the process to finish.
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "Local outpost exited with error: %v\n", err)
		}
	}()

	return ch, nil
}


// HTTPClient runs checks by making an HTTP GET to a remote scoutpost outpost.
type HTTPClient struct {
	url     string
	token   string
	timeout time.Duration
}

// NewHTTPClient creates a Client that communicates with a remote outpost over HTTP.
func NewHTTPClient(url, token string, timeout int) *HTTPClient {
	return &HTTPClient{
		url:     url,
		token:   token,
		timeout: time.Duration(timeout) * time.Second,
	}
}

// Run sends an HTTP GET to the outpost URL with a bearer token and streams JSON-lines results from the response body.
func (c *HTTPClient) Run() (<-chan protocol.WireResult, error) {
	req, err := http.NewRequest("GET", c.url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("outpost request: %w", err)
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("outpost returned status %d", resp.StatusCode)
	}

	ch := make(chan protocol.WireResult)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		for wr := range protocol.ReadResults(resp.Body) {
			ch <- wr
		}
	}()

	return ch, nil
}

// parseCGIResponse reads CGI headers from r, then streams JSON-lines results to ch until EOF.
func parseCGIResponse(r io.Reader, ch chan<- protocol.WireResult) error {
	scanner := bufio.NewScanner(r)

	// Read CGI headers until blank line.
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		// Check for non-200 status.
		if len(line) > 7 && line[:7] == "Status:" {
			status := line[7:]
			if len(status) > 0 && status[0] == ' ' {
				status = status[1:]
			}
			if status != "" && status[:3] != "200" {
				// Read the rest of the error body and discard.
				for scanner.Scan() {
				}
				return fmt.Errorf("outpost returned status: %s", status)
			}
		}
	}

	// Read JSON-lines body.
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var wr protocol.WireResult
		if err := json.Unmarshal(line, &wr); err != nil {
			fmt.Fprintf(os.Stderr, "parse result line error: %v\n", err)
			continue
		}
		ch <- wr
	}
	return scanner.Err()
}
