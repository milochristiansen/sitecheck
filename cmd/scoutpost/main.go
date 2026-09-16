package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"

	_ "sitecheck/checktypes/dns"
	_ "sitecheck/checktypes/exec"
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

	enableDebug()
	wd, _ := os.Getwd()
	debugf("start pid=%d cwd=%q", os.Getpid(), wd)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
	debugf("config resources_dir=%q workers=%d default_timeout=%d listen=%q auth_enabled=%t",
		cfg.ResourcesDir, cfg.Workers, cfg.DefaultTimeout, cfg.Listen, cfg.Token != "")

	// Mode detection: GATEWAY_INTERFACE set → CGI, otherwise server.
	if os.Getenv("GATEWAY_INTERFACE") != "" {
		debugf("mode=cgi (GATEWAY_INTERFACE=%q)", os.Getenv("GATEWAY_INTERFACE"))
		runCGI(cfg)
		debugf("cgi: exiting")
		return
	}

	debugf("mode=server")
	if err := runServer(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
