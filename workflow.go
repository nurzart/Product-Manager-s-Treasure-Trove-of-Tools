package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type WorkbenchService struct {
	repo      *Repository
	aiManager *AIManager
	prototype *PrototypeService
}

func NewWorkbenchService(repo *Repository, aiManager *AIManager, prototype *PrototypeService) *WorkbenchService {
	return &WorkbenchService{
		repo:      repo,
		aiManager: aiManager,
		prototype: prototype,
	}
}

func (s *WorkbenchService) CreateProject(ctx context.Context, input CreateProjectInput) (Project, error) {
	return s.repo.CreateProject(ctx, input)
}

func (s *WorkbenchService) ListProjects(ctx context.Context) ([]Project, error) {
	projects, err := s.repo.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	return normalizeProjects(projects), nil
}

func (s *WorkbenchService) GetWorkspace(ctx context.Context, projectID string) (WorkspaceSnapshot, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	requirement, err := s.repo.GetRequirementInputByProject(ctx, projectID)
	var requirementPtr *RequirementInput
	if err == nil {
		requirementPtr = &requirement
	} else if !errors.Is(err, sql.ErrNoRows) {
		return WorkspaceSnapshot{}, err
	}
	artifacts, err := s.repo.ListArtifactBundles(ctx, projectID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	sourceMaterials, intake, consensusBrief, requirementModel, err := s.repo.LatestTruthLayer(ctx, projectID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs, err := s.repo.ListWorkflowRuns(ctx, projectID)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return normalizeWorkspaceSnapshot(WorkspaceSnapshot{
		Project:           project,
		RequirementInput:  requirementPtr,
		SourceMaterials:   sourceMaterials,
		RequirementIntake: intake,
		ConsensusBrief:    consensusBrief,
		RequirementModel:  requirementModel,
		Artifacts:         artifacts,
		WorkflowRuns:      runs,
	}), nil
}

func (s *WorkbenchService) SaveRequirementInput(ctx context.Context, input SaveRequirementInputInput) (RequirementInput, error) {
	requirement, err := s.repo.SaveRequirementInput(ctx, input)
	if err != nil {
		return RequirementInput{}, err
	}
	if _, err := s.repo.UpsertAutoRequirementSourceMaterial(ctx, input.ProjectID, requirement.RawInput); err != nil {
		return RequirementInput{}, err
	}
	if _, err := s.BuildRequirementIntake(ctx, BuildRequirementIntakeInput{ProjectID: input.ProjectID}); err != nil {
		return RequirementInput{}, err
	}
	if err := s.repo.MarkAllArtifactsStale(ctx, input.ProjectID); err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (s *WorkbenchService) GenerateClarifications(ctx context.Context, projectID string) (RequirementInput, error) {
	project, requirement, err := s.loadProjectAndRequirement(ctx, projectID)
	if err != nil {
		return RequirementInput{}, err
	}

	contextPayload := map[string]any{
		"project_name":        project.Name,
		"project_description": project.Description,
		"raw_input":           requirement.RawInput,
	}
	prompt := strings.Join([]string{
		"请基于以下 CONTEXT_JSON，输出一个包含 summary 与 questions 的 JSON 对象。",
		"summary 需提炼产品名、目标用户、业务目标、核心场景、约束和待确认点。",
		"questions 需给出 3-7 个高价值澄清问题，每个问题包含 id、question、rationale、priority。",
		mustJSON(contextPayload),
	}, "\n")

	run, err := s.repo.CreateWorkflowRun(ctx, projectID, "GenerateClarifications", mustJSON(contextPayload))
	if err != nil {
		return RequirementInput{}, err
	}

	provider := s.aiManager.Provider(ctx)
	payload, err := provider.GenerateStructured(ctx, "clarifications", prompt)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", "{}", err.Error())
		return RequirementInput{}, err
	}
	summary, questions, err := parseClarificationPayload(payload)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return RequirementInput{}, err
	}
	for index := range questions {
		if strings.TrimSpace(questions[index].ID) == "" {
			questions[index].ID = fmt.Sprintf("clarification-%d", index+1)
		}
	}
	requirement.Summary = normalizeRequirementSummary(summary)
	requirement.Clarifications = ensureSlice(questions)
	requirement.Answers = []ClarificationAnswer{}
	if err := s.repo.UpdateRequirementInput(ctx, requirement); err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return RequirementInput{}, err
	}
	_ = s.repo.FinishWorkflowRun(ctx, run.ID, "completed", payload, "")
	return normalizeRequirementInput(requirement), nil
}

