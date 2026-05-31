package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractJSONBlock(t *testing.T) {
	payload, err := extractJSONBlock("```json\n{\"hello\":\"world\"}\n```")
	if err != nil {
		t.Fatalf("extractJSONBlock returned error: %v", err)
	}
	if payload != "{\"hello\":\"world\"}" {
		t.Fatalf("unexpected payload: %s", payload)
	}
}

func TestExtractJSONBlock_FindsFirstBalancedObjectInsideMixedText(t *testing.T) {
	payload, err := extractJSONBlock("前置说明 {\"hello\":\"world\"} 后置说明 {\"ignored\":true}")
	if err != nil {
		t.Fatalf("extractJSONBlock returned error: %v", err)
	}
	if payload != "{\"hello\":\"world\"}" {
		t.Fatalf("unexpected payload: %s", payload)
	}
}

func TestParseDocumentJSON_CoercesFlexibleOpenQuestions(t *testing.T) {
	document, err := parseDocumentJSON(`{
		"artifactType":"PRD",
		"title":"Demo PRD",
		"summary":"summary",
		"sections":[
			{"id":"background","title":"背景","body":"内容","html":"<p>内容</p>"}
		],
		"openQuestions":[
			{"question":"谁来审批？"},
			{"text":"是否需要云同步？"}
		]
	}`)
	if err != nil {
		t.Fatalf("parseDocumentJSON returned error: %v", err)
	}
	if len(document.OpenQuestions) != 2 {
		t.Fatalf("expected 2 coerced open questions, got %d", len(document.OpenQuestions))
	}
	if document.OpenQuestions[0] != "谁来审批？" {
		t.Fatalf("unexpected first open question: %s", document.OpenQuestions[0])
	}
}

func TestParseDocumentJSON_CoercesTopLevelFieldsIntoSections(t *testing.T) {
	document, err := parseDocumentJSON(`{
		"artifactType":"FunctionalSpec",
		"title":"Demo Functional Spec",
		"summary":"summary",
		"modules":"需求输入、PRD、原型导出",
		"acceptance":"能够跑通全链路"
	}`)
	if err != nil {
		t.Fatalf("parseDocumentJSON returned error: %v", err)
	}
	if len(document.Sections) < 2 {
		t.Fatalf("expected top-level fields to become sections, got %d", len(document.Sections))
	}
	if document.Title == "" {
		t.Fatalf("expected parser to provide a default title")
	}
}

func TestParseClarificationPayload_CoercesStringSummary(t *testing.T) {
	summary, questions, err := parseClarificationPayload(`{
		"summary":"这是一个产品经理 AI 工作台，用于把模糊需求收敛成文档和原型。",
		"questions":[
			{"question":"第一版优先支持哪些模块？"},
			"是否需要多人协作？"
		]
	}`)
	if err != nil {
		t.Fatalf("parseClarificationPayload returned error: %v", err)
	}
	if len(summary.BusinessGoals) == 0 {
		t.Fatalf("expected string summary to be coerced into business goals")
	}
	if len(questions) != 2 {
		t.Fatalf("expected 2 coerced questions, got %d", len(questions))
	}
	if questions[1].Question != "是否需要多人协作？" {
		t.Fatalf("unexpected second question: %s", questions[1].Question)
	}
}

