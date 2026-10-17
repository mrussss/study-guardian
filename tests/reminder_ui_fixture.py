"""Isolated native reminder UI fixture. Never points to a production database.

Bind only 127.0.0.1:17441. Run a Pet copy with its runtime root under
D:/StudyGuardianBuild/tests and a matching dummy auth.token/config.yaml.
POST /__test__/due reopens a new generation; GET /__test__ reports UI actions.
No capture, AI, real activity sampling, or production settings are involved.
"""
import datetime as dt
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

TOKEN = "isolated-reminder-ui-token"
state = {"phase": "SHORT_BREAK_DUE", "revision": 1, "claimed": False, "suppressed": False, "actions": []}

def status():
    now = dt.datetime.now(dt.timezone.utc).isoformat()
    return dict(enabled=True, phase=state["phase"], local_date=now[:10],
                focus_segment_seconds=2400, focus_since_long_break_seconds=2400,
                snooze_count=0, completed_short_breaks=0, completed_long_breaks=0,
                retry_focus_after_seconds=0, revision=state["revision"], updated_at=now,
                notification_suppressed=state["suppressed"])

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def respond(self, value, code=200):
        data = json.dumps(value).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.headers.get("Authorization") != "Bearer " + TOKEN:
            return self.respond({}, 401)
        if self.path == "/__test__":
            return self.respond(state)
        if self.path == "/v1/status":
            return self.respond(dict(study_seconds=837))
        if self.path == "/v1/activity/current":
            return self.respond(dict(schema_version=1, observed_at=dt.datetime.now(dt.timezone.utc).isoformat(),
                fresh=True,user_mode="STUDY",task="UI fixture",interaction="ACTIVE",relation="FOCUSED",
                privacy="NORMAL",activity="GENERAL_STUDY",confidence=.9))
        if self.path == "/v1/settings/eye-care":
            return self.respond(dict(enabled=True,focus_minutes=40,short_break_minutes=5,
                long_break_after_focus_minutes=120,long_break_minutes=20,snooze_minutes=5,max_snoozes=2))
        if self.path == "/v1/eye-care/status":
            return self.respond(status())
        return self.respond({}, 404)

    def do_POST(self):
        if self.headers.get("Authorization") != "Bearer " + TOKEN:
            return self.respond({}, 401)
        payload=json.loads(self.rfile.read(int(self.headers.get("Content-Length",0))) or b"{}")
        if self.path == "/__test__/due":
            state.update(phase="SHORT_BREAK_DUE", revision=state["revision"]+1,claimed=False,suppressed=False)
            return self.respond(state)
        if self.path == "/v1/eye-care/notification/claim":
            claim = not state["claimed"] and state["phase"] == "SHORT_BREAK_DUE"
            state["claimed"] = True
            return self.respond(dict(claimed=claim))
        if self.path == "/v1/eye-care/action":
            if payload.get("expected_revision") != state["revision"]:
                return self.respond({},409)
            state["actions"].append(payload["action"])
            state["revision"]+=1
            if payload["action"]=="SNOOZE":
                state["suppressed"]=True
            elif payload["action"]=="START_SHORT_BREAK":
                state["phase"]="SHORT_BREAK"
            else:
                state["phase"]="FOCUSING"
            return self.respond(status())
        return self.respond({},404)

if __name__=="__main__":
    HTTPServer(("127.0.0.1",17441),Handler).serve_forever()
