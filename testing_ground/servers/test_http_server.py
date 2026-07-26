#!/usr/bin/env python3
"""A test HTTP server for SiteCheck end-to-end testing.

Endpoints:
  /ok        → 200 OK with JSON body
  /slow      → 200 OK after a configurable delay (default 2s)
  /error     → 500 Internal Server Error
  /notfound  → 404 Not Found
  /health    → 200 OK (always, for TCP checks)
  /echo      → POST echo, returns the body as-is
"""
import argparse
import json
import time
from http.server import HTTPServer, BaseHTTPRequestHandler


class TestHandler(BaseHTTPRequestHandler):
    delay = 2.0  # seconds for /slow

    def do_GET(self):
        if self.path == "/ok":
            self._json(200, {"status": "ok", "message": "All systems operational"})
        elif self.path == "/slow":
            time.sleep(self.delay)
            self._json(200, {"status": "ok", "message": "Slow but working", "delay_s": self.delay})
        elif self.path == "/error":
            self._json(500, {"status": "error", "message": "Internal server error"})
        elif self.path == "/notfound":
            self._json(404, {"status": "error", "message": "Resource not found"})
        elif self.path == "/health":
            self._plain(200, "OK")
        else:
            self._json(404, {"status": "error", "message": f"Unknown path: {self.path}"})

    def do_POST(self):
        if self.path == "/echo":
            content_length = int(self.headers.get("Content-Length", 0))
            body = self.rfile.read(content_length) if content_length else b""
            self._raw(200, body)
        else:
            self._json(404, {"status": "error", "message": "POST not supported on this path"})

    def _json(self, code, data):
        body = json.dumps(data).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _plain(self, code, text):
        body = text.encode()
        self.send_response(code)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _raw(self, code, body):
        self.send_response(code)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        # Suppress default stderr logging — the test harness manages output.
        pass


def main():
    parser = argparse.ArgumentParser(description="SiteCheck test HTTP server")
    parser.add_argument("--port", type=int, default=19976, help="Listen port")
    parser.add_argument("--delay", type=float, default=2.0, help="Delay for /slow endpoint")
    args = parser.parse_args()

    TestHandler.delay = args.delay
    server = HTTPServer(("127.0.0.1", args.port), TestHandler)
    print(f"Test HTTP server listening on 127.0.0.1:{args.port}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
