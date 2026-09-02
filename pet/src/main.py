import argparse
import os
import sys
from PyQt6.QtCore import QTimer, QPoint, QObject, QThread, pyqtSignal, pyqtSlot
from PyQt6.QtGui import QIcon, QAction
from PyQt6.QtWidgets import QApplication, QSystemTrayIcon, QMenu

from client import SupervisorClient
from renderer import PetWidget
from bubble import SpeechBubble
from menu import PetContextMenu
from skins import SkinRegistry
from study_center import StudyCenter


class StatusWorker(QObject):
    """Perform Supervisor HTTP polling away from the Qt GUI event loop."""

    poll_requested = pyqtSignal()
    status_ready = pyqtSignal(object)

    def __init__(self, client: SupervisorClient):
        super().__init__()
        self.client = client
        self.poll_requested.connect(self.poll)

    @pyqtSlot()
    def poll(self):
        status = self.client.get_status()
        motivation = self.client.get_motivation_status() if status else None
        self.status_ready.emit({"status": status, "motivation": motivation})


class StudyPetApp(QObject):
    def __init__(self, host: str, port: int, token: str, asset_dir: str, user_skins: str = "", pet_config: str = "", requested_skin: str = ""):
        super().__init__()
        self.client = SupervisorClient(base_url=f"http://{host}:{port}", auth_token=token)
        self.asset_dir = asset_dir
        if not user_skins:
            user_skins = os.path.abspath(os.path.join(asset_dir, "../../config/pet-skins"))
        if not pet_config:
            pet_config = os.path.abspath(os.path.join(asset_dir, "../../config/pet.json"))
        self.skin_registry = SkinRegistry(os.path.join(asset_dir, "skins"), user_skins, pet_config, requested_skin)
        self.study_center = None

        self.last_status = None
        self.last_shown_reminder_id = None

        # Create widgets
        self.pet_widget = PetWidget(asset_dir=asset_dir, on_click=self._on_pet_clicked, skin_registry=self.skin_registry)
        self.bubble = SpeechBubble(on_feedback=self._on_feedback)
        self.context_menu = PetContextMenu(parent=self.pet_widget, client=self.client, on_mode_changed=self._request_status, on_open_study_center=self.open_study_center, skin_registry=self.skin_registry, on_skin_changed=self.change_skin)

        # Tray Icon
        self._setup_tray()

        # Position Pet initially in bottom-right corner
        screen = QApplication.primaryScreen().geometry()
        self.pet_widget.move(screen.width() - 180, screen.height() - 220)
        self.pet_widget.show()

        # Polling timer
        self.poll_timer = QTimer()
        self.poll_timer.timeout.connect(self._request_status)
        self.poll_timer.start(1500)

        self.poll_in_flight = False
        self.worker_thread = QThread()
        self.status_worker = StatusWorker(self.client)
        self.status_worker.moveToThread(self.worker_thread)
        self.status_worker.status_ready.connect(self._handle_status)
        self.worker_thread.finished.connect(self.status_worker.deleteLater)
        self.worker_thread.start()
        self._request_status()

    def _request_status(self):
        if not self.poll_in_flight:
            self.poll_in_flight = True
            self.status_worker.poll_requested.emit()

    def _setup_tray(self):
        if not hasattr(self, "tray_icon"):
            self.tray_icon = QSystemTrayIcon()
        skin = self.skin_registry.current()
        icon_path = os.path.join(skin.root, "icon.png") if skin else os.path.join(self.asset_dir, "pet_icon.png")
        if os.path.exists(icon_path):
            self.tray_icon.setIcon(QIcon(icon_path))
        else:
            # Fallback icon from sprite
            sprite_path = os.path.join(self.asset_dir, "sprites", "idle.png")
            if os.path.exists(sprite_path):
                self.tray_icon.setIcon(QIcon(sprite_path))

        if self.tray_icon.contextMenu() is None:
            tray_menu = QMenu()
            show_action = QAction("显示桌宠", tray_menu)
            show_action.triggered.connect(self.pet_widget.show)
            tray_menu.addAction(show_action)

            hide_action = QAction("隐藏桌宠", tray_menu)
            hide_action.triggered.connect(self.pet_widget.hide)
            tray_menu.addAction(hide_action)

            tray_menu.addSeparator()
            quit_action = QAction("退出", tray_menu)
            quit_action.triggered.connect(QApplication.instance().quit)
            tray_menu.addAction(quit_action)
            self.tray_icon.setContextMenu(tray_menu)
        self.tray_icon.show()

    def _on_pet_clicked(self, global_pos: QPoint):
        self.context_menu.show_menu(global_pos, self.last_status)

    def _on_feedback(self, event_id: str, feedback: str):
        self.client.send_feedback(event_id, feedback)

    def open_study_center(self):
        if self.study_center is None:
            self.study_center = StudyCenter(self.client)
        self.study_center.show()
        self.study_center.raise_()
        self.study_center.activateWindow()

    def change_skin(self, skin_id: str):
        if self.pet_widget.reload_skin(skin_id):
            self._setup_tray()

    def _celebrate(self, event):
        event_key = f"{event.get('type')}:{event.get('created_at')}"
        if event_key == getattr(self, "last_motivation_event", ""):
            return
        self.last_motivation_event = event_key
        self.pet_widget.set_animation_state("celebrate")
        self.bubble.show_message(event.get("message", "做得很好！"), event_id=event_key, duration_ms=5000, target_pos=self.pet_widget.geometry().center())
        QTimer.singleShot(5000, lambda: self.pet_widget.set_animation_state("idle"))

    @pyqtSlot(object)
    def _handle_status(self, payload):
        self.poll_in_flight = False
        status = payload.get("status") if isinstance(payload, dict) and "status" in payload else payload
        motivation = payload.get("motivation") if isinstance(payload, dict) else None
        self.last_status = status

        if not status:
            self.pet_widget.set_animation_state("idle")
            self.tray_icon.setToolTip("StudyGuardian: 未连接到 Supervisor")
            return

        if motivation and motivation.get("last_event"):
            self._celebrate(motivation["last_event"])

        mode = status.get("user_mode", "STANDBY")
        task = status.get("task", "")
        relation = status.get("task_relation", "UNKNOWN")
        reminder = status.get("current_reminder")

        # Update animation state
        if mode == "STUDY":
            if relation == "DISTRACTED":
                self.pet_widget.set_animation_state("distracted")
            else:
                self.pet_widget.set_animation_state("study")
            tooltip = f"StudyGuardian: 学习中 ({task})" if task else "StudyGuardian: 学习中"
        elif mode == "BREAK":
            self.pet_widget.set_animation_state("rest")
            tooltip = "StudyGuardian: 休息中"
        elif mode == "OFF":
            self.pet_widget.set_animation_state("idle")
            tooltip = "StudyGuardian: 今日已结束"
        else:
            self.pet_widget.set_animation_state("idle")
            tooltip = "StudyGuardian: 待机中"

        self.tray_icon.setToolTip(tooltip)

        # Handle bubble reminder
        if reminder:
            rem_id = reminder.get("id")
            if rem_id and rem_id != self.last_shown_reminder_id:
                self.last_shown_reminder_id = rem_id
                msg = reminder.get("message", "")
                target_pt = self.pet_widget.geometry().center()
                self.bubble.show_message(msg, event_id=rem_id, duration_ms=10000, target_pos=target_pt)

    def shutdown(self):
        self.poll_timer.stop()
        self.worker_thread.quit()
        self.worker_thread.wait(1500)


def main():
    parser = argparse.ArgumentParser(description="StudyGuardian Pet UI Shell")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=17321)
    parser.add_argument("--token-file", default="")
    parser.add_argument("--token", default="")
    parser.add_argument("--assets", default="")
    parser.add_argument("--user-skins", default="")
    parser.add_argument("--pet-config", default="")
    parser.add_argument("--skin", default="")
    args = parser.parse_args()

    token = args.token
    if not token and args.token_file and os.path.exists(args.token_file):
        try:
            with open(args.token_file, "r", encoding="utf-8") as f:
                token = f.read().strip()
        except Exception:
            pass

    asset_dir = args.assets
    if not asset_dir:
        # Default relative to script location
        asset_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "../assets"))

    app = QApplication(sys.argv)
    app.setQuitOnLastWindowClosed(False)
    pet_app = StudyPetApp(host=args.host, port=args.port, token=token, asset_dir=asset_dir, user_skins=args.user_skins, pet_config=args.pet_config, requested_skin=args.skin)
    app.aboutToQuit.connect(pet_app.shutdown)
    sys.exit(app.exec())


if __name__ == "__main__":
    main()
