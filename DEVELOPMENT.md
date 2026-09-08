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

## 5. AI 网络与模型 fallback

AI 的 `ai.proxy` 是文字、视觉和 Daily Review 共享的 transport 策略。默认 `environment` 只读取 Supervisor 进程的 `HTTP_PROXY`/`HTTPS_PROXY`；`direct` 强制直连；`manual` 仅接受不带凭据、路径、查询或片段的 `http(s)` 主机地址。Control Center 保存草稿后才能执行“测试网络”，该测试请求文字端点的 `/models`，合法 HTTP 响应即视为网络可达，不返回响应正文。

模型 fallback 只接受明确的模型级失败：模型不存在、无可用通道、明确模型限流或无效输出。裸 429、账号/配额、鉴权、DNS/TCP、代理、TLS、取消和未归属超时都停止当前 provider 的模型链。没有真实凭据时，不能把模型连接测试或视觉 JPEG E2E 标记为通过。

### 真实 AIHubMix E2E 记录（2026-09-07）

- 代理测试：`manual`，1005ms，`error_kind` 为空；Text、Vision、Daily Review 均通过同一共享 transport 和手动代理。
- Text：provider `aihubmix`，model `glm-5.3-flash`，4792ms，`error_kind` 为空。
- Vision：provider `aihubmix`，model `ox-alpha`，4853ms，`error_kind` 为空；使用真实 JPEG 请求验证，不把普通文字模型作为视觉模型。
- Daily Review：`READY` / `AI`，provider `aihubmix`，model `glm-5.3-flash`，生成 API 总耗时 45772ms，`error_code` 为空；监督状态接口未被阻塞。
- 本轮曾验证 20 秒和 30 秒超时会写入 deterministic fallback；模型级失败与传输/账户失败的 fallback 分类由 Go 回归测试覆盖。费用信息不由这些接口返回，因此只记录为“未知/可能产生费用”，不推断金额。
- 记录不包含 API Key、请求正文、截图内容、聊天内容或个人数据。

## 6. 自动监督与语义证据

- Daily Review 的 sessions、observations、reminders、distraction events 和 AP ledger 使用显式 `local_date`；新写入时间去除 Go monotonic clock 并保存为 UTC，日历归属由 Supervisor 本地时区计算。
- `DistractionTracker` 独立维护连续偏离区间；短暂窗口切换不会直接落库，恢复专注、锁屏、BREAK/OFF、重启和跨午夜会关闭当前区间。
- 本地规则、文字 AI 和视觉 AI 共用结构化分类结果；只保存经过白名单、长度和隐私校验的 activity/topic/subtopic/action/progress/source 字段，不保存原始截图或原始 Provider 响应。
- 自动学习计时默认关闭。启用后由独立 `AutomationController` 在 Supervisor 主循环外生成转场意图，Manager 在锁外应用；手动 BREAK/OFF 保留人工优先级，主动分心不会自动结束学习会话。
- 自动化检查依次使用 `go test ./...`、`./scripts/pet-v3.sh check`、Windows `cargo test`、`./scripts/pet-v3.sh native` 和 `./scripts/pet-v3.sh candidate`。真实游戏/锁屏/静态离开、Windows Toast 和 Vision JPEG 仍必须在目标机器按 Windows E2E Gate 单独验收，不能用单元测试替代。
- 第二轮修复增加了重启时长迁移标记 `session_duration_repairs`；只对可确定继承旧时长的相邻 `RESTART_RECOVERY` 会话做事务修正，重复启动幂等，无法确定的历史行不猜测修改。
- Daily Review 以 `evidence_revisions` 和 `generated_evidence_revision` 判断新证据；生成期间发生新记录时结果保留但状态为 `STALE`。学习主题只接受正常隐私、FOCUSED、置信度达标且稳定的枚举 Activity，MESSAGING/GAMING/UNKNOWN 不进入学习主题。
- 自动化确认意图通过 `/v1/automation/pending`、`/accept`、`/reject` 暴露给 Control Center；确认队列 15 秒过期。`auto_pause.confirm` 的界面文案为“暂停前提醒”，手动 STUDY 仍允许锁屏/静止自动暂停，手动 BREAK/OFF 不会自动恢复。
- 提醒的 `current_reminder` 仅表示当前有效提醒；历史行保留，恢复稳定专注 20 秒、模式切换或有效期到达会标记 inactive 并从 `/v1/status` 清除。
- 本轮验证：`go test ./...`、Pet 84 项测试、TypeScript/Vite build、Windows Rust 17/17、native/candidate、完整 Windows release build/deploy 均通过。部署后代理测试 `manual` 1239ms；Text provider `aihubmix` / model `coding-glm-5.3-flash-free` 896ms，`account_rate_limited`，按规则停止同 provider fallback；Vision provider `aihubmix` / model `minimax-m3-free` 7962ms 通过；Daily Review 接口生成曾返回 `READY` / `FALLBACK`、revision 6、`error_kind=account_rate_limited`，生成本地总结成功，随后因新证据按设计变为 `STALE`。实机锁屏/游戏/微信/Toast 和完整 UI E2E 仍按目标机 Gate 单独验收。
