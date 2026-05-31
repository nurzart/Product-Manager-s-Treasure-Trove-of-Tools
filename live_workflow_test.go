package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLive_OpenAICompatibleWorkflow(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("LIVE_OPENAI_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("LIVE_OPENAI_API_KEY"))
	model := strings.TrimSpace(os.Getenv("LIVE_OPENAI_MODEL"))
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("set LIVE_OPENAI_BASE_URL, LIVE_OPENAI_API_KEY, and LIVE_OPENAI_MODEL to run live workflow tests")
	}

	formats := []AIAPIFormat{AIAPIFormatResponses, AIAPIFormatChatCompletions}
	for _, format := range formats {
		t.Run(string(format), func(t *testing.T) {
			ctx := context.Background()
			service, repo, tempDir := newTestWorkbench(t)
			defer func() {
				_ = repo.Close()
			}()

			_, err := repo.SaveProviderConfig(ctx, ProviderConfig{
				Name:         "Live Provider",
				ProviderType: "openai-compatible",
				BaseURL:      baseURL,
				APIKey:       apiKey,
				Model:        model,
				APIFormat:    format,
				Enabled:      true,
				UseMock:      false,
			})
			if err != nil {
				t.Fatalf("SaveProviderConfig returned error: %v", err)
			}
			t.Logf("provider configured with format=%s model=%s", format, model)

			project, err := service.CreateProject(ctx, CreateProjectInput{
				Name:        "Live API Workflow",
				Description: "validate clarifications, artifacts and prototype generation against a real provider",
			})
			if err != nil {
				t.Fatalf("CreateProject returned error: %v", err)
			}
			t.Log("project created")

			_, err = service.SaveRequirementInput(ctx, SaveRequirementInputInput{
				ProjectID: project.ID,
				RawInput:  "做一个本地优先的产品经理 AI 工作台，用户输入客户的模糊需求后，先生成澄清问题，再生成 PRD、功能规格、研发说明，最后生成可预览和可导出的 React 原型脚手架。",
			})
			if err != nil {
				t.Fatalf("SaveRequirementInput returned error: %v", err)
			}
			t.Log("requirement saved")

			requirement, err := service.GenerateClarifications(ctx, project.ID)
			if err != nil {
				t.Fatalf("GenerateClarifications returned error: %v", err)
			}
			if len(requirement.Clarifications) == 0 {
				t.Fatalf("expected clarifications to be generated")
			}
			t.Logf("clarifications generated: %d", len(requirement.Clarifications))

			answers := make([]ClarificationAnswer, 0, len(requirement.Clarifications))
			for _, question := range requirement.Clarifications {
				answers = append(answers, ClarificationAnswer{
					QuestionID: question.ID,
					Answer:     "第一版重点验证需求澄清、文档生成、原型导出三段闭环，先不做多人协作。",
				})
			}
			requirement, err = service.AnswerClarifications(ctx, AnswerClarificationsInput{
				ProjectID: project.ID,
				Answers:   answers,
			})
			if err != nil {
				t.Fatalf("AnswerClarifications returned error: %v", err)
			}
			if len(requirement.Answers) == 0 {
				t.Fatalf("expected clarification answers to be persisted")
			}
			t.Logf("clarifications answered: %d", len(requirement.Answers))

			prdVersion, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD})
			if err != nil {
				t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
			}
			if prdVersion.Type != ArtifactTypePRD {
				t.Fatalf("expected PRD version, got %s", prdVersion.Type)
			}
			t.Log("PRD generated")

			functionalVersion, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeFunctionalSpec})
			if err != nil {
				t.Fatalf("GenerateArtifact(FunctionalSpec) returned error: %v", err)
			}
			if functionalVersion.Type != ArtifactTypeFunctionalSpec {
				t.Fatalf("expected FunctionalSpec version, got %s", functionalVersion.Type)
			}
			t.Log("FunctionalSpec generated")

			technicalVersion, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypeTechnicalSpec})
			if err != nil {
				t.Fatalf("GenerateArtifact(TechnicalSpec) returned error: %v", err)
			}
			if technicalVersion.Type != ArtifactTypeTechnicalSpec {
				t.Fatalf("expected TechnicalSpec version, got %s", technicalVersion.Type)
			}
			t.Log("TechnicalSpec generated")

			uiSchemaVersion, err := service.GenerateUISchema(ctx, project.ID)
			if err != nil {
				t.Fatalf("GenerateUISchema returned error: %v", err)
			}
			if uiSchemaVersion.Type != ArtifactTypePrototype {
				t.Fatalf("expected PrototypeSource version, got %s", uiSchemaVersion.Type)
			}
			t.Log("UISchema generated")

			preview, err := service.PreviewPrototype(ctx, project.ID)
			if err != nil {
				t.Fatalf("PreviewPrototype returned error: %v", err)
			}
			if len(preview.UISchema.Routes) == 0 {
				t.Fatalf("expected preview to contain routes")
			}
			if len(preview.Files) == 0 {
				t.Fatalf("expected preview bundle to contain generated files")
			}
			t.Logf("prototype preview generated: %d files", len(preview.Files))

			exported, err := service.ExportPrototype(ctx, ExportPrototypeInput{
				ProjectID: project.ID,
				OutputDir: filepath.Join(tempDir, "live-export"),
			})
			if err != nil {
				t.Fatalf("ExportPrototype returned error: %v", err)
			}
			if exported.ExportPath == "" {
				t.Fatalf("expected export path to be populated")
			}
			t.Log("prototype exported")

			workspace, err := service.GetWorkspace(ctx, project.ID)
			if err != nil {
				t.Fatalf("GetWorkspace returned error: %v", err)
			}
			if len(workspace.Artifacts) < 4 {
				t.Fatalf("expected all artifact bundles to exist, got %d", len(workspace.Artifacts))
			}
			t.Logf("workspace artifacts verified: %d bundles", len(workspace.Artifacts))
		})
	}
}
