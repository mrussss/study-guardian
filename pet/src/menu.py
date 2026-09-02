from PyQt6.QtWidgets import QMenu, QInputDialog, QMessageBox
from PyQt6.QtGui import QAction, QIcon


class PetContextMenu:
    def __init__(self, parent, client, on_mode_changed=None):
        self.parent = parent
        self.client = client
        self.on_mode_changed = on_mode_changed

    def show_menu(self, pos, current_status=None):
        menu = QMenu(self.parent)
        menu.setStyleSheet("""
            QMenu {
                background-color: #ffffff;
                border: 1px solid #e5e7eb;
                border-radius: 8px;
                padding: 6px;
                font-size: 13px;
                color: #1f2937;
            }
            QMenu::item {
                padding: 6px 18px;
                border-radius: 4px;
            }
            QMenu::item:selected {
                background-color: #eff6ff;
                color: #2563eb;
            }
            QMenu::separator {
                height: 1px;
                background: #e5e7eb;
                margin: 4px 0px;
            }
        """)

        # Status header
        mode_str = "未连接"
        task_str = ""
        if current_status:
            mode = current_status.get("user_mode", "STANDBY")
            task = current_status.get("task", "")
            if mode == "STUDY":
                mode_str = f"🟢 学习中: {task}" if task else "🟢 学习中"
            elif mode == "BREAK":
                mode_str = "🟡 休息中"
            elif mode == "OFF":
                mode_str = "⚪ 今日已结束"
            else:
                mode_str = "⚪ 待机中"

        status_action = QAction(mode_str, self.parent)
        status_action.setEnabled(False)
        menu.addAction(status_action)
        menu.addSeparator()

        # Mode actions
        study_action = QAction("开始学习...", self.parent)
        study_action.triggered.connect(self._handle_start_study)
        menu.addAction(study_action)

        break_action = QAction("开始休息", self.parent)
        break_action.triggered.connect(self._handle_start_break)
        menu.addAction(break_action)

        off_action = QAction("结束今天", self.parent)
        off_action.triggered.connect(self._handle_start_off)
        menu.addAction(off_action)

        menu.addSeparator()

        task_action = QAction("修改当前任务...", self.parent)
        task_action.triggered.connect(self._handle_change_task)
        menu.addAction(task_action)

        menu.addSeparator()

        quit_action = QAction("退出桌宠", self.parent)
        quit_action.triggered.connect(self.parent.close)
        menu.addAction(quit_action)

        menu.exec(pos)

    def _handle_start_study(self):
        task, ok = QInputDialog.getText(self.parent, "开始学习", "请输入当前学习任务（例如：Go 语言并发编程）：")
        if ok:
            res = self.client.set_mode_study(task.strip())
            if self.on_mode_changed:
                self.on_mode_changed()

    def _handle_start_break(self):
        self.client.set_mode_break()
        if self.on_mode_changed:
            self.on_mode_changed()

    def _handle_start_off(self):
        self.client.set_mode_off()
        if self.on_mode_changed:
            self.on_mode_changed()

    def _handle_change_task(self):
        task, ok = QInputDialog.getText(self.parent, "修改任务", "请输入新的学习任务名称：")
        if ok and task.strip():
            self.client.set_task(task.strip())
            if self.on_mode_changed:
                self.on_mode_changed()
