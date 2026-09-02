import json
import os
import subprocess
import sys
import time
import unittest
from http.server import HTTPServer, BaseHTTPRequestHandler
import threading

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../pet/src")))
from client import SupervisorClient


class MockActivityWatchHandler(BaseHTTPRequestHandler):
    current_app = "Code.exe"
    current_title = "main.go - study-guardian"
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
                "aw-watcher-window_MOCK": {
                    "id": "aw-watcher-window_MOCK",
                    "type": "currentwindow"
                },
                "aw-watcher-afk_MOCK": {
                    "id": "aw-watcher-afk_MOCK",
                    "type": "afkstatus"
                }
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
                "data": {
                    "app": self.__class__.current_app,
                    "title": self.__class__.current_title
                }
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
                "data": {
                    "status": self.__class__.current_status
                }
            }]
            self.wfile.write(json.dumps(events).encode("utf-8"))
            return

        self.send_response(404)
        self.end_headers()


class TestPhase2ActivityWatch(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.aw_server = HTTPServer(("127.0.0.1", 15699), MockActivityWatchHandler)
        cls.aw_thread = threading.Thread(target=cls.aw_server.serve_forever, daemon=True)
        cls.aw_thread.start()

        cls.token = "phase2-test-token"
        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
        build_cmd = ["go", "build", "-o", "/tmp/study-supervisor-phase2", "./cmd/supervisor"]
        subprocess.check_call(build_cmd, cwd=repo_root)

        cls.config_path = "/tmp/studyguardian-phase2-config.yaml"
        cls.token_path = "/tmp/studyguardian-phase2-auth.token"
        cls.db_path = "/tmp/studyguardian-phase2.db"
        if os.path.exists(cls.db_path):
            os.remove(cls.db_path)

        with open(cls.token_path, "w") as f:
            f.write(cls.token + "\n")

        with open(cls.config_path, "w") as f:
            f.write(f"""
ipc:
  supervisor_host: "127.0.0.1"
  supervisor_port: 17384
  auth_token: "{cls.token}"
""")

        cls.supervisor_proc = subprocess.Popen(
            ["/tmp/study-supervisor-phase2", "-config", cls.config_path, "-token", cls.token_path,
             "-db", cls.db_path, "-aw-url", "http://127.0.0.1:15699"],
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
        for p in [cls.config_path, cls.token_path, cls.db_path, "/tmp/study-supervisor-phase2"]:
            if os.path.exists(p):
                try:
                    os.remove(p)
                except Exception:
                    pass

    def test_activitywatch_supervision_flow(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17384", auth_token=self.token)

        # 1. Health should report activitywatch_ok = true
        h = client.get_health()
        self.assertIsNotNone(h)
        self.assertTrue(h.get("activitywatch_ok"))

        # 2. Enter STUDY mode
        client.set_mode_study("Go Backend Architecture")

        # 3. Simulate active focused development
        MockActivityWatchHandler.current_app = "Code.exe"
        MockActivityWatchHandler.current_title = "server.go - study-guardian"
        MockActivityWatchHandler.current_status = "not-afk"

        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["interaction_state"], "ACTIVE")
        self.assertEqual(st["task_relation"], "FOCUSED")
        self.assertGreaterEqual(st["active_seconds"], 2)

        # 4. Simulate distraction (Steam game)
        MockActivityWatchHandler.current_app = "Steam.exe"
        MockActivityWatchHandler.current_title = "Steam Store"

        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["task_relation"], "DISTRACTED")


if __name__ == "__main__":
    unittest.main()
