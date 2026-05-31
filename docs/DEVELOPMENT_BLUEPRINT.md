# 产品经理全能工具开发蓝图

## 1. 文档目标

这份文档用于把当前的 MVP 从“文档串行生成器”升级为“完整的产品经理工作台”。目标不是单纯让 AI 多写几份文档，而是建立一套稳定的需求真相层，让以下产物都从同一份结构化语义中生长：

- 需求澄清记录
- 需求共识稿
- PRD
- 功能规格说明
- 研发实现说明
- 原型与 `UISchema`
- 任务拆解、测试建议和后续版本追踪

本蓝图默认技术栈仍然是：

- 后端：`Go + SQLite`
- 桌面壳：`Wails`
- 前端：`React + TypeScript`
- AI 接入：`OpenAI-compatible API`

## 2. 产品定位

### 2.1 产品定义

产品定位不是“PRD 生成器”，而是“本地优先的产品经理工作台”。它应覆盖产品经理从需求输入、澄清、文档生产、原型、研发协同到验收准备的完整链路。

### 2.2 北极星目标

系统最终要帮助产品经理完成四件事：

1. 把模糊需求收敛成可共识的问题定义。
2. 把问题定义沉淀为结构化、可编辑、可追溯的产品资产。
3. 把产品资产向下游研发和设计转换为可执行材料。
4. 把每次变更的影响范围、风险和重生成关系管理好。

### 2.3 设计原则

1. 先结构化，再成文档。
2. 先人机共识，再自动生成。
3. 每一层产物都可人工编辑、可版本化、可局部重生成。
4. 每个下游产物都必须能追溯上游来源。
5. 文本只是视图，结构化数据才是真相。

## 3. 核心方法论：需求真相层

当前产品最需要补的不是更多 prompt，而是一层稳定的“需求真相层”。

### 3.1 什么是需求真相层

需求真相层是统一的结构化领域对象，用来存储：

- 问题定义
- 用户角色
- 业务目标
- 核心场景
- 功能边界
- 页面与模块
- 字段与规则
- 权限与状态
- 验收标准
- 技术约束

后续所有内容都只是这层真相的不同投影：

- PRD = 业务视图
- 功能规格 = 交互和模块视图
- 技术实现说明 = 研发执行视图
- 原型 = 页面结构视图
- 任务拆解 = 执行计划视图

### 3.2 为什么必须先做这层

如果系统直接从自然语言跳到 PRD、再跳到功能规格、再跳到原型，就会出现：

- 上游说法与下游实现不一致
- 用户改了一个点，下游全部失真
- 原型只能凭感觉生成
- 导入外部 PRD 时无法统一处理
- 版本 diff 只能看文本，无法看语义变化

所以后续开发必须围绕“真相层”组织，而不是围绕 markdown 文档组织。

## 4. 完整产品信息架构

### 4.1 一级信息架构

建议桌面工作台一级导航拆成以下 8 个域：

1. 项目总览
2. 需求入口
3. 需求澄清
4. 需求共识
5. 产品文档
6. 原型与页面模型
7. 研发协同
8. 设置与资产

### 4.2 二级信息架构

#### 4.2.1 项目总览

- 项目基本信息
- 当前阶段进度
- 关键风险
- 最近变更
- 下游过期提醒
- 工作流时间线

#### 4.2.2 需求入口

- 输入模糊需求
- 上传已有 PRD
- 上传会议纪要
- 上传聊天记录
- 上传招标/需求说明材料
- 需求来源标签与版本

#### 4.2.3 需求澄清

- AI 逐题提问
- 历史回答
- 信息缺口地图
- 可跳过问题
- 建议答案
- 问题优先级

#### 4.2.4 需求共识

- 问题定义
- 目标用户
- 业务目标
- 关键流程
- 范围与边界
- 关键假设
- 风险与依赖
- 成功标准

#### 4.2.5 产品文档

- PRD
- 功能规格说明
- 研发实现说明
- 文档版本记录
- 文档对比
- 审阅状态

#### 4.2.6 原型与页面模型

- 页面清单
- 页面结构模型
- 组件树
- 状态与异常态
- 原型预览
- 原型导出

#### 4.2.7 研发协同

