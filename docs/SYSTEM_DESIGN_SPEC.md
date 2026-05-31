# 产品经理全能工具系统设计说明

## 1. 文档目标

这份文档承接 [开发蓝图](./DEVELOPMENT_BLUEPRINT.md)，把产品层面的规划进一步下钻为研发可执行说明。重点回答 5 个问题：

- 数据层应该怎么建
- 前端页面应该怎么拆
- Go 服务应该怎么分层
- Wails 桥接接口应该暴露什么
- AI 工作流应该如何编排、如何做状态管理和失败恢复

本文默认目标仍是 `Go + Wails + React + SQLite + OpenAI-compatible API`。

## 2. 系统设计总览

### 2.1 总体分层

建议整个系统保持 6 层结构：

1. `React 工作台层`
2. `Wails 桥接层`
3. `Go 应用服务层`
4. `Go 领域模型层`
5. `AI 编排层`
6. `SQLite 持久化层`

### 2.2 关键设计原则

1. 任何文本产物都不是主数据，主数据永远是结构化对象。
2. 任何自动生成结果都要落版本，并记录输入来源和依赖版本。
3. 任何下游产物都必须可以判断自己是否 `stale`。
4. AI 节点尽量输出结构化结果，再由渲染器生成文档或原型。
5. 同一项目允许混合来源：手输需求、上传 PRD、上传会议纪要、AI 生成文档、人工编辑文档。

### 2.3 核心主线

推荐主链路固定为：

`SourceMaterial -> RequirementIntake -> ClarificationSession -> ConsensusBrief -> RequirementModel -> PRD -> FunctionalSpec -> TechnicalSpec -> PageModel -> UISchema -> PrototypeBundle -> TaskBundle/TestBundle`

## 3. 数据库表设计

### 3.1 设计规范

- 主键统一使用 `uuid`
- 全表保留：
  - `id`
  - `created_at`
  - `updated_at`
  - `remote_id`
  - `sync_status`
  - `owner_id`
- JSON 型字段优先保存结构化快照，不依赖多张极细颗粒度子表
- 文档型对象统一用 `artifacts` + `artifact_versions` 管版本

### 3.2 表清单

#### 3.2.1 `projects`

用途：项目主记录

关键字段：

- `id`
- `name`
- `description`
- `domain`
- `stage`
- `status`
- `current_requirement_model_id`
- `current_consensus_brief_id`
- `archived_at`

索引建议：

- `idx_projects_updated_at`
- `idx_projects_stage`

#### 3.2.2 `source_materials`

用途：原始输入材料中心

关键字段：

- `id`
- `project_id`
- `source_type`
  - `free_text`
  - `prd_upload`
  - `meeting_notes`
  - `chat_log`
  - `bid_doc`
  - `other_file`
- `title`
- `raw_text`
- `file_name`
- `mime_type`
- `file_path`
- `parsed_content_json`
- `extraction_meta_json`
- `is_primary`

索引建议：

- `idx_source_materials_project_id`
- `idx_source_materials_source_type`

#### 3.2.3 `requirement_intakes`

用途：把多个原始来源归并成一次正式需求入口

关键字段：

- `id`
- `project_id`
- `title`
- `summary`
- `source_material_ids_json`
- `normalized_input_json`
- `domain_guess`
- `readiness_score`
- `status`

说明：

- 一个项目可以有多次 intake
- 一个 intake 可引用多个 `source_materials`

#### 3.2.4 `clarification_sessions`

用途：记录一次完整的澄清对话

关键字段：

- `id`
- `project_id`
- `requirement_intake_id`
- `mode`
  - `guided`
  - `full_auto`
- `status`
  - `pending`
  - `in_progress`
  - `completed`
  - `aborted`
- `current_question_index`
- `completion_score`
- `gap_map_json`

#### 3.2.5 `clarification_questions`

用途：保存澄清问题清单

关键字段：

- `id`
- `session_id`
- `question_order`
- `question_text`
- `question_type`
  - `goal`
  - `user`
  - `scope`
  - `workflow`
  - `permission`
  - `metric`
  - `risk`
- `why_this_matters`
- `priority`
- `status`
  - `pending`
  - `answered`
  - `skipped`

#### 3.2.6 `clarification_answers`

用途：保存用户逐题回答

关键字段：

- `id`
- `question_id`
- `answer_text`
- `answer_source`
  - `user`
  - `ai_suggested_then_accepted`