func (s *WorkbenchService) AnswerClarifications(ctx context.Context, input AnswerClarificationsInput) (RequirementInput, error) {
	requirement, err := s.repo.GetRequirementInputByProject(ctx, input.ProjectID)
	if err != nil {
		return RequirementInput{}, err
	}
	answersByID := map[string]string{}
	for _, answer := range input.Answers {
		answersByID[answer.QuestionID] = strings.TrimSpace(answer.Answer)
	}
	for index := range requirement.Clarifications {
		requirement.Clarifications[index].Answer = answersByID[requirement.Clarifications[index].ID]
	}
	var answers []ClarificationAnswer
	for _, question := range requirement.Clarifications {
		if strings.TrimSpace(question.Answer) == "" {
			continue
		}
		answers = append(answers, ClarificationAnswer{
			QuestionID: question.ID,
			Answer:     question.Answer,
		})
	}
	requirement.Answers = answers
	if err := s.repo.UpdateRequirementInput(ctx, requirement); err != nil {
		return RequirementInput{}, err
	}
	return normalizeRequirementInput(requirement), nil
}

func (s *WorkbenchService) GenerateArtifact(ctx context.Context, input GenerateArtifactInput) (ArtifactVersion, error) {
	project, requirement, err := s.loadProjectAndOptionalRequirement(ctx, input.ProjectID)
	if err != nil {
		return ArtifactVersion{}, err
	}
	provider := s.aiManager.Provider(ctx)

	contextPayload, schemaName, sourceVersionID, err := s.buildArtifactContext(ctx, project, requirement, input.ArtifactType)
	if err != nil {
		return ArtifactVersion{}, err
	}
	run, err := s.repo.CreateWorkflowRun(ctx, input.ProjectID, "Generate"+string(input.ArtifactType), mustJSON(contextPayload))
	if err != nil {
		return ArtifactVersion{}, err
	}

	prompt := buildArtifactPrompt(schemaName, contextPayload)
	payload, err := provider.GenerateStructured(ctx, schemaName, prompt)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", "{}", err.Error())
		return ArtifactVersion{}, err
	}

	artifact, err := s.repo.EnsureArtifact(ctx, input.ProjectID, input.ArtifactType, artifactDisplayTitle(project.Name, input.ArtifactType))
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return ArtifactVersion{}, err
	}

	var version ArtifactVersion
	switch input.ArtifactType {
	case ArtifactTypePrototype:
		uiSchema, err := parseUISchemaJSON(payload)
		if err != nil {
			_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
			return ArtifactVersion{}, err
		}
		version = ArtifactVersion{
			SourceVersionID:   sourceVersionID,
			Status:            VersionStatusGenerated,
			StructuredJSON:    mustJSON(uiSchema),
			RenderedMarkdown:  renderUISchemaSummary(uiSchema),
			PlainTextSnapshot: renderUISchemaSummary(uiSchema),
			GenerationMeta:    mustJSON(buildArtifactGenerationMeta(provider.Descriptor(), schemaName, input.ArtifactType, contextPayload)),
		}
	default:
		document, err := parseDocumentJSON(payload)
		if err != nil {
			_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
			return ArtifactVersion{}, err
		}
		version = ArtifactVersion{
			SourceVersionID:   sourceVersionID,
			Status:            VersionStatusGenerated,
			StructuredJSON:    mustJSON(document),
			RenderedMarkdown:  renderDocumentMarkdown(document),
			PlainTextSnapshot: renderDocumentPlainText(document),
			GenerationMeta:    mustJSON(buildArtifactGenerationMeta(provider.Descriptor(), schemaName, input.ArtifactType, contextPayload)),
		}
	}

	version, err = s.repo.CreateArtifactVersion(ctx, artifact, version)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return ArtifactVersion{}, err
	}
	if err := s.repo.MarkDownstreamArtifactsStale(ctx, input.ProjectID, input.ArtifactType); err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return ArtifactVersion{}, err
	}
	_ = s.repo.FinishWorkflowRun(ctx, run.ID, "completed", version.StructuredJSON, "")
	return version, nil
}

