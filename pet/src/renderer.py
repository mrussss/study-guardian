import os
from typing import Dict, List, Optional
from PyQt6.QtCore import Qt, QRect, QPoint, QTimer
from PyQt6.QtGui import QPixmap, QPainter, QMouseEvent
from PyQt6.QtWidgets import QWidget

from skins import SkinRegistry


class PetWidget(QWidget):
    def __init__(self, asset_dir: str, on_click=None, parent=None, skin_registry: Optional[SkinRegistry] = None):
        super().__init__(parent, Qt.WindowType.FramelessWindowHint | Qt.WindowType.WindowStaysOnTopHint)
        self.setWindowTitle("StudyGuardian Pet")
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground)
        self.setAttribute(Qt.WidgetAttribute.WA_NoSystemBackground)

        self.asset_dir = asset_dir
        self.skin_registry = skin_registry
        self.on_click = on_click
        self.drag_position = QPoint()

        # Animation states
        self.current_state = "idle" # idle, study, distracted, rest, talk, celebrate
        self.frames: Dict[str, List[QPixmap]] = {}
        self.current_frame_idx = 0

        self._load_sprites()

        # Timer for frame animation (e.g. 150ms per frame)
        self.anim_timer = QTimer(self)
        self.anim_timer.timeout.connect(self._next_frame)
        self.anim_timer.start(150)

        skin = self.skin_registry.current() if self.skin_registry else None
        display_size = skin.display_size if skin else 128
        self.setFixedSize(display_size, display_size)

    def _load_sprites(self):
        sprites_dir = os.path.join(self.asset_dir, "sprites")
        state_files = {
            "idle": "idle.png",
            "study": "actions.png",
            "distracted": "react.png",
            "rest": "rest.png",
            "talk": "talk.png",
            "celebrate": "celebrate.png",
        }

        for state_name, file_name in state_files.items():
            skin = self.skin_registry.current() if self.skin_registry else None
            path = skin.path_for(state_name) if skin else os.path.join(sprites_dir, file_name)
            self.frames[state_name] = []
            if os.path.exists(path):
                full_pixmap = QPixmap(path)
                # Split horizontal sprite sheet into square frames if wider than tall
                if full_pixmap.height() > 0:
                    frame_w = skin.frame_size if skin else full_pixmap.height()
                    frame_h = skin.frame_size if skin else full_pixmap.height()
                    num_frames = full_pixmap.width() // frame_w
                    for i in range(max(1, num_frames)):
                        rect = QRect(i * frame_w, 0, frame_w, frame_h)
                        display_size = skin.display_size if skin else 128
                        transform = Qt.TransformationMode.FastTransformation if skin and skin.pixel_art else Qt.TransformationMode.SmoothTransformation
                        frame = full_pixmap.copy(rect).scaled(display_size, display_size, Qt.AspectRatioMode.KeepAspectRatio, transform)
                        self.frames[state_name].append(frame)
            if not self.frames[state_name]:
                if skin and state_name != "idle" and self.frames.get("idle"):
                    self.frames[state_name] = list(self.frames["idle"])
                else:
                    blank_size = skin.display_size if skin else 128
                    blank = QPixmap(blank_size, blank_size)
                    blank.fill(Qt.GlobalColor.transparent)
                    self.frames[state_name].append(blank)

    def reload_skin(self, skin_id: str) -> bool:
        if not self.skin_registry or not self.skin_registry.select(skin_id):
            return False
        self.frames.clear()
        skin = self.skin_registry.current()
        display_size = skin.display_size if skin else 128
        self.setFixedSize(display_size, display_size)
        self._load_sprites()
        self.current_frame_idx = 0
        self.update()
        return True

    def set_animation_state(self, state_name: str):
        if state_name in self.frames and state_name != self.current_state:
            self.current_state = state_name
            self.current_frame_idx = 0
            self.update()

    def _next_frame(self):
        frame_list = self.frames.get(self.current_state, [])
        if frame_list:
            self.current_frame_idx = (self.current_frame_idx + 1) % len(frame_list)
            self.update()

    def paintEvent(self, event):
        painter = QPainter(self)
        painter.setRenderHint(QPainter.RenderHint.Antialiasing)
        frame_list = self.frames.get(self.current_state, [])
        if frame_list and self.current_frame_idx < len(frame_list):
            pixmap = frame_list[self.current_frame_idx]
            x = (self.width() - pixmap.width()) // 2
            y = (self.height() - pixmap.height()) // 2
            painter.drawPixmap(x, y, pixmap)

    def mousePressEvent(self, event: QMouseEvent):
        if event.button() == Qt.MouseButton.LeftButton:
            self.drag_position = event.globalPosition().toPoint() - self.frameGeometry().topLeft()
            event.accept()
        elif event.button() == Qt.MouseButton.RightButton:
            if self.on_click:
                self.on_click(event.globalPosition().toPoint())
            event.accept()

    def mouseMoveEvent(self, event: QMouseEvent):
        if event.buttons() & Qt.MouseButton.LeftButton:
            self.move(event.globalPosition().toPoint() - self.drag_position)
            event.accept()

    def mouseReleaseEvent(self, event: QMouseEvent):
        if event.button() == Qt.MouseButton.LeftButton:
            # If moved very little, treat as click
            diff = (event.globalPosition().toPoint() - self.frameGeometry().topLeft() - self.drag_position).manhattanLength()
            if diff < 5 and self.on_click:
                self.on_click(event.globalPosition().toPoint())
            event.accept()