- `confidence`
- `answered_at`

#### 3.2.7 `consensus_briefs`

用途：需求共识稿

关键字段：

- `id`
- `project_id`
- `requirement_intake_id`
- `clarification_session_id`
- `version_no`
- `status`
  - `draft`
  - `generated`
  - `reviewed`
  - `approved`
- `structured_json`
- `rendered_markdown`
- `quality_report_json`

#### 3.2.8 `requirement_models`

用途：需求真相层核心对象

关键字段：

- `id`
- `project_id`
- `consensus_brief_id`
- `version_no`
- `status`
- `problem_definition_json`
- `actors_json`
- `goals_json`
- `flows_json`
- `scope_json`
- `entities_json`
- `rules_json`
- `acceptance_json`
- `constraints_json`
- `traceability_json`

说明：

- 这是最重要的一张表
- 后续所有文档和原型都应从这里派生

#### 3.2.9 `artifacts`

用途：逻辑文档容器

关键字段：

- `id`
- `project_id`
- `artifact_type`
  - `prd`
  - `functional_spec`
  - `technical_spec`
  - `prototype_source`
  - `task_bundle`
  - `test_bundle`
- `title`
- `current_version_id`

#### 3.2.10 `artifact_versions`

用途：文档版本快照

关键字段：

- `id`
- `artifact_id`
- `project_id`
- `source_version_id`
- `source_requirement_model_id`
- `version_no`
- `status`
  - `draft`
  - `generated`
  - `reviewed`
  - `approved`
  - `stale`
- `structured_json`
- `rendered_markdown`
- `plain_text_snapshot`
- `generation_meta_json`
- `quality_report_json`

#### 3.2.11 `page_models`

用途：页面和模块规划层

关键字段：

- `id`
- `project_id`
- `requirement_model_id`
- `version_no`
- `status`
- `app_structure_json`
- `pages_json`
- `modules_json`
- `navigation_json`
- `component_rules_json`

#### 3.2.12 `ui_schemas`

用途：原型渲染结构

关键字段：

- `id`
- `project_id`
- `page_model_id`
- `version_no`
- `status`
- `schema_json`
- `validation_report_json`

#### 3.2.13 `prototype_bundles`

用途：原型导出结果

关键字段：

- `id`
- `project_id`
- `ui_schema_id`
- `bundle_type`
  - `preview_only`
  - `react_export`
- `output_path`
- `manifest_json`

#### 3.2.14 `workflow_runs`

用途：所有 AI 流程的运行记录

关键字段：

- `id`
- `project_id`
- `workflow_type`
- `node_name`
- `status`
- `input_ref_json`
- `output_ref_json`
- `provider_snapshot_json`
- `latency_ms`
- `error_message`

#### 3.2.15 `provider_configs`

用途：AI 配置

关键字段：

- `id`
- `provider_name`
- `base_url`
- `resolved_api_base`
- `api_format`
  - `responses`
  - `chat_completions`
- `model`
- `api_key_encrypted`
- `timeout_ms`
- `temperature`
- `is_active`
- `last_probe_result_json`

### 3.3 关键关系

- 一个 `project` 有多个 `source_materials`
- 一个 `requirement_intake` 可关联多个 `source_materials`
- 一个 `clarification_session` 对应一个 intake
- 一个 `consensus_brief` 基于一次澄清会话
- 一个 `requirement_model` 基于一个 `consensus_brief`
- 多个 `artifacts` 都引用同一个 `requirement_model`
- `page_models` 基于 `requirement_model`
- `ui_schemas` 基于 `page_models`
- `prototype_bundles` 基于 `ui_schemas`

## 4. 前端页面与线框说明

### 4.1 桌面布局原则

建议统一采用：

- 左侧：项目导航和阶段导航
- 中间：当前阶段主工作区
- 右侧：AI 解释、质量报告、影响分析、快捷动作

### 4.2 页面清单

#### 4.2.1 项目总览页

主要区块：

- 项目头部信息
- 阶段进度条
- 待处理事项卡片
- 最近版本时间线
- 下游失效提醒
- 快捷入口

页面目标：

- 让用户明确当前做到哪里
- 让用户知道哪些内容改了会影响下游

#### 4.2.2 需求入口页

主要区块：

- 入口切换器
- 原始材料上传区
- 材料列表
- 解析结果预览
- AI 初始识别摘要