func (s *WorkbenchService) GenerateUISchema(ctx context.Context, projectID string) (ArtifactVersion, error) {
	return s.GenerateArtifact(ctx, GenerateArtifactInput{
		ProjectID:    projectID,
		ArtifactType: ArtifactTypePrototype,
	})
}

func (s *WorkbenchService) UpdateArtifactSection(ctx context.Context, input UpdateArtifactSectionInput) (ArtifactVersion, error) {
	current, err := s.repo.GetArtifactVersion(ctx, input.VersionID)
	if err != nil {
		return ArtifactVersion{}, err
	}
	document, err := parseDocumentJSON(current.StructuredJSON)
	if err != nil {
		return ArtifactVersion{}, err
	}
	updated := false
	for index := range document.Sections {
		if document.Sections[index].ID != input.SectionID {
			continue
		}
		document.Sections[index].Body = strings.TrimSpace(input.Content)
		document.Sections[index].HTML = strings.TrimSpace(input.ContentHTML)
		updated = true
		break
	}
	if !updated {
		return ArtifactVersion{}, fmt.Errorf("section %s not found", input.SectionID)
	}
	if err := document.Validate(); err != nil {
		return ArtifactVersion{}, err
	}
	artifact, err := s.repo.GetArtifactByType(ctx, current.ProjectID, current.Type)
	if err != nil {
		return ArtifactVersion{}, err
	}
	nextVersion, err := s.repo.CreateArtifactVersion(ctx, artifact, ArtifactVersion{
		SourceVersionID:   current.SourceVersionID,
		Status:            VersionStatusDraft,
		StructuredJSON:    mustJSON(document),
		RenderedMarkdown:  renderDocumentMarkdown(document),
		PlainTextSnapshot: renderDocumentPlainText(document),
		GenerationMeta:    mustJSON(map[string]any{"manual_edit_of": current.ID}),
	})
	if err != nil {
		return ArtifactVersion{}, err
	}
	if err := s.repo.MarkDownstreamArtifactsStale(ctx, current.ProjectID, current.Type); err != nil {
		return ArtifactVersion{}, err
	}
	return nextVersion, nil
}

func (s *WorkbenchService) ApproveArtifactVersion(ctx context.Context, input ApproveArtifactVersionInput) (ArtifactVersion, error) {
	version, err := s.repo.GetArtifactVersion(ctx, input.VersionID)
	if err != nil {
		return ArtifactVersion{}, err
	}
	version.Status = VersionStatusApproved
	version.GenerationMeta = mustJSON(map[string]any{"approved_at": nowUTC().Format(timeLayout), "previous_meta": version.GenerationMeta})
	if err := s.repo.UpdateArtifactVersion(ctx, version); err != nil {
		return ArtifactVersion{}, err
	}
	return version, nil
}

func (s *WorkbenchService) RegenerateDownstreamArtifacts(ctx context.Context, input RegenerateDownstreamArtifactsInput) (WorkspaceSnapshot, error) {
	for _, artifactType := range downstreamArtifacts(input.SourceArtifactType) {
		if artifactType == ArtifactTypePrototype {
			if _, err := s.GenerateUISchema(ctx, input.ProjectID); err != nil {
				return WorkspaceSnapshot{}, err
			}
			continue
		}
		if _, err := s.GenerateArtifact(ctx, GenerateArtifactInput{ProjectID: input.ProjectID, ArtifactType: artifactType}); err != nil {
			return WorkspaceSnapshot{}, err
		}
	}
	return s.GetWorkspace(ctx, input.ProjectID)
}

func (s *WorkbenchService) GeneratePrototypeBundle(ctx context.Context, projectID string) (PrototypeBundle, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	version, err := s.repo.GetLatestArtifactVersion(ctx, projectID, ArtifactTypePrototype)
	if err != nil {
		return PrototypeBundle{}, err
	}
	uiSchema, err := parseUISchemaJSON(version.StructuredJSON)
	if err != nil {
		return PrototypeBundle{}, err
	}
	renderSpec := s.resolvePrototypeRenderBundleSpec(ctx, project, version, uiSchema, true)
	bundle := s.prototype.BuildBundle(project, uiSchema, renderSpec)
	bundle.Diagnostics = diagnosePrototypeRenderSpec(uiSchema, renderSpec)
	if report, ok := cachedPrototypeRepairReport(version.GenerationMeta); ok {
		bundle.RepairReport = &report
	}
	return normalizePrototypeBundle(bundle), nil
}

