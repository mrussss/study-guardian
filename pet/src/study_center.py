from PyQt6.QtCore import Qt, QObject, QThread, pyqtSignal, pyqtSlot
from PyQt6.QtWidgets import (
    QWidget, QTabWidget, QVBoxLayout, QLabel, QPushButton,
    QListWidget, QListWidgetItem, QInputDialog,
)

from client import SupervisorClient


class StudyCenterWorker(QObject):
    finished = pyqtSignal(object)

    def __init__(self, client: SupervisorClient, operation=None):
        super().__init__()
        self.client = client
        self.operation = operation

    @pyqtSlot()
    def run(self):
        result = {"error": None}
        try:
            if self.operation:
                kind, args = self.operation
                if kind == "create":
                    self.client.create_mission(*args)
                elif kind == "complete":
                    self.client.complete_mission(*args)
                elif kind == "cancel":
                    self.client.cancel_mission(*args)
                elif kind == "redeem":
                    self.client.redeem_reward(*args)
                elif kind == "target":
                    self.client.set_daily_target(*args)
            result.update({
                "status": self.client.get_motivation_status(),
                "settings": self.client.get_motivation_settings(),
                "history": self.client.get_history(7),
                "missions": self.client.get_missions(),
                "achievements": self.client.get_achievements(),
                "rewards": self.client.get_rewards(),
            })
            if not result["status"]:
                result["error"] = self.client.last_error or "Supervisor 暂时无法连接"
        except Exception as exc:
            result["error"] = str(exc)
        self.finished.emit(result)