- 用户故事
- 开发任务
- 接口清单
- 测试建议
- 埋点建议
- 上线检查项

#### 4.2.8 设置与资产

- Provider 设置
- 模板库
- 行业术语配置
- 文档导入导出
- 项目归档
- 工作流日志

### 4.3 项目对象关系

建议核心对象关系如下：

- `Project`
- `SourceMaterial`
- `RequirementIntake`
- `ClarificationSession`
- `ConsensusBrief`
- `RequirementModel`
- `Artifact`
- `ArtifactVersion`
- `PageModel`
- `UISchema`
- `PrototypeBundle`
- `TaskBundle`
- `TestBundle`

其中：

- `RequirementModel` 是真相层核心对象。
- `ArtifactVersion` 是任何文本产物的版本快照。
- `PageModel` 比 `UISchema` 更靠近业务页面规划。
- `UISchema` 是原型渲染层的结构对象。

## 5. 每个页面应该长什么样

整体界面建议采用三栏结构：

- 左栏：项目与阶段导航
- 中栏：当前阶段主工作区
- 右栏：AI 助手、质检、影响分析、操作建议

### 5.1 项目总览页

目标：让用户一眼知道当前项目在哪个阶段，哪些内容已经确认，哪里还缺。

页面结构：

- 顶部：项目名称、标签、阶段状态、最近更新时间
- 中部左侧：阶段进度卡片
- 中部右侧：风险与待确认事项
- 底部左侧：最近文档与版本记录
- 底部右侧：原型和研发协同摘要

关键动作：

- 继续当前阶段
- 查看过期下游
- 重新生成建议
- 打开最新版本

### 5.2 需求入口页

目标：统一接住各种需求来源。

页面结构：

- 入口模式切换：
  - 直接输入
  - 上传 PRD
  - 上传会议纪要
  - 上传聊天记录
  - 上传多文件材料
- 左侧：原始材料列表
- 中间：解析结果预览
- 右侧：AI 识别的摘要、领域、风险和缺失项

关键动作：

- 开始解析
- 合并多个来源
- 标注优先来源
- 进入澄清流程

### 5.3 需求澄清页

这是最关键的页面，不应再是表单列表，而应是“逐题弹窗式 AI 面试官”。

推荐交互：

- 主页面展示当前澄清进度和历史回答摘要
- 弹窗一次只问一个问题
- 答完一个后弹下一个
- 每个问题支持：
  - 直接回答
  - 让我举例
  - 让我给建议答案
  - 暂时跳过
  - 返回上一题

主页面需要展示：

- 已回答数 / 总问题数
- 信息完整度评分
- 关键未覆盖维度
- 最终“生成需求共识稿”按钮

### 5.4 需求共识页

目标：在正式 PRD 前形成业务共识。

页面结构：

- 顶部：共识状态与来源摘要
- 主体分区：
  - 问题定义
  - 用户与角色
  - 目标与指标
  - 核心场景
  - 范围边界
  - 假设与依赖
  - 风险点
  - 待确认问题
- 右侧：AI 质检面板

关键动作：

- 人工编辑
- 确认共识稿
- 生成 PRD
- 查看影响到哪些下游

### 5.5 PRD 页

目标：PRD 成为正式业务文档，不再只是生成结果展示页。

页面结构：

- 左侧：章节树
- 中间：富文本编辑器 + Markdown/结构化混合视图
- 右侧：
  - 结构化字段预览
  - 质检结果
  - 缺失项提醒
  - 重生成建议

关键动作：

- 编辑章节
- 锁定章节不被覆盖
- 生成/重生成指定章节
- 查看版本 diff
- 基于 PRD 生成功能规格

### 5.6 功能规格页

目标：从“业务文档”转为“交互执行文档”。

页面结构：

- 左侧：页面清单 / 模块树
- 中间：页面规格或模块规格详情
- 右侧：字段规则、交互规则、状态规则、验收点

每个规格单元必须可以切换视图：

- 页面视图
- 模块视图
- 字段视图
- 状态视图
- 验收视图

### 5.7 研发实现说明页

目标：让研发可以直接估工、拆任务、评审方案。

页面结构：

- 左侧：技术章节树
- 中间：技术说明主体
- 右侧：实体、接口、状态机、任务建议

推荐分组：