func (s *WorkbenchService) PreviewPrototype(ctx context.Context, projectID string) (PrototypeBundle, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	version, err := s.repo.GetLatestArtifactVersion(ctx, projectID, ArtifactTypePrototype)
	if err != nil {
		return PrototypeBundle{}, err
	}
	uiSchema, err := parseUISchemaJSON(version.StructuredJSON)
	if err != nil {
		return PrototypeBundle{}, err
	}
	renderSpec := s.resolvePrototypeRenderBundleSpec(ctx, project, version, uiSchema, false)
	bundle := s.prototype.BuildBundle(project, uiSchema, renderSpec)
	bundle.Diagnostics = diagnosePrototypeRenderSpec(uiSchema, renderSpec)
	if report, ok := cachedPrototypeRepairReport(version.GenerationMeta); ok {
		bundle.RepairReport = &report
	}
	return normalizePrototypeBundle(bundle), nil
}

func (s *WorkbenchService) ExportPrototype(ctx context.Context, input ExportPrototypeInput) (PrototypeBundle, error) {
	project, err := s.repo.GetProject(ctx, input.ProjectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	version, err := s.repo.GetLatestArtifactVersion(ctx, input.ProjectID, ArtifactTypePrototype)
	if err != nil {
		return PrototypeBundle{}, err
	}
	uiSchema, err := parseUISchemaJSON(version.StructuredJSON)
	if err != nil {
		return PrototypeBundle{}, err
	}
	renderSpec := s.resolvePrototypeRenderBundleSpec(ctx, project, version, uiSchema, false)
	bundle, err := s.prototype.ExportBundle(project, uiSchema, renderSpec, input.OutputDir)
	if err != nil {
		return PrototypeBundle{}, err
	}
	bundle.Diagnostics = diagnosePrototypeRenderSpec(uiSchema, renderSpec)
	if report, ok := cachedPrototypeRepairReport(version.GenerationMeta); ok {
		bundle.RepairReport = &report
	}
	return normalizePrototypeBundle(bundle), nil
}

func (s *WorkbenchService) RepairPrototype(ctx context.Context, input PrototypeRepairInput) (PrototypeBundle, error) {
	project, err := s.repo.GetProject(ctx, input.ProjectID)
	if err != nil {
		return PrototypeBundle{}, err
	}
	version, err := s.repo.GetLatestArtifactVersion(ctx, input.ProjectID, ArtifactTypePrototype)
	if err != nil {
		return PrototypeBundle{}, err
	}
	uiSchema, err := parseUISchemaJSON(version.StructuredJSON)
	if err != nil {
		return PrototypeBundle{}, err
	}
	artifact, err := s.repo.GetArtifactByType(ctx, input.ProjectID, ArtifactTypePrototype)
	if err != nil {
		return PrototypeBundle{}, err
	}

	currentSpec := s.resolvePrototypeRenderBundleSpec(ctx, project, version, uiSchema, false)
	currentDiagnostics := diagnosePrototypeRenderSpec(uiSchema, currentSpec)
	runInput := map[string]any{
		"repair_request":        input,
		"current_render_bundle": currentSpec,
		"diagnostics":           currentDiagnostics,
	}
	run, err := s.repo.CreateWorkflowRun(ctx, input.ProjectID, "RepairPrototypePreview", mustJSON(runInput))
	if err != nil {
		return PrototypeBundle{}, err
	}

	provider := s.aiManager.Provider(ctx)
	prompt := buildPrototypeRepairPrompt(project, uiSchema, currentSpec, input, currentDiagnostics, version.GenerationMeta)

	renderSpec := PrototypeRenderBundleSpec{}
	strategy := "ai_targeted_repair"
	payload, err := provider.GenerateStructured(ctx, "prototype_repair_bundle", prompt)
	if err == nil {
		renderSpec, err = parsePrototypeRenderBundleJSON(payload)
	}
	if err != nil || strings.TrimSpace(renderSpec.PreviewHTML) == "" || strings.TrimSpace(renderSpec.AppTSX) == "" {
		renderSpec = localPrototypeRepairBundle(project, uiSchema, currentSpec, input)
		strategy = "project_scoped_fallback_repair"
	}
	renderSpec = normalizePrototypeRenderBundleSpec(renderSpec)
	postDiagnostics := diagnosePrototypeRenderSpec(uiSchema, renderSpec)
	report := buildPrototypeRepairReport(input, strategy, postDiagnostics)

	nextVersion, err := s.repo.CreateArtifactVersion(ctx, artifact, ArtifactVersion{
		SourceVersionID:   version.ID,
		Status:            VersionStatusDraft,
		StructuredJSON:    version.StructuredJSON,
		RenderedMarkdown:  version.RenderedMarkdown,
		PlainTextSnapshot: version.PlainTextSnapshot,
		GenerationMeta:    mergePrototypeGenerationMeta(version.GenerationMeta, renderSpec, report, postDiagnostics),
	})
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", "{}", err.Error())
		return PrototypeBundle{}, err
	}

	output := map[string]any{
		"repair_version_id": nextVersion.ID,
		"strategy":          strategy,
		"diagnostics":       postDiagnostics,
	}
	_ = s.repo.FinishWorkflowRun(ctx, run.ID, "completed", mustJSON(output), "")

	bundle := s.prototype.BuildBundle(project, uiSchema, renderSpec)
	bundle.Diagnostics = postDiagnostics
	bundle.RepairReport = &report
	return normalizePrototypeBundle(bundle), nil
}

