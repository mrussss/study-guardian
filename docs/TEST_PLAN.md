# StudyGuardian 测试方案与覆盖矩阵

## 1. 测试层级架构

1. **单元测试 (Unit Tests)**：
   - `internal/state/`: 时钟抽象、状态转移、跨自然日、Sleep/Lock 暂停、会话时长计算；
   - `internal/rules/`: 隐私门禁匹配、确定性规则分类、任务关键词分词匹配；
   - `internal/reminder/`: 提醒阈值决策、冷却机制 (Cooldown)、降频与多级提醒；
   - `internal/storage/`: SQLite CGO-free 驱动、表结构迁移、会话与观察记录、分类缓存 TTL；
   - `internal/activitywatch/`: Bucket 动态发现、有界事件解析、URL 域名提取、离线容错；
   - `internal/sensor/`: dHash 计算、Hamming 差异距离计算、认证鉴权、多显示器捕获；
   - `internal/classifier/`: 结构化 JSON 校验、缓存优先、AI 失败优雅回退；
   - `pet/tests/`: PyQt 客户端 API 通信、连接失败与 401 软失败处理。

2. **集成测试 (Integration Tests)**：
   - `tests/integration/test_localhost_poc.py`: Phase 0 Pet → Supervisor → Sensor 三组件 localhost 通信及 fail-soft 测试；
   - `tests/integration/test_phase1_core.py`: Phase 1 状态机全生命周期、会话记录、用户反馈闭环集成测试；
   - `tests/integration/test_phase2_activitywatch.py`: Phase 2 ActivityWatch 动态发现、窗口/AFK 轮询与监督驱动集成测试；
   - `tests/integration/test_phase3_sensor.py`: Phase 3 屏幕变化检测与 Privacy Gate 截图阻断集成测试；
   - `tests/integration/test_phase4_ai.py`: Phase 4 AI 分类服务、多维缓存与兜底机制集成测试。

3. **安全部署测试 (Deploy Safety Tests)**：
   - `tests/test_deploy_safety.sh`: 验证多次连续构建与部署时，绝不覆盖或删除 `D:\StudyGuardianDev\config`、`data`、`logs`、`run`、`handoff` 下的用户数据与凭证。