func TestWorkflowEndToEndMock(t *testing.T) {
	ctx := context.Background()
	service, repo, tempDir := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "PM AI 平台",
		Description: "从模糊需求到原型的本地工作台",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "客户希望做一款覆盖产品经理全流程的 AI 工具，从需求到 PRD 再到原型都能串起来。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}

	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if len(requirement.Clarifications) < 3 {
		t.Fatalf("expected clarifications to be generated, got %d", len(requirement.Clarifications))
	}

	answers := []ClarificationAnswer{
		{QuestionID: requirement.Clarifications[0].ID, Answer: "第一版先聚焦个人产品经理工作流。"},
		{QuestionID: requirement.Clarifications[1].ID, Answer: "审批流暂时不用，后续再加。"},
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers:   answers,
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}

	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeTechnicalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(TechnicalSpec) returned error: %v", err)
	}
	uiVersion, err := service.GenerateUISchema(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateUISchema returned error: %v", err)
	}
	if uiVersion.Type != ArtifactTypePrototype {
		t.Fatalf("expected ui version type %s, got %s", ArtifactTypePrototype, uiVersion.Type)
	}

	bundle, err := service.GeneratePrototypeBundle(ctx, project.ID)
	if err != nil {
		t.Fatalf("GeneratePrototypeBundle returned error: %v", err)
	}
	if len(bundle.Files) < 4 {
		t.Fatalf("expected prototype bundle files, got %d", len(bundle.Files))
	}
	if strings.TrimSpace(bundle.PreviewHTML) == "" {
		t.Fatalf("expected preview html to be generated")
	}
	if strings.TrimSpace(bundle.RenderMode) == "" {
		t.Fatalf("expected render mode to be set")
	}
	foundPreviewFile := false
	for _, file := range bundle.Files {
		if file.Path == "preview.html" {
			foundPreviewFile = true
			break
		}
	}
	if !foundPreviewFile {
		t.Fatalf("expected preview.html to exist in bundle")
	}

	exported, err := service.ExportPrototype(ctx, ExportPrototypeInput{
		ProjectID: project.ID,
		OutputDir: filepath.Join(tempDir, "exports"),
	})
	if err != nil {
		t.Fatalf("ExportPrototype returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exported.ExportPath, "src", "App.tsx")); err != nil {
		t.Fatalf("expected exported App.tsx to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exported.ExportPath, "preview.html")); err != nil {
		t.Fatalf("expected exported preview.html to exist: %v", err)
	}
}

func TestManualEditMarksDownstreamArtifactsStale(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Stale Test", Description: "测试失效标记"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "需要从需求自动生成完整文档链路"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "先做好个人版"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}
	prd, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD})
	if err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}

	document, err := parseDocumentJSON(prd.StructuredJSON)
	if err != nil {
		t.Fatalf("parseDocumentJSON returned error: %v", err)
	}
	sectionID := document.Sections[0].ID
	if _, err := service.UpdateArtifactSection(ctx, UpdateArtifactSectionInput{
		VersionID:   prd.ID,
		SectionID:   sectionID,
		Content:     "修改后的 PRD 内容",
		ContentHTML: "<p>修改后的 PRD 内容</p>",
	}); err != nil {
		t.Fatalf("UpdateArtifactSection returned error: %v", err)
	}

	functionalVersion, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypeFunctionalSpec)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion returned error: %v", err)
	}
	if functionalVersion.Status != VersionStatusStale {
		t.Fatalf("expected downstream functional spec to be stale, got %s", functionalVersion.Status)
	}
}

func TestRepairPrototypeCreatesProjectScopedPrototypeVersion(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "修复型原型",
		Description: "验证项目级原型修复",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "做一个用户管理系统，需要概览、用户列表、角色管理、审计日志页面。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "主要给内部管理员使用。"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := service.GenerateUISchema(ctx, project.ID); err != nil {
		t.Fatalf("GenerateUISchema returned error: %v", err)
	}

	bundle, err := service.RepairPrototype(ctx, PrototypeRepairInput{
		ProjectID:         project.ID,
		PageID:            "dashboard",
		CurrentIssue:      "左侧菜单点击无反应。",
		ExpectedBehavior:  "点击菜单要切换到对应页面。",
		OptimizationNotes: "保留当前项目的菜单和信息架构。",
	})
	if err != nil {
		t.Fatalf("RepairPrototype returned error: %v", err)
	}
	if bundle.RepairReport == nil {
		t.Fatalf("expected repair report to be returned")
	}
	if !bundle.RepairReport.Applied {
		t.Fatalf("expected repair to be marked as applied")
	}

	version, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypePrototype)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion returned error: %v", err)
	}
	if version.VersionNumber < 2 {
		t.Fatalf("expected repaired prototype to create a new version, got %d", version.VersionNumber)
	}
	if !strings.Contains(version.GenerationMeta, "prototype_render_bundle") {
		t.Fatalf("expected repaired version meta to persist prototype render bundle, got %s", version.GenerationMeta)
	}
	if !strings.Contains(version.GenerationMeta, "左侧菜单点击无反应") {
		t.Fatalf("expected repaired version meta to include repair issue, got %s", version.GenerationMeta)
	}

	previewBundle, err := service.PreviewPrototype(ctx, project.ID)
	if err != nil {
		t.Fatalf("PreviewPrototype returned error: %v", err)
	}
	if previewBundle.RepairReport == nil {
		t.Fatalf("expected preview bundle to include cached repair report")
	}
}