func (s *WorkbenchService) generatePrototypeRenderBundleSpec(ctx context.Context, project Project, version ArtifactVersion, uiSchema UISchema) PrototypeRenderBundleSpec {
	provider := s.aiManager.Provider(ctx)
	prompt := buildPrototypeRenderPrompt(project, uiSchema, version.GenerationMeta)
	payload, err := provider.GenerateStructured(ctx, "prototype_render_bundle", prompt)
	if err != nil {
		return buildFallbackPrototypeRenderBundle(project, uiSchema)
	}

	spec, err := parsePrototypeRenderBundleJSON(payload)
	if err != nil {
		return buildFallbackPrototypeRenderBundle(project, uiSchema)
	}
	if strings.TrimSpace(spec.PreviewHTML) == "" || strings.TrimSpace(spec.AppTSX) == "" {
		return buildFallbackPrototypeRenderBundle(project, uiSchema)
	}
	if strings.TrimSpace(spec.StylesCSS) == "" {
		spec.StylesCSS = prototypeStylesCSS()
	}
	return normalizePrototypeRenderBundleSpec(spec)
}

func (s *WorkbenchService) resolvePrototypeRenderBundleSpec(ctx context.Context, project Project, version ArtifactVersion, uiSchema UISchema, forceRefresh bool) PrototypeRenderBundleSpec {
	if !forceRefresh {
		if cached, ok := cachedPrototypeRenderBundleSpec(version.GenerationMeta); ok {
			return cached
		}
	}
	return s.generatePrototypeRenderBundleSpec(ctx, project, version, uiSchema)
}

func (s *WorkbenchService) GetProviderConfig(ctx context.Context) (ProviderConfig, error) {
	config, err := s.repo.GetProviderConfig(ctx)
	if err == nil {
		return config, nil
	}
	return ProviderConfig{
		Name:         "Default Provider",
		ProviderType: "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		Model:        "gpt-4.1-mini",
		APIFormat:    AIAPIFormatResponses,
		Enabled:      false,
		UseMock:      true,
	}, nil
}

func (s *WorkbenchService) SaveProviderConfig(ctx context.Context, config ProviderConfig) (ProviderConfig, error) {
	return s.repo.SaveProviderConfig(ctx, config)
}

func (s *WorkbenchService) TestProviderConfig(ctx context.Context, config ProviderConfig) (ProviderConnectionResult, error) {
	return s.aiManager.TestProviderConfig(ctx, config)
}

func (s *WorkbenchService) loadProjectAndRequirement(ctx context.Context, projectID string) (Project, RequirementInput, error) {
	project, requirement, err := s.loadProjectAndOptionalRequirement(ctx, projectID)
	if err != nil {
		return Project{}, RequirementInput{}, err
	}
	if requirement == nil {
		return Project{}, RequirementInput{}, errors.New("please save requirement input first")
	}
	return project, *requirement, nil
}

