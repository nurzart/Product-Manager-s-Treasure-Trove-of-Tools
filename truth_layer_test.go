package main

import (
	"context"
	"strings"
	"testing"
)

func TestParseConsensusBriefJSON_CoercesFlexibleFieldNames(t *testing.T) {
	data, err := parseConsensusBriefJSON(`{
		"problemStatement":"需要把模糊需求转成稳定的产品资产",
		"targetUsers":["产品经理","业务负责人"],
		"goals":["缩短文档产出周期"],
		"metrics":["PRD 初稿 20 分钟内完成"],
		"coreScenarios":["澄清需求","生成 PRD"],
		"inScopeItems":["需求澄清","需求共识稿"],
		"outOfScopeItems":["审批流"],
		"risks":["模型输出不稳定"],
		"dependencies":["OpenAI-compatible API"]
	}`)
	if err != nil {
		t.Fatalf("parseConsensusBriefJSON returned error: %v", err)
	}
	if data.ProblemStatement == "" {
		t.Fatalf("expected problem statement to be parsed")
	}
	if len(data.BusinessGoals) != 1 || data.BusinessGoals[0] != "缩短文档产出周期" {
		t.Fatalf("unexpected business goals: %+v", data.BusinessGoals)
	}
	if len(data.InScopeItems) != 2 {
		t.Fatalf("expected in-scope items to be parsed, got %+v", data.InScopeItems)
	}
}

func TestParseRequirementModelJSON_CoercesAlternativeShapes(t *testing.T) {
	model, err := parseRequirementModelJSON(`{
		"problemDefinition":{"summary":"做一条稳定需求链路","outcome":"让 PRD、规格和原型统一生长"},
		"actors":[{"role":"产品经理","responsibilities":["输入需求","审阅结果"]}],
		"businessGoals":["统一需求真相层"],
		"journeys":[{"title":"需求收敛","steps":["输入","澄清","共识"]}],
		"scope":{"inScope":["需求澄清","PRD"],"outScope":["知识库"]},
		"domainEntities":[{"title":"RequirementModel","fields":["problem_definition","goals"]}],
		"businessRules":[{"title":"traceability","content":"所有下游产物必须可追溯"}],
		"acceptance":["可以稳定生成 RequirementModel"],
		"technicalConstraints":["本地优先"]
	}`)
	if err != nil {
		t.Fatalf("parseRequirementModelJSON returned error: %v", err)
	}
	if model.ProblemDefinition.Summary == "" {
		t.Fatalf("expected problem definition summary to be parsed")
	}
	if len(model.Actors) != 1 || model.Actors[0].Name != "产品经理" {
		t.Fatalf("unexpected actors: %+v", model.Actors)
	}
	if len(model.Scope.In) == 0 {
		t.Fatalf("expected scope.in to be coerced")
	}
	if len(model.Entities) != 1 || model.Entities[0].Name != "RequirementModel" {
		t.Fatalf("unexpected entities: %+v", model.Entities)
	}
}

func TestIntegration_TruthLayerWorkflow_Mock(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "Truth Layer Project",
		Description: "验证真相层闭环",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "做一个产品经理全能工具，先澄清，再出共识稿、PRD、规格、技术说明和原型。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}

	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if len(requirement.Clarifications) == 0 {
		t.Fatalf("expected clarifications")
	}

	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "第一版先支持个人产品经理工作台"},
			{QuestionID: requirement.Clarifications[1].ID, Answer: "审批流先不做"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}

	intake, err := service.BuildRequirementIntake(ctx, BuildRequirementIntakeInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("BuildRequirementIntake returned error: %v", err)
	}
	if intake.ReadinessScore <= 0 {
		t.Fatalf("expected readiness score to be calculated, got %d", intake.ReadinessScore)
	}
	if !strings.Contains(mustJSON(intake.NormalizedInput), "combined_text") {
		t.Fatalf("expected normalized input to include combined_text")
	}

	brief, err := service.GenerateConsensusBrief(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateConsensusBrief returned error: %v", err)
	}
	if brief.Structured.ProblemStatement == "" {
		t.Fatalf("expected consensus brief problem statement")
	}
	if len(brief.Structured.InScopeItems) == 0 {
		t.Fatalf("expected consensus brief scope")
	}

	model, err := service.BuildRequirementModel(ctx, project.ID)
	if err != nil {
		t.Fatalf("BuildRequirementModel returned error: %v", err)
	}
	if len(model.Actors) == 0 || len(model.Goals) == 0 {
		t.Fatalf("expected requirement model core fields, got %+v", model)
	}
	if model.Traceability["consensusBriefId"] == "" {
		t.Fatalf("expected traceability to record consensus brief id")
	}

	workspace, err := service.GetWorkspace(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetWorkspace returned error: %v", err)
	}
	if len(workspace.SourceMaterials) == 0 {
		t.Fatalf("expected source materials in workspace")
	}
	if workspace.RequirementIntake == nil || workspace.ConsensusBrief == nil || workspace.RequirementModel == nil {
		t.Fatalf("expected truth layer objects in workspace, got %+v", workspace)
	}
}

