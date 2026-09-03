# ChatGPT Collector 基线

`browser/chatgpt-collector` 是 Manifest V3 DOM-only 采集器。Content Script
只读取 `chatgpt.com` 当前页面，在 attach 时建立内存 baseline，不自动导入
已有历史消息；Background Service Worker 才能读取 `chrome.storage.local` 中
的 scoped collector token，并向 `127.0.0.1:17321/v1/collector/*` 发请求。

Collector token 与 Supervisor 主 token 分离。Supervisor 根据 Turn 开始时的
`mode_at_start` 冻结 `eligible_for_review`，并使用 `observed_at` 计算本地日期。
离线消息进入有界 FIFO 队列（1000 条或 10 MiB），优先丢弃已完成的 Assistant
payload，保留用户问题。

当前 POC 不依赖 ChatGPT 私有 API、cookie、history、webRequest 或 remote code。
