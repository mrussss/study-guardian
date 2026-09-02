from PyQt6.QtCore import Qt, QTimer, QPoint
from PyQt6.QtGui import QPainter, QColor, QPainterPath, QFont
from PyQt6.QtWidgets import QWidget, QLabel, QVBoxLayout, QPushButton, QHBoxLayout


class SpeechBubble(QWidget):
    def __init__(self, parent=None, on_feedback=None):
        super().__init__(parent, Qt.WindowType.ToolTip | Qt.WindowType.FramelessWindowHint | Qt.WindowType.WindowStaysOnTopHint)
        self.setAttribute(Qt.WidgetAttribute.WA_TranslucentBackground)
        self.setAttribute(Qt.WidgetAttribute.WA_ShowWithoutActivating)
        self.on_feedback = on_feedback

        self.layout = QVBoxLayout(self)
        self.layout.setContentsMargins(14, 12, 14, 14)
        self.layout.setSpacing(8)

        self.msg_label = QLabel(self)
        self.msg_label.setWordWrap(True)
        self.msg_label.setFont(QFont("Segoe UI", 10))
        self.msg_label.setStyleSheet("color: #1a1a1a;")
        self.layout.addWidget(self.msg_label)

        # Feedback buttons layout
        self.btn_layout = QHBoxLayout()
        self.btn_layout.setSpacing(6)

        self.btn_studying = QPushButton("我其实在学习", self)
        self.btn_studying.setStyleSheet("font-size: 11px; padding: 3px 8px; border-radius: 4px; background: #e0f2fe; color: #0369a1; border: 1px solid #bae6fd;")
        self.btn_studying.clicked.connect(self._feedback_studying)
        self.btn_layout.addWidget(self.btn_studying)

        self.btn_dismiss = QPushButton("知道了", self)
        self.btn_dismiss.setStyleSheet("font-size: 11px; padding: 3px 8px; border-radius: 4px; background: #f3f4f6; color: #374151; border: 1px solid #d1d5db;")
        self.btn_dismiss.clicked.connect(self.hide)
        self.btn_layout.addWidget(self.btn_dismiss)

        self.layout.addLayout(self.btn_layout)

        self.current_event_id = None
        self.hide_timer = QTimer(self)
        self.hide_timer.setSingleShot(True)
        self.hide_timer.timeout.connect(self.hide)

    def show_message(self, text: str, event_id: str = None, duration_ms: int = 8000, target_pos: QPoint = None):
        self.current_event_id = event_id
        self.msg_label.setText(text)
        self.adjustSize()

        if target_pos:
            self.move(target_pos.x() - self.width() // 2, target_pos.y() - self.height() - 10)

        self.show()
        if duration_ms > 0:
            self.hide_timer.start(duration_ms)

    def _feedback_studying(self):
        if self.on_feedback and self.current_event_id:
            self.on_feedback(self.current_event_id, "ACTUALLY_STUDYING")
        self.hide()

    def paintEvent(self, event):
        painter = QPainter(self)
        painter.setRenderHint(QPainter.RenderHint.Antialiasing)

        path = QPainterPath()
        rect = self.rect().adjusted(1, 1, -1, -1)
        path.addRoundedRect(float(rect.x()), float(rect.y()), float(rect.width()), float(rect.height()), 10.0, 10.0)

        # Bubble background
        painter.fillPath(path, QColor(255, 255, 255, 245))
        painter.setPen(QColor(200, 200, 210))
        painter.drawPath(path)
