# Open Source Dependencies & Upstream Records

| 项目 | 许可证 | 使用方式 | 是否修改 | 上游仓库 / 来源 | 资产许可证 |
|---|---|---|---|---|---|
| `ActivityWatch` | MPL-2.0 | 外部进程 / REST API | 否 | `https://github.com/ActivityWatch/activitywatch` | MPL-2.0 |
| `desktop-pet` | MIT | PyQt6 UI Shell 基础改造 | 是（剥离独立业务逻辑，保留透明置顶与动画） | `https://github.com/UIU-Developers-Hub/desktop-pet` | MIT / 示例资产 |
| `python-mss` | MIT | 屏幕采集底层 | 否（作为依赖包调用） | `https://github.com/BoboTiG/python-mss` | MIT |
| `modernc.org/sqlite` | BSD-3-Clause | Go SQLite Driver (CGO-free) | 否（作为 Go 依赖包） | `https://gitlab.com/cznic/sqlite` | BSD-3-Clause |
| `gopkg.in/yaml.v3` | MIT / Apache-2.0 | YAML 配置文件解析 | 否 | `https://github.com/go-yaml/yaml` | MIT / Apache-2.0 |
| `PyQt6` | GPL-3.0 / Commercial | Python GUI 库 | 否（作为环境依赖） | Riverbank Computing | GPL-3.0 |
| `requests` | Apache-2.0 | Python HTTP 请求库 | 否 | `https://github.com/psf/requests` | Apache-2.0 |

## 本仓库原创素材与发布边界

`pet/assets/skins/studyguardian-pixel` 和 `builtin-minimal` 是本项目 Phase 6 的原创占位像素素材，使用 OpenAI ImageGen 生成于 2026-09-02，manifest 中保留了用途声明。它们不继承上表 `desktop-pet` 的示例资产授权；发布时应保留对应 manifest 和本节说明。

第三方皮肤不是本项目代码许可证的自动组成部分。用户皮肤必须自行提供作者、来源、许可证和允许再分发的证据；来源或许可证不明确的图片只可本地使用，不得打包进公开发行物。

PyQt6 使用 GPL-3.0 / Commercial 双许可证。个人自用和 GPL 发行路径可以使用 GPL 版本；若进行闭源商业分发，需要在发布前完成 Riverbank 的商业许可证核查。
