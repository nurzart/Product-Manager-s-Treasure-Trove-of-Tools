package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx             context.Context
	baseDir         string
	baseDirOverride string
	repo            *Repository
	workbench       *WorkbenchService
	initErr         error
}

func NewApp() *App {
	app := &App{}
	app.initErr = app.bootstrap()
	return app
}

func NewAppWithBaseDir(baseDir string) *App {
	app := &App{baseDirOverride: baseDir}
	app.initErr = app.bootstrap()
	return app
}

func (a *App) bootstrap() error {
	baseDir := a.baseDirOverride
	if baseDir == "" {
		var err error
		baseDir, err = resolveAppDir()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	repo, err := NewRepository(filepath.Join(baseDir, "workspace.sqlite"))
	if err != nil {
		return err
	}
	exportsDir := filepath.Join(baseDir, "exports")
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return err
	}
	aiManager := NewAIManager(repo)
	prototypeService := NewPrototypeService(exportsDir)

	a.baseDir = baseDir
	a.repo = repo
	a.workbench = NewWorkbenchService(repo, aiManager, prototypeService)
	return nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(context.Context) {
	if a.repo != nil {
		_ = a.repo.Close()
	}
}

func (a *App) ensureReady() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.workbench == nil {
		return fmt.Errorf("application services are not initialized")
	}
	return nil
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) AppInfo() map[string]string {
	return map[string]string{
		"storageDir": a.baseDir,
		"mode":       "local-first",
	}
}

func (a *App) ListProjects() ([]Project, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	projects, err := a.workbench.ListProjects(a.context())
	if err != nil {
		return nil, err
	}
	return normalizeProjects(projects), nil
}

func (a *App) CreateProject(input CreateProjectInput) (Project, error) {
	if err := a.ensureReady(); err != nil {
		return Project{}, err
	}
	return a.workbench.CreateProject(a.context(), input)
}