func TestIntegration_MultiSourceRequirementIntake_MergesMaterials(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "Multi Source Project",
		Description: "验证多来源材料归一化",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeFreeText,
		Title:      "客户原始想法",
		RawText:    "想做一个产品经理全链路 AI 工作台。",
		IsPrimary:  true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial(primary) returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeMeetingNotes,
		Title:      "4 月 11 日会议纪要",
		RawText:    "重点讨论了澄清流程、共识稿和 PRD 的衔接。",
		IsPrimary:  false,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial(meeting) returned error: %v", err)
	}
	if _, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "历史 PRD 草稿",
		RawText:    "# 背景\n已有一些页面结构和角色假设。\n## 核心页面\n需求页、PRD 页、原型页。",
		IsPrimary:  false,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial(prd) returned error: %v", err)
	}

	intake, err := service.BuildRequirementIntake(ctx, BuildRequirementIntakeInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("BuildRequirementIntake returned error: %v", err)
	}
	if len(intake.SourceMaterialIDs) != 3 {
		t.Fatalf("expected 3 source materials in intake, got %d", len(intake.SourceMaterialIDs))
	}
	if intake.ReadinessScore < 50 {
		t.Fatalf("expected multi-source intake to have a meaningful readiness score, got %d", intake.ReadinessScore)
	}
	normalizedJSON := mustJSON(intake.NormalizedInput)
	if !strings.Contains(normalizedJSON, "4 月 11 日会议纪要") || !strings.Contains(normalizedJSON, "历史 PRD 草稿") {
		t.Fatalf("expected normalized input to contain merged source titles, got %s", normalizedJSON)
	}
	if !strings.Contains(normalizedJSON, "parsed_material_count") || !strings.Contains(normalizedJSON, "total_section_count") {
		t.Fatalf("expected normalized input to expose parsed material metadata, got %s", normalizedJSON)
	}
	if !strings.Contains(normalizedJSON, "prd_baselines") || !strings.Contains(normalizedJSON, "completeness") {
		t.Fatalf("expected normalized input to expose prd baseline metadata, got %s", normalizedJSON)
	}
	materials, err := service.ListSourceMaterials(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListSourceMaterials returned error: %v", err)
	}
	var prdMaterial SourceMaterial
	for _, material := range materials {
		if material.SourceType == SourceMaterialTypePRDUpload {
			prdMaterial = material
			break
		}
	}
	if sourceMaterialSectionCount(prdMaterial) == 0 {
		t.Fatalf("expected uploaded prd material to be parsed into sections, got %+v", prdMaterial.ParsedContent)
	}
	if sourceMaterialPRDBaselineCompleteness(prdMaterial) <= 0 {
		t.Fatalf("expected uploaded prd material to expose baseline completeness, got %+v", prdMaterial.ParsedContent)
	}
}

func TestIntegration_SavePRDBaseline_RebuildsIntakeAndMarksArtifactsStale(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{
		Name:        "Editable PRD Baseline",
		Description: "验证保存标准章节后会重建 intake 并标记下游失效",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	source, err := service.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户 PRD 草稿",
		RawText: strings.Join([]string{
			"# 背景与问题",
			"当前 PRD 结构不稳定。",
			"## 核心功能",
			"需要支持澄清和 PRD 生成。",
		}, "\n"),
		IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	originalCompleteness := sourceMaterialPRDBaselineCompleteness(source)
	if originalCompleteness <= 0 {
		t.Fatalf("expected original completeness to be calculated, got %d", originalCompleteness)
	}

	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}

	saved, err := service.SavePRDBaseline(ctx, SavePRDBaselineInput{
		SourceMaterialID: source.ID,
		Sections: []PRDBaselineSectionInput{
			{Key: "goals", Content: "一小时内完成需求标准化与功能规格初稿。"},
			{Key: "acceptance", Content: "验收时需要覆盖字段规则、状态和异常场景。"},
		},
	})
	if err != nil {
		t.Fatalf("SavePRDBaseline returned error: %v", err)
	}
	if sourceMaterialPRDBaselineCompleteness(saved) <= originalCompleteness {
		t.Fatalf("expected completeness to increase after manual baseline save, before=%d after=%d", originalCompleteness, sourceMaterialPRDBaselineCompleteness(saved))
	}
	if saved.ExtractionMeta["baseline_editor"] != "manual-review" {
		t.Fatalf("expected baseline editor metadata, got %+v", saved.ExtractionMeta)
	}

	intake, err := repo.GetLatestRequirementIntake(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetLatestRequirementIntake returned error: %v", err)
	}
	if !strings.Contains(mustJSON(intake.NormalizedInput), "验收与交付") {
		t.Fatalf("expected rebuilt intake to contain updated baseline, got %s", mustJSON(intake.NormalizedInput))
	}

	prdVersion, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypePRD)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion returned error: %v", err)
	}
	if prdVersion.Status != VersionStatusStale {
		t.Fatalf("expected generated artifacts to be marked stale after baseline edit, got %s", prdVersion.Status)
	}
}
