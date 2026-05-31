# 产品经理 AI 工作台

基于 `Go + Wails + React` 的本地优先产品经理全链路工具。当前 MVP 已经打通这条闭环：

- 输入客户的模糊需求
- 自动生成澄清问题和需求理解摘要
- 生成 `PRD`、`功能规格说明`、`研发实现说明`
- 根据 `UISchema` 预览原型
- 导出 React 原型脚手架到本地目录

## 技术栈

- 桌面壳：Wails 2
- 后端：Go + SQLite
- 前端：React + TypeScript + React Router + Zustand + TipTap + Tailwind CSS
- AI 接入：OpenAI-compatible API，未配置时自动回退 Mock provider
- Provider 设置支持在 app 内切换 `Responses API` 与 `Chat Completions API`

## 本地开发

```bash
wails dev
```

如果只想单独运行前端：

```bash
cd frontend
npm install --cache .npm-cache
npm run dev
```

## 运行测试

后端：

```bash
go test ./...
```

前端：

```bash
cd frontend
npm run test
```

更细的测试分层与步骤说明见：

`TESTING.md`

更完整的产品与研发蓝图见：

`docs/DEVELOPMENT_BLUEPRINT.md`

更偏系统设计与研发实施的说明见：

`docs/SYSTEM_DESIGN_SPEC.md`

## 构建

```bash
wails build -skipbindings
```

构建产物默认位于：

`build/bin/pm-ai-workbench.app`

## 当前实现重点

- 本地 SQLite 持久化项目、需求、文档版本、工作流记录和模型配置
- 文档版本状态流转：`draft / generated / approved / stale`
- 上游文档修改后自动标记下游文档为 `stale`
- `UISchema -> 应用内预览 -> React 原型导出` 的统一路径
- 支持 Mock 回退，便于无 API Key 时也能演示完整流程

## 后续建议

- 增加登录与云同步
- 引入团队协作、评论、审批流
- 支持行业模板与知识库
- 将导出的原型脚手架进一步贴近正式工程规范