class StudyCenter(QWidget):
    """Lazy-created view; all HTTP work stays outside the Qt GUI thread."""

    def __init__(self, client: SupervisorClient, parent=None):
        super().__init__(parent)
        self.client = client
        self.setWindowTitle("StudyGuardian 学习中心")
        self.resize(620, 460)
        self.tabs = QTabWidget(self)
        self.overview = QLabel("正在加载…")
        self.history = QListWidget()
        self.missions = QListWidget()
        self.achievements = QListWidget()
        self.rewards = QListWidget()
        self.current_target = 120
        self._thread = None
        self._worker = None
        self._build_ui()
        self.missions.itemDoubleClicked.connect(self.complete_selected_mission)
        self.rewards.itemDoubleClicked.connect(self.redeem_selected_reward)

    def _build_ui(self):
        root = QVBoxLayout(self)
        root.addWidget(self.tabs)

        overview_page = QWidget()
        overview_layout = QVBoxLayout(overview_page)
        self.overview.setWordWrap(True)
        overview_layout.addWidget(self.overview)
        target_button = QPushButton("修改每日目标")
        target_button.clicked.connect(self.change_target)
        overview_layout.addWidget(target_button)
        overview_layout.addWidget(QLabel("最近 7 天"))
        overview_layout.addWidget(self.history)
        self.tabs.addTab(overview_page, "总览")

        mission_page = QWidget()
        mission_layout = QVBoxLayout(mission_page)
        mission_layout.addWidget(self.missions)
        add_button = QPushButton("新增任务")
        add_button.clicked.connect(self.add_mission)
        mission_layout.addWidget(add_button)
        cancel_button = QPushButton("取消选中任务")
        cancel_button.clicked.connect(self.cancel_selected_mission)
        mission_layout.addWidget(cancel_button)
        self.tabs.addTab(mission_page, "任务")

        achievement_page = QWidget()
        QVBoxLayout(achievement_page).addWidget(self.achievements)
        self.tabs.addTab(achievement_page, "成就")

        reward_page = QWidget()
        QVBoxLayout(reward_page).addWidget(self.rewards)
        self.tabs.addTab(reward_page, "奖励")

    def showEvent(self, event):
        super().showEvent(event)
        self.refresh()

    def refresh(self, operation=None):
        if self._thread is not None and self._thread.isRunning():
            return
        self.overview.setText("正在从 Supervisor 加载…")
        self._thread = QThread(self)
        self._worker = StudyCenterWorker(self.client, operation)
        self._worker.moveToThread(self._thread)
        self._thread.started.connect(self._worker.run)
        self._worker.finished.connect(self._apply_data)
        self._worker.finished.connect(self._thread.quit)
        self._worker.finished.connect(self._worker.deleteLater)
        self._thread.finished.connect(self._thread.deleteLater)
        self._thread.finished.connect(self._clear_worker)
        self._thread.start()

    @pyqtSlot(object)
    def _apply_data(self, data):
        if data.get("error"):
            self.overview.setText("暂时无法连接 Supervisor：" + data["error"])
            return
        status = data.get("status") or {}
        settings = data.get("settings") or {}
        self.current_target = int(settings.get("daily_target_minutes", status.get("daily_target_minutes", 120)) or 120)
        self.overview.setText(
            f"今日有效专注：{status.get('today_credited_focus_minutes', 0)} 分钟\n"
            f"今日目标：{status.get('target_progress', 0):.0%} / {self.current_target} 分钟\n"
            f"连续打卡：{status.get('streak_days', 0)} 天\n"
            f"今日赚取 AP：{status.get('today_earned_ap_milli', 0) / 1000:.3f}\n"
            f"今日消费 AP：{status.get('today_spent_ap_milli', 0) / 1000:.3f}\n"
            f"AP 余额：{status.get('balance_ap_milli', 0) / 1000:.3f}"
        )
        self.history.clear()
        for day in data.get("history") or []:
            mark = "已打卡" if day.get("checkin_completed") else ""
            self.history.addItem(f"{day.get('date')}: {day.get('focus_minutes', 0)} 分钟  {mark}")

        self.missions.clear()
        for mission in data.get("missions") or []:
            item = QListWidgetItem(f"[{mission.get('status')}] {mission.get('title')}  +{mission.get('reward_milli_ap', 0)/1000:.3f} AP")
            item.setData(Qt.ItemDataRole.UserRole, mission.get("id"))
            self.missions.addItem(item)

        self.achievements.clear()
        for achievement in data.get("achievements") or []:
            mark = "已解锁" if achievement.get("unlocked") else f"进度 {achievement.get('progress', 0):.0%}"
            self.achievements.addItem(f"{mark}  {achievement.get('name')}: {achievement.get('description')}")

        self.rewards.clear()
        for reward in data.get("rewards") or []:
            item = QListWidgetItem(f"{reward.get('name')}  {reward.get('cost_milli_ap', 0)/1000:.3f} AP — 双击兑换")
            item.setData(Qt.ItemDataRole.UserRole, reward.get("id"))
            self.rewards.addItem(item)

    def _clear_worker(self):
        self._thread = None
        self._worker = None

    def change_target(self):
        minutes, ok = QInputDialog.getInt(self, "每日目标", "有效专注分钟数", self.current_target, 1, 1440)
        if ok:
            self.refresh(("target", (minutes,)))

    def add_mission(self):
        title, ok = QInputDialog.getText(self, "新增任务", "任务名称")
        if ok and title.strip():
            self.refresh(("create", (title.strip(),)))

    def complete_selected_mission(self, item):
        mission_id = item.data(Qt.ItemDataRole.UserRole)
        if mission_id:
            self.refresh(("complete", (mission_id,)))

    def cancel_selected_mission(self):
        item = self.missions.currentItem()
        mission_id = item.data(Qt.ItemDataRole.UserRole) if item else None
        if mission_id:
            self.refresh(("cancel", (mission_id,)))

    def redeem_selected_reward(self, item):
        reward_id = item.data(Qt.ItemDataRole.UserRole)
        if reward_id:
            self.refresh(("redeem", (reward_id,)))

    def closeEvent(self, event):
        self.hide()
        event.ignore()