关键交互：

- 选择一种输入方式
- 上传或粘贴原始材料
- 标记主输入源
- 合并多个来源

#### 4.2.3 需求澄清页

主要区块：

- 顶部进度卡
- 历史回答摘要
- 当前问题弹窗
- 信息缺口地图
- 跳题和返回上一题按钮

交互规则：

- 一次只出现一个问题
- 回答完成后再推进下一题
- 所有问题回答完成后自动进入共识稿生成

#### 4.2.4 需求共识页

主要区块：

- 问题定义
- 用户与角色
- 业务目标
- 范围与边界
- 核心流程
- 风险与依赖
- 成功指标
- AI 质量诊断卡

页面目标：

- 这是 PRD 前的关键确认页
- 没有确认共识稿，就不建议放行 PRD

#### 4.2.5 PRD 工作页

主要区块：

- 章节导航
- 富文本/结构化双视图
- 质量检查面板
- 与共识稿差异提示
- 版本切换与 Diff

关键动作：

- 重新生成单章节
- 人工编辑
- 批准版本
- 标记下游为过期

#### 4.2.6 功能规格页

主要区块：

- 页面树
- 模块树
- 字段清单
- 交互说明
- 状态与异常态
- 验收标准

页面目标：

- 把 PRD 真正落成研发和设计可执行说明

#### 4.2.7 研发实现页

主要区块：

- 领域实体
- 接口与事件
- 状态流转
- 权限模型
- 非功能约束
- 任务拆解

页面目标：

- 面向研发评审
- 让工程师可以直接估工和拆任务

#### 4.2.8 页面模型与原型页

主要区块：

- 页面清单
- 页面结构树
- 路由图
- `UISchema` JSON 视图
- 预览区
- 导出区

关键动作：

- 只重生成某个页面
- 查看页面状态变体
- 导出 React 脚手架

#### 4.2.9 研发协同页

主要区块：

- 用户故事
- 开发任务
- 测试场景
- 埋点建议
- 上线检查项

说明：

- 这是 V1.5 或 V2 非常值得补的一页
- 它能把产品经理工作自然延伸到交付前

#### 4.2.10 设置页

主要区块：

- Provider 配置
- API 连接测试
- 模板设置
- 项目导入导出
- 日志和诊断

## 5. 每个阶段的数据字段定义

### 5.1 需求入口阶段

建议字段：

- `project_name`
- `business_domain`
- `source_type`
- `raw_input_text`
- `source_files`
- `initial_goal`
- `known_constraints`
- `deadline_hint`
- `priority_hint`

### 5.2 澄清阶段

建议字段：

- `clarification_session_id`
- `question_set`
- `answered_count`
- `skipped_count`
- `gap_map`
- `confidence_score`
- `blocking_unknowns`

### 5.3 共识阶段

建议字段：

- `problem_statement`
- `target_users`
- `business_goals`
- `success_metrics`
- `core_scenarios`
- `in_scope_items`
- `out_of_scope_items`
- `risks`
- `dependencies`
- `assumptions`

### 5.4 PRD 阶段

建议字段：

- `background`
- `opportunity`
- `goals`
- `personas`
- `journeys`
- `feature_list`
- `non_functional_requirements`
- `acceptance_criteria`
- `release_scope`

### 5.5 功能规格阶段

建议字段：

- `page_inventory`
- `module_inventory`
- `field_definitions`
- `interaction_rules`
- `status_variants`
- `permissions`
- `exceptions`
- `acceptance_cases`

### 5.6 技术实现阶段

建议字段：

- `domain_entities`
- `api_contracts`
- `events`
- `state_machines`
- `permission_model`
- `logging_rules`
- `monitoring_points`
- `technical_risks`
- `task_breakdown`

### 5.7 原型阶段

建议字段：

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

## 6. Go 服务边界与职责

### 6.1 `ProjectService`

职责：

- 项目创建、列表、归档
- 当前阶段管理
- 当前主版本指针管理

### 6.2 `SourceMaterialService`

职责：

- 保存文本与文件型来源
- 调用解析器抽取文本
- 聚合多来源输入

### 6.3 `ClarificationService`

职责：

- 生成问题集合
- 按顺序推进问题
- 保存逐题回答
- 输出澄清完成度和 gap map

### 6.4 `ConsensusService`

职责：

