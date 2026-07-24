package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	_ "sitecheck/checktypes/dns"
	_ "sitecheck/checktypes/http"
	_ "sitecheck/checktypes/outpost"
	_ "sitecheck/checktypes/ping"
	_ "sitecheck/checktypes/ssl"
	_ "sitecheck/checktypes/systemd"
	_ "sitecheck/checktypes/tcp"
)

func main() {
	// Load .env from working directory; not fatal if missing.
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: no .env file found, using defaults (%v)\n", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	// Mode detection: GATEWAY_INTERFACE set → CGI, otherwise server.
	if os.Getenv("GATEWAY_INTERFACE") != "" {
		runCGI(cfg)
		return
	}

	if err := runServer(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
