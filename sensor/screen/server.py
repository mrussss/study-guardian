import argparse
import json
import os
import sys
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from socketserver import ThreadingMixIn
from typing import Optional

from capture import ScreenCapturer


class ThreadedHTTPServer(ThreadingMixIn, HTTPServer):
    daemon_threads = True


class SensorHandler(BaseHTTPRequestHandler):
    capturer = ScreenCapturer()
    auth_token: Optional[str] = None

    def log_message(self, format, *args):
        # Clean logging without leaking auth headers
        sys.stderr.write(f"[Sensor] {self.address_string()} - {format % args}\n")

    def _check_auth(self) -> bool:
        if not self.auth_token:
            return True
        auth_header = self.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"error": "unauthorized", "reason": "missing Bearer token"}')
            return False
        token = auth_header[7:].strip()
        if token != self.auth_token:
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"error": "unauthorized", "reason": "invalid token"}')
            return False
        return True

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "status": "ok",
                "service": "screen-sensor",
                "mss_available": self.capturer.is_available(),
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
            }
            self.wfile.write(json.dumps(resp).encode("utf-8"))
            return

        if self.path == "/v1/monitors":
            if not self._check_auth():
                return
            result = self.capturer.list_monitors()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(result).encode("utf-8"))
            return

        self.send_response(404)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"error": "not found"}')

    def do_POST(self):
        if self.path == "/v1/capture":
            if not self._check_auth():
                return

            content_len = int(self.headers.get("Content-Length", 0))
            body = {}
            if content_len > 0:
                raw_body = self.rfile.read(content_len)
                try:
                    body = json.loads(raw_body.decode("utf-8"))
                except Exception:
                    body = {}

            monitor_idx = body.get("monitor", 0)
            if monitor_idx == "primary":
                monitor_idx = 0
            elif not isinstance(monitor_idx, int):
                monitor_idx = 0

            include_analysis = bool(body.get("include_analysis_image", False))
            max_width = int(body.get("max_width", 960))

            result = self.capturer.capture(
                monitor_idx=monitor_idx,
                include_analysis_image=include_analysis,
                max_width=max_width
            )

            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(result).encode("utf-8"))
            return

        self.send_response(404)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"error": "not found"}')


def main():
    parser = argparse.ArgumentParser(description="StudyGuardian Screen Sensor")
    parser.add_argument("--host", default="127.0.0.1", help="Host to bind")
    parser.add_argument("--port", type=int, default=17322, help="Port to bind")
    parser.add_argument("--token-file", default="", help="Path to auth.token")
    parser.add_argument("--token", default="", help="Direct auth token string")
    args = parser.parse_args()

    token = args.token
    if not token and args.token_file and os.path.exists(args.token_file):
        try:
            with open(args.token_file, "r", encoding="utf-8") as f:
                token = f.read().strip()
        except Exception as e:
            sys.stderr.write(f"[Sensor] Warning: could not read token file: {e}\n")

    SensorHandler.auth_token = token if token else None

    server_address = (args.host, args.port)
    httpd = ThreadedHTTPServer(server_address, SensorHandler)
    sys.stderr.write(f"[Sensor] Screen Sensor listening on http://{args.host}:{args.port}\n")

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        sys.stderr.write("[Sensor] Shutting down...\n")
        httpd.server_close()


if __name__ == "__main__":
    main()
