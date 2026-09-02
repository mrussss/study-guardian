# Study Forest 数据迁移说明

## Study Forest 原有机制

Study Forest 原型已有 FP、AP、每日目标、按日期 streak、Focus Timer、任务清单、奖励兑换和最近 7 天统计，数据保存在浏览器 `localStorage`，没有后端、SQLite 或 AI。

## StudyGuardian 新增与修正

StudyGuardian 使用 Supervisor 作为唯一业务大脑，使用 SQLite 新增以下表：

- `motivation_daily`：按自然日保存有效专注、目标和打卡状态；
- `ap_ledger`：以 milli-AP 记录任务奖励和兑换扣除；
- `missions`、`achievements`：任务与成就的幂等状态；
- `reward_catalog`、`reward_redemptions`：本地奖励目录和兑换记录。
- `motivation_settings`：用户可写的每日目标真源；YAML 只提供首次初始化默认值；
- `ui_events`：Supervisor 持久化事件，Pet 通过 `after_id` cursor 顺序消费。

迁移在 Supervisor 启动时由 `CREATE TABLE IF NOT EXISTS` 完成，不会删除或重建已有业务表。旧数据库可以直接启动；新奖励目录使用 `INSERT OR IGNORE`，因此不会覆盖用户已经调整的奖励项。

有效专注只来自单次 `TickOutcome`，不会由 Motivation 自己再计算 wall-clock delta：`STUDY`、未锁定、ActivityWatch 有效、任务关系不是 `DISTRACTED`。`ACTIVE` 和 `IDLE_DYNAMIC` 全部计入；`IDLE_STATIC` 只有 `FOCUSED` 且当前静止秒数不超过 `idle_static_credit_grace_seconds`（默认 300）才计入；`UNKNOWN` 交互不计入。`ACTIVE`/`IDLE_DYNAMIC` 的 `UNKNOWN` 关系仍可计入，避免 AI 离线时误伤专注时长。所有 AP 使用整数 milli-AP，默认每小时 1000 milli-AP，可通过 `motivation.ap_per_focus_hour_milli` 调整。

FP 是 `credited_focus_seconds / 60`；Focus AP 由累计有效专注确定性计算，不写每 Tick ledger。Ledger 只记录 Mission、Achievement、兑换等离散事务。

公开 API：

`GET /v1/motivation/status`、`GET /v1/motivation/history?days=7`、`GET /v1/motivation/achievements`、`GET/PUT /v1/motivation/settings`、`GET /v1/events?after_id=<cursor>&limit=20`、`GET/POST /v1/missions`、`POST /v1/missions/{id}/complete|cancel`、`GET /v1/rewards`、`POST /v1/rewards/{id}/redeem`。

状态 API 使用 `today_credited_focus_minutes`、`total_credited_focus_minutes`、`today_earned_ap_milli`、`today_spent_ap_milli`、`balance_ap_milli`、`checkin_completed`、`daily_target_minutes`、`target_progress`、`streak_days` 等 canonical 字段，避免把 raw study time 当作奖励时间。

成就阈值与用户设置分离：`DAILY_120` 永远要求单日 120 分钟有效专注，即使用户把每日目标改成 60 分钟；`COMEBACK` 只统计最近一次分心之后连续累积的有效专注，旧的累计专注不会直接触发。该连续状态持久化在 `motivation_comeback_state`，跨进程重启仍保持一致。`TickOutcome.Now` 由 State Manager 写入，Motivation 不再自行取得第二个 wall-clock 时间。

Study Center 只是这些 API 的懒加载 UI，不拥有第二份业务状态。
