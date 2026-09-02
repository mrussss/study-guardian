# StudyGuardian

AI 学习监督系统（StudyGuardian）是一个个人自用、长期后台运行、低打扰的 AI 学习监督助手。

## 核心设计特性

- **手动意图与系统观察分离**：用户主动声明 `STANDBY` / `STUDY` / `BREAK` / `OFF` 状态。
- **三维正交观察模型**：严格解耦 `InteractionState`（交互）、`TaskRelation`（任务相关度）、`PrivacyState`（隐私状态）。
- **隐私门禁优先 (Privacy Gate First)**：在进行任何截图或 AI 分类前，本地规则首先进行敏感应用与敏感域名过滤。
- **规则优先，AI 兜底**：明确规则快速判定，模糊场景使用结构化 AI 分类模型并维护多维哈希缓存。
- **低干扰桌宠交互**：纯 PyQt6 UI Shell，置顶透明窗口、动画正负反馈、气泡提醒与用户反馈闭环。
- **持久数据安全**：CGO-free SQLite 驱动，部署与更新严禁破坏用户配置与历史数据。

详细架构设计与开发文档请参考 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) 与 [DEVELOPMENT.md](DEVELOPMENT.md)。