func (a *App) GetWorkspace(projectID string) (WorkspaceSnapshot, error) {
	if err := a.ensureReady(); err != nil {
		return WorkspaceSnapshot{}, err
	}
	workspace, err := a.workbench.GetWorkspace(a.context(), projectID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return normalizeWorkspaceSnapshot(workspace), nil
}

func (a *App) SaveRequirementInput(input SaveRequirementInputInput) (RequirementInput, error) {
	if err := a.ensureReady(); err != nil {
		return RequirementInput{}, err
	}
	requirement, err := a.workbench.SaveRequirementInput(a.context(), input)
	if err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (a *App) SaveSourceMaterial(input SaveSourceMaterialInput) (SourceMaterial, error) {
	if err := a.ensureReady(); err != nil {
		return SourceMaterial{}, err
	}
	material, err := a.workbench.SaveSourceMaterial(a.context(), input)
	if err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func (a *App) SavePRDBaseline(input SavePRDBaselineInput) (SourceMaterial, error) {
	if err := a.ensureReady(); err != nil {
		return SourceMaterial{}, err
	}
	material, err := a.workbench.SavePRDBaseline(a.context(), input)
	if err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func (a *App) ImportSourceMaterialFromFile(input ImportSourceMaterialFromFileInput) (SourceMaterial, error) {
	if err := a.ensureReady(); err != nil {
		return SourceMaterial{}, err
	}
	material, err := a.workbench.ImportSourceMaterialFromFile(a.context(), input)
	if err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func (a *App) ChooseAndImportSourceMaterial(input ImportSourceMaterialFromFileInput) (SourceMaterial, error) {
	if err := a.ensureReady(); err != nil {
		return SourceMaterial{}, err
	}
	if a.ctx == nil {
		return SourceMaterial{}, errors.New("desktop runtime is not ready")
	}

	defaultDir := ""
	if strings.TrimSpace(input.FilePath) != "" {
		defaultDir = filepath.Dir(input.FilePath)
	} else if a.baseDir != "" {
		defaultDir = a.baseDir
	}

	filePath, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{
		Title:            "选择来源材料文件",
		DefaultDirectory: defaultDir,
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Supported Documents",
				Pattern:     "*.txt;*.md;*.markdown;*.json;*.csv;*.docx;*.pdf",
			},
		},
	})
	if err != nil {
		return SourceMaterial{}, err
	}
	if strings.TrimSpace(filePath) == "" {
		return SourceMaterial{}, errors.New("file import cancelled")
	}

	input.FilePath = filePath
	return a.ImportSourceMaterialFromFile(input)
}

func (a *App) ListSourceMaterials(projectID string) ([]SourceMaterial, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	materials, err := a.workbench.ListSourceMaterials(a.context(), projectID)
	if err != nil {
		return nil, err
	}
	return normalizeSourceMaterials(materials), nil
}

func (a *App) BuildRequirementIntake(input BuildRequirementIntakeInput) (RequirementIntake, error) {
	if err := a.ensureReady(); err != nil {
		return RequirementIntake{}, err
	}
	intake, err := a.workbench.BuildRequirementIntake(a.context(), input)
	if err != nil {
		return RequirementIntake{}, err
	}
	return normalizeRequirementIntake(intake), nil
}

func (a *App) GenerateClarifications(projectID string) (RequirementInput, error) {
	if err := a.ensureReady(); err != nil {
		return RequirementInput{}, err
	}
	requirement, err := a.workbench.GenerateClarifications(a.context(), projectID)
	if err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (a *App) GenerateConsensusBrief(projectID string) (ConsensusBrief, error) {
	if err := a.ensureReady(); err != nil {
		return ConsensusBrief{}, err
	}
	brief, err := a.workbench.GenerateConsensusBrief(a.context(), projectID)
	if err != nil {
		return ConsensusBrief{}, err
	}
	return normalizeConsensusBrief(brief), nil
}

func (a *App) BuildRequirementModel(projectID string) (RequirementModel, error) {
	if err := a.ensureReady(); err != nil {
		return RequirementModel{}, err
	}
	model, err := a.workbench.BuildRequirementModel(a.context(), projectID)
	if err != nil {
		return RequirementModel{}, err
	}
	return normalizeRequirementModel(model), nil
}

func (a *App) AnswerClarifications(input AnswerClarificationsInput) (RequirementInput, error) {
	if err := a.ensureReady(); err != nil {
		return RequirementInput{}, err
	}
	requirement, err := a.workbench.AnswerClarifications(a.context(), input)
	if err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (a *App) GenerateArtifact(input GenerateArtifactInput) (ArtifactVersion, error) {
	if err := a.ensureReady(); err != nil {
		return ArtifactVersion{}, err
	}
	return a.workbench.GenerateArtifact(a.context(), input)
}

func (a *App) UpdateArtifactSection(input UpdateArtifactSectionInput) (ArtifactVersion, error) {
	if err := a.ensureReady(); err != nil {
		return ArtifactVersion{}, err
	}
	return a.workbench.UpdateArtifactSection(a.context(), input)
}

func (a *App) ApproveArtifactVersion(input ApproveArtifactVersionInput) (ArtifactVersion, error) {
	if err := a.ensureReady(); err != nil {
		return ArtifactVersion{}, err
	}
	return a.workbench.ApproveArtifactVersion(a.context(), input)
}

func (a *App) RegenerateDownstreamArtifacts(input RegenerateDownstreamArtifactsInput) (WorkspaceSnapshot, error) {
	if err := a.ensureReady(); err != nil {
		return WorkspaceSnapshot{}, err
	}
	return a.workbench.RegenerateDownstreamArtifacts(a.context(), input)
}

func (a *App) GenerateUISchema(projectID string) (ArtifactVersion, error) {
	if err := a.ensureReady(); err != nil {
		return ArtifactVersion{}, err
	}
	return a.workbench.GenerateUISchema(a.context(), projectID)
}

func (a *App) GeneratePrototypeBundle(projectID string) (PrototypeBundle, error) {
	if err := a.ensureReady(); err != nil {
		return PrototypeBundle{}, err
	}
	bundle, err := a.workbench.GeneratePrototypeBundle(a.context(), projectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	return normalizePrototypeBundle(bundle), nil
}

func (a *App) PreviewPrototype(projectID string) (PrototypeBundle, error) {
	if err := a.ensureReady(); err != nil {
		return PrototypeBundle{}, err
	}
	bundle, err := a.workbench.PreviewPrototype(a.context(), projectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	return normalizePrototypeBundle(bundle), nil
}

func (a *App) RepairPrototype(input PrototypeRepairInput) (PrototypeBundle, error) {
	if err := a.ensureReady(); err != nil {
		return PrototypeBundle{}, err
	}
	bundle, err := a.workbench.RepairPrototype(a.context(), input)
	if err != nil {
		return PrototypeBundle{}, err
	}
	return normalizePrototypeBundle(bundle), nil
}

func (a *App) ExportPrototype(input ExportPrototypeInput) (PrototypeBundle, error) {
	if err := a.ensureReady(); err != nil {
		return PrototypeBundle{}, err
	}
	bundle, err := a.workbench.ExportPrototype(a.context(), input)
	if err != nil {
		return PrototypeBundle{}, err
	}
	return normalizePrototypeBundle(bundle), nil
}

func (a *App) GetProviderConfig() (ProviderConfig, error) {
	if err := a.ensureReady(); err != nil {
		return ProviderConfig{}, err
	}
	return a.workbench.GetProviderConfig(a.context())
}

func (a *App) SaveProviderConfig(config ProviderConfig) (ProviderConfig, error) {
	if err := a.ensureReady(); err != nil {
		return ProviderConfig{}, err
	}
	return a.workbench.SaveProviderConfig(a.context(), config)
}

func (a *App) TestProviderConfig(config ProviderConfig) (ProviderConnectionResult, error) {
	if err := a.ensureReady(); err != nil {
		return ProviderConnectionResult{}, err
	}
	return a.workbench.TestProviderConfig(a.context(), config)
}

func resolveAppDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", err
		}
		return filepath.Join(cwd, ".pm-ai-workbench"), nil
	}
	return filepath.Join(configDir, "pm-ai-workbench"), nil
}