- 领域实体
- 权限模型
- API 清单
- 数据流与状态流
- 非功能要求
- 异常处理
- 埋点与日志
- 任务拆解建议

### 5.8 原型与页面模型页

目标：不直接“画图”，而是先管理页面模型，再渲染原型。

页面结构：

- 左侧：页面导航
- 中间：原型画布
- 右侧：
  - 页面元信息
  - 组件树
  - 状态切换
  - mock data
  - 导出文件预览

页面上需要明确区分：

- 页面模型
- `UISchema`
- 原型渲染结果

### 5.9 研发协同页

目标：把产物延伸到研发和测试准备。

页面结构：

- 用户故事列表
- 开发任务拆解
- 测试点建议
- 埋点建议
- 上线 checklist

## 6. 每个阶段的数据字段定义

以下字段定义建议作为真相层和文档层的基础 Schema。

### 6.1 `Project`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 项目 ID |
| `name` | string | 项目名称 |
| `description` | string | 项目描述 |
| `domain` | string | 行业或领域 |
| `product_type` | string | 产品类型，如后台、SaaS、工具、平台 |
| `current_stage` | string | 当前阶段 |
| `status` | string | active / archived |
| `created_at` | datetime | 创建时间 |
| `updated_at` | datetime | 更新时间 |

### 6.2 `SourceMaterial`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 材料 ID |
| `project_id` | string | 归属项目 |
| `source_type` | string | text / prd / transcript / chat / file |
| `title` | string | 材料标题 |
| `content_raw` | text | 原始内容 |
| `content_parsed` | json | 解析后的结构 |
| `priority` | int | 信息可信优先级 |
| `language` | string | 语言 |
| `created_at` | datetime | 创建时间 |

### 6.3 `RequirementIntake`

| 字段 | 类型 | 说明 |
|---|---|---|
| `project_id` | string | 项目 ID |
| `intake_mode` | string | direct / import |
| `raw_input` | text | 用户直接输入的模糊需求 |
| `source_material_ids` | string[] | 来源材料 |
| `domain_guess` | string | AI 识别领域 |
| `problem_statement_draft` | text | 初步问题定义 |
| `information_gaps` | json | 信息缺口清单 |
| `quality_score` | number | 输入质量评分 |

### 6.4 `ClarificationSession`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 会话 ID |
| `project_id` | string | 项目 ID |
| `mode` | string | guided_dialog |
| `questions` | json | 问题列表 |
| `answers` | json | 回答列表 |
| `current_question_index` | int | 当前问题序号 |
| `completion_rate` | number | 完成度 |
| `open_gaps` | json | 仍未补齐的信息 |
| `status` | string | draft / completed |

单个问题建议字段：

- `id`
- `dimension`
- `question`
- `rationale`
- `priority`
- `answer_type`
- `is_required`
- `can_skip`
- `answer`
- `confidence_after_answer`

### 6.5 `ConsensusBrief`

| 字段 | 类型 | 说明 |
|---|---|---|
| `project_id` | string | 项目 ID |
| `problem_statement` | text | 问题定义 |
| `target_users` | json | 目标用户 |
| `business_goals` | json | 业务目标 |
| `success_metrics` | json | 成功指标 |
| `core_scenarios` | json | 核心场景 |
| `scope_in` | json | 做什么 |
| `scope_out` | json | 不做什么 |
| `assumptions` | json | 假设 |
| `risks` | json | 风险 |
| `dependencies` | json | 依赖 |
| `open_questions` | json | 待确认问题 |
| `confirmed` | bool | 是否确认 |

### 6.6 `RequirementModel`

这是最核心的结构对象，建议拆成多个子域：

#### 业务域

- `business_context`
- `goals`
- `personas`
- `roles`
- `terminology`

#### 产品域

- `features`
- `modules`
- `capabilities`
- `scope`
- `constraints`

#### 页面域

- `pages`
- `page_flows`
- `components`
- `field_definitions`
- `permissions`

#### 验收域

- `acceptance_criteria`
- `non_functional_requirements`
- `edge_cases`
- `error_states`

### 6.7 `PRDDocument`

建议固定字段：

