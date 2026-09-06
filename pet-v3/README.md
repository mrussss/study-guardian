# StudyGuardian Pet v3

Tauri 2 / React / TypeScript 桌面 UI：Pet、Quick Panel、Control Center 三个页面。业务状态仍来自 Supervisor；Rust 读取本地凭据、访问受限 localhost API，只返回脱敏 DTO。

## 当前交互

按住猫身体拖动；点击下方“学习面板”打开快捷面板。原生 CSS 拖动区域与按钮分离，没有 JS 手势定时器或逐帧窗口移动。面板可通过关闭按钮或 Escape 隐藏；托盘也可打开面板、控制中心和设置。

辅助 WebView 必须在异步工作线程里创建。不要在同步 Tauri command 或 UI 事件回调里直接调用 `WebviewWindowBuilder::build()`，否则 Windows 可能死锁。Supervisor 网络请求也不能放回 UI 线程。

## 构建与验证

日常开发从 WSL 仓库使用统一入口：

```sh
../scripts/pet-v3.sh dev          # Vite 热更新，不构建或替换 exe
../scripts/pet-v3.sh check        # 前端测试、类型检查、Vite 构建、diff 检查
../scripts/pet-v3.sh native       # 复用 D 盘缓存构建 Windows debug exe
../scripts/pet-v3.sh candidate    # 检查、Rust 测试、Pet 专用部署与哈希核验
../scripts/pet-v3.sh build        # 生成经过测试的 Windows release Pet 产物
../scripts/pet-v3.sh verify       # 核对 debug 产物与当前运行 exe
```

Windows 原生构建固定使用 `D:\StudyGuardianBuild`，复用 staging、`node_modules`、npm 下载缓存和 debug/release Cargo target。`D:\StudyGuardianDev` 只作为运行目录。Pet 专用部署只停止并替换 `pet-v3\StudyGuardian.exe`，不会停止 Supervisor 或 Sensor。

```sh
npm ci
npm test
npm run build
```

### 浏览器 Mock 场景

`dev` 启动后，Control Center 与 Quick Panel 默认使用状态化 Mock Adapter，页面右上角可切换场景：

```text
http://127.0.0.1:1420/control-center.html?mock=normal
http://127.0.0.1:1420/quick-panel.html?mock=normal
```

支持 `normal`、`slow`、`failure`、`offline`、`rapid`、`progress-empty`、`progress-complete` 和 `reminder`。Mock 与 native 共用正式 adapter 接口；拖动、透明窗口、托盘和 single-instance 仍使用 `native/candidate` 验证。

### 缓存和工具链

```sh
../scripts/pet-v3.sh cache-status
../scripts/pet-v3.sh cache-prune
../scripts/pet-v3.sh cache-reset
```

Pet 固定使用 Node 22.22.1、npm 10.9.4 和 Rust 1.98.1。Windows Node 由构建脚本下载到 D 盘并核对官方 SHA256，不修改系统 Node。完整说明见 [开发工作流](../docs/DEVELOPMENT_WORKFLOW.md)。

Windows 原生工具链中使用已提交的锁文件：

```powershell
cargo check --locked --manifest-path src-tauri/Cargo.toml
cargo test --locked --manifest-path src-tauri/Cargo.toml
npm run tauri build -- --debug --no-bundle
```

Windows 从 WSL 同步的构建副本必须包含最新 `src`、`src-tauri`、`dist` 和锁文件；不要仅更新前端后复用旧 exe。测试前核对实际进程路径、exe 时间与 SHA256。`STUDYGUARDIAN_RUNTIME_DIR` 可指向隔离运行目录，默认现有 `D:\StudyGuardianDev`。

Pet 单页外观调试仍可用 `VITE_PET_DEV_PANEL=1 npm run dev` 显示旧的宠物控制条；Control Center 与 Quick Panel 场景测试统一使用上面的 Mock Adapter。

独立输入/面板实验见 [diagnostics/README.md](diagnostics/README.md)，当前实测与未通过的人工 Gate 见 [架构记录](../docs/PET_INPUT_ARCH_DECISION.md)。完整 Settings 与生产切换尚待后续开发验收。
