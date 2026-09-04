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


class FakeMSSContext:
    def __init__(self, monitors):
        self.monitors = monitors
        self.closed = False

    def close(self):
        self.closed = True


class RediscoveringCapturer(ScreenCapturer):
    def __init__(self, contexts):
        self.contexts = iter(contexts)
        self.sct = None
        self.refresh_count = 0
        super().__init__()

    def _refresh_capture_context(self):
        previous = self.sct
        self.sct = next(self.contexts)
        self.refresh_count += 1
        if previous is not None:
            previous.close()
        return True


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

    def test_monitor_geometry_is_authenticated_and_sanitized(self):
        url = "http://127.0.0.1:17395/v1/monitors"
        req = urllib.request.Request(url)
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            urllib.request.urlopen(req)
        self.assertEqual(ctx.exception.code, 401)

        req = urllib.request.Request(url, headers={"Authorization": "Bearer test-token-456"})
        with urllib.request.urlopen(req) as resp:
            self.assertEqual(resp.status, 200)
            data = json.loads(resp.read().decode("utf-8"))
            self.assertEqual(data["count"], len(data["monitors"]))
            for monitor in data["monitors"]:
                self.assertIn("index", monitor)
                self.assertIsInstance(monitor["width"], int)
                self.assertIsInstance(monitor["height"], int)

    def test_monitor_listing_rediscoveries_geometry_after_display_change(self):
        first = FakeMSSContext([
            {"left": 0, "top": 0, "width": 3840, "height": 2160},
            {"left": -1920, "top": 0, "width": 1920, "height": 1080, "is_primary": False},
        ])
        second = FakeMSSContext([
            {"left": 0, "top": 0, "width": 3840, "height": 2160},
            {"left": -1920, "top": 0, "width": 1920, "height": 1080, "is_primary": False},
        ])
        third = FakeMSSContext([
            {"left": 0, "top": 0, "width": 2560, "height": 1440},
            {"left": 2560, "top": 0, "width": 1920, "height": 1080, "is_primary": False},
        ])
        capturer = RediscoveringCapturer([first, second, third])

        initial = capturer.list_monitors()
        updated = capturer.list_monitors()

        self.assertEqual(initial["count"], 2)
        self.assertEqual(initial["monitors"][1]["left"], -1920)
        self.assertTrue(initial["monitors"][0]["is_virtual"])
        self.assertEqual(updated["monitors"][0]["width"], 2560)
        self.assertEqual(updated["monitors"][1]["left"], 2560)
        self.assertEqual(capturer.refresh_count, 3)
        self.assertTrue(first.closed)
        self.assertTrue(second.closed)


if __name__ == "__main__":
    unittest.main()
