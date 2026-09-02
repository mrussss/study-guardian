# StudyGuardian 独立全量审计交接文档 (FINAL AUDIT)

## 1. 审计基线与元信息

- **项目名称**：StudyGuardian
- **源码 Source of Truth**：WSL2 `~/projects/study-guardian`
- **Windows 部署与运行目录**：`D:\StudyGuardianDev` (`/mnt/d/StudyGuardianDev`)
- **开发轮次**：v0.3 全 Phase (Phase 0 ～ Phase 5) 完成
- **Git HEAD Commit**：请通过 `git log -n 1` 获取
- **发布 Tag**：`phase0-passed`

---

## 2. 架构合规性核查矩阵

| 架构设计约束 | 要求说明 | 实现位置 | 状态 |
|---|---|---|---|
| **单一真源** | 正式源码在 WSL，不在 Windows 运行时直接编辑 | `~/projects/study-guardian` | [x] 合规 |
| **构建部署分离** | Go 编译生成物留在 `dist/windows/`，通过脚本部署到 D 盘 | `scripts/build-windows.sh`, `scripts/deploy-windows.sh` | [x] 合规 |
| **持久数据保护** | 部署与重启严禁删除 `config/`, `data/`, `logs/`, `run/`, `handoff/` | `tests/test_deploy_safety.sh` | [x] 合规 |
| **三维正交观察模型** | `InteractionState` / `TaskRelation` / `PrivacyState` 严格解耦 | `internal/state/types.go` | [x] 合规 |
| **用户意图与观察解耦**| 用户主动声明 `UserMode`，系统不隐式擅自篡改模式 | `internal/state/manager.go` | [x] 合规 |
| **可测试时钟抽象** | 核心状态机基于 `Clock` 接口，支持跨自然日与 Lock/Sleep 暂停 | `internal/state/clock.go`, `internal/state/manager.go` | [x] 合规 |
| **AW 动态发现** | 动态探测 window/afk/web buckets，禁止硬编码 Hostname | `internal/activitywatch/client.go` | [x] 合规 |
| **隐私门禁优先** | 截图前先过本地 Privacy Gate，敏感窗口绝对阻断截图与视觉 AI | `internal/rules/privacy_gate.go` | [x] 合规 |
| **规则优先，AI 兜底** | 明确白名单/黑名单优先，模糊歧义才调用 AI，多维哈希缓存 | `internal/classifier/service.go` | [x] 合规 |
| **桌宠纯 UI Shell** | 剥离桌面宠物的独立业务逻辑，仅连接 Supervisor | `pet/src/` | [x] 合规 |
| **Localhost 安全** | 仅绑定 `127.0.0.1`，Bearer Token 认证，脱敏日志 | `internal/api/server.go`, `internal/config/config.go` | [x] 合规 |

---

## 3. 测试覆盖与验证记录

### 3.1 单元测试集合
- `go test -v ./...`
  - `internal/api`: API 路由、Token 拦截、模式切换；
  - `internal/state`: 状态机、跨日重置、AFK 活跃累计；
  - `internal/storage`: SQLite 迁移、会话保存、观察与缓存；
  - `internal/rules`: 隐私门禁、规则引擎、分词；
  - `internal/reminder`: 提醒阈值、Cooldown、降频；
  - `internal/activitywatch`: Bucket 动态发现、有界查询、域名提取；
  - `internal/sensor`: dHash 差异、Hamming 距离、鉴权；
  - `internal/classifier`: 结构化分类、多维缓存、优雅降级；
  - `internal/platform/windows`: 日志轮转、Toast 通知。
- `python3 pet/tests/test_client.py`: Pet Client 5 项用例全通。
- `python3 sensor/tests/test_sensor.py`: Sensor 4 项用例全通。

### 3.2 阶段集成测试
- `python3 tests/integration/test_localhost_poc.py`: Phase 0 三组件通信与 fail-soft (PASS)
- `python3 tests/integration/test_phase1_core.py`: Phase 1 确定性核心生命周期 (PASS)
- `python3 tests/integration/test_phase2_activitywatch.py`: Phase 2 ActivityWatch 驱动 (PASS)
- `python3 tests/integration/test_phase3_sensor.py`: Phase 3 屏幕变化与门禁阻断 (PASS)
- `python3 tests/integration/test_phase4_ai.py`: Phase 4 AI 分类与缓存回退 (PASS)
- `bash tests/test_deploy_safety.sh`: 部署安全与数据持久保护 (PASS)

---

## 4. 独立审计核查清单 (Checklist for Auditor)

1. [ ] 运行 `go test -v ./...` 确认全套 Go 测试通过；
2. [ ] 运行所有 Python 单元与集成测试确认通过；
3. [ ] 检查 `D:\StudyGuardianDev` 目录结构与权限；
4. [ ] 检查 `.gitignore` 确保无敏感数据或 token 被提交至 Git；
5. [ ] 检查所有 API 与服务端是否均仅监听 `127.0.0.1`；
6. [ ] 审查 `docs/OPEN_SOURCE.md` 开源许可证归属；
7. [ ] 检查日志脱敏，确认 Authorization 与敏感窗口不被记录。
