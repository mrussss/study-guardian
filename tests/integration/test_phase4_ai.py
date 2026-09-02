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


class MockAWHandlerForPhase4(BaseHTTPRequestHandler):
    current_app = "chrome.exe"
    current_title = "Video Tutorial #101"
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
                "aw-watcher-window_P4": {"id": "aw-watcher-window_P4", "type": "currentwindow"},
                "aw-watcher-afk_P4": {"id": "aw-watcher-afk_P4", "type": "afkstatus"}
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


class TestPhase4AI(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = "phase4-test-token"
        
        # Start Mock AW
        cls.aw_server = HTTPServer(("127.0.0.1", 15697), MockAWHandlerForPhase4)
        cls.aw_thread = threading.Thread(target=cls.aw_server.serve_forever, daemon=True)
        cls.aw_thread.start()

        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
        build_cmd = ["go", "build", "-o", "/tmp/study-supervisor-phase4", "./cmd/supervisor"]
        subprocess.check_call(build_cmd, cwd=repo_root)

        cls.config_path = "/tmp/studyguardian-phase4-config.yaml"
        cls.token_path = "/tmp/studyguardian-phase4-auth.token"
        cls.db_path = "/tmp/studyguardian-phase4.db"
        if os.path.exists(cls.db_path):
            os.remove(cls.db_path)

        with open(cls.token_path, "w") as f:
            f.write(cls.token + "\n")

        with open(cls.config_path, "w") as f:
            f.write(f"""
ipc:
  supervisor_host: "127.0.0.1"
  supervisor_port: 17387
  auth_token: "{cls.token}"
ai:
  enabled: true
  provider: "fake"
  min_confidence: 0.75
privacy:
  sensitive_apps:
    - "bitwarden"
""")

        cls.supervisor_proc = subprocess.Popen(
            ["/tmp/study-supervisor-phase4", "-config", cls.config_path, "-token", cls.token_path,
             "-db", cls.db_path, "-aw-url", "http://127.0.0.1:15697"],
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
        for p in [cls.config_path, cls.token_path, cls.db_path, "/tmp/study-supervisor-phase4"]:
            if os.path.exists(p):
                try:
                    os.remove(p)
                except Exception:
                    pass

    def test_ai_classification_pipeline(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17387", auth_token=self.token)

        # 1. Enter STUDY mode
        client.set_mode_study("Algorithms and Data Structures")

        # 2. Ambiguous title: AI fake provider should classify as FOCUSED
        MockAWHandlerForPhase4.current_app = "chrome.exe"
        MockAWHandlerForPhase4.current_title = "Video Tutorial #101"
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["task_relation"], "FOCUSED")
        self.assertGreaterEqual(st["confidence"], 0.75)

        # 3. Entertainment title: AI fake provider classifies as DISTRACTED
        MockAWHandlerForPhase4.current_app = "chrome.exe"
        MockAWHandlerForPhase4.current_title = "Top 10 Anime Battles Gameplay"
        time.sleep(2.5)
        st = client.get_status()
        self.assertEqual(st["task_relation"], "DISTRACTED")


if __name__ == "__main__":
    unittest.main()
