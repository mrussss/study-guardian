import json
import os
import subprocess
import sys
import time
import unittest

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../pet/src")))
from client import SupervisorClient


class TestPhase1Core(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = "phase1-test-token"
        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
        build_cmd = ["go", "build", "-o", "/tmp/study-supervisor-phase1", "./cmd/supervisor"]
        subprocess.check_call(build_cmd, cwd=repo_root)

        cls.config_path = "/tmp/studyguardian-phase1-config.yaml"
        cls.token_path = "/tmp/studyguardian-phase1-auth.token"
        cls.db_path = "/tmp/studyguardian-phase1.db"
        if os.path.exists(cls.db_path):
            os.remove(cls.db_path)

        with open(cls.token_path, "w") as f:
            f.write(cls.token + "\n")

        with open(cls.config_path, "w") as f:
            f.write(f"""
ipc:
  supervisor_host: "127.0.0.1"
  supervisor_port: 17383
  auth_token: "{cls.token}"
""")

        cls.supervisor_proc = subprocess.Popen(
            ["/tmp/study-supervisor-phase1", "-config", cls.config_path, "-token", cls.token_path, "-db", cls.db_path],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        time.sleep(0.5)

    @classmethod
    def tearDownClass(cls):
        if hasattr(cls, 'supervisor_proc') and cls.supervisor_proc:
            cls.supervisor_proc.terminate()
            cls.supervisor_proc.wait()
        for p in [cls.config_path, cls.token_path, cls.db_path, "/tmp/study-supervisor-phase1"]:
            if os.path.exists(p):
                try:
                    os.remove(p)
                except Exception:
                    pass

    def test_full_state_cycle(self):
        client = SupervisorClient(base_url="http://127.0.0.1:17383", auth_token=self.token)

        # 1. Verify health
        h = client.get_health()
        self.assertIsNotNone(h)
        self.assertEqual(h["status"], "ok")

        # 2. Initial state is STANDBY
        st = client.get_status()
        self.assertEqual(st["user_mode"], "STANDBY")

        # 3. Enter STUDY mode with task
        st = client.set_mode_study("Phase 1 Integration Test Task")
        self.assertEqual(st["user_mode"], "STUDY")
        self.assertEqual(st["task"], "Phase 1 Integration Test Task")

        # Wait for ticker to advance
        time.sleep(2.5)
        st = client.get_status()
        self.assertGreaterEqual(st["study_seconds"], 1)

        # 4. Modify task
        st = client.set_task("Updated Task Title")
        self.assertEqual(st["task"], "Updated Task Title")

        # 5. Switch to BREAK
        st = client.set_mode_break()
        self.assertEqual(st["user_mode"], "BREAK")

        time.sleep(2.5)
        st = client.get_status()
        self.assertGreaterEqual(st["break_seconds"], 1)

        # 6. Send feedback
        fb = client.send_feedback("rem-test-1", "ACTUALLY_STUDYING")
        self.assertEqual(fb.get("status"), "ok")

        # 7. Switch to OFF
        st = client.set_mode_off()
        self.assertEqual(st["user_mode"], "OFF")


if __name__ == "__main__":
    unittest.main()
