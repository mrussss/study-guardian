"""Helpers for deterministic integration-test process startup."""

import time
from urllib.error import URLError
from urllib.request import urlopen


def wait_for_supervisor(proc, port, timeout=5.0):
    """Wait until a test Supervisor serves /healthz or fail with diagnostics."""
    deadline = time.monotonic() + timeout
    last_error = None
    health_url = f"http://127.0.0.1:{port}/healthz"

    while time.monotonic() < deadline:
        if proc.poll() is not None:
            stderr = proc.stderr.read().decode(errors="replace") if proc.stderr else ""
            raise RuntimeError(
                f"Supervisor exited before becoming ready (code={proc.returncode}): {stderr.strip()}"
            )
        try:
            with urlopen(health_url, timeout=0.2) as response:
                if response.status == 200:
                    return
                last_error = RuntimeError(f"unexpected health status {response.status}")
        except (OSError, URLError) as exc:
            last_error = exc
        time.sleep(0.05)

    raise RuntimeError(f"Supervisor did not become ready on port {port}: {last_error}")
