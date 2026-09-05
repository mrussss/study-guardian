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

## Deterministic fallback v2

AI 关闭、未配置或请求失败时，Review 使用本机证据生成 `generation_mode=FALLBACK` 的事实型总结。主任务按 credited study duration 和最后活动排序；主题证据顺序为 task、semantic snapshot、eligible chat turn。输出明确分为“今日进展”“可以确认”“不能确认”和与真实任务相关的明日优先级，不会把停留时长推断成掌握、完成或质量评价。

结束学习后的 debounce 仍会自动生成 Review；Control Center 也可以立即生成。UI 将 `FALLBACK` 显示为“本地总结”，只有通过 provider、sanitizer 和 validator 的结果才显示为 AI 总结。默认免打扰时段只抑制提醒，不停止数据记录或 Review 生成。
