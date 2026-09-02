# Windows Runtime Audit

## 程序目录与持久化目录

部署脚本只替换以下程序路径：`bin/study-supervisor.exe`、`bin/config-helper.exe`、`pet/src`、`pet/assets/skins`、`sensor/screen`、`scripts`。`pet\.venv`、`sensor\.venv`、`config`、`data`、`logs`、`run`、`handoff` 不在替换集合内。

部署先复制到目标目录下的临时 staging，再校验关键文件，最后替换明确路径。`tests/test_deploy_safety.sh` 会跨两次部署检查配置、数据库、日志、运行态、handoff 和 token 的 canary。

## 验收命令

```bash
bash scripts/build-windows.sh
bash tests/test_deploy_safety.sh
bash scripts/test-all.sh
```

Windows 端依赖由 `scripts/rebuild-python-envs.ps1` 分别安装 `pet/requirements.txt` 和 `sensor/requirements.txt`，不共享 venv。真实运行前使用 `scripts/start-all.ps1`，停止使用 `scripts/stop-all.ps1`。

## Fail-soft 约束

Supervisor、Sensor、ActivityWatch、AI 任一依赖离线时，桌宠仍可拖动、打开菜单和显示当前连接状态；网络请求不在 Qt GUI 线程执行。Toast 仅由 Supervisor 的提醒引擎触发，Pet 不直接承担业务计时。

锁屏和 Sleep/Resume 按本轮验收决定保留为后续项目，不作为当前日常试运行阻塞项。

## 依赖与死代码分类

| 分类 | 项目 | 结论 |
|---|---|---|
| KEEP | PyQt6 | Pet GUI 直接依赖；另有 GPL/Commercial 发布门禁 |
| KEEP | mss、Pillow | Sensor 的真实屏幕采集依赖 |
| KEEP | Python 标准库 `urllib.request` | Pet Supervisor Client 的 HTTP 实现 |
| KEEP | `modernc.org/sqlite`、`gopkg.in/yaml.v3` | Supervisor 直接 Go 依赖 |
| REMOVE / NOT RUNTIME | `requests`、`pywin32` | 当前源码无真实 Runtime import，不进入 requirements |
| DOC_ONLY | desktop-pet 上游记录 | 仅保留来源与代码许可证说明，不复制其未知来源图片 |
| RUNTIME_LEGACY | 旧 Runtime screenshot、旧 PoC source、旧重复 dist | 只在本机审计；不由新 Deploy 复制回 Git |

## 测量记录

测量原则：磁盘大小、包数量、进程 Working Set、启动时间分别记录；不能用磁盘大小推断 RAM 节省。最终 Windows 数值随每台机器的 Python/驱动和运行状态变化，当前验收记录见 `docs/WINDOWS_E2E_REPORT.md`。

推荐复测命令：

```powershell
Get-ChildItem -Recurse D:\StudyGuardianDev\pet -File | Measure-Object Length -Sum
Get-ChildItem -Recurse D:\StudyGuardianDev\sensor -File | Measure-Object Length -Sum
Get-Process -Name study-supervisor | Select-Object Name,WorkingSet64
Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -like '*StudyGuardianDev*pet*main.py*' -or $_.CommandLine -like '*StudyGuardianDev*sensor*server.py*' } | Select-Object ProcessId,CommandLine
```
