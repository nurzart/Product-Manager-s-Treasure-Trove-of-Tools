package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlackBox_AppExposedWorkflow(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	if err := app.ensureReady(); err != nil {
		t.Fatalf("app failed to initialize: %v", err)
	}

	savedConfig, err := app.SaveProviderConfig(ProviderConfig{
		Name:         "UI Saved Provider",
		ProviderType: "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatChatCompletions,
		Enabled:      false,
		UseMock:      true,
	})
	if err != nil {
		t.Fatalf("SaveProviderConfig returned error: %v", err)
	}
	if savedConfig.APIFormat != AIAPIFormatChatCompletions {
		t.Fatalf("expected app to persist chat_completions selection, got %s", savedConfig.APIFormat)
	}

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Black Box Project",
		Description: "通过 App 暴露方法跑通闭环",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	if _, err := app.SaveRequirementInput(SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "需要一个本地优先的产品经理 AI 工具，可以从需求自动生成 PRD 和原型。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}

	requirement, err := app.GenerateClarifications(project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if len(requirement.Clarifications) == 0 {
		t.Fatalf("expected clarifications to be generated")
	}
	if _, err := app.GenerateConsensusBrief(project.ID); err != nil {
		t.Fatalf("GenerateConsensusBrief returned error: %v", err)
	}
	if _, err := app.BuildRequirementModel(project.ID); err != nil {
		t.Fatalf("BuildRequirementModel returned error: %v", err)
	}

	if _, err := app.GenerateArtifact(GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	if _, err := app.GenerateArtifact(GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := app.GenerateUISchema(project.ID); err != nil {
		t.Fatalf("GenerateUISchema returned error: %v", err)
	}

	bundle, err := app.ExportPrototype(ExportPrototypeInput{
		ProjectID: project.ID,
		OutputDir: filepath.Join(app.baseDir, "blackbox-export"),
	})
	if err != nil {
		t.Fatalf("ExportPrototype returned error: %v", err)
	}
	if bundle.ExportPath == "" {
		t.Fatalf("expected export path to be returned")
	}
}

func TestBlackBox_AppSerializesEmptyCollectionsAsArrays(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	projects, err := app.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects returned error: %v", err)
	}
	projectsJSON, err := json.Marshal(projects)
	if err != nil {
		t.Fatalf("json.Marshal(projects) returned error: %v", err)
	}
	if string(projectsJSON) != "[]" {
		t.Fatalf("expected empty projects to marshal as [], got %s", string(projectsJSON))
	}

	project, err := app.CreateProject(CreateProjectInput{Name: "Empty Workspace", Description: "check empty arrays"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}

	workspace, err := app.GetWorkspace(project.ID)
	if err != nil {
		t.Fatalf("GetWorkspace returned error: %v", err)
	}
	workspaceJSON, err := json.Marshal(workspace)
	if err != nil {
		t.Fatalf("json.Marshal(workspace) returned error: %v", err)
	}
	payload := string(workspaceJSON)
	if strings.Contains(payload, `"artifacts":null`) {
		t.Fatalf("expected artifacts to marshal as [], got %s", payload)
	}
	if strings.Contains(payload, `"workflowRuns":null`) {
		t.Fatalf("expected workflowRuns to marshal as [], got %s", payload)
	}
	if strings.Contains(payload, `"sourceMaterials":null`) {
		t.Fatalf("expected sourceMaterials to marshal as [], got %s", payload)
	}
}

func TestBlackBox_AppTruthLayerWorkflow(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Truth Layer App Flow",
		Description: "通过 App 暴露方法验证真相层",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeFreeText,
		Title:      "客户原始需求",
		RawText:    "要做一个产品经理全流程 AI 工作台。",
		IsPrimary:  true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	intake, err := app.BuildRequirementIntake(BuildRequirementIntakeInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("BuildRequirementIntake returned error: %v", err)
	}
	if intake.ID == "" {
		t.Fatalf("expected intake id")
	}

	if _, err := app.SaveRequirementInput(SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "补充说明：第一版先覆盖个人使用，不做审批流。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	if _, err := app.GenerateClarifications(project.ID); err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	brief, err := app.GenerateConsensusBrief(project.ID)
	if err != nil {
		t.Fatalf("GenerateConsensusBrief returned error: %v", err)
	}
	if brief.Structured.ProblemStatement == "" {
		t.Fatalf("expected problem statement")
	}
	model, err := app.BuildRequirementModel(project.ID)
	if err != nil {
		t.Fatalf("BuildRequirementModel returned error: %v", err)
	}
	if len(model.Goals) == 0 {
		t.Fatalf("expected goals in requirement model")
	}
}

func TestBlackBox_AppSupportsMultiSourceRequirementIntake(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "App Multi Source",
		Description: "验证多来源入口",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeFreeText,
		Title:      "客户原始需求",
		RawText:    "做一个产品经理 AI 工作台。",
		IsPrimary:  true,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial(primary) returned error: %v", err)
	}
	if _, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypeMeetingNotes,
		Title:      "会议纪要",
		RawText:    "讨论了澄清向导、多来源和共识稿。",
		IsPrimary:  false,
	}); err != nil {
		t.Fatalf("SaveSourceMaterial(meeting) returned error: %v", err)
	}

	intake, err := app.BuildRequirementIntake(BuildRequirementIntakeInput{ProjectID: project.ID})
	if err != nil {
		t.Fatalf("BuildRequirementIntake returned error: %v", err)
	}
	if intake.ID == "" || len(intake.SourceMaterialIDs) != 2 {
		t.Fatalf("expected intake with 2 source materials, got %+v", intake)
	}

	workspace, err := app.GetWorkspace(project.ID)
	if err != nil {
		t.Fatalf("GetWorkspace returned error: %v", err)
	}
	if len(workspace.SourceMaterials) != 2 {
		t.Fatalf("expected workspace to expose 2 source materials, got %d", len(workspace.SourceMaterials))
	}
}

func TestBlackBox_AppCanGenerateFunctionalSpecFromUploadedPRD(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Uploaded PRD App Flow",
		Description: "验证上传 PRD 后可直接生成功能规格",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	source, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户现有 PRD",
		RawText:    "# 客户 PRD\n\n包含业务背景、页面结构和验收条件。",
		IsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	version, err := app.GenerateArtifact(GenerateArtifactInput{
		ProjectID:    project.ID,
		ArtifactType: ArtifactTypeFunctionalSpec,
	})
	if err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if version.SourceVersionID != source.ID {
		t.Fatalf("expected source version id to point at uploaded PRD, got %s", version.SourceVersionID)
	}
}

func TestBlackBox_AppCanRepairPrototypePreview(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Prototype Repair App Flow",
		Description: "验证 App 暴露原型修复能力",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := app.SaveRequirementInput(SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "做一个用户治理后台，需要概览、用户列表、角色权限。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := app.GenerateClarifications(project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := app.AnswerClarifications(AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "主要给内部管理员使用。"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}
	if _, err := app.GenerateArtifact(GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}
	if _, err := app.GenerateArtifact(GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec}); err != nil {
		t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
	}
	if _, err := app.GenerateUISchema(project.ID); err != nil {
		t.Fatalf("GenerateUISchema returned error: %v", err)
	}

	bundle, err := app.RepairPrototype(PrototypeRepairInput{
		ProjectID:        project.ID,
		PageID:           "dashboard",
		CurrentIssue:     "菜单点击没有反应。",
		ExpectedBehavior: "点击菜单切换到对应页面。",
	})
	if err != nil {
		t.Fatalf("RepairPrototype returned error: %v", err)
	}
	if bundle.RepairReport == nil {
		t.Fatalf("expected repair report in repaired bundle")
	}
	if !bundle.RepairReport.Applied {
		t.Fatalf("expected repaired bundle to be marked as applied")
	}
}

func TestBlackBox_AppParsesUploadedPRDMaterial(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Parsed PRD App Flow",
		Description: "验证上传 PRD 会自动结构化解析",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	material, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户现有 PRD",
		RawText:    "# 背景\n需要统一需求链路。\n## 核心功能\n需求澄清、功能规格、原型。",
		IsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}
	if sourceMaterialSectionCount(material) == 0 {
		t.Fatalf("expected parsed source material sections, got %+v", material.ParsedContent)
	}
	if sourceMaterialPRDBaselineCompleteness(material) <= 0 {
		t.Fatalf("expected app layer to expose prd baseline completeness, got %+v", material.ParsedContent)
	}
	if material.ExtractionMeta["parser"] != "builtin-structure-parser" {
		t.Fatalf("expected parser metadata, got %+v", material.ExtractionMeta)
	}
}

func TestBlackBox_AppCanSaveEditablePRDBaseline(t *testing.T) {
	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	project, err := app.CreateProject(CreateProjectInput{
		Name:        "Editable PRD App Flow",
		Description: "验证 App 暴露的标准章节保存能力",
	})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	material, err := app.SaveSourceMaterial(SaveSourceMaterialInput{
		ProjectID:  project.ID,
		SourceType: SourceMaterialTypePRDUpload,
		Title:      "客户现有 PRD",
		RawText:    "# 背景\n当前文档需要标准化。",
		IsPrimary:  true,
	})
	if err != nil {
		t.Fatalf("SaveSourceMaterial returned error: %v", err)
	}

	updated, err := app.SavePRDBaseline(SavePRDBaselineInput{
		SourceMaterialID: material.ID,
		Sections: []PRDBaselineSectionInput{
			{Key: "goals", Content: "20 分钟内完成 PRD 标准化。"},
			{Key: "acceptance", Content: "验收要覆盖字段、状态与异常流。"},
		},
	})
	if err != nil {
		t.Fatalf("SavePRDBaseline returned error: %v", err)
	}
	if sourceMaterialPRDBaselineCompleteness(updated) <= sourceMaterialPRDBaselineCompleteness(material) {
		t.Fatalf("expected completeness to improve after app baseline save, before=%d after=%d", sourceMaterialPRDBaselineCompleteness(material), sourceMaterialPRDBaselineCompleteness(updated))
	}
}

func TestBlackBox_AppCanProbeProviderConfig(t *testing.T) {
	var rootAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models", "/chat/completions":
			rootAttempts++
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>not api</html>"))
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "gpt-5.4"},
				},
			})
		case "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"role":    "assistant",
							"content": "OK",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := NewAppWithBaseDir(t.TempDir())
	defer app.shutdown(app.context())

	app.workbench.aiManager.httpClient = server.Client()

	result, err := app.TestProviderConfig(ProviderConfig{
		Name:         "Probe Provider",
		ProviderType: "openai-compatible",
		BaseURL:      server.URL,
		APIKey:       "test-key",
		Model:        "gpt-5.4",
		APIFormat:    AIAPIFormatChatCompletions,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("TestProviderConfig returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected provider probe to succeed, got %+v", result)
	}
	if rootAttempts == 0 {
		t.Fatalf("expected root endpoints to be attempted before /v1 fallback")
	}
	if result.ResolvedBaseURL != server.URL+"/v1" {
		t.Fatalf("expected resolved base url to include /v1, got %s", result.ResolvedBaseURL)
	}
}
