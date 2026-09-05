# StudyGuardian

AI 学习监督系统（StudyGuardian）是一个个人自用、长期后台运行、低打扰的 AI 学习监督助手。

## 核心设计特性

- **手动意图与系统观察分离**：用户主动声明 `STANDBY` / `STUDY` / `BREAK` / `OFF` 状态。
- **三维正交观察模型**：严格解耦 `InteractionState`（交互）、`TaskRelation`（任务相关度）、`PrivacyState`（隐私状态）。
- **隐私门禁优先 (Privacy Gate First)**：在进行任何截图或 AI 分类前，本地规则首先进行敏感应用与敏感域名过滤。
- **规则优先，AI 兜底**：明确规则快速判定，模糊场景使用结构化 AI 分类模型并维护多维哈希缓存。
- **低干扰桌宠交互**：PyQt6 是当前默认 Pet；Tauri v3 提供已打包的现代 Quick Panel / Control Center，并在人工输入 Gate 通过前作为候选 UI。
- **持久数据安全**：CGO-free SQLite 驱动，部署与更新严禁破坏用户配置与历史数据。
- **稳定 Windows 入口**：桌面快捷方式和可选开机启动都调用自定位 Launcher；重复打开只激活已有 Control Center。

详细架构设计与开发文档请参考 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)、[docs/WINDOWS_RUNTIME.md](docs/WINDOWS_RUNTIME.md)、[集成交付报告](docs/INTEGRATED_PRODUCT_COMPLETION_REPORT.md) 与 [DEVELOPMENT.md](DEVELOPMENT.md)。