func TestIntegration_WorkflowPrerequisiteErrors(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Prereq Test", Description: "验证前置条件"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "准备一个产品经理 AI 工作台"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	if _, err := service.GenerateClarifications(ctx, project.ID); err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}

	_, err = service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec})
	if err == nil || !strings.Contains(err.Error(), "generate PRD before functional spec") {
		t.Fatalf("expected missing PRD prerequisite error, got %v", err)
	}

	_, err = service.GenerateUISchema(ctx, project.ID)
	if err == nil || !strings.Contains(err.Error(), "generate functional spec before UI schema") {
		t.Fatalf("expected missing FunctionalSpec prerequisite error, got %v", err)
	}
}

func TestIntegration_BuildArtifactContext_UsesStandardizedUploadedPRDBaseline(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "Uploaded PRD Baseline",
		Description: "验证功能规格上下文优先使用 PRD 标准化基线",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	source, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户 PRD 初稿",
		RawText: strings.Join([]string{
			"# 背景与问题",
			"当前需求反复改写，文档与原型经常脱节。",
			"## 目标与指标",
			"30 分钟内生成可审阅的 PRD 与功能规格。",
			"## 核心功能",
			"支持澄清、PRD 标准化、功能规格。",
			"## 验收标准",
			"能够输出结构化功能规格。",
		}, "\n"),
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	contextPayload, schemaName, sourceVersionID, err := service.buildArtifactContext(ctx, project, nil, ArtifactTypeFunctionalSpec)
	if err != nil {
		t.Fatalf("buildArtifactContext returned error: %v", err)
	}
	if schemaName != "functional_spec" {
		t.Fatalf("expected functional_spec schema, got %s", schemaName)
	}
	if sourceVersionID != source.ID {
		t.Fatalf("expected source version id to point at uploaded prd, got %s", sourceVersionID)
	}
	if !strings.Contains(fmt.Sprint(contextPayload["prd_markdown"]), "## 背景与问题") {
		t.Fatalf("expected prd markdown to use canonical baseline headings, got %+v", contextPayload["prd_markdown"])
	}
	baseline, ok := contextPayload["uploaded_prd_baseline"].(map[string]any)
	if !ok {
		t.Fatalf("expected uploaded_prd_baseline in context, got %+v", contextPayload["uploaded_prd_baseline"])
	}
	if baseline["completeness_score"] == nil {
		t.Fatalf("expected completeness score in uploaded_prd_baseline, got %+v", baseline)
	}
	functionalSeed, ok := contextPayload["functional_spec_seed"].(FunctionalSpecSeed)
	if !ok {
		t.Fatalf("expected functional_spec_seed in context, got %+v", contextPayload["functional_spec_seed"])
	}
	if len(functionalSeed.Pages) == 0 {
		t.Fatalf("expected functional_spec_seed to include pages, got %+v", functionalSeed)
	}
}

func TestIntegration_BuildArtifactContext_UsesTechnicalSpecSeed(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "Technical Seed",
		Description: "验证技术种子上下文",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "技术种子 PRD",
		RawText: strings.Join([]string{
			"# 核心功能",
			"客户列表",
			"客户详情",
			"## 验收标准",
			"需要覆盖筛选与详情查看。",
		}, "\n"),
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}

	contextPayload, schemaName, _, err := service.buildArtifactContext(ctx, project, nil, ArtifactTypeTechnicalSpec)
	if err != nil {
		t.Fatalf("buildArtifactContext returned error: %v", err)
	}
	if schemaName != "technical_spec" {
		t.Fatalf("expected technical_spec schema, got %s", schemaName)
	}
	technicalSeed, ok := contextPayload["technical_spec_seed"].(TechnicalSpecSeed)
	if !ok {
		t.Fatalf("expected technical_spec_seed in context, got %+v", contextPayload["technical_spec_seed"])
	}
	if len(technicalSeed.Interfaces) == 0 {
		t.Fatalf("expected technical seed interfaces, got %+v", technicalSeed)
	}
}

func TestIntegration_GeneratePRDFromUploadedSourceMaterial(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Imported PRD", Description: "验证上传 PRD 标准化"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	source, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户现有 PRD",
		RawText:    "# 客户现有 PRD\n\n## 背景\n已有较完整的目标、角色和范围描述。\n\n## 核心功能\n需求澄清、PRD、功能规格、原型。",
		IsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	version, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD})
	if err != nil {
		t.Fatalf("GenerateArtifact(PRD) from uploaded source returned error: %v", err)
	}
	if version.Type != ArtifactTypePRD {
		t.Fatalf("expected PRD artifact, got %s", version.Type)
	}
	if version.SourceVersionID != source.ID {
		t.Fatalf("expected source version id to point at uploaded material, got %s", version.SourceVersionID)
	}
	if !strings.Contains(version.RenderedMarkdown, "PRD") {
		t.Fatalf("expected rendered markdown to contain PRD content, got %s", version.RenderedMarkdown)
	}
}

