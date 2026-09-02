import json
import logging
import urllib.request
import urllib.error
from typing import Optional, Dict, Any

logger = logging.getLogger("StudyPet.Client")


class SupervisorClient:
    def __init__(self, base_url: str = "http://127.0.0.1:17321", auth_token: str = ""):
        self.base_url = base_url.rstrip("/")
        self.auth_token = auth_token.strip()
        self.last_error: Optional[str] = None
        self.is_connected: bool = False

    def _make_request(self, method: str, path: str, data: Optional[Dict[str, Any]] = None, timeout: float = 0.5) -> Optional[Dict[str, Any]]:
        url = f"{self.base_url}{path}"
        headers = {"Content-Type": "application/json"}
        if self.auth_token:
            headers["Authorization"] = f"Bearer {self.auth_token}"

        body_bytes = None
        if data is not None:
            body_bytes = json.dumps(data).encode("utf-8")

        req = urllib.request.Request(url, data=body_bytes, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=timeout) as resp:
                self.is_connected = True
                self.last_error = None
                resp_data = resp.read().decode("utf-8")
                if resp_data:
                    return json.loads(resp_data)
                return {}
        except urllib.error.HTTPError as e:
            self.is_connected = False
            self.last_error = f"HTTP {e.code}: {e.reason}"
            logger.warning("Supervisor request %s %s failed: %s", method, path, self.last_error)
            return None
        except Exception as e:
            self.is_connected = False
            self.last_error = str(e)
            logger.warning("Supervisor connection error %s %s: %s", method, path, self.last_error)
            return None

    def get_health(self) -> Optional[Dict[str, Any]]:
        return self._make_request("GET", "/healthz")

    def get_status(self) -> Optional[Dict[str, Any]]:
        return self._make_request("GET", "/v1/status")

    def set_mode_study(self, task: str = "") -> Optional[Dict[str, Any]]:
        return self._make_request("POST", "/v1/mode/study", {"task": task})

    def set_mode_break(self) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", "/v1/mode/break")

    def set_mode_off(self) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", "/v1/mode/off")

    def set_task(self, task: str) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", "/v1/task", {"task": task})

    def send_feedback(self, event_id: str, feedback: str) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", "/v1/feedback", {"event_id": event_id, "feedback": feedback})

    def get_motivation_status(self) -> Optional[Dict[str, Any]]:
        return self._make_request("GET", "/v1/motivation/status")

    def get_history(self, days: int = 7) -> Optional[list]:
        return self._make_request("GET", f"/v1/motivation/history?days={days}")

    def get_achievements(self) -> Optional[list]:
        return self._make_request("GET", "/v1/motivation/achievements")

    def get_missions(self) -> Optional[list]:
        return self._make_request("GET", "/v1/missions")

    def create_mission(self, title: str, description: str = "", reward_milli_ap: int = 0, due_date: str = "") -> Optional[Dict[str, Any]]:
        body = {"title": title, "description": description, "reward_milli_ap": reward_milli_ap}
        if due_date:
            body["due_date"] = due_date
        return self._make_request("POST", "/v1/missions", body)

    def complete_mission(self, mission_id: str) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", f"/v1/missions/{mission_id}/complete")

    def cancel_mission(self, mission_id: str) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", f"/v1/missions/{mission_id}/cancel")

    def get_rewards(self) -> Optional[list]:
        return self._make_request("GET", "/v1/rewards")

    def redeem_reward(self, reward_id: str) -> Optional[Dict[str, Any]]:
        return self._make_request("POST", f"/v1/rewards/{reward_id}/redeem")
