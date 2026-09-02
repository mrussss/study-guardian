# Study Forest 数据迁移说明

Phase 6 使用 SQLite 新增以下表：

- `motivation_daily`：按自然日保存有效专注、目标和打卡状态；
- `ap_ledger`：以 milli-AP 记录任务奖励和兑换扣除；
- `missions`、`achievements`：任务与成就的幂等状态；
- `reward_catalog`、`reward_redemptions`：本地奖励目录和兑换记录。

迁移在 Supervisor 启动时由 `CREATE TABLE IF NOT EXISTS` 完成，不会删除或重建已有业务表。旧数据库可以直接启动；新奖励目录使用 `INSERT OR IGNORE`，因此不会覆盖用户已经调整的奖励项。

有效专注只来自单次 `TickOutcome`：`STUDY`、未锁定、ActivityWatch 有效、交互为 `ACTIVE`/`IDLE_DYNAMIC`，且任务关系不是 `DISTRACTED`。`UNKNOWN` 关系可以计入，避免 AI 离线时误伤专注时长。所有 AP 使用整数 milli-AP，默认每小时 1000 milli-AP，可通过 `motivation.ap_per_focus_hour_milli` 调整。

公开 API：

`GET /v1/motivation/status`、`GET /v1/motivation/history?days=7`、`GET /v1/motivation/achievements`、`GET/POST /v1/missions`、`POST /v1/missions/{id}/complete|cancel`、`GET /v1/rewards`、`POST /v1/rewards/{id}/redeem`。

Study Center 只是这些 API 的懒加载 UI，不拥有第二份业务状态。
