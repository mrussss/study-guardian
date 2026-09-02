import json
import os
import subprocess
import sys
import time
import unittest
from http.server import HTTPServer, BaseHTTPRequestHandler
import threading

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../pet/src")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../sensor/screen")))

from client import SupervisorClient
from server import ThreadedHTTPServer, SensorHandler


class MockAWHandlerForPhase3(BaseHTTPRequestHandler):
    current_app = "Code.exe"
    current_title = "main.go"
    current_status = "not-afk"

    def log_message(self, format, *args):
        pass

    def do_GET(self):
        if self.path == "/api/0/info":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"version": "v0.13.2"}')
            return

        if self.path == "/api/0/buckets/":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            buckets = {
                "aw-watcher-window_P3": {"id": "aw-watcher-window_P3", "type": "currentwindow"},
                "aw-watcher-afk_P3": {"id": "aw-watcher-afk_P3", "type": "afkstatus"}
            }
            self.wfile.write(json.dumps(buckets).encode("utf-8"))
            return

        if "aw-watcher-window" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            events = [{
                "id": 1,
                "timestamp": "2026-09-02T10:00:00Z",
                "duration": 5.0,
                "data": {"app": self.__class__.current_app, "title": self.__class__.current_title}
            }]
            self.wfile.write(json.dumps(events).encode("utf-8"))
            return

        if "aw-watcher-afk" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            events = [{
                "id": 2,
                "timestamp": "2026-09-02T10:00:00Z",
                "duration": 5.0,
                "data": {"status": self.__class__.current_status}
            }]
            self.wfile.write(json.dumps(events).encode("utf-8"))
            return

        self.send_response(404)
        self.end_headers()


class MockScreenSensorHandler(BaseHTTPRequestHandler):
    capture_count = 0
    screen_changed = False

    def log_message(self, format, *args):
        pass

    def do_GET(self):
        if self.path == "/healthz":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok","service":"screen-sensor"}')
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.path == "/v1/capture":
            self.__class__.capture_count += 1
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "timestamp": "2026-09-02T10:00:00Z",
                "monitor": 1,
                "changed": self.__class__.screen_changed,
                "hash": "aabbccdd11223344",
                "is_stub": True,
                "error": None
            }
            self.wfile.write(json.dumps(resp).encode("utf-8"))
            return
        self.send_response(404)
        self.end_headers()


class TestPhase3ScreenSensor(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = "phase3-test-token"
        
        # Start Mock AW
        cls.aw_server = HTTPServer(("127.0.0.1", 15698), MockAWHandlerForPhase3)
        cls.aw_thread = threading.Thread(target=cls.aw_server.serve_forever, daemon=True)
        cls.aw_thread.start()

        # Start Mock Sensor
        cls.sensor_server = HTTPServer(("127.0.0.1", 17386), MockScreenSensorHandler)
        cls.sensor_thread = threading.Thread(target=cls.sensor_server.serve_forever, daemon=True)
        cls.sensor_thread.start()

        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
        build_cmd = ["go", "build", "-o", "/tmp/study-supervisor-phase3", "./cmd/supervisor"]
        subprocess.check_call(build_cmd, cwd=repo_root)

        cls.config_path = "/tmp/studyguardian-phase3-config.yaml"
        cls.token_path = "/tmp/studyguardian-phase3-auth.token"
        cls.db_path = "/tmp/studyguardian-phase3.db"
        if os.path.exists(cls.db_path):
            os.remove(cls.db_path)

        with open(cls.token_path, "w") as f:
            f.write(cls.token + "\n")

        with open(cls.config_path, "w") as f:
            f.write(f"""
ipc:
  supervisor_host: "127.0.0.1"
  supervisor_port: 17385
  sensor_host: "127.0.0.1"
  sensor_port: 17386
  auth_token: "{cls.token}"
screen:
  enabled: true
privacy:
  sensitive_apps:
    - "keepass"
    - "1password"
    - "bitwarden"
""")

        cls.supervisor_proc = subprocess.Popen(
            ["/tmp/study-supervisor-phase3", "-config", cls.config_path, "-token", cls.token_path,
             "-db", cls.db_path, "-aw-url", "http://127.0.0.1:15698"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        time.sleep(0.5)

    @classmethod
    def tearDownClass(cls):
        if hasattr(cls, 'supervisor_proc') and cls.supervisor_proc:
            cls.supervisor_proc.terminate()
            cls.supervisor_proc.wait()
        if hasattr(cls, 'aw_server') and cls.aw_server:
            cls.aw_server.shutdown()
            cls.aw_server.server_close()
        if hasattr(cls, 'sensor_server') and cls.sensor_server:
            cls.sensor_server.shutdown()
            cls.sensor_server.server_close()
        for p in [cls.config_path, cls.token_path, cls.db_path, "/tmp/study-supervisor-phase3"]:
            if os.path.exists(p):
                try:
                    os.remove(p)
                except Exception:
                    pass

    def test_sensor_interaction_states(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17385", auth_token=self.token)

        # 1. Active user -> ACTIVE
        MockAWHandlerForPhase3.current_app = "Code.exe"
        MockAWHandlerForPhase3.current_status = "not-afk"
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["interaction_state"], "ACTIVE")

        # 2. AFK + Screen Static -> IDLE_STATIC
        MockAWHandlerForPhase3.current_status = "afk"
        MockScreenSensorHandler.screen_changed = False
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["interaction_state"], "IDLE_STATIC")

        # 3. AFK + Screen Dynamic (video playing) -> IDLE_DYNAMIC
        MockScreenSensorHandler.screen_changed = True
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["interaction_state"], "IDLE_DYNAMIC")

        # 4. Privacy Gate: Sensitive App (Bitwarden)
        init_capture_count = MockScreenSensorHandler.capture_count
        MockAWHandlerForPhase3.current_app = "Bitwarden.exe"
        MockAWHandlerForPhase3.current_title = "My Passwords"
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["privacy_state"], "SENSITIVE")
        # Ensure capture was NOT called when sensitive
        self.assertEqual(MockScreenSensorHandler.capture_count, init_capture_count)


if __name__ == "__main__":
    unittest.main()