- `title`
- `summary`
- `background`
- `problem_statement`
- `target_users`
- `business_goals`
- `success_metrics`
- `core_scenarios`
- `scope`
- `functional_overview`
- `non_functional_requirements`
- `risks_and_dependencies`
- `acceptance_criteria`

### 6.8 `FunctionalSpec`

建议字段按页面和模块组织：

- `pages`
- `modules`
- `field_rules`
- `interaction_rules`
- `state_definitions`
- `permission_rules`
- `validation_rules`
- `exception_flows`
- `acceptance_cases`

### 6.9 `TechnicalSpec`

建议字段：

- `system_context`
- `domain_entities`
- `data_model_draft`
- `api_endpoints`
- `event_flows`
- `state_machines`
- `permission_model`
- `logging_and_audit`
- `telemetry`
- `performance_constraints`
- `task_splitting`

### 6.10 `PageModel`

建议字段：

- `page_id`
- `title`
- `page_type`
- `goal`
- `target_role`
- `layout_type`
- `sections`
- `components`
- `primary_actions`
- `secondary_actions`
- `entry_conditions`
- `empty_states`
- `error_states`
- `loading_states`

### 6.11 `UISchema`

沿用现有方向，但要更明确作为页面结构层：

- `app_meta`
- `routes`
- `pages`
- `components`
- `forms`
- `tables`
- `actions`
- `mock_data`
- `state_variants`
- `navigation_map`

### 6.12 `TaskBundle` 与 `TestBundle`

这些适合 V2，但字段需要提前预留：

`TaskBundle`：

- `stories`
- `tasks`
- `priorities`
- `dependencies`
- `estimate_hints`

`TestBundle`：

- `test_scenarios`
- `acceptance_cases`
- `edge_cases`
- `regression_scope`

## 7. 每个 AI 节点应该怎么编排

AI 编排不应该是“一步一个 prompt”，而应当是有输入、输出、校验、人工确认和下游依赖关系的节点图。

### 7.1 总体编排原则

1. 每个节点只做一件事。
2. 每个节点都输出结构化结果。
3. 关键节点后必须有质检节点。
4. 上游改动后，下游只标记过期，不自动覆盖。
5. 文本生成节点只能读取真相层，不直接读取自由文本做全量发挥。

### 7.2 节点清单

#### N1. 输入归一化节点

输入：

- 原始文本
- 导入文件
- 多来源材料

输出：

- `RequirementIntake`
- 材料摘要
- 领域判断
- 缺口初判

#### N2. 领域识别节点

作用：

- 判断是后台系统、电商、SaaS、平台、内部工具还是内容产品
- 生成领域标签和术语基线

输出：

- `domain_guess`
- `product_type`
- `terminology_seed`

#### N3. 信息缺口评分节点

作用：

- 判断哪些信息缺失最影响后续生成质量

输出：

- 缺口维度列表
- 缺口严重度
- 建议提问顺序

#### N4. 澄清问题生成节点

作用：

- 根据缺口动态生成问题，不走固定题库

输出：

- 问题列表
- 优先级
- 回答类型

#### N5. 逐题追问节点

作用：

- 用户每回答一道题后，评估是否仍有缺口，并决定下一题

输出：

- 更新后的 `ClarificationSession`
- 新问题或结束信号

#### N6. 需求共识生成节点

作用：

- 汇总原始需求 + 澄清回答，产出共识稿

输出：

- `ConsensusBrief`

#### N7. 共识质检节点

作用：

- 检查是否缺少目标、边界、风险、验收等关键项

输出：

- 完整性评分
- 缺失项
- 修正建议

#### N8. PRD 生成节点

作用：

- 依据 `ConsensusBrief + RequirementModel` 生成正式 PRD

输出：

- `PRDDocument`

#### N9. PRD 标准化节点

作用：

- 当用户上传外部 PRD 时，把外部文档转成标准 PRD Schema

输出：

- 规范化后的 `PRDDocument`
- 与标准模板的差异

#### N10. 功能规格拆解节点

作用：

- 把 PRD 细化为页面、模块、字段、交互、状态、异常、验收

输出：

- `FunctionalSpec`
- `PageModel`

#### N11. 功能规格质检节点

作用：

- 检查是否存在：
  - 无页面对应的功能
  - 无字段规则的表单
  - 无异常流的关键操作
  - 无验收点的需求项

