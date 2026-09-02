import json
import threading
import time
import unittest
import urllib.request
import urllib.error
import sys
import os

# Add screen directory to path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../screen")))

from capture import ScreenCapturer
from server import ThreadedHTTPServer, SensorHandler


class TestScreenSensor(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        SensorHandler.auth_token = "test-token-456"
        cls.server = ThreadedHTTPServer(("127.0.0.1", 17399), SensorHandler)
        cls.server_thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.server_thread.start()
        time.sleep(0.1)

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def test_capturer_stub(self):
        cap = ScreenCapturer()
        res = cap.capture()
        self.assertIn("timestamp", res)
        self.assertIn("hash", res)
        self.assertIn("changed", res)
        self.assertIsNone(res["error"])

    def test_healthz_endpoint(self):
        url = "http://127.0.0.1:17399/healthz"
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req) as resp:
            self.assertEqual(resp.status, 200)
            data = json.loads(resp.read().decode("utf-8"))
            self.assertEqual(data["status"], "ok")
            self.assertEqual(data["service"], "screen-sensor")

    def test_capture_auth_missing(self):
        url = "http://127.0.0.1:17399/v1/capture"
        req = urllib.request.Request(url, data=b"{}", headers={"Content-Type": "application/json"})
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req)
        self.assertEqual(ctx.exception.code, 401)

    def test_capture_auth_invalid(self):
        url = "http://127.0.0.1:17399/v1/capture"
        req = urllib.request.Request(url, data=b"{}", headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer bad-token"
        })
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req)
        self.assertEqual(ctx.exception.code, 401)

    def test_capture_auth_valid(self):
        url = "http://127.0.0.1:17399/v1/capture"
        body = json.dumps({"monitor": 1, "include_analysis_image": False}).encode("utf-8")
        req = urllib.request.Request(url, data=body, headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer test-token-456"
        })
        with urllib.request.urlopen(req) as resp:
            self.assertEqual(resp.status, 200)
            data = json.loads(resp.read().decode("utf-8"))
            self.assertIn("timestamp", data)
            self.assertIn("hash", data)
            self.assertIn("changed", data)


if __name__ == "__main__":
    unittest.main()
