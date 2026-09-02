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