func TestIntegration_GenerateFunctionalSpecFromUploadedPRD(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Imported Functional", Description: "验证上传 PRD 直转功能规格"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	source, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "历史 PRD 文档",
		RawText: strings.Join([]string{
			"# 核心功能",
			"历史 PRD 列表",
			"历史 PRD 详情",
			"历史 PRD 编辑",
			"## 验收标准",
			"需要覆盖页面、字段和验收条件。",
		}, "\n"),
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	version, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec})
	if err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) from uploaded PRD returned error: %v", err)
	}
	if version.Type != ArtifactTypeFunctionalSpec {
		t.Fatalf("expected FunctionalSpec artifact, got %s", version.Type)
	}
	if version.SourceVersionID != source.ID {
		t.Fatalf("expected functional spec to trace back to uploaded PRD material, got %s", version.SourceVersionID)
	}
	if !strings.Contains(version.PlainTextSnapshot, "页面与流程") || !strings.Contains(version.PlainTextSnapshot, "历史 PRD 列表") {
		t.Fatalf("expected functional spec output to reflect execution seed, got %s", version.PlainTextSnapshot)
	}
	if !strings.Contains(version.GenerationMeta, "functional_spec_seed") {
		t.Fatalf("expected generation meta to persist functional execution seed, got %s", version.GenerationMeta)
	}
}

func TestIntegration_GenerateTechnicalSpecUsesExecutionSeed(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Technical Seed Output", Description: "验证研发说明使用技术种子"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户 PRD",
		RawText: strings.Join([]string{
			"# 核心功能",
			"客户列表",
			"新建客户",
			"## 风险与依赖",
			"需要覆盖接口异常提示。",
		}, "\n"),
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}

	version, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeTechnicalSpec})
	if err != nil {
		t.Fatalf("GenerateArtifact(TechnicalSpec) returned error: %v", err)
	}
	if !strings.Contains(version.PlainTextSnapshot, "接口与动作") || !strings.Contains(version.PlainTextSnapshot, "Query") {
		t.Fatalf("expected technical spec output to reflect technical seed, got %s", version.PlainTextSnapshot)
	}
	if !strings.Contains(version.GenerationMeta, "technical_spec_seed") {
		t.Fatalf("expected generation meta to persist technical execution seed, got %s", version.GenerationMeta)
	}
}

func TestIntegration_BuildArtifactContext_UsesUISchemaSeed(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Prototype Seed Context", Description: "验证原型蓝图上下文"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "原型种子 PRD",
		RawText: strings.Join([]string{
			"# 核心功能",
			"客户总览",
			"客户列表",
			"新建客户",
			"客户详情",
			"## 验收标准",
			"需要覆盖筛选、表单提交和详情查看。",
		}, "\n"),
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeTechnicalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(TechnicalSpec) returned error: %v", err)
	}

	contextPayload, schemaName, _, err := service.buildArtifactContext(ctx, project, nil, ArtifactTypePrototype)
	if err != nil {
		t.Fatalf("buildArtifactContext returned error: %v", err)
	}
	if schemaName != "ui_schema" {
		t.Fatalf("expected ui_schema schema, got %s", schemaName)
	}
	uiSchemaSeed, ok := contextPayload["ui_schema_seed"].(UISchemaSeed)
	if !ok {
		t.Fatalf("expected ui_schema_seed in prototype context, got %+v", contextPayload["ui_schema_seed"])
	}
	if len(uiSchemaSeed.Pages) == 0 || len(uiSchemaSeed.Actions) == 0 {
		t.Fatalf("expected ui schema seed pages and actions, got %+v", uiSchemaSeed)
	}
}

