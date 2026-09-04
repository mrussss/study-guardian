# Daily Review 实现状态

截至 2026-09-04，Daily Review 主线 92–100 已在当前 WSL HEAD 完成并通过 Go
Review 单测；实现不会新增白天 Screen capture 或 Vision 调用。

已完成：

- `internal/ai` 提供业务无关的 OpenAI-compatible `JSONClient`，包含认证、超时、JSON mode fallback；
- SQLite 已迁移 `chat_conversations`、`chat_turns`、`chat_messages`、`semantic_snapshots`、`review_exclusions`、`daily_reviews`；
- Collector 使用独立 token 和独立 API 路径，Turn 按 `observed_at` 归属日期；
- Evidence aggregator、deterministic fallback、conversation compaction、ReviewProvider、evidence validator、cloud sanitizer；
- canonical daily review persistence、input hash、Markdown 原子输出、READY 时序、OFF debounce/stale/restart/backfill；
- Pet / Study Center Review UI，以及仅允许 `chat_turn:` / `chat_conversation:` 的安全 evidence exclusion；
- raw chat / semantic retention：Supervisor 启动时及每日 worker 按 `observed_at` 清理，保留 daily reviews、sessions 和 Study Forest 数据。

仍未完成：

- Task 101 Full Windows E2E；
- Task 102–104 的长期试运行、Collector 审计和 Review factuality audit；
- Task 89 的真实 Chrome + ChatGPT E2E 仍受 Debugger unattached 阻塞。
