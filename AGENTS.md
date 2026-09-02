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
