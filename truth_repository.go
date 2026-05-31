package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (r *Repository) SaveSourceMaterial(ctx context.Context, input SaveSourceMaterialInput) (SourceMaterial, error) {
	now := nowUTC()
	material := SourceMaterial{
		ID:             strings.TrimSpace(input.ID),
		ProjectID:      input.ProjectID,
		SourceType:     input.SourceType,
		Title:          strings.TrimSpace(input.Title),
		RawText:        strings.TrimSpace(input.RawText),
		FileName:       strings.TrimSpace(input.FileName),
		MimeType:       strings.TrimSpace(input.MimeType),
		FilePath:       strings.TrimSpace(input.FilePath),
		ParsedContent:  ensureMap(input.ParsedContent),
		ExtractionMeta: ensureMap(input.ExtractionMeta),
		IsPrimary:      input.IsPrimary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if material.SourceType == "" {
		material.SourceType = SourceMaterialTypeOtherFile
	}
	if material.Title == "" {
		material.Title = "未命名材料"
	}
	if material.RawText == "" && material.FilePath == "" && material.FileName == "" {
		return SourceMaterial{}, errors.New("source material requires raw text or file metadata")
	}

	if material.ID == "" {
		material.ID = uuid.NewString()
		_, err := r.db.ExecContext(
			ctx,
			`INSERT INTO source_materials
			 (id, project_id, source_type, title, raw_text, file_name, mime_type, file_path, parsed_content_json, extraction_meta_json, is_primary, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			material.ID,
			material.ProjectID,
			string(material.SourceType),
			material.Title,
			material.RawText,
			material.FileName,
			material.MimeType,
			material.FilePath,
			mustJSON(material.ParsedContent),
			mustJSON(material.ExtractionMeta),
			boolToInt(material.IsPrimary),
			material.CreatedAt.Format(timeLayout),
			material.UpdatedAt.Format(timeLayout),
		)
		if err != nil {
			return SourceMaterial{}, err
		}
	} else {
		current, err := r.GetSourceMaterial(ctx, material.ID)
		if err != nil {
			return SourceMaterial{}, err
		}
		material.CreatedAt = current.CreatedAt
		_, err = r.db.ExecContext(
			ctx,
			`UPDATE source_materials
			 SET source_type = ?, title = ?, raw_text = ?, file_name = ?, mime_type = ?, file_path = ?, parsed_content_json = ?, extraction_meta_json = ?, is_primary = ?, updated_at = ?
			 WHERE id = ?`,
			string(material.SourceType),
			material.Title,
			material.RawText,
			material.FileName,
			material.MimeType,
			material.FilePath,
			mustJSON(material.ParsedContent),
			mustJSON(material.ExtractionMeta),
			boolToInt(material.IsPrimary),
			material.UpdatedAt.Format(timeLayout),
			material.ID,
		)
		if err != nil {
			return SourceMaterial{}, err
		}
	}

	if material.IsPrimary {
		if _, err := r.db.ExecContext(ctx, `UPDATE source_materials SET is_primary = 0 WHERE project_id = ? AND id <> ?`, material.ProjectID, material.ID); err != nil {
			return SourceMaterial{}, err
		}
	}
	if err := r.touchProject(ctx, material.ProjectID, material.UpdatedAt); err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func (r *Repository) UpsertAutoRequirementSourceMaterial(ctx context.Context, projectID, rawInput string) (SourceMaterial, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, source_type, title, raw_text, file_name, mime_type, file_path, parsed_content_json, extraction_meta_json, is_primary, created_at, updated_at
		 FROM source_materials
		 WHERE project_id = ? AND source_type = ? AND title = ?
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		projectID,
		string(SourceMaterialTypeFreeText),
		"需求输入",
	)
	material, err := scanSourceMaterial(row)
	if err == nil {
		return r.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
			ID:         material.ID,
			ProjectID:  projectID,
			SourceType: SourceMaterialTypeFreeText,
			Title:      "需求输入",
			RawText:    rawInput,
			IsPrimary:  true,
		})
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SourceMaterial{}, err
	}
	return r.SaveSourceMaterial(ctx, SaveSourceMaterialInput{
		ProjectID:  projectID,
		SourceType: SourceMaterialTypeFreeText,
		Title:      "需求输入",
		RawText:    rawInput,
		IsPrimary:  true,
	})
}

func (r *Repository) GetSourceMaterial(ctx context.Context, materialID string) (SourceMaterial, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, source_type, title, raw_text, file_name, mime_type, file_path, parsed_content_json, extraction_meta_json, is_primary, created_at, updated_at
		 FROM source_materials WHERE id = ?`,
		materialID,
	)
	return scanSourceMaterial(row)
}

func (r *Repository) ListSourceMaterials(ctx context.Context, projectID string) ([]SourceMaterial, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, project_id, source_type, title, raw_text, file_name, mime_type, file_path, parsed_content_json, extraction_meta_json, is_primary, created_at, updated_at
		 FROM source_materials
		 WHERE project_id = ?
		 ORDER BY is_primary DESC, updated_at DESC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	materials := make([]SourceMaterial, 0)
	for rows.Next() {
		material, err := scanSourceMaterial(rows)
		if err != nil {
			return nil, err
		}
		materials = append(materials, material)
	}
	return normalizeSourceMaterials(materials), rows.Err()
}

func (r *Repository) CreateRequirementIntake(ctx context.Context, intake RequirementIntake) (RequirementIntake, error) {
	now := nowUTC()
	intake.ID = uuid.NewString()
	intake.CreatedAt = now
	intake.UpdatedAt = now
	intake = normalizeRequirementIntake(intake)
	if strings.TrimSpace(intake.Status) == "" {
		intake.Status = "normalized"
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO requirement_intakes
		 (id, project_id, title, summary, source_material_ids_json, normalized_input_json, domain_guess, readiness_score, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		intake.ID,
		intake.ProjectID,
		intake.Title,
		intake.Summary,
		mustJSON(intake.SourceMaterialIDs),
		mustJSON(intake.NormalizedInput),
		intake.DomainGuess,
		intake.ReadinessScore,
		intake.Status,
		intake.CreatedAt.Format(timeLayout),
		intake.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return RequirementIntake{}, err
	}
	if err := r.touchProject(ctx, intake.ProjectID, now); err != nil {
		return RequirementIntake{}, err
	}
	return intake, nil
}

func (r *Repository) GetLatestRequirementIntake(ctx context.Context, projectID string) (RequirementIntake, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, title, summary, source_material_ids_json, normalized_input_json, domain_guess, readiness_score, status, created_at, updated_at
		 FROM requirement_intakes
		 WHERE project_id = ?
		 ORDER BY created_at DESC
		 LIMIT 1`,
		projectID,
	)
	return scanRequirementIntake(row)
}

func (r *Repository) CreateConsensusBriefVersion(ctx context.Context, brief ConsensusBrief) (ConsensusBrief, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number), 0) FROM consensus_briefs WHERE project_id = ?`, brief.ProjectID)
	if err := row.Scan(&brief.VersionNumber); err != nil {
		return ConsensusBrief{}, err
	}
	brief.VersionNumber++
	brief.ID = uuid.NewString()
	brief.CreatedAt = nowUTC()
	brief.UpdatedAt = brief.CreatedAt
	brief = normalizeConsensusBrief(brief)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO consensus_briefs
		 (id, project_id, requirement_intake_id, version_number, status, structured_json, rendered_markdown, quality_report_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		brief.ID,
		brief.ProjectID,
		brief.RequirementIntakeID,
		brief.VersionNumber,
		string(brief.Status),
		mustJSON(brief.Structured),
		brief.RenderedMarkdown,
		mustJSON(brief.QualityReport),
		brief.CreatedAt.Format(timeLayout),
		brief.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return ConsensusBrief{}, err
	}
	if err := r.touchProject(ctx, brief.ProjectID, brief.UpdatedAt); err != nil {
		return ConsensusBrief{}, err
	}
	return brief, nil
}

func (r *Repository) GetLatestConsensusBrief(ctx context.Context, projectID string) (ConsensusBrief, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, requirement_intake_id, version_number, status, structured_json, rendered_markdown, quality_report_json, created_at, updated_at
		 FROM consensus_briefs
		 WHERE project_id = ?
		 ORDER BY version_number DESC
		 LIMIT 1`,
		projectID,
	)
	return scanConsensusBrief(row)
}

func (r *Repository) CreateRequirementModelVersion(ctx context.Context, model RequirementModel) (RequirementModel, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_number), 0) FROM requirement_models WHERE project_id = ?`, model.ProjectID)
	if err := row.Scan(&model.VersionNumber); err != nil {
		return RequirementModel{}, err
	}
	model.VersionNumber++
	model.ID = uuid.NewString()
	model.CreatedAt = nowUTC()
	model.UpdatedAt = model.CreatedAt
	model = normalizeRequirementModel(model)
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO requirement_models
		 (id, project_id, consensus_brief_id, version_number, status, problem_definition_json, actors_json, goals_json, flows_json, scope_json, entities_json, rules_json, acceptance_json, constraints_json, traceability_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		model.ID,
		model.ProjectID,
		model.ConsensusBriefID,
		model.VersionNumber,
		string(model.Status),
		mustJSON(model.ProblemDefinition),
		mustJSON(model.Actors),
		mustJSON(model.Goals),
		mustJSON(model.Flows),
		mustJSON(model.Scope),
		mustJSON(model.Entities),
		mustJSON(model.Rules),
		mustJSON(model.AcceptanceCriteria),
		mustJSON(model.Constraints),
		mustJSON(model.Traceability),
		model.CreatedAt.Format(timeLayout),
		model.UpdatedAt.Format(timeLayout),
	)
	if err != nil {
		return RequirementModel{}, err
	}
	if err := r.touchProject(ctx, model.ProjectID, model.UpdatedAt); err != nil {
		return RequirementModel{}, err
	}
	return model, nil
}

func (r *Repository) GetLatestRequirementModel(ctx context.Context, projectID string) (RequirementModel, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, project_id, consensus_brief_id, version_number, status, problem_definition_json, actors_json, goals_json, flows_json, scope_json, entities_json, rules_json, acceptance_json, constraints_json, traceability_json, created_at, updated_at
		 FROM requirement_models
		 WHERE project_id = ?
		 ORDER BY version_number DESC
		 LIMIT 1`,
		projectID,
	)
	return scanRequirementModel(row)
}

func scanSourceMaterial(scanner interface{ Scan(dest ...any) error }) (SourceMaterial, error) {
	var (
		material                              SourceMaterial
		sourceType                            string
		parsedContentJSON, extractionMetaJSON string
		isPrimary                             int
		createdAt, updatedAt                  string
	)
	if err := scanner.Scan(
		&material.ID,
		&material.ProjectID,
		&sourceType,
		&material.Title,
		&material.RawText,
		&material.FileName,
		&material.MimeType,
		&material.FilePath,
		&parsedContentJSON,
		&extractionMetaJSON,
		&isPrimary,
		&createdAt,
		&updatedAt,
	); err != nil {
		return SourceMaterial{}, err
	}
	material.SourceType = SourceMaterialType(sourceType)
	material.IsPrimary = isPrimary == 1
	if err := decodeJSON(parsedContentJSON, &material.ParsedContent); err != nil {
		return SourceMaterial{}, err
	}
	if err := decodeJSON(extractionMetaJSON, &material.ExtractionMeta); err != nil {
		return SourceMaterial{}, err
	}
	var err error
	material.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return SourceMaterial{}, err
	}
	material.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return SourceMaterial{}, err
	}
	return normalizeSourceMaterial(material), nil
}

func scanRequirementIntake(scanner interface{ Scan(dest ...any) error }) (RequirementIntake, error) {
	var (
		intake                                RequirementIntake
		sourceMaterialIDsJSON, normalizedJSON string
		createdAt, updatedAt                  string
	)
	if err := scanner.Scan(
		&intake.ID,
		&intake.ProjectID,
		&intake.Title,
		&intake.Summary,
		&sourceMaterialIDsJSON,
		&normalizedJSON,
		&intake.DomainGuess,
		&intake.ReadinessScore,
		&intake.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		return RequirementIntake{}, err
	}
	if err := decodeJSON(sourceMaterialIDsJSON, &intake.SourceMaterialIDs); err != nil {
		return RequirementIntake{}, err
	}
	if err := decodeJSON(normalizedJSON, &intake.NormalizedInput); err != nil {
		return RequirementIntake{}, err
	}
	var err error
	intake.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return RequirementIntake{}, err
	}
	intake.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return RequirementIntake{}, err
	}
	return normalizeRequirementIntake(intake), nil
}

func scanConsensusBrief(scanner interface{ Scan(dest ...any) error }) (ConsensusBrief, error) {
	var (
		brief                             ConsensusBrief
		statusValue                       string
		structuredJSON, qualityReportJSON string
		createdAt, updatedAt              string
	)
	if err := scanner.Scan(
		&brief.ID,
		&brief.ProjectID,
		&brief.RequirementIntakeID,
		&brief.VersionNumber,
		&statusValue,
		&structuredJSON,
		&brief.RenderedMarkdown,
		&qualityReportJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return ConsensusBrief{}, err
	}
	brief.Status = VersionStatus(statusValue)
	if err := decodeJSON(structuredJSON, &brief.Structured); err != nil {
		return ConsensusBrief{}, err
	}
	if err := decodeJSON(qualityReportJSON, &brief.QualityReport); err != nil {
		return ConsensusBrief{}, err
	}
	var err error
	brief.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return ConsensusBrief{}, err
	}
	brief.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return ConsensusBrief{}, err
	}
	return normalizeConsensusBrief(brief), nil
}

func scanRequirementModel(scanner interface{ Scan(dest ...any) error }) (RequirementModel, error) {
	var (
		model                                                               RequirementModel
		statusValue                                                         string
		problemDefinitionJSON, actorsJSON, goalsJSON, flowsJSON             string
		scopeJSON, entitiesJSON, rulesJSON, acceptanceJSON, constraintsJSON string
		traceabilityJSON, createdAt, updatedAt                              string
	)
	if err := scanner.Scan(
		&model.ID,
		&model.ProjectID,
		&model.ConsensusBriefID,
		&model.VersionNumber,
		&statusValue,
		&problemDefinitionJSON,
		&actorsJSON,
		&goalsJSON,
		&flowsJSON,
		&scopeJSON,
		&entitiesJSON,
		&rulesJSON,
		&acceptanceJSON,
		&constraintsJSON,
		&traceabilityJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return RequirementModel{}, err
	}
	model.Status = VersionStatus(statusValue)
	if err := decodeJSON(problemDefinitionJSON, &model.ProblemDefinition); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(actorsJSON, &model.Actors); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(goalsJSON, &model.Goals); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(flowsJSON, &model.Flows); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(scopeJSON, &model.Scope); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(entitiesJSON, &model.Entities); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(rulesJSON, &model.Rules); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(acceptanceJSON, &model.AcceptanceCriteria); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(constraintsJSON, &model.Constraints); err != nil {
		return RequirementModel{}, err
	}
	if err := decodeJSON(traceabilityJSON, &model.Traceability); err != nil {
		return RequirementModel{}, err
	}
	var err error
	model.CreatedAt, err = parseStoredTime(createdAt)
	if err != nil {
		return RequirementModel{}, err
	}
	model.UpdatedAt, err = parseStoredTime(updatedAt)
	if err != nil {
		return RequirementModel{}, err
	}
	return normalizeRequirementModel(model), nil
}

func (r *Repository) LatestTruthLayer(ctx context.Context, projectID string) ([]SourceMaterial, *RequirementIntake, *ConsensusBrief, *RequirementModel, error) {
	materials, err := r.ListSourceMaterials(ctx, projectID)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var intakePtr *RequirementIntake
	if intake, err := r.GetLatestRequirementIntake(ctx, projectID); err == nil {
		intakePtr = &intake
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil, err
	}

	var briefPtr *ConsensusBrief
	if brief, err := r.GetLatestConsensusBrief(ctx, projectID); err == nil {
		briefPtr = &brief
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil, err
	}

	var modelPtr *RequirementModel
	if model, err := r.GetLatestRequirementModel(ctx, projectID); err == nil {
		modelPtr = &model
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil, err
	}

	return normalizeSourceMaterials(materials), intakePtr, briefPtr, modelPtr, nil
}

func (r *Repository) RequireLatestRequirementIntake(ctx context.Context, projectID string) (RequirementIntake, error) {
	intake, err := r.GetLatestRequirementIntake(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RequirementIntake{}, fmt.Errorf("build requirement intake before continuing")
		}
		return RequirementIntake{}, err
	}
	return intake, nil
}
