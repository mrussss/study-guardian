# StudyGuardian UI Design System

## 基线

产品 UI 使用 Windows Fluent-like 的清晰度、shadcn/Radix 的克制层级和
StudyGuardian 的柔和紫色 accent。核心正文不低于 12px；间距以 4px 为基准，
常用值为 `4 8 12 16 20 24 32 40`。

字体优先级：`Segoe UI Variable`、`Segoe UI`、`system-ui`、sans-serif。

## Token

实现位于 `pet-v3/src/shared/theme/tokens.css`，包含 System/Light/Dark 两套
颜色变量：

- Light：`#F6F7F9` background、`#FFFFFF` surface、`#665CF6` accent。
- Dark：`#0E1014` background、`#15181E` surface、`#8077FF` accent。
- semantic：success 用于健康/完成，warning 用于待处理，danger 仅用于真正
  的危险状态。
- elevation 只有 flat、card、floating 三层；不使用大面积模糊阴影。
- radius：control 8px、button 10px、card 14px、large panel 18px。

## 组件规则

- 每个页面只保留一个明确 primary action，secondary action 使用浅表面。
- 图标统一使用 Lucide；emoji 不作为主导航或主操作图标。
- loading、error、success、dirty 状态必须可读，技术错误只进入 Diagnostics。
- Motion 仅用于短促的进入、状态变化和成功反馈，并尊重 reduced-motion。
- Dashboard 使用一个 hero + supporting metrics，避免四个同权重巨型卡片。

## 验收口径

视觉验收检查层级、密度、对比度、焦点环、键盘可达性、Light/Dark/System 三种
主题以及 920×640 到大窗口的响应式布局。自动测试只能验证结构/纯函数，不能
替代最终 Windows 视觉验收。