func (s *WorkbenchService) loadProjectAndOptionalRequirement(ctx context.Context, projectID string) (Project, *RequirementInput, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, nil, err
	}
	requirement, err := s.repo.GetRequirementInputByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project, nil, nil
		}
		return Project{}, nil, err
	}
	return project, &requirement, nil
}

func (s *WorkbenchService) buildArtifactContext(ctx context.Context, project Project, requirement *RequirementInput, artifactType ArtifactType) (map[string]any, string, string, error) {
	contextPayload := map[string]any{
		"project_name":        project.Name,
		"project_description": project.Description,
	}
	var (
		intakePtr *RequirementIntake
		briefPtr  *ConsensusBrief
		modelPtr  *RequirementModel
	)
	if requirement != nil {
		contextPayload["raw_input"] = requirement.RawInput
		contextPayload["summary"] = requirement.Summary
		contextPayload["clarifications"] = requirement.Clarifications
		contextPayload["clarification_answers"] = flattenClarificationAnswers(*requirement)
	}
	if intake, err := s.repo.GetLatestRequirementIntake(ctx, project.ID); err == nil {
		contextPayload["requirement_intake"] = intake
		intakePtr = &intake
	}
	if brief, err := s.repo.GetLatestConsensusBrief(ctx, project.ID); err == nil {
		contextPayload["consensus_brief"] = brief.Structured
		briefPtr = &brief
	}
	if model, err := s.repo.GetLatestRequirementModel(ctx, project.ID); err == nil {
		contextPayload["requirement_model"] = model
		modelPtr = &model
	}

	switch artifactType {
	case ArtifactTypePRD:
		sourceMaterial, ok, err := s.findPreferredSourceMaterialByType(ctx, project.ID, SourceMaterialTypePRDUpload)
		if err != nil {
			return nil, "", "", err
		}
		if requirement == nil && !ok {
			return nil, "", "", errors.New("please save requirement input or upload PRD first")
		}
		if ok {
			contextPayload["uploaded_prd_text"] = sourceMaterial.RawText
			contextPayload["uploaded_prd_title"] = sourceMaterial.Title
			contextPayload["uploaded_prd_source_type"] = sourceMaterial.SourceType
			if baseline := sourceMaterialPRDBaseline(sourceMaterial); len(baseline) > 0 {
				contextPayload["uploaded_prd_baseline"] = baseline
			}
			return contextPayload, "prd", sourceMaterial.ID, nil
		}
		return contextPayload, "prd", "", nil
	case ArtifactTypeFunctionalSpec:
		prdContent, sourceVersionID, imported, baseline, err := s.resolvePRDBaseline(ctx, project.ID)
		if err != nil {
			return nil, "", "", err
		}
		contextPayload["prd_markdown"] = prdContent
		if imported {
			contextPayload["prd_imported"] = true
		}
		if len(baseline) > 0 {
			contextPayload["uploaded_prd_baseline"] = baseline
			contextPayload["functional_spec_seed"] = buildFunctionalSpecSeed(project, baseline, intakePtr, briefPtr, modelPtr)
		}
		return contextPayload, "functional_spec", sourceVersionID, nil
	case ArtifactTypeTechnicalSpec:
		functional, err := s.repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypeFunctionalSpec)
		if err != nil {
			return nil, "", "", errors.New("generate functional spec before technical spec")
		}
		prdContent, _, _, baseline, err := s.resolvePRDBaseline(ctx, project.ID)
		if err == nil {
			contextPayload["prd_markdown"] = prdContent
			if len(baseline) > 0 {
				contextPayload["uploaded_prd_baseline"] = baseline
				functionalSeed := buildFunctionalSpecSeed(project, baseline, intakePtr, briefPtr, modelPtr)
				contextPayload["functional_spec_seed"] = functionalSeed
				contextPayload["technical_spec_seed"] = buildTechnicalSpecSeed(project, baseline, briefPtr, modelPtr, functionalSeed)
			}
		}
		contextPayload["functional_markdown"] = functional.RenderedMarkdown
		return contextPayload, "technical_spec", functional.ID, nil
	case ArtifactTypePrototype:
		functional, err := s.repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypeFunctionalSpec)
		if err != nil {
			return nil, "", "", errors.New("generate functional spec before UI schema")
		}
		technical, _ := s.repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypeTechnicalSpec)
		prd, _ := s.repo.GetLatestArtifactVersion(ctx, project.ID, ArtifactTypePRD)
		contextPayload["prd_snapshot"] = clipText(prd.PlainTextSnapshot, 1800)
		contextPayload["functional_snapshot"] = clipText(functional.PlainTextSnapshot, 2600)
		contextPayload["technical_snapshot"] = clipText(technical.PlainTextSnapshot, 1800)
		functionalSeed := readFunctionalSpecSeedFromVersion(functional)
		technicalSeed := TechnicalSpecSeed{}
		if technical.ID != "" {
			technicalSeed = readTechnicalSpecSeedFromVersion(technical)
		}
		if len(functionalSeed.Pages) > 0 || len(functionalSeed.Modules) > 0 {
			contextPayload["functional_spec_seed"] = functionalSeed
		}
		if len(technicalSeed.Entities) > 0 || len(technicalSeed.Interfaces) > 0 {
			contextPayload["technical_spec_seed"] = technicalSeed
		}
		uiSchemaSeed := buildUISchemaSeed(functionalSeed, technicalSeed)
		if len(uiSchemaSeed.Pages) > 0 || len(uiSchemaSeed.Actions) > 0 {
			contextPayload["ui_schema_seed"] = uiSchemaSeed
		}
		return contextPayload, "ui_schema", functional.ID, nil
	default:
		return nil, "", "", fmt.Errorf("unsupported artifact type %s", artifactType)
	}
}

