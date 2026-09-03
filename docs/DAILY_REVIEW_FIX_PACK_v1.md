# Daily Review Fix Pack v1.0 审计记录

本轮只处理 Collector Context freshness、MV3 Worker restart、Baseline/Dedupe
基础正确性和 Review Exclusion；AI Review、Conversation Compaction、Semantic
Snapshot 正式接入、自动触发和 UI 均未扩展。

已验证的代码路径：

- Context `<=5s` 直接使用，`>5s` 优先刷新，刷新失败且 `>15s` 时 fail closed；
- Content Script attach baseline 过滤旧消息，冻结 Turn Context 使用 `chrome.storage.session`；
- offline queue 保持 1000 条 / 10 MiB 上限，优先丢弃 finalized assistant；
- `chat_turn`、`chat_conversation`、`ALWAYS_EXCLUDE` 从 Evidence Bundle 过滤；
- exclusion 类型校验，已有 READY Review 变 STALE，重新生成 revision/input hash 更新；
- Collector token 不能访问主业务 API，主 token 不能访问 Collector API。

自动测试：

```text
go test ./...                         PASS
node --test tests/*.test.js          PASS（当前 14 个测试）
```

Windows 运行 smoke 已通过，Supervisor 与 Sensor 均正常。真实 Chrome + ChatGPT
最小 E2E 尚未通过：Computer Use 读取现有 ChatGPT 页面时返回 `Debugger unattached`；
本轮未安装扩展、未登录、未发送测试消息，因此不能把 Chrome 真机项标记为 PASS。
