# StudyGuardian 开发指南

## 1. 快速构建与部署

### WSL 端构建
```bash
# 运行单元测试并交叉编译 Windows 可执行文件
./scripts/build-windows.sh
```

### 部署至 Windows
```bash
# 复制构建产物至 D:\StudyGuardianDev 并自动保护持久数据
./scripts/deploy-windows.sh /mnt/d/StudyGuardianDev
```

### Windows 启动与停止 (PowerShell)
```powershell
# 用户入口：启动组件并打开已有或新的 Control Center
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\launch-studyguardian.ps1 -OpenControlCenter

# 后台启动所有组件
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\launch-studyguardian.ps1 -Background

# 停止所有组件
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\stop-all.ps1

# 创建桌面入口；开机启动保持用户当前选择
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\install-windows-integration.ps1
```

Pet 运行时由 `D:\StudyGuardianDev\config\runtime.json` 选择。人工 Tauri Pet Gate
通过前保持 `{"pet_runtime":"pyqt"}`。详见 `docs/WINDOWS_RUNTIME.md`。

---

## 2. 自动化测试套件

```bash
# 1. 运行所有 Go 单元测试
go test -v ./...

# 2. 运行 Python 单元测试
python3 pet/tests/test_client.py
python3 sensor/tests/test_sensor.py

# 3. 运行全阶段集成测试
python3 tests/integration/test_localhost_poc.py
python3 tests/integration/test_phase1_core.py
python3 tests/integration/test_phase2_activitywatch.py
python3 tests/integration/test_phase3_sensor.py
python3 tests/integration/test_phase4_ai.py

# 4. 运行部署安全测试
./tests/test_deploy_safety.sh

# 5. Windows PowerShell 路径与快捷方式测试
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tests\test-windows-integration.ps1
```
