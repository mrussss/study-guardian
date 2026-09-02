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
# 启动所有组件 (ActivityWatch + Supervisor + Sensor + Pet)
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\start-all.ps1

# 停止所有组件
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\stop-all.ps1
```

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
```
