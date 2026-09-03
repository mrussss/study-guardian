# Daily Review 实现状态

当前已完成 Daily Review 基础设施：

- `internal/ai` 提供业务无关的 OpenAI-compatible `JSONClient`，包含认证、超时、JSON mode fallback；
- SQLite 已迁移 `chat_conversations`、`chat_turns`、`chat_messages`、`semantic_snapshots`、`review_exclusions`、`daily_reviews`；
- Collector 使用独立 token 和独立 API 路径，Turn 按 `observed_at` 归属日期；
- ChatGPT MV3 baseline、Turn eligibility 冻结与离线队列 POC 已加入构建产物。

Evidence aggregator、deterministic fallback、Review worker、Markdown 原子输出和
Study Center UI 仍按 `TASKS.md` 的 Phase 8 顺序开发。当前实现不会新增白天
Screen capture 或 Vision 调用。
