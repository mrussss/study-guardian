# AI Provider V2

StudyGuardian 的 AI 只作为本地规则之后的兜底分类器。默认关闭 AI，默认 provider 为 `none`；规则引擎、隐私门禁和 ActivityWatch/Sensor fail-soft 行为不依赖 AI。

## 配置

`config/config.yaml` 使用 `ai.schema_version: 2`，文本与视觉端点分开：

```yaml
ai:
  schema_version: 2
  enabled: true
  use_vision_only_when_needed: true
  text:
    provider: deepseek
    model: deepseek-chat
    api_key_env: DEEPSEEK_API_KEY
    timeout_seconds: 6
    json_mode: auto
    temperature: null
  vision:
    enabled: false
    provider: none
```

内置 profile：`openai`、`openai-compatible`、`deepseek`、`qwen`、`kimi`、`zhipu`、`siliconflow`、`doubao`、`ollama`、`none`。Qwen 开箱默认使用共享地址 `https://dashscope.aliyuncs.com/compatible-mode/v1`，密钥环境变量为 `DASHSCOPE_API_KEY`。密钥解析顺序是 endpoint 的 `api_key_env`、profile 默认环境变量、`api_key_file`；日志和错误不会输出密钥。

文本和视觉 provider 是两个独立实例。视觉分类只有在配置了 `vision.enabled`、视觉 provider 和模型，并且调用方提供经过隐私门禁处理的图片时才启用；不能仅凭 provider 名称推断支持视觉。`temperature` 是可选指针：默认请求完全省略该字段，只有用户明确配置时才发送。

Windows 上可运行 `scripts/configure-ai.ps1`。脚本先生成带时间戳的备份，再通过 `bin/config-helper.exe` 更新 YAML；不使用字符串替换，也不会覆盖无关配置。`scripts/migrate-config.ps1` 只负责把旧版扁平字段映射为 V2。

## JSON 与退避

`json_mode: auto` 仅对声明支持 JSON mode 的 profile 发送 `response_format=json_object`。若服务明确以 HTTP 400/422 表示不支持，最多降级重试一次；401/403、429、5xx 和超时不会盲目重试，并进入短暂 cooldown。非法结构化返回会回退到本地规则。

视觉请求只在调用方提供经过隐私门禁和缩放的 `analysis_image_base64` 时发送；敏感应用/域名不会进入视觉请求。

开发测试若使用 `fake`，必须同时设置 `ai.developer_mode: true`；生产配置中 fake 会被强制关闭。
