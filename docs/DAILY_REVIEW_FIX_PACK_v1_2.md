# Daily Review Collector Fix Pack v1.2 审计记录

本轮只处理 Collector 的浏览器打包、离线队列顺序和 fixture 回归：

- `manifest.json` 不再直接加载带 static import 的 `src/content.js`，改为加载 `dist/content.js`；
- `scripts/bundle-content.py` 已进入 `scripts/build-windows.sh`，从模块化 `src/` 生成 classic Content Script；构建和部署都会拒绝含顶层 static import 的 bundle，并排除测试用 `node_modules`；
- Background 保持 `src/background.js` + `type: module`；
- Offline Queue 先按旧到新 flush，旧队列 flush 失败时当前 payload 只入队，不立即额外 POST；
- `conversation-a`、streaming 1/2、final fixture 已通过 `linkedom` 驱动 `parseConversation()` 和 streaming state 回归；
- v1.1 的 Node 数量记录已修正为 26/26，本轮最终数量以实际命令输出为准。

本轮没有扩展 Task 88、Task 92-95、Task 97、Task 99、Task 100、Git Evidence，
也没有实现 Regenerate/Edit/复杂 Branch。

自动验证：

```text
go test ./...                         PASS
node --test tests/*.test.js          PASS（33/33）
git diff --check                      PASS
```

Windows 验证：

```text
scripts/build-windows.sh              PASS
scripts/deploy-windows.sh /mnt/d/StudyGuardianDev  PASS
GET /healthz                          PASS
artifact manifest/bundle smoke        PASS
```

部署目录中的 `manifest.json` 指向 `dist/content.js`，bundle 存在且不含顶层
static import，Background module 仍位于 `src/background.js`，测试依赖没有进入
运行 artifact。

真实 Windows Chrome + ChatGPT E2E 仍未完成。Computer Use 仍受 `Debugger unattached`
阻塞，未绕过登录、未伪造 PASS；因此 Task 89 保持 `[ ]`，Task 86 保持 `[~]`。