func TestIntegration_GenerateUISchemaUsesExecutionSeed(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Prototype Seed Output", Description: "验证 UISchema 使用原型蓝图"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户管理 PRD",
		RawText: strings.Join([]string{
			"# 核心功能",
			"客户总览",
			"客户列表",
			"新建客户",
			"客户详情",
			"## 验收标准",
			"需要覆盖列表筛选、表单提交和详情动作。",
		}, "\n"),
		IsPrimary: true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeTechnicalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(TechnicalSpec) returned error: %v", err)
	}

	version, err := service.GenerateUISchema(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateUISchema returned error: %v", err)
	}
	if !strings.Contains(version.GenerationMeta, "ui_schema_seed") {
		t.Fatalf("expected generation meta to persist ui_schema_seed, got %s", version.GenerationMeta)
	}
	uiSchema, err := parseUISchemaJSON(version.StructuredJSON)
	if err != nil {
		t.Fatalf("parseUISchemaJSON returned error: %v", err)
	}
	if len(uiSchema.Pages) < 3 {
		t.Fatalf("expected seeded ui schema pages, got %+v", uiSchema.Pages)
	}
	if uiSchema.Pages[0].Title == "" || len(uiSchema.Actions) == 0 {
		t.Fatalf("expected ui schema to include seeded page titles and actions, got %+v", uiSchema)
	}
	if len(uiSchema.Tables) == 0 || len(uiSchema.Forms) == 0 {
		t.Fatalf("expected ui schema to include seeded tables and forms, got %+v", uiSchema)
	}
	if _, ok := uiSchema.MockData["table_rows"]; !ok {
		t.Fatalf("expected ui schema mock data to include table_rows, got %+v", uiSchema.MockData)
	}
	if _, ok := uiSchema.MockData["detail_records"]; !ok {
		t.Fatalf("expected ui schema mock data to include detail_records, got %+v", uiSchema.MockData)
	}
}

func TestGrayBox_WorkflowRunMarkedFailedOnMalformedProviderJSON(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{
				{
					"content": []map[string]any{
						{"type": "output_text", "text": "{not-json"},
					},
				},
			},
		})
	}))
	defer server.Close()

	service, repo := newConfiguredWorkbench(t, ProviderConfig{
		Name:         "Malformed Responses",
		ProviderType: "openai-compatible",
		BaseURL:      server.URL,
		APIKey:       "test-key",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatResponses,
		Enabled:      true,
		UseMock:      false,
	})
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Malformed Test", Description: "验证失败工作流记录"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "输入需求"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}

	_, err = service.GenerateClarifications(ctx, project.ID)
	if err == nil {
		t.Fatalf("expected GenerateClarifications to fail when provider returns malformed JSON")
	}

	runs, err := repo.ListWorkflowRuns(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListWorkflowRuns returned error: %v", err)
	}
	if len(runs) == 0 {
		t.Fatalf("expected workflow runs to be recorded")
	}
	if runs[0].Status != "failed" {
		t.Fatalf("expected workflow run to be marked failed, got %s", runs[0].Status)
	}
}

func TestBlackBox_RegenerateDownstreamArtifactsCreatesFreshVersions(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Regenerate Test", Description: "验证下游重生成"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "做一个 AI 工作台"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "先做单机版"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	workspace, err := service.RegenerateDownstreamArtifacts(ctx, RegenerateDownstreamArtifactsInput{
		ProjectID:          project.ID,
		SourceArtifactType: ArtifactTypePRD,
	})
	if err != nil {
		t.Fatalf("RegenerateDownstreamArtifacts returned error: %v", err)
	}
	if len(workspace.Artifacts) < 3 {
		t.Fatalf("expected downstream artifacts to be created, got %d", len(workspace.Artifacts))
	}
}

func TestUISchemaValidation(t *testing.T) {
	err := (UISchema{
		AppMeta: UIAppMeta{Name: "demo"},
		Routes:  []UIRoute{{Path: "/", PageID: "missing"}},
		Pages:   []UIPage{},
	}).Validate()
	if err == nil {
		t.Fatalf("expected ui schema validation to fail")
	}
}

func newTestWorkbench(t *testing.T) (*WorkbenchService, *Repository, string) {
	t.Helper()

	tempDir := t.TempDir()
	repo, err := NewRepository(filepath.Join(tempDir, "test.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	service := NewWorkbenchService(
		repo,
		NewAIManager(repo),
		NewPrototypeService(filepath.Join(tempDir, "prototype-exports")),
	)
	return service, repo, tempDir
}

func newConfiguredWorkbench(t *testing.T, config ProviderConfig) (*WorkbenchService, *Repository) {
	t.Helper()

	tempDir := t.TempDir()
	repo, err := NewRepository(filepath.Join(tempDir, "test.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	if _, err := repo.SaveProviderConfig(context.Background(), config); err != nil {
		t.Fatalf("SaveProviderConfig returned error: %v", err)
	}
	service := NewWorkbenchService(
		repo,
		NewAIManager(repo),
		NewPrototypeService(filepath.Join(tempDir, "prototype-exports")),
	)
	return service, repo
}
