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

Mock 与 native 使用同一组 `SupervisorDashboardAdapter`、`SupervisorControlAdapter` 和 `SystemIntegrationAdapter` 接口。透明窗口、拖动、托盘、single-instance 和真实 Windows 集成仍必须使用 `native/candidate` 验证。

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
