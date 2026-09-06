# Pet v3 开发工作流

## 目录职责

| 位置 | 职责 | 是否可以重建 |
|---|---|---|
| WSL `~/projects/study-guardian` | 唯一正式源码和 Git 真源 | 否 |
| `D:\StudyGuardianBuild` | staging、Node、npm、Cargo、artifact 和部署备份缓存 | 是 |
| `D:\StudyGuardianDev` | Windows 运行副本以及 config/data/logs/run/handoff 持久数据 | 程序文件可替换，持久数据不可删除 |

禁止在 `D:\StudyGuardianDev` 直接修改源码。缓存出现异常时使用受路径保护的缓存命令，不要手工把构建目录重新放回 C 盘。

## 日常循环

从 WSL 仓库根目录执行：

```bash
./scripts/pet-v3.sh dev
./scripts/pet-v3.sh check
./scripts/pet-v3.sh native
./scripts/pet-v3.sh candidate
```

- `dev`：启动 Vite HMR。用于 React、CSS、任务交互、状态页面和错误状态开发。
- `check`：运行前端测试、TypeScript 与 Vite 构建、`git diff --check`。
- `native`：复用 D 盘缓存生成 Windows debug EXE，不替换正在运行的程序。
- `candidate`：提交前门禁；运行前端/Rust 测试、原生构建、Pet 专用部署、哈希与单实例核验。
- `build`：生成经过测试的 release Pet 产物。
- `release`：执行完整产品构建。只有需要同时交付 Supervisor、Sensor、Pet 和脚本时使用。

只有完整产品发布使用 `scripts/build-windows.sh` 与 `scripts/deploy-windows.sh`。普通 Pet UI 修改不得运行完整产品部署。

## 浏览器 Mock

Vite 浏览器页面默认使用状态化 Mock Adapter，不访问真实 token、Supervisor 或 Windows 设置：

```text
http://127.0.0.1:1420/control-center.html?mock=normal
http://127.0.0.1:1420/quick-panel.html?mock=normal
```

页面右上角可以切换场景，也可以直接使用：

| 参数 | 场景 |
|---|---|
| `normal` | 正常 Supervisor 数据和可写交互 |
| `slow` | 约 1 秒控制延迟 |
| `failure` | 控制请求被拒绝 |
| `offline` | Supervisor 离线 |
| `rapid` | 延迟中的快速任务连点 |
| `progress-empty` | 今日尚未开始 |
| `progress-complete` | 今日目标已完成 |
| `reminder` | 分心提醒状态 |

Mock 与 native 使用同一组 `SupervisorDashboardAdapter`、`SupervisorControlAdapter` 和 `SystemIntegrationAdapter` 接口。透明窗口、拖动、托盘、single-instance 和真实 Windows 集成仍必须使用 `native/candidate` 构建，并通过下面的原生视觉 Gate 验收。

## Windows 原生视觉 Gate

`native` / `candidate` 的进程、日志和哈希结果不能证明窗口最终画面正确。涉及 Pet、Quick Panel 或 Control Center 的原生行为时，按以下顺序验收：

1. 先完成 `check` 和所需的 `native` / `candidate`，确认被观察的是最新运行产物。
2. 使用已安装的 `computer-use` skill。专用 `node_repl` 首次调用只初始化 `@oai/sky`：

   ```javascript
   if (!globalThis.sky) {
     const { sky } = await import("@oai/sky");
     globalThis.sky = sky;
   }
   ```

3. 调用 `sky.list_apps()` 或 `sky.list_windows()`；从返回对象中选择唯一的 StudyGuardian 进程和目标窗口，禁止猜测 app id、window id 或句柄。
4. 使用 `sky.get_window_state(..., { include_screenshot: true, include_text: true })` 获取真实窗口截图和可访问性树。按改动范围检查 Pet、Quick Panel 圆角与外部透明区、Control Center、任务切换、失焦隐藏以及可定位时的托盘/任务栏图标。
5. 报告中写明实际观察到的窗口、截图结论和仍未覆盖的 Gate。日志只能作为事件与链路证据，不能代替画面。

统一 `cua_repl` 只列出浏览器时，说明该统一入口当前配置为 browser-only。此时改用上述专用 Computer Use 通道，不得直接把原生能力标记为不可用。若窗口枚举超时，等待约 2 秒后重试一次；仍失败时重置专用 REPL、重新初始化 `@oai/sky` 并再次枚举。只有这套恢复流程仍无法取得目标窗口时，才能降级为用户人工视觉 Gate。若工具报告检测到用户输入，立即停止 UI 自动操作，保留已取得的证据并说明剩余项目。

构建、启动和日志命令继续通过普通 shell 与项目脚本执行，不使用 Computer Use 操作终端。

## 缓存维护

```bash
./scripts/pet-v3.sh cache-status
./scripts/pet-v3.sh cache-prune
./scripts/pet-v3.sh cache-reset
```

- `cache-prune` 保留最近 3 个 Pet 备份，删除超过 14 天的日志、超过 1 天的测试目录和超过 30 天未使用的 release Cargo cache。
- `cache-reset` 只允许清空 `D:\StudyGuardianBuild` 内的可重建内容；若缓存中的构建工具仍在运行则拒绝执行。
- reset 后第一次 native 构建会重新下载并校验固定 Node，重新安装 npm 依赖并进行冷 Rust 编译。

## 固定工具链

- Node：22.22.1
- npm：10.9.4
- Rust：1.98.1
- Rust target：`x86_64-pc-windows-msvc`

WSL 通过 `.nvmrc` 和 `.node-version` 固定 Node。Windows 构建通过 `scripts/ensure-windows-node.ps1` 下载 Node 官方 ZIP、核对官方 SHA256，并安装到 D 盘缓存；不会修改系统 Node。Rust 通过 `pet-v3/rust-toolchain.toml` 固定。

## CI 门禁

`.github/workflows/ci.yml` 在 push 到 main 和 Pull Request 时运行：

- Pet 前端测试、TypeScript、Vite build
- Go、Python 与集成测试
- 部署安全测试
- PowerShell 全脚本解析
- Pet 专用部署与缓存删除安全测试

`.github/workflows/windows-native.yml` 在影响 Pet 的 Pull Request 或手动触发时运行 Windows Tauri debug 构建，避免普通提交都承担原生编译成本。

## 故障定位

1. `check` 失败：先修前端、类型或测试，不进入 Windows 构建。
2. `native` 失败：查看工具链版本、`D:\StudyGuardianBuild\artifacts` 和 manifest。
3. `candidate` 部署失败：脚本自动恢复旧 EXE；检查备份和哈希输出。
4. 只在缓存确定损坏时执行 `cache-reset`；常规空间回收使用 `cache-prune`。
