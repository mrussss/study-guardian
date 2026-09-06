# StudyGuardian 开发指南

## 1. Pet v3 日常开发

WSL `~/projects/study-guardian` 是唯一源码真源，`D:\StudyGuardianBuild` 是可删除、可重建的 Windows 构建缓存，`D:\StudyGuardianDev` 只保存运行副本和持久数据。不要在 Windows 运行目录编辑源码。

```bash
# React/CSS 与完整状态化浏览器 Mock
./scripts/pet-v3.sh dev

# 前端测试、TypeScript、Vite 和 diff 检查
./scripts/pet-v3.sh check

# 复用 D 盘缓存构建 Windows debug EXE
./scripts/pet-v3.sh native

# 提交前：前端/Rust 测试、构建、Pet 独立部署、哈希核验
./scripts/pet-v3.sh candidate
```

浏览器 Mock 支持正常、慢请求、失败、离线、快速任务切换、空进度、完成进度和提醒场景。完整命令、URL 和缓存维护见 [Pet v3 开发工作流](docs/DEVELOPMENT_WORKFLOW.md)。

## 2. Windows 原生视觉验收

`native` 和 `candidate` 证明 Windows 产物可构建、可部署，但不能替代透明窗口、圆角、失焦、托盘、任务栏和实际点击的视觉验收。涉及这些行为时，使用已安装的 `computer-use` skill，并通过专用 `node_repl` 加载 `@oai/sky`：

```javascript
if (!globalThis.sky) {
  const { sky } = await import("@oai/sky");
  globalThis.sky = sky;
}
```

随后使用 `sky.list_apps()` / `sky.list_windows()` 选择返回的 StudyGuardian 进程和窗口，并用 `sky.get_window_state(...)` 获取真实截图与可访问性状态。统一 `cua_repl` 若只返回浏览器，表示该入口为 browser-only，不代表原生 UI 能力不可用。详细恢复步骤和验收范围见 [Pet v3 开发工作流](docs/DEVELOPMENT_WORKFLOW.md)。

## 3. 完整产品构建与部署

只有需要同时发布 Supervisor、Sensor、Pet 和 Windows 脚本时使用：

```bash
./scripts/build-windows.sh
./scripts/deploy-windows.sh /mnt/d/StudyGuardianDev
```

完整部署必须保护 `config`、`data`、`logs`、`run`、`handoff` 和虚拟环境。普通 Pet UI 修改不得运行完整产品部署。

### Windows 启动与停止

```powershell
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\launch-studyguardian.ps1 -OpenControlCenter
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\launch-studyguardian.ps1 -Background
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\stop-all.ps1
powershell.exe -ExecutionPolicy Bypass -File D:\StudyGuardianDev\scripts\install-windows-integration.ps1
```

Pet 运行时由 `D:\StudyGuardianDev\config\runtime.json` 选择，切换必须遵守对应人工 Gate，详见 `docs/WINDOWS_RUNTIME.md`。

## 4. 自动化测试套件

```bash
# 本地完整测试
bash scripts/test-all.sh

# Pet 快速门禁
./scripts/pet-v3.sh check
```

完整测试覆盖 Go、Python、集成测试、部署安全、PowerShell 解析、Pet 独立部署和缓存删除边界。GitHub CI 配置见 `.github/workflows/ci.yml`；Windows Tauri 构建在 Pet Pull Request 或手动工作流中运行。
