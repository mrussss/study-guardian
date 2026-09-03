# Daily Review Collector Fix Pack v1.1 审计记录

本轮仅处理 Collector 第二轮的四个可靠性问题：

- `background.js` 通过 `buildTurnPayload()` 正确 `await tracker.contextFor()`，完整 payload 保留冻结的 `mode_at_start`、`task_at_start` 和 `eligible_for_review`；
- Parser 输出统一的 message canonical 状态，Content Script 以稳定 DOM identity 关联 Assistant streaming，并在内容连续稳定至少 1500ms 后标记 `is_final`；
- Content Script 以 `platform + external_conversation_id` 管理 Conversation epoch，SPA 从 A 切换到 B 时重新 baseline、清理 streaming/delivered 状态；
- 当前 Conversation 内以 Turn snapshot hash 抑制不变候选，允许 streaming 内容变化和 final 状态变化各自同步；离线时 current payload 只入队，不立即重复 flush。

本轮没有扩展 Semantic Snapshot、Conversation Compaction、AI Review、Validator、Sanitizer、自动 Review 或 Pet UI。

自动验证：

```text
go test ./...                         PASS
node --test tests/*.test.js          PASS（26/26）
git diff --check                      PASS
```

Windows runtime smoke：

```text
build-windows.sh                     PASS
deploy-windows.sh /mnt/d/StudyGuardianDev  PASS
GET /healthz                         PASS（status=ok，ActivityWatch/Screen Sensor=true）
GET /v1/status                       PASS（main token）
GET /v1/collector/context             PASS（collector token）
GET /v1/review/evidence               PASS
```

部署后的扩展目录已包含 `collector.js`、`stream_state.js`、`conversation_epoch.js`、
`candidate_dedupe.js` 和更新后的 `content.js` / `background.js`。

已加入脱敏 fixtures：

```text
browser/chatgpt-collector/tests/fixtures/chatgpt/
```

真实 Windows Chrome + ChatGPT E2E 仍未能标记 PASS。此前 Computer Use 读取现有 Chrome 页面时返回 `Debugger unattached`；本轮没有绕过该限制安装扩展、登录 ChatGPT 或发送测试消息。因此 Task 89 保持未完成，Task 86 仍只标记为部分完成。
