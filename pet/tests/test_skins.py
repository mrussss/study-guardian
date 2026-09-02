import json
import os
import tempfile
import unittest

from skins import SkinRegistry


class SkinRegistryTest(unittest.TestCase):
    def test_invalid_user_skin_is_ignored_and_preference_is_persisted(self):
        with tempfile.TemporaryDirectory() as root:
            builtin = os.path.join(root, "builtin")
            users = os.path.join(root, "users")
            os.makedirs(os.path.join(builtin, "builtin-minimal", "sprites"))
            os.makedirs(os.path.join(users, "broken"))
            with open(os.path.join(builtin, "builtin-minimal", "manifest.json"), "w", encoding="utf-8") as f:
                json.dump({"id": "builtin-minimal", "name": "Minimal", "frame_size": 1, "display_size": 2,
                           "states": {"idle": "sprites/idle.png"}}, f)
            open(os.path.join(builtin, "builtin-minimal", "sprites", "idle.png"), "wb").close()
            with open(os.path.join(users, "broken", "manifest.json"), "w", encoding="utf-8") as f:
                json.dump({"id": "broken", "frame_size": 1, "display_size": 2, "states": {}}, f)
            preference = os.path.join(root, "config", "pet.json")
            registry = SkinRegistry(builtin, users, preference, "broken")
            self.assertEqual(registry.current_id, "builtin-minimal")
            self.assertTrue(registry.select("builtin-minimal"))
            with open(preference, encoding="utf-8") as f:
                self.assertEqual(json.load(f)["skin"], "builtin-minimal")


if __name__ == "__main__":
    unittest.main()
