package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestGrayBox_MigrationCreatesTruthLayerTables(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "workspace.sqlite"))
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	defer func() {
		_ = repo.Close()
	}()

	rows, err := repo.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	defer rows.Close()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("rows.Scan returned error: %v", err)
		}
		names[name] = true
	}

	for _, tableName := range []string{"source_materials", "requirement_intakes", "consensus_briefs", "requirement_models"} {
		if !names[tableName] {
			t.Fatalf("expected table %s to be created", tableName)
		}
	}
}

func TestGrayBox_MigrationAddsProviderAPIFormatColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	_, err = db.Exec(`
		CREATE TABLE provider_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			model TEXT NOT NULL,
			enabled INTEGER NOT NULL,
			use_mock INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create legacy schema: %v", err)
	}
	_ = db.Close()

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}
	defer func() {
		_ = repo.Close()
	}()

	config, err := repo.SaveProviderConfig(context.Background(), ProviderConfig{
		Name:         "Migrated Provider",
		ProviderType: "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatResponses,
		Enabled:      true,
		UseMock:      false,
	})
	if err != nil {
		t.Fatalf("SaveProviderConfig returned error after migration: %v", err)
	}
	if config.APIFormat != AIAPIFormatResponses {
		t.Fatalf("expected API format to persist after migration, got %s", config.APIFormat)
	}
}

func TestGrayBox_SaveRequirementInputMarksArtifactsStale(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Stale Requirement", Description: "save requirement stale test"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "初始需求"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "先支持桌面端"},
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

	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "更新后的需求，增加更多边界条件"}); err != nil {
		t.Fatalf("SaveRequirementInput(second) returned error: %v", err)
	}

	prdVersion, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypePRD)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion(PRD) returned error: %v", err)
	}
	functionalVersion, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypeFunctionalSpec)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion(FunctionalSpec) returned error: %v", err)
	}
	if prdVersion.Status != VersionStatusStale {
		t.Fatalf("expected PRD to be stale after upstream requirement change, got %s", prdVersion.Status)
	}
	if functionalVersion.Status != VersionStatusStale {
		t.Fatalf("expected FunctionalSpec to be stale after upstream requirement change, got %s", functionalVersion.Status)
	}
}

func TestGrayBox_SaveRequirementInputSynchronizesTruthLayer(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Truth Sync", Description: "验证同步"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{
		ProjectID: project.ID,
		RawInput:  "客户要一款产品经理全能工具，覆盖需求、文档和原型。",
	}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}

	materials, err := repo.ListSourceMaterials(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListSourceMaterials returned error: %v", err)
	}
	if len(materials) != 1 {
		t.Fatalf("expected one synchronized source material, got %d", len(materials))
	}
	if !materials[0].IsPrimary || materials[0].SourceType != SourceMaterialTypeFreeText {
		t.Fatalf("unexpected synchronized material: %+v", materials[0])
	}

	intake, err := repo.GetLatestRequirementIntake(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetLatestRequirementIntake returned error: %v", err)
	}
	if intake.ProjectID != project.ID {
		t.Fatalf("expected intake to belong to project")
	}
	if !strings.Contains(intake.Summary, "已归一化 1 份材料") {
		t.Fatalf("unexpected intake summary: %s", intake.Summary)
	}
	if !strings.Contains(mustJSON(intake.NormalizedInput), "产品经理全能工具") {
		t.Fatalf("expected normalized intake to retain original requirement text, got %+v", intake.NormalizedInput)
	}

	payload, err := json.Marshal(intake)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	if strings.Contains(string(payload), `"sourceMaterialIds":null`) {
		t.Fatalf("expected sourceMaterialIds to serialize as []")
	}
}

func TestGrayBox_BuildRequirementModelMarksArtifactsStale(t *testing.T) {
	ctx := context.Background()
	service, repo, _ := newTestWorkbench(t)
	defer func() {
		_ = repo.Close()
	}()

	project, err := service.CreateProject(ctx, CreateProjectInput{Name: "Model Stale", Description: "验证模型更新"})
	if err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	if _, err := service.SaveRequirementInput(ctx, SaveRequirementInputInput{ProjectID: project.ID, RawInput: "先做需求共识和真相层"}); err != nil {
		t.Fatalf("SaveRequirementInput returned error: %v", err)
	}
	requirement, err := service.GenerateClarifications(ctx, project.ID)
	if err != nil {
		t.Fatalf("GenerateClarifications returned error: %v", err)
	}
	if _, err := service.AnswerClarifications(ctx, AnswerClarificationsInput{
		ProjectID: project.ID,
		Answers: []ClarificationAnswer{
			{QuestionID: requirement.Clarifications[0].ID, Answer: "个人版优先"},
		},
	}); err != nil {
		t.Fatalf("AnswerClarifications returned error: %v", err)
	}
	if _, err := service.GenerateConsensusBrief(ctx, project.ID); err != nil {
		t.Fatalf("GenerateConsensusBrief returned error: %v", err)
	}
	if _, err := service.BuildRequirementModel(ctx, project.ID); err != nil {
		t.Fatalf("BuildRequirementModel returned error: %v", err)
	}
	if _, err := service.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: project.ID, ArtifactType: ArtifactTypePRD}); err != nil {
		t.Fatalf("GenerateArtifact(PRD) returned error: %v", err)
	}

	if _, err := service.BuildRequirementModel(ctx, project.ID); err != nil {
		t.Fatalf("BuildRequirementModel(second) returned error: %v", err)
	}

	prdVersion, err := repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypePRD)
	if err != nil {
		t.Fatalf("GetLatestArtifactVersion returned error: %v", err)
	}
	if prdVersion.Status != VersionStatusStale {
		t.Fatalf("expected PRD to be stale after requirement model rebuild, got %s", prdVersion.Status)
	}
}
