import json
import threading
import time
import unittest
from http.server import HTTPServer, BaseHTTPRequestHandler
import sys
import os

# Add src to path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../src")))

from client import SupervisorClient


class MockSupervisorHandler(BaseHTTPRequestHandler):
    auth_token = "valid-token-789"
    current_mode = "STANDBY"
    current_task = ""

    def log_message(self, format, *args):
        pass

    def _check_auth(self):
        auth = self.headers.get("Authorization", "")
        return auth == f"Bearer {self.auth_token}"

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok","service":"supervisor"}')
            return

        if self.path == "/v1/status":
            if not self._check_auth():
                self.send_response(401)
                self.end_headers()
                self.wfile.write(b'{"error":"unauthorized"}')
                return

            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "user_mode": self.current_mode,
                "task": self.current_task,
                "study_seconds": 120,
                "break_seconds": 0,
                "activitywatch_ok": True,
                "screen_sensor_ok": True
            }
            self.wfile.write(json.dumps(resp).encode("utf-8"))
            return

        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if not self._check_auth():
            self.send_response(401)
            self.end_headers()
            self.wfile.write(b'{"error":"unauthorized"}')
            return

        length = int(self.headers.get("Content-Length", 0))
        body = {}
        if length > 0:
            body = json.loads(self.rfile.read(length).decode("utf-8"))

        if self.path == "/v1/mode/study":
            self.__class__.current_mode = "STUDY"
            self.__class__.current_task = body.get("task", "")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"user_mode": "STUDY", "task": self.current_task}).encode("utf-8"))
            return

        if self.path == "/v1/mode/break":
            self.__class__.current_mode = "BREAK"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"user_mode": "BREAK"}).encode("utf-8"))
            return

        if self.path == "/v1/mode/off":
            self.__class__.current_mode = "OFF"
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"user_mode": "OFF"}).encode("utf-8"))
            return

        self.send_response(404)
        self.end_headers()


class TestSupervisorClient(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 17398), MockSupervisorHandler)
        cls.server_thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.server_thread.start()
        time.sleep(0.1)

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def test_health_check(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17398")
        res = client.get_health()
        self.assertIsNotNone(res)
        self.assertEqual(res.get("status"), "ok")
        self.assertTrue(client.is_connected)

    def test_status_with_valid_auth(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17398", auth_token="valid-token-789")
        st = client.get_status()
        self.assertIsNotNone(st)
        self.assertIn("user_mode", st)
        self.assertTrue(client.is_connected)

    def test_status_with_invalid_auth(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17398", auth_token="wrong-token")
        st = client.get_status()
        self.assertIsNone(st)
        self.assertIn("401", client.last_error)
        self.assertFalse(client.is_connected)

    def test_mode_transitions(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17398", auth_token="valid-token-789")
        
        # STUDY
        res = client.set_mode_study("Math Problem Set")
        self.assertIsNotNone(res)
        self.assertEqual(res.get("user_mode"), "STUDY")
        self.assertEqual(res.get("task"), "Math Problem Set")

        # BREAK
        res = client.set_mode_break()
        self.assertIsNotNone(res)
        self.assertEqual(res.get("user_mode"), "BREAK")

        # OFF
        res = client.set_mode_off()
        self.assertIsNotNone(res)
        self.assertEqual(res.get("user_mode"), "OFF")

    def test_disconnected_server_fail_soft(self):
        # Target port that is not running
        client = SupervisorClient(base_url="http://127.0.0.1:17397", auth_token="valid-token-789")
        st = client.get_status()
        self.assertIsNone(st)
        self.assertFalse(client.is_connected)
        self.assertIsNotNone(client.last_error)


if __name__ == "__main__":
    unittest.main()
