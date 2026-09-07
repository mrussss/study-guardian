# AI Provider V2

StudyGuardian 的 AI 只作为本地规则之后的兜底分类器。默认关闭 AI，默认 provider 为 `none`；规则引擎、隐私门禁和 ActivityWatch/Sensor fail-soft 行为不依赖 AI。

## 配置

`config/config.yaml` 使用 `ai.schema_version: 2`，文本与视觉端点分开：

```yaml
ai:
  schema_version: 2
  enabled: true
  use_vision_only_when_needed: true
  proxy:
    mode: environment # environment | direct | manual
    url: ""
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

内置 profile：`aihubmix`、`openai`、`openai-compatible`、`deepseek`、`qwen`、`kimi`、`zhipu`、`siliconflow`、`doubao`、`ollama`、`none`。AIHubMix 使用 OpenAI 兼容地址 `https://aihubmix.com/v1` 和环境变量 `AIHUBMIX_API_KEY`；Qwen 开箱默认使用共享地址 `https://dashscope.aliyuncs.com/compatible-mode/v1`，密钥环境变量为 `DASHSCOPE_API_KEY`。密钥解析顺序是 endpoint 的 `api_key_env`、profile 默认环境变量、`api_key_file`；日志和错误不会输出密钥。

每个文本或视觉端点都可以配置一个主模型和最多三个备用模型。`model` 保持为主模型字段，`fallback_models` 是有序列表，因此旧的单模型配置无需迁移。默认文字模型是 `coding-glm-5.3-free` 且默认备用列表为空；付费 `coding-glm-5.3` 只能由用户明确填写。视觉默认模型是 `ox-alpha`、默认备用列表为空，不会自动启用视觉或自动加入 `glm-5.3-flash`：

```yaml
ai:
  enabled: true
  text:
    enabled: true
    provider: aihubmix
    model: coding-glm-5.3-free
    fallback_models: []
    base_url: https://aihubmix.com/v1
    timeout_seconds: 20
    json_mode: auto
  vision:
    enabled: false
    provider: aihubmix
    model: ox-alpha
    fallback_models: []
    base_url: https://aihubmix.com/v1
    timeout_seconds: 20
    json_mode: auto
```

Control Center 选择“AIHubMix 中转”时会填入上述当前推荐模型，但仍由用户保存并配置 Key 后才生效。模型 ID 以带当前 Key 请求 `GET /v1/models` 返回的规范名称为准；旧别名即使暂时可用也不写入默认配置。中转链路通常超过 6 秒，因此 AIHubMix 默认单模型超时为 20 秒。备用模型可能计费；列表为空时永远不会自动切换到第二个模型。视觉仍默认关闭。

## 全局代理

`ai.proxy` 由文字分类、视觉分类和 Daily Review 共同使用：

- `environment`（默认）读取 Supervisor 进程的 `HTTP_PROXY`/`HTTPS_PROXY`，不读取 Windows 系统代理设置。
- `direct` 强制直连。
- `manual` 只接受不含账号密码、路径、查询参数或片段的 `http://`/`https://` 主机地址。

代理 transport 从 Go 标准 transport 克隆创建，不修改 `http.DefaultTransport`。Control Center 的“测试网络”会先保存当前草稿，再通过同一策略请求文字端点的 `/models`；任何合法 HTTP 响应都只证明网络可达，不会把响应 body 返回 UI。

文本和视觉 provider 是两个独立实例。视觉分类只有在配置了 `vision.enabled`、视觉 provider 和模型，并且调用方提供经过隐私门禁处理的图片时才启用；不能仅凭 provider 名称推断支持视觉。`temperature` 是可选指针：默认请求完全省略该字段，只有用户明确配置时才发送。

Windows 上可运行 `scripts/configure-ai.ps1`。脚本先生成带时间戳的备份，再通过 `bin/config-helper.exe` 更新 YAML；不使用字符串替换，也不会覆盖无关配置。`scripts/migrate-config.ps1` 只负责把旧版扁平字段映射为 V2。

## JSON 与退避

`json_mode: auto` 仅对声明支持 JSON mode 的 profile 发送 `response_format=json_object`。若服务明确以 HTTP 400/422 表示不支持，当前模型最多降级重试一次。模型链只在明确属于当前模型的 `model_not_found`、`model_unavailable`、`no_available_channel`、模型限流或无效输出时前进。裸 HTTP 429 不等于模型失败；账号/配额限流、鉴权失败、DNS/TCP、代理拒绝、TLS、取消和未明确归属的超时都会停止同一 provider 的模型链，避免付费备用模型掩盖根因。实时分类链中的每个模型保有独立 cooldown，整条链失败后才回退本地规则。

Daily Review 在 `inherit_text_profile: true` 时继承完整文字模型链，并保存真正成功的模型名。所有模型都失败后仍使用 deterministic fallback，不影响复盘可用性。

视觉请求只在文本分类结果仍为 `UNKNOWN` 或低于最小置信度、且调用方提供经过隐私门禁和缩放的 `analysis_image_base64` 时发送；敏感应用/域名不会进入视觉请求。实际流程是 `Rules -> Text AI -> Vision AI fallback`，而不是“只要有截图就直接走 Vision”。各端点使用自己的 timeout；Control Center 的 AIHubMix 推荐配置为每个模型 20 秒，不再由 Classifier 统一压成 3 秒。

开发测试若使用 `fake`，必须同时设置 `ai.developer_mode: true`；生产配置中 fake 会被强制关闭。

## Control Center 设置与 Secret

现代 Control Center 通过 `/v1/settings/ai` 保存文本和视觉端点，并在保存后立即重建运行时 provider。`GET /v1/settings/ai` 只返回脱敏配置和 `api_key_configured`，不会返回 key、secret 文件名或绝对路径。

密钥使用 `/v1/settings/ai/secret` 单独写入或删除。Supervisor 在 `config/secrets` 中原子替换密钥文件；React 输入框不回显已经保存的值。`POST /v1/settings/ai/test` 会向选定 provider 发出最小结构化请求，并只返回 provider、实际成功的 model、延迟和有限错误种类。`POST /v1/settings/ai/proxy/test` 使用已保存代理策略请求 `/models`，只返回 `ok`、模式、延迟和有限错误种类。HTTP 429 只有带明确模型错误码时才允许模型 fallback；裸 429 会归类为账号限流。没有真实凭据时不得把模型连接测试标记为 PASS。