func buildArtifactPrompt(schemaName string, contextPayload map[string]any) string {
	switch schemaName {
	case "prd":
		return strings.Join([]string{
			"请输出 ArtifactDocument JSON。",
			"如果 CONTEXT_JSON 中包含 uploaded_prd_text，说明用户已经提供了现成 PRD，请优先做标准化整理，而不是重新虚构需求。",
			"如果同时包含 uploaded_prd_baseline，请把它当作标准章节骨架，按这个骨架补齐正式 PRD。",
			"要求包含 artifactType、title、summary、sections、highlights、openQuestions、metadata。",
			"sections 中每项都包含 id、title、body、html。",
			mustJSON(contextPayload),
		}, "\n")
	case "functional_spec":
		return strings.Join([]string{
			"请输出功能规格说明的 ArtifactDocument JSON。",
			"如果 CONTEXT_JSON 中 prd_imported=true，说明上游 PRD 来自用户上传材料；请先完成 PRD 结构归一，再展开成功能规格。",
			"如果 CONTEXT_JSON 中同时有 uploaded_prd_baseline，请严格沿用这些标准章节去展开页面、字段、状态、异常和验收，不要跳回自由发挥。",
			"如果 CONTEXT_JSON 中存在 functional_spec_seed，请把它视为页面、模块、字段、状态和验收的强约束骨架。",
			"重点覆盖模块拆解、页面与流程、字段规则、状态规则、验收标准。",
			"metadata 里补充 pages 列表，便于后续生成 UISchema。",
			mustJSON(contextPayload),
		}, "\n")
	case "technical_spec":
		return strings.Join([]string{
			"请输出研发实现说明的 ArtifactDocument JSON。",
			"如果 CONTEXT_JSON 中存在 uploaded_prd_baseline，请把它视为上游标准需求骨架，确保技术拆解与这些章节保持追溯关系。",
			"如果 CONTEXT_JSON 中存在 technical_spec_seed，请优先按其中的实体、接口、状态流转、测试关注点展开。",
			"重点覆盖实体、接口、状态流转、非功能要求和任务拆解。",
			mustJSON(contextPayload),
		}, "\n")
	case "ui_schema":
		return strings.Join([]string{
			"请输出 UISchema JSON。",
			"必须包含 app_meta、routes、pages、components、forms、tables、actions、mock_data、state_variants、navigation_map。",
			"页面至少覆盖 dashboard、form、list、detail 四类结构。",
			"如果 CONTEXT_JSON 中存在 ui_schema_seed，请把它视为页面、动作、状态和 mock data 的强约束骨架。",
			"如果同时存在 functional_spec_seed 和 technical_spec_seed，请确保 UISchema 与它们的页面蓝图、接口动作和状态流转保持一致。",
			mustJSON(contextPayload),
		}, "\n")
	default:
		return mustJSON(contextPayload)
	}
}

