# Windows 运行与集成

WSL `~/projects/study-guardian` 是源码真源，`D:\StudyGuardianDev` 是可替换的 Windows 运行目录。构建使用 `scripts/build-windows.sh`，其中 Windows PowerShell helper 在本机 Rust/MSVC 环境生成 `dist/windows/pet-v3/StudyGuardian.exe`。部署脚本带 staging、备份、健康烟测和失败回滚；`config`、`data`、`logs`、`run`、`handoff`、`pet/.venv` 与 `sensor/.venv` 不属于替换集合。

## 稳定 Launcher

桌面和 Startup 快捷方式都调用：

```text
D:\StudyGuardianDev\scripts\launch-studyguardian.ps1
```

`-Background` 只在后台启动运行时，`-OpenControlCenter` 打开控制中心，`-OpenQuickPanel` 打开快捷面板。Launcher 从自己的路径推导根目录，不依赖当前工作目录。启动和停止脚本只按完整安装路径识别产品进程；watchdog 有重试窗口和退避，日志写入有界的 `logs/launcher.log`。

Tauri EXE 使用 single-instance IPC。重复桌面启动不会创建第二套 Supervisor、Sensor、watchdog 或 UI shell，而是让已有进程显示请求的窗口。

## Pet 选择与回退

`config/runtime.json` 示例：

```json
{"pet_runtime":"pyqt"}
```

缺少或损坏 marker 时安全回退到 `pyqt`。Tauri Pet 通过人工拖动/点击 Gate 前不得改为默认值。在 PyQt 模式下，Tauri EXE 使用 `--no-pet` 只承载现代 Control Center；屏幕上仍只有 PyQt Pet。部署失败可回滚程序文件，选择 marker 和用户数据保持不变。

## 桌面与开机启动

```powershell
# 创建或修复桌面入口
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\set-desktop-shortcut.ps1 -Create

# 开启、查询、关闭开机启动
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\set-autostart.ps1 -Enable
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\set-autostart.ps1 -GetState
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\set-autostart.ps1 -Disable
```

脚本使用 `[Environment]::GetFolderPath` 获取实际 Desktop 和 Startup 文件夹，并且重复执行只保留一个 `StudyGuardian.lnk`。Control Center 的 General 设置通过 native command 调用同一 autostart 脚本。部署不会自行打开开机启动。

## 免打扰与 Review

默认 quiet periods 是 12:00–14:00、17:30–19:00、21:00–24:00。quiet 只抑制主动提醒，继续记录学习证据；离开 quiet 后不会补发提醒债务。

Review 的 `FALLBACK` 表示确定性本地总结。`AI` 表示结果经过 provider、cloud sanitizer、结构校验和 evidence validator；两种模式都写入 canonical Review storage。
