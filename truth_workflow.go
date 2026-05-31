package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *WorkbenchService) SaveSourceMaterial(ctx context.Context, input SaveSourceMaterialInput) (SourceMaterial, error) {
	input = enrichSourceMaterialInput(input)
	material, err := s.repo.SaveSourceMaterial(ctx, input)
	if err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func (s *WorkbenchService) SavePRDBaseline(ctx context.Context, input SavePRDBaselineInput) (SourceMaterial, error) {
	material, err := s.repo.GetSourceMaterial(ctx, strings.TrimSpace(input.SourceMaterialID))
	if err != nil {
		return SourceMaterial{}, err
	}
	if material.SourceType != SourceMaterialTypePRDUpload {
		return SourceMaterial{}, errors.New("prd baseline editing only supports uploaded PRD materials")
	}

	material = normalizeSourceMaterial(material)
	baseline := sourceMaterialPRDBaseline(material)
	if len(baseline) == 0 {
		baseline = buildPRDBaseline(extractStructuredSections(material.RawText), sourceMaterialKeyPoints(material))
	}
	material.ParsedContent["prd_baseline"] = mergePRDBaselineEdits(baseline, input.Sections)
	material.ExtractionMeta["baseline_editor"] = "manual-review"
	material.ExtractionMeta["baseline_edited_at"] = nowUTC().Format(timeLayout)

	saved, err := s.repo.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ID:             material.ID,
		ProjectID:      material.ProjectID,
		SourceType:     material.SourceType,
		Title:          material.Title,
		RawText:        material.RawText,
		FileName:       material.FileName,
		MimeType:       material.MimeType,
		FilePath:       material.FilePath,
		ParsedContent:  material.ParsedContent,
		ExtractionMeta: material.ExtractionMeta,
		IsPrimary:      material.IsPrimary,
	})
	if err != nil {
		return SourceMaterial{}, err
	}
	if _, err := s.BuildRequirementIntake(ctx, BuildRequirementIntakeInput{ProjectID: material.ProjectID}); err != nil {
		return SourceMaterial{}, err
	}
	if err := s.repo.MarkAllArtifactsStale(ctx, material.ProjectID); err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(saved), nil
}

func (s *WorkbenchService) ListSourceMaterials(ctx context.Context, projectID string) ([]SourceMaterial, error) {
	materials, err := s.repo.ListSourceMaterials(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return normalizeSourceMaterials(materials), nil
}

func (s *WorkbenchService) BuildRequirementIntake(ctx context.Context, input BuildRequirementIntakeInput) (RequirementIntake, error) {
	project, err := s.repo.GetProject(ctx, input.ProjectID)
	if err != nil {
		return RequirementIntake{}, err
	}

	materials, err := s.repo.ListSourceMaterials(ctx, input.ProjectID)
	if err != nil {
		return RequirementIntake{}, err
	}
	if len(materials) == 0 {
		requirement, err := s.repo.GetRequirementInputByProject(ctx, input.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return RequirementIntake{}, errors.New("save requirement input or source materials before building intake")
			}
			return RequirementIntake{}, err
		}
		if _, err := s.repo.UpsertAutoRequirementSourceMaterial(ctx, input.ProjectID, requirement.RawInput); err != nil {
			return RequirementIntake{}, err
		}
		materials, err = s.repo.ListSourceMaterials(ctx, input.ProjectID)
		if err != nil {
			return RequirementIntake{}, err
		}
	}
	if len(materials) == 0 {
		return RequirementIntake{}, errors.New("requirement intake needs at least one source material")
	}

	intake := buildRequirementIntake(project, input.Title, materials)
	intake, err = s.repo.CreateRequirementIntake(ctx, intake)
	if err != nil {
		return RequirementIntake{}, err
	}
	return normalizeRequirementIntake(intake), nil
}