func buildArtifactGenerationMeta(provider, schemaName string, artifactType ArtifactType, contextPayload map[string]any) map[string]any {
	meta := map[string]any{
		"provider": provider,
		"schema":   schemaName,
	}
	switch artifactType {
	case ArtifactTypeFunctionalSpec:
		if seed, ok := contextPayload["functional_spec_seed"]; ok {
			meta["execution_seed"] = map[string]any{
				"functional_spec_seed": seed,
			}
		}
	case ArtifactTypeTechnicalSpec:
		executionSeed := map[string]any{}
		if seed, ok := contextPayload["functional_spec_seed"]; ok {
			executionSeed["functional_spec_seed"] = seed
		}
		if seed, ok := contextPayload["technical_spec_seed"]; ok {
			executionSeed["technical_spec_seed"] = seed
		}
		if len(executionSeed) > 0 {
			meta["execution_seed"] = executionSeed
		}
	case ArtifactTypePrototype:
		executionSeed := map[string]any{}
		if seed, ok := contextPayload["functional_spec_seed"]; ok {
			executionSeed["functional_spec_seed"] = seed
		}
		if seed, ok := contextPayload["technical_spec_seed"]; ok {
			executionSeed["technical_spec_seed"] = seed
		}
		if seed, ok := contextPayload["ui_schema_seed"]; ok {
			executionSeed["ui_schema_seed"] = seed
		}
		if len(executionSeed) > 0 {
			meta["execution_seed"] = executionSeed
		}
	}
	return meta
}

func flattenClarificationAnswers(requirement RequirementInput) []string {
	var answers []string
	for _, answer := range requirement.Answers {
		if strings.TrimSpace(answer.Answer) == "" {
			continue
		}
		answers = append(answers, answer.Answer)
	}
	return answers
}

func (s *WorkbenchService) resolvePRDBaseline(ctx context.Context, projectID string) (string, string, bool, map[string]any, error) {
	prd, err := s.repo.GetLatestArtifactVersion(ctx, projectID, ArtifactTypePRD)
	if err == nil && strings.TrimSpace(prd.RenderedMarkdown) != "" {
		return prd.RenderedMarkdown, prd.ID, false, nil, nil
	}
	sourceMaterial, ok, sourceErr := s.findPreferredSourceMaterialByType(ctx, projectID, SourceMaterialTypePRDUpload)
	if sourceErr != nil {
		return "", "", false, nil, sourceErr
	}
	if ok && strings.TrimSpace(sourceMaterial.RawText) != "" {
		return sourceMaterialPRDBaselineMarkdown(sourceMaterial), sourceMaterial.ID, true, sourceMaterialPRDBaseline(sourceMaterial), nil
	}
	return "", "", false, nil, errors.New("generate PRD before functional spec")
}

func (s *WorkbenchService) findPreferredSourceMaterialByType(ctx context.Context, projectID string, sourceType SourceMaterialType) (SourceMaterial, bool, error) {
	materials, err := s.repo.ListSourceMaterials(ctx, projectID)
	if err != nil {
		return SourceMaterial{}, false, err
	}
	var fallback *SourceMaterial
	for index := range materials {
		material := materials[index]
		if material.SourceType != sourceType {
			continue
		}
		if material.IsPrimary {
			return material, true, nil
		}
		if fallback == nil {
			fallback = &material
		}
	}
	if fallback != nil {
		return *fallback, true, nil
	}
	return SourceMaterial{}, false, nil
}

func renderUISchemaSummary(schema UISchema) string {
	var lines []string
	lines = append(lines, "# "+schema.AppMeta.Name)
	if schema.AppMeta.Description != "" {
		lines = append(lines, schema.AppMeta.Description)
	}
	lines = append(lines, "## Routes")
	for _, route := range schema.Routes {
		lines = append(lines, fmt.Sprintf("- %s -> %s", route.Path, route.PageID))
	}
	lines = append(lines, "## Pages")
	for _, page := range schema.Pages {
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", page.Title, page.Type, page.Description))
	}
	return strings.Join(lines, "\n")
}
