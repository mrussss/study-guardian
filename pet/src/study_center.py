from PyQt6.QtCore import Qt
from PyQt6.QtWidgets import (
    QWidget, QTabWidget, QVBoxLayout, QHBoxLayout, QLabel, QPushButton,
    QListWidget, QListWidgetItem, QInputDialog, QMessageBox,
)

from client import SupervisorClient


class StudyCenter(QWidget):
    """Small lazy-created view; Supervisor remains the only business source."""

    def __init__(self, client: SupervisorClient, parent=None):
        super().__init__(parent)
        self.client = client
        self.setWindowTitle("StudyGuardian 学习中心")
        self.resize(620, 460)
        self.tabs = QTabWidget(self)
        self.overview = QLabel()
        self.history = QListWidget()
        self.missions = QListWidget()
        self.achievements = QListWidget()
        self.rewards = QListWidget()
        self._build_ui()
        self.missions.itemDoubleClicked.connect(self.complete_selected_mission)
        self.rewards.itemDoubleClicked.connect(self.redeem_selected_reward)
        self.refresh()

    def _build_ui(self):
        root = QVBoxLayout(self)
        root.addWidget(self.tabs)

        overview_page = QWidget()
        overview_layout = QVBoxLayout(overview_page)
        self.overview.setWordWrap(True)
        overview_layout.addWidget(self.overview)
        overview_layout.addWidget(QLabel("最近 7 天"))
        overview_layout.addWidget(self.history)
        self.tabs.addTab(overview_page, "总览")

        mission_page = QWidget()
        mission_layout = QVBoxLayout(mission_page)
        mission_layout.addWidget(self.missions)
        add_button = QPushButton("新增任务")
        add_button.clicked.connect(self.add_mission)
        mission_layout.addWidget(add_button)
        self.tabs.addTab(mission_page, "任务")

        achievement_page = QWidget()
        QVBoxLayout(achievement_page).addWidget(self.achievements)
        self.tabs.addTab(achievement_page, "成就")

        reward_page = QWidget()
        QVBoxLayout(reward_page).addWidget(self.rewards)
        self.tabs.addTab(reward_page, "奖励")

    def refresh(self):
        status = self.client.get_motivation_status() or {}
        self.overview.setText(
            f"今日有效专注：{status.get('today_focus_minutes', 0)} 分钟\n"
            f"今日目标：{status.get('target_progress', 0):.0%} / {status.get('daily_target_minutes', 120)} 分钟\n"
            f"连续打卡：{status.get('streak_days', 0)} 天\n"
            f"今日 FP：{status.get('today_focus_minutes', 0)}\n"
            f"AP 余额：{status.get('total_ap_milli', 0) / 1000:.3f} AP"
        )
        self.history.clear()
        for day in self.client.get_history(7) or []:
            self.history.addItem(f"{day.get('date')}: {day.get('focus_minutes', 0)} 分钟  {'已打卡' if day.get('checkin_completed') else ''}")

        self.missions.clear()
        for mission in self.client.get_missions() or []:
            item = QListWidgetItem(f"[{mission.get('status')}] {mission.get('title')}  +{mission.get('reward_milli_ap', 0)/1000:.3f} AP")
            item.setData(Qt.ItemDataRole.UserRole, mission.get("id"))
            self.missions.addItem(item)

        self.achievements.clear()
        for achievement in self.client.get_achievements() or []:
            mark = "已解锁" if achievement.get("unlocked") else f"进度 {achievement.get('progress', 0):.0%}"
            self.achievements.addItem(f"{mark}  {achievement.get('name')}: {achievement.get('description')}")

        self.rewards.clear()
        for reward in self.client.get_rewards() or []:
            item = QListWidgetItem(f"{reward.get('name')}  {reward.get('cost_milli_ap', 0)/1000:.3f} AP — 双击兑换")
            item.setData(Qt.ItemDataRole.UserRole, reward.get("id"))
            self.rewards.addItem(item)

    def add_mission(self):
        title, ok = QInputDialog.getText(self, "新增任务", "任务名称")
        if ok and title.strip():
            self.client.create_mission(title.strip())
            self.refresh()

    def complete_selected_mission(self, item):
        result = self.client.complete_mission(item.data(Qt.ItemDataRole.UserRole))
        if result is not None:
            self.refresh()

    def redeem_selected_reward(self, item):
        result = self.client.redeem_reward(item.data(Qt.ItemDataRole.UserRole))
        if result is None and self.client.last_error:
            QMessageBox.information(self, "兑换失败", self.client.last_error)
        self.refresh()