#### N12. 技术实现映射节点

作用：

- 把功能规格翻译成研发语言

输出：

- `TechnicalSpec`

#### N13. 页面模型生成节点

作用：

- 从 `FunctionalSpec` 提取页面和组件结构

输出：

- `PageModel[]`

#### N14. `UISchema` 生成节点

作用：

- 根据页面模型生成稳定原型结构

输出：

- `UISchema`

#### N15. 原型导出节点

作用：

- 生成预览
- 导出 React 脚手架

#### N16. 任务与测试建议节点

作用：

- 生成用户故事、开发任务、测试点、埋点建议

适合 V2 起启用。

### 7.3 每个节点的运行控制

每个节点需要统一支持：

- `draft`
- `generated`
- `reviewed`
- `approved`
- `stale`

同时每个节点都需要记录：

- 输入来源版本
- 输出版本
- prompt/context 摘要
- provider 信息
- 质量评分
- 错误原因

## 8. V1 / V2 / V3 的优先级路线图

### 8.1 V1：打透“需求到原型”的主闭环

目标：

- 证明从模糊需求到结构化产物再到原型预览的完整价值

必须包含：

1. 项目管理
2. 多入口需求输入
3. 逐题弹窗式澄清
4. 需求共识稿
5. PRD 生成与编辑
6. 外部 PRD 导入并标准化
7. 功能规格生成
8. 技术实现说明生成
9. 页面模型与 `UISchema`
10. 原型预览与导出
11. 版本、diff、stale 标记
12. Provider 设置与基础探针

暂不包含：

- 多人协作
- 评论审批流
- 企业知识库
- 高保真设计稿
- 云同步

V1 验收标准：

- 一个用户可以从模糊需求或上传 PRD 两条入口走完整流程
- 每个阶段都可编辑、可回退、可重生成
- 上游改动后下游不会静默污染

### 8.2 V2：补齐研发协同与质量控制

目标：

- 让系统从“文档和原型工具”升级为“研发准备工具”

新增范围：

1. 用户故事生成
2. 开发任务拆解
3. 测试用例建议
4. 埋点建议
5. 质检评分与缺陷提醒
6. 术语库与模板库
7. 外部材料批量导入
8. 页面级局部重生成

V2 核心指标：

- 研发拿到技术说明后能直接估工
- QA 能拿到初版测试场景
- 产品经理可对比不同版本变更影响

### 8.3 V3：平台化与协同化

目标：

- 让产品具备团队协作和平台能力

新增范围：

1. 登录与云同步
2. 组织、项目空间、权限
3. 评论、审阅、审批流
4. 多人协作编辑
5. 企业知识库与行业模板
6. 发布记录和反馈回流
7. 数据埋点闭环与版本复盘

V3 核心指标：

- 从个人工作台升级为团队产品平台

## 9. 推荐研发分层

### 9.1 Go 后端

建议拆成这些服务：

- `ProjectService`
- `SourceMaterialService`
- `ClarificationService`
- `RequirementModelService`
- `ArtifactService`
- `PrototypeService`
- `AIOrchestrator`
- `ProviderProbeService`

### 9.2 React 前端

建议按页面域拆分：

- `overview`
- `intake`
- `clarification`
- `consensus`
- `artifacts`
- `prototype`
- `delivery`
- `settings`

### 9.3 存储层

SQLite 第一版建议存三类内容：

1. 结构化业务对象
2. 文档与版本快照
3. 工作流与诊断日志

### 9.4 最重要的实现优先级

如果只能先做三件事，优先顺序是：

1. 需求真相层与共识稿
2. 逐题弹窗式澄清流程
3. 功能规格 / 页面模型 / `UISchema` 的结构化链路

## 10. 最后结论

真正的“产品经理全能工具”，不是让 AI 依次写：

- PRD
- 需求文档
- 原型

而是要建立这样一条主链路：

`多来源需求输入 -> AI 面试式澄清 -> 需求共识稿 -> 需求真相层 -> PRD -> 功能规格 -> 技术实现说明 -> 页面模型 -> UISchema -> 原型 -> 任务/测试/上线准备`

这条链路一旦稳定下来，后续无论做多人协作、模板库、知识库，还是云同步，都会自然长出来。