func (s *WorkbenchService) GenerateConsensusBrief(ctx context.Context, projectID string) (ConsensusBrief, error) {
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return ConsensusBrief{}, err
	}
	intake, err := s.repo.GetLatestRequirementIntake(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			intake, err = s.BuildRequirementIntake(ctx, BuildRequirementIntakeInput{ProjectID: projectID})
			if err != nil {
				return ConsensusBrief{}, err
			}
		} else {
			return ConsensusBrief{}, err
		}
	}

	requirement, err := s.repo.GetRequirementInputByProject(ctx, projectID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ConsensusBrief{}, err
	}

	contextPayload := map[string]any{
		"project_name":          project.Name,
		"project_description":   project.Description,
		"requirement_intake":    intake,
		"summary":               requirement.Summary,
		"clarifications":        requirement.Clarifications,
		"clarification_answers": flattenClarificationAnswers(requirement),
	}
	prompt := strings.Join([]string{
		"请基于以下 CONTEXT_JSON，输出需求共识稿 JSON。",
		"必须包含 problem_statement、target_users、business_goals、success_metrics、core_scenarios、in_scope_items、out_of_scope_items、risks、dependencies、assumptions。",
		"不要输出 markdown，只输出 JSON。",
		mustJSON(contextPayload),
	}, "\n")

	run, err := s.repo.CreateWorkflowRun(ctx, projectID, "GenerateConsensusBrief", mustJSON(contextPayload))
	if err != nil {
		return ConsensusBrief{}, err
	}

	payload, err := s.aiManager.Provider(ctx).GenerateStructured(ctx, "consensus_brief", prompt)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", "{}", err.Error())
		return ConsensusBrief{}, err
	}
	structured, err := parseConsensusBriefJSON(payload)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return ConsensusBrief{}, err
	}

	brief, err := s.repo.CreateConsensusBriefVersion(ctx, ConsensusBrief{
		ProjectID:           projectID,
		RequirementIntakeID: intake.ID,
		Status:              VersionStatusGenerated,
		Structured:          structured,
		RenderedMarkdown:    renderConsensusBriefMarkdown(structured),
		QualityReport:       buildConsensusQualityReport(structured),
	})
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return ConsensusBrief{}, err
	}
	_ = s.repo.FinishWorkflowRun(ctx, run.ID, "completed", mustJSON(brief.Structured), "")
	return normalizeConsensusBrief(brief), nil
}

func (s *WorkbenchService) BuildRequirementModel(ctx context.Context, projectID string) (RequirementModel, error) {
	brief, err := s.repo.GetLatestConsensusBrief(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RequirementModel{}, errors.New("generate consensus brief before requirement model")
		}
		return RequirementModel{}, err
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return RequirementModel{}, err
	}

	contextPayload := map[string]any{
		"project_name":        project.Name,
		"project_description": project.Description,
		"consensus_brief":     brief.Structured,
	}
	prompt := strings.Join([]string{
		"请把以下需求共识稿转成 RequirementModel JSON。",
		"必须包含 problem_definition、actors、goals、flows、scope、entities、rules、acceptance_criteria、constraints、traceability。",
		"不要输出任何说明，只输出 JSON。",
		mustJSON(contextPayload),
	}, "\n")

	run, err := s.repo.CreateWorkflowRun(ctx, projectID, "BuildRequirementModel", mustJSON(contextPayload))
	if err != nil {
		return RequirementModel{}, err
	}

	payload, err := s.aiManager.Provider(ctx).GenerateStructured(ctx, "requirement_model", prompt)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", "{}", err.Error())
		return RequirementModel{}, err
	}
	model, err := parseRequirementModelJSON(payload)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return RequirementModel{}, err
	}
	model.ProjectID = projectID
	model.ConsensusBriefID = brief.ID
	model.Status = VersionStatusGenerated
	model.Traceability = ensureMap(model.Traceability)
	model.Traceability["consensusBriefId"] = brief.ID
	model.Traceability["consensusVersion"] = brief.VersionNumber

	model, err = s.repo.CreateRequirementModelVersion(ctx, model)
	if err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return RequirementModel{}, err
	}
	if err := s.repo.MarkAllArtifactsStale(ctx, projectID); err != nil {
		_ = s.repo.FinishWorkflowRun(ctx, run.ID, "failed", payload, err.Error())
		return RequirementModel{}, err
	}
	_ = s.repo.FinishWorkflowRun(ctx, run.ID, "completed", mustJSON(model), "")
	return normalizeRequirementModel(model), nil
}

