import json
import logging
import os
import tempfile
from dataclasses import dataclass
from typing import Dict, Optional

logger = logging.getLogger("StudyPet.Skins")
STATES = ("idle", "study", "distracted", "rest", "talk", "celebrate")


@dataclass
class Skin:
    id: str
    name: str
    root: str
    frame_size: int
    display_size: int
    fps: int
    pixel_art: bool
    states: Dict[str, str]

    def path_for(self, state: str) -> str:
        rel = self.states.get(state) or self.states.get("idle")
        return os.path.join(self.root, rel) if rel else ""


class SkinRegistry:
    def __init__(self, builtin_root: str, user_root: str, preference_path: str, requested: str = ""):
        self.builtin_root = builtin_root
        self.user_root = user_root
        self.preference_path = preference_path
        self.skins: Dict[str, Skin] = {}
        self._scan(builtin_root)
        self._scan(user_root)
        self.current_id = requested or self._read_preference() or "studyguardian-pixel"
        if self.current_id not in self.skins:
            self.current_id = "builtin-minimal" if "builtin-minimal" in self.skins else next(iter(self.skins), "")

    def _scan(self, root: str) -> None:
        if not root or not os.path.isdir(root):
            return
        for name in sorted(os.listdir(root)):
            directory = os.path.join(root, name)
            manifest_path = os.path.join(directory, "manifest.json")
            if not os.path.isdir(directory) or not os.path.isfile(manifest_path):
                continue
            try:
                with open(manifest_path, "r", encoding="utf-8") as f:
                    raw = json.load(f)
                skin = self._validate(raw, directory)
                if skin:
                    self.skins[skin.id] = skin
            except Exception as exc:
                logger.warning("Ignoring invalid skin %s: %s", manifest_path, exc)

    @staticmethod
    def _validate(raw: dict, root: str) -> Optional[Skin]:
        skin_id = str(raw.get("id", "")).strip()
        frame_size = int(raw.get("frame_size", 0))
        display_size = int(raw.get("display_size", 0))
        states = raw.get("states")
        if not skin_id or frame_size <= 0 or display_size <= 0 or not isinstance(states, dict) or not states.get("idle"):
            raise ValueError("manifest requires id, frame_size, display_size and states.idle")
        resolved = {str(k): str(v) for k, v in states.items() if isinstance(v, str)}
        for state, rel in list(resolved.items()):
            if not os.path.isfile(os.path.join(root, rel)):
                if state == "idle":
                    raise ValueError("idle sprite is missing")
                resolved.pop(state, None)
        return Skin(skin_id, str(raw.get("name", skin_id)), root, frame_size, display_size,
                    max(1, int(raw.get("fps", 7))), bool(raw.get("pixel_art", False)), resolved)

    def _read_preference(self) -> str:
        try:
            with open(self.preference_path, "r", encoding="utf-8") as f:
                value = json.load(f).get("skin", "")
                return str(value).strip()
        except (OSError, ValueError, TypeError):
            return ""

    def select(self, skin_id: str) -> bool:
        if skin_id not in self.skins:
            return False
        self.current_id = skin_id
        try:
            os.makedirs(os.path.dirname(self.preference_path), exist_ok=True)
            fd, temp_path = tempfile.mkstemp(prefix="pet-", suffix=".json", dir=os.path.dirname(self.preference_path))
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                json.dump({"skin": skin_id}, f, ensure_ascii=False, indent=2)
                f.write("\n")
            os.replace(temp_path, self.preference_path)
        except OSError as exc:
            logger.warning("Could not persist skin preference: %s", exc)
        return True

    def current(self) -> Optional[Skin]:
        return self.skins.get(self.current_id)

    def available(self) -> Dict[str, Skin]:
        return dict(self.skins)
