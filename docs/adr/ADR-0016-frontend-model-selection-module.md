# ADR-0016: 前端模型选择独立成 `useModelSelection`，值改为 (服务商, 模型) 二元组

## 状态

已接受（2026-10-26）· 落实 [ADR-0015](ADR-0015-active-provider-and-model-identity.md) 的前端部分

## 背景

ADR-0015 定了"激活的服务商决定一切"，落到前端有三个具体问题：

1. **模型的值不够表达身份**。`useChat.model` 是 `ref<string>`，值就是模型名。同名模型（中转站与官方都有 `deepseek-chat`）在前端是同一个值——`ModelPicker` 的 `aria-selected` 用 `m.id === modelValue` 比较，同名时**两行同时高亮**；请求体也只能带模型名，后端无法知道要哪个服务商。
2. **"选中的模型"这件事被塞在 `useChat` 里**。`useChat` 有 8 个方法（send / retry / edit / stop / refresh / …），每个都要带模型，于是 `model` 只能是它内部的 `ref`。现在多了一条规则——"激活的服务商一变，当前模型可能失效，要回落到新服务商的默认模型"——这条规则需要读 `providers.activeProvider`。如果放进 `useChat`，`useChat` 就要认识"服务商"，**它本来只认识"消息与流"**。
3. **回落痕迹是 UI 状态，不是消息**。它要在消息区显示一条"服务商换成 X，模型跟着换成 Y"，但不该落库。

不决策的代价：要么把服务商概念塞进 `useChat`（一个模块两个职责，测试要同时造假上游和假服务商），要么在 `ChatArea` 里散着写 watch（规则重复、没人测）。

## 决策

**三件事一起定：**

1. **新增深模块 `composables/useModelSelection.ts`**，接口只有 6 个成员：

   ```ts
   useModelSelection(activeProvider: ComputedRef<Provider | null>) → {
     selected: Ref<SelectedModel | null>   // { serviceProvider, model }
     models:   ComputedRef<ProviderModel[]>// 激活服务商的模型
     label:    ComputedRef<string>          // 按钮与落款行用
     fallback: Ref<{ from: string; to: string } | null>
     select(m: SelectedModel): void
     clearFallback(): void
   }
   ```

   它**独吞**四条规则：初值取激活服务商的 `model` 字段、激活变化时判断"当前模型还在不在新清单里"、不在就回落 + 记 `fallback`、无激活时 `selected === null`。

2. **模型值改为 `SelectedModel = { serviceProvider: string; model: string }`**（不是复合字符串）。`POST …/messages` 请求体带 `{serviceProvider, model}` 两个字段。

3. **`useChat` 不再持有模型状态**：签名从 `model` / `setModel` 改成 `deps.getModel?: () => SelectedModel | null`，每次请求前调一次。`ChatArea` 把 `() => selection.selected.value` 注入进去。

**回落痕迹的生命周期**：`ChatArea` 在下次 `send` 时 `clearFallback()` —— 只留到下一轮，不堆积、不落库。

## 权衡

| 备选 | 代价 | 为什么没选 |
|------|------|------------|
| 保持 `model: ref<string>`，后端按模型名反查服务商 | 前端零改动 | 同名模型**无解**（ADR-0015 的 Bug 2 原样保留）；且把歧义从接口层藏到后端实现层 |
| **复合字符串** `serviceProvider::model` | 前端改动更小（值仍是 `string`，`ref`/`props` 不用改类型） | 解析脆弱：模型名本身可能带 `/`（OpenRouter `anthropic/claude-3.5-sonnet`）；这个值还要进请求体、日志、`aria-label`，处处转义。二元组是"类型即文档" |
| 回落规则写在 `ChatArea` 的 `watch` 里 | 少一个文件 | 规则会散在组件里、**没法单测**（要 mount 整个聊天区 + 造假服务商）；而它恰恰是最容易写错的一条（"在不在新清单里"的判定） |
| 回落规则放进 `useChat` | 模块少一个 | `useChat` 要认识 `providers`，"消息与流"和"服务商配置"两个变化原因耦合；测试 `useChat` 就得连带造服务商数据 |
| 把 `selected` 提升到 `App` | 全局一份，跨会话共享 | 它只有一个消费方（`ChatArea` 子树），提升后要往下传两层，还让"聊天内部状态"变成应用级状态。**`providers` 已经 `provide` 下去了**，`ChatArea` 直接读同一个实例即可，不需要事件广播 |
| `fallback` 落库成一条 system 消息 | 刷新后痕迹还在 | 为一条 UI 提示加一张表的写入路径，且会让"消息数"虚高、导出 Markdown 里多出无意义内容 |
| 模型选择持久化到会话（刷新后还记得） | 更符合 #9 字面 | 会与"激活服务商"形成**两个互相打架的状态源**（激活变了、会话里记的模型可能已失效）。本轮不做，记进 `02-前端设计.md` §9 待确认 |

