# Daily Review Collector Fix Pack v1.2.1 审计记录

本轮只收尾 Offline Queue FIFO 一致性和开发说明：

- `queue.js` 新增 `peekQueue()` / `ackQueueHead()`；flush 发送前只 peek，只有服务端 POST 成功且队首仍是同一个 immutable payload 时才删除；
- POST 失败时失败 payload 留在原队首，不会移动到队尾；队列 mutation 使用轻量串行保护，避免 ack 覆盖并发 enqueue；
- `flushQueuedPayloads()` 在旧队列 flush 失败时停止，current payload 只追加到队尾，不会立即额外 POST；
- Frozen Context、`observed_at` 和 cross-midnight payload 在 queue flush 中不重新计算；
- README 已明确区分源码目录开发流程和 Windows artifact 的 Load unpacked 流程，并说明必须先生成 `dist/content.js`；
- Task 86、Task 89、Task 92 状态保持 `[~]`、`[ ]`、`[ ]`。

自动验证：

```text
go test ./...                         PASS
node --test tests/*.test.js          PASS（37/37）
git diff --check                      PASS
```

新增真实 queue 层测试覆盖：失败原队首保序、current 追加、成功逐条 ack、immutable
payload 校验和最终 final snapshot 状态。HTML fixture、bundle smoke 和既有 Collector
回归继续通过。

Windows 验证：

```text
scripts/build-windows.sh              PASS
scripts/deploy-windows.sh /mnt/d/StudyGuardianDev  PASS
GET /healthz                          PASS
artifact manifest/bundle smoke        PASS
```

部署 artifact 继续满足：manifest 加载 `dist/content.js`、bundle 无顶层 static import、
Background 使用 `src/background.js` module，且运行目录没有测试用 `node_modules`。

真实 Windows Chrome + ChatGPT E2E 仍未完成。Computer Use 仍返回
`Debugger unattached`；没有绕过登录或伪造 PASS，因此 Task 89 保持 `[ ]`。