- 基于 intake 和澄清结果生成共识稿
- 执行共识稿质量检查
- 允许用户确认或退回

### 6.5 `RequirementModelService`

职责：

- 维护真相层结构
- 负责从共识稿归一到正式需求模型
- 做字段级 diff 和影响分析

### 6.6 `ArtifactService`

职责：

- 管理 PRD、功能规格、技术说明等文档容器
- 创建版本
- 标记 `stale`
- 执行文档重生成

### 6.7 `PrototypeService`

职责：

- 从 `RequirementModel` 生成 `PageModel`
- 从 `PageModel` 生成 `UISchema`
- 渲染预览
- 导出 React 原型脚手架

### 6.8 `DeliveryService`

职责：

- 生成用户故事
- 输出开发任务建议
- 输出测试建议
- 输出埋点和上线检查项

### 6.9 `ProviderService`

职责：

- 管理 provider 配置
- 连接测试
- 标准接口探测
- 请求封装、重试、降级、超时

### 6.10 `WorkflowOrchestrator`

职责：

- 编排各阶段 AI 节点
- 管理 workflow run
- 处理中断、重试、恢复
- 保证版本引用链正确

## 7. Wails 桥接接口清单

### 7.1 项目与来源

- `CreateProject`
- `ListProjects`
- `ArchiveProject`
- `SaveRequirementInput`
- `ImportSourceMaterial`
- `ListSourceMaterials`
- `CreateRequirementIntake`

### 7.2 澄清与共识

- `StartClarificationSession`
- `GetCurrentClarificationQuestion`
- `SubmitClarificationAnswer`
- `SkipClarificationQuestion`
- `BackToPreviousClarificationQuestion`
- `CompleteClarificationSession`
- `GenerateConsensusBrief`
- `ApproveConsensusBrief`
- `RegenerateConsensusBrief`

### 7.3 需求真相层与文档

- `BuildRequirementModel`
- `GetRequirementModel`
- `DiffRequirementModelVersions`
- `GenerateArtifact`
- `UpdateArtifactSection`
- `ApproveArtifactVersion`
- `ListArtifactVersions`
- `CompareArtifactVersions`
- `RegenerateDownstreamArtifacts`

### 7.4 原型与交付

- `GeneratePageModel`
- `GenerateUISchema`
- `PreviewPrototype`
- `GeneratePrototypeBundle`
- `ExportPrototype`
- `GenerateTaskBundle`
- `GenerateTestBundle`

### 7.5 Provider 与诊断

- `GetProviderConfig`
- `SaveProviderConfig`
- `TestProviderConnection`
- `ListAvailableModels`
- `GetWorkflowRunHistory`
- `GetDiagnostics`

## 8. AI 工作流状态机

### 8.1 主状态

一个项目建议至少有以下工作流状态：

- `intake_pending`
- `intake_normalized`
- `clarification_in_progress`
- `clarification_completed`
- `consensus_generated`
- `consensus_approved`
- `requirement_model_built`
- `prd_generated`
- `functional_spec_generated`
- `technical_spec_generated`
- `page_model_generated`
- `ui_schema_generated`
- `prototype_exported`
- `delivery_bundle_generated`

### 8.2 版本状态

每个产物版本建议统一使用：

- `draft`
- `generated`
- `reviewed`
- `approved`
- `stale`

### 8.3 典型流转规则

1. 共识稿未批准，不建议正式生成 PRD。
2. `RequirementModel` 更新后，PRD、功能规格、技术说明、页面模型全部标 `stale`。
3. PRD 人工修改后，功能规格和技术说明标 `stale`。
4. 页面模型更新后，`UISchema` 和原型导出标 `stale`。
5. 任何阶段生成失败，都要保留失败记录和输入快照。

## 9. AI 节点编排与契约

### 9.1 节点 1：输入归一化

输入：

- 原始文本
- 文件解析结果
- 来源标签

输出：

- `normalized_input_json`
- 初始领域识别
- 缺失信息列表

校验重点：

- 是否提取了目标用户
- 是否提取了核心目标
- 是否识别了边界和约束

### 9.2 节点 2：澄清问题生成

输入：

- `normalized_input_json`
- 当前 gap map

输出：

- 3 到 7 个高价值问题
- 每个问题的原因和优先级

校验重点：

- 问题是否覆盖用户、目标、范围、流程、指标、风险
- 问题是否避免重复和无关发散

