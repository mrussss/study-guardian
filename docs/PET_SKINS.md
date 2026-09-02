# Pet 皮肤系统

内置皮肤位于 `pet/assets/skins/<id>/`，用户皮肤位于运行目录 `D:\StudyGuardianDev\config\pet-skins\<id>\`。每个目录必须包含 `manifest.json` 和 `states.idle` 指向的图片；其他状态缺失时自动回退到 idle，非法皮肤被记录并忽略。

最小 manifest：

```json
{
  "schema_version": 1,
  "id": "my-skin",
  "name": "My Skin",
  "license": "CC BY 4.0",
  "frame_size": 64,
  "display_size": 128,
  "fps": 7,
  "pixel_art": true,
  "states": {"idle": "sprites/idle.png"}
}
```

支持状态：`idle`、`study`、`distracted`、`rest`、`talk`、`celebrate`。像素皮肤使用 nearest/fast scaling，不拉伸到非正方形；偏好保存到 `config/pet.json`，写入采用临时文件替换。

本仓库的 `studyguardian-pixel` 与 `builtin-minimal` 是 Phase 6 的原创占位素材，生成工具为 OpenAI ImageGen，生成日期为 2026-09-02，按项目文件中的 Original placeholder 声明使用；最终视觉资产仍标记为 `Final Visual Asset Pending`。发布第三方皮肤时必须在 manifest 中注明作者、来源和许可证；不明确授权的素材不能随仓库发布。

`config/pet.json` 是 Pet UI preference 的唯一真源，保存 `skin` 和 `last_event_id`。Motivation 事件由 Supervisor 写入 `ui_events`，Pet 按 ID 顺序消费重要事件、更新 cursor 后再持久化，避免重启重复展示或丢失。`config/pet-skins` 是用户资产目录，Deploy 只替换 built-in skin，不会删除用户 skin。
