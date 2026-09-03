# AGENTS.md — StudyGuardian AI 协作指南

## 1. 架构原则与执行约束
- **单一主开发者**：WSL Codex CLI 作为核心开发者，在 WSL2 (`~/projects/study-guardian`) 编写正式源码、单测与构建脚本。
- **运行与交付目标**：Windows 11 (`D:\StudyGuardianDev` / `/mnt/d/StudyGuardianDev`)。
- **Source of Truth**：源码唯一真源在 WSL `~/projects/study-guardian`，禁止在 Windows 运行时目录直接编辑正式源码。
- **Build 与 Deploy 分离**：构建输出留在 WSL `dist/windows/`，部署通过 `scripts/deploy-windows.sh` 复制到 `D:\StudyGuardianDev`。
- **持久数据保护**：部署严禁删除或覆盖 `D:\StudyGuardianDev` 下的 `config/`、`data/`、`logs/`、`run/`、`handoff/` 目录。
- **三维系统观察解耦**：严格拆分 `InteractionState`（ACTIVE/IDLE_STATIC/IDLE_DYNAMIC/UNKNOWN）、`TaskRelation`（FOCUSED/DISTRACTED/UNKNOWN）、`PrivacyState`（NORMAL/SENSITIVE），禁止合并为单一枚举。
- **小步提交与推送**：每完成一个独立模块/功能并验证后立即 Git commit，并在远端可用时 push。
- **汇报语言**：所有面向用户的开发汇报、阶段总结与审计材料均使用中文。

## 2. 模块职责与端口约定
- **Supervisor** (`127.0.0.1:17321`): 唯一的业务与状态机大脑（Go 实现，CGO-free SQLite）。
- **Screen Sensor** (`127.0.0.1:17322`): 极薄屏幕采集与 diff 适配层（Python/mss 实现）。
- **Study Pet** (PyQt6 UI Shell): 纯 UI 交互层，不含独立业务/监督逻辑，仅连接 Supervisor。
- **ActivityWatch** (`127.0.0.1:5600`): 外部事实采集源，通过 Adapter 访问，不修改上游。

## 3. Daily Review 与浏览器采集约束
- Daily Review 不得引入第二套 Supervisor/状态机。
- Review Evidence 是聚合层，不得复制现有 sessions/observations/distraction_events 成第二真源。
- ChatGPT 自动采集必须先 baseline，禁止把页面已有历史消息当今天新消息。
- Chat Turn 的 eligibility 在用户 Turn 开始时冻结，Assistant 继承。
- 日历归属使用 observed_at + Supervisor local date，不使用 ingested_at。
- Collector 使用 scoped token，不得持有主业务 Bearer Token；Content Script 永远不得读取 localhost token。
- Daily Review 不得增加 Screen capture / Vision 调用频率；Review AI 使用通用 AI transport，不得直接复用 TaskRelationProvider。
- Review 必须有 deterministic fallback，且生成必须异步，不阻塞 mode API。
- Topic Evidence 与 Accomplishment Evidence 必须区分；AI accomplishments 必须引用有效的 accomplishment evidence。
- Collector 故障不得影响实时监督；ActivityWatch enrichment 故障不得导致 Review 整体失败。
- raw chat / review / token 禁止进入 Git；复用开源 ChatGPT parser 时必须记录 upstream SHA/license。
