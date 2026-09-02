import json
import threading
import time
import unittest
import urllib.request
import urllib.error
import sys
import os

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "../screen")))

from capture import ScreenCapturer, hamming_distance
from server import ThreadedHTTPServer, SensorHandler


class TestScreenSensor(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        SensorHandler.auth_token = "test-token-456"
        cls.server = ThreadedHTTPServer(("127.0.0.1", 17395), SensorHandler)
        cls.server_thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.server_thread.start()
        time.sleep(0.1)

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def test_hamming_distance(self):
        h1 = "ffff0000ffff0000"
        h2 = "ffff0000ffff0000"
        self.assertEqual(hamming_distance(h1, h2), 0)

        h3 = "ffff0000ffff0001" # 1 bit difference
        self.assertEqual(hamming_distance(h1, h3), 1)

        h4 = "ffff0000ffff000f" # 4 bit difference
        self.assertEqual(hamming_distance(h1, h4), 4)

    def test_capturer_stub(self):
        cap = ScreenCapturer(change_threshold=4)
        res = cap.capture()
        self.assertIn("timestamp", res)
        self.assertIn("hash", res)
        self.assertIn("changed", res)
        self.assertIsNone(res["error"])

    def test_healthz_endpoint(self):
        url = "http://127.0.0.1:17395/healthz"
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req) as resp:
            self.assertEqual(resp.status, 200)
            data = json.loads(resp.read().decode("utf-8"))
            self.assertEqual(data["status"], "ok")
            self.assertEqual(data["service"], "screen-sensor")

    def test_capture_auth(self):
        url = "http://127.0.0.1:17395/v1/capture"
        # 1. Missing auth -> 401
        req = urllib.request.Request(url, data=b"{}", headers={"Content-Type": "application/json"})
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req)
        self.assertEqual(ctx.exception.code, 401)

        # 2. Valid auth -> 200
        req = urllib.request.Request(url, data=b"{}", headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer test-token-456"
        })
        with urllib.request.urlopen(req) as resp:
            self.assertEqual(resp.status, 200)
            data = json.loads(resp.read().decode("utf-8"))
            self.assertIn("hash", data)
            self.assertIn("changed", data)


if __name__ == "__main__":
    unittest.main()
