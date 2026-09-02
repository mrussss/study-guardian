import json
import os
import subprocess
import sys
import time
import unittest

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../pet/src")))
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../../sensor/screen")))

from client import SupervisorClient
from server import ThreadedHTTPServer, SensorHandler


class TestLocalhostTriadPoC(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = "poc-secret-token-2026"
        SensorHandler.auth_token = cls.token
        cls.sensor_server = ThreadedHTTPServer(("127.0.0.1", 17382), SensorHandler)
        import threading
        cls.sensor_thread = threading.Thread(target=cls.sensor_server.serve_forever, daemon=True)
        cls.sensor_thread.start()

        # Build supervisor binary for linux test
        repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
        build_cmd = ["go", "build", "-o", "/tmp/study-supervisor-test", "./cmd/supervisor"]
        subprocess.check_call(build_cmd, cwd=repo_root)

        # Create temporary config
        cls.config_path = "/tmp/studyguardian-test-config.yaml"
        cls.token_path = "/tmp/studyguardian-test-auth.token"
        with open(cls.token_path, "w") as f:
            f.write(cls.token + "\n")

        with open(cls.config_path, "w") as f:
            f.write(f"""
ipc:
  supervisor_host: "127.0.0.1"
  supervisor_port: 17381
  sensor_host: "127.0.0.1"
  sensor_port: 17382
  auth_token: "{cls.token}"
""")

        # Start supervisor
        cls.supervisor_proc = subprocess.Popen(
            ["/tmp/study-supervisor-test", "-config", cls.config_path, "-token", cls.token_path],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE
        )
        time.sleep(2.5)

    @classmethod
    def tearDownClass(cls):
        if hasattr(cls, 'supervisor_proc') and cls.supervisor_proc:
            cls.supervisor_proc.terminate()
            cls.supervisor_proc.wait()
        if hasattr(cls, 'sensor_server') and cls.sensor_server:
            cls.sensor_server.shutdown()
            cls.sensor_server.server_close()

    def test_localhost_communication(self):
        pet_client = SupervisorClient(base_url="http://127.0.0.1:17381", auth_token=self.token)
        
        # 1. Health check
        health = pet_client.get_health()
        self.assertIsNotNone(health)
        self.assertEqual(health.get("status"), "ok")

        # 2. Get initial status
        st = pet_client.get_status()
        self.assertIsNotNone(st)
        self.assertEqual(st.get("user_mode"), "STANDBY")

        # 3. Transition to STUDY
        study_res = pet_client.set_mode_study("Reading Architecture Docs")
        self.assertIsNotNone(study_res)
        self.assertEqual(study_res.get("user_mode"), "STUDY")
        self.assertEqual(study_res.get("task"), "Reading Architecture Docs")

        # 4. Transition to BREAK
        break_res = pet_client.set_mode_break()
        self.assertIsNotNone(break_res)
        self.assertEqual(break_res.get("user_mode"), "BREAK")

        # 5. Transition to OFF
        off_res = pet_client.set_mode_off()
        self.assertIsNotNone(off_res)
        self.assertEqual(off_res.get("user_mode"), "OFF")

    def test_invalid_token_fail_soft(self):
        bad_client = SupervisorClient(base_url="http://127.0.0.1:17381", auth_token="wrong-token-abc")
        st = bad_client.get_status()
        self.assertIsNone(st)
        self.assertFalse(bad_client.is_connected)
        self.assertIn("401", bad_client.last_error)

    def test_disconnected_supervisor_fail_soft(self):
        offline_client = SupervisorClient(base_url="http://127.0.0.1:17399", auth_token=self.token)
        st = offline_client.get_status()
        self.assertIsNone(st)
        self.assertFalse(offline_client.is_connected)


if __name__ == "__main__":
    unittest.main()