func buildRequirementIntake(project Project, title string, materials []SourceMaterial) RequirementIntake {
	materials = normalizeSourceMaterials(materials)

	var (
		combinedParts []string
		sourceTypes   []string
		sourceIDs     []string
		materialViews []map[string]any
		prdBaselines  []map[string]any
		parsedCount   int
		totalSections int
		importedPRDs  int
	)
	primaryTitle := ""
	primaryHeadings := []string{}
	for _, material := range materials {
		if material.IsPrimary && primaryTitle == "" {
			primaryTitle = material.Title
			primaryHeadings = sourceMaterialHeadingTitles(material)
		}
		if strings.TrimSpace(material.RawText) != "" {
			combinedParts = append(combinedParts, material.RawText)
		}
		headings := sourceMaterialHeadingTitles(material)
		sectionCount := sourceMaterialSectionCount(material)
		keyPoints := sourceMaterialKeyPoints(material)
		documentKind := sourceMaterialDocumentKind(material)
		prdBaseline := sourceMaterialPRDBaseline(material)
		if sectionCount > 0 {
			parsedCount++
			totalSections += sectionCount
		}
		if material.SourceType == SourceMaterialTypePRDUpload {
			importedPRDs++
		}
		sourceTypes = append(sourceTypes, string(material.SourceType))
		sourceIDs = append(sourceIDs, material.ID)
		materialViews = append(materialViews, map[string]any{
			"id":           material.ID,
			"title":        material.Title,
			"sourceType":   material.SourceType,
			"isPrimary":    material.IsPrimary,
			"excerpt":      clipText(material.RawText, 160),
			"sectionCount": sectionCount,
			"headings":     headings,
			"keyPoints":    keyPoints,
			"documentKind": documentKind,
		})
		if len(prdBaseline) > 0 {
			materialViews[len(materialViews)-1]["prdBaseline"] = prdBaseline
			prdBaselines = append(prdBaselines, map[string]any{
				"sourceMaterialID": material.ID,
				"title":            material.Title,
				"completeness":     sourceMaterialPRDBaselineCompleteness(material),
				"summary":          prdBaseline["summary"],
				"presentKeys":      prdBaseline["present_keys"],
				"missingKeys":      prdBaseline["missing_keys"],
			})
		}
	}

	combinedText := strings.TrimSpace(strings.Join(combinedParts, "\n\n"))
	if title == "" {
		title = project.Name + " 需求入口"
	}
	if primaryTitle == "" && len(materials) > 0 {
		primaryTitle = materials[0].Title
	}

	readinessScore := 35
	if len(materials) > 1 {
		readinessScore += 15
	}
	if len(combinedText) > 120 {
		readinessScore += 20
	}
	if len(combinedText) > 400 {
		readinessScore += 10
	}
	if primaryTitle != "" {
		readinessScore += 10
	}
	if parsedCount > 0 {
		readinessScore += 10
	}
	if totalSections >= 4 {
		readinessScore += 5
	}
	if importedPRDs > 0 {
		readinessScore += 10
	}
	if readinessScore > 100 {
		readinessScore = 100
	}

	summary := clipText(combinedText, 220)
	if len(materials) > 0 {
		summaryParts := []string{fmt.Sprintf("已归一化 %d 份材料", len(materials))}
		if importedPRDs > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("其中包含 %d 份 PRD", importedPRDs))
		}
		if parsedCount > 0 {
			summaryParts = append(summaryParts, fmt.Sprintf("自动识别出 %d 个结构化章节", totalSections))
		}
		if primaryTitle != "" {
			summaryParts = append(summaryParts, fmt.Sprintf("主来源：%s", primaryTitle))
		}
		summary = strings.Join(summaryParts, "，") + "。"
	}

	normalized := map[string]any{
		"project_name":          project.Name,
		"project_description":   project.Description,
		"primary_source_title":  primaryTitle,
		"primary_headings":      primaryHeadings,
		"source_count":          len(materials),
		"source_types":          uniqueStrings(sourceTypes),
		"parsed_material_count": parsedCount,
		"total_section_count":   totalSections,
		"imported_prd_count":    importedPRDs,
		"prd_baselines":         prdBaselines,
		"combined_text":         combinedText,
		"materials":             materialViews,
	}
	return RequirementIntake{
		ProjectID:         project.ID,
		Title:             title,
		Summary:           summary,
		SourceMaterialIDs: uniqueStrings(sourceIDs),
		NormalizedInput:   normalized,
		DomainGuess:       guessDomain(project.Description + "\n" + combinedText),
		ReadinessScore:    readinessScore,
		Status:            "normalized",
	}
}

func guessDomain(input string) string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	switch {
	case strings.Contains(normalized, "电商"):
		return "ecommerce"
	case strings.Contains(normalized, "crm"), strings.Contains(normalized, "销售"):
		return "crm"
	case strings.Contains(normalized, "后台"), strings.Contains(normalized, "管理"):
		return "management"
	case strings.Contains(normalized, "saas"):
		return "saas"
	default:
		return "general"
	}
}
