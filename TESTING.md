# 测试策略

本文档把当前 MVP 的测试分成 4 层。这里将你提到的“继承测试”按常见研发语境解释为“集成测试”。

## 1. 单元测试

目标是验证最小可判断单元的纯逻辑正确性，不依赖完整工作流。

- `extractJSONBlock`：验证 JSON、代码块 JSON、非法文本的解析行为。
- `normalizeAPIFormat`：验证 `responses`、`chat_completions` 及别名归一化。
- `extractResponsesText`：验证 Responses API 嵌套 `output[].content[]` 的文本提取。
- `UISchema.Validate`：验证 route/page 关联约束。
- 前端 `artifact` helper：验证文档 JSON 解析与 diff 结果。

## 2. 集成测试

目标是验证多个模块协同时是否按预期工作。

### Provider 集成

- `chat/completions`：验证请求发送到正确 endpoint，模型名与消息体正确透传，响应内容能被提取。
- `responses`：验证请求发送到 `/responses`，`instructions` 和 `input` 结构正确，响应文本能被提取。

### 工作流集成

- 从模糊需求输入开始，依次生成澄清问题、PRD、功能规格、研发说明、UISchema、原型导出。
- 校验导出的 React 脚手架文件是否真的落地到本地目录。

## 3. 灰盒测试

目标是通过公开业务入口触发流程，同时检查内部状态是否发生了正确变化。

- 手动编辑 PRD 某章节后，功能规格/研发说明/原型版本应被标记为 `stale`。
- 保存 Provider 配置后，数据库里必须保存对应 `apiFormat`，不能只存在前端内存状态。
- 生成链路过程中 `workflow_runs` 要保留运行记录，便于后续扩展审计与调试。

## 4. 黑盒测试

目标是只通过对外暴露的 App 方法验证系统可用性，不读取内部 service 细节。

- 使用 `App.CreateProject`、`App.SaveRequirementInput`、`App.GenerateClarifications`、`App.GenerateArtifact`、`App.GenerateUISchema`、`App.ExportPrototype` 跑通闭环。
- 使用 `App.SaveProviderConfig` 验证 UI 层保存的 `apiFormat` 能驱动后端配置持久化。

## 5. 前端黑盒/UI 测试

目标是从用户可见交互出发，验证关键动作不会失效。

- Provider 设置表单可以切换 `Responses API` 与 `Chat Completions API` 并提交。
- 需求输入面板可以录入原始需求并触发保存动作。
- 原型画布可以正确渲染 dashboard 页面和 mock 指标。

## 6. 每个步骤的验证重点

### 需求输入

- 空输入必须报错。
- 保存后项目工作区应能重新加载出该输入。

### 澄清问题生成

- 至少生成 3 个问题。
- 问题必须包含 `id/question/rationale/priority`。

### PRD 生成

- 必须是合法 `ArtifactDocument`。
- 至少包含一个 section，且每个 section 有 `id/title/body/html`。

### 功能规格 / 研发说明

- 必须依赖上游文档版本。
- 上游缺失时不能继续生成。

### UISchema / 原型

- 必须包含 `app_meta/routes/pages/components/forms/tables/actions/mock_data/state_variants/navigation_map`。
- 导出后至少包含 `src/App.tsx`、`src/main.tsx`、`src/schema.ts`、`src/styles.css`。

### Provider 兼容

- 支持 `chat/completions`。
- 支持 `responses`。
- 两种格式都应能通过 app 内设置切换。

## 7. 当前自动化命令

后端：

```bash
go test ./...
```

前端：

```bash
cd frontend
npm run test
npm run build
```

桌面打包：

```bash
wails build -skipbindings
```
