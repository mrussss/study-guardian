# StudyGuardian 隐私与数据安全说明

## 1. 核心隐私保护原则

1. **绝对禁止键盘记录器 (No Keylogger)**：
   - 系统仅通过 ActivityWatch 获取是否有键鼠输入（AFK 状态与活跃时长），绝不记录、捕获或存储用户输入的具体字符或击键内容。

2. **截图默认不永久保存 (Ephemeral Screenshots)**：
   - Screen Sensor 仅在用户 AFK 期间按需截取单帧用于计算哈希差异 (dHash)，比对完成后即刻销毁，不写入磁盘。

3. **本地隐私门禁 (Privacy Gate)**：
   - 系统内置与支持用户自定义敏感应用清单（密码管理器、银行客户端等）及敏感域名清单。
   - 当当前活动窗口命中敏感规则时，系统判定 `PrivacyState = SENSITIVE`，在此状态下：
     - 禁止向 Screen Sensor 请求捕获图像；
     - 禁止将截图或窗口敏感文本发送至任何 AI 模型；
     - 日志与数据库中对敏感信息进行过滤脱敏。

4. **Localhost 通信安全与 Token 认证**：
   - Supervisor 与 Screen Sensor 均仅绑定监听 `127.0.0.1` 本地回环网络，禁止对公网暴露。
   - 内部 API 访问采用运行时生成的 Bearer Token 认证，Token 仅保存在本地 `D:\StudyGuardianDev\config\auth.token`。
   - 日志输出中严格脱敏 Authorization 请求头。