### 9.3 节点 3：共识稿生成

输入：

- 原始来源摘要
- 全部澄清问答

输出：

- 共识稿结构 JSON
- 可读 markdown
- 质量报告

校验重点：

- 是否存在范围边界
- 是否存在成功标准
- 是否存在风险与依赖

### 9.4 节点 4：需求真相层构建

输入：

- 共识稿结构 JSON

输出：

- `RequirementModel`

校验重点：

- 字段完整性
- 对象之间引用一致性
- 页面和流程是否可落地

### 9.5 节点 5：PRD 生成

输入：

- `RequirementModel`

输出：

- PRD 结构 JSON
- PRD markdown

校验重点：

- 是否有背景、目标、角色、范围、验收、风险

### 9.6 节点 6：功能规格生成

输入：

- `RequirementModel`
- PRD

输出：

- 页面级和模块级规格 JSON
- 可读规格文档

校验重点：

- 是否落到页面、模块、字段、状态、异常

### 9.7 节点 7：技术实现说明生成

输入：

- `RequirementModel`
- 功能规格

输出：

- 研发说明 JSON
- 研发说明 markdown

校验重点：

- 是否具备实体、接口、事件、任务拆解

### 9.8 节点 8：页面模型生成

输入：

- 功能规格
- 技术约束

输出：

- `PageModel`

校验重点：

- 页面数量是否合理
- 是否包含列表、详情、表单、仪表盘等必要模式

### 9.9 节点 9：`UISchema` 生成

输入：

- `PageModel`

输出：

- `UISchema`

校验重点：

- schema 是否结构完整
- 组件引用是否有效
- 状态变体是否齐全

### 9.10 节点 10：任务与测试建议生成

输入：

- `RequirementModel`
- 功能规格
- 技术说明

输出：

- 任务拆解
- 测试建议
- 埋点建议

校验重点：

- 是否能直接用于研发和 QA 拆工

## 10. Prompt 与 Schema 设计原则

### 10.1 Prompt 设计原则

1. 尽量让模型输出 JSON，再由程序渲染文本。
2. Prompt 里明确角色、输入范围、禁止臆造规则。
3. 每个节点都限制输出目标，不让一个节点承担过多任务。
4. 所有关键节点都应有 fallback 和解析失败恢复逻辑。

### 10.2 Schema 设计原则

1. 每个字段都要有固定名称和含义。
2. 枚举字段尽量固定值域。
3. 所有列表字段都要允许空数组，但不返回 `null`。
4. 所有引用对象都要带来源版本。

## 11. V1 / V2 / V3 路线图

### 11.1 V1：打透需求到原型闭环

必须做：

- 多来源需求入口
- 逐题弹窗式澄清
- 需求共识稿
- 真相层 `RequirementModel`
- PRD / 功能规格 / 技术说明
- `PageModel -> UISchema -> 原型预览`
- 版本关系和 `stale` 标记

可以延后：

- 登录
- 云同步
- 多人协作
- 评论审批流

### 11.2 V2：补齐交付和协同能力

重点做：

- 任务拆解
- 测试建议
- 埋点方案
- 上线检查项
- 团队协作
- 评审流程
- 角色权限

### 11.3 V3：平台化与知识化

重点做：

- 行业模板库
- 企业知识库
- RAG
- 团队资产复用
- 云端同步与组织空间
- 版本分析与项目复盘

## 12. 推荐实施顺序

### 12.1 第一阶段

先重做数据模型：

- 引入 `SourceMaterial`
- 引入 `ConsensusBrief`
- 引入 `RequirementModel`
- 引入 `PageModel`

### 12.2 第二阶段

再重做交互主线：

- 需求入口页
- 逐题澄清页
- 共识确认页

### 12.3 第三阶段

然后重做文档和原型链路：

- PRD 生成器
- 功能规格生成器
- 技术说明生成器
- 页面模型和 `UISchema` 生成器

### 12.4 第四阶段

最后补交付和平台能力：

- 任务与测试建议
- 协同与评论
- 登录与云同步

## 13. 最后的建议

如果要把这个产品做成真正的“产品经理全能工具”，最关键的不是把每份文档写得更长，而是先建立：

- 统一的需求真相层
- 统一的版本引用链
- 统一的页面模型与原型中间层

一旦这三件事立住，PRD、功能规格、研发说明、原型和交付建议的质量才会一起上升。
