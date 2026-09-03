# Daily Review Collector Fix Pack v1.2.2 审计记录

本轮只收尾 Collector 全局 Delivery Pipeline 的顺序一致性：

- 新增 `DeliverySerializer`，Background 中所有 `sendTurn(candidate)` 都进入同一个全局 Promise chain；
- 串行范围覆盖 payload 构造、旧 Queue flush、current POST、失败 enqueue，而不是只锁 HTTP POST；
- serializer 的 rejected operation 会被隔离，后续 operation 仍能继续执行；
- v1.2.1 的 Queue `peek → POST → ack`、失败原队首保留和 mutation lock 保持不变；
- Frozen Context、`observed_at`、candidate dedupe、Assistant canonicalization 均未改变。

本轮没有扩展 Task 88、Task 92-95、Task 97、Task 99、Task 100、Git Evidence，
也没有实现 Regenerate/Edit/复杂 Branch。

自动验证：

```text
go test ./...                         PASS
node --test tests/*.test.js          PASS（41/41）
git diff --check                      PASS
```

新增并发测试覆盖：

- slow `H` / fast `He` 不允许新 snapshot 超车；
- `H` / `He` 同时 offline 时 enqueue 顺序保持不变；
- 第一个 operation 失败后第二个仍执行；
- `H`、`He`、`Hello final` 最终按顺序到达 mock server，最终状态为 `Hello / is_final=true`。

Windows 验证：

```text
scripts/build-windows.sh              PASS
scripts/deploy-windows.sh /mnt/d/StudyGuardianDev  PASS
GET /healthz                          PASS
artifact manifest/bundle smoke        PASS
```

真实 Windows Chrome + ChatGPT E2E 仍未完成。Computer Use 仍受
`Debugger unattached` 阻塞，没有绕过登录或伪造 PASS；因此 Task 89 保持 `[ ]`，
Task 86 保持 `[~]`，Task 92 保持 `[ ]`。