## 后果

- **正面**：
  - 回落规则**只有一处**、可单测（`useModelSelection` 的 7 条用例，含"同名模型切换后 `selected.serviceProvider` 跟着变"）。
  - `useChat` 保持单一职责（消息与流），它的测试不需要任何服务商数据。
  - `ModelPicker` 的 `aria-selected` 改成按 `{serviceProvider, model}` 比较，**同名双高亮的 bug 顺带消失**。
  - `useProviders.models` 只取激活服务商后，`ModelPicker` 里"跨服务商同名"这个场景从数据结构上不存在了——不需要去重、不需要分组。
- **负面 / 代价**：
  1. **`useChat` 的签名变了**：`model` / `setModel` 移除，改 `deps.getModel`。已有 8 个用例与 `ChatArea` 的 `setModel` 调用要改（`ChatArea` 里那个 `watch(() => props.defaultModel, ...)` 直接删掉——默认值现在由 `useModelSelection` 负责）。
  2. **`ChatArea` 多了一个 inject 依赖**（`providers`），它原来只收 props。这是"读同一个 `useProviders` 实例"的代价，换来的是激活变化能自动触发回落，不需要事件广播。
  3. **模型选择仍不持久化**（刷新回默认），与 #9 字面存在偏差——已在文档记明，不假装满足。
  4. 回落痕迹不落库，所以**刷新后就没了**；如果用户刷新前没注意到，就不知道模型被换过。可接受（换模型痕迹本来就是"当场提示"性质）。
- **2026-10-26 修正（执行任务 17 时发现）**：`selected` 的归属从 **ChatArea** 改为 **App**。
  原设计说"它只有一个消费方，放 `ChatArea` 即可"，但忽略了一件事实：**切到服务商设置页时 `ChatArea` 会被 `v-if` 卸载**，而"去设置页激活服务商 → 回聊天页"正是这条功能的主路径。状态放在被卸载的组件里，等于每次切激活都重新初始化：回落永远不会发生、痕迹也永远不会出现（实测：回聊天页后模型静默变成新服务商的默认模型）。
  改为 `App` 创建 `useModelSelection` 并 `provide('modelSelection', …)` 后，watch 在 pane 切换期间持续存活，回落与痕迹都正常（已端到端实测）。
  **教训**：判断状态归属时，"几个消费方"不够，还要问"**持有者会不会被卸载**"——`v-if` 切换的主区视图之间共享的状态，必须活在两者的共同祖先里。
- **可逆性**：中。把 `SelectedModel` 退回 `string` 容易（改类型 + 请求体去掉 `serviceProvider`），但会让 ADR-0015 的 Bug 1/2 复现，实际不会退。`useModelSelection` 的规则可以原地演进（比如加持久化），不影响其他模块。
- **验证方式**：
  - `useModelSelection` 7 条单测（初值 / select / 回落 / 不回落 / 无激活 / clearFallback / 同名模型带服务商）。
  - `useProviders` 6 条（只含激活服务商 / 无激活为空 / activate 替换 / activate 失败不变 / 删除激活者 → null / test 不弹 toast）。
  - `ProviderRow` 7 条（含"点圆点 emit activate 且不触发 edit"）。
  - 端到端：真浏览器点圆点 → 模型按钮换源 → 发一条，断言假上游收到的 `base_url` / `api_key` 是**激活服务商**的（这正是 ADR-0015 Bug 1 的回归测试）。
